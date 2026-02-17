package model

import "time"

// APIKey represents an API key used to authenticate requests against a role.
// The raw key is never stored; only a SHA-256 hash and a short prefix for
// identification are persisted.
type APIKey struct {
	ID        int64      `json:"id" db:"id"`
	KeyHash   string     `json:"-" db:"key_hash"`      // SHA-256 hash, never expose
	KeyPrefix string     `json:"key_prefix" db:"key_prefix"` // First 8 chars for identification
	Label     string     `json:"label" db:"label"`
	RoleID    int64      `json:"role_id" db:"role_id"`
	IsActive  bool       `json:"is_active" db:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty" db:"last_used"`
}
