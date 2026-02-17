package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Placeholder functions
// ---------------------------------------------------------------------------

// PlaceholderFunc returns the SQL placeholder for a given 1-based parameter index.
type PlaceholderFunc func(index int) string

// DollarPlaceholder returns $1, $2, etc. (PostgreSQL).
func DollarPlaceholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// QuestionPlaceholder returns ? for all params (MySQL, SQLite).
func QuestionPlaceholder(_ int) string {
	return "?"
}

// AtPPlaceholder returns @p1, @p2, etc. (SQL Server).
func AtPPlaceholder(index int) string {
	return fmt.Sprintf("@p%d", index)
}

// ---------------------------------------------------------------------------
// ParsedFilter
// ---------------------------------------------------------------------------

// ParsedFilter holds a parameterized SQL WHERE fragment and its bind values.
type ParsedFilter struct {
	SQL    string        // e.g. "(age > $1) AND (status = $2)"
	Params []interface{} // e.g. [21, "active"]
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// ParseFilter parses a DreamFactory-compatible filter string into a
// parameterized SQL WHERE clause fragment.
//
// ph controls placeholder style ($1, ?, @p1). startIndex is the 1-based
// index for the first placeholder (useful when appending to an existing
// parameterized query).
//
// Returns nil, nil for an empty filter string.
func ParseFilter(filter string, ph PlaceholderFunc, startIndex int) (*ParsedFilter, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}
	if ph == nil {
		ph = DollarPlaceholder
	}
	if startIndex < 1 {
		startIndex = 1
	}

	tokens, err := tokenize(filter)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}

	p := &parser{
		tokens:    tokens,
		pos:       0,
		ph:        ph,
		nextIndex: startIndex,
	}

	node, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Ensure all tokens were consumed.
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected token %q at position %d", p.tokens[p.pos].value, p.tokens[p.pos].pos)
	}

	return &ParsedFilter{
		SQL:    node.sql,
		Params: node.params,
	}, nil
}

// ---------------------------------------------------------------------------
// Token types
// ---------------------------------------------------------------------------

type tokenType int

const (
	tokIdentifier tokenType = iota
	tokNumber
	tokString
	tokOperator // =, !=, <>, >, >=, <, <=
	tokLParen
	tokRParen
	tokComma
	// Keywords (identifiers promoted to keywords during tokenization).
	tokAND
	tokOR
	tokNOT
	tokIN
	tokLIKE
	tokIS
	tokNULL
	tokBETWEEN
	tokCONTAINS
	tokSTARTS
	tokENDS
	tokWITH
)

type token struct {
	typ   tokenType
	value string // Original text (keywords uppercased).
	pos   int    // Byte offset in the input for error messages.
}

// keywords maps uppercased words to keyword token types.
var keywords = map[string]tokenType{
	"AND":      tokAND,
	"OR":       tokOR,
	"NOT":      tokNOT,
	"IN":       tokIN,
	"LIKE":     tokLIKE,
	"IS":       tokIS,
	"NULL":     tokNULL,
	"BETWEEN":  tokBETWEEN,
	"CONTAINS": tokCONTAINS,
	"STARTS":   tokSTARTS,
	"ENDS":     tokENDS,
	"WITH":     tokWITH,
}

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

func tokenize(input string) ([]token, error) {
	var tokens []token
	i := 0
	n := len(input)

	for i < n {
		// Skip whitespace.
		if unicode.IsSpace(rune(input[i])) {
			i++
			continue
		}

		ch := input[i]

		// Parentheses and comma.
		switch ch {
		case '(':
			tokens = append(tokens, token{typ: tokLParen, value: "(", pos: i})
			i++
			continue
		case ')':
			tokens = append(tokens, token{typ: tokRParen, value: ")", pos: i})
			i++
			continue
		case ',':
			tokens = append(tokens, token{typ: tokComma, value: ",", pos: i})
			i++
			continue
		}

		// Two-character operators.
		if i+1 < n {
			two := input[i : i+2]
			switch two {
			case "!=", "<>", ">=", "<=":
				tokens = append(tokens, token{typ: tokOperator, value: two, pos: i})
				i += 2
				continue
			}
		}

		// Single-character operators.
		switch ch {
		case '=', '>', '<':
			tokens = append(tokens, token{typ: tokOperator, value: string(ch), pos: i})
			i++
			continue
		}

		// Single-quoted string literal.
		if ch == '\'' {
			start := i
			i++ // skip opening quote
			var sb strings.Builder
			closed := false
			for i < n {
				if input[i] == '\'' {
					// Check for escaped quote ('').
					if i+1 < n && input[i+1] == '\'' {
						sb.WriteByte('\'')
						i += 2
						continue
					}
					// End of string.
					i++ // skip closing quote
					closed = true
					break
				}
				sb.WriteByte(input[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated string literal starting at position %d", start)
			}
			tokens = append(tokens, token{typ: tokString, value: sb.String(), pos: start})
			continue
		}

		// Number: digits, optional decimal point, optional leading minus
		// is NOT handled here (minus is not a filter operator in DreamFactory).
		// Negative numbers in filters are expressed via the unary minus on
		// the API side or as string values. We do handle plain integers and
		// decimals here.
		if ch >= '0' && ch <= '9' {
			start := i
			for i < n && input[i] >= '0' && input[i] <= '9' {
				i++
			}
			// Optional decimal part.
			if i < n && input[i] == '.' {
				i++
				if i >= n || input[i] < '0' || input[i] > '9' {
					return nil, fmt.Errorf("invalid number at position %d: trailing decimal point", start)
				}
				for i < n && input[i] >= '0' && input[i] <= '9' {
					i++
				}
			}
			tokens = append(tokens, token{typ: tokNumber, value: input[start:i], pos: start})
			continue
		}

		// Negative numbers: minus followed by digit(s).
		if ch == '-' && i+1 < n && input[i+1] >= '0' && input[i+1] <= '9' {
			start := i
			i++ // skip the minus
			for i < n && input[i] >= '0' && input[i] <= '9' {
				i++
			}
			if i < n && input[i] == '.' {
				i++
				if i >= n || input[i] < '0' || input[i] > '9' {
					return nil, fmt.Errorf("invalid number at position %d: trailing decimal point", start)
				}
				for i < n && input[i] >= '0' && input[i] <= '9' {
					i++
				}
			}
			tokens = append(tokens, token{typ: tokNumber, value: input[start:i], pos: start})
			continue
		}

		// Identifier or keyword.
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			start := i
			for i < n && (input[i] == '_' || (input[i] >= 'a' && input[i] <= 'z') || (input[i] >= 'A' && input[i] <= 'Z') || (input[i] >= '0' && input[i] <= '9') || input[i] == '.') {
				i++
			}
			word := input[start:i]
			upper := strings.ToUpper(word)

			if kt, ok := keywords[upper]; ok {
				tokens = append(tokens, token{typ: kt, value: upper, pos: start})
			} else {
				tokens = append(tokens, token{typ: tokIdentifier, value: word, pos: start})
			}
			continue
		}

		return nil, fmt.Errorf("unexpected character %q at position %d", string(ch), i)
	}

	return tokens, nil
}

// ---------------------------------------------------------------------------
// Parser (recursive descent)
// ---------------------------------------------------------------------------

// parseResult is the internal representation of a parsed SQL fragment.
type parseResult struct {
	sql    string
	params []interface{}
}

type parser struct {
	tokens    []token
	pos       int
	ph        PlaceholderFunc
	nextIndex int // Next placeholder index (1-based).
}

// peek returns the current token without advancing, or nil if at EOF.
func (p *parser) peek() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}

// advance moves to the next token and returns the consumed token.
func (p *parser) advance() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	t := &p.tokens[p.pos]
	p.pos++
	return t
}

// expect consumes the next token, requiring it to match the given type.
func (p *parser) expect(typ tokenType) (*token, error) {
	t := p.advance()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of filter, expected %v", tokenTypeName(typ))
	}
	if t.typ != typ {
		return nil, fmt.Errorf("expected %v but got %q at position %d", tokenTypeName(typ), t.value, t.pos)
	}
	return t, nil
}

// addParam registers a bind parameter and returns its placeholder string.
func (p *parser) addParam(val interface{}) string {
	placeholder := p.ph(p.nextIndex)
	p.nextIndex++
	return placeholder
}

// ---------------------------------------------------------------------------
// Grammar rules
// ---------------------------------------------------------------------------

// parseExpression is the entry point: expression → or_expr
func (p *parser) parseExpression() (*parseResult, error) {
	return p.parseOr()
}

// parseOr: or_expr → and_expr ( "OR" and_expr )*
func (p *parser) parseOr() (*parseResult, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.typ != tokOR {
			break
		}
		p.advance() // consume OR

		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}

		left = &parseResult{
			sql:    left.sql + " OR " + right.sql,
			params: append(left.params, right.params...),
		}
	}
	return left, nil
}

// parseAnd: and_expr → not_expr ( "AND" not_expr )*
func (p *parser) parseAnd() (*parseResult, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()
		if t == nil || t.typ != tokAND {
			break
		}
		p.advance() // consume AND

		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}

		left = &parseResult{
			sql:    left.sql + " AND " + right.sql,
			params: append(left.params, right.params...),
		}
	}
	return left, nil
}

// parseNot: not_expr → "NOT" not_expr | primary_expr
func (p *parser) parseNot() (*parseResult, error) {
	t := p.peek()
	if t != nil && t.typ == tokNOT {
		// Check that NOT is not followed by IN/LIKE/BETWEEN/NULL (those are
		// handled in parseComparison as compound operators). We peek ahead
		// to see if NOT is being used as a boolean prefix or as part of a
		// compound operator. If the token *before* NOT was an identifier,
		// then NOT belongs to a comparison (NOT IN, NOT LIKE, etc.).
		// Since we only get here when NOT appears at expression level (not
		// after an identifier), it's always boolean NOT.
		p.advance() // consume NOT

		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &parseResult{
			sql:    "NOT " + inner.sql,
			params: inner.params,
		}, nil
	}
	return p.parsePrimary()
}

// parsePrimary: primary_expr → "(" expression ")" | comparison
func (p *parser) parsePrimary() (*parseResult, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of filter expression")
	}

	if t.typ == tokLParen {
		p.advance() // consume (
		inner, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen); err != nil {
			return nil, err
		}
		return &parseResult{
			sql:    "(" + inner.sql + ")",
			params: inner.params,
		}, nil
	}

	return p.parseComparison()
}

// parseComparison handles all comparison forms:
//
//	column op value
//	column [NOT] IN (value_list)
//	column [NOT] LIKE value
//	column [NOT] BETWEEN value AND value
//	column IS [NOT] NULL
//	column CONTAINS value
//	column STARTS WITH value
//	column ENDS WITH value
func (p *parser) parseComparison() (*parseResult, error) {
	// Expect an identifier (column name).
	colTok, err := p.expect(tokIdentifier)
	if err != nil {
		return nil, fmt.Errorf("expected column name: %w", err)
	}

	// Validate the column name. Supports qualified names like "table.column".
	if err := validateColumnRef(colTok.value); err != nil {
		return nil, fmt.Errorf("invalid column name: %w", err)
	}

	col := colTok.value

	// Look at the next token to determine the operator.
	opTok := p.peek()
	if opTok == nil {
		return nil, fmt.Errorf("unexpected end of filter after column %q", col)
	}

	switch opTok.typ {

	// Simple comparison operators: =, !=, <>, >, >=, <, <=
	case tokOperator:
		p.advance()
		val, err := p.parseValue()
		if err != nil {
			return nil, fmt.Errorf("expected value after %s %s: %w", col, opTok.value, err)
		}
		ph := p.addParam(val.value)
		return &parseResult{
			sql:    col + " " + opTok.value + " " + ph,
			params: []interface{}{val.value},
		}, nil

	// IS NULL / IS NOT NULL
	case tokIS:
		p.advance() // consume IS
		next := p.peek()
		if next == nil {
			return nil, fmt.Errorf("unexpected end of filter after %s IS", col)
		}
		if next.typ == tokNOT {
			p.advance() // consume NOT
			if _, err := p.expect(tokNULL); err != nil {
				return nil, fmt.Errorf("expected NULL after %s IS NOT: %w", col, err)
			}
			return &parseResult{
				sql:    col + " IS NOT NULL",
				params: nil,
			}, nil
		}
		if next.typ == tokNULL {
			p.advance() // consume NULL
			return &parseResult{
				sql:    col + " IS NULL",
				params: nil,
			}, nil
		}
		return nil, fmt.Errorf("expected NULL or NOT NULL after %s IS, got %q", col, next.value)

	// NOT IN / NOT LIKE / NOT BETWEEN
	case tokNOT:
		p.advance() // consume NOT
		next := p.peek()
		if next == nil {
			return nil, fmt.Errorf("unexpected end of filter after %s NOT", col)
		}
		switch next.typ {
		case tokIN:
			return p.parseInList(col, "NOT IN")
		case tokLIKE:
			return p.parseLike(col, "NOT LIKE")
		case tokBETWEEN:
			return p.parseBetween(col, "NOT BETWEEN")
		default:
			return nil, fmt.Errorf("expected IN, LIKE, or BETWEEN after %s NOT, got %q", col, next.value)
		}

	// IN (value_list)
	case tokIN:
		return p.parseInList(col, "IN")

	// LIKE value
	case tokLIKE:
		return p.parseLike(col, "LIKE")

	// BETWEEN value AND value
	case tokBETWEEN:
		return p.parseBetween(col, "BETWEEN")

	// CONTAINS value → LIKE '%val%'
	case tokCONTAINS:
		p.advance() // consume CONTAINS
		val, err := p.parseValue()
		if err != nil {
			return nil, fmt.Errorf("expected value after %s CONTAINS: %w", col, err)
		}
		s, ok := val.value.(string)
		if !ok {
			return nil, fmt.Errorf("CONTAINS requires a string value, got %T", val.value)
		}
		ph := p.addParam("%" + s + "%")
		return &parseResult{
			sql:    col + " LIKE " + ph,
			params: []interface{}{"%" + s + "%"},
		}, nil

	// STARTS WITH value → LIKE 'val%'
	case tokSTARTS:
		p.advance() // consume STARTS
		if _, err := p.expect(tokWITH); err != nil {
			return nil, fmt.Errorf("expected WITH after %s STARTS: %w", col, err)
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, fmt.Errorf("expected value after %s STARTS WITH: %w", col, err)
		}
		s, ok := val.value.(string)
		if !ok {
			return nil, fmt.Errorf("STARTS WITH requires a string value, got %T", val.value)
		}
		ph := p.addParam(s + "%")
		return &parseResult{
			sql:    col + " LIKE " + ph,
			params: []interface{}{s + "%"},
		}, nil

	// ENDS WITH value → LIKE '%val'
	case tokENDS:
		p.advance() // consume ENDS
		if _, err := p.expect(tokWITH); err != nil {
			return nil, fmt.Errorf("expected WITH after %s ENDS: %w", col, err)
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, fmt.Errorf("expected value after %s ENDS WITH: %w", col, err)
		}
		s, ok := val.value.(string)
		if !ok {
			return nil, fmt.Errorf("ENDS WITH requires a string value, got %T", val.value)
		}
		ph := p.addParam("%" + s)
		return &parseResult{
			sql:    col + " LIKE " + ph,
			params: []interface{}{"%" + s},
		}, nil

	default:
		return nil, fmt.Errorf("unexpected token %q after column %q at position %d", opTok.value, col, opTok.pos)
	}
}

// parseInList: [NOT] IN (value, value, ...)
// The caller already matched the column; this consumes "IN (" through ")".
func (p *parser) parseInList(col, op string) (*parseResult, error) {
	p.advance() // consume IN

	if _, err := p.expect(tokLParen); err != nil {
		return nil, fmt.Errorf("expected '(' after %s %s: %w", col, op, err)
	}

	var placeholders []string
	var params []interface{}

	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, fmt.Errorf("expected value in %s %s list: %w", col, op, err)
		}
		ph := p.addParam(val.value)
		placeholders = append(placeholders, ph)
		params = append(params, val.value)

		// Check for comma or closing paren.
		next := p.peek()
		if next == nil {
			return nil, fmt.Errorf("unexpected end of filter in %s %s list", col, op)
		}
		if next.typ == tokComma {
			p.advance() // consume comma
			continue
		}
		if next.typ == tokRParen {
			p.advance() // consume )
			break
		}
		return nil, fmt.Errorf("expected ',' or ')' in %s %s list, got %q", col, op, next.value)
	}

	if len(placeholders) == 0 {
		return nil, fmt.Errorf("%s %s requires at least one value", col, op)
	}

	return &parseResult{
		sql:    col + " " + op + " (" + strings.Join(placeholders, ", ") + ")",
		params: params,
	}, nil
}

// parseLike: [NOT] LIKE value
// The caller already matched the column; this consumes "LIKE value".
func (p *parser) parseLike(col, op string) (*parseResult, error) {
	p.advance() // consume LIKE

	val, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("expected value after %s %s: %w", col, op, err)
	}

	ph := p.addParam(val.value)
	return &parseResult{
		sql:    col + " " + op + " " + ph,
		params: []interface{}{val.value},
	}, nil
}

// parseBetween: [NOT] BETWEEN value AND value
// The caller already matched the column; this consumes "BETWEEN val AND val".
func (p *parser) parseBetween(col, op string) (*parseResult, error) {
	p.advance() // consume BETWEEN

	low, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("expected lower bound after %s %s: %w", col, op, err)
	}

	// The AND here is part of BETWEEN syntax, NOT a boolean operator.
	if _, err := p.expect(tokAND); err != nil {
		return nil, fmt.Errorf("expected AND in %s %s: %w", col, op, err)
	}

	high, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("expected upper bound in %s %s: %w", col, op, err)
	}

	phLow := p.addParam(low.value)
	phHigh := p.addParam(high.value)
	return &parseResult{
		sql:    col + " " + op + " " + phLow + " AND " + phHigh,
		params: []interface{}{low.value, high.value},
	}, nil
}

// ---------------------------------------------------------------------------
// Value parsing
// ---------------------------------------------------------------------------

// parsedValue wraps a typed Go value extracted from a token.
type parsedValue struct {
	value interface{} // string or numeric type
}

// parseValue consumes and returns the next value token (string or number).
func (p *parser) parseValue() (*parsedValue, error) {
	t := p.advance()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of filter, expected a value")
	}

	switch t.typ {
	case tokString:
		return &parsedValue{value: t.value}, nil
	case tokNumber:
		return p.parseNumericValue(t.value)
	default:
		return nil, fmt.Errorf("expected a value (string or number), got %q at position %d", t.value, t.pos)
	}
}

// parseNumericValue converts a numeric string token to int64 or float64.
func (p *parser) parseNumericValue(s string) (*parsedValue, error) {
	// Try integer first.
	if !strings.Contains(s, ".") {
		n, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return &parsedValue{value: n}, nil
		}
	}
	// Fall back to float.
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number %q: %w", s, err)
	}
	return &parsedValue{value: f}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// tokenTypeName returns a human-readable name for a token type (for errors).
func tokenTypeName(t tokenType) string {
	switch t {
	case tokIdentifier:
		return "identifier"
	case tokNumber:
		return "number"
	case tokString:
		return "string"
	case tokOperator:
		return "operator"
	case tokLParen:
		return "'('"
	case tokRParen:
		return "')'"
	case tokComma:
		return "','"
	case tokAND:
		return "AND"
	case tokOR:
		return "OR"
	case tokNOT:
		return "NOT"
	case tokIN:
		return "IN"
	case tokLIKE:
		return "LIKE"
	case tokIS:
		return "IS"
	case tokNULL:
		return "NULL"
	case tokBETWEEN:
		return "BETWEEN"
	case tokCONTAINS:
		return "CONTAINS"
	case tokSTARTS:
		return "STARTS"
	case tokENDS:
		return "ENDS"
	case tokWITH:
		return "WITH"
	default:
		return fmt.Sprintf("token(%d)", t)
	}
}

// validateColumnRef validates a column reference, which may be a simple name
// like "age" or a qualified name like "users.age" (table.column).
// Each component must pass ValidateIdentifier validation independently.
func validateColumnRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("column reference cannot be empty")
	}
	parts := strings.Split(ref, ".")
	if len(parts) > 3 {
		return fmt.Errorf("column reference %q has too many parts (max: schema.table.column)", ref)
	}
	for _, part := range parts {
		if err := ValidateIdentifier(part); err != nil {
			return fmt.Errorf("in column reference %q: %w", ref, err)
		}
	}
	return nil
}
