package contract

import (
	"fmt"
	"time"

	"github.com/faucetdb/faucet/internal/model"
)

// DiffTable compares a locked schema contract against the live schema and
// returns a drift report classifying each difference as additive or breaking.
func DiffTable(serviceName string, locked, live model.TableSchema, lockedAt time.Time) DriftReport {
	report := DriftReport{
		ServiceName: serviceName,
		TableName:   locked.Name,
		LockedAt:    lockedAt,
		CheckedAt:   time.Now().UTC(),
	}

	// Index live columns by name for fast lookup.
	liveByName := make(map[string]model.Column, len(live.Columns))
	for _, col := range live.Columns {
		liveByName[col.Name] = col
	}

	// Index locked columns by name.
	lockedByName := make(map[string]model.Column, len(locked.Columns))
	for _, col := range locked.Columns {
		lockedByName[col.Name] = col
	}

	// Check for removed or modified columns (breaking).
	for _, lockedCol := range locked.Columns {
		liveCol, exists := liveByName[lockedCol.Name]
		if !exists {
			report.Items = append(report.Items, DriftItem{
				Type:        DriftBreaking,
				Category:    "column_removed",
				TableName:   locked.Name,
				ColumnName:  lockedCol.Name,
				OldValue:    lockedCol.Type,
				Description: fmt.Sprintf("Column %q was removed from table %q", lockedCol.Name, locked.Name),
			})
			continue
		}

		// Check type change.
		if lockedCol.Type != liveCol.Type {
			report.Items = append(report.Items, DriftItem{
				Type:        DriftBreaking,
				Category:    "type_changed",
				TableName:   locked.Name,
				ColumnName:  lockedCol.Name,
				OldValue:    lockedCol.Type,
				NewValue:    liveCol.Type,
				Description: fmt.Sprintf("Column %q type changed from %q to %q", lockedCol.Name, lockedCol.Type, liveCol.Type),
			})
		}

		// Check nullable change (making non-nullable is breaking for consumers writing data).
		if lockedCol.Nullable && !liveCol.Nullable {
			report.Items = append(report.Items, DriftItem{
				Type:        DriftBreaking,
				Category:    "nullable_changed",
				TableName:   locked.Name,
				ColumnName:  lockedCol.Name,
				OldValue:    "nullable",
				NewValue:    "not null",
				Description: fmt.Sprintf("Column %q changed from nullable to NOT NULL", lockedCol.Name),
			})
		} else if !lockedCol.Nullable && liveCol.Nullable {
			// Making a column nullable is additive (consumers can still send non-null).
			report.Items = append(report.Items, DriftItem{
				Type:        DriftAdditive,
				Category:    "nullable_changed",
				TableName:   locked.Name,
				ColumnName:  lockedCol.Name,
				OldValue:    "not null",
				NewValue:    "nullable",
				Description: fmt.Sprintf("Column %q changed from NOT NULL to nullable", lockedCol.Name),
			})
		}
	}

	// Check for added columns (additive).
	for _, liveCol := range live.Columns {
		if _, exists := lockedByName[liveCol.Name]; !exists {
			report.Items = append(report.Items, DriftItem{
				Type:        DriftAdditive,
				Category:    "column_added",
				TableName:   locked.Name,
				ColumnName:  liveCol.Name,
				NewValue:    liveCol.Type,
				Description: fmt.Sprintf("Column %q was added to table %q", liveCol.Name, locked.Name),
			})
		}
	}

	// Summarize.
	for _, item := range report.Items {
		switch item.Type {
		case DriftAdditive:
			report.AdditiveCount++
		case DriftBreaking:
			report.BreakingCount++
		}
	}
	report.HasDrift = len(report.Items) > 0
	report.HasBreaking = report.BreakingCount > 0

	return report
}

// DiffSchema compares all locked contracts for a service against the live schema.
func DiffSchema(serviceName string, contracts []Contract, live *model.Schema, lockMode LockMode) ServiceDriftReport {
	report := ServiceDriftReport{
		ServiceName: serviceName,
		LockMode:    lockMode,
		TotalTables: len(contracts),
	}

	// Index live tables/views by name.
	liveByName := make(map[string]model.TableSchema)
	for _, t := range live.Tables {
		liveByName[t.Name] = t
	}
	for _, v := range live.Views {
		liveByName[v.Name] = v
	}

	for _, c := range contracts {
		liveTable, exists := liveByName[c.TableName]
		if !exists {
			// Entire table was dropped — breaking.
			dr := DriftReport{
				ServiceName:   serviceName,
				TableName:     c.TableName,
				HasDrift:      true,
				HasBreaking:   true,
				BreakingCount: 1,
				LockedAt:      c.LockedAt,
				CheckedAt:     time.Now().UTC(),
				Items: []DriftItem{{
					Type:        DriftBreaking,
					Category:    "table_removed",
					TableName:   c.TableName,
					Description: fmt.Sprintf("Table %q was removed from the database", c.TableName),
				}},
			}
			report.Tables = append(report.Tables, dr)
			report.DriftedTables++
			report.BreakingCount++
			continue
		}

		dr := DiffTable(serviceName, c.Schema, liveTable, c.LockedAt)
		report.Tables = append(report.Tables, dr)
		if dr.HasDrift {
			report.DriftedTables++
		}
		if dr.HasBreaking {
			report.BreakingCount += dr.BreakingCount
		}
	}

	return report
}
