package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/faucetdb/faucet/internal/contract"
	"github.com/faucetdb/faucet/internal/model"
)

// SaveContract creates or replaces a schema contract for a service/table pair.
func (s *Store) SaveContract(ctx context.Context, serviceName, tableName string, schema model.TableSchema) (*contract.Contract, error) {
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}

	now := time.Now().UTC()
	const q = `INSERT INTO schema_contracts (service_name, table_name, schema_json, locked_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(service_name, table_name) DO UPDATE SET
			schema_json = excluded.schema_json,
			locked_at = excluded.locked_at,
			promoted_at = excluded.locked_at`

	result, err := s.db.ExecContext(ctx, q, serviceName, tableName, string(schemaJSON), now)
	if err != nil {
		return nil, fmt.Errorf("save contract: %w", err)
	}

	id, _ := result.LastInsertId()
	return &contract.Contract{
		ID:          id,
		ServiceName: serviceName,
		TableName:   tableName,
		Schema:      schema,
		SchemaJSON:  string(schemaJSON),
		LockedAt:    now,
		PromotedAt:  &now,
	}, nil
}

// GetContract returns a single schema contract by service and table name.
func (s *Store) GetContract(ctx context.Context, serviceName, tableName string) (*contract.Contract, error) {
	var c contract.Contract
	const q = `SELECT id, service_name, table_name, schema_json, locked_at, promoted_at
		FROM schema_contracts WHERE service_name = ? AND table_name = ?`
	if err := s.db.GetContext(ctx, &c, q, serviceName, tableName); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get contract: %w", err)
	}
	if err := json.Unmarshal([]byte(c.SchemaJSON), &c.Schema); err != nil {
		return nil, fmt.Errorf("unmarshal contract schema: %w", err)
	}
	return &c, nil
}

// ListContracts returns all schema contracts for a service.
func (s *Store) ListContracts(ctx context.Context, serviceName string) ([]contract.Contract, error) {
	var rows []contract.Contract
	const q = `SELECT id, service_name, table_name, schema_json, locked_at, promoted_at
		FROM schema_contracts WHERE service_name = ? ORDER BY table_name`
	if err := s.db.SelectContext(ctx, &rows, q, serviceName); err != nil {
		return nil, fmt.Errorf("list contracts: %w", err)
	}

	for i := range rows {
		if err := json.Unmarshal([]byte(rows[i].SchemaJSON), &rows[i].Schema); err != nil {
			return nil, fmt.Errorf("unmarshal contract schema for %s: %w", rows[i].TableName, err)
		}
	}
	return rows, nil
}

// DeleteContract removes a schema contract for a service/table pair.
func (s *Store) DeleteContract(ctx context.Context, serviceName, tableName string) error {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM schema_contracts WHERE service_name = ? AND table_name = ?",
		serviceName, tableName)
	if err != nil {
		return fmt.Errorf("delete contract: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteServiceContracts removes all schema contracts for a service.
func (s *Store) DeleteServiceContracts(ctx context.Context, serviceName string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM schema_contracts WHERE service_name = ?", serviceName)
	if err != nil {
		return 0, fmt.Errorf("delete service contracts: %w", err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// PromoteContract updates a contract's schema to the latest live schema snapshot.
func (s *Store) PromoteContract(ctx context.Context, serviceName, tableName string, schema model.TableSchema) error {
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	now := time.Now().UTC()
	const q = `UPDATE schema_contracts SET schema_json = ?, promoted_at = ?
		WHERE service_name = ? AND table_name = ?`
	result, err := s.db.ExecContext(ctx, q, string(schemaJSON), now, serviceName, tableName)
	if err != nil {
		return fmt.Errorf("promote contract: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
