package model

import "time"

// Admin represents an administrative user who can manage Faucet configuration
// through the admin API. Passwords are stored as bcrypt hashes.
type Admin struct {
	ID           int64      `json:"id" db:"id"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"` // bcrypt hash, never expose
	Name         string     `json:"name" db:"name"`
	IsActive     bool       `json:"is_active" db:"is_active"`
	IsSuperAdmin bool       `json:"is_super_admin" db:"is_super_admin"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}
