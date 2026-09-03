package model

import (
	"strings"
	"time"
)

// Role defines an RBAC role that groups a set of access rules together.
// API keys are bound to roles to determine what operations they can perform.
type Role struct {
	ID          int64        `json:"id" db:"id"`
	Name        string       `json:"name" db:"name"`
	Description string       `json:"description" db:"description"`
	IsActive    bool         `json:"is_active" db:"is_active"`
	Access      []RoleAccess `json:"access"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
}

// RoleAccess defines a single access rule within a role, controlling which
// HTTP verbs are allowed on a specific service component.
type RoleAccess struct {
	ID            int64    `json:"id" db:"id"`
	RoleID        int64    `json:"role_id" db:"role_id"`
	ServiceName   string   `json:"service_name" db:"service_name"`
	Component     string   `json:"component" db:"component"`
	VerbMask      int      `json:"verb_mask" db:"verb_mask"`
	RequestorMask int      `json:"requestor_mask" db:"requestor_mask"`
	Filters       []Filter `json:"filters"`
	FilterOp      string   `json:"filter_op" db:"filter_op"`
}

// Filter defines a row-level filter applied to a role access rule.
type Filter struct {
	Name     string `json:"name"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// Verb mask constants define which HTTP methods are allowed.
const (
	VerbGet    = 1
	VerbPost   = 2
	VerbPut    = 4
	VerbPatch  = 8
	VerbDelete = 16
	VerbAll    = VerbGet | VerbPost | VerbPut | VerbPatch | VerbDelete
)

// Requestor mask constants define the source of the request.
const (
	RequestorAPI    = 1
	RequestorScript = 2
	RequestorAdmin  = 4
)

// VerbFromMethod maps an HTTP method to its verb mask bit. Unknown methods
// map to 0, which no access rule can grant.
func VerbFromMethod(method string) int {
	switch method {
	case "GET", "HEAD":
		return VerbGet
	case "POST":
		return VerbPost
	case "PUT":
		return VerbPut
	case "PATCH":
		return VerbPatch
	case "DELETE":
		return VerbDelete
	default:
		return 0
	}
}

// VerbName returns the HTTP method name for a single verb bit, for use in
// error messages.
func VerbName(verb int) string {
	switch verb {
	case VerbGet:
		return "GET"
	case VerbPost:
		return "POST"
	case VerbPut:
		return "PUT"
	case VerbPatch:
		return "PATCH"
	case VerbDelete:
		return "DELETE"
	case VerbAll:
		return "ALL"
	default:
		return "UNKNOWN"
	}
}

// Allows reports whether the role grants every bit in verb on the given
// service component. A component is the request path below the service,
// e.g. "_table/users", "_schema" or "_proc/refresh". Rules are additive: the
// request is allowed if any matching rule grants the verb. A role with no
// rules, or an inactive role, grants nothing.
func (r *Role) Allows(serviceName, component string, verb int) bool {
	if r == nil || !r.IsActive || verb == 0 {
		return false
	}
	for _, a := range r.Access {
		if !matchName(a.ServiceName, serviceName) || !MatchComponent(a.Component, component) {
			continue
		}
		if a.VerbMask&verb == verb {
			return true
		}
	}
	return false
}

// matchName matches a service name pattern: "*" matches any service,
// otherwise the names must be equal. An empty pattern matches nothing so a
// rule saved with a blank field can never widen access by accident.
func matchName(pattern, name string) bool {
	return pattern == "*" || (pattern != "" && pattern == name)
}

// MatchComponent matches a rule's component pattern against a request
// component. "*" matches everything and an empty pattern matches nothing. A
// pattern ending in "/*" matches the prefix itself and anything below it, so
// "_table/*" covers both the table list ("_table") and every table
// ("_table/users"). Any other pattern must match exactly.
func MatchComponent(pattern, component string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == "" {
		return false
	}
	if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
		return component == prefix || strings.HasPrefix(component, prefix+"/")
	}
	return pattern == component
}
