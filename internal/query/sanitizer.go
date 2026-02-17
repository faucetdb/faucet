// Package query provides filter parsing, query building, and SQL injection
// prevention for the Faucet API layer. It converts DreamFactory-compatible
// filter expressions into parameterized SQL WHERE clauses.
package query

import (
	"fmt"
	"regexp"
	"strings"
)

// identifierRegex validates SQL identifiers (column names, table names).
// Must start with a letter or underscore, followed by alphanumeric or underscore.
var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// sqlReservedWords contains SQL keywords that cannot be used as identifiers.
// This is a defense-in-depth measure; parameterization handles most injection,
// but rejecting reserved words as column names prevents query structure attacks.
var sqlReservedWords = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"DROP": true, "CREATE": true, "ALTER": true, "TRUNCATE": true,
	"EXEC": true, "EXECUTE": true, "UNION": true, "INTO": true,
	"FROM": true, "WHERE": true, "TABLE": true, "DATABASE": true,
	"GRANT": true, "REVOKE": true, "INDEX": true, "VIEW": true,
	"PROCEDURE": true, "FUNCTION": true, "TRIGGER": true, "SCHEMA": true,
}

// ValidateIdentifier ensures a SQL identifier (column name, table name) is safe.
// It rejects empty strings, strings over 128 characters, strings that don't
// match the identifier pattern, and SQL reserved words.
func ValidateIdentifier(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("identifier cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("identifier too long (max 128 chars): %q", name)
	}
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("invalid identifier %q: must match [a-zA-Z_][a-zA-Z0-9_]*", name)
	}
	if sqlReservedWords[strings.ToUpper(name)] {
		return fmt.Errorf("identifier %q is a SQL reserved word", name)
	}
	return nil
}

// ValidateIdentifiers validates multiple identifiers, returning the first error found.
func ValidateIdentifiers(names []string) error {
	for _, name := range names {
		if err := ValidateIdentifier(name); err != nil {
			return err
		}
	}
	return nil
}

// SanitizeStringValue removes null bytes and validates string length.
// This is a secondary defense; parameterization is the primary protection.
func SanitizeStringValue(val string, maxLen int) (string, error) {
	if maxLen <= 0 {
		maxLen = 65535
	}
	// Remove null bytes which can cause issues in some databases.
	val = strings.ReplaceAll(val, "\x00", "")
	if len(val) > maxLen {
		return "", fmt.Errorf("string value too long (max %d chars)", maxLen)
	}
	return val, nil
}
