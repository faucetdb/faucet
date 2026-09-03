package rbac

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
)

// ---------------------------------------------------------------------------
// Verb helpers
// ---------------------------------------------------------------------------

func TestVerbFromMethod(t *testing.T) {
	tests := []struct {
		method string
		want   int
		ok     bool
	}{
		{http.MethodGet, model.VerbGet, true},
		{http.MethodHead, model.VerbGet, true},
		{http.MethodPost, model.VerbPost, true},
		{http.MethodPut, model.VerbPut, true},
		{http.MethodPatch, model.VerbPatch, true},
		{http.MethodDelete, model.VerbDelete, true},
		{"get", model.VerbGet, true},
		{"patch", model.VerbPatch, true},
		{http.MethodOptions, 0, false},
		{"TRACE", 0, false},
		{"", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			got, ok := VerbFromMethod(tc.method)
			if got != tc.want || ok != tc.ok {
				t.Errorf("VerbFromMethod(%q) = (%d, %v), want (%d, %v)", tc.method, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseVerbs(t *testing.T) {
	tests := []struct {
		spec    string
		want    int
		wantErr bool
	}{
		{"GET", model.VerbGet, false},
		{"get, post", model.VerbGet | model.VerbPost, false},
		{" delete ,PATCH", model.VerbDelete | model.VerbPatch, false},
		{"*", model.VerbAll, false},
		{"all", model.VerbAll, false},
		{"ALL", model.VerbAll, false},
		{"GET,*", model.VerbAll, false},
		{"", 0, true},
		{" , ", 0, true},
		{"HEAD", 0, true},
		{"FOO", 0, true},
		{"GET,FOO", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			got, err := ParseVerbs(tc.spec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseVerbs(%q) error = %v, wantErr %v", tc.spec, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ParseVerbs(%q) = %d, want %d", tc.spec, got, tc.want)
			}
		})
	}
}

func TestVerbName(t *testing.T) {
	tests := []struct {
		mask int
		want string
	}{
		{model.VerbGet, "GET"},
		{model.VerbPost, "POST"},
		{model.VerbPut, "PUT"},
		{model.VerbPatch, "PATCH"},
		{model.VerbDelete, "DELETE"},
		{model.VerbAll, "GET,POST,PUT,PATCH,DELETE"},
		{model.VerbDelete | model.VerbGet, "GET,DELETE"},
		{0, "NONE"},
		{1 << 10, "NONE"}, // unknown bit is ignored
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := VerbName(tc.mask); got != tc.want {
				t.Errorf("VerbName(%d) = %q, want %q", tc.mask, got, tc.want)
			}
		})
	}
}

func TestVerbNames(t *testing.T) {
	tests := []struct {
		mask int
		want []string
	}{
		{model.VerbGet, []string{"GET"}},
		{model.VerbAll, []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
		{model.VerbPatch | model.VerbPost, []string{"POST", "PATCH"}},
		{0, nil},
	}
	for _, tc := range tests {
		got := VerbNames(tc.mask)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("VerbNames(%d) = %v, want %v", tc.mask, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Component / service matching
// ---------------------------------------------------------------------------

func TestNormalizeComponent(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/_table/customers/", "_table/customers"},
		{"_table/customers", "_table/customers"},
		{"/_table", "_table"},
		{"_table/", "_table"},
		{"", ""},
		{"   ", ""},
		{"/", ""},
		{"/_table/customers/123", "_table/customers"},
		{"_schema/customers/columns/id", "_schema/customers"},
		{"  /_proc/do_thing  ", "_proc/do_thing"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := NormalizeComponent(tc.in); got != tc.want {
				t.Errorf("NormalizeComponent(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMatchService(t *testing.T) {
	tests := []struct {
		pattern string
		service string
		want    bool
	}{
		{"*", "anything", true},
		{"", "anything", true},
		{"  ", "anything", true},
		{"mydb", "mydb", true},
		{"MyDB", "mydb", true},
		{"mydb", "MYDB", true},
		{"prod_*", "prod_orders", true},
		{"PROD_*", "prod_orders", true},
		{"prod_*", "prod_", true},
		{"prod_*", "staging_orders", false},
		{"prod_*", "prod", false},
		{"mydb", "otherdb", false},
		{"mydb", "mydb2", false},
		{"mydb", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+"/"+tc.service, func(t *testing.T) {
			if got := MatchService(tc.pattern, tc.service); got != tc.want {
				t.Errorf("MatchService(%q, %q) = %v, want %v", tc.pattern, tc.service, got, tc.want)
			}
		})
	}
}

func TestMatchComponent(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		component string
		want      bool
	}{
		// wildcard / empty
		{"star matches table", "*", "_table/customers", true},
		{"star matches listing", "*", "_table", true},
		{"star matches empty", "*", "", true},
		{"empty pattern matches", "", "_schema/x", true},

		// exact
		{"exact table", "_table/customers", "_table/customers", true},
		{"exact listing", "_table", "_table", true},
		{"exact case-insensitive pattern", "_TABLE/Customers", "_table/customers", true},
		{"exact case-insensitive component", "_table/customers", "_Table/CUSTOMERS", true},
		{"listing does not match table", "_table", "_table/customers", false},
		{"table does not match listing", "_table/customers", "_table", false},
		{"exact does not match other table", "_table/customers", "_table/orders", false},
		{"exact does not match prefix", "_table/customers", "_table/customers2", false},
		{"exact does not match other collection", "_table/customers", "_schema/customers", false},

		// prefix wildcard
		{"collection wildcard matches table", "_table/*", "_table/x", true},
		{"collection wildcard matches listing", "_table/*", "_table", true},
		{"collection wildcard case-insensitive", "_TABLE/*", "_table/x", true},
		{"collection wildcard does not match schema", "_table/*", "_schema", false},
		{"collection wildcard does not match schema table", "_table/*", "_schema/x", false},
		{"name prefix wildcard matches", "_table/cust*", "_table/customers", true},
		{"name prefix wildcard matches exact prefix", "_table/cust*", "_table/cust", true},
		{"name prefix wildcard case-insensitive", "_table/CUST*", "_table/customers", true},
		{"name prefix wildcard does not match other", "_table/cust*", "_table/orders", false},
		{"name prefix wildcard does not match listing", "_table/cust*", "_table", false},
		{"bare wildcard prefix", "_t*", "_table/customers", true},

		// bare resource name
		{"bare name matches table", "customers", "_table/customers", true},
		{"bare name matches schema", "customers", "_schema/customers", true},
		{"bare name case-insensitive", "Customers", "_table/CUSTOMERS", true},
		{"bare name does not match prefix", "customers", "_table/customers2", false},
		{"bare name does not match suffix", "customers", "_table/vip_customers", false},
		{"bare name does not match listing", "customers", "_table", false},
		{"bare name does not match collection", "_table", "_table/customers", false},

		// leading / trailing slashes
		{"pattern with slashes", "/_table/customers/", "_table/customers", true},
		{"component with slashes", "_table/customers", "/_table/customers/", true},
		{"both with slashes", "/_table/", "/_table/", true},
		{"wildcard pattern with slashes", "/_table/*/", "_table/x", true},
		{"bare name with slashes", "/customers/", "_table/customers", true},
		{"deep component is truncated", "_table/customers", "/_table/customers/42", true},

		// nothing matches nothing
		{"empty component exact", "_table", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchComponent(tc.pattern, tc.component); got != tc.want {
				t.Errorf("MatchComponent(%q, %q) = %v, want %v", tc.pattern, tc.component, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Allowed / CanAccessService
// ---------------------------------------------------------------------------

func rule(service, component string, mask int) model.RoleAccess {
	return model.RoleAccess{ServiceName: service, Component: component, VerbMask: mask}
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		name      string
		access    []model.RoleAccess
		service   string
		component string
		verb      int
		want      bool
	}{
		{
			name:   "no rules grants nothing",
			access: nil, service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: false,
		},
		{
			name:   "empty slice grants nothing",
			access: []model.RoleAccess{}, service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: false,
		},
		{
			name:    "verb 0 never allowed even with full access",
			access:  []model.RoleAccess{rule("*", "*", model.VerbAll)},
			service: "mydb", component: "_table/x", verb: 0,
			want: false,
		},
		{
			name:    "verb_mask 0 never matches",
			access:  []model.RoleAccess{rule("*", "*", 0)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: false,
		},
		{
			name:    "wildcard rule allows GET",
			access:  []model.RoleAccess{rule("*", "*", model.VerbGet)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: true,
		},
		{
			name:    "GET-only rule denies PATCH",
			access:  []model.RoleAccess{rule("*", "*", model.VerbGet)},
			service: "mydb", component: "_table/x", verb: model.VerbPatch,
			want: false,
		},
		{
			name:    "GET-only rule denies DELETE",
			access:  []model.RoleAccess{rule("*", "*", model.VerbGet)},
			service: "mydb", component: "_table/x", verb: model.VerbDelete,
			want: false,
		},
		{
			name:    "multi-bit verb needs a single rule covering all bits",
			access:  []model.RoleAccess{rule("*", "*", model.VerbGet), rule("*", "*", model.VerbPost)},
			service: "mydb", component: "_table/x", verb: model.VerbGet | model.VerbPost,
			want: false,
		},
		{
			name:    "multi-bit verb satisfied by one rule",
			access:  []model.RoleAccess{rule("*", "*", model.VerbGet|model.VerbPost)},
			service: "mydb", component: "_table/x", verb: model.VerbGet | model.VerbPost,
			want: true,
		},
		{
			name:    "multi-bit verb satisfied by VerbAll",
			access:  []model.RoleAccess{rule("*", "*", model.VerbAll)},
			service: "mydb", component: "_table/x", verb: model.VerbGet | model.VerbDelete,
			want: true,
		},
		{
			name: "first matching rule wins among several",
			access: []model.RoleAccess{
				rule("otherdb", "*", model.VerbAll),
				rule("mydb", "_schema/*", model.VerbAll),
				rule("mydb", "_table/x", model.VerbPatch),
				rule("mydb", "_table/x", model.VerbGet),
			},
			service: "mydb", component: "_table/x", verb: model.VerbPatch,
			want: true,
		},
		{
			name: "later rule matches when earlier ones do not",
			access: []model.RoleAccess{
				rule("mydb", "_table/x", model.VerbPatch),
				rule("mydb", "_table/x", model.VerbGet),
			},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: true,
		},
		{
			name:    "service mismatch",
			access:  []model.RoleAccess{rule("otherdb", "*", model.VerbAll)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: false,
		},
		{
			name:    "service prefix wildcard",
			access:  []model.RoleAccess{rule("my*", "*", model.VerbAll)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: true,
		},
		{
			name:    "component mismatch",
			access:  []model.RoleAccess{rule("mydb", "_table/y", model.VerbAll)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: false,
		},
		{
			name:    "component listing not covered by exact table rule",
			access:  []model.RoleAccess{rule("mydb", "_table/x", model.VerbAll)},
			service: "mydb", component: "_table", verb: model.VerbGet,
			want: false,
		},
		{
			name:    "component listing covered by collection wildcard",
			access:  []model.RoleAccess{rule("mydb", "_table/*", model.VerbAll)},
			service: "mydb", component: "_table", verb: model.VerbGet,
			want: true,
		},
		{
			name:    "un-normalized component input is normalized",
			access:  []model.RoleAccess{rule("mydb", "_table/x", model.VerbGet)},
			service: "mydb", component: "/_table/x/", verb: model.VerbGet,
			want: true,
		},
		{
			name:    "case-insensitive service and component",
			access:  []model.RoleAccess{rule("MYDB", "_TABLE/X", model.VerbGet)},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allowed(tc.access, tc.service, tc.component, tc.verb); got != tc.want {
				t.Errorf("Allowed(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanAccessService(t *testing.T) {
	tests := []struct {
		name    string
		access  []model.RoleAccess
		service string
		want    bool
	}{
		{"no rules", nil, "mydb", false},
		{"wildcard service", []model.RoleAccess{rule("*", "*", model.VerbGet)}, "mydb", true},
		{"exact service", []model.RoleAccess{rule("mydb", "_table/x", model.VerbGet)}, "mydb", true},
		{"exact service case-insensitive", []model.RoleAccess{rule("MyDB", "*", model.VerbGet)}, "mydb", true},
		{"prefix service", []model.RoleAccess{rule("my*", "*", model.VerbDelete)}, "mydb", true},
		{"other service only", []model.RoleAccess{rule("otherdb", "*", model.VerbAll)}, "mydb", false},
		{"verb_mask 0 grants nothing", []model.RoleAccess{rule("*", "*", 0)}, "mydb", false},
		{"unknown bits only grant nothing", []model.RoleAccess{rule("*", "*", 1<<10)}, "mydb", false},
		{
			"one of many rules matches",
			[]model.RoleAccess{rule("a", "*", model.VerbAll), rule("b", "*", 0), rule("mydb", "*", model.VerbPost)},
			"mydb", true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanAccessService(tc.access, tc.service); got != tc.want {
				t.Errorf("CanAccessService(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Principal context helpers
// ---------------------------------------------------------------------------

func TestPrincipalContext(t *testing.T) {
	if got := PrincipalFromContext(context.Background()); got != nil {
		t.Errorf("expected nil principal from bare context, got %+v", got)
	}

	p := &Principal{Type: PrincipalAPIKey, KeyID: 7, RoleID: 3}
	ctx := WithPrincipal(context.Background(), p)
	if got := PrincipalFromContext(ctx); got != p {
		t.Errorf("PrincipalFromContext returned %+v, want %+v", got, p)
	}

	// A wrong-typed value under the same key is ignored.
	ctx = context.WithValue(context.Background(), PrincipalContextKey, "not a principal")
	if got := PrincipalFromContext(ctx); got != nil {
		t.Errorf("expected nil for mistyped context value, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// Enforcer
// ---------------------------------------------------------------------------

// mapRoleLoader is an in-memory RoleLoader; unknown IDs return config.ErrNotFound.
type mapRoleLoader map[int64]*model.Role

func (m mapRoleLoader) GetRole(_ context.Context, id int64) (*model.Role, error) {
	if r, ok := m[id]; ok {
		return r, nil
	}
	return nil, config.ErrNotFound
}

// errRoleLoader always fails with the configured error.
type errRoleLoader struct{ err error }

func (e errRoleLoader) GetRole(context.Context, int64) (*model.Role, error) {
	return nil, e.err
}

func TestEnforcerAuthorize(t *testing.T) {
	ctx := context.Background()
	loader := mapRoleLoader{
		1: {ID: 1, Name: "reader", IsActive: true, Access: []model.RoleAccess{rule("*", "*", model.VerbGet)}},
		2: {ID: 2, Name: "sleeper", IsActive: false, Access: []model.RoleAccess{rule("*", "*", model.VerbAll)}},
		3: {ID: 3, Name: "empty", IsActive: true},
		4: {ID: 4, Name: "writer", IsActive: true, Access: []model.RoleAccess{rule("mydb", "_table/*", model.VerbAll)}},
	}
	enf := NewEnforcer(loader)

	apiKey := func(roleID int64) *Principal {
		return &Principal{Type: PrincipalAPIKey, KeyID: 100 + roleID, RoleID: roleID}
	}

	tests := []struct {
		name       string
		enf        *Enforcer
		principal  *Principal
		service    string
		component  string
		verb       int
		wantAllow  bool
		wantDenial bool
		wantRole   string
	}{
		{
			name: "nil principal is refused with a Denial",
			enf:  enf, principal: nil,
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true,
		},
		{
			name: "admin bypasses RBAC",
			enf:  enf, principal: &Principal{Type: PrincipalAdmin, AdminID: 1, IsAdmin: true},
			service: "mydb", component: "_table/x", verb: model.VerbDelete,
			wantAllow: true,
		},
		{
			name:      "admin bypasses even without a loader hit",
			enf:       NewEnforcer(errRoleLoader{errors.New("boom")}),
			principal: &Principal{Type: PrincipalAdmin, IsAdmin: true},
			service:   "mydb", component: "_table/x", verb: model.VerbDelete,
			wantAllow: true,
		},
		{
			name: "happy path GET",
			enf:  enf, principal: apiKey(1),
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantAllow: true,
		},
		{
			name: "GET-only role denies PATCH",
			enf:  enf, principal: apiKey(1),
			service: "mydb", component: "_table/x", verb: model.VerbPatch,
			wantDenial: true, wantRole: "reader",
		},
		{
			name: "missing role is a Denial",
			enf:  enf, principal: apiKey(999),
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true,
		},
		{
			name: "inactive role is a Denial",
			enf:  enf, principal: apiKey(2),
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true, wantRole: "sleeper",
		},
		{
			name: "role with no rules is a Denial",
			enf:  enf, principal: apiKey(3),
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true, wantRole: "empty",
		},
		{
			name: "unsupported principal type is a Denial",
			enf:  enf, principal: &Principal{Type: "something-else", RoleID: 1},
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true,
		},
		{
			name: "loader ErrNotFound is a Denial",
			enf:  NewEnforcer(errRoleLoader{config.ErrNotFound}), principal: apiKey(1),
			service: "mydb", component: "_table/x", verb: model.VerbGet,
			wantDenial: true,
		},
		{
			name: "scoped writer allowed on its service",
			enf:  enf, principal: apiKey(4),
			service: "mydb", component: "/_table/orders/", verb: model.VerbDelete,
			wantAllow: true,
		},
		{
			name: "scoped writer denied on other service",
			enf:  enf, principal: apiKey(4),
			service: "otherdb", component: "_table/orders", verb: model.VerbDelete,
			wantDenial: true, wantRole: "writer",
		},
		{
			name: "scoped writer denied on schema component",
			enf:  enf, principal: apiKey(4),
			service: "mydb", component: "_schema/orders", verb: model.VerbGet,
			wantDenial: true, wantRole: "writer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.enf.Authorize(ctx, tc.principal, tc.service, tc.component, tc.verb)
			if tc.wantAllow {
				if err != nil {
					t.Fatalf("expected allow, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			var d *Denial
			if errors.As(err, &d) != tc.wantDenial {
				t.Fatalf("errors.As(Denial) = %v, want %v (err=%v)", !tc.wantDenial, tc.wantDenial, err)
			}
			if !errors.Is(err, ErrForbidden) {
				t.Errorf("expected errors.Is(err, ErrForbidden) for %v", err)
			}
			if d.Reason == "" {
				t.Error("Denial.Reason should not be empty")
			}
			if d.Service != tc.service {
				t.Errorf("Denial.Service = %q, want %q", d.Service, tc.service)
			}
			if d.Verb != tc.verb {
				t.Errorf("Denial.Verb = %d, want %d", d.Verb, tc.verb)
			}
			if tc.wantRole != "" && d.Role != tc.wantRole {
				t.Errorf("Denial.Role = %q, want %q", d.Role, tc.wantRole)
			}
			if d.Component == "" && tc.component != "" {
				t.Error("Denial.Component should be populated")
			}
		})
	}
}

func TestEnforcerAuthorize_LoaderError(t *testing.T) {
	boom := errors.New("boom")
	enf := NewEnforcer(errRoleLoader{boom})
	p := &Principal{Type: PrincipalAPIKey, RoleID: 1}

	err := enf.Authorize(context.Background(), p, "mydb", "_table/x", model.VerbGet)
	if err == nil {
		t.Fatal("expected error from failing loader")
	}
	var d *Denial
	if errors.As(err, &d) {
		t.Errorf("loader failure must not be a Denial, got %v", err)
	}
	if errors.Is(err, ErrForbidden) {
		t.Errorf("loader failure must not unwrap to ErrForbidden, got %v", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("loader failure should wrap the original error, got %v", err)
	}
}

func TestEnforcerRole(t *testing.T) {
	loader := mapRoleLoader{
		1: {ID: 1, Name: "reader", IsActive: true},
		2: {ID: 2, Name: "sleeper", IsActive: false},
	}
	enf := NewEnforcer(loader)
	ctx := context.Background()

	t.Run("nil principal", func(t *testing.T) {
		_, err := enf.Role(ctx, nil)
		var d *Denial
		if !errors.As(err, &d) {
			t.Fatalf("expected Denial, got %v", err)
		}
	})
	t.Run("admin principal is unsupported", func(t *testing.T) {
		_, err := enf.Role(ctx, &Principal{Type: PrincipalAdmin, IsAdmin: true})
		var d *Denial
		if !errors.As(err, &d) {
			t.Fatalf("expected Denial, got %v", err)
		}
	})
	t.Run("active role", func(t *testing.T) {
		role, err := enf.Role(ctx, &Principal{Type: PrincipalAPIKey, RoleID: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if role.Name != "reader" {
			t.Errorf("role.Name = %q, want reader", role.Name)
		}
	})
	t.Run("inactive role", func(t *testing.T) {
		_, err := enf.Role(ctx, &Principal{Type: PrincipalAPIKey, RoleID: 2})
		var d *Denial
		if !errors.As(err, &d) {
			t.Fatalf("expected Denial, got %v", err)
		}
		if d.Role != "sleeper" {
			t.Errorf("Denial.Role = %q, want sleeper", d.Role)
		}
	})
	t.Run("missing role", func(t *testing.T) {
		_, err := enf.Role(ctx, &Principal{Type: PrincipalAPIKey, RoleID: 42})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})
}

func TestDenialFieldsAndContext(t *testing.T) {
	loader := mapRoleLoader{
		1: {ID: 1, Name: "reader", IsActive: true, Access: []model.RoleAccess{rule("*", "*", model.VerbGet)}},
	}
	enf := NewEnforcer(loader)
	p := &Principal{Type: PrincipalAPIKey, KeyID: 9, RoleID: 1}

	err := enf.Authorize(context.Background(), p, "mydb", "/_table/items/", model.VerbDelete)
	var d *Denial
	if !errors.As(err, &d) {
		t.Fatalf("expected Denial, got %v", err)
	}
	if d.Role != "reader" {
		t.Errorf("Role = %q, want reader", d.Role)
	}
	if d.Service != "mydb" {
		t.Errorf("Service = %q, want mydb", d.Service)
	}
	if d.Component != "_table/items" {
		t.Errorf("Component = %q, want _table/items (normalized)", d.Component)
	}
	if d.Verb != model.VerbDelete {
		t.Errorf("Verb = %d, want %d", d.Verb, model.VerbDelete)
	}
	if d.Error() == "" || d.Error() != d.Reason {
		t.Errorf("Error() = %q, want Reason %q", d.Error(), d.Reason)
	}

	ctx := d.Context()
	want := map[string]interface{}{
		"role":      "reader",
		"service":   "mydb",
		"component": "_table/items",
		"verb":      "DELETE",
	}
	if !reflect.DeepEqual(ctx, want) {
		t.Errorf("Context() = %v, want %v", ctx, want)
	}
}

func TestDenialContext_OmitsEmptyFields(t *testing.T) {
	d := &Denial{Reason: "nope"}
	if got := d.Context(); len(got) != 0 {
		t.Errorf("expected empty context for bare Denial, got %v", got)
	}

	d = &Denial{Reason: "nope", Service: "mydb", Verb: model.VerbGet | model.VerbPost}
	got := d.Context()
	if got["service"] != "mydb" {
		t.Errorf("service = %v, want mydb", got["service"])
	}
	if got["verb"] != "GET,POST" {
		t.Errorf("verb = %v, want GET,POST", got["verb"])
	}
	if _, ok := got["role"]; ok {
		t.Error("role should be omitted when empty")
	}
	if _, ok := got["component"]; ok {
		t.Error("component should be omitted when empty")
	}
}
