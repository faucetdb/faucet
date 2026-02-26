package contract

import (
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/model"
)

func TestDiffTable_NoDrift(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer", Nullable: false},
			{Name: "name", Type: "varchar(255)", Nullable: false},
			{Name: "email", Type: "varchar(255)", Nullable: true},
		},
	}

	report := DiffTable("mydb", locked, locked, time.Now())

	if report.HasDrift {
		t.Errorf("expected no drift, got %d items", len(report.Items))
	}
	if report.HasBreaking {
		t.Error("expected no breaking changes")
	}
}

func TestDiffTable_ColumnAdded(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "varchar(255)"},
		},
	}
	live := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "varchar(255)"},
			{Name: "bio", Type: "text"},
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if !report.HasDrift {
		t.Fatal("expected drift")
	}
	if report.HasBreaking {
		t.Error("adding a column should not be breaking")
	}
	if report.AdditiveCount != 1 {
		t.Errorf("expected 1 additive change, got %d", report.AdditiveCount)
	}
	if report.Items[0].Category != "column_added" {
		t.Errorf("expected column_added, got %s", report.Items[0].Category)
	}
	if report.Items[0].ColumnName != "bio" {
		t.Errorf("expected column name 'bio', got %s", report.Items[0].ColumnName)
	}
}

func TestDiffTable_ColumnRemoved(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "varchar(255)"},
			{Name: "bio", Type: "text"},
		},
	}
	live := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "name", Type: "varchar(255)"},
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if !report.HasBreaking {
		t.Fatal("removing a column should be breaking")
	}
	if report.BreakingCount != 1 {
		t.Errorf("expected 1 breaking change, got %d", report.BreakingCount)
	}
	if report.Items[0].Category != "column_removed" {
		t.Errorf("expected column_removed, got %s", report.Items[0].Category)
	}
}

func TestDiffTable_TypeChanged(t *testing.T) {
	locked := model.TableSchema{
		Name: "orders",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "total", Type: "decimal(10,2)"},
		},
	}
	live := model.TableSchema{
		Name: "orders",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "total", Type: "integer"},
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if !report.HasBreaking {
		t.Fatal("type change should be breaking")
	}
	if report.Items[0].Category != "type_changed" {
		t.Errorf("expected type_changed, got %s", report.Items[0].Category)
	}
	if report.Items[0].OldValue != "decimal(10,2)" {
		t.Errorf("expected old value decimal(10,2), got %s", report.Items[0].OldValue)
	}
	if report.Items[0].NewValue != "integer" {
		t.Errorf("expected new value integer, got %s", report.Items[0].NewValue)
	}
}

func TestDiffTable_NullableToNotNull(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "bio", Type: "text", Nullable: true},
		},
	}
	live := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "bio", Type: "text", Nullable: false},
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if !report.HasBreaking {
		t.Fatal("nullable -> not null should be breaking")
	}
	if report.Items[0].Category != "nullable_changed" {
		t.Errorf("expected nullable_changed, got %s", report.Items[0].Category)
	}
}

func TestDiffTable_NotNullToNullable(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "bio", Type: "text", Nullable: false},
		},
	}
	live := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "bio", Type: "text", Nullable: true},
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if report.HasBreaking {
		t.Error("not null -> nullable should not be breaking")
	}
	if report.AdditiveCount != 1 {
		t.Errorf("expected 1 additive change, got %d", report.AdditiveCount)
	}
}

func TestDiffTable_MultipleChanges(t *testing.T) {
	locked := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "username", Type: "varchar(50)"},
			{Name: "email", Type: "varchar(255)", Nullable: true},
		},
	}
	live := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "user_name", Type: "varchar(100)"}, // "username" removed, "user_name" added
			{Name: "email", Type: "text", Nullable: true}, // type changed
			{Name: "bio", Type: "text", Nullable: true}, // new column
		},
	}

	report := DiffTable("mydb", locked, live, time.Now())

	if !report.HasBreaking {
		t.Fatal("expected breaking changes")
	}
	// username removed (breaking), email type changed (breaking), user_name added (additive), bio added (additive)
	if report.BreakingCount != 2 {
		t.Errorf("expected 2 breaking changes, got %d", report.BreakingCount)
	}
	if report.AdditiveCount != 2 {
		t.Errorf("expected 2 additive changes, got %d", report.AdditiveCount)
	}
}

func TestDiffSchema_TableRemoved(t *testing.T) {
	contracts := []Contract{
		{
			ServiceName: "mydb",
			TableName:   "users",
			Schema: model.TableSchema{
				Name:    "users",
				Columns: []model.Column{{Name: "id", Type: "integer"}},
			},
			LockedAt: time.Now(),
		},
	}

	live := &model.Schema{
		Tables: []model.TableSchema{}, // users table was dropped
	}

	report := DiffSchema("mydb", contracts, live, LockModeAuto)

	if report.DriftedTables != 1 {
		t.Errorf("expected 1 drifted table, got %d", report.DriftedTables)
	}
	if report.BreakingCount != 1 {
		t.Errorf("expected 1 breaking change, got %d", report.BreakingCount)
	}
	if report.Tables[0].Items[0].Category != "table_removed" {
		t.Errorf("expected table_removed, got %s", report.Tables[0].Items[0].Category)
	}
}

func TestDiffSchema_NoContracts(t *testing.T) {
	report := DiffSchema("mydb", nil, &model.Schema{}, LockModeNone)

	if report.DriftedTables != 0 {
		t.Errorf("expected 0 drifted tables, got %d", report.DriftedTables)
	}
}

func TestValidLockMode(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"none", true},
		{"auto", true},
		{"strict", true},
		{"", false},
		{"invalid", false},
		{"NONE", false},
	}
	for _, tt := range tests {
		if got := ValidLockMode(tt.input); got != tt.valid {
			t.Errorf("ValidLockMode(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}
