// Package main is the entry point for the Master node of the Distributed Database System.
// The Master is responsible for coordinating all database operations, managing schemas,
// and replicating data across Worker nodes.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/distributed-db/master/handlers"
	"github.com/distributed-db/master/replication"
	"github.com/distributed-db/master/storage"
)

//go:embed web
var webFS embed.FS

func main() {
	// Configure logging with timestamps and file info
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[MASTER] ")

	// Initialize the in-memory + file-backed storage engine
	store := storage.NewEngine("./data")
	if err := store.Load(); err != nil {
		log.Printf("Warning: Could not load existing data: %v (starting fresh)", err)
	}

	// Initialize the replication manager — reads worker addresses from env or defaults
	workerAddrs := []string{
		getEnv("WORKER_GO_ADDR", "http://localhost:8081"),
		getEnv("WORKER_PY_ADDR", "http://localhost:8082"),
	}
	replicator := replication.NewManager(workerAddrs)

	// Initialize the HTTP handler layer
	h := handlers.NewHandler(store, replicator)

	// Register all REST API routes
	mux := http.NewServeMux()

	// Database management
	mux.HandleFunc("/create-db", h.CreateDatabase)
	mux.HandleFunc("/drop-db", h.DropDatabase)

	// Table management
	mux.HandleFunc("/create-table", h.CreateTable)

	// Data operations
	mux.HandleFunc("/insert", h.Insert)
	mux.HandleFunc("/select", h.Select)
	mux.HandleFunc("/update", h.Update)
	mux.HandleFunc("/delete", h.Delete)
	mux.HandleFunc("/search", h.Search)

	// Health & status
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/status", h.Status)

	// Proxy routes for worker special tasks (GUI uses these to avoid CORS)
	mux.HandleFunc("/proxy/analytics", h.ProxyToGoAnalytics)
	mux.HandleFunc("/proxy/transform", h.ProxyToPythonTransform)

	// GUI - serve embedded web files
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal("Failed to load web folder:", err)
	}
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(webContent))))
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
	})

	port := getEnv("MASTER_PORT", "8080")
	addr := ":" + port

	log.Printf("Master node starting on %s", addr)
	log.Printf("Connected workers: %v", workerAddrs)
	log.Printf("GUI available at http://localhost:%s/ui/", port)

	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("Master failed to start: %v", err)
	}
}

// loggingMiddleware logs every incoming HTTP request with method, path, and remote address.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// getEnv returns the value of an environment variable or a fallback default.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
