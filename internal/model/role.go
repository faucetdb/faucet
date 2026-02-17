package model

import "time"

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
