package mcp

import (
	"testing"
)

func TestClamp(t *testing.T) {
	tests := []struct {
		name     string
		val      int
		min      int
		max      int
		expected int
	}{
		{"value in range", 5, 1, 10, 5},
		{"value below min", -3, 1, 10, 1},
		{"value above max", 15, 1, 10, 10},
		{"value equals min", 1, 1, 10, 1},
		{"value equals max", 10, 1, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp(tt.val, tt.min, tt.max)
			if got != tt.expected {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.val, tt.min, tt.max, got, tt.expected)
			}
		})
	}
}

func TestCleanMapValues(t *testing.T) {
	m := map[string]interface{}{
		"bytes_val":  []byte("hello"),
		"string_val": "world",
		"int_val":    42,
		"nil_val":    nil,
		"bool_val":   true,
	}

	cleanMapValues(m)

	// []byte should be converted to string
	if s, ok := m["bytes_val"].(string); !ok {
		t.Errorf("bytes_val should be string after cleaning, got %T", m["bytes_val"])
	} else if s != "hello" {
		t.Errorf("bytes_val = %q, want %q", s, "hello")
	}

	// string should remain unchanged
	if s, ok := m["string_val"].(string); !ok {
		t.Errorf("string_val should remain string, got %T", m["string_val"])
	} else if s != "world" {
		t.Errorf("string_val = %q, want %q", s, "world")
	}

	// int should remain unchanged
	if v, ok := m["int_val"].(int); !ok {
		t.Errorf("int_val should remain int, got %T", m["int_val"])
	} else if v != 42 {
		t.Errorf("int_val = %d, want 42", v)
	}

	// nil should remain nil
	if m["nil_val"] != nil {
		t.Errorf("nil_val should remain nil, got %v", m["nil_val"])
	}

	// bool should remain unchanged
	if v, ok := m["bool_val"].(bool); !ok {
		t.Errorf("bool_val should remain bool, got %T", m["bool_val"])
	} else if v != true {
		t.Errorf("bool_val = %v, want true", v)
	}
}

func TestBoolPtr(t *testing.T) {
	truePtr := boolPtr(true)
	if truePtr == nil {
		t.Fatal("boolPtr(true) returned nil")
	}
	if *truePtr != true {
		t.Errorf("*boolPtr(true) = %v, want true", *truePtr)
	}

	falsePtr := boolPtr(false)
	if falsePtr == nil {
		t.Fatal("boolPtr(false) returned nil")
	}
	if *falsePtr != false {
		t.Errorf("*boolPtr(false) = %v, want false", *falsePtr)
	}

	// Verify they are distinct pointers
	if truePtr == falsePtr {
		t.Error("boolPtr(true) and boolPtr(false) should return distinct pointers")
	}
}

func TestReadOnlyAnnotation(t *testing.T) {
	ann := readOnlyAnnotation()

	if ann.ReadOnlyHint == nil {
		t.Fatal("ReadOnlyHint should not be nil for readOnlyAnnotation")
	}
	if *ann.ReadOnlyHint != true {
		t.Errorf("ReadOnlyHint = %v, want true", *ann.ReadOnlyHint)
	}
}

func TestMutatingAnnotation(t *testing.T) {
	ann := mutatingAnnotation()

	if ann.ReadOnlyHint == nil {
		t.Fatal("ReadOnlyHint should not be nil for mutatingAnnotation")
	}
	if *ann.ReadOnlyHint != false {
		t.Errorf("ReadOnlyHint = %v, want false", *ann.ReadOnlyHint)
	}
}
