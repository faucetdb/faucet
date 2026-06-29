package query

import (
	"reflect"
	"testing"
)

// dqQuote is a test double for a dialect quote function (double-quote style).
func dqQuote(s string) string { return `"` + s + `"` }

func TestParseProjection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []SelectItem
		wantErr bool
	}{
		{
			name:  "empty returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "plain columns",
			input: "id,name,email",
			want: []SelectItem{
				{Column: "id"},
				{Column: "name"},
				{Column: "email"},
			},
		},
		{
			name:  "sum aggregate with default alias",
			input: "SUM(amount)",
			want:  []SelectItem{{Func: "SUM", Column: "amount", Alias: "sum_amount"}},
		},
		{
			name:  "count star with default alias",
			input: "COUNT(*)",
			want:  []SelectItem{{Func: "COUNT", Column: "*", Alias: "count"}},
		},
		{
			name:  "explicit alias",
			input: "AVG(price) AS avg_price",
			want:  []SelectItem{{Func: "AVG", Column: "price", Alias: "avg_price"}},
		},
		{
			name:  "lowercase function name is normalized",
			input: "min(score)",
			want:  []SelectItem{{Func: "MIN", Column: "score", Alias: "min_score"}},
		},
		{
			name:  "mixed plain and aggregate preserves order",
			input: "region, SUM(amount) AS total, COUNT(*)",
			want: []SelectItem{
				{Column: "region"},
				{Func: "SUM", Column: "amount", Alias: "total"},
				{Func: "COUNT", Column: "*", Alias: "count"},
			},
		},
		{
			name:  "whitespace inside parens is tolerated",
			input: "MAX(  amount  )",
			want:  []SelectItem{{Func: "MAX", Column: "amount", Alias: "max_amount"}},
		},
		{
			name:    "unknown aggregate function rejected",
			input:   "TOTAL(amount)",
			wantErr: true,
		},
		{
			name:    "star only allowed for COUNT",
			input:   "SUM(*)",
			wantErr: true,
		},
		{
			name:    "reserved word as alias rejected",
			input:   "SUM(amount) AS select",
			wantErr: true,
		},
		{
			name:    "injection in aggregate argument rejected",
			input:   "SUM(1); DROP TABLE users--)",
			wantErr: true,
		},
		{
			name:    "injection masquerading as column rejected",
			input:   "id; DROP TABLE users",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProjection(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProjection(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseProjection(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateGroupedProjection(t *testing.T) {
	tests := []struct {
		name    string
		items   []SelectItem
		groupBy []string
		wantErr bool
	}{
		{
			name:  "no aggregate no group is fine",
			items: []SelectItem{{Column: "id"}, {Column: "name"}},
		},
		{
			name:  "whole-table aggregate without group is fine",
			items: []SelectItem{{Func: "SUM", Column: "amount", Alias: "sum_amount"}},
		},
		{
			name:    "aggregate beside bare column without group is rejected",
			items:   []SelectItem{{Column: "region"}, {Func: "SUM", Column: "amount", Alias: "sum_amount"}},
			wantErr: true,
		},
		{
			name:    "grouped query with the bare column listed is fine",
			items:   []SelectItem{{Column: "region"}, {Func: "SUM", Column: "amount", Alias: "sum_amount"}},
			groupBy: []string{"region"},
		},
		{
			name:    "bare column missing from group is rejected",
			items:   []SelectItem{{Column: "region"}, {Column: "city"}, {Func: "COUNT", Column: "*", Alias: "count"}},
			groupBy: []string{"region"},
			wantErr: true,
		},
		{
			name:    "group without any fields is rejected",
			items:   nil,
			groupBy: []string{"region"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupedProjection(tt.items, tt.groupBy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateGroupedProjection() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildProjection(t *testing.T) {
	items := []SelectItem{
		{Column: "region"},
		{Func: "SUM", Column: "amount", Alias: "sum_amount"},
		{Func: "COUNT", Column: "*", Alias: "count"},
	}
	got := BuildProjection(items, dqQuote)
	want := `"region", SUM("amount") AS "sum_amount", COUNT(*) AS "count"`
	if got != want {
		t.Errorf("BuildProjection() = %q, want %q", got, want)
	}
}

func TestBuildSelectList(t *testing.T) {
	tests := []struct {
		name       string
		projection []SelectItem
		fields     []string
		want       string
	}{
		{
			name: "empty yields star",
			want: "*",
		},
		{
			name:   "plain fields fallback preserves legacy behavior",
			fields: []string{"id", "name"},
			want:   `"id", "name"`,
		},
		{
			name:       "projection takes precedence over fields",
			projection: []SelectItem{{Func: "SUM", Column: "amount", Alias: "sum_amount"}},
			fields:     []string{"ignored"},
			want:       `SUM("amount") AS "sum_amount"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildSelectList(tt.projection, tt.fields, dqQuote); got != tt.want {
				t.Errorf("BuildSelectList() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseGroupBy(t *testing.T) {
	got, err := ParseGroupBy("region, status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"region", "status"}) {
		t.Errorf("ParseGroupBy() = %#v", got)
	}

	if _, err := ParseGroupBy(""); err != nil {
		t.Errorf("empty group should not error, got %v", err)
	}
	if _, err := ParseGroupBy("region; DROP TABLE x"); err == nil {
		t.Errorf("expected error for injection attempt in group")
	}
}

func TestBuildGroupBy(t *testing.T) {
	if got := BuildGroupBy(nil, dqQuote); got != "" {
		t.Errorf("BuildGroupBy(nil) = %q, want empty", got)
	}
	got := BuildGroupBy([]string{"region", "created_at"}, dqQuote)
	want := `GROUP BY "region", "created_at"`
	if got != want {
		t.Errorf("BuildGroupBy() = %q, want %q", got, want)
	}
}
