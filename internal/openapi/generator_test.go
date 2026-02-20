package openapi

import (
	"testing"

	"github.com/faucetdb/faucet/internal/model"
)

// ─── MapDBType Tests ────────────────────────────────────────────────────────

func TestMapDBType_KnownTypes(t *testing.T) {
	tests := []struct {
		dbType     string
		wantType   string
		wantFormat string
	}{
		// Integer types
		{"int", "integer", "int32"},
		{"integer", "integer", "int32"},
		{"bigint", "integer", "int64"},
		{"smallint", "integer", "int32"},
		{"tinyint", "integer", "int32"},
		{"serial", "integer", "int32"},
		{"bigserial", "integer", "int64"},
		{"int2", "integer", "int32"},
		{"int4", "integer", "int32"},
		{"int8", "integer", "int64"},

		// Float types
		{"float", "number", "float"},
		{"double", "number", "double"},
		{"double precision", "number", "double"},
		{"decimal", "number", "double"},
		{"numeric", "number", "double"},
		{"real", "number", "float"},
		{"money", "number", "double"},
		{"number", "number", "double"},
		{"float4", "number", "float"},
		{"float8", "number", "double"},

		// String types
		{"varchar", "string", ""},
		{"char", "string", ""},
		{"text", "string", ""},
		{"nvarchar", "string", ""},
		{"ntext", "string", ""},
		{"xml", "string", ""},
		{"character varying", "string", ""},
		{"character", "string", ""},
		{"citext", "string", ""},

		// Date/time types
		{"date", "string", "date"},
		{"datetime", "string", "date-time"},
		{"timestamp", "string", "date-time"},
		{"timestamptz", "string", "date-time"},
		{"timestamp with time zone", "string", "date-time"},
		{"timestamp without time zone", "string", "date-time"},
		{"time", "string", "time"},
		{"timetz", "string", "time"},
		{"interval", "string", ""},

		// Boolean
		{"boolean", "boolean", ""},
		{"bool", "boolean", ""},
		{"bit", "boolean", ""},

		// Binary
		{"bytea", "string", "byte"},
		{"binary", "string", "byte"},
		{"varbinary", "string", "byte"},
		{"blob", "string", "byte"},

		// UUID
		{"uuid", "string", "uuid"},
		{"uniqueidentifier", "string", "uuid"},

		// JSON
		{"json", "object", ""},
		{"jsonb", "object", ""},
		{"variant", "object", ""},

		// Array
		{"array", "array", ""},

		// Network
		{"inet", "string", ""},
		{"cidr", "string", ""},
		{"macaddr", "string", ""},

		// Other
		{"oid", "integer", "int64"},
	}

	for _, tt := range tests {
		t.Run(tt.dbType, func(t *testing.T) {
			got := MapDBType(tt.dbType)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.dbType, got.Type, tt.wantType)
			}
			if got.Format != tt.wantFormat {
				t.Errorf("MapDBType(%q).Format = %q, want %q", tt.dbType, got.Format, tt.wantFormat)
			}
		})
	}
}

func TestMapDBType_UnknownFallsBackToString(t *testing.T) {
	unknowns := []string{
		"geography",
		"customtype",
		"somethingweird",
		"",
	}
	for _, dbType := range unknowns {
		t.Run(dbType, func(t *testing.T) {
			got := MapDBType(dbType)
			if got.Type != "string" {
				t.Errorf("MapDBType(%q).Type = %q, want %q", dbType, got.Type, "string")
			}
			if got.Format != "" {
				t.Errorf("MapDBType(%q).Format = %q, want %q", dbType, got.Format, "")
			}
		})
	}
}

func TestMapDBType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
	}{
		{"VARCHAR", "string"},
		{"Varchar", "string"},
		{"vARCHAR", "string"},
		{"BOOLEAN", "boolean"},
		{"Boolean", "boolean"},
		{"INTEGER", "integer"},
		{"BIGINT", "integer"},
		{"UUID", "string"},
		{"Json", "object"},
		{"TIMESTAMP", "string"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapDBType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.input, got.Type, tt.wantType)
			}
		})
	}
}

func TestMapDBType_StripParentheses(t *testing.T) {
	tests := []struct {
		input      string
		wantType   string
		wantFormat string
	}{
		{"varchar(255)", "string", ""},
		{"char(10)", "string", ""},
		{"decimal(10,2)", "number", "double"},
		{"numeric(18,4)", "number", "double"},
		{"int(11)", "integer", "int32"},
		{"nvarchar(MAX)", "string", ""},
		{"float(53)", "number", "float"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapDBType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.input, got.Type, tt.wantType)
			}
			if got.Format != tt.wantFormat {
				t.Errorf("MapDBType(%q).Format = %q, want %q", tt.input, got.Format, tt.wantFormat)
			}
		})
	}
}

func TestMapDBType_StripUnsigned(t *testing.T) {
	tests := []struct {
		input      string
		wantType   string
		wantFormat string
	}{
		{"int unsigned", "integer", "int32"},
		{"bigint unsigned", "integer", "int64"},
		{"smallint unsigned", "integer", "int32"},
		{"tinyint unsigned", "integer", "int32"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapDBType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.input, got.Type, tt.wantType)
			}
			if got.Format != tt.wantFormat {
				t.Errorf("MapDBType(%q).Format = %q, want %q", tt.input, got.Format, tt.wantFormat)
			}
		})
	}
}

func TestMapDBType_StripArrayBrackets(t *testing.T) {
	tests := []struct {
		input      string
		wantType   string
		wantFormat string
	}{
		{"text[]", "string", ""},
		{"integer[]", "integer", "int32"},
		{"boolean[]", "boolean", ""},
		{"uuid[]", "string", "uuid"},
		{"varchar[]", "string", ""},
		{"jsonb[]", "object", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapDBType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.input, got.Type, tt.wantType)
			}
			if got.Format != tt.wantFormat {
				t.Errorf("MapDBType(%q).Format = %q, want %q", tt.input, got.Format, tt.wantFormat)
			}
		})
	}
}

func TestMapDBType_TrimWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
	}{
		{"  varchar  ", "string"},
		{"\tint\t", "integer"},
		{" boolean ", "boolean"},
		{"  uuid  ", "string"},
		{" text ", "string"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapDBType(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("MapDBType(%q).Type = %q, want %q", tt.input, got.Type, tt.wantType)
			}
		})
	}
}

// ─── GenerateServiceSpec Tests ──────────────────────────────────────────────

func testSchema() *model.Schema {
	defaultVal := "now()"
	maxLen := int64(255)
	return &model.Schema{
		Tables: []model.TableSchema{
			{
				Name: "users",
				Type: "table",
				Columns: []model.Column{
					{Name: "id", Position: 1, Type: "integer", Nullable: false, IsPrimaryKey: true, IsAutoIncrement: true},
					{Name: "name", Position: 2, Type: "varchar", Nullable: false, MaxLength: &maxLen},
					{Name: "email", Position: 3, Type: "varchar", Nullable: false, MaxLength: &maxLen},
					{Name: "bio", Position: 4, Type: "text", Nullable: true},
					{Name: "created_at", Position: 5, Type: "timestamp", Nullable: false, Default: &defaultVal},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Views: []model.TableSchema{
			{
				Name: "active_users",
				Type: "view",
				Columns: []model.Column{
					{Name: "id", Position: 1, Type: "integer", Nullable: false},
					{Name: "name", Position: 2, Type: "varchar", Nullable: false},
				},
			},
		},
		Procedures: []model.StoredProcedure{
			{
				Name: "get_user_stats",
				Type: "procedure",
				Parameters: []model.ProcedureParam{
					{Name: "user_id", Type: "integer", Direction: "in"},
					{Name: "result", Type: "json", Direction: "out"},
				},
			},
		},
		Functions: []model.StoredProcedure{
			{
				Name:       "calculate_total",
				Type:       "function",
				ReturnType: "numeric",
				Parameters: []model.ProcedureParam{
					{Name: "order_id", Type: "integer", Direction: "in"},
				},
			},
		},
	}
}

func TestGenerateServiceSpec_ValidOpenAPI(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("OpenAPI version = %q, want %q", doc.OpenAPI, "3.1.0")
	}
	if doc.Info == nil {
		t.Fatal("Info is nil")
	}
	if doc.Info.Title != "My Database API" {
		t.Errorf("Info.Title = %q, want %q", doc.Info.Title, "My Database API")
	}
	if doc.Info.Version != "1.0.0" {
		t.Errorf("Info.Version = %q, want %q", doc.Info.Version, "1.0.0")
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "http://localhost:8080" {
		t.Errorf("Servers not set correctly")
	}
}

func TestGenerateServiceSpec_SecuritySchemes(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	if doc.Components == nil {
		t.Fatal("Components is nil")
	}

	apiKey, ok := doc.Components.SecuritySchemes["apiKey"]
	if !ok {
		t.Fatal("apiKey security scheme not found")
	}
	if apiKey.Value.Type != "apiKey" {
		t.Errorf("apiKey.Type = %q, want %q", apiKey.Value.Type, "apiKey")
	}
	if apiKey.Value.In != "header" {
		t.Errorf("apiKey.In = %q, want %q", apiKey.Value.In, "header")
	}
	if apiKey.Value.Name != "X-API-Key" {
		t.Errorf("apiKey.Name = %q, want %q", apiKey.Value.Name, "X-API-Key")
	}

	bearer, ok := doc.Components.SecuritySchemes["bearerAuth"]
	if !ok {
		t.Fatal("bearerAuth security scheme not found")
	}
	if bearer.Value.Type != "http" {
		t.Errorf("bearerAuth.Type = %q, want %q", bearer.Value.Type, "http")
	}
	if bearer.Value.Scheme != "bearer" {
		t.Errorf("bearerAuth.Scheme = %q, want %q", bearer.Value.Scheme, "bearer")
	}
	if bearer.Value.BearerFormat != "JWT" {
		t.Errorf("bearerAuth.BearerFormat = %q, want %q", bearer.Value.BearerFormat, "JWT")
	}

	// Verify global security requirements
	if len(doc.Security) != 2 {
		t.Errorf("Security requirements count = %d, want 2", len(doc.Security))
	}
}

func TestGenerateServiceSpec_TablePaths(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	tablePath := doc.Paths.Find("/api/v1/mydb/_table/users")
	if tablePath == nil {
		t.Fatal("Table path /api/v1/mydb/_table/users not found")
	}

	// Tables should have all CRUD operations
	if tablePath.Get == nil {
		t.Error("GET operation missing for table")
	}
	if tablePath.Post == nil {
		t.Error("POST operation missing for table")
	}
	if tablePath.Put == nil {
		t.Error("PUT operation missing for table")
	}
	if tablePath.Patch == nil {
		t.Error("PATCH operation missing for table")
	}
	if tablePath.Delete == nil {
		t.Error("DELETE operation missing for table")
	}

	// Check schema path exists too
	schemaPath := doc.Paths.Find("/api/v1/mydb/_schema/users")
	if schemaPath == nil {
		t.Error("Schema path /api/v1/mydb/_schema/users not found")
	}
}

func TestGenerateServiceSpec_ViewPaths(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	viewPath := doc.Paths.Find("/api/v1/mydb/_table/active_users")
	if viewPath == nil {
		t.Fatal("View path /api/v1/mydb/_table/active_users not found")
	}

	// Views should only have GET
	if viewPath.Get == nil {
		t.Error("GET operation missing for view")
	}
	if viewPath.Post != nil {
		t.Error("POST operation should not exist for view")
	}
	if viewPath.Put != nil {
		t.Error("PUT operation should not exist for view")
	}
	if viewPath.Patch != nil {
		t.Error("PATCH operation should not exist for view")
	}
	if viewPath.Delete != nil {
		t.Error("DELETE operation should not exist for view")
	}
}

func TestGenerateServiceSpec_ProcedurePaths(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	// Procedure path
	procPath := doc.Paths.Find("/api/v1/mydb/_proc/get_user_stats")
	if procPath == nil {
		t.Fatal("Procedure path /api/v1/mydb/_proc/get_user_stats not found")
	}
	if procPath.Post == nil {
		t.Error("POST operation missing for procedure")
	}

	// Function path
	funcPath := doc.Paths.Find("/api/v1/mydb/_func/calculate_total")
	if funcPath == nil {
		t.Fatal("Function path /api/v1/mydb/_func/calculate_total not found")
	}
	if funcPath.Post == nil {
		t.Error("POST operation missing for function")
	}
}

func TestGenerateServiceSpec_ErrorResponseSchema(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	errSchema, ok := doc.Components.Schemas["ErrorResponse"]
	if !ok {
		t.Fatal("ErrorResponse schema not found in components")
	}
	if errSchema.Value == nil {
		t.Fatal("ErrorResponse schema value is nil")
	}

	errorProp, ok := errSchema.Value.Properties["error"]
	if !ok {
		t.Fatal("error property not found in ErrorResponse schema")
	}

	// Check error sub-properties
	codeProp, ok := errorProp.Value.Properties["code"]
	if !ok {
		t.Error("code property not found in error object")
	} else if codeProp.Value.Type.Slice()[0] != "integer" {
		t.Errorf("code type = %v, want integer", codeProp.Value.Type)
	}

	messageProp, ok := errorProp.Value.Properties["message"]
	if !ok {
		t.Error("message property not found in error object")
	} else if messageProp.Value.Type.Slice()[0] != "string" {
		t.Errorf("message type = %v, want string", messageProp.Value.Type)
	}

	contextProp, ok := errorProp.Value.Properties["context"]
	if !ok {
		t.Error("context property not found in error object")
	} else if contextProp.Value.Type.Slice()[0] != "object" {
		t.Errorf("context type = %v, want object", contextProp.Value.Type)
	}
}

func TestGenerateServiceSpec_ComponentSchemas(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	// Table should have base, Create, and Update schema variants
	schemaName := "Mydb_Users"
	if _, ok := doc.Components.Schemas[schemaName]; !ok {
		t.Errorf("Schema %q not found in components", schemaName)
	}
	if _, ok := doc.Components.Schemas[schemaName+"Create"]; !ok {
		t.Errorf("Schema %q not found in components", schemaName+"Create")
	}
	if _, ok := doc.Components.Schemas[schemaName+"Update"]; !ok {
		t.Errorf("Schema %q not found in components", schemaName+"Update")
	}

	// View should only have base schema (no Create/Update)
	viewSchemaName := "Mydb_Active_users"
	if _, ok := doc.Components.Schemas[viewSchemaName]; !ok {
		t.Errorf("Schema %q not found in components", viewSchemaName)
	}
	// Views do not get Create or Update variants
	if _, ok := doc.Components.Schemas[viewSchemaName+"Create"]; ok {
		t.Errorf("Schema %q should NOT exist for view", viewSchemaName+"Create")
	}
	if _, ok := doc.Components.Schemas[viewSchemaName+"Update"]; ok {
		t.Errorf("Schema %q should NOT exist for view", viewSchemaName+"Update")
	}
}

func TestGenerateServiceSpec_CreateSchemaExcludesAutoIncrement(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	createSchema := doc.Components.Schemas["Mydb_UsersCreate"]
	if createSchema == nil {
		t.Fatal("Mydb_UsersCreate schema not found")
	}

	// "id" is auto-increment, should be excluded from create schema
	if _, ok := createSchema.Value.Properties["id"]; ok {
		t.Error("Auto-increment column 'id' should not appear in create schema")
	}

	// Other columns should still be present
	if _, ok := createSchema.Value.Properties["name"]; !ok {
		t.Error("Column 'name' should be present in create schema")
	}
	if _, ok := createSchema.Value.Properties["email"]; !ok {
		t.Error("Column 'email' should be present in create schema")
	}
}

func TestGenerateServiceSpec_CreateSchemaRequiredFields(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	createSchema := doc.Components.Schemas["Mydb_UsersCreate"]
	if createSchema == nil {
		t.Fatal("Mydb_UsersCreate schema not found")
	}

	// "name" and "email" are non-nullable without defaults -> required
	// "created_at" is non-nullable but has a default -> not required
	// "bio" is nullable -> not required
	required := make(map[string]bool)
	for _, r := range createSchema.Value.Required {
		required[r] = true
	}

	if !required["name"] {
		t.Error("'name' should be required on create")
	}
	if !required["email"] {
		t.Error("'email' should be required on create")
	}
	if required["created_at"] {
		t.Error("'created_at' should NOT be required (has default)")
	}
	if required["bio"] {
		t.Error("'bio' should NOT be required (nullable)")
	}
}

func TestGenerateServiceSpec_UpdateSchemaExcludesAutoIncrement(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	updateSchema := doc.Components.Schemas["Mydb_UsersUpdate"]
	if updateSchema == nil {
		t.Fatal("Mydb_UsersUpdate schema not found")
	}

	// "id" is auto-increment, should be excluded
	if _, ok := updateSchema.Value.Properties["id"]; ok {
		t.Error("Auto-increment column 'id' should not appear in update schema")
	}

	// All fields should be optional (no Required)
	if len(updateSchema.Value.Required) > 0 {
		t.Errorf("Update schema should have no required fields, got %v", updateSchema.Value.Required)
	}
}

func TestGenerateServiceSpec_ProcedureRequestBody(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	procPath := doc.Paths.Find("/api/v1/mydb/_proc/get_user_stats")
	if procPath == nil || procPath.Post == nil {
		t.Fatal("Procedure POST operation not found")
	}

	reqBody := procPath.Post.RequestBody
	if reqBody == nil || reqBody.Value == nil {
		t.Fatal("Procedure request body is nil")
	}

	jsonContent, ok := reqBody.Value.Content["application/json"]
	if !ok {
		t.Fatal("application/json content not found in request body")
	}

	// Should include "in" parameters but not "out" parameters
	props := jsonContent.Schema.Value.Properties
	if _, ok := props["user_id"]; !ok {
		t.Error("'user_id' (in param) should be in request body schema")
	}
	if _, ok := props["result"]; ok {
		t.Error("'result' (out param) should NOT be in request body schema")
	}
}

func TestGenerateServiceSpec_TableOperationTags(t *testing.T) {
	schema := testSchema()
	doc := GenerateServiceSpec("mydb", "My Database", "postgres", "http://localhost:8080", schema)

	tablePath := doc.Paths.Find("/api/v1/mydb/_table/users")
	if tablePath == nil {
		t.Fatal("Table path not found")
	}

	// Each operation should have the table name as a tag
	checkTags := func(name string, tags []string) {
		t.Helper()
		if len(tags) == 0 || tags[0] != "users" {
			t.Errorf("%s operation tags = %v, want [users]", name, tags)
		}
	}
	checkTags("GET", tablePath.Get.Tags)
	checkTags("POST", tablePath.Post.Tags)
	checkTags("PUT", tablePath.Put.Tags)
	checkTags("PATCH", tablePath.Patch.Tags)
	checkTags("DELETE", tablePath.Delete.Tags)
}

func TestGenerateServiceSpec_EmptySchema(t *testing.T) {
	schema := &model.Schema{}
	doc := GenerateServiceSpec("empty", "Empty DB", "postgres", "http://localhost:8080", schema)

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("OpenAPI version = %q, want %q", doc.OpenAPI, "3.1.0")
	}

	// Should still have error response schema and security schemes
	if _, ok := doc.Components.Schemas["ErrorResponse"]; !ok {
		t.Error("ErrorResponse schema should exist even with empty schema")
	}
	if _, ok := doc.Components.SecuritySchemes["apiKey"]; !ok {
		t.Error("apiKey security scheme should exist even with empty schema")
	}
}

// ─── GenerateCombinedSpec Tests ─────────────────────────────────────────────

func TestGenerateCombinedSpec_CombinesMultipleServices(t *testing.T) {
	schema1 := &model.Schema{
		Tables: []model.TableSchema{
			{
				Name: "users",
				Type: "table",
				Columns: []model.Column{
					{Name: "id", Position: 1, Type: "integer", IsPrimaryKey: true},
					{Name: "name", Position: 2, Type: "varchar"},
				},
			},
		},
	}

	schema2 := &model.Schema{
		Tables: []model.TableSchema{
			{
				Name: "products",
				Type: "table",
				Columns: []model.Column{
					{Name: "id", Position: 1, Type: "integer", IsPrimaryKey: true},
					{Name: "title", Position: 2, Type: "varchar"},
				},
			},
		},
	}

	services := []ServiceSpec{
		{Name: "db1", Label: "Database 1", Driver: "postgres", Schema: schema1},
		{Name: "db2", Label: "Database 2", Driver: "mysql", Schema: schema2},
	}

	doc := GenerateCombinedSpec(services, "http://localhost:8080")

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("OpenAPI version = %q, want %q", doc.OpenAPI, "3.1.0")
	}
	if doc.Info.Title != "Faucet API" {
		t.Errorf("Info.Title = %q, want %q", doc.Info.Title, "Faucet API")
	}

	// Paths should be namespaced by service name
	if doc.Paths.Find("/api/v1/db1/_table/users") == nil {
		t.Error("Path /api/v1/db1/_table/users not found")
	}
	if doc.Paths.Find("/api/v1/db2/_table/products") == nil {
		t.Error("Path /api/v1/db2/_table/products not found")
	}
}

func TestGenerateCombinedSpec_SkipsNilSchema(t *testing.T) {
	schema1 := &model.Schema{
		Tables: []model.TableSchema{
			{
				Name: "users",
				Type: "table",
				Columns: []model.Column{
					{Name: "id", Position: 1, Type: "integer"},
				},
			},
		},
	}

	services := []ServiceSpec{
		{Name: "db1", Label: "Database 1", Driver: "postgres", Schema: schema1},
		{Name: "db2", Label: "Database 2", Driver: "mysql", Schema: nil},
	}

	doc := GenerateCombinedSpec(services, "http://localhost:8080")

	// db1 paths should exist
	if doc.Paths.Find("/api/v1/db1/_table/users") == nil {
		t.Error("Path /api/v1/db1/_table/users not found")
	}

	// db2 should have no paths since schema was nil
	if doc.Paths.Find("/api/v1/db2/_table/users") != nil {
		t.Error("Path from nil-schema service should not exist")
	}
}

func TestGenerateCombinedSpec_PathsNamespacedByService(t *testing.T) {
	schema := &model.Schema{
		Tables: []model.TableSchema{
			{
				Name:    "orders",
				Type:    "table",
				Columns: []model.Column{{Name: "id", Position: 1, Type: "integer"}},
			},
		},
		Views: []model.TableSchema{
			{
				Name:    "order_summary",
				Type:    "view",
				Columns: []model.Column{{Name: "total", Position: 1, Type: "numeric"}},
			},
		},
		Procedures: []model.StoredProcedure{
			{Name: "process_order", Type: "procedure", Parameters: []model.ProcedureParam{
				{Name: "order_id", Type: "integer", Direction: "in"},
			}},
		},
		Functions: []model.StoredProcedure{
			{Name: "order_total", Type: "function", Parameters: []model.ProcedureParam{
				{Name: "order_id", Type: "integer", Direction: "in"},
			}},
		},
	}

	services := []ServiceSpec{
		{Name: "shop", Label: "Shop", Driver: "postgres", Schema: schema},
	}

	doc := GenerateCombinedSpec(services, "http://localhost:8080")

	expectedPaths := []string{
		"/api/v1/shop/_table/orders",
		"/api/v1/shop/_schema/orders",
		"/api/v1/shop/_table/order_summary",
		"/api/v1/shop/_schema/order_summary",
		"/api/v1/shop/_proc/process_order",
		"/api/v1/shop/_func/order_total",
	}

	for _, path := range expectedPaths {
		if doc.Paths.Find(path) == nil {
			t.Errorf("Path %q not found in combined spec", path)
		}
	}
}

func TestGenerateCombinedSpec_EmptyServiceList(t *testing.T) {
	doc := GenerateCombinedSpec(nil, "http://localhost:8080")

	if doc.OpenAPI != "3.1.0" {
		t.Errorf("OpenAPI version = %q, want %q", doc.OpenAPI, "3.1.0")
	}
	if _, ok := doc.Components.Schemas["ErrorResponse"]; !ok {
		t.Error("ErrorResponse schema should exist even with no services")
	}
}

func TestGenerateCombinedSpec_SecuritySchemes(t *testing.T) {
	doc := GenerateCombinedSpec(nil, "http://localhost:8080")

	if _, ok := doc.Components.SecuritySchemes["apiKey"]; !ok {
		t.Error("apiKey security scheme not found in combined spec")
	}
	if _, ok := doc.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Error("bearerAuth security scheme not found in combined spec")
	}
}

// ─── sanitizeSchemaName Tests ───────────────────────────────────────────────

func TestSanitizeSchemaName(t *testing.T) {
	tests := []struct {
		service string
		table   string
		want    string
	}{
		{"mydb", "users", "Mydb_Users"},
		{"db", "my_table", "Db_My_table"},
		{"service", "table-name", "Service_Table_name"},
		{"svc", "table.name", "Svc_Table_name"},
		{"DB", "Users", "DB_Users"},
		{"a", "b", "A_B"},
	}
	for _, tt := range tests {
		t.Run(tt.service+"_"+tt.table, func(t *testing.T) {
			got := sanitizeSchemaName(tt.service, tt.table)
			if got != tt.want {
				t.Errorf("sanitizeSchemaName(%q, %q) = %q, want %q", tt.service, tt.table, got, tt.want)
			}
		})
	}
}

func TestSanitizeSchemaName_CapitalizesFirstLetter(t *testing.T) {
	got := sanitizeSchemaName("myservice", "mytable")
	if got[0] != 'M' {
		t.Errorf("First char should be uppercase, got %c", got[0])
	}
}

func TestSanitizeSchemaName_ReplacesNonAlphanumeric(t *testing.T) {
	tests := []struct {
		service string
		table   string
		want    string
	}{
		{"svc", "table!name", "Svc_Table_name"},
		{"svc", "table@name", "Svc_Table_name"},
		{"svc", "table#name", "Svc_Table_name"},
		{"svc", "table$name", "Svc_Table_name"},
		{"svc", "table name", "Svc_Table_name"},
	}
	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			got := sanitizeSchemaName(tt.service, tt.table)
			if got != tt.want {
				t.Errorf("sanitizeSchemaName(%q, %q) = %q, want %q", tt.service, tt.table, got, tt.want)
			}
		})
	}
}

// ─── capitalize Tests ───────────────────────────────────────────────────────

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"hello", "Hello"},
		{"Hello", "Hello"},
		{"a", "A"},
		{"A", "A"},
		{"myService", "MyService"},
		{"123abc", "123abc"},
		{"already Capitalized", "Already Capitalized"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := capitalize(tt.input)
			if got != tt.want {
				t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCapitalize_EmptyString(t *testing.T) {
	got := capitalize("")
	if got != "" {
		t.Errorf("capitalize(%q) = %q, want %q", "", got, "")
	}
}

func TestCapitalize_SingleChar(t *testing.T) {
	got := capitalize("x")
	if got != "X" {
		t.Errorf("capitalize(%q) = %q, want %q", "x", got, "X")
	}
}
