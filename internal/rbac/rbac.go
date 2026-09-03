// Package rbac implements Faucet's role-based access control: it decides
// whether an authenticated principal may perform a given HTTP verb on a
// component (table, schema, procedure, ...) of a database service.
//
// Access is evaluated against the role bound to an API key. A role carries a
// list of access rules (model.RoleAccess); a request is allowed when at least
// one rule matches the service and component and its verb_mask contains the
// requested verb. Anything that does not match is denied (fail closed):
//
//   - a role with no access rules grants nothing
//   - an inactive role grants nothing
//   - a rule with verb_mask=0 grants nothing
//
// Admin principals (JWT sessions) bypass RBAC entirely.
package rbac

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
)

// ---------------------------------------------------------------------------
// Principal
// ---------------------------------------------------------------------------

// Principal types.
const (
	PrincipalAdmin  = "admin"
	PrincipalAPIKey = "api_key"
)

// Principal represents the authenticated identity making a request.
type Principal struct {
	Type    string // PrincipalAdmin or PrincipalAPIKey
	AdminID int64
	KeyID   int64
	RoleID  int64
	IsAdmin bool
}

type contextKey string

// PrincipalContextKey is the context key under which the authenticated
// principal is stored.
const PrincipalContextKey contextKey = "auth_principal"

// WithPrincipal returns a copy of ctx carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, PrincipalContextKey, p)
}

// PrincipalFromContext extracts the authenticated principal from ctx.
// It returns nil when no principal is present (unauthenticated request).
func PrincipalFromContext(ctx context.Context) *Principal {
	if p, ok := ctx.Value(PrincipalContextKey).(*Principal); ok {
		return p
	}
	return nil
}

// ---------------------------------------------------------------------------
// Verbs
// ---------------------------------------------------------------------------

var verbNames = []struct {
	bit  int
	name string
}{
	{model.VerbGet, http.MethodGet},
	{model.VerbPost, http.MethodPost},
	{model.VerbPut, http.MethodPut},
	{model.VerbPatch, http.MethodPatch},
	{model.VerbDelete, http.MethodDelete},
}

// VerbFromMethod maps an HTTP method to its verb bit. HEAD is treated as GET.
// The second return value is false for methods that have no verb bit.
func VerbFromMethod(method string) (int, bool) {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		return model.VerbGet, true
	case http.MethodPost:
		return model.VerbPost, true
	case http.MethodPut:
		return model.VerbPut, true
	case http.MethodPatch:
		return model.VerbPatch, true
	case http.MethodDelete:
		return model.VerbDelete, true
	}
	return 0, false
}

// VerbName returns the HTTP method name for a single verb bit, or a
// comma-separated list for a multi-bit mask. Unknown bits are ignored.
func VerbName(mask int) string {
	names := VerbNames(mask)
	if len(names) == 0 {
		return "NONE"
	}
	return strings.Join(names, ",")
}

// VerbNames returns the HTTP method names present in mask, in canonical order.
func VerbNames(mask int) []string {
	var out []string
	for _, v := range verbNames {
		if mask&v.bit != 0 {
			out = append(out, v.name)
		}
	}
	return out
}

// ParseVerbs converts a comma-separated list of HTTP method names into a
// verb mask. "*" and "all" (case-insensitive) expand to every verb.
// Whitespace around names is ignored. An empty result is an error, so a
// caller can never accidentally persist a rule that grants nothing.
func ParseVerbs(spec string) (int, error) {
	mask := 0
	for _, raw := range strings.Split(spec, ",") {
		name := strings.ToUpper(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if name == "*" || name == "ALL" {
			return model.VerbAll, nil
		}
		bit, ok := VerbFromMethod(name)
		if !ok || name == http.MethodHead {
			return 0, fmt.Errorf("unknown verb %q (expected GET, POST, PUT, PATCH, DELETE or *)", strings.TrimSpace(raw))
		}
		mask |= bit
	}
	if mask == 0 {
		return 0, errors.New("no verbs specified (expected GET, POST, PUT, PATCH, DELETE or *)")
	}
	return mask, nil
}

// ---------------------------------------------------------------------------
// Rule matching
// ---------------------------------------------------------------------------

// Wildcard matches every service or component.
const Wildcard = "*"

// NormalizeComponent turns a route path such as "/_table/customers/" into
// the canonical component form "_table/customers". Only the first two path
// segments are significant; anything deeper is dropped. The empty string is
// returned for an empty path.
func NormalizeComponent(routePath string) string {
	p := strings.Trim(strings.TrimSpace(routePath), "/")
	if p == "" {
		return ""
	}
	segs := strings.Split(p, "/")
	if len(segs) > 2 {
		segs = segs[:2]
	}
	return strings.Join(segs, "/")
}

// MatchService reports whether a rule's service pattern matches service.
// Supported patterns: "*" (or empty) for all services, an exact name
// (case-insensitive), or a prefix followed by "*" (e.g. "prod_*").
func MatchService(pattern, service string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" || p == Wildcard {
		return true
	}
	if strings.HasSuffix(p, Wildcard) {
		return hasPrefixFold(service, strings.TrimSuffix(p, Wildcard))
	}
	return strings.EqualFold(p, service)
}

// MatchComponent reports whether a rule's component pattern matches the
// (normalized) request component. Supported patterns, all case-insensitive:
//
//   - "*" or "" matches everything
//   - an exact component, e.g. "_table/customers" or "_table"
//   - a prefix wildcard, e.g. "_table/*" (all tables, and the "_table"
//     listing itself) or "_table/cust*"
//   - a bare resource name, e.g. "customers", which matches that resource
//     under any collection ("_table/customers", "_schema/customers", ...)
func MatchComponent(pattern, component string) bool {
	p := strings.Trim(strings.TrimSpace(pattern), "/")
	c := NormalizeComponent(component)
	if p == "" || p == Wildcard {
		return true
	}
	if strings.EqualFold(p, c) {
		return true
	}
	if strings.HasSuffix(p, Wildcard) {
		prefix := strings.TrimSuffix(p, Wildcard)
		if hasPrefixFold(c, prefix) {
			return true
		}
		// "_table/*" also covers the bare collection "_table".
		if strings.HasSuffix(prefix, "/") && strings.EqualFold(strings.TrimSuffix(prefix, "/"), c) {
			return true
		}
		return false
	}
	if !strings.Contains(p, "/") {
		if i := strings.Index(c, "/"); i >= 0 && strings.EqualFold(c[i+1:], p) {
			return true
		}
	}
	return false
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// Allowed reports whether the access rules grant every verb bit in verb on
// the given service and component. A single rule must cover all requested
// bits. Rules with verb_mask=0 never match.
func Allowed(access []model.RoleAccess, service, component string, verb int) bool {
	if verb == 0 {
		return false
	}
	for _, rule := range access {
		if rule.VerbMask&verb != verb {
			continue
		}
		if !MatchService(rule.ServiceName, service) {
			continue
		}
		if !MatchComponent(rule.Component, component) {
			continue
		}
		return true
	}
	return false
}

// CanAccessService reports whether the access rules grant any verb on any
// component of service. It is used to filter service listings.
func CanAccessService(access []model.RoleAccess, service string) bool {
	for _, rule := range access {
		if rule.VerbMask&model.VerbAll != 0 && MatchService(rule.ServiceName, service) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Enforcement
// ---------------------------------------------------------------------------

// ErrForbidden is the sentinel wrapped by every Denial.
var ErrForbidden = errors.New("forbidden")

// Denial describes why a request was refused. It unwraps to ErrForbidden.
type Denial struct {
	Reason    string
	Role      string
	Service   string
	Component string
	Verb      int
}

func (d *Denial) Error() string { return d.Reason }

// Unwrap lets errors.Is(err, ErrForbidden) succeed.
func (d *Denial) Unwrap() error { return ErrForbidden }

// RoleLoader loads a role (including its access rules) by ID. *config.Store
// satisfies this interface.
type RoleLoader interface {
	GetRole(ctx context.Context, id int64) (*model.Role, error)
}

// Enforcer evaluates access decisions for principals.
type Enforcer struct {
	roles RoleLoader
}

// NewEnforcer creates an Enforcer backed by the given role loader.
func NewEnforcer(roles RoleLoader) *Enforcer {
	return &Enforcer{roles: roles}
}

// Role loads the role bound to an API-key principal. It returns a *Denial
// when the principal is missing, is not an API key, or its role does not
// exist or is inactive. Any other error is a loader failure.
func (e *Enforcer) Role(ctx context.Context, p *Principal) (*model.Role, error) {
	if p == nil {
		return nil, &Denial{Reason: "Authentication required"}
	}
	if p.Type != PrincipalAPIKey {
		return nil, &Denial{Reason: "Unsupported principal type"}
	}
	role, err := e.roles.GetRole(ctx, p.RoleID)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return nil, &Denial{Reason: "API key is bound to a role that no longer exists"}
		}
		return nil, fmt.Errorf("load role %d: %w", p.RoleID, err)
	}
	if !role.IsActive {
		return nil, &Denial{Reason: fmt.Sprintf("Role %q is inactive", role.Name), Role: role.Name}
	}
	return role, nil
}

// Authorize decides whether p may perform verb on service/component.
// It returns nil when allowed, a *Denial when refused, or another error if
// the role could not be loaded. Admin principals are always allowed; a nil
// principal is always refused.
func (e *Enforcer) Authorize(ctx context.Context, p *Principal, service, component string, verb int) error {
	if p == nil {
		return &Denial{Reason: "Authentication required", Service: service, Component: component, Verb: verb}
	}
	if p.IsAdmin {
		return nil
	}
	role, err := e.Role(ctx, p)
	if err != nil {
		var d *Denial
		if errors.As(err, &d) {
			d.Service, d.Component, d.Verb = service, component, verb
		}
		return err
	}
	component = NormalizeComponent(component)
	if !Allowed(role.Access, service, component, verb) {
		return &Denial{
			Reason: fmt.Sprintf("Role %q does not permit %s on %s/%s",
				role.Name, VerbName(verb), service, component),
			Role:      role.Name,
			Service:   service,
			Component: component,
			Verb:      verb,
		}
	}
	return nil
}

// Context returns a *Denial's structured fields as a map suitable for the
// "context" member of Faucet's error envelope.
func (d *Denial) Context() map[string]interface{} {
	m := map[string]interface{}{}
	if d.Role != "" {
		m["role"] = d.Role
	}
	if d.Service != "" {
		m["service"] = d.Service
	}
	if d.Component != "" {
		m["component"] = d.Component
	}
	if d.Verb != 0 {
		m["verb"] = VerbName(d.Verb)
	}
	return m
}
