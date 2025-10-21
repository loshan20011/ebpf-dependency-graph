#!/usr/bin/env bash
set -euo pipefail

# Cleanup helper: remove logs, stale binaries, and temp files.
# Usage:
#   scripts/clean.sh              # safe clean (logs only)
#   scripts/clean.sh --deep       # also removes bin and regenerates protobuf + bpf (next build)
#   scripts/clean.sh --prune-demo # remove demo artifacts (keeps source)

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

DEEP=false
PRUNE_DEMO=false

for arg in "${@:-}"; do
  case "$arg" in
    --deep) DEEP=true ;;
    --prune-demo) PRUNE_DEMO=true ;;
    "") ;;
    *) echo "Unknown arg: $arg" >&2; exit 1 ;;
  esac
done

mkdir -p logs

echo "🧹 Cleaning logs/"
rm -f logs/*.log || true

if $DEEP; then
  echo "🧹 Removing bin/ (binaries)"
  rm -rf bin || true
  echo "🧹 Removing Go build cache (module local)"
  go clean -cache -testcache || true
fi

if $PRUNE_DEMO; then
  echo "🧹 Pruning demo caches (no source deletions)"
  rm -f **/.DS_Store || true
fi

echo "✅ Clean complete. Rebuild with: ./scripts/build.sh"
