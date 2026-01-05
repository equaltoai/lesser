#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT="docs/contracts/graphql-schema.graphql"
if [[ ! -f "${OUT}" ]]; then
  echo "✗ Missing: ${OUT}"
  echo "  -> Run: lesser schema"
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

./scripts/generate_schema.sh --out "${tmp}" >/dev/null

if diff -q "${OUT}" "${tmp}" >/dev/null; then
  echo "✓ GraphQL schema contract is up to date (${OUT})"
  exit 0
fi

echo "✗ GraphQL schema contract is stale (${OUT})"
echo "  -> Run: lesser schema"
exit 1
