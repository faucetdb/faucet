package model

// Schema represents the full introspection result for a database service,
// including tables, views, stored procedures, and functions.
type Schema struct {
	Tables     []TableSchema     `json:"tables"`
	Views      []TableSchema     `json:"views"`
	Procedures []StoredProcedure `json:"procedures"`
	Functions  []StoredProcedure `json:"functions"`
}

// TableSchema describes the structure of a single table or view.
type TableSchema struct {
	Name        string       `json:"name"`
	Type        string       `json:"type"` // "table" or "view"
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key"`
	ForeignKeys []ForeignKey `json:"foreign_keys"`
	Indexes     []Index      `json:"indexes"`
	RowCount    *int64       `json:"row_count,omitempty"`
}

// Column describes a single column within a table or view.
type Column struct {
	Name            string  `json:"name"`
	Position        int     `json:"position"`
	Type            string  `json:"db_type"`
	GoType          string  `json:"go_type"`
	JsonType        string  `json:"json_type"`
	Nullable        bool    `json:"nullable"`
	Default         *string `json:"default,omitempty"`
	MaxLength       *int64  `json:"max_length,omitempty"`
	IsPrimaryKey    bool    `json:"is_primary_key"`
	IsAutoIncrement bool    `json:"is_auto_increment"`
	IsUnique        bool    `json:"is_unique"`
	Comment         string  `json:"comment,omitempty"`
}

// ForeignKey describes a foreign key constraint between two tables.
type ForeignKey struct {
	Name             string `json:"name"`
	ColumnName       string `json:"column_name"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
	OnDelete         string `json:"on_delete"`
	OnUpdate         string `json:"on_update"`
}

// Index describes a database index on one or more columns.
type Index struct {
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	IsUnique bool     `json:"is_unique"`
}

// StoredProcedure describes a stored procedure or function in the database.
type StoredProcedure struct {
	Name       string           `json:"name"`
	Type       string           `json:"type"` // "procedure" or "function"
	ReturnType string           `json:"return_type,omitempty"`
	Parameters []ProcedureParam `json:"parameters,omitempty"`
}

// ProcedureParam describes a single parameter of a stored procedure or function.
type ProcedureParam struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Direction string `json:"direction"` // "in", "out", "inout"
}
