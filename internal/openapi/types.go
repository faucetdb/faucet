package openapi

import "strings"

// TypeMapping maps database column types to OpenAPI type/format pairs.
type TypeMapping struct {
	Type   string // OpenAPI type: string, integer, number, boolean, object, array
	Format string // OpenAPI format: int32, int64, float, double, date, date-time, uuid, byte, etc.
}

// dbTypeToOpenAPI maps common database column types to OpenAPI types (case-insensitive lookup).
var dbTypeToOpenAPI = map[string]TypeMapping{
	// Integer types
	"int":       {"integer", "int32"},
	"int2":      {"integer", "int32"},
	"int4":      {"integer", "int32"},
	"int8":      {"integer", "int64"},
	"integer":   {"integer", "int32"},
	"bigint":    {"integer", "int64"},
	"smallint":  {"integer", "int32"},
	"tinyint":   {"integer", "int32"},
	"serial":    {"integer", "int32"},
	"bigserial": {"integer", "int64"},

	// Float types
	"float":            {"number", "float"},
	"float4":           {"number", "float"},
	"float8":           {"number", "double"},
	"double":           {"number", "double"},
	"double precision": {"number", "double"},
	"decimal":          {"number", "double"},
	"numeric":          {"number", "double"},
	"real":             {"number", "float"},
	"money":            {"number", "double"},
	"number":           {"number", "double"},

	// String types
	"varchar":             {"string", ""},
	"char":                {"string", ""},
	"character":           {"string", ""},
	"character varying":   {"string", ""},
	"text":                {"string", ""},
	"nvarchar":            {"string", ""},
	"nchar":               {"string", ""},
	"ntext":               {"string", ""},
	"string":              {"string", ""},
	"xml":                 {"string", ""},
	"citext":              {"string", ""},
	"name":                {"string", ""},
	"enum":                {"string", ""},
	"user-defined":        {"string", ""},
	"character_data":      {"string", ""},
	"information_schema.": {"string", ""},

	// Date/time types
	"date":                          {"string", "date"},
	"datetime":                      {"string", "date-time"},
	"datetime2":                     {"string", "date-time"},
	"datetimeoffset":                {"string", "date-time"},
	"timestamp":                     {"string", "date-time"},
	"timestamptz":                   {"string", "date-time"},
	"timestamp with time zone":      {"string", "date-time"},
	"timestamp without time zone":   {"string", "date-time"},
	"time":                          {"string", "time"},
	"timetz":                        {"string", "time"},
	"time with time zone":           {"string", "time"},
	"time without time zone":        {"string", "time"},
	"interval":                      {"string", ""},
	"smalldatetime":                 {"string", "date-time"},

	// Boolean
	"boolean": {"boolean", ""},
	"bool":    {"boolean", ""},
	"bit":     {"boolean", ""},

	// Binary
	"bytea":     {"string", "byte"},
	"binary":    {"string", "byte"},
	"varbinary": {"string", "byte"},
	"blob":      {"string", "byte"},
	"image":     {"string", "byte"},

	// UUID
	"uuid":             {"string", "uuid"},
	"uniqueidentifier": {"string", "uuid"},

	// JSON
	"json":    {"object", ""},
	"jsonb":   {"object", ""},
	"variant": {"object", ""},
	"object":  {"object", ""},
	"array":   {"array", ""},

	// Network types (PostgreSQL)
	"inet":    {"string", ""},
	"cidr":    {"string", ""},
	"macaddr": {"string", ""},

	// Geometric types (PostgreSQL)
	"point":   {"string", ""},
	"line":    {"string", ""},
	"polygon": {"string", ""},
	"circle":  {"string", ""},
	"box":     {"string", ""},
	"path":    {"string", ""},

	// Range types (PostgreSQL)
	"int4range": {"string", ""},
	"int8range": {"string", ""},
	"numrange":  {"string", ""},
	"tsrange":   {"string", ""},
	"tstzrange": {"string", ""},
	"daterange": {"string", ""},

	// Other
	"oid":     {"integer", "int64"},
	"regproc": {"string", ""},
	"void":    {"string", ""},
}

// MapDBType converts a database column type to an OpenAPI type mapping.
// Falls back to {"string", ""} for unknown types.
func MapDBType(dbType string) TypeMapping {
	// Normalize: lowercase, strip leading/trailing whitespace
	normalized := strings.ToLower(strings.TrimSpace(dbType))

	// Strip anything after opening paren: "varchar(255)" -> "varchar"
	if idx := strings.IndexByte(normalized, '('); idx >= 0 {
		normalized = normalized[:idx]
	}

	// Strip "unsigned" suffix: "int unsigned" -> "int"
	normalized = strings.TrimSuffix(normalized, " unsigned")
	normalized = strings.TrimSpace(normalized)

	// Strip array brackets: "text[]" -> "text"
	normalized = strings.TrimSuffix(normalized, "[]")

	if m, ok := dbTypeToOpenAPI[normalized]; ok {
		return m
	}
	return TypeMapping{"string", ""}
}
