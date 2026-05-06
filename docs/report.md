# Distributed Database System Using Go
## Academic Technical Report

**Course:** Distributed Systems  
**Project:** Distributed Database Implementation  
**Language Stack:** Go (Master + Worker-1), Python/Flask (Worker-2)  
**Date:** May 2025  

---

## 1. Introduction

Distributed database systems form the backbone of modern large-scale applications, from social media platforms handling billions of records to financial systems requiring near-zero downtime. Unlike traditional single-node databases, a distributed database spreads data and computation across multiple independent nodes connected over a network, offering horizontal scalability, improved fault tolerance, and geographic flexibility.

This project implements a functional distributed database system as an academic exploration of the core principles: data replication, schema management, query execution, and fault detection. The system is intentionally constrained in scope — it forgoes consensus algorithms (Raft, Paxos) and advanced recovery protocols in favor of clear, readable implementations of the fundamental mechanisms. The result is a working three-node distributed database where each node is a self-contained HTTP service, communicating entirely over REST APIs.

The choice of Go as the primary language was deliberate. Go's built-in concurrency primitives (goroutines, `sync.WaitGroup`, `sync.RWMutex`) make it natural to express concurrent replication and background health checks, while its static typing and simple module system produce clean, deployable binaries. The inclusion of a Python worker node demonstrates heterogeneous node interoperability — a defining characteristic of modern distributed systems where different teams may own different services.

---

## 2. System Architecture

### 2.1 Node Roles

The system follows a **master-worker (primary-replica)** topology, the most common pattern in distributed databases:

**Master Node (Go, port 8080)**  
The master is the sole entry point for all client writes. It owns schema operations (creating and dropping databases and tables), validates incoming data against schemas, and coordinates replication. The master maintains authoritative state and is the source of truth.

**Worker-Go Node (Go, port 8081)**  
A replica node implemented in Go. It receives replicated writes from the master over HTTP, stores them in an identical format, and exposes the same query interface for reads. It additionally provides an analytics endpoint — its "special task" — that computes column-level statistics (unique value counts, non-null counts, sample values) without moving all data to the client.

**Worker-Python Node (Python/Flask, port 8082)**  
A replica node implemented in Python. It participates in the same replication protocol as the Go worker, but its special task is a **data transformation pipeline**: clients can request non-destructive, in-memory transformations (filter, project, rename, cast, sort, limit) on any stored table. This is useful for ETL scenarios and ad-hoc reporting.

### 2.2 Communication Protocol

All inter-node communication uses HTTP/1.1 with JSON-encoded bodies. This choice was made for several reasons:

- **Universality:** Any language can speak HTTP, making heterogeneous nodes straightforward
- **Debuggability:** Requests are human-readable and can be inspected with standard tools (curl, Postman)
- **Simplicity:** No custom binary protocol needed; the standard library suffices in both Go and Python

Replication uses a dedicated `POST /replicate` endpoint on each worker. The payload contains an `operation` field discriminating the type of write, along with all data needed to apply it deterministically.

### 2.3 Storage Engine

Both the Go master and the Go worker share the same storage architecture: an in-memory map of databases, protected by a `sync.RWMutex` for concurrent access, with every mutation immediately persisted to a JSON file on disk. The JSON file serves as a write-ahead log replacement — on startup, all files are read back into memory, enabling crash recovery.

The Python worker uses an equivalent design: a `threading.RLock`-protected dictionary with immediate JSON file persistence via Python's `json.dump`.

Data model hierarchy: **Database → Table → Record**. Each record carries a nanosecond-precision ID generated at insert time on the master, ensuring global uniqueness without a separate ID sequence. Tables have an explicit schema stored alongside their records, enabling field validation on insert.

---

## 3. Design Decisions

### 3.1 Synchronous vs. Asynchronous Replication

A central decision in any distributed database is whether replication should be synchronous (master waits for worker acknowledgment before responding to client) or asynchronous (master responds immediately, workers catch up in the background).

This system implements **synchronous replication**. When the master processes a write, it fans out to all alive workers using goroutines and waits for all of them to complete before returning to the client. The consequences:

- **Consistency:** At the time the client receives a success response, all alive nodes hold the write. This provides strong consistency for reads immediately after writes.
- **Latency:** Write latency is bounded by the slowest responding worker. In a LAN environment this is negligible, but over a WAN it becomes a bottleneck.
- **Availability trade-off:** If a worker is slow, writes slow down. This is mitigated by a 5-second timeout on the HTTP client used for replication.

Asynchronous replication would improve write latency and availability but introduce the risk of serving stale reads from workers. For an academic system emphasizing correctness, synchronous replication is the appropriate choice.

### 3.2 Partial Failure Handling

What should happen when a write succeeds on the master but fails on one worker? This project takes a **best-effort approach**: the master logs the failure, marks the worker as `alive: false`, and returns success to the client. The successful write on the master and surviving workers is not rolled back.

This means a recovered worker will have **stale data** — records written while it was down are absent. A production system would address this with:
- A write-ahead log (WAL) that workers replay after reconnecting
- A version counter (LSN — Log Sequence Number) that workers use to request missing entries
- Or a consensus algorithm (Raft) that prevents any write from committing unless a majority of nodes acknowledge it

For academic scope, the simple approach demonstrates the core problem clearly without requiring a full consensus implementation.

### 3.3 Schema Enforcement

Tables have named, typed columns with an optional `required` flag. On every INSERT, the master validates:
1. All required columns are present
2. No unknown columns are provided

Type coercion is left to the client (JSON numbers are stored as-is). This lightweight schema serves as a contract between clients and the database, preventing obvious data quality issues without the complexity of a full type system.

### 3.4 Modular Package Structure

Each Go node is organized into three packages: `handlers` (HTTP layer), `storage` (data engine), and `models` (shared types). This separation enforces a clean dependency direction: handlers depend on storage and models; storage depends only on models; models have no dependencies. This makes each layer independently testable.

---

## 4. Challenges Faced

### 4.1 Concurrent Replication and Race Conditions

The first non-trivial challenge was ensuring that concurrent writes from multiple clients don't produce inconsistent state. The storage engine uses a `sync.RWMutex` with fine-grained locking: read operations (SELECT, SEARCH) acquire a read lock allowing concurrency; write operations (INSERT, UPDATE, DELETE) acquire an exclusive write lock. Special care was taken to ensure the lock is always released — the `defer` keyword in Go makes this natural.

Replication itself is concurrent: the master fans out to all workers simultaneously using goroutines. A `sync.WaitGroup` synchronizes the completion of all goroutine sends before the function returns.

### 4.2 Cross-Language Replication

Making the Go master replicate to a Python worker required careful attention to JSON serialization. Go's `time.Time` type serializes to RFC3339 format by default; Python's `datetime.utcnow().isoformat()` produces a similar but not identical string. Both workers store timestamps as strings, avoiding parse errors.

JSON number handling also required attention: Go marshals `float64` values as floating-point numbers, which Python receives correctly via `json.load`. Record IDs are serialized as strings (nanosecond Unix timestamps) to avoid integer overflow in JavaScript clients.

### 4.3 Idempotent Replication

Network failures can cause replication payloads to be delivered more than once (the master retries or the network duplicates the packet). To handle this safely, all `create_db` and `create_table` operations on workers are idempotent — if the resource already exists, the operation silently succeeds. INSERT operations use IDs generated by the master, so a duplicate insert simply overwrites the same record with identical data.

### 4.4 Python Worker Health During Startup

A subtle issue arose during startup: the master begins pinging workers for health 10 seconds after startup, but the Python worker may not have fully initialized. The health loop's `GET /health` endpoint in Flask is registered before `app.run()` is called, so Flask is not yet accepting connections during early initialization. The `start-all.sh` script starts workers before the master and allows a brief settling period to mitigate this.

---

## 5. Future Improvements

### 5.1 Raft-Based Leader Election

The most significant limitation of the current system is the single point of failure at the master. If the master crashes, no new writes are possible. Implementing the Raft consensus algorithm would allow workers to elect a new leader automatically, enabling full write availability during master failures.

### 5.2 Write-Ahead Log (WAL) and Log Replication

Currently, a worker that was offline during writes will have stale data upon recovery. Introducing a WAL on the master — an append-only log of all operations — would allow workers to replay missed entries by fetching them from the master after reconnection. Each worker would store its last-applied log sequence number (LSN).

### 5.3 Query Language (SQL Subset)

The current system uses a REST/JSON interface for all operations. Adding a basic SQL parser (even supporting only SELECT, INSERT, UPDATE, DELETE) would make the system usable with standard database clients and demonstrate a classic systems engineering challenge: parsing, query planning, and execution.

### 5.4 Transactions and MVCC

The current system has no transactional semantics. Two concurrent updates to the same record can produce a non-deterministic result. Implementing Multi-Version Concurrency Control (MVCC) — maintaining multiple versions of each record and resolving conflicts at commit time — would bring the system closer to a production database.

### 5.5 Sharding and Horizontal Scalability

Currently all nodes hold all data (full replication). Sharding would partition data across workers by key range or hash, with each worker responsible for a subset of records. The master would act as a router, forwarding queries to the appropriate shard. This would enable the system to scale storage beyond what any single machine can hold.

### 5.6 Authentication and TLS

All inter-node communication is currently unauthenticated plaintext HTTP. Production deployment would require mutual TLS (mTLS) between nodes and API key or JWT authentication for client requests.

---

## 6. Conclusion

This project demonstrates the core concepts of a distributed database system: schema management, record-level CRUD operations, synchronous replication, and basic fault detection. The Master-Worker topology is one of the most widely deployed architectures in the industry (used by MySQL replication, Redis Sentinel, and MongoDB replica sets), and implementing it from scratch — even in simplified form — provides concrete insight into the trade-offs every database engineer faces.

The heterogeneous worker design (Go + Python) highlights that distributed systems are fundamentally about protocols, not implementations: any node that correctly speaks the replication protocol can participate in the cluster, regardless of language or framework. This is the same principle that allows real-world databases like Kafka to have clients in a dozen languages, or cloud providers to expose APIs consumed by clients written in any language.

The extensions outlined above (Raft, WAL, SQL, MVCC, sharding) represent the progression from an academic prototype to a production system — a roadmap that mirrors the actual history of systems like CockroachDB and TiDB, which began as academic papers and grew into full distributed SQL databases.

---

*End of Report*
