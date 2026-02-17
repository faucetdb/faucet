package query

import (
	"fmt"
	"strings"
)

// OrderClause represents a single column ordering directive.
type OrderClause struct {
	Column    string // Validated column name.
	Direction string // "ASC" or "DESC".
}

// String returns the SQL fragment for this order clause, e.g. "created_at DESC".
func (o OrderClause) String() string {
	return o.Column + " " + o.Direction
}

// ParseOrderClause parses a DreamFactory-style order string like
// "created_at DESC, name ASC" into validated OrderClause slices.
// Each element is "column [ASC|DESC]"; direction defaults to ASC if omitted.
func ParseOrderClause(order string) ([]OrderClause, error) {
	order = strings.TrimSpace(order)
	if order == "" {
		return nil, nil
	}

	parts := strings.Split(order, ",")
	clauses := make([]OrderClause, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		tokens := strings.Fields(part)
		if len(tokens) == 0 {
			continue
		}
		if len(tokens) > 2 {
			return nil, fmt.Errorf("invalid order clause %q: expected 'column [ASC|DESC]'", part)
		}

		col := tokens[0]
		if err := ValidateIdentifier(col); err != nil {
			return nil, fmt.Errorf("invalid order column: %w", err)
		}

		dir := "ASC"
		if len(tokens) == 2 {
			d := strings.ToUpper(tokens[1])
			switch d {
			case "ASC", "DESC":
				dir = d
			default:
				return nil, fmt.Errorf("invalid order direction %q: must be ASC or DESC", tokens[1])
			}
		}

		clauses = append(clauses, OrderClause{Column: col, Direction: dir})
	}

	if len(clauses) == 0 {
		return nil, nil
	}
	return clauses, nil
}

// BuildOrderSQL builds an ORDER BY SQL fragment from order clauses, applying
// the given quote function to column names.
func BuildOrderSQL(clauses []OrderClause, quoteFn func(string) string) string {
	if len(clauses) == 0 {
		return ""
	}
	parts := make([]string, len(clauses))
	for i, c := range clauses {
		parts[i] = quoteFn(c.Column) + " " + c.Direction
	}
	return "ORDER BY " + strings.Join(parts, ", ")
}

// ParseFieldSelection parses a comma-separated field list like "id,name,email"
// into a slice of validated column names. Whitespace around names is trimmed.
// Returns nil for an empty input string.
func ParseFieldSelection(fields string) ([]string, error) {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return nil, nil
	}

	parts := strings.Split(fields, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		col := strings.TrimSpace(part)
		if col == "" {
			continue
		}
		if err := ValidateIdentifier(col); err != nil {
			return nil, fmt.Errorf("invalid field name: %w", err)
		}
		result = append(result, col)
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// QuoteIdentifiers validates, quotes, and joins column names into a
// comma-separated SQL fragment. For example, with PostgreSQL quoting:
// ["id", "name", "email"] -> `"id", "name", "email"`
func QuoteIdentifiers(names []string, quoteFn func(string) string) (string, error) {
	if len(names) == 0 {
		return "*", nil
	}

	quoted := make([]string, len(names))
	for i, name := range names {
		if err := ValidateIdentifier(name); err != nil {
			return "", err
		}
		quoted[i] = quoteFn(name)
	}
	return strings.Join(quoted, ", "), nil
}

// PostgresQuote returns a PostgreSQL-style double-quoted identifier.
func PostgresQuote(name string) string {
	// Escape any embedded double quotes by doubling them.
	escaped := strings.ReplaceAll(name, `"`, `""`)
	return `"` + escaped + `"`
}

// MySQLQuote returns a MySQL-style backtick-quoted identifier.
func MySQLQuote(name string) string {
	escaped := strings.ReplaceAll(name, "`", "``")
	return "`" + escaped + "`"
}

// SQLServerQuote returns a SQL Server-style bracket-quoted identifier.
func SQLServerQuote(name string) string {
	escaped := strings.ReplaceAll(name, "]", "]]")
	return "[" + escaped + "]"
}

// BuildLimitOffset returns a LIMIT/OFFSET SQL fragment suitable for
// PostgreSQL and MySQL. Returns empty string if limit is 0.
func BuildLimitOffset(limit, offset int) string {
	if limit <= 0 {
		return ""
	}
	s := fmt.Sprintf("LIMIT %d", limit)
	if offset > 0 {
		s += fmt.Sprintf(" OFFSET %d", offset)
	}
	return s
}
