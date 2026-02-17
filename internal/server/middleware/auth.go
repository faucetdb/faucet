package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/faucetdb/faucet/internal/service"
)

type contextKeyAuth string

const (
	// AuthPrincipalKey is the context key for the authenticated principal.
	AuthPrincipalKey contextKeyAuth = "auth_principal"
)

// Principal represents the authenticated identity making the request.
type Principal struct {
	Type    string // "admin" or "api_key"
	AdminID int64
	RoleID  int64
	IsAdmin bool
}

// Authenticate returns an HTTP middleware that validates the request's
// authentication credentials. It supports two methods:
//
//  1. API key via the X-API-Key header (for service consumers)
//  2. JWT Bearer token via the Authorization header (for admin users)
//
// On success, a Principal is attached to the request context. On failure,
// a 401 JSON error response is returned.
func Authenticate(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var principal *Principal

			// Try API key first
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				p, err := authSvc.ValidateAPIKey(r.Context(), apiKey)
				if err != nil {
					writeAuthError(w, http.StatusUnauthorized, "Invalid API key")
					return
				}
				principal = &Principal{
					Type:   "api_key",
					RoleID: p.RoleID,
				}
			}

			// Try JWT Bearer token
			if principal == nil {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					p, err := authSvc.ValidateJWT(r.Context(), token)
					if err != nil {
						writeAuthError(w, http.StatusUnauthorized, "Invalid token")
						return
					}
					principal = &Principal{
						Type:    "admin",
						AdminID: p.AdminID,
						IsAdmin: true,
					}
				}
			}

			if principal == nil {
				writeAuthError(w, http.StatusUnauthorized,
					"Authentication required. Provide X-API-Key header or Bearer token.")
				return
			}

			ctx := context.WithValue(r.Context(), AuthPrincipalKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin returns an HTTP middleware that enforces admin-level access.
// It must be used after Authenticate in the middleware chain.
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetPrincipal(r.Context())
			if principal == nil || !principal.IsAdmin {
				writeAuthError(w, http.StatusForbidden, "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetPrincipal extracts the authenticated principal from the context.
// Returns nil if no principal is present (i.e., unauthenticated request).
func GetPrincipal(ctx context.Context) *Principal {
	if p, ok := ctx.Value(AuthPrincipalKey).(*Principal); ok {
		return p
	}
	return nil
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Manually construct JSON to avoid import cycle with handler package
	w.Write([]byte(`{"error":{"code":` + httpStatusString(status) + `,"message":"` + message + `"}}`))
}

func httpStatusString(code int) string {
	switch code {
	case 401:
		return "401"
	case 403:
		return "403"
	default:
		return "500"
	}
}
