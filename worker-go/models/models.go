// Package models defines the data structures used by the Go Worker node.
package models

import "time"

// ColumnType represents supported column data types.
type ColumnType string

const (
	TypeString ColumnType = "string"
	TypeInt    ColumnType = "int"
	TypeFloat  ColumnType = "float"
	TypeBool   ColumnType = "bool"
)

// Column defines a single column in a table schema.
type Column struct {
	Name     string     `json:"name"`
	Type     ColumnType `json:"type"`
	Required bool       `json:"required"`
}

// TableSchema describes the structure of a table.
type TableSchema struct {
	Name      string    `json:"name"`
	Columns   []Column  `json:"columns"`
	CreatedAt time.Time `json:"created_at"`
}

// Record represents a single row in a table.
type Record struct {
	ID        string                 `json:"id"`
	Fields    map[string]interface{} `json:"fields"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Database groups a name with its collection of tables.
type Database struct {
	Name      string             `json:"name"`
	Tables    map[string]*Table  `json:"tables"`
	CreatedAt time.Time          `json:"created_at"`
}

// Table holds a schema and all its records.
type Table struct {
	Schema  TableSchema        `json:"schema"`
	Records map[string]*Record `json:"records"`
}

// ReplicationPayload is the payload received from the Master for all write operations.
type ReplicationPayload struct {
	Operation string                 `json:"operation"`
	Database  string                 `json:"database"`
	Table     string                 `json:"table,omitempty"`
	Schema    *TableSchema           `json:"schema,omitempty"`
	Record    *Record                `json:"record,omitempty"`
	RecordID  string                 `json:"record_id,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// APIResponse is a generic JSON response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// AnalyticsResult is returned by the /analytics endpoint (special task).
type AnalyticsResult struct {
	Database    string                 `json:"database"`
	Table       string                 `json:"table"`
	TotalRows   int                    `json:"total_rows"`
	ColumnStats map[string]ColumnStats `json:"column_stats"`
}

// ColumnStats holds basic statistics for a single column.
type ColumnStats struct {
	NonNullCount int     `json:"non_null_count"`
	UniqueValues int     `json:"unique_values"`
	SampleValues []string `json:"sample_values"`
}
