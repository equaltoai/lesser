#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p tmp/go-cache
GOCACHE="$ROOT_DIR/tmp/go-cache" go run ./tools/ai_training_verify
