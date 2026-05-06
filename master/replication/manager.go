// Package replication handles synchronous replication from the Master to all Worker nodes.
// When a write operation succeeds on the Master, it is immediately forwarded to all
// registered workers. If a worker is unreachable, the error is logged but the master
// operation is not rolled back (best-effort synchronous replication).
package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/distributed-db/master/models"
)

// WorkerStatus tracks liveness of each worker.
type WorkerStatus struct {
	Address   string    `json:"address"`
	Alive     bool      `json:"alive"`
	LastCheck time.Time `json:"last_check"`
	LastError string    `json:"last_error,omitempty"`
}

// Manager coordinates replication to all worker nodes.
type Manager struct {
	mu      sync.RWMutex
	workers []*WorkerStatus
	client  *http.Client
}

// NewManager creates a replication manager with the given worker addresses.
func NewManager(addresses []string) *Manager {
	workers := make([]*WorkerStatus, len(addresses))
	for i, addr := range addresses {
		workers[i] = &WorkerStatus{
			Address:   addr,
			Alive:     true,
			LastCheck: time.Now(),
		}
	}
	m := &Manager{
		workers: workers,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	// Start background health checker
	go m.healthLoop()
	return m
}

// Replicate sends a payload to all alive workers synchronously.
// Returns an error only if ALL workers fail (partial failure is logged).
func (m *Manager) Replicate(payload models.ReplicationPayload) error {
	m.mu.RLock()
	workers := make([]*WorkerStatus, len(m.workers))
	copy(workers, m.workers)
	m.mu.RUnlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("replication marshal error: %w", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, len(workers))

	for i, worker := range workers {
		if !worker.Alive {
			log.Printf("Skipping unreachable worker: %s", worker.Address)
			continue
		}
		wg.Add(1)
		go func(idx int, w *WorkerStatus) {
			defer wg.Done()
			errs[idx] = m.sendToWorker(w, data)
			if errs[idx] != nil {
				log.Printf("Replication to %s failed: %v", w.Address, errs[idx])
				m.markWorker(w.Address, false, errs[idx].Error())
			} else {
				log.Printf("Replicated %s to %s", payload.Operation, w.Address)
			}
		}(i, worker)
	}
	wg.Wait()

	// Count successes
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		}
	}
	if successes == 0 && len(workers) > 0 {
		return fmt.Errorf("replication failed: all workers unreachable")
	}
	return nil
}

// sendToWorker POSTs the replication payload to a single worker's /replicate endpoint.
func (m *Manager) sendToWorker(w *WorkerStatus, data []byte) error {
	url := w.Address + "/replicate"
	resp, err := m.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("worker returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// WorkerStatuses returns the current status of all known workers.
func (m *Manager) WorkerStatuses() []*WorkerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*WorkerStatus, len(m.workers))
	copy(out, m.workers)
	return out
}

// markWorker updates the alive status of a worker by address.
func (m *Manager) markWorker(address string, alive bool, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.workers {
		if w.Address == address {
			w.Alive = alive
			w.LastCheck = time.Now()
			w.LastError = errMsg
			return
		}
	}
}

// healthLoop pings all workers every 10 seconds to detect recovery.
func (m *Manager) healthLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.RLock()
		workers := make([]*WorkerStatus, len(m.workers))
		copy(workers, m.workers)
		m.mu.RUnlock()

		for _, w := range workers {
			go m.pingWorker(w)
		}
	}
}

// pingWorker checks if a worker is alive via its /health endpoint.
func (m *Manager) pingWorker(w *WorkerStatus) {
	resp, err := m.client.Get(w.Address + "/health")
	if err != nil {
		m.markWorker(w.Address, false, err.Error())
		return
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		m.markWorker(w.Address, true, "")
		if !w.Alive {
			log.Printf("Worker %s is back online", w.Address)
		}
	}
}
