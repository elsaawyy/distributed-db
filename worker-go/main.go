// Package main is the entry point for the Go Worker node.
// The Go Worker receives replicated data from the Master, stores it locally,
// serves read requests directly, and provides a special analytics endpoint.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/distributed-db/worker-go/handlers"
	"github.com/distributed-db/worker-go/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[WORKER-GO] ")

	store := storage.NewEngine("./data")
	if err := store.Load(); err != nil {
		log.Printf("Warning: Could not load existing data: %v", err)
	}

	h := handlers.NewHandler(store)

	mux := http.NewServeMux()

	// Replication endpoint — called by Master
	mux.HandleFunc("/replicate", h.Replicate)

	// Read endpoints — available directly for fault-tolerant reads
	mux.HandleFunc("/select", h.Select)

	// Special task: analytics
	mux.HandleFunc("/analytics", h.Analytics)

	// Health & status
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/status", h.Status)

	port := getEnv("WORKER_GO_PORT", "8081")
	addr := ":" + port

	log.Printf("Go Worker node starting on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("Go Worker failed to start: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
