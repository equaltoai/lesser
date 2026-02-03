#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v go >/dev/null 2>&1; then
  echo "✗ Missing required tool: go"
  exit 1
fi

mkdir -p report

if [[ -z "${GOCACHE:-}" ]]; then
  export GOCACHE="$ROOT_DIR/tmp/go-cache/$(go env GOVERSION)"
fi
mkdir -p "$GOCACHE"

if [[ -z "${GOMODCACHE:-}" ]]; then
  export GOMODCACHE="$(go env GOMODCACHE)"
fi
mkdir -p "$GOMODCACHE"

echo "=== Supply Chain Verification ==="
echo

echo "1) Local module replaces (go.mod):"
local_replaces=$(grep -nE '=>[[:space:]]*(\\./|\\../|/)' go.mod || true)
if [[ -n "${local_replaces}" ]]; then
  echo "  ✗ Local filesystem replace directives found (breaks reproducible builds):"
  echo "${local_replaces}"
  echo "  -> Use published module versions (or a temporary go.work for local dev)."
  exit 1
fi
echo "  ✓ No local replace directives found"
echo

echo "2) go mod verify:"
if go mod verify; then
  echo "  ✓ Module content matches go.sum"
else
  echo "  ✗ go mod verify failed"
  exit 1
fi
echo

echo "3) Module inventory snapshot:"
inventory="report/module_inventory.txt"
go list -m all | sort -u > "${inventory}"
go version > report/go_version.txt
echo "  ✓ Wrote ${inventory}"
echo

echo "✅ Supply chain verification passed"
