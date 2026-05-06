# Distributed Database System (Go + Python)

> An academic distributed database with a Go Master node, a Go Worker node, and a Python Worker node — all communicating over HTTP with synchronous replication, fault-tolerant reads, and heterogeneous special-task processing.

---

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Running the System](#running-the-system)
- [API Reference](#api-reference)
- [Replication Explained](#replication-explained)
- [Fault Tolerance](#fault-tolerance)
- [Special Tasks](#special-tasks)
- [Example Curl Requests](#example-curl-requests)
- [Report](#report)

---

## Overview

This project implements a distributed database system from scratch using:

| Node          | Technology | Port | Role                                        |
|---------------|------------|------|---------------------------------------------|
| **Master**    | Go         | 8080 | Schema management, query coordination, replication orchestrator |
| **Worker-Go** | Go         | 8081 | Replicated storage, read-fallback, analytics |
| **Worker-Py** | Python/Flask | 8082 | Replicated storage, read-fallback, data transformation |

**Key properties:**
- Synchronous replication: every write on the master is forwarded to all workers before returning
- Fault-tolerant reads: clients can query workers directly if the master is down
- Heterogeneous workers: Go and Python nodes coexist, each offering a unique "special task"
- File-backed persistence: JSON files on disk survive node restarts
- Dynamic schemas: create tables with arbitrary column definitions at runtime

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          CLIENT                                 │
│        (curl / Postman / application)                           │
└────────────────────────┬────────────────────────────────────────┘
                         │ REST API (HTTP)
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      MASTER NODE  :8080                         │
│                                                                 │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────┐   │
│  │  HTTP        │  │  Storage     │  │  Replication        │   │
│  │  Handlers   │  │  Engine      │  │  Manager            │   │
│  │             │  │  (in-memory  │  │                     │   │
│  │ /create-db  │  │   + JSON     │  │ ● Health loop       │   │
│  │ /create-    │  │   files)     │  │ ● Fanout goroutines │   │
│  │   table     │  │              │  │ ● Worker status     │   │
│  │ /insert     │  └──────────────┘  └────────┬────────────┘   │
│  │ /select     │                              │                 │
│  │ /update     │                              │ POST /replicate │
│  │ /delete     │                              │                 │
│  │ /search     │                              │                 │
│  │ /drop-db    │                              │                 │
│  └─────────────┘                              │                 │
└──────────────────────────────────────────────┬─────────────────┘
                                               │
                         ┌─────────────────────┼──────────────────┐
                         │                     │                  │
                         ▼                     ▼                  │
          ┌──────────────────────┐  ┌──────────────────────┐     │
          │  WORKER-GO   :8081   │  │  WORKER-PY   :8082   │     │
          │                      │  │                      │     │
          │  /replicate          │  │  /replicate          │     │
          │  /select    ◄── fault-tolerant reads ──►       │     │
          │  /analytics          │  │  /transform          │     │
          │  /health             │  │  /health             │     │
          │  /status             │  │  /status             │     │
          │                      │  │                      │     │
          │  Storage:            │  │  Storage:            │     │
          │  ./data/*.json       │  │  ./data/*.json       │     │
          └──────────────────────┘  └──────────────────────┘     │
                                                                  │
          ◄───── Replication Flow (Master → Workers) ────────────►│
                                                                  │
          ◄───── Fault-Tolerant Reads (Client → Workers) ────────►│
```

### Data Flow for a Write Request (e.g., INSERT)

```
Client → POST /insert → Master
    │
    ├── 1. Validate request
    ├── 2. Write to Master storage
    ├── 3. Replicate to Worker-Go  (goroutine, POST /replicate)
    ├── 4. Replicate to Worker-Py  (goroutine, POST /replicate)
    └── 5. Return success to Client
```

---

## Project Structure

```
distributed-db/
│
├── master/                         # Master node (Go)
│   ├── main.go                     # Entry point, route registration
│   ├── go.mod
│   ├── handlers/
│   │   └── handlers.go             # HTTP handlers for all REST endpoints
│   ├── storage/
│   │   └── engine.go               # In-memory + JSON storage engine
│   ├── replication/
│   │   └── manager.go              # Replication manager, health loop
│   └── models/
│       └── models.go               # Shared data types and DTOs
│
├── worker-go/                      # Go Worker node
│   ├── main.go
│   ├── go.mod
│   ├── handlers/
│   │   └── handlers.go             # Replicate, select, analytics, health
│   ├── storage/
│   │   └── engine.go               # Worker storage engine
│   └── models/
│       └── models.go
│
├── worker-python/                  # Python Worker node
│   ├── app.py                      # Flask app — all endpoints in one file
│   └── requirements.txt
│
├── shared/
│   └── contracts.go                # Cross-node contract documentation
│
├── docs/
│   └── report.md                   # 4-page academic report
│
├── start-all.sh                    # Start all 3 nodes
├── stop-all.sh                     # Stop all nodes
├── demo.sh                         # Full end-to-end demo
└── README.md
```

---

## Prerequisites

| Tool      | Minimum Version | Purpose             |
|-----------|-----------------|---------------------|
| Go        | 1.21            | Master + Worker-Go  |
| Python    | 3.9             | Worker-Python       |
| pip       | any             | Python deps         |
| bash      | any             | Run scripts         |

Install Go: https://go.dev/dl/  
Install Python: https://www.python.org/downloads/

---

## Running the System

### Option A — All-in-One Script

```bash
chmod +x start-all.sh stop-all.sh demo.sh
./start-all.sh
```

This builds and starts all 3 nodes in the background. Logs go to `/tmp/master.log`, `/tmp/worker-go.log`, `/tmp/worker-python.log`.

### Option B — Manual (3 terminals)

**Terminal 1 — Worker-Python:**
```bash
cd worker-python
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
mkdir -p data
WORKER_PY_PORT=8082 python3 app.py
```

**Terminal 2 — Worker-Go:**
```bash
cd worker-go
mkdir -p data
go run .
# or: go build -o worker-go-bin . && WORKER_GO_PORT=8081 ./worker-go-bin
```

**Terminal 3 — Master:**
```bash
cd master
mkdir -p data
MASTER_PORT=8080 \
WORKER_GO_ADDR=http://localhost:8081 \
WORKER_PY_ADDR=http://localhost:8082 \
go run .
```

### Verify All Nodes Are Up

```bash
curl http://localhost:8080/health
curl http://localhost:8081/health
curl http://localhost:8082/health
```

### Run the Demo

```bash
./demo.sh
```

---

## API Reference

### Master Node (`:8080`)

| Method | Endpoint       | Description                     |
|--------|---------------|---------------------------------|
| POST   | /create-db    | Create a new database           |
| DELETE | /drop-db      | Drop a database (master only)   |
| POST   | /create-table | Create a table with schema      |
| POST   | /insert       | Insert a record                 |
| GET    | /select       | Select records (optional filter)|
| PUT    | /update       | Update a record by ID           |
| DELETE | /delete       | Delete a record by ID           |
| GET    | /search       | Full-text search on a column    |
| GET    | /health       | Liveness check                  |
| GET    | /status       | System status + worker states   |

### Worker-Go Node (`:8081`)

| Method | Endpoint     | Description                            |
|--------|-------------|----------------------------------------|
| POST   | /replicate  | Receive replication from master        |
| GET    | /select     | Direct read (fault-tolerant)           |
| GET    | /analytics  | **Special Task**: column statistics    |
| GET    | /health     | Liveness check                         |
| GET    | /status     | Node status                            |

### Worker-Python Node (`:8082`)

| Method | Endpoint     | Description                                |
|--------|-------------|--------------------------------------------|
| POST   | /replicate  | Receive replication from master            |
| GET    | /select     | Direct read (fault-tolerant)               |
| POST   | /transform  | **Special Task**: data transformation pipeline |
| GET    | /health     | Liveness check                             |
| GET    | /status     | Node status                                |

---

## Example Curl Requests

### Create a database
```bash
curl -X POST http://localhost:8080/create-db \
  -H "Content-Type: application/json" \
  -d '{"name": "school"}'
```

### Create a table
```bash
curl -X POST http://localhost:8080/create-table \
  -H "Content-Type: application/json" \
  -d '{
    "database": "school",
    "table": "students",
    "columns": [
      {"name": "name",   "type": "string", "required": true},
      {"name": "grade",  "type": "string", "required": true},
      {"name": "score",  "type": "float",  "required": false},
      {"name": "active", "type": "bool",   "required": false}
    ]
  }'
```

### Insert a record
```bash
curl -X POST http://localhost:8080/insert \
  -H "Content-Type: application/json" \
  -d '{
    "database": "school",
    "table": "students",
    "fields": {"name": "Alice", "grade": "A", "score": 95.5, "active": true}
  }'
```

### Select all records
```bash
curl "http://localhost:8080/select?database=school&table=students"
```

### Select with WHERE filter
```bash
curl "http://localhost:8080/select?database=school&table=students&where_grade=A"
```

### Update a record
```bash
curl -X PUT http://localhost:8080/update \
  -H "Content-Type: application/json" \
  -d '{"database":"school","table":"students","id":"<RECORD_ID>","fields":{"score":98.0}}'
```

### Search (substring match)
```bash
curl "http://localhost:8080/search?database=school&table=students&column=name&value=Ali"
```

### Delete a record
```bash
curl -X DELETE http://localhost:8080/delete \
  -H "Content-Type: application/json" \
  -d '{"database":"school","table":"students","id":"<RECORD_ID>"}'
```

### Drop a database
```bash
curl -X DELETE http://localhost:8080/drop-db \
  -H "Content-Type: application/json" \
  -d '{"name": "school"}'
```

### Go Worker — Analytics (Special Task)
```bash
curl "http://localhost:8081/analytics?database=school&table=students"
```

### Python Worker — Transform (Special Task)
```bash
curl -X POST http://localhost:8082/transform \
  -H "Content-Type: application/json" \
  -d '{
    "database": "school",
    "table": "students",
    "transformations": [
      {"type": "filter",  "column": "grade",  "operator": "eq",   "value": "A"},
      {"type": "project", "columns": ["name", "score"]},
      {"type": "cast",    "column": "score",  "to_type": "float"},
      {"type": "sort",    "column": "score",  "order": "desc"},
      {"type": "limit",   "count": 5}
    ]
  }'
```

### Fault-Tolerant Read (directly from worker)
```bash
# Read from Go worker when master is unavailable
curl "http://localhost:8081/select?database=school&table=students"

# Read from Python worker
curl "http://localhost:8082/select?database=school&table=students"
```

---

## Replication Explained

### How It Works

Every write operation (create_db, drop_db, create_table, insert, update, delete) on the Master triggers synchronous replication to all registered workers **before** the response is returned to the client.

```
Master receives write → applies locally → fans out to workers (goroutines) → waits for all → responds
```

### Implementation Details

- The `replication.Manager` holds a list of `WorkerStatus` structs, one per worker
- On each write, the manager spawns one goroutine per worker and uses `sync.WaitGroup` to wait
- Each goroutine POSTs a `ReplicationPayload` JSON to `POST /replicate` on the worker
- Workers apply the payload via `ApplyReplication()`, which is a switch on the `operation` field
- Workers are **idempotent** for create operations (duplicate creates are silently ignored)

### Payload Format

```json
{
  "operation": "insert",
  "database": "school",
  "table": "students",
  "record": {
    "id": "1714567890123456789",
    "fields": {"name": "Alice", "grade": "A", "score": 95.5},
    "created_at": "2025-05-06T10:00:00Z",
    "updated_at": "2025-05-06T10:00:00Z"
  }
}
```

---

## Fault Tolerance

### Scenario: Master Node Fails

1. **Reads still work** — clients can send `GET /select` directly to `:8081` (Go Worker) or `:8082` (Python Worker). Both nodes hold a complete replica of all data.
2. **Writes are unavailable** — no new writes are accepted (no leader election in this implementation). This is by design for academic scope.

### Scenario: A Worker Node Fails

1. The `replication.Manager` runs a background goroutine that pings each worker's `/health` every 10 seconds.
2. If a worker returns an error or is unreachable, it is marked `alive: false`.
3. Future replication calls skip dead workers (logged as warnings).
4. When the worker comes back online, the health loop detects the recovery and marks it `alive: true`.
5. **Note:** There is no catch-up replication in this implementation. Data written while a worker was down will be missing on that worker. For production, you would add a WAL (Write-Ahead Log) replay mechanism.

### Check Worker Health
```bash
curl http://localhost:8080/status
```
This returns the master's view of all workers including `alive` status and `last_error`.

---

## Special Tasks

### Go Worker — Analytics

**Endpoint:** `GET /analytics?database=<db>&table=<table>`

Computes column-level statistics without transmitting all data to the client:
- Total row count
- Per-column: non-null count, unique value count, sample values (up to 5)

Use case: quick data profiling, dashboard summaries.

### Python Worker — Transformation Pipeline

**Endpoint:** `POST /transform`

Applies an ordered pipeline of transformations to table data **in memory** (non-destructive):

| Transform | Description                                    |
|-----------|------------------------------------------------|
| `filter`  | Keep rows matching a condition (eq/ne/gt/lt/contains) |
| `project` | Keep only specified columns                    |
| `rename`  | Rename a column                                |
| `cast`    | Convert a column's values to int/float/bool/string |
| `sort`    | Sort rows by a column asc/desc                 |
| `limit`   | Return only the first N rows                   |

Use case: ETL pipelines, reporting, data preparation without modifying source data.

---

## Environment Variables

| Variable         | Default                   | Description                    |
|------------------|---------------------------|--------------------------------|
| `MASTER_PORT`    | `8080`                    | Master HTTP port               |
| `WORKER_GO_ADDR` | `http://localhost:8081`   | Go Worker address (master cfg) |
| `WORKER_PY_ADDR` | `http://localhost:8082`   | Py Worker address (master cfg) |
| `WORKER_GO_PORT` | `8081`                    | Go Worker HTTP port            |
| `WORKER_PY_PORT` | `8082`                    | Python Worker HTTP port        |
| `DATA_DIR`       | `./data`                  | Python Worker data directory   |
