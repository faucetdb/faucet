package config

import (
	"context"
	"testing"

	"github.com/faucetdb/faucet/internal/model"
)

func TestSaveAndGetContract(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	schema := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer", IsPrimaryKey: true},
			{Name: "name", Type: "varchar(255)"},
		},
		PrimaryKey: []string{"id"},
	}

	c, err := store.SaveContract(ctx, "mydb", "users", schema)
	if err != nil {
		t.Fatalf("save contract: %v", err)
	}
	if c.ServiceName != "mydb" {
		t.Errorf("expected service name mydb, got %s", c.ServiceName)
	}
	if c.TableName != "users" {
		t.Errorf("expected table name users, got %s", c.TableName)
	}

	// Retrieve it.
	got, err := store.GetContract(ctx, "mydb", "users")
	if err != nil {
		t.Fatalf("get contract: %v", err)
	}
	if got.Schema.Name != "users" {
		t.Errorf("expected schema name users, got %s", got.Schema.Name)
	}
	if len(got.Schema.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(got.Schema.Columns))
	}
}

func TestSaveContract_Upsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	schema1 := model.TableSchema{
		Name:    "users",
		Columns: []model.Column{{Name: "id", Type: "integer"}},
	}
	schema2 := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "email", Type: "varchar(255)"},
		},
	}

	store.SaveContract(ctx, "mydb", "users", schema1)
	store.SaveContract(ctx, "mydb", "users", schema2) // upsert

	got, err := store.GetContract(ctx, "mydb", "users")
	if err != nil {
		t.Fatalf("get contract: %v", err)
	}
	if len(got.Schema.Columns) != 2 {
		t.Errorf("expected 2 columns after upsert, got %d", len(got.Schema.Columns))
	}
}

func TestListContracts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveContract(ctx, "mydb", "users", model.TableSchema{Name: "users"})
	store.SaveContract(ctx, "mydb", "orders", model.TableSchema{Name: "orders"})
	store.SaveContract(ctx, "otherdb", "products", model.TableSchema{Name: "products"})

	contracts, err := store.ListContracts(ctx, "mydb")
	if err != nil {
		t.Fatalf("list contracts: %v", err)
	}
	if len(contracts) != 2 {
		t.Errorf("expected 2 contracts for mydb, got %d", len(contracts))
	}
}

func TestDeleteContract(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveContract(ctx, "mydb", "users", model.TableSchema{Name: "users"})

	err := store.DeleteContract(ctx, "mydb", "users")
	if err != nil {
		t.Fatalf("delete contract: %v", err)
	}

	_, err = store.GetContract(ctx, "mydb", "users")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteContract_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.DeleteContract(ctx, "mydb", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteServiceContracts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SaveContract(ctx, "mydb", "users", model.TableSchema{Name: "users"})
	store.SaveContract(ctx, "mydb", "orders", model.TableSchema{Name: "orders"})

	n, err := store.DeleteServiceContracts(ctx, "mydb")
	if err != nil {
		t.Fatalf("delete service contracts: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 deleted, got %d", n)
	}

	contracts, _ := store.ListContracts(ctx, "mydb")
	if len(contracts) != 0 {
		t.Errorf("expected 0 contracts after delete, got %d", len(contracts))
	}
}

func TestPromoteContract(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	original := model.TableSchema{
		Name:    "users",
		Columns: []model.Column{{Name: "id", Type: "integer"}},
	}
	store.SaveContract(ctx, "mydb", "users", original)

	updated := model.TableSchema{
		Name: "users",
		Columns: []model.Column{
			{Name: "id", Type: "integer"},
			{Name: "bio", Type: "text"},
		},
	}
	err := store.PromoteContract(ctx, "mydb", "users", updated)
	if err != nil {
		t.Fatalf("promote contract: %v", err)
	}

	got, _ := store.GetContract(ctx, "mydb", "users")
	if len(got.Schema.Columns) != 2 {
		t.Errorf("expected 2 columns after promote, got %d", len(got.Schema.Columns))
	}
	if got.PromotedAt == nil {
		t.Error("expected promoted_at to be set")
	}
}

func TestPromoteContract_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.PromoteContract(ctx, "mydb", "nonexistent", model.TableSchema{})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSchemaLockMigration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create a service — schema_lock should default to 'none'.
	svc := &model.ServiceConfig{
		Name:   "testdb",
		Driver: "postgres",
		DSN:    "postgres://localhost/test",
		Pool:   model.DefaultPoolConfig(),
	}
	if err := store.CreateService(ctx, svc); err != nil {
		t.Fatalf("create service: %v", err)
	}

	got, err := store.GetServiceByName(ctx, "testdb")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if got.SchemaLock != "none" {
		t.Errorf("expected schema_lock 'none', got %q", got.SchemaLock)
	}

	// Update to auto.
	got.SchemaLock = "auto"
	if err := store.UpdateService(ctx, got); err != nil {
		t.Fatalf("update service: %v", err)
	}

	got2, _ := store.GetServiceByName(ctx, "testdb")
	if got2.SchemaLock != "auto" {
		t.Errorf("expected schema_lock 'auto' after update, got %q", got2.SchemaLock)
	}
}
