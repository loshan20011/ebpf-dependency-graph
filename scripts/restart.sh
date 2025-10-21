#!/usr/bin/env bash
set -euo pipefail

# Restart eBPF dependency tracker (operator + agent) and optionally microservices
# Usage:
#   scripts/restart.sh [--with-microservices] [--with-traffic]
#
# Notes:
# - Requires sudo for attaching eBPF programs
# - Logs go to logs/operator.log and logs/agent.log

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

WITH_MICROSERVICES=false
WITH_TRAFFIC=false

for arg in "$@"; do
  case "$arg" in
    --with-microservices) WITH_MICROSERVICES=true ;;
    --with-traffic) WITH_TRAFFIC=true ;;
    *) echo "Unknown arg: $arg" >&2; exit 1 ;;
  esac

done

# Stop everything
./scripts/stop-all.sh || true
sleep 1

# Build (if needed) and start operator+agent
./scripts/build.sh
sudo ./scripts/start-tracking.sh

# Optionally start demo microservices and traffic
if $WITH_MICROSERVICES; then
  ./scripts/start-microservices.sh
fi

if $WITH_TRAFFIC; then
  ./scripts/generate-traffic.sh
fi

echo
echo "✅ Restart complete."
echo "- Health:        curl http://localhost:8080/health"
echo "- Dependency UI: open http://localhost:8080/"
echo "- Logs:          tail -f logs/*.log"
