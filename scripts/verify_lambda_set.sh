#!/usr/bin/env bash

# verify_lambda_set.sh
# Usage: scripts/verify_lambda_set.sh
# Purpose:
#   - Extract Makefile LAMBDAS
#   - Ensure each lambda has cmd/<name>/main.go
#   - Detect extra cmd handlers not listed in Makefile (excluding known tooling/infra)
#   - If bin/*.zip exist, ensure they include Makefile LAMBDAS (excluding known tooling/infra)
# Exit codes:
#   0 = no drift detected
#   1 = drift detected (see summary)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ALLOW_EXTRA_CMD=(
  cloudfront-keygen
  configure-instance
  init-deploy
)

ALLOW_EXTRA_ZIPS=(
  cloudfront-keygen
)

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

readarray -t CMD_LAMBDAS < <(for dir in cmd/*; do
  [ -d "$dir" ] || continue
  name="$(basename "$dir")"
  if [ -f "$dir/main.go" ]; then
    printf '%s\n' "$name"
  fi
done | sort)

shopt -s nullglob
BIN_ZIPS=()
for zip in bin/*.zip; do
  BIN_ZIPS+=("$(basename "$zip" .zip)")
done
shopt -u nullglob

missing_cmd=()
extra_cmd=()
allowed_extra_cmd=()
missing_zip=()
extra_zip=()
allowed_extra_zip=()

for lambda in "${MAKEFILE_LAMBDAS[@]}"; do
  if [ ! -f "cmd/${lambda}/main.go" ]; then
    missing_cmd+=("$lambda")
  fi
done

for handler in "${CMD_LAMBDAS[@]}"; do
  if ! contains "$handler" "${MAKEFILE_LAMBDAS[@]}"; then
    if contains "$handler" "${ALLOW_EXTRA_CMD[@]}"; then
      allowed_extra_cmd+=("$handler")
    else
      extra_cmd+=("$handler")
    fi
  fi
done

if [ "${#BIN_ZIPS[@]}" -gt 0 ]; then
  for lambda in "${MAKEFILE_LAMBDAS[@]}"; do
    if [ ! -f "bin/${lambda}.zip" ]; then
      missing_zip+=("$lambda")
    fi
  done
  for zipname in "${BIN_ZIPS[@]}"; do
    if ! contains "$zipname" "${MAKEFILE_LAMBDAS[@]}"; then
      if contains "$zipname" "${ALLOW_EXTRA_ZIPS[@]}"; then
        allowed_extra_zip+=("$zipname")
      else
        extra_zip+=("$zipname")
      fi
    fi
  done
fi

echo "=== Lambda Set Verification ==="
echo "Makefile LAMBDAS: ${#MAKEFILE_LAMBDAS[@]}"
echo

echo "1) cmd/<lambda>/main.go presence:"
if [ ${#missing_cmd[@]} -eq 0 ]; then
  echo "  ✓ All Makefile entries have cmd/<name>/main.go"
else
  printf '  ✗ Missing cmd entries: %s\n' "${missing_cmd[*]}"
fi
if [ ${#extra_cmd[@]} -gt 0 ]; then
  printf '  ✗ Extra cmd handlers not in Makefile: %s\n' "${extra_cmd[*]}"
fi
if [ ${#allowed_extra_cmd[@]} -gt 0 ]; then
  printf '  • Allowed extra cmd handlers: %s\n' "${allowed_extra_cmd[*]}"
fi
echo

echo "2) bin/<lambda>.zip parity:"
if [ "${#BIN_ZIPS[@]}" -eq 0 ]; then
  echo "  • No bin/*.zip artifacts present (zip parity check skipped)"
else
  if [ ${#missing_zip[@]} -eq 0 ] && [ ${#extra_zip[@]} -eq 0 ]; then
    echo "  ✓ bin/*.zip matches Makefile LAMBDAS"
  else
    [ ${#missing_zip[@]} -gt 0 ] && printf '  ✗ Missing zips: %s\n' "${missing_zip[*]}"
    [ ${#extra_zip[@]} -gt 0 ] && printf '  ✗ Extra zips not in Makefile: %s\n' "${extra_zip[*]}"
    [ ${#allowed_extra_zip[@]} -gt 0 ] && printf '  • Allowed extra zips: %s\n' "${allowed_extra_zip[*]}"
  fi
fi
echo

drift=0
if [ ${#missing_cmd[@]} -gt 0 ] || [ ${#extra_cmd[@]} -gt 0 ] || [ ${#missing_zip[@]} -gt 0 ] || [ ${#extra_zip[@]} -gt 0 ]; then
  drift=1
fi

if [ "$drift" -eq 0 ]; then
  echo "✅ Lambda set is consistent (Makefile ↔ cmd[/main.go] ↔ bin/*.zip)"
  exit 0
else
  echo "❌ Drift detected. Resolve issues above."
  exit 1
fi
