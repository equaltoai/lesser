#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE_DIR="$ROOT_DIR/tmp/go-cache/$(go env GOVERSION)"
mkdir -p "$GOCACHE_DIR"
GOCACHE="$GOCACHE_DIR" go run ./tools/ai_training_verify
