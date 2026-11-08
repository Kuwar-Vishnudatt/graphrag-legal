#!/usr/bin/env bash
# Runs the same query at 0 hops (effectively chunk-based top-k retrieval
# — the seed section only) and at 1 hop (real graph traversal), side by
# side, so the difference GraphRAG makes is visible directly rather than
# only provable in a test assertion.
set -euo pipefail
cd "$(dirname "$0")/.."

QUERY="What is the data retention period for this regulation, and does it apply to a hospital's patient health records?"

echo "=== 0 hops (seed section only — what chunk-based top-k retrieval would give you) ==="
./bin/graphrag -query "$QUERY" -hops 0
echo
echo "=== 1 hop (real graph traversal: reaches the healthcare exception + its definitions) ==="
./bin/graphrag -query "$QUERY" -hops 1
