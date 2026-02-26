package contract

import (
	"time"

	"github.com/faucetdb/faucet/internal/model"
)

// LockMode controls how Faucet handles schema drift for a service.
type LockMode string

const (
	// LockModeNone disables contract locking. Live schema = live API.
	LockModeNone LockMode = "none"
	// LockModeAuto locks on first introspection. Additive changes auto-promote;
	// breaking changes are blocked and the locked contract shape is served.
	LockModeAuto LockMode = "auto"
	// LockModeStrict requires explicit promotion for ALL changes, even additive.
	LockModeStrict LockMode = "strict"
)

// ValidLockMode returns true if m is a recognized lock mode.
func ValidLockMode(m string) bool {
	switch LockMode(m) {
	case LockModeNone, LockModeAuto, LockModeStrict:
		return true
	}
	return false
}

// Contract represents a locked schema snapshot for a single table within a service.
type Contract struct {
	ID          int64             `json:"id" db:"id"`
	ServiceName string            `json:"service_name" db:"service_name"`
	TableName   string            `json:"table_name" db:"table_name"`
	Schema      model.TableSchema `json:"schema"`
	SchemaJSON  string            `json:"-" db:"schema_json"`
	LockedAt    time.Time         `json:"locked_at" db:"locked_at"`
	PromotedAt  *time.Time        `json:"promoted_at,omitempty" db:"promoted_at"`
}

// DriftType classifies the severity of a schema change.
type DriftType string

const (
	// DriftAdditive means a new column or table was added. Safe for consumers.
	DriftAdditive DriftType = "additive"
	// DriftBreaking means a column was renamed, removed, or had its type changed.
	DriftBreaking DriftType = "breaking"
)

// DriftItem describes a single difference between the locked and live schemas.
type DriftItem struct {
	Type        DriftType `json:"type"`
	Category    string    `json:"category"`    // "column_added", "column_removed", "column_renamed", "type_changed", "nullable_changed", "table_removed"
	TableName   string    `json:"table_name"`
	ColumnName  string    `json:"column_name,omitempty"`
	OldValue    string    `json:"old_value,omitempty"`
	NewValue    string    `json:"new_value,omitempty"`
	Description string    `json:"description"`
}

// DriftReport summarizes all differences between a locked contract and the live schema.
type DriftReport struct {
	ServiceName    string      `json:"service_name"`
	TableName      string      `json:"table_name"`
	HasDrift       bool        `json:"has_drift"`
	HasBreaking    bool        `json:"has_breaking"`
	AdditiveCount  int         `json:"additive_count"`
	BreakingCount  int         `json:"breaking_count"`
	Items          []DriftItem `json:"items"`
	LockedAt       time.Time   `json:"locked_at"`
	CheckedAt      time.Time   `json:"checked_at"`
}

// ServiceDriftReport summarizes drift across all locked tables in a service.
type ServiceDriftReport struct {
	ServiceName   string        `json:"service_name"`
	LockMode      LockMode      `json:"lock_mode"`
	TotalTables   int           `json:"total_tables"`
	DriftedTables int           `json:"drifted_tables"`
	BreakingCount int           `json:"breaking_count"`
	Tables        []DriftReport `json:"tables"`
}
