#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# start-all.sh — Start Master + Worker-Go + Worker-Python in separate terminals
# Usage: ./start-all.sh
# ─────────────────────────────────────────────────────────────────────────────

set -e

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "╔══════════════════════════════════════════════════════╗"
echo "║     Distributed Database System — Starting Nodes     ║"
echo "╚══════════════════════════════════════════════════════╝"

# ─── Worker-Python ────────────────────────────────────────────────────────────
echo ""
echo "▶ Starting Worker-Python (port 8082)..."
cd "$ROOT_DIR/worker-python"
if [ ! -d "venv" ]; then
    python3 -m venv venv
    echo "  Created virtualenv"
fi
source venv/bin/activate
pip install -r requirements.txt -q
mkdir -p data
WORKER_PY_PORT=8082 nohup python3 app.py > /tmp/worker-python.log 2>&1 &
WPID=$!
echo "  Worker-Python PID: $WPID  →  logs: /tmp/worker-python.log"
deactivate

# ─── Worker-Go ────────────────────────────────────────────────────────────────
echo ""
echo "▶ Starting Worker-Go (port 8081)..."
cd "$ROOT_DIR/worker-go"
mkdir -p data
go build -o worker-go-bin . 2>/dev/null || { echo "  ERROR: Go build failed for worker-go"; exit 1; }
WORKER_GO_PORT=8081 nohup ./worker-go-bin > /tmp/worker-go.log 2>&1 &
WGPID=$!
echo "  Worker-Go PID: $WGPID  →  logs: /tmp/worker-go.log"

# ─── Master ───────────────────────────────────────────────────────────────────
echo ""
echo "▶ Starting Master (port 8080)..."
cd "$ROOT_DIR/master"
mkdir -p data
go build -o master-bin . 2>/dev/null || { echo "  ERROR: Go build failed for master"; exit 1; }
MASTER_PORT=8080 \
WORKER_GO_ADDR=http://localhost:8081 \
WORKER_PY_ADDR=http://localhost:8082 \
nohup ./master-bin > /tmp/master.log 2>&1 &
MPID=$!
echo "  Master PID: $MPID  →  logs: /tmp/master.log"

echo ""
echo "─────────────────────────────────────────────────────────"
echo "  All nodes started. Wait 2 seconds then run:"
echo "  curl http://localhost:8080/health"
echo "  curl http://localhost:8081/health"
echo "  curl http://localhost:8082/health"
echo ""
echo "  To stop all: kill $MPID $WGPID $WPID"
echo "─────────────────────────────────────────────────────────"

# Save PIDs for stop script
echo "$MPID $WGPID $WPID" > /tmp/distributed-db.pids
