package query

import (
	"fmt"
	"regexp"
	"strings"
)

// aggregateFuncs is the allowlist of SQL aggregate functions permitted in a
// fields= projection. The rendered function name is always taken from this
// fixed set (never from user input verbatim), so it cannot be a vector for
// SQL injection.
var aggregateFuncs = map[string]bool{
	"SUM":   true,
	"COUNT": true,
	"AVG":   true,
	"MIN":   true,
	"MAX":   true,
}

// aggregateRegex matches a single aggregate projection element such as
// "SUM(amount)", "COUNT(*)", or "AVG(price) AS avg_price".
// Submatches: 1 = function name, 2 = column or "*", 3 = optional alias.
var aggregateRegex = regexp.MustCompile(
	`(?i)^([a-z]+)\(\s*(\*|[a-z_][a-z0-9_]*)\s*\)(?:\s+as\s+([a-z_][a-z0-9_]*))?$`,
)

// SelectItem is one entry in a SELECT projection. A plain column has an empty
// Func; an aggregate has Func set to an allowlisted function name (uppercased),
// Column set to the argument ("*" only for COUNT), and Alias set to the output
// column name.
type SelectItem struct {
	Func   string
	Column string
	Alias  string
}

// IsAggregate reports whether this item is an aggregate expression.
func (s SelectItem) IsAggregate() bool { return s.Func != "" }

// ParseProjection parses a comma-separated fields list that may mix plain
// columns with aggregate expressions, e.g. "region,SUM(amount) AS total".
// Plain columns and aggregate arguments are validated as SQL identifiers.
// Returns nil for an empty input string (the caller then selects all columns).
func ParseProjection(fields string) ([]SelectItem, error) {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return nil, nil
	}

	parts := strings.Split(fields, ",")
	items := make([]SelectItem, 0, len(parts))

	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}

		if m := aggregateRegex.FindStringSubmatch(part); m != nil {
			fn := strings.ToUpper(m[1])
			if !aggregateFuncs[fn] {
				return nil, fmt.Errorf("unsupported aggregate function %q: allowed functions are AVG, COUNT, MAX, MIN, SUM", m[1])
			}
			arg := m[2]
			if arg == "*" && fn != "COUNT" {
				return nil, fmt.Errorf("%s(*) is not allowed; only COUNT supports the * argument", fn)
			}
			if arg != "*" {
				if err := ValidateIdentifier(arg); err != nil {
					return nil, fmt.Errorf("invalid aggregate column: %w", err)
				}
			}
			alias := m[3]
			if alias == "" {
				alias = defaultAlias(fn, arg)
			}
			if err := ValidateIdentifier(alias); err != nil {
				return nil, fmt.Errorf("invalid aggregate alias: %w", err)
			}
			items = append(items, SelectItem{Func: fn, Column: arg, Alias: alias})
			continue
		}

		// Plain column.
		if err := ValidateIdentifier(part); err != nil {
			return nil, fmt.Errorf("invalid field name: %w", err)
		}
		items = append(items, SelectItem{Column: part})
	}

	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

// defaultAlias derives a deterministic output name for an aggregate that has no
// explicit alias: "count" for COUNT(*), otherwise "<func>_<column>" lowercased,
// e.g. SUM(amount) -> "sum_amount".
func defaultAlias(fn, arg string) string {
	if arg == "*" {
		return strings.ToLower(fn)
	}
	return strings.ToLower(fn) + "_" + arg
}

// HasAggregate reports whether any item in the projection is an aggregate.
func HasAggregate(items []SelectItem) bool {
	for _, it := range items {
		if it.IsAggregate() {
			return true
		}
	}
	return false
}

// ValidateGroupedProjection enforces that a projection mixing aggregates with
// plain columns is well-formed and portable across SQL dialects:
//   - if a GROUP BY is present, every plain (non-aggregated) column must appear
//     in it;
//   - if an aggregate is present without a GROUP BY, no plain columns may be
//     selected alongside it (a bare column next to an aggregate is undefined in
//     standard SQL and silently picks an arbitrary row in SQLite);
//   - a GROUP BY requires an explicit fields list (no implicit SELECT *).
func ValidateGroupedProjection(items []SelectItem, groupBy []string) error {
	hasGroup := len(groupBy) > 0
	hasAgg := HasAggregate(items)
	if !hasGroup && !hasAgg {
		return nil
	}
	if hasGroup && len(items) == 0 {
		return fmt.Errorf("group parameter requires an explicit fields list")
	}

	groupSet := make(map[string]bool, len(groupBy))
	for _, g := range groupBy {
		groupSet[g] = true
	}
	for _, it := range items {
		if it.IsAggregate() {
			continue
		}
		if !groupSet[it.Column] {
			return fmt.Errorf("column %q must be wrapped in an aggregate function or listed in the group parameter", it.Column)
		}
	}
	return nil
}

// BuildProjection renders a projection into a SQL select list, quoting all
// identifiers with quoteFn. Aggregates render as FUNC(col) AS "alias".
func BuildProjection(items []SelectItem, quoteFn func(string) string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		if !it.IsAggregate() {
			parts[i] = quoteFn(it.Column)
			continue
		}
		inner := "*"
		if it.Column != "*" {
			inner = quoteFn(it.Column)
		}
		parts[i] = it.Func + "(" + inner + ") AS " + quoteFn(it.Alias)
	}
	return strings.Join(parts, ", ")
}

// BuildSelectList renders the SELECT list for a query. A structured projection
// takes precedence; otherwise the plain field list is quoted; an empty field
// list yields "*". This preserves the original field-selection behavior while
// adding aggregate support.
func BuildSelectList(projection []SelectItem, fields []string, quoteFn func(string) string) string {
	if len(projection) > 0 {
		return BuildProjection(projection, quoteFn)
	}
	if len(fields) > 0 {
		quoted := make([]string, len(fields))
		for i, f := range fields {
			quoted[i] = quoteFn(f)
		}
		return strings.Join(quoted, ", ")
	}
	return "*"
}

// ParseGroupBy parses a comma-separated list of column names for a GROUP BY
// clause, validating each as a SQL identifier. Returns nil for empty input.
func ParseGroupBy(group string) ([]string, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, nil
	}
	parts := strings.Split(group, ",")
	cols := make([]string, 0, len(parts))
	for _, raw := range parts {
		col := strings.TrimSpace(raw)
		if col == "" {
			continue
		}
		if err := ValidateIdentifier(col); err != nil {
			return nil, fmt.Errorf("invalid group column: %w", err)
		}
		cols = append(cols, col)
	}
	if len(cols) == 0 {
		return nil, nil
	}
	return cols, nil
}

// BuildGroupBy renders a GROUP BY clause (without a leading space), quoting each
// column with quoteFn. Returns "" when there are no columns.
func BuildGroupBy(cols []string, quoteFn func(string) string) string {
	if len(cols) == 0 {
		return ""
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteFn(c)
	}
	return "GROUP BY " + strings.Join(quoted, ", ")
}
