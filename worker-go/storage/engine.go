// Package storage implements the storage engine for the Go Worker node.
// It mirrors the Master's storage interface but is populated exclusively
// through replication — workers never accept direct write requests from clients.
package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/distributed-db/worker-go/models"
)

// Engine is the worker's storage layer.
type Engine struct {
	mu        sync.RWMutex
	databases map[string]*models.Database
	dataDir   string
}

// NewEngine creates a new storage engine.
func NewEngine(dataDir string) *Engine {
	os.MkdirAll(dataDir, 0755)
	return &Engine{
		databases: make(map[string]*models.Database),
		dataDir:   dataDir,
	}
}

// Load reads all persisted databases from disk.
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
		data, err := os.ReadFile(filepath.Join(e.dataDir, entry.Name()))
		if err != nil {
			continue
		}
		var db models.Database
		if err := json.Unmarshal(data, &db); err != nil {
			continue
		}
		e.databases[db.Name] = &db
		log.Printf("Loaded database '%s'", db.Name)
	}
	return nil
}

func (e *Engine) persist(dbName string) {
	db, ok := e.databases[dbName]
	if !ok {
		return
	}
	data, _ := json.MarshalIndent(db, "", "  ")
	os.WriteFile(filepath.Join(e.dataDir, dbName+".json"), data, 0644)
}

// ApplyReplication processes a replication payload from the master.
func (e *Engine) ApplyReplication(payload models.ReplicationPayload) error {
	switch payload.Operation {
	case "create_db":
		return e.createDatabase(payload.Database)
	case "drop_db":
		return e.dropDatabase(payload.Database)
	case "create_table":
		if payload.Schema == nil {
			return fmt.Errorf("missing schema in create_table replication")
		}
		return e.createTable(payload.Database, *payload.Schema)
	case "insert":
		if payload.Record == nil {
			return fmt.Errorf("missing record in insert replication")
		}
		return e.insertRecord(payload.Database, payload.Table, payload.Record)
	case "update":
		return e.updateRecord(payload.Database, payload.Table, payload.RecordID, payload.Fields)
	case "delete":
		return e.deleteRecord(payload.Database, payload.Table, payload.RecordID)
	default:
		return fmt.Errorf("unknown operation: %s", payload.Operation)
	}
}

func (e *Engine) createDatabase(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.databases[name]; exists {
		return nil // idempotent
	}
	e.databases[name] = &models.Database{
		Name:      name,
		Tables:    make(map[string]*models.Table),
		CreatedAt: time.Now(),
	}
	e.persist(name)
	return nil
}

func (e *Engine) dropDatabase(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.databases, name)
	os.Remove(filepath.Join(e.dataDir, name+".json"))
	return nil
}

func (e *Engine) createTable(dbName string, schema models.TableSchema) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	db, err := e.getDB(dbName)
	if err != nil {
		return err
	}
	if _, exists := db.Tables[schema.Name]; exists {
		return nil // idempotent
	}
	db.Tables[schema.Name] = &models.Table{
		Schema:  schema,
		Records: make(map[string]*models.Record),
	}
	e.persist(dbName)
	return nil
}

func (e *Engine) insertRecord(dbName, tableName string, record *models.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return err
	}
	table.Records[record.ID] = record
	e.persist(dbName)
	return nil
}

func (e *Engine) updateRecord(dbName, tableName, id string, fields map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return err
	}
	record, exists := table.Records[id]
	if !exists {
		return fmt.Errorf("record '%s' not found", id)
	}
	for k, v := range fields {
		record.Fields[k] = v
	}
	record.UpdatedAt = time.Now()
	e.persist(dbName)
	return nil
}

func (e *Engine) deleteRecord(dbName, tableName, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return err
	}
	delete(table.Records, id)
	e.persist(dbName)
	return nil
}

// Select retrieves records with optional where filters.
func (e *Engine) Select(dbName, tableName string, where map[string]string) ([]*models.Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}
	var results []*models.Record
	for _, r := range table.Records {
		if matchesWhere(r, where) {
			results = append(results, r)
		}
	}
	return results, nil
}

// Analytics computes basic statistics for a table (special task for Go worker).
func (e *Engine) Analytics(dbName, tableName string) (*models.AnalyticsResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	table, err := e.getTable(dbName, tableName)
	if err != nil {
		return nil, err
	}

	colStats := make(map[string]models.ColumnStats)
	for _, col := range table.Schema.Columns {
		colStats[col.Name] = models.ColumnStats{
			SampleValues: []string{},
		}
	}

	uniqueVals := make(map[string]map[string]struct{})
	for _, col := range table.Schema.Columns {
		uniqueVals[col.Name] = make(map[string]struct{})
	}

	for _, record := range table.Records {
		for colName, stat := range colStats {
			if val, ok := record.Fields[colName]; ok && val != nil {
				stat.NonNullCount++
				strVal := fmt.Sprintf("%v", val)
				uniqueVals[colName][strVal] = struct{}{}
				if len(stat.SampleValues) < 5 {
					stat.SampleValues = append(stat.SampleValues, strVal)
				}
				colStats[colName] = stat
			}
		}
	}
	for colName, uv := range uniqueVals {
		if stat, ok := colStats[colName]; ok {
			stat.UniqueValues = len(uv)
			colStats[colName] = stat
		}
	}

	return &models.AnalyticsResult{
		Database:    dbName,
		Table:       tableName,
		TotalRows:   len(table.Records),
		ColumnStats: colStats,
	}, nil
}

// ListDatabases returns names of all databases.
func (e *Engine) ListDatabases() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.databases))
	for name := range e.databases {
		names = append(names, name)
	}
	return names
}

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
		return nil, fmt.Errorf("table '%s' not found", tableName)
	}
	return table, nil
}

func matchesWhere(record *models.Record, where map[string]string) bool {
	for k, v := range where {
		fv, ok := record.Fields[k]
		if !ok || fmt.Sprintf("%v", fv) != v {
			return false
		}
	}
	return true
}
