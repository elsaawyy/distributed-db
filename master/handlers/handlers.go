// Package handlers implements all HTTP handler functions for the Master node REST API.
package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/distributed-db/master/models"
	"github.com/distributed-db/master/replication"
	"github.com/distributed-db/master/storage"
)

// Handler bundles the storage engine and replication manager,
// and serves as the receiver for all HTTP handler methods.
type Handler struct {
	store      *storage.Engine
	replicator *replication.Manager
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(store *storage.Engine, replicator *replication.Manager) *Handler {
	return &Handler{store: store, replicator: replicator}
}

// --- Helpers ---

func respond(w http.ResponseWriter, status int, resp models.APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func decodeBody(r *http.Request, dst interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// --- Database Handlers ---

// CreateDatabase handles POST /create-db
// Body: {"name": "mydb"}
func (h *Handler) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.CreateDBRequest
	if err := decodeBody(r, &req); err != nil || req.Name == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request body; 'name' is required"})
		return
	}
	if err := h.store.CreateDatabase(req.Name); err != nil {
		respond(w, http.StatusConflict, models.APIResponse{Error: err.Error()})
		return
	}
	// Replicate to workers
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "create_db",
		Database:  req.Name,
	})
	respond(w, http.StatusCreated, models.APIResponse{Success: true, Message: "Database created: " + req.Name})
}

// ProxyToGoAnalytics forwards GET /analytics to Go worker
func (h *Handler) ProxyToGoAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}

	// Get worker address from replicator
	workers := h.replicator.WorkerStatuses()
	var goWorkerAddr string
	for _, w := range workers {
		if w.Address == "http://localhost:8081" || strings.Contains(w.Address, "8081") {
			goWorkerAddr = w.Address
			break
		}
	}

	if goWorkerAddr == "" {
		respond(w, http.StatusServiceUnavailable, models.APIResponse{Error: "Go worker not available"})
		return
	}

	// Forward query params
	url := goWorkerAddr + "/analytics?" + r.URL.RawQuery
	resp, err := http.Get(url)
	if err != nil {
		respond(w, http.StatusBadGateway, models.APIResponse{Error: err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// ProxyToPythonTransform forwards POST /transform to Python worker
func (h *Handler) ProxyToPythonTransform(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}

	workers := h.replicator.WorkerStatuses()
	var pyWorkerAddr string
	for _, w := range workers {
		if w.Address == "http://localhost:8082" || w.Address == os.Getenv("WORKER_PY_ADDR") {
			pyWorkerAddr = w.Address
			break
		}
	}

	if pyWorkerAddr == "" {
		respond(w, http.StatusServiceUnavailable, models.APIResponse{Error: "Python worker not available"})
		return
	}

	// Forward request body
	bodyBytes, _ := io.ReadAll(r.Body)
	url := pyWorkerAddr + "/transform"

	resp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		respond(w, http.StatusBadGateway, models.APIResponse{Error: err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// DropDatabase handles DELETE /drop-db
// Body: {"name": "mydb"}
func (h *Handler) DropDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.DropDBRequest
	if err := decodeBody(r, &req); err != nil || req.Name == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request body; 'name' is required"})
		return
	}
	if err := h.store.DropDatabase(req.Name); err != nil {
		respond(w, http.StatusNotFound, models.APIResponse{Error: err.Error()})
		return
	}
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "drop_db",
		Database:  req.Name,
	})
	respond(w, http.StatusOK, models.APIResponse{Success: true, Message: "Database dropped: " + req.Name})
}

// --- Table Handlers ---

// CreateTable handles POST /create-table
func (h *Handler) CreateTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.CreateTableRequest
	if err := decodeBody(r, &req); err != nil || req.Database == "" || req.Table == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request; 'database' and 'table' are required"})
		return
	}
	schema := models.TableSchema{
		Name:    req.Table,
		Columns: req.Columns,
	}
	if err := h.store.CreateTable(req.Database, schema); err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: err.Error()})
		return
	}
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "create_table",
		Database:  req.Database,
		Table:     req.Table,
		Schema:    &schema,
	})
	respond(w, http.StatusCreated, models.APIResponse{
		Success: true,
		Message: "Table created: " + req.Table,
		Data:    schema,
	})
}

// --- Data Operation Handlers ---

// Insert handles POST /insert
func (h *Handler) Insert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.InsertRequest
	if err := decodeBody(r, &req); err != nil || req.Database == "" || req.Table == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request; 'database', 'table', and 'fields' are required"})
		return
	}
	record, err := h.store.Insert(req.Database, req.Table, req.Fields)
	if err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: err.Error()})
		return
	}
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "insert",
		Database:  req.Database,
		Table:     req.Table,
		Record:    record,
	})
	respond(w, http.StatusCreated, models.APIResponse{Success: true, Message: "Record inserted", Data: record})
}

// Select handles GET /select
// Query params: database, table, and optionally where_<column>=<value>
func (h *Handler) Select(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	q := r.URL.Query()
	dbName := q.Get("database")
	tableName := q.Get("table")
	if dbName == "" || tableName == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "'database' and 'table' query params required"})
		return
	}
	// Parse optional where_<column>=<value> filters
	where := make(map[string]string)
	for key, vals := range q {
		if len(key) > 6 && key[:6] == "where_" {
			where[key[6:]] = vals[0]
		}
	}
	records, err := h.store.Select(dbName, tableName, where)
	if err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: err.Error()})
		return
	}
	if records == nil {
		records = []*models.Record{}
	}
	respond(w, http.StatusOK, models.APIResponse{Success: true, Data: records})
}

// Update handles PUT /update
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.UpdateRequest
	if err := decodeBody(r, &req); err != nil || req.Database == "" || req.Table == "" || req.ID == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request; 'database', 'table', 'id', and 'fields' are required"})
		return
	}
	record, err := h.store.Update(req.Database, req.Table, req.ID, req.Fields)
	if err != nil {
		respond(w, http.StatusNotFound, models.APIResponse{Error: err.Error()})
		return
	}
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "update",
		Database:  req.Database,
		Table:     req.Table,
		RecordID:  req.ID,
		Fields:    req.Fields,
	})
	respond(w, http.StatusOK, models.APIResponse{Success: true, Message: "Record updated", Data: record})
}

// Delete handles DELETE /delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	var req models.DeleteRequest
	if err := decodeBody(r, &req); err != nil || req.Database == "" || req.Table == "" || req.ID == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid request; 'database', 'table', and 'id' are required"})
		return
	}
	if err := h.store.Delete(req.Database, req.Table, req.ID); err != nil {
		respond(w, http.StatusNotFound, models.APIResponse{Error: err.Error()})
		return
	}
	h.replicator.Replicate(models.ReplicationPayload{
		Operation: "delete",
		Database:  req.Database,
		Table:     req.Table,
		RecordID:  req.ID,
	})
	respond(w, http.StatusOK, models.APIResponse{Success: true, Message: "Record deleted"})
}

// Search handles GET /search
// Query params: database, table, column, value
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	q := r.URL.Query()
	dbName := q.Get("database")
	tableName := q.Get("table")
	column := q.Get("column")
	value := q.Get("value")
	if dbName == "" || tableName == "" || column == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "'database', 'table', and 'column' are required"})
		return
	}
	records, err := h.store.Search(dbName, tableName, column, value)
	if err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: err.Error()})
		return
	}
	if records == nil {
		records = []*models.Record{}
	}
	respond(w, http.StatusOK, models.APIResponse{Success: true, Data: records})
}

// --- Health / Status ---

// Health handles GET /health — lightweight liveness check.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Master is healthy",
		Data:    map[string]string{"node": "master", "time": time.Now().Format(time.RFC3339)},
	})
}

// Status handles GET /status — returns databases and worker statuses.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	workers := h.replicator.WorkerStatuses()
	databases := h.store.ListDatabases()
	log.Printf("Status requested: %d databases, %d workers", len(databases), len(workers))
	respond(w, http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"node":      "master",
			"databases": databases,
			"workers":   workers,
		},
	})
}
