// Package models defines the core data structures shared across the Master node.
package models

import "time"

// ColumnType represents supported column data types.
type ColumnType string

const (
	TypeString  ColumnType = "string"
	TypeInt     ColumnType = "int"
	TypeFloat   ColumnType = "float"
	TypeBool    ColumnType = "bool"
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
// Fields is a map of column name → value.
type Record struct {
	ID        string                 `json:"id"`
	Fields    map[string]interface{} `json:"fields"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Database groups a name with its collection of tables.
type Database struct {
	Name      string                  `json:"name"`
	Tables    map[string]*Table       `json:"tables"`
	CreatedAt time.Time               `json:"created_at"`
}

// Table holds a schema and all its records.
type Table struct {
	Schema  TableSchema        `json:"schema"`
	Records map[string]*Record `json:"records"`
}

// --- Request/Response DTOs ---

// CreateDBRequest is the payload for POST /create-db.
type CreateDBRequest struct {
	Name string `json:"name"`
}

// CreateTableRequest is the payload for POST /create-table.
type CreateTableRequest struct {
	Database string      `json:"database"`
	Table    string      `json:"table"`
	Columns  []Column    `json:"columns"`
}

// InsertRequest is the payload for POST /insert.
type InsertRequest struct {
	Database string                 `json:"database"`
	Table    string                 `json:"table"`
	Fields   map[string]interface{} `json:"fields"`
}

// SelectRequest is the query payload for GET /select.
type SelectRequest struct {
	Database string            `json:"database"`
	Table    string            `json:"table"`
	Where    map[string]string `json:"where,omitempty"`
}

// UpdateRequest is the payload for PUT /update.
type UpdateRequest struct {
	Database string                 `json:"database"`
	Table    string                 `json:"table"`
	ID       string                 `json:"id"`
	Fields   map[string]interface{} `json:"fields"`
}

// DeleteRequest is the payload for DELETE /delete.
type DeleteRequest struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	ID       string `json:"id"`
}

// SearchRequest is the payload for GET /search.
type SearchRequest struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	Column   string `json:"column"`
	Value    string `json:"value"`
}

// DropDBRequest is the payload for DELETE /drop-db.
type DropDBRequest struct {
	Name string `json:"name"`
}

// APIResponse is a generic JSON response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ReplicationPayload is sent from Master → Worker for all write operations.
type ReplicationPayload struct {
	Operation string                 `json:"operation"` // create_db, create_table, insert, update, delete, drop_db
	Database  string                 `json:"database"`
	Table     string                 `json:"table,omitempty"`
	Schema    *TableSchema           `json:"schema,omitempty"`
	Record    *Record                `json:"record,omitempty"`
	RecordID  string                 `json:"record_id,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}
