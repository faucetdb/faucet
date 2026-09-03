package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/model"
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

// RequireAccess returns an HTTP middleware that enforces the role's access
// rules for database service routes. It must be mounted inside the
// /{serviceName} route group, after Authenticate. Admin principals bypass
// the check; API-key principals must hold a rule granting the request's HTTP
// verb on the requested component (the path below the service name, e.g.
// "_table/users"). Denied requests receive a 403 JSON error. The check runs
// before the sub-router matches a route, so a key without a matching rule
// gets 403 even for unknown paths; that hides 404/405 but leaks nothing.
func RequireAccess(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetPrincipal(r.Context())
			if principal == nil {
				writeAuthError(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			if principal.IsAdmin {
				next.ServeHTTP(w, r)
				return
			}

			serviceName := chi.URLParam(r, "serviceName")
			component := serviceComponent(r)
			verb := model.VerbFromMethod(r.Method)

			err := authSvc.Authorize(r.Context(), principal.RoleID, serviceName, component, verb)
			if err == nil {
				next.ServeHTTP(w, r)
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				writeAuthError(w, http.StatusForbidden,
					"Role does not permit "+model.VerbName(verb)+" on "+serviceName+"/"+component)
				return
			}
			writeAuthError(w, http.StatusInternalServerError, "Authorization check failed")
		})
	}
}

// serviceComponent returns the request path below the service segment,
// without leading or trailing slashes. Chi exposes the remaining path of a
// mounted sub-router as RoutePath, so this only works inside the
// /{serviceName} route group.
func serviceComponent(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return strings.Trim(rctx.RoutePath, "/")
	}
	return ""
}

// GetPrincipal extracts the authenticated principal from the context.
// Returns nil if no principal is present (i.e., unauthenticated request).
func GetPrincipal(ctx context.Context) *Principal {
	if p, ok := ctx.Value(AuthPrincipalKey).(*Principal); ok {
		return p
	}
	return nil
}

// writeAuthError writes the standard error envelope. The handler package's
// helper is not used here to avoid an import cycle; messages may contain
// request-derived strings, so they are JSON-encoded rather than concatenated.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    status,
			"message": message,
		},
	})
}
