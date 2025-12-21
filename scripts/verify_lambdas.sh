#!/bin/bash

# Compatibility wrapper; canonical logic lives in scripts/verify_lambda_set.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

exec "$ROOT_DIR/scripts/verify_lambda_set.sh" "$@"
