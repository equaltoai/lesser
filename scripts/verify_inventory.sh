#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -z "${GOCACHE:-}" ]]; then
  export GOCACHE="$ROOT_DIR/tmp/go-cache"
fi
mkdir -p "$GOCACHE"

contains() {
  local needle="$1"; shift
  for item in "$@"; do
    [[ "$item" == "$needle" ]] && return 0
  done
  return 1
}

readarray -t MAKEFILE_LAMBDAS < <(awk '
  $0 ~ /^LAMBDAS :=/ { in_lambdas=1; next }
  in_lambdas {
    if ($0 ~ /^[[:space:]]*$/) { exit }
    gsub(/\\/, "", $0)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0)
    if ($0 != "") print $0
  }
' Makefile)

readarray -t INVENTORY_LAMBDAS < <(awk -F'"' '/Name:[[:space:]]*"/ {print $2}' infra/cdk/inventory/lambdas.go | sort)

missing_in_inventory=()
extra_in_inventory=()

for lambda in "${MAKEFILE_LAMBDAS[@]}"; do
  if ! contains "$lambda" "${INVENTORY_LAMBDAS[@]}"; then
    missing_in_inventory+=("$lambda")
  fi
done

for lambda in "${INVENTORY_LAMBDAS[@]}"; do
  if ! contains "$lambda" "${MAKEFILE_LAMBDAS[@]}"; then
    extra_in_inventory+=("$lambda")
  fi
done

echo "=== Inventory Drift Verification ==="
echo
echo "1) Set equality (Makefile LAMBDAS vs inventory.LambdaInventory):"
if [ ${#missing_in_inventory[@]} -eq 0 ] && [ ${#extra_in_inventory[@]} -eq 0 ]; then
  echo "  ✓ Sets match"
else
  [ ${#missing_in_inventory[@]} -gt 0 ] && printf '  ✗ Missing in inventory: %s\n' "${missing_in_inventory[*]}"
  [ ${#extra_in_inventory[@]} -gt 0 ] && printf '  ✗ Extra in inventory: %s\n' "${extra_in_inventory[*]}"
fi
echo

echo "2) docs/specs/01-lambda-inventory-matrix.md freshness:"
doc_status=0
if cd "$ROOT_DIR/infra/cdk" && go run ./cmd/generate-inventory -check; then
  echo "  ✓ Doc is up to date with generator output"
else
  echo "  ✗ Doc is stale: run 'cd infra/cdk && go run ./cmd/generate-inventory'"
  doc_status=1
fi
echo

drift=0
[ ${#missing_in_inventory[@]} -gt 0 ] && drift=1
[ ${#extra_in_inventory[@]} -gt 0 ] && drift=1
[ $doc_status -ne 0 ] && drift=1

if [ $drift -eq 0 ]; then
  echo "✅ Inventory drift check passed"
  exit 0
fi

echo "❌ Inventory drift detected. Resolve issues above."
exit 1
