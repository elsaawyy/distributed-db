#!/bin/bash
# stop-all.sh — Stop all Distributed DB nodes

if [ -f /tmp/distributed-db.pids ]; then
    PIDS=$(cat /tmp/distributed-db.pids)
    echo "Stopping PIDs: $PIDS"
    kill $PIDS 2>/dev/null && echo "All nodes stopped." || echo "Some processes may have already exited."
    rm /tmp/distributed-db.pids
else
    # Fallback: kill by binary name
    pkill -f master-bin 2>/dev/null
    pkill -f worker-go-bin 2>/dev/null
    pkill -f "python3 app.py" 2>/dev/null
    echo "Stopped all distributed-db processes."
fi
