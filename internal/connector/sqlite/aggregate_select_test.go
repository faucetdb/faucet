package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/query"
)

// TestBuildSelectAggregates verifies GROUP BY and aggregate projection
// rendering, including ordering by an aggregate's alias.
func TestBuildSelectAggregates(t *testing.T) {
	c := newTestConnector()

	tests := []struct {
		name     string
		req      connector.SelectRequest
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name: "group by with sum aggregate",
			req: connector.SelectRequest{
				Table: "invoices",
				Projection: []query.SelectItem{
					{Column: "region"},
					{Func: "SUM", Column: "amount", Alias: "sum_amount"},
				},
				GroupBy: []string{"region"},
			},
			wantSQL: `SELECT "region", SUM("amount") AS "sum_amount" FROM "invoices" GROUP BY "region"`,
		},
		{
			name: "count star grouped, ordered by alias, with limit",
			req: connector.SelectRequest{
				Table: "invoices",
				Projection: []query.SelectItem{
					{Column: "status"},
					{Func: "COUNT", Column: "*", Alias: "count"},
				},
				GroupBy: []string{"status"},
				Order:   `"count" DESC`,
				Limit:   5,
			},
			wantSQL:  `SELECT "status", COUNT(*) AS "count" FROM "invoices" GROUP BY "status" ORDER BY "count" DESC LIMIT ?`,
			wantArgs: []interface{}{5},
		},
		{
			name: "whole-table aggregate without group",
			req: connector.SelectRequest{
				Table: "invoices",
				Projection: []query.SelectItem{
					{Func: "SUM", Column: "amount", Alias: "sum_amount"},
				},
			},
			wantSQL: `SELECT SUM("amount") AS "sum_amount" FROM "invoices"`,
		},
		{
			name: "multi-column group by",
			req: connector.SelectRequest{
				Table: "invoices",
				Projection: []query.SelectItem{
					{Column: "region"},
					{Column: "status"},
					{Func: "AVG", Column: "amount", Alias: "avg_amount"},
				},
				GroupBy: []string{"region", "status"},
			},
			wantSQL: `SELECT "region", "status", AVG("amount") AS "avg_amount" FROM "invoices" GROUP BY "region", "status"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotArgs, err := c.BuildSelect(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("BuildSelect() unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL =\n  %s\nwant\n  %s", gotSQL, tt.wantSQL)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}
