package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/rbac"
	"github.com/faucetdb/faucet/internal/service"
)

// AuthPrincipalKey is the context key for the authenticated principal.
// It is shared with the rbac package so that non-HTTP consumers (the MCP
// server) can read the same principal.
const AuthPrincipalKey = rbac.PrincipalContextKey

// Principal represents the authenticated identity making the request.
type Principal = rbac.Principal

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
					writeAuthError(w, http.StatusUnauthorized, "Invalid API key", nil)
					return
				}
				principal = &Principal{
					Type:   rbac.PrincipalAPIKey,
					KeyID:  p.KeyID,
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
						writeAuthError(w, http.StatusUnauthorized, "Invalid token", nil)
						return
					}
					principal = &Principal{
						Type:    rbac.PrincipalAdmin,
						AdminID: p.AdminID,
						IsAdmin: true,
					}
				}
			}

			if principal == nil {
				writeAuthError(w, http.StatusUnauthorized,
					"Authentication required. Provide X-API-Key header or Bearer token.", nil)
				return
			}

			ctx := rbac.WithPrincipal(r.Context(), principal)
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
				writeAuthError(w, http.StatusForbidden, "Admin access required", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAccess returns an HTTP middleware that enforces the role-based
// access rules (service, component, verb_mask) bound to an API key.
//
// It must be mounted inside the "/{serviceName}" route group, after
// Authenticate: the service name is read from the route parameter and the
// component (e.g. "_table/customers") from the remaining route path.
// Admin principals bypass the check. Requests are refused with 403 when the
// role is missing, inactive, has no matching rule, or the rule's verb_mask
// does not include the request method.
func RequireAccess(enforcer *rbac.Enforcer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetPrincipal(r.Context())
			if principal == nil {
				writeAuthError(w, http.StatusUnauthorized,
					"Authentication required. Provide X-API-Key header or Bearer token.", nil)
				return
			}
			if principal.IsAdmin {
				next.ServeHTTP(w, r)
				return
			}

			serviceName := chi.URLParam(r, "serviceName")
			component := routeComponent(r)
			verb, ok := rbac.VerbFromMethod(r.Method)
			if !ok {
				writeAuthError(w, http.StatusForbidden, "Method not permitted: "+r.Method, nil)
				return
			}

			if err := enforcer.Authorize(r.Context(), principal, serviceName, component, verb); err != nil {
				var d *rbac.Denial
				if errors.As(err, &d) {
					writeAuthError(w, http.StatusForbidden, d.Reason, d.Context())
					return
				}
				// Fail closed on unexpected errors (e.g. config store failure).
				writeAuthError(w, http.StatusInternalServerError, "Authorization check failed", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ReadOnlyGuard returns an HTTP middleware that rejects every non-GET/HEAD
// request to a service flagged read_only, regardless of the principal's
// role. It must be mounted inside the "/{serviceName}" route group.
func ReadOnlyGuard(store *config.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			serviceName := chi.URLParam(r, "serviceName")
			svc, err := store.GetServiceByName(r.Context(), serviceName)
			if err == nil && svc.ReadOnly {
				writeAuthError(w, http.StatusForbidden,
					"Service "+serviceName+" is read-only; "+r.Method+" is not permitted",
					map[string]interface{}{"service": serviceName, "verb": r.Method})
				return
			}
			// Unknown services fall through to the handler, which returns 404.
			next.ServeHTTP(w, r)
		})
	}
}

// routeComponent derives the RBAC component ("_table/customers", "_schema",
// ...) from the part of the URL that follows "/{serviceName}".
func routeComponent(r *http.Request) string {
	p := ""
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		p = rctx.RoutePath
	}
	if p == "" {
		// Fallback for handlers mounted outside a chi sub-router: strip the
		// "/api/v1/{serviceName}" prefix from the request path.
		p = r.URL.Path
		if i := strings.Index(p, "/"+chi.URLParam(r, "serviceName")+"/"); i >= 0 {
			p = p[i+len(chi.URLParam(r, "serviceName"))+1:]
		}
	}
	if unescaped, err := url.PathUnescape(p); err == nil {
		p = unescaped
	}
	return rbac.NormalizeComponent(p)
}

// GetPrincipal extracts the authenticated principal from the context.
// Returns nil if no principal is present (i.e., unauthenticated request).
func GetPrincipal(ctx context.Context) *Principal {
	return rbac.PrincipalFromContext(ctx)
}

func writeAuthError(w http.ResponseWriter, status int, message string, ctx map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{
		Error: model.ErrorDetail{
			Code:    status,
			Message: message,
			Context: ctx,
		},
	})
}
