package query

import (
	"strings"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "name", false, ""},
		{"valid underscore prefix", "_id", false, ""},
		{"valid with numbers", "col123", false, ""},
		{"valid mixed", "user_name_2", false, ""},
		{"empty", "", true, "cannot be empty"},
		{"starts with number", "1col", true, "must match"},
		{"contains space", "col name", true, "must match"},
		{"contains dash", "col-name", true, "must match"},
		{"contains semicolon", "col;name", true, "must match"},
		{"SQL injection attempt", "1; DROP TABLE--", true, "must match"},
		{"reserved word SELECT", "SELECT", true, "reserved word"},
		{"reserved word drop", "drop", true, "reserved word"},
		{"reserved word Union", "Union", true, "reserved word"},
		{"too long", strings.Repeat("a", 129), true, "too long"},
		{"max length ok", strings.Repeat("a", 128), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentifier(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.input)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %q: %v", tt.input, err)
				}
			}
		})
	}
}

func TestValidateIdentifiers(t *testing.T) {
	// All valid.
	err := ValidateIdentifiers([]string{"id", "name", "email"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// One invalid.
	err = ValidateIdentifiers([]string{"id", "DROP", "email"})
	if err == nil {
		t.Error("expected error for reserved word, got nil")
	}
}

func TestSanitizeStringValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		maxLen  int
		want    string
		wantErr bool
	}{
		{"normal string", "hello", 0, "hello", false},
		{"with null bytes", "hel\x00lo", 0, "hello", false},
		{"too long", "hello", 3, "", true},
		{"max length ok", "hello", 5, "hello", false},
		{"empty string", "", 0, "", false},
		{"default max len", strings.Repeat("x", 65535), 0, strings.Repeat("x", 65535), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeStringValue(tt.input, tt.maxLen)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}
