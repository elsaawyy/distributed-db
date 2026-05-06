// Package storage implements the in-memory storage engine with JSON file persistence
// for the Master node.
package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/distributed-db/master/models"
)

// Engine is the core storage layer. It holds all databases in memory
// and periodically flushes changes to disk as JSON files.
type Engine struct {
	mu        sync.RWMutex
	databases map[string]*models.Database
	dataDir   string
}

// NewEngine creates a new storage engine that persists data to dataDir.
func NewEngine(dataDir string) *Engine {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Warning: could not create data directory: %v", err)
	}
	return &Engine{
		databases: make(map[string]*models.Database),
		dataDir:   dataDir,
	}
}

// Load reads all persisted databases from disk into memory.
func (e *Engine) Load() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	entries, err := os.ReadDir(e.dataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(e.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: could not read %s: %v", path, err)
			continue
		}
		var db models.Database
		if err := json.Unmarshal(data, &db); err != nil {
			log.Printf("Warning: could not parse %s: %v", path, err)
			continue
		}
		e.databases[db.Name] = &db
		log.Printf("Loaded database '%s' from disk", db.Name)
	}
	return nil
}

// persist saves a single database to a JSON file on disk.
// Must be called with the write lock held (or within a locked section).
func (e *Engine) persist(dbName string) {
	db, ok := e.databases[dbName]
	if !ok {
		return
	}
	path := filepath.Join(e.dataDir, dbName+".json")
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		log.Printf("Error marshaling database '%s': %v", dbName, err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Error persisting database '%s': %v", dbName, err)
	}
}

// CreateDatabase creates a new empty database.
func (e *Engine) CreateDatabase(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.databases[name]; exists {
		return fmt.Errorf("database '%s' already exists", name)
	}
	e.databases[name] = &models.Database{
		Name:      name,
		Tables:    make(map[string]*models.Table),
		CreatedAt: time.Now(),
	}
	e.persist(name)
	log.Printf("Created database '%s'", name)
	return nil
}

// DropDatabase removes a database and deletes its file from disk.
func (e *Engine) DropDatabase(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.databases[name]; !exists {
		return fmt.Errorf("database '%s' does not exist", name)
	}
	delete(e.databases, name)
	path := filepath.Join(e.dataDir, name+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not delete file for database '%s': %v", name, err)
	}
	log.Printf("Dropped database '%s'", name)
	return nil
}

// CreateTable creates a new table with the given schema inside a database.
func (e *Engine) CreateTable(dbName string, schema models.TableSchema) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	db, err := e.getDB(dbName)
	if err != nil {
		return err
	}
	if _, exists := db.Tables[schema.Name]; exists {
		return fmt.Errorf("table '%s' already exists in database '%s'", schema.Name, dbName)
	}
	schema.CreatedAt = time.Now()
	db.Tables[schema.Name] = &models.Table{
		Schema:  schema,
		Records: make(map[string]*models.Record),
	}
	e.persist(dbName)
	log.Printf("Created table '%s' in database '%s'", schema.Name, dbName)
	return nil
}

// Insert adds a new record to a table and returns the generated record.
func (e *Engine) Insert(dbName, tableName string, fields map[string]interface{}) (*models.Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}
	// Validate fields against schema
	if err := validateFields(table.Schema, fields); err != nil {
		return nil, err
	}
	record := &models.Record{
		ID:        generateID(),
		Fields:    fields,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	table.Records[record.ID] = record
	e.persist(dbName)
	return record, nil
}

// Select retrieves records from a table, optionally filtered by where conditions.
func (e *Engine) Select(dbName, tableName string, where map[string]string) ([]*models.Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}
	var results []*models.Record
	for _, record := range table.Records {
		if matchesWhere(record, where) {
			results = append(results, record)
		}
	}
	return results, nil
}

// Update modifies specific fields of an existing record.
func (e *Engine) Update(dbName, tableName, recordID string, fields map[string]interface{}) (*models.Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}
	record, exists := table.Records[recordID]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found", recordID)
	}
	for k, v := range fields {
		record.Fields[k] = v
	}
	record.UpdatedAt = time.Now()
	e.persist(dbName)
	return record, nil
}

// Delete removes a record from a table by ID.
func (e *Engine) Delete(dbName, tableName, recordID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return err
	}
	if _, exists := table.Records[recordID]; !exists {
		return fmt.Errorf("record '%s' not found", recordID)
	}
	delete(table.Records, recordID)
	e.persist(dbName)
	return nil
}

// Search finds all records where a specific column contains the search value (case-insensitive substring).
func (e *Engine) Search(dbName, tableName, column, value string) ([]*models.Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}
	var results []*models.Record
	for _, record := range table.Records {
		if fieldVal, ok := record.Fields[column]; ok {
			if containsStr(fmt.Sprintf("%v", fieldVal), value) {
				results = append(results, record)
			}
		}
	}
	return results, nil
}

// ListDatabases returns the names of all databases.
func (e *Engine) ListDatabases() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.databases))
	for name := range e.databases {
		names = append(names, name)
	}
	return names
}

// GetDatabase returns a snapshot of the database metadata (no records).
func (e *Engine) GetDatabase(name string) (*models.Database, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.getDB(name)
}

// --- Internal helpers ---

func (e *Engine) getDB(name string) (*models.Database, error) {
	db, ok := e.databases[name]
	if !ok {
		return nil, fmt.Errorf("database '%s' does not exist", name)
	}
	return db, nil
}

func (e *Engine) getTable(dbName, tableName string) (*models.Table, error) {
	db, err := e.getDB(dbName)
	if err != nil {
		return nil, err
	}
	table, ok := db.Tables[tableName]
	if !ok {
		return nil, fmt.Errorf("table '%s' does not exist in database '%s'", tableName, dbName)
	}
	return table, nil
}

func validateFields(schema models.TableSchema, fields map[string]interface{}) error {
	colMap := make(map[string]models.Column)
	for _, col := range schema.Columns {
		colMap[col.Name] = col
	}
	for _, col := range schema.Columns {
		if col.Required {
			if _, ok := fields[col.Name]; !ok {
				return fmt.Errorf("required column '%s' is missing", col.Name)
			}
		}
	}
	for key := range fields {
		if _, ok := colMap[key]; !ok {
			return fmt.Errorf("unknown column '%s'", key)
		}
	}
	return nil
}

func matchesWhere(record *models.Record, where map[string]string) bool {
	if len(where) == 0 {
		return true
	}
	for k, v := range where {
		fieldVal, ok := record.Fields[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", fieldVal) != v {
			return false
		}
	}
	return true
}

func containsStr(haystack, needle string) bool {
	return len(needle) == 0 ||
		len(haystack) >= len(needle) &&
			(haystack == needle ||
				(func() bool {
					for i := 0; i <= len(haystack)-len(needle); i++ {
						if haystack[i:i+len(needle)] == needle {
							return true
						}
					}
					return false
				})())
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
