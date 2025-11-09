#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CORE="$ROOT_DIR/graph/core.graphql"
PHASE2="$ROOT_DIR/graph/phase2.graphql"
PHASE3="$ROOT_DIR/graph/phase3.graphql"
OUTPUT="$ROOT_DIR/graph/schema.graphql"

if [[ ! -f "$CORE" ]]; then
  echo "Core schema not found at $CORE" >&2
  exit 1
fi

{
  echo "# DO NOT EDIT: aggregated schema for client consumption"
  echo "# Generated $(date -u '+%Y-%m-%dT%H:%M:%SZ') via scripts/generate_schema.sh"
  echo
  cat "$CORE"
  echo
  cat "$PHASE2"
  echo
  cat "$PHASE3"
} > "$OUTPUT"

echo "Wrote aggregated schema to $OUTPUT"
