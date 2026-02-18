package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// RequestID middleware tests
// ---------------------------------------------------------------------------

func TestRequestIDGeneratesUUID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Error("expected non-empty request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	respID := rr.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID in response header")
	}
	// UUID v7 format check: 36 chars with dashes
	if len(respID) != 36 {
		t.Errorf("expected UUID-length request ID, got %q (len=%d)", respID, len(respID))
	}
}

func TestRequestIDPreservesClientID(t *testing.T) {
	clientID := "my-custom-trace-id-123"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id != clientID {
			t.Errorf("expected context ID %q, got %q", clientID, id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", clientID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	respID := rr.Header().Get("X-Request-ID")
	if respID != clientID {
		t.Errorf("expected response X-Request-ID %q, got %q", clientID, respID)
	}
}

func TestGetRequestIDEmptyContext(t *testing.T) {
	id := GetRequestID(context.Background())
	if id != "" {
		t.Errorf("expected empty string from bare context, got %q", id)
	}
}

// ---------------------------------------------------------------------------
// RequireAdmin middleware tests
// ---------------------------------------------------------------------------

func TestRequireAdminAllowsAdmins(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	ctx := context.WithValue(req.Context(), AuthPrincipalKey, &Principal{
		Type:    "admin",
		AdminID: 1,
		IsAdmin: true,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdminBlocksNonAdmins(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for non-admin")
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	ctx := context.WithValue(req.Context(), AuthPrincipalKey, &Principal{
		Type:    "api_key",
		RoleID:  1,
		IsAdmin: false,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdminBlocksUnauthenticated(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for unauthenticated")
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// GetPrincipal tests
// ---------------------------------------------------------------------------

func TestGetPrincipalWithValue(t *testing.T) {
	expected := &Principal{Type: "admin", AdminID: 42, IsAdmin: true}
	ctx := context.WithValue(context.Background(), AuthPrincipalKey, expected)

	got := GetPrincipal(ctx)
	if got == nil {
		t.Fatal("expected non-nil principal")
	}
	if got.AdminID != 42 {
		t.Errorf("expected AdminID 42, got %d", got.AdminID)
	}
	if !got.IsAdmin {
		t.Error("expected IsAdmin true")
	}
}

func TestGetPrincipalWithoutValue(t *testing.T) {
	got := GetPrincipal(context.Background())
	if got != nil {
		t.Error("expected nil principal from bare context")
	}
}
