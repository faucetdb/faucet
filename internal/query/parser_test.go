package query

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tokenizer tests
// ---------------------------------------------------------------------------

func TestTokenize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []tokenType
		wantErr bool
	}{
		{
			"simple comparison",
			"age > 21",
			[]tokenType{tokIdentifier, tokOperator, tokNumber},
			false,
		},
		{
			"string value",
			"name = 'John'",
			[]tokenType{tokIdentifier, tokOperator, tokString},
			false,
		},
		{
			"parenthesized AND",
			"(age > 21) AND (status = 'active')",
			[]tokenType{tokLParen, tokIdentifier, tokOperator, tokNumber, tokRParen, tokAND, tokLParen, tokIdentifier, tokOperator, tokString, tokRParen},
			false,
		},
		{
			"IN list",
			"name IN ('John', 'Jane')",
			[]tokenType{tokIdentifier, tokIN, tokLParen, tokString, tokComma, tokString, tokRParen},
			false,
		},
		{
			"IS NULL",
			"email IS NULL",
			[]tokenType{tokIdentifier, tokIS, tokNULL},
			false,
		},
		{
			"BETWEEN",
			"age BETWEEN 18 AND 65",
			[]tokenType{tokIdentifier, tokBETWEEN, tokNumber, tokAND, tokNumber},
			false,
		},
		{
			"all operators",
			"a = 1 AND b != 2 AND c <> 3 AND d > 4 AND e >= 5 AND f < 6 AND g <= 7",
			[]tokenType{
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber, tokAND,
				tokIdentifier, tokOperator, tokNumber,
			},
			false,
		},
		{
			"case insensitive keywords",
			"name like 'test' and status in ('a') or active is not null",
			[]tokenType{
				tokIdentifier, tokLIKE, tokString, tokAND,
				tokIdentifier, tokIN, tokLParen, tokString, tokRParen, tokOR,
				tokIdentifier, tokIS, tokNOT, tokNULL,
			},
			false,
		},
		{
			"escaped quote in string",
			"name = 'O''Brien'",
			[]tokenType{tokIdentifier, tokOperator, tokString},
			false,
		},
		{
			"decimal number",
			"price > 19.99",
			[]tokenType{tokIdentifier, tokOperator, tokNumber},
			false,
		},
		{
			"negative number",
			"temp > -10",
			[]tokenType{tokIdentifier, tokOperator, tokNumber},
			false,
		},
		{
			"CONTAINS keyword",
			"name CONTAINS 'test'",
			[]tokenType{tokIdentifier, tokCONTAINS, tokString},
			false,
		},
		{
			"STARTS WITH keywords",
			"name STARTS WITH 'J'",
			[]tokenType{tokIdentifier, tokSTARTS, tokWITH, tokString},
			false,
		},
		{
			"ENDS WITH keywords",
			"name ENDS WITH 'son'",
			[]tokenType{tokIdentifier, tokENDS, tokWITH, tokString},
			false,
		},
		{
			"unterminated string",
			"name = 'John",
			nil,
			true,
		},
		{
			"unexpected character",
			"name @ 'test'",
			nil,
			true,
		},
		{
			"trailing decimal point",
			"price > 19.",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tokens) != len(tt.want) {
				types := make([]string, len(tokens))
				for i, tok := range tokens {
					types[i] = fmt.Sprintf("%s(%q)", tokenTypeName(tok.typ), tok.value)
				}
				t.Fatalf("got %d tokens %v, want %d types", len(tokens), types, len(tt.want))
			}
			for i := range tokens {
				if tokens[i].typ != tt.want[i] {
					t.Errorf("token[%d].typ = %s, want %s (value=%q)", i, tokenTypeName(tokens[i].typ), tokenTypeName(tt.want[i]), tokens[i].value)
				}
			}
		})
	}
}

func TestTokenizeEscapedQuote(t *testing.T) {
	tokens, err := tokenize("name = 'O''Brien'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[2].value != "O'Brien" {
		t.Errorf("expected %q, got %q", "O'Brien", tokens[2].value)
	}
}

// ---------------------------------------------------------------------------
// ParseFilter tests
// ---------------------------------------------------------------------------

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name       string
		filter     string
		wantSQL    string
		wantParams []interface{}
		wantErr    bool
	}{
		// Basic comparisons.
		{
			"simple greater than",
			"age > 21",
			"age > $1",
			[]interface{}{int64(21)},
			false,
		},
		{
			"string equality",
			"name = 'John'",
			"name = $1",
			[]interface{}{"John"},
			false,
		},
		{
			"not equal !=",
			"status != 'inactive'",
			"status != $1",
			[]interface{}{"inactive"},
			false,
		},
		{
			"not equal <>",
			"status <> 'inactive'",
			"status <> $1",
			[]interface{}{"inactive"},
			false,
		},
		{
			"less than or equal",
			"age <= 65",
			"age <= $1",
			[]interface{}{int64(65)},
			false,
		},
		{
			"greater than or equal",
			"age >= 18",
			"age >= $1",
			[]interface{}{int64(18)},
			false,
		},
		{
			"less than",
			"price < 100",
			"price < $1",
			[]interface{}{int64(100)},
			false,
		},
		{
			"decimal value",
			"price > 19.99",
			"price > $1",
			[]interface{}{float64(19.99)},
			false,
		},
		{
			"negative number",
			"temp > -10",
			"temp > $1",
			[]interface{}{int64(-10)},
			false,
		},

		// Boolean logic.
		{
			"AND",
			"(age > 21) AND (status = 'active')",
			"(age > $1) AND (status = $2)",
			[]interface{}{int64(21), "active"},
			false,
		},
		{
			"OR",
			"(age < 18) OR (age > 65)",
			"(age < $1) OR (age > $2)",
			[]interface{}{int64(18), int64(65)},
			false,
		},
		{
			"NOT",
			"NOT (status = 'deleted')",
			"NOT (status = $1)",
			[]interface{}{"deleted"},
			false,
		},
		{
			"complex boolean",
			"(age > 21) AND ((status = 'active') OR (role = 'admin'))",
			"(age > $1) AND ((status = $2) OR (role = $3))",
			[]interface{}{int64(21), "active", "admin"},
			false,
		},
		{
			"AND without parens",
			"age > 21 AND status = 'active'",
			"age > $1 AND status = $2",
			[]interface{}{int64(21), "active"},
			false,
		},
		{
			"OR without parens",
			"age < 18 OR age > 65",
			"age < $1 OR age > $2",
			[]interface{}{int64(18), int64(65)},
			false,
		},

		// IN lists.
		{
			"IN with strings",
			"name IN ('John', 'Jane')",
			"name IN ($1, $2)",
			[]interface{}{"John", "Jane"},
			false,
		},
		{
			"IN with numbers",
			"id IN (1, 2, 3)",
			"id IN ($1, $2, $3)",
			[]interface{}{int64(1), int64(2), int64(3)},
			false,
		},
		{
			"NOT IN",
			"status NOT IN ('deleted', 'archived')",
			"status NOT IN ($1, $2)",
			[]interface{}{"deleted", "archived"},
			false,
		},

		// LIKE.
		{
			"LIKE",
			"name LIKE 'J%'",
			"name LIKE $1",
			[]interface{}{"J%"},
			false,
		},
		{
			"NOT LIKE",
			"name NOT LIKE '%test%'",
			"name NOT LIKE $1",
			[]interface{}{"%test%"},
			false,
		},

		// IS NULL / IS NOT NULL.
		{
			"IS NULL",
			"email IS NULL",
			"email IS NULL",
			nil,
			false,
		},
		{
			"IS NOT NULL",
			"email IS NOT NULL",
			"email IS NOT NULL",
			nil,
			false,
		},

		// BETWEEN.
		{
			"BETWEEN numbers",
			"age BETWEEN 18 AND 65",
			"age BETWEEN $1 AND $2",
			[]interface{}{int64(18), int64(65)},
			false,
		},
		{
			"BETWEEN strings",
			"name BETWEEN 'A' AND 'M'",
			"name BETWEEN $1 AND $2",
			[]interface{}{"A", "M"},
			false,
		},
		{
			"NOT BETWEEN",
			"age NOT BETWEEN 18 AND 65",
			"age NOT BETWEEN $1 AND $2",
			[]interface{}{int64(18), int64(65)},
			false,
		},

		// Synthetic operators.
		{
			"CONTAINS",
			"name CONTAINS 'test'",
			"name LIKE $1",
			[]interface{}{"%test%"},
			false,
		},
		{
			"STARTS WITH",
			"name STARTS WITH 'J'",
			"name LIKE $1",
			[]interface{}{"J%"},
			false,
		},
		{
			"ENDS WITH",
			"name ENDS WITH 'son'",
			"name LIKE $1",
			[]interface{}{"%son"},
			false,
		},

		// Empty filter.
		{
			"empty string",
			"",
			"",
			nil,
			false,
		},
		{
			"whitespace only",
			"   ",
			"",
			nil,
			false,
		},

		// Error cases.
		{
			"invalid column name",
			"1bad > 5",
			"",
			nil,
			true,
		},
		{
			"missing value",
			"age >",
			"",
			nil,
			true,
		},
		{
			"unclosed paren",
			"(age > 21",
			"",
			nil,
			true,
		},
		{
			"unexpected token",
			"age > 21 BLAH",
			"",
			nil,
			true,
		},
		{
			"IN without paren",
			"name IN 'John'",
			"",
			nil,
			true,
		},
		{
			"BETWEEN without AND",
			"age BETWEEN 18 OR 65",
			"",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFilter(tt.filter, DollarPlaceholder, 1)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for filter %q, got nil", tt.filter)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for filter %q: %v", tt.filter, err)
			}

			// Handle nil result for empty filters.
			if tt.wantSQL == "" {
				if result != nil {
					t.Errorf("expected nil result for empty filter, got SQL=%q", result.SQL)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected non-nil result for filter %q", tt.filter)
			}

			if result.SQL != tt.wantSQL {
				t.Errorf("SQL:\n  got  %q\n  want %q", result.SQL, tt.wantSQL)
			}

			if len(result.Params) != len(tt.wantParams) {
				t.Fatalf("params: got %v (len %d), want %v (len %d)", result.Params, len(result.Params), tt.wantParams, len(tt.wantParams))
			}
			for i := range result.Params {
				if result.Params[i] != tt.wantParams[i] {
					t.Errorf("param[%d]: got %v (%T), want %v (%T)", i, result.Params[i], result.Params[i], tt.wantParams[i], tt.wantParams[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Placeholder function tests
// ---------------------------------------------------------------------------

func TestPlaceholderFunctions(t *testing.T) {
	tests := []struct {
		name string
		fn   PlaceholderFunc
		idx  int
		want string
	}{
		{"dollar 1", DollarPlaceholder, 1, "$1"},
		{"dollar 5", DollarPlaceholder, 5, "$5"},
		{"question 1", QuestionPlaceholder, 1, "?"},
		{"question 5", QuestionPlaceholder, 5, "?"},
		{"atp 1", AtPPlaceholder, 1, "@p1"},
		{"atp 3", AtPPlaceholder, 3, "@p3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.idx)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Placeholder style tests with ParseFilter
// ---------------------------------------------------------------------------

func TestParseFilterWithDifferentPlaceholders(t *testing.T) {
	filter := "(age > 21) AND (name = 'John')"

	t.Run("mysql question marks", func(t *testing.T) {
		result, err := ParseFilter(filter, QuestionPlaceholder, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SQL != "(age > ?) AND (name = ?)" {
			t.Errorf("got %q", result.SQL)
		}
	})

	t.Run("sqlserver at-p", func(t *testing.T) {
		result, err := ParseFilter(filter, AtPPlaceholder, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SQL != "(age > @p1) AND (name = @p2)" {
			t.Errorf("got %q", result.SQL)
		}
	})
}

// ---------------------------------------------------------------------------
// Start index tests
// ---------------------------------------------------------------------------

func TestParseFilterStartIndex(t *testing.T) {
	result, err := ParseFilter("age > 21 AND name = 'John'", DollarPlaceholder, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SQL != "age > $5 AND name = $6" {
		t.Errorf("got SQL %q, want %q", result.SQL, "age > $5 AND name = $6")
	}
}

// ---------------------------------------------------------------------------
// SQL injection prevention tests
// ---------------------------------------------------------------------------

func TestParseFilterSQLInjection(t *testing.T) {
	injections := []struct {
		name   string
		filter string
	}{
		{"semicolon in column", "name; DROP TABLE users-- = 'x'"},
		{"comment in column", "name-- = 'x'"},
		{"subquery attempt", "(SELECT 1) = 1"},
	}

	for _, tt := range injections {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFilter(tt.filter, DollarPlaceholder, 1)
			if err == nil && result != nil {
				t.Errorf("expected error for injection attempt %q, got SQL=%q", tt.filter, result.SQL)
			}
		})
	}
}

// Test that values are NEVER interpolated into SQL.
func TestParseFilterParameterization(t *testing.T) {
	// A malicious string value that would be dangerous if interpolated.
	filter := "name = 'Robert''; DROP TABLE users--'"
	result, err := ParseFilter(filter, DollarPlaceholder, 1)
	if err != nil {
		// The tokenizer should handle this with escaped quotes. If it errors,
		// that's also safe (we never interpolate).
		return
	}

	// The SQL should only contain the placeholder, never the raw value.
	if strings.Contains(result.SQL, "DROP") {
		t.Errorf("SQL contains injected content: %q", result.SQL)
	}
	if strings.Contains(result.SQL, "Robert") {
		t.Errorf("SQL contains raw value (not parameterized): %q", result.SQL)
	}
}

// ---------------------------------------------------------------------------
// Nil placeholder defaults to dollar
// ---------------------------------------------------------------------------

func TestParseFilterNilPlaceholder(t *testing.T) {
	result, err := ParseFilter("age > 21", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SQL != "age > $1" {
		t.Errorf("got SQL %q, want %q", result.SQL, "age > $1")
	}
}

// ---------------------------------------------------------------------------
// Case insensitivity for operators
// ---------------------------------------------------------------------------

func TestParseFilterCaseInsensitive(t *testing.T) {
	tests := []struct {
		filter  string
		wantSQL string
	}{
		{"age > 21 and name = 'John'", "age > $1 AND name = $2"},
		{"age > 21 or name = 'John'", "age > $1 OR name = $2"},
		{"email is null", "email IS NULL"},
		{"email is not null", "email IS NOT NULL"},
		{"name like 'J%'", "name LIKE $1"},
		{"name in ('A', 'B')", "name IN ($1, $2)"},
		{"age between 1 and 10", "age BETWEEN $1 AND $2"},
		{"name contains 'test'", "name LIKE $1"},
		{"name starts with 'J'", "name LIKE $1"},
		{"name ends with 'y'", "name LIKE $1"},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			result, err := ParseFilter(tt.filter, DollarPlaceholder, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.SQL != tt.wantSQL {
				t.Errorf("got SQL %q, want %q", result.SQL, tt.wantSQL)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Deeply nested parentheses
// ---------------------------------------------------------------------------

func TestParseFilterNestedParens(t *testing.T) {
	filter := "((((age > 21))))"
	result, err := ParseFilter(filter, DollarPlaceholder, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SQL != "((((age > $1))))" {
		t.Errorf("got SQL %q", result.SQL)
	}
}

// ---------------------------------------------------------------------------
// Complex real-world filter
// ---------------------------------------------------------------------------

func TestParseFilterComplex(t *testing.T) {
	filter := "(status = 'active') AND (age >= 18) AND (age <= 65) AND (department IN ('eng', 'sales', 'marketing')) AND (email IS NOT NULL) AND (name LIKE 'J%')"
	result, err := ParseFilter(filter, DollarPlaceholder, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "(status = $1) AND (age >= $2) AND (age <= $3) AND (department IN ($4, $5, $6)) AND (email IS NOT NULL) AND (name LIKE $7)"
	if result.SQL != wantSQL {
		t.Errorf("SQL:\n  got  %q\n  want %q", result.SQL, wantSQL)
	}

	wantParams := []interface{}{"active", int64(18), int64(65), "eng", "sales", "marketing", "J%"}
	if len(result.Params) != len(wantParams) {
		t.Fatalf("params length: got %d, want %d", len(result.Params), len(wantParams))
	}
	for i := range result.Params {
		if result.Params[i] != wantParams[i] {
			t.Errorf("param[%d]: got %v (%T), want %v (%T)", i, result.Params[i], result.Params[i], wantParams[i], wantParams[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkParseFilter(b *testing.B) {
	filters := []struct {
		name   string
		filter string
	}{
		{"simple", "age > 21"},
		{"with_and", "(age > 21) AND (status = 'active')"},
		{"complex", "(status = 'active') AND (age >= 18) AND (department IN ('eng', 'sales')) AND (email IS NOT NULL)"},
	}

	for _, f := range filters {
		b.Run(f.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := ParseFilter(f.filter, DollarPlaceholder, 1)
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Ensure the tokenTypeName function covers all types for completeness.
func TestTokenTypeName(t *testing.T) {
	types := []tokenType{
		tokIdentifier, tokNumber, tokString, tokOperator,
		tokLParen, tokRParen, tokComma,
		tokAND, tokOR, tokNOT, tokIN, tokLIKE, tokIS, tokNULL,
		tokBETWEEN, tokCONTAINS, tokSTARTS, tokENDS, tokWITH,
	}
	for _, tt := range types {
		name := tokenTypeName(tt)
		if name == "" || strings.HasPrefix(name, "token(") {
			t.Errorf("tokenTypeName(%d) returned fallback %q", tt, name)
		}
	}

	// Unknown type should return fallback.
	name := tokenTypeName(tokenType(999))
	if !strings.HasPrefix(name, "token(") {
		t.Errorf("expected fallback for unknown type, got %q", name)
	}
}

// Test that qualified column names with dots are supported.
func TestParseFilterQualifiedColumn(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		wantSQL string
		wantErr bool
	}{
		{
			"table.column",
			"users.age > 21",
			"users.age > $1",
			false,
		},
		{
			"schema.table.column",
			"public.users.age > 21",
			"public.users.age > $1",
			false,
		},
		{
			"too many dots",
			"a.b.c.d > 1",
			"",
			true,
		},
		{
			"reserved word in qualifier",
			"SELECT.age > 1",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFilter(tt.filter, DollarPlaceholder, 1)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.filter)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.filter, err)
			}
			if result.SQL != tt.wantSQL {
				t.Errorf("got SQL %q, want %q", result.SQL, tt.wantSQL)
			}
		})
	}
}
