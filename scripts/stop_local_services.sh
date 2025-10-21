#!/bin/bash

echo "🛑 Stopping Local Microservices"
echo "==============================="

# Kill nginx processes
echo "🔪 Stopping nginx services..."
pkill -f "nginx.*tmp/nginx-" || true

# Clean up PIDs file
if [ -f /tmp/microservices.pids ]; then
    echo "🧹 Cleaning up PID files..."
    while IFS= read -r pid; do
        kill "$pid" 2>/dev/null || true
    done < /tmp/microservices.pids
    rm -f /tmp/microservices.pids
fi

# Clean up config directories
echo "🗂️ Cleaning up config directories..."
rm -rf /tmp/nginx-*

sleep 2

echo "✅ All local microservices stopped and cleaned up!"