package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/service"
)

// SystemHandler manages Faucet's own configuration: services, roles, admins,
// and API keys.
type SystemHandler struct {
	store    *config.Store
	authSvc  *service.AuthService
	registry *connector.Registry
}

// NewSystemHandler creates a new SystemHandler.
func NewSystemHandler(store *config.Store, authSvc *service.AuthService, registry *connector.Registry) *SystemHandler {
	return &SystemHandler{
		store:    store,
		authSvc:  authSvc,
		registry: registry,
	}
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// loginRequest is the expected payload for the Login endpoint.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the response payload for a successful login.
type loginResponse struct {
	Token     string `json:"session_token"`
	TokenType string `json:"token_type"`
	ExpiresIn int    `json:"expires_in"`
	AdminID   int64  `json:"admin_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

// Login authenticates an admin user and returns a JWT session token.
// POST /api/v2/system/admin/session
func (h *SystemHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Email and password are required")
		return
	}

	// Look up the admin by email.
	admin, err := h.store.GetAdminByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "Authentication error: "+err.Error())
		return
	}

	if !admin.IsActive {
		writeError(w, http.StatusUnauthorized, "Account is disabled")
		return
	}

	// Verify the password hash. In production the AuthService performs bcrypt
	// comparison and issues the JWT in one flow. For now, we verify the SHA-256
	// hash and delegate token issuance to AuthService.IssueJWT.
	candidateHash := config.HashAPIKey(req.Password)
	if candidateHash != admin.PasswordHash {
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	ttl := 24 * time.Hour
	token, err := h.authSvc.IssueJWT(r.Context(), admin.ID, admin.Email, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to issue token: "+err.Error())
		return
	}

	// Update last login timestamp.
	_ = h.store.UpdateAdminLastLogin(r.Context(), admin.ID)

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		TokenType: "bearer",
		ExpiresIn: int(ttl.Seconds()),
		AdminID:   admin.ID,
		Email:     admin.Email,
		Name:      admin.Name,
	})
}

// Logout invalidates the current session. Since JWTs are stateless, this is
// a no-op on the server side. Clients should discard their token.
// DELETE /api/v2/system/admin/session
func (h *SystemHandler) Logout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Session invalidated",
	})
}

// ---------------------------------------------------------------------------
// Service management
// ---------------------------------------------------------------------------

// ListServices returns all configured database services.
// GET /api/v2/system/service
func (h *SystemHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	services, err := h.store.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list services: "+err.Error())
		return
	}

	resources := make([]map[string]interface{}, 0, len(services))
	for i := range services {
		resources = append(resources, serviceToMap(&services[i]))
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: resources,
		Meta: &model.ResponseMeta{
			Count: len(resources),
		},
	})
}

// CreateService registers a new database service.
// POST /api/v2/system/service
func (h *SystemHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	var svc model.ServiceConfig
	if err := readJSON(r, &svc); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if svc.Name == "" {
		writeError(w, http.StatusBadRequest, "Service name is required")
		return
	}
	if svc.Driver == "" {
		writeError(w, http.StatusBadRequest, "Driver is required")
		return
	}
	if svc.DSN == "" {
		writeError(w, http.StatusBadRequest, "DSN is required")
		return
	}

	// Check for name collision.
	existing, err := h.store.GetServiceByName(r.Context(), svc.Name)
	if err == nil && existing != nil {
		writeError(w, http.StatusConflict, "Service already exists: "+svc.Name)
		return
	}

	svc.IsActive = true

	// Sanitize the DSN to ensure special characters in passwords are properly
	// URL-encoded for URL-style DSNs (postgres://, sqlserver://).
	svc.DSN = connector.SanitizeDSN(svc.Driver, svc.DSN)

	if err := h.store.CreateService(r.Context(), &svc); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create service: "+err.Error())
		return
	}

	// Connect the service in the live connector registry so it's immediately usable.
	cfg := connector.ConnectionConfig{
		Driver:          svc.Driver,
		DSN:             svc.DSN,
		PrivateKeyPath:  svc.PrivateKeyPath,
		SchemaName:      svc.Schema,
		MaxOpenConns:    svc.Pool.MaxOpenConns,
		MaxIdleConns:    svc.Pool.MaxIdleConns,
		ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
		ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
	}
	if err := h.registry.Connect(svc.Name, cfg); err != nil {
		// Service is persisted but connection failed — report it but don't fail the create.
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"service":            serviceToMap(&svc),
			"connection_warning": "Service saved but connection failed: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, serviceToMap(&svc))
}

// GetService returns a single service by name.
// GET /api/v2/system/service/{serviceName}
func (h *SystemHandler) GetService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "serviceName")

	svc, err := h.store.GetServiceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Service not found: "+name)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get service: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, serviceToMap(svc))
}

// UpdateService modifies an existing service configuration.
// PUT /api/v2/system/service/{serviceName}
func (h *SystemHandler) UpdateService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "serviceName")

	existing, err := h.store.GetServiceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Service not found: "+name)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get service: "+err.Error())
		return
	}

	var updates model.ServiceConfig
	if err := readJSON(r, &updates); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Apply non-zero updates to the existing service.
	if updates.Label != "" {
		existing.Label = updates.Label
	}
	if updates.Driver != "" {
		existing.Driver = updates.Driver
	}
	if updates.DSN != "" {
		existing.DSN = connector.SanitizeDSN(existing.Driver, updates.DSN)
	}
	if updates.PrivateKeyPath != "" {
		existing.PrivateKeyPath = updates.PrivateKeyPath
	}
	if updates.Schema != "" {
		existing.Schema = updates.Schema
	}
	existing.ReadOnly = updates.ReadOnly
	existing.RawSQL = updates.RawSQL
	existing.IsActive = updates.IsActive

	if err := h.store.UpdateService(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update service: "+err.Error())
		return
	}

	// Reconnect the service in the registry with updated config.
	if existing.IsActive {
		cfg := connector.ConnectionConfig{
			Driver:          existing.Driver,
			DSN:             existing.DSN,
			PrivateKeyPath:  existing.PrivateKeyPath,
			SchemaName:      existing.Schema,
			MaxOpenConns:    existing.Pool.MaxOpenConns,
			MaxIdleConns:    existing.Pool.MaxIdleConns,
			ConnMaxLifetime: existing.Pool.ConnMaxLifetime,
			ConnMaxIdleTime: existing.Pool.ConnMaxIdleTime,
		}
		if err := h.registry.Connect(existing.Name, cfg); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"service":            serviceToMap(existing),
				"connection_warning": "Service updated but reconnection failed: " + err.Error(),
			})
			return
		}
	} else {
		// Service deactivated — disconnect from registry.
		_ = h.registry.Disconnect(existing.Name)
	}

	writeJSON(w, http.StatusOK, serviceToMap(existing))
}

// DeleteService removes a service and disconnects it.
// DELETE /api/v2/system/service/{serviceName}
func (h *SystemHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "serviceName")

	svc, err := h.store.GetServiceByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Service not found: "+name)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get service: "+err.Error())
		return
	}

	if err := h.store.DeleteService(r.Context(), svc.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete service: "+err.Error())
		return
	}

	// Disconnect from the live registry.
	_ = h.registry.Disconnect(name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Service '" + name + "' deleted",
	})
}

// TestConnection tests an active service's database connectivity by pinging it.
// GET /api/v2/system/service/{serviceName}/test
func (h *SystemHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "serviceName")

	conn, err := h.registry.Get(name)
	if err != nil {
		// Not in registry — check if it exists in the store to give a better error.
		svc, storeErr := h.store.GetServiceByName(r.Context(), name)
		if storeErr != nil {
			writeError(w, http.StatusNotFound, "Service not found: "+name)
			return
		}
		// Service exists in store but not in registry — try to reconnect it now.
		cfg := connector.ConnectionConfig{
			Driver:          svc.Driver,
			DSN:             svc.DSN,
			PrivateKeyPath:  svc.PrivateKeyPath,
			SchemaName:      svc.Schema,
			MaxOpenConns:    svc.Pool.MaxOpenConns,
			MaxIdleConns:    svc.Pool.MaxIdleConns,
			ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
			ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
		}
		if connErr := h.registry.Connect(svc.Name, cfg); connErr != nil {
			writeError(w, http.StatusServiceUnavailable, "Connection failed: "+connErr.Error())
			return
		}
		conn, _ = h.registry.Get(name)
	}

	if err := conn.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "Ping failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
	})
}

// ---------------------------------------------------------------------------
// Role management
// ---------------------------------------------------------------------------

// ListRoles returns all configured roles.
// GET /api/v2/system/role
func (h *SystemHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.store.ListRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list roles: "+err.Error())
		return
	}

	resources := make([]map[string]interface{}, 0, len(roles))
	for i := range roles {
		resources = append(resources, roleToMap(&roles[i]))
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: resources,
		Meta: &model.ResponseMeta{
			Count: len(resources),
		},
	})
}

// CreateRole creates a new role.
// POST /api/v2/system/role
func (h *SystemHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var role model.Role
	if err := readJSON(r, &role); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if role.Name == "" {
		writeError(w, http.StatusBadRequest, "Role name is required")
		return
	}

	role.IsActive = true
	if role.Access == nil {
		role.Access = []model.RoleAccess{}
	}

	if err := h.store.CreateRole(r.Context(), &role); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create role: "+err.Error())
		return
	}

	// If access rules were provided, set them.
	if len(role.Access) > 0 {
		if err := h.store.SetRoleAccess(r.Context(), role.ID, role.Access); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to set role access: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusCreated, roleToMap(&role))
}

// GetRole returns a single role by ID.
// GET /api/v2/system/role/{roleId}
func (h *SystemHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "roleId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid role ID: "+idStr)
		return
	}

	role, err := h.store.GetRole(r.Context(), id)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Role not found: "+idStr)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, roleToMap(role))
}

// UpdateRole modifies an existing role.
// PUT /api/v2/system/role/{roleId}
func (h *SystemHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "roleId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid role ID: "+idStr)
		return
	}

	existing, err := h.store.GetRole(r.Context(), id)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Role not found: "+idStr)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get role: "+err.Error())
		return
	}

	var updates model.Role
	if err := readJSON(r, &updates); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	existing.IsActive = updates.IsActive

	if err := h.store.UpdateRole(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update role: "+err.Error())
		return
	}

	// If access rules were provided, replace them.
	if updates.Access != nil {
		if err := h.store.SetRoleAccess(r.Context(), id, updates.Access); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update role access: "+err.Error())
			return
		}
		existing.Access = updates.Access
	}

	writeJSON(w, http.StatusOK, roleToMap(existing))
}

// DeleteRole removes a role by ID.
// DELETE /api/v2/system/role/{roleId}
func (h *SystemHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "roleId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid role ID: "+idStr)
		return
	}

	if err := h.store.DeleteRole(r.Context(), id); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Role not found: "+idStr)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to delete role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Role deleted",
	})
}

// ---------------------------------------------------------------------------
// Admin management
// ---------------------------------------------------------------------------

// ListAdmins returns all admin accounts.
// GET /api/v2/system/admin
func (h *SystemHandler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.store.ListAdmins(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list admins: "+err.Error())
		return
	}

	resources := make([]map[string]interface{}, 0, len(admins))
	for i := range admins {
		resources = append(resources, adminToMap(&admins[i]))
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: resources,
		Meta: &model.ResponseMeta{
			Count: len(resources),
		},
	})
}

// CreateAdmin creates a new admin account.
// POST /api/v2/system/admin
func (h *SystemHandler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "Email is required")
		return
	}
	if body.Password == "" {
		writeError(w, http.StatusBadRequest, "Password is required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	// Check for duplicate email.
	if existing, err := h.store.GetAdminByEmail(r.Context(), body.Email); err == nil && existing != nil {
		writeError(w, http.StatusConflict, "Admin with this email already exists")
		return
	}

	// Hash the password. In production this would use bcrypt via AuthService.
	// For now, use SHA-256 as a placeholder.
	passwordHash := config.HashAPIKey(body.Password)

	admin := &model.Admin{
		Email:        body.Email,
		PasswordHash: passwordHash,
		Name:         body.Name,
		IsActive:     true,
	}

	if err := h.store.CreateAdmin(r.Context(), admin); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create admin: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, adminToMap(admin))
}

// ---------------------------------------------------------------------------
// API Key management
// ---------------------------------------------------------------------------

// ListAPIKeys returns all configured API keys (without exposing the actual key).
// GET /api/v2/system/api_key
func (h *SystemHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.store.ListAPIKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list API keys: "+err.Error())
		return
	}

	// Build a role ID → name lookup so the UI can display role names.
	roles, _ := h.store.ListRoles(r.Context())
	roleNames := make(map[int64]string, len(roles))
	for _, role := range roles {
		roleNames[role.ID] = role.Name
	}

	resources := make([]map[string]interface{}, 0, len(keys))
	for i := range keys {
		m := apiKeyToMap(&keys[i])
		m["role_name"] = roleNames[keys[i].RoleID]
		resources = append(resources, m)
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: resources,
		Meta: &model.ResponseMeta{
			Count: len(resources),
		},
	})
}

// createAPIKeyRequest is the expected payload for CreateAPIKey.
type createAPIKeyRequest struct {
	Label     string     `json:"label"`
	RoleID    int64      `json:"role_id"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// createAPIKeyResponse includes the plaintext key (shown once only).
type createAPIKeyResponse struct {
	ID        int64      `json:"id"`
	Key       string     `json:"api_key"` // Plaintext, shown ONCE.
	KeyPrefix string     `json:"key_prefix"`
	Label     string     `json:"label"`
	RoleID    int64      `json:"role_id"`
	IsActive  bool       `json:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateAPIKey generates a new API key, hashes it, stores the hash, and
// returns the plaintext key exactly once.
// POST /api/v2/system/api_key
func (h *SystemHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createAPIKeyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.RoleID == 0 {
		writeError(w, http.StatusBadRequest, "role_id is required")
		return
	}

	// Validate that the role exists.
	if _, err := h.store.GetRole(r.Context(), req.RoleID); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Role not found: %d", req.RoleID))
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to validate role: "+err.Error())
		return
	}

	// Generate a 32-byte random key and encode as hex (64 chars).
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate key: "+err.Error())
		return
	}
	plaintext := "faucet_" + hex.EncodeToString(rawBytes)

	// Hash the key for storage using the store's utility.
	keyHash := config.HashAPIKey(plaintext)
	keyPrefix := plaintext[:15] // "faucet_" + first 8 hex chars

	apiKey := &model.APIKey{
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Label:     req.Label,
		RoleID:    req.RoleID,
		IsActive:  true,
		ExpiresAt: req.ExpiresAt,
	}

	if err := h.store.CreateAPIKey(r.Context(), apiKey); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save API key: "+err.Error())
		return
	}

	// Return the plaintext key. This is the ONLY time it will be visible.
	writeJSON(w, http.StatusCreated, createAPIKeyResponse{
		ID:        apiKey.ID,
		Key:       plaintext,
		KeyPrefix: keyPrefix,
		Label:     apiKey.Label,
		RoleID:    apiKey.RoleID,
		IsActive:  apiKey.IsActive,
		ExpiresAt: apiKey.ExpiresAt,
		CreatedAt: apiKey.CreatedAt,
	})
}

// RevokeAPIKey deactivates an API key by ID.
// DELETE /api/v2/system/api_key/{keyId}
func (h *SystemHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "keyId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid key ID: "+idStr)
		return
	}

	if err := h.store.RevokeAPIKey(r.Context(), id); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeError(w, http.StatusNotFound, "API key not found: "+idStr)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to revoke API key: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "API key revoked",
	})
}

// ---------------------------------------------------------------------------
// Serialization helpers (avoid exposing sensitive fields like DSN, password)
// ---------------------------------------------------------------------------

func serviceToMap(svc *model.ServiceConfig) map[string]interface{} {
	m := map[string]interface{}{
		"id":              svc.ID,
		"name":            svc.Name,
		"label":           svc.Label,
		"driver":          svc.Driver,
		"schema":          svc.Schema,
		"read_only":       svc.ReadOnly,
		"raw_sql_allowed": svc.RawSQL,
		"is_active":       svc.IsActive,
		"created_at":      svc.CreatedAt,
		"updated_at":      svc.UpdatedAt,
	}
	if svc.PrivateKeyPath != "" {
		m["private_key_path"] = svc.PrivateKeyPath
	}
	return m
}

func roleToMap(role *model.Role) map[string]interface{} {
	return map[string]interface{}{
		"id":          role.ID,
		"name":        role.Name,
		"description": role.Description,
		"is_active":   role.IsActive,
		"access":      role.Access,
		"created_at":  role.CreatedAt,
		"updated_at":  role.UpdatedAt,
	}
}

func adminToMap(admin *model.Admin) map[string]interface{} {
	m := map[string]interface{}{
		"id":             admin.ID,
		"email":          admin.Email,
		"name":           admin.Name,
		"is_active":      admin.IsActive,
		"is_super_admin": admin.IsSuperAdmin,
		"created_at":     admin.CreatedAt,
		"updated_at":     admin.UpdatedAt,
	}
	if admin.LastLoginAt != nil {
		m["last_login_at"] = admin.LastLoginAt
	}
	return m
}

func apiKeyToMap(key *model.APIKey) map[string]interface{} {
	m := map[string]interface{}{
		"id":         key.ID,
		"key_prefix": key.KeyPrefix,
		"label":      key.Label,
		"role_id":    key.RoleID,
		"is_active":  key.IsActive,
		"created_at": key.CreatedAt,
	}
	if key.ExpiresAt != nil {
		m["expires_at"] = key.ExpiresAt
	}
	if key.LastUsed != nil {
		m["last_used"] = key.LastUsed
	}
	return m
}
