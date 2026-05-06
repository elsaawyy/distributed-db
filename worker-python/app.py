"""
Worker-Python: A Python (Flask) implementation of a Distributed DB Worker node.

This worker:
- Receives replicated data from the Go Master node via POST /replicate
- Stores data in-memory with JSON file persistence
- Serves read requests for fault-tolerant read access
- Provides a SPECIAL TASK: data transformation & filtering endpoint (/transform)

Special Task Details:
  POST /transform  →  Applies user-defined transformations to table data:
                      - Field renaming
                      - Value type casting
                      - Row filtering by expression
                      - Field projection (select specific columns)
"""

import json
import logging
import os
import time
import threading
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from flask import Flask, jsonify, request

# ─── Logging Setup ─────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="[WORKER-PY] %(asctime)s %(levelname)s: %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("worker-python")

# ─── In-Memory Storage Engine ──────────────────────────────────────────────────

class StorageEngine:
    """Thread-safe in-memory storage engine with JSON file persistence."""

    def __init__(self, data_dir: str = "./data"):
        self._lock = threading.RLock()
        self._databases: Dict[str, dict] = {}
        self._data_dir = Path(data_dir)
        self._data_dir.mkdir(parents=True, exist_ok=True)

    def load(self):
        """Load all persisted databases from disk."""
        for json_file in self._data_dir.glob("*.json"):
            try:
                with open(json_file) as f:
                    db = json.load(f)
                self._databases[db["name"]] = db
                logger.info("Loaded database '%s' from disk", db["name"])
            except Exception as e:
                logger.warning("Could not load %s: %s", json_file, e)

    def _persist(self, db_name: str):
        """Write a database to its JSON file."""
        db = self._databases.get(db_name)
        if db is None:
            return
        path = self._data_dir / f"{db_name}.json"
        try:
            with open(path, "w") as f:
                json.dump(db, f, indent=2, default=str)
        except Exception as e:
            logger.error("Could not persist database '%s': %s", db_name, e)

    def apply_replication(self, payload: dict) -> Optional[str]:
        """
        Apply a replication payload from the master.
        Returns None on success, or an error string on failure.
        """
        op = payload.get("operation")
        db_name = payload.get("database", "")

        with self._lock:
            if op == "create_db":
                return self._create_db(db_name)
            elif op == "drop_db":
                return self._drop_db(db_name)
            elif op == "create_table":
                schema = payload.get("schema")
                if not schema:
                    return "missing schema"
                return self._create_table(db_name, schema)
            elif op == "insert":
                record = payload.get("record")
                if not record:
                    return "missing record"
                return self._insert(db_name, payload.get("table", ""), record)
            elif op == "update":
                return self._update(
                    db_name,
                    payload.get("table", ""),
                    payload.get("record_id", ""),
                    payload.get("fields", {}),
                )
            elif op == "delete":
                return self._delete(
                    db_name,
                    payload.get("table", ""),
                    payload.get("record_id", ""),
                )
            else:
                return f"unknown operation: {op}"

    def _create_db(self, name: str) -> Optional[str]:
        if name not in self._databases:
            self._databases[name] = {
                "name": name,
                "tables": {},
                "created_at": datetime.utcnow().isoformat(),
            }
            self._persist(name)
        return None  # idempotent

    def _drop_db(self, name: str) -> Optional[str]:
        self._databases.pop(name, None)
        path = self._data_dir / f"{name}.json"
        path.unlink(missing_ok=True)
        return None

    def _create_table(self, db_name: str, schema: dict) -> Optional[str]:
        db = self._databases.get(db_name)
        if db is None:
            return f"database '{db_name}' does not exist"
        table_name = schema.get("name", "")
        if table_name not in db["tables"]:
            db["tables"][table_name] = {"schema": schema, "records": {}}
            self._persist(db_name)
        return None

    def _insert(self, db_name: str, table_name: str, record: dict) -> Optional[str]:
        table = self._get_table(db_name, table_name)
        if isinstance(table, str):
            return table
        table["records"][record["id"]] = record
        self._persist(db_name)
        return None

    def _update(self, db_name: str, table_name: str, record_id: str, fields: dict) -> Optional[str]:
        table = self._get_table(db_name, table_name)
        if isinstance(table, str):
            return table
        record = table["records"].get(record_id)
        if not record:
            return f"record '{record_id}' not found"
        record["fields"].update(fields)
        record["updated_at"] = datetime.utcnow().isoformat()
        self._persist(db_name)
        return None

    def _delete(self, db_name: str, table_name: str, record_id: str) -> Optional[str]:
        table = self._get_table(db_name, table_name)
        if isinstance(table, str):
            return table
        table["records"].pop(record_id, None)
        self._persist(db_name)
        return None

    def _get_table(self, db_name: str, table_name: str):
        db = self._databases.get(db_name)
        if db is None:
            return f"database '{db_name}' does not exist"
        table = db["tables"].get(table_name)
        if table is None:
            return f"table '{table_name}' not found"
        return table

    def select(self, db_name: str, table_name: str, where: Dict[str, str]) -> tuple:
        """Returns (records_list, error_string_or_None)."""
        with self._lock:
            table = self._get_table(db_name, table_name)
            if isinstance(table, str):
                return None, table
            results = []
            for record in table["records"].values():
                if self._matches_where(record, where):
                    results.append(record)
            return results, None

    def get_all_records(self, db_name: str, table_name: str) -> tuple:
        """Returns all records in a table."""
        with self._lock:
            table = self._get_table(db_name, table_name)
            if isinstance(table, str):
                return None, table
            return list(table["records"].values()), None

    def get_schema(self, db_name: str, table_name: str) -> tuple:
        with self._lock:
            table = self._get_table(db_name, table_name)
            if isinstance(table, str):
                return None, table
            return table["schema"], None

    def list_databases(self) -> List[str]:
        with self._lock:
            return list(self._databases.keys())

    @staticmethod
    def _matches_where(record: dict, where: Dict[str, str]) -> bool:
        for key, val in where.items():
            if str(record.get("fields", {}).get(key, "")) != val:
                return False
        return True


# ─── Flask Application ─────────────────────────────────────────────────────────

app = Flask(__name__)
store = StorageEngine(os.environ.get("DATA_DIR", "./data"))


def success(data=None, message="", status=200):
    return jsonify({"success": True, "message": message, "data": data}), status


def error(msg: str, status=400):
    return jsonify({"success": False, "error": msg}), status


# ─── Replication Endpoint ──────────────────────────────────────────────────────

@app.route("/replicate", methods=["POST"])
def replicate():
    """
    Receives replication payloads from the Master node.
    Applies create_db, drop_db, create_table, insert, update, delete operations.
    """
    payload = request.get_json(force=True, silent=True)
    if not payload:
        return error("Invalid JSON body")

    logger.info("Replicating operation: %s on db: %s", payload.get("operation"), payload.get("database"))

    err = store.apply_replication(payload)
    if err:
        logger.error("Replication error: %s", err)
        return error(err, 500)

    return success(message=f"Replicated: {payload.get('operation')}")


# ─── Read Endpoint ─────────────────────────────────────────────────────────────

@app.route("/select", methods=["GET"])
def select():
    """
    Read endpoint for fault-tolerant reads when master is unavailable.
    Query params: database, table, where_<column>=<value>
    """
    db_name = request.args.get("database", "")
    table_name = request.args.get("table", "")
    if not db_name or not table_name:
        return error("'database' and 'table' query params required")

    where = {
        k[6:]: v
        for k, v in request.args.items()
        if k.startswith("where_")
    }

    records, err = store.select(db_name, table_name, where)
    if err:
        return error(err)

    return success(data=records or [])


# ─── Special Task: Data Transformation ────────────────────────────────────────

@app.route("/transform", methods=["POST"])
def transform():
    """
    SPECIAL TASK — Data Transformation Pipeline.

    This is the Python worker's unique capability: it applies a transformation
    pipeline to table data and returns the transformed result without modifying
    the stored data.

    Request body:
    {
        "database": "mydb",
        "table": "users",
        "transformations": [
            {"type": "filter",   "column": "age",  "operator": "gt", "value": "18"},
            {"type": "project",  "columns": ["name", "email"]},
            {"type": "rename",   "from": "name",   "to": "full_name"},
            {"type": "cast",     "column": "age",  "to_type": "int"},
            {"type": "sort",     "column": "full_name", "order": "asc"},
            {"type": "limit",    "count": 10}
        ]
    }

    Supported transformation types:
    - filter:  Filter rows. Operators: eq, ne, gt, lt, gte, lte, contains
    - project: Keep only specified columns
    - rename:  Rename a field
    - cast:    Cast a field to int, float, bool, or string
    - sort:    Sort rows by a column (asc/desc)
    - limit:   Return only the first N rows
    """
    body = request.get_json(force=True, silent=True)
    if not body:
        return error("Invalid JSON body")

    db_name = body.get("database", "")
    table_name = body.get("table", "")
    transformations = body.get("transformations", [])

    if not db_name or not table_name:
        return error("'database' and 'table' are required")

    records, err = store.get_all_records(db_name, table_name)
    if err:
        return error(err)

    # Work with a copy of fields for transformation
    rows = [dict(r["fields"]) for r in records]
    ids = [r["id"] for r in records]

    # Attach IDs so they survive projections
    for i, row in enumerate(rows):
        row["__id"] = ids[i]

    # Apply transformations in order
    for t in transformations:
        t_type = t.get("type")
        try:
            if t_type == "filter":
                rows = _apply_filter(rows, t)
            elif t_type == "project":
                rows = _apply_project(rows, t)
            elif t_type == "rename":
                rows = _apply_rename(rows, t)
            elif t_type == "cast":
                rows = _apply_cast(rows, t)
            elif t_type == "sort":
                rows = _apply_sort(rows, t)
            elif t_type == "limit":
                rows = rows[:int(t.get("count", len(rows)))]
            else:
                return error(f"Unknown transformation type: {t_type}")
        except Exception as e:
            return error(f"Transformation '{t_type}' failed: {e}")

    logger.info(
        "Transform on %s.%s: %d transformations, %d rows returned",
        db_name, table_name, len(transformations), len(rows)
    )

    return success(
        data={
            "database": db_name,
            "table": table_name,
            "transformations_applied": len(transformations),
            "row_count": len(rows),
            "rows": rows,
        },
        message="Transformation completed by Python Worker",
    )


def _apply_filter(rows: list, t: dict) -> list:
    col = t.get("column")
    op = t.get("operator", "eq")
    val = t.get("value", "")

    def cast_val(v):
        try:
            return float(v)
        except (ValueError, TypeError):
            return v

    def matches(row):
        cell = row.get(col)
        if cell is None:
            return False
        if op == "eq":
            return str(cell) == str(val)
        elif op == "ne":
            return str(cell) != str(val)
        elif op == "gt":
            return cast_val(cell) > cast_val(val)
        elif op == "lt":
            return cast_val(cell) < cast_val(val)
        elif op == "gte":
            return cast_val(cell) >= cast_val(val)
        elif op == "lte":
            return cast_val(cell) <= cast_val(val)
        elif op == "contains":
            return str(val).lower() in str(cell).lower()
        return False

    return [r for r in rows if matches(r)]


def _apply_project(rows: list, t: dict) -> list:
    cols = set(t.get("columns", []))
    cols.add("__id")  # always keep ID
    return [{k: v for k, v in row.items() if k in cols} for row in rows]


def _apply_rename(rows: list, t: dict) -> list:
    src = t.get("from")
    dst = t.get("to")
    result = []
    for row in rows:
        new_row = {}
        for k, v in row.items():
            new_row[dst if k == src else k] = v
        result.append(new_row)
    return result


def _apply_cast(rows: list, t: dict) -> list:
    col = t.get("column")
    to_type = t.get("to_type", "string")
    cast_fn = {"int": int, "float": float, "bool": bool, "string": str}.get(to_type, str)
    for row in rows:
        if col in row:
            try:
                row[col] = cast_fn(row[col])
            except (ValueError, TypeError):
                pass
    return rows


def _apply_sort(rows: list, t: dict) -> list:
    col = t.get("column")
    reverse = t.get("order", "asc") == "desc"
    return sorted(rows, key=lambda r: (r.get(col) is None, r.get(col, "")), reverse=reverse)


# ─── Health & Status ───────────────────────────────────────────────────────────

@app.route("/health", methods=["GET"])
def health():
    return success(
        data={"node": "worker-python", "time": datetime.utcnow().isoformat()},
        message="Python Worker is healthy",
    )


@app.route("/status", methods=["GET"])
def status():
    return success(
        data={"node": "worker-python", "databases": store.list_databases()}
    )


# ─── Entry Point ───────────────────────────────────────────────────────────────

if __name__ == "__main__":
    store.load()
    port = int(os.environ.get("WORKER_PY_PORT", 8082))
    logger.info("Python Worker starting on port %d", port)
    app.run(host="0.0.0.0", port=port, debug=False)
