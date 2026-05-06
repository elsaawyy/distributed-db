// Package handlers implements HTTP handlers for the Go Worker node.
package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/distributed-db/worker-go/models"
	"github.com/distributed-db/worker-go/storage"
)

// Handler contains the storage engine reference.
type Handler struct {
	store *storage.Engine
}

// NewHandler creates a new Handler.
func NewHandler(store *storage.Engine) *Handler {
	return &Handler{store: store}
}

func respond(w http.ResponseWriter, status int, resp models.APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// Replicate handles POST /replicate — called by the Master to push write operations.
func (h *Handler) Replicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "could not read body"})
		return
	}
	var payload models.ReplicationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if err := h.store.ApplyReplication(payload); err != nil {
		respond(w, http.StatusInternalServerError, models.APIResponse{Error: err.Error()})
		return
	}
	respond(w, http.StatusOK, models.APIResponse{Success: true, Message: "Replicated: " + payload.Operation})
}

// Select handles GET /select — allows reads directly from the worker.
// This supports fault tolerance: clients can read from workers when master is down.
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

// Analytics handles GET /analytics — the Go worker's "special task".
// Returns aggregated statistics (row count, unique values, non-null counts) for a table.
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respond(w, http.StatusMethodNotAllowed, models.APIResponse{Error: "method not allowed"})
		return
	}
	q := r.URL.Query()
	dbName := q.Get("database")
	tableName := q.Get("table")
	if dbName == "" || tableName == "" {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: "'database' and 'table' required"})
		return
	}
	result, err := h.store.Analytics(dbName, tableName)
	if err != nil {
		respond(w, http.StatusBadRequest, models.APIResponse{Error: err.Error()})
		return
	}
	respond(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Analytics computed by Go Worker",
		Data:    result,
	})
}

// Health handles GET /health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Go Worker is healthy",
		Data: map[string]interface{}{
			"node":      "worker-go",
			"time":      time.Now().Format(time.RFC3339),
			"databases": h.store.ListDatabases(),
		},
	})
}

// Status handles GET /status — returns stored databases.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"node":      "worker-go",
			"databases": h.store.ListDatabases(),
		},
	})
}
