package cli

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
)

// useTempDataDir points the package-level data directory (used by
// openConfigStore via resolveDataDir) at a fresh temp dir for the test.
func useTempDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := dataDir
	dataDir = dir
	t.Setenv("FAUCET_DATA_DIR", "") // make sure the env var cannot override the flag
	t.Cleanup(func() { dataDir = prev })
	return dir
}

func openTestStore(t *testing.T) *config.Store {
	t.Helper()
	store, err := openConfigStore()
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestParseAccessRule(t *testing.T) {
	t.Run("normalizes empty and slash-wrapped patterns", func(t *testing.T) {
		cases := []struct {
			service, component string
			wantService        string
			wantComponent      string
		}{
			{"", "", "*", "*"},
			{"  ", "  ", "*", "*"},
			{"mydb", "/x/", "mydb", "x"},
			{" mydb ", " /_table/orders/ ", "mydb", "_table/orders"},
			{"*", "*", "*", "*"},
		}
		for _, tc := range cases {
			rule, err := parseAccessRule(tc.service, tc.component, "GET")
			if err != nil {
				t.Fatalf("parseAccessRule(%q,%q): %v", tc.service, tc.component, err)
			}
			if rule.ServiceName != tc.wantService {
				t.Errorf("service %q -> got %q want %q", tc.service, rule.ServiceName, tc.wantService)
			}
			if rule.Component != tc.wantComponent {
				t.Errorf("component %q -> got %q want %q", tc.component, rule.Component, tc.wantComponent)
			}
			if rule.VerbMask != model.VerbGet {
				t.Errorf("verb mask: got %d want %d", rule.VerbMask, model.VerbGet)
			}
			if rule.RequestorMask != model.RequestorAPI {
				t.Errorf("requestor mask: got %d want %d", rule.RequestorMask, model.RequestorAPI)
			}
			if rule.Filters == nil || rule.FilterOp != "AND" {
				t.Errorf("filters not initialised: %+v", rule)
			}
		}
	})

	t.Run("verb combinations", func(t *testing.T) {
		rule, err := parseAccessRule("*", "*", "get, Post,PATCH")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := model.VerbGet | model.VerbPost | model.VerbPatch
		if rule.VerbMask != want {
			t.Errorf("verb mask: got %d want %d", rule.VerbMask, want)
		}
		all, err := parseAccessRule("*", "*", "*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if all.VerbMask != model.VerbAll {
			t.Errorf("'*' verb mask: got %d want %d", all.VerbMask, model.VerbAll)
		}
	})

	t.Run("rejects empty and bogus verbs", func(t *testing.T) {
		for _, verbs := range []string{"", "   ", ",", "HEAD", "GET,FLY", "1"} {
			if _, err := parseAccessRule("*", "*", verbs); err == nil {
				t.Errorf("parseAccessRule verbs=%q: expected error, got nil", verbs)
			} else if !strings.Contains(err.Error(), "--verbs") {
				t.Errorf("parseAccessRule verbs=%q: error should mention --verbs, got %q", verbs, err)
			}
		}
	})
}

func TestRunRoleCreate(t *testing.T) {
	t.Run("with verbs writes role and one rule", func(t *testing.T) {
		useTempDataDir(t)
		if err := runRoleCreate("writer", "writes stuff", "mydb", "/_table/orders/", "GET,POST", true); err != nil {
			t.Fatalf("runRoleCreate: %v", err)
		}

		store := openTestStore(t)
		role, err := store.GetRoleByName(context.Background(), "writer")
		if err != nil {
			t.Fatalf("GetRoleByName: %v", err)
		}
		if !role.IsActive {
			t.Error("role should be active")
		}
		if role.Description != "writes stuff" {
			t.Errorf("description: got %q", role.Description)
		}
		if len(role.Access) != 1 {
			t.Fatalf("expected 1 access rule, got %d: %+v", len(role.Access), role.Access)
		}
		a := role.Access[0]
		if a.ServiceName != "mydb" || a.Component != "_table/orders" {
			t.Errorf("rule target: got %s/%s", a.ServiceName, a.Component)
		}
		if a.VerbMask != model.VerbGet|model.VerbPost {
			t.Errorf("rule verbs: got %d", a.VerbMask)
		}
		if a.RequestorMask != model.RequestorAPI {
			t.Errorf("rule requestor: got %d", a.RequestorMask)
		}
	})

	t.Run("without verbs creates role with no rules", func(t *testing.T) {
		useTempDataDir(t)
		if err := runRoleCreate("empty", "", "*", "*", "", false); err != nil {
			t.Fatalf("runRoleCreate: %v", err)
		}
		store := openTestStore(t)
		role, err := store.GetRoleByName(context.Background(), "empty")
		if err != nil {
			t.Fatalf("GetRoleByName: %v", err)
		}
		if len(role.Access) != 0 {
			t.Errorf("expected no access rules, got %+v", role.Access)
		}
	})

	t.Run("bad verbs creates nothing", func(t *testing.T) {
		useTempDataDir(t)
		err := runRoleCreate("broken", "", "*", "*", "GET,BOGUS", true)
		if err == nil {
			t.Fatal("expected error for bogus verbs")
		}
		store := openTestStore(t)
		roles, err := store.ListRoles(context.Background())
		if err != nil {
			t.Fatalf("ListRoles: %v", err)
		}
		if len(roles) != 0 {
			t.Errorf("expected no roles to be created, got %+v", roles)
		}
	})

	t.Run("duplicate name fails", func(t *testing.T) {
		useTempDataDir(t)
		if err := runRoleCreate("dup", "", "*", "*", "GET", true); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if err := runRoleCreate("dup", "", "*", "*", "GET", true); err == nil {
			t.Error("second create with the same name should fail")
		}
	})
}

func TestRunRoleGrantAppends(t *testing.T) {
	useTempDataDir(t)
	if err := runRoleCreate("readonly", "", "*", "*", "GET", true); err != nil {
		t.Fatalf("runRoleCreate: %v", err)
	}
	if err := runRoleGrant("readonly", "mydb", "_table/orders", "POST,DELETE"); err != nil {
		t.Fatalf("runRoleGrant: %v", err)
	}
	if err := runRoleGrant("readonly", "", "", "PUT"); err != nil {
		t.Fatalf("runRoleGrant (empty patterns): %v", err)
	}

	store := openTestStore(t)
	role, err := store.GetRoleByName(context.Background(), "readonly")
	if err != nil {
		t.Fatalf("GetRoleByName: %v", err)
	}
	if len(role.Access) != 3 {
		t.Fatalf("expected 3 rules after two grants, got %d: %+v", len(role.Access), role.Access)
	}
	// Original rule must survive.
	found := false
	for _, a := range role.Access {
		if a.ServiceName == "*" && a.Component == "*" && a.VerbMask == model.VerbGet {
			found = true
		}
	}
	if !found {
		t.Errorf("original GET rule was replaced instead of appended: %+v", role.Access)
	}
	var sawOrders, sawPut bool
	for _, a := range role.Access {
		if a.ServiceName == "mydb" && a.Component == "_table/orders" && a.VerbMask == model.VerbPost|model.VerbDelete {
			sawOrders = true
		}
		if a.ServiceName == "*" && a.Component == "*" && a.VerbMask == model.VerbPut {
			sawPut = true
		}
	}
	if !sawOrders || !sawPut {
		t.Errorf("granted rules missing: %+v", role.Access)
	}

	t.Run("bad verbs leaves rules untouched", func(t *testing.T) {
		if err := runRoleGrant("readonly", "*", "*", ""); err == nil {
			t.Fatal("expected error for empty verbs")
		}
		role, err := store.GetRoleByName(context.Background(), "readonly")
		if err != nil {
			t.Fatalf("GetRoleByName: %v", err)
		}
		if len(role.Access) != 3 {
			t.Errorf("rule count changed after failed grant: %d", len(role.Access))
		}
	})

	t.Run("unknown role fails", func(t *testing.T) {
		if err := runRoleGrant("nope", "*", "*", "GET"); err == nil {
			t.Error("expected error for unknown role")
		}
	})
}

// --- warnUnusableRoles ---

func mustCreateRole(t *testing.T, store *config.Store, name string, active bool, access []model.RoleAccess) *model.Role {
	t.Helper()
	ctx := context.Background()
	role := &model.Role{Name: name, IsActive: active}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole %s: %v", name, err)
	}
	if len(access) > 0 {
		if err := store.SetRoleAccess(ctx, role.ID, access); err != nil {
			t.Fatalf("SetRoleAccess %s: %v", name, err)
		}
	}
	return role
}

var keySeq int

func mustCreateKey(t *testing.T, store *config.Store, roleID int64, active bool, expires *time.Time) {
	t.Helper()
	keySeq++
	raw := fmt.Sprintf("faucet_testkey_%d", keySeq)
	key := &model.APIKey{
		KeyHash:   config.HashAPIKey(raw),
		KeyPrefix: raw[:8],
		Label:     "k",
		RoleID:    roleID,
		IsActive:  active,
		ExpiresAt: expires,
	}
	if err := store.CreateAPIKey(context.Background(), key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
}

func TestWarnUnusableRoles(t *testing.T) {
	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	rule := func(mask int) []model.RoleAccess {
		return []model.RoleAccess{{ServiceName: "*", Component: "*", VerbMask: mask, RequestorMask: model.RequestorAPI, Filters: []model.Filter{}, FilterOp: "AND"}}
	}

	noRules := mustCreateRole(t, store, "norules", true, nil)
	zeroMask := mustCreateRole(t, store, "zeromask", true, rule(0))
	inactive := mustCreateRole(t, store, "inactive", false, rule(model.VerbGet))
	healthy := mustCreateRole(t, store, "healthy", true, rule(model.VerbGet))
	orphan := mustCreateRole(t, store, "orphan", true, nil) // no keys -> no warning
	revokedOnly := mustCreateRole(t, store, "revokedonly", true, nil)
	expiredOnly := mustCreateRole(t, store, "expiredonly", true, nil)

	mustCreateKey(t, store, noRules.ID, true, nil)
	mustCreateKey(t, store, noRules.ID, true, nil)
	mustCreateKey(t, store, noRules.ID, false, nil) // revoked, must not be counted
	mustCreateKey(t, store, zeroMask.ID, true, nil)
	mustCreateKey(t, store, inactive.ID, true, nil)
	mustCreateKey(t, store, healthy.ID, true, nil)
	mustCreateKey(t, store, revokedOnly.ID, false, nil)
	past := time.Now().Add(-time.Hour)
	mustCreateKey(t, store, expiredOnly.ID, true, &past)
	_ = orphan

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	warnUnusableRoles(context.Background(), store, logger)
	out := buf.String()

	wantContains := []string{
		`role \"norules\" (id=` + itoa(noRules.ID) + `) has no access rules; 2 active API keys will be denied (403) — run: faucet role grant --role norules --verbs GET`,
		`role \"zeromask\" (id=` + itoa(zeroMask.ID) + `) has only rules with verb_mask=0; 1 active API key will be denied (403) — run: faucet role grant --role zeromask --verbs GET`,
		`role \"inactive\" (id=` + itoa(inactive.ID) + `) is inactive; 1 active API key will be denied (403)`,
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("expected log output to contain %q\n--- got ---\n%s", w, out)
		}
	}
	for _, absent := range []string{`"healthy"`, `"orphan"`, `"revokedonly"`, `"expiredonly"`} {
		if strings.Contains(out, absent) {
			t.Errorf("did not expect a warning for role %s\n--- got ---\n%s", absent, out)
		}
	}
	if got := strings.Count(out, "level=WARN"); got != 3 {
		t.Errorf("expected exactly 3 warnings, got %d\n--- got ---\n%s", got, out)
	}

	t.Run("nil store and logger are tolerated", func(t *testing.T) {
		warnUnusableRoles(context.Background(), nil, logger)
		warnUnusableRoles(context.Background(), store, nil)
	})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
