package query

import (
	"strings"
	"testing"
)

func TestParseOrderClause(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []OrderClause
		wantErr bool
	}{
		{
			"single column default ASC",
			"name",
			[]OrderClause{{Column: "name", Direction: "ASC"}},
			false,
		},
		{
			"single column DESC",
			"created_at DESC",
			[]OrderClause{{Column: "created_at", Direction: "DESC"}},
			false,
		},
		{
			"multiple columns",
			"created_at DESC, name ASC",
			[]OrderClause{
				{Column: "created_at", Direction: "DESC"},
				{Column: "name", Direction: "ASC"},
			},
			false,
		},
		{
			"case insensitive direction",
			"name desc",
			[]OrderClause{{Column: "name", Direction: "DESC"}},
			false,
		},
		{
			"empty string",
			"",
			nil,
			false,
		},
		{
			"whitespace only",
			"   ",
			nil,
			false,
		},
		{
			"invalid direction",
			"name SIDEWAYS",
			nil,
			true,
		},
		{
			"too many tokens",
			"name ASC EXTRA",
			nil,
			true,
		},
		{
			"invalid column name",
			"1name ASC",
			nil,
			true,
		},
		{
			"reserved word column",
			"SELECT DESC",
			nil,
			true,
		},
		{
			"trailing comma ignored",
			"name ASC,",
			[]OrderClause{{Column: "name", Direction: "ASC"}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOrderClause(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d clauses, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Column != tt.want[i].Column || got[i].Direction != tt.want[i].Direction {
					t.Errorf("clause[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildOrderSQL(t *testing.T) {
	clauses := []OrderClause{
		{Column: "created_at", Direction: "DESC"},
		{Column: "name", Direction: "ASC"},
	}

	got := BuildOrderSQL(clauses, PostgresQuote)
	want := `ORDER BY "created_at" DESC, "name" ASC`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Empty clauses.
	got = BuildOrderSQL(nil, PostgresQuote)
	if got != "" {
		t.Errorf("expected empty string for nil clauses, got %q", got)
	}
}

func TestParseFieldSelection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"basic fields", "id,name,email", []string{"id", "name", "email"}, false},
		{"with spaces", " id , name , email ", []string{"id", "name", "email"}, false},
		{"single field", "id", []string{"id"}, false},
		{"empty string", "", nil, false},
		{"whitespace only", "   ", nil, false},
		{"invalid field", "id,1bad,name", nil, true},
		{"reserved word", "id,SELECT,name", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFieldSelection(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("field[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestQuoteIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		quoteFn func(string) string
		want    string
		wantErr bool
	}{
		{"postgres style", []string{"id", "name"}, PostgresQuote, `"id", "name"`, false},
		{"mysql style", []string{"id", "name"}, MySQLQuote, "`id`, `name`", false},
		{"sqlserver style", []string{"id", "name"}, SQLServerQuote, "[id], [name]", false},
		{"empty returns star", nil, PostgresQuote, "*", false},
		{"invalid identifier", []string{"id", "1bad"}, PostgresQuote, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := QuoteIdentifiers(tt.names, tt.quoteFn)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrderClauseString(t *testing.T) {
	oc := OrderClause{Column: "name", Direction: "DESC"}
	if oc.String() != "name DESC" {
		t.Errorf("got %q, want %q", oc.String(), "name DESC")
	}
}

func TestBuildLimitOffset(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		offset int
		want   string
	}{
		{"no limit", 0, 0, ""},
		{"negative limit", -1, 0, ""},
		{"limit only", 10, 0, "LIMIT 10"},
		{"limit and offset", 10, 20, "LIMIT 10 OFFSET 20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLimitOffset(tt.limit, tt.offset)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuoteFunctions(t *testing.T) {
	// Test embedded special characters in identifiers (defense in depth).
	t.Run("postgres double quote escape", func(t *testing.T) {
		got := PostgresQuote(`col"name`)
		if !strings.Contains(got, `""`) {
			t.Errorf("expected escaped double quote, got %q", got)
		}
	})

	t.Run("mysql backtick escape", func(t *testing.T) {
		got := MySQLQuote("col`name")
		if !strings.Contains(got, "``") {
			t.Errorf("expected escaped backtick, got %q", got)
		}
	})

	t.Run("sqlserver bracket escape", func(t *testing.T) {
		got := SQLServerQuote("col]name")
		if !strings.Contains(got, "]]") {
			t.Errorf("expected escaped bracket, got %q", got)
		}
	})
}
