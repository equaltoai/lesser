#!/usr/bin/env bash

# verify_lambda_set.sh
# Usage: scripts/verify_lambda_set.sh
# Purpose:
#   - Extract Makefile LAMBDAS
#   - Ensure each lambda has cmd/<name>/main.go with func main()
#   - Detect extra cmd handlers not listed in Makefile (excluding known tooling/infra)
#   - Ensure bin/*.zip artifacts match Makefile LAMBDAS (excluding known tooling/infra)
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

has_func_main() {
  local file="$1"
  if command -v rg >/dev/null 2>&1; then
    rg -q '^[[:space:]]*func main\(\)' "$file"
  else
    grep -Eq '^[[:space:]]*func main\(\)' "$file"
  fi
}

readarray -t MAKEFILE_LAMBDAS < <(awk '
  $0 ~ /^LAMBDAS :=/ { in_lambdas=1; next }
  in_lambdas {
    if ($0 ~ /^[[:space:]]*$/) { exit }
    gsub(/\\/, "", $0)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0)
    if ($0 != "") print $0
  }
' Makefile | sort)

CMD_WITH_MAIN=()
for dir in cmd/*; do
  [ -d "$dir" ] || continue
  name="$(basename "$dir")"
  main_path="$dir/main.go"
  if [ -f "$main_path" ] && has_func_main "$main_path"; then
    CMD_WITH_MAIN+=("$name")
  fi
done
IFS=$'\n' CMD_WITH_MAIN=($(printf '%s\n' "${CMD_WITH_MAIN[@]}" | sort -u))
unset IFS

shopt -s nullglob
BIN_ZIPS=()
for zip in bin/*.zip; do
  BIN_ZIPS+=("$(basename "$zip" .zip)")
done
shopt -u nullglob
IFS=$'\n' BIN_ZIPS=($(printf '%s\n' "${BIN_ZIPS[@]}" | sort -u))
unset IFS

missing_cmd_file=()
missing_cmd_entrypoint=()
for lambda in "${MAKEFILE_LAMBDAS[@]}"; do
  main_path="cmd/${lambda}/main.go"
  if [ ! -f "$main_path" ]; then
    missing_cmd_file+=("$lambda")
  elif ! has_func_main "$main_path"; then
    missing_cmd_entrypoint+=("$lambda")
  fi
done

extra_cmd=()
allowed_extra_cmd=()
for handler in "${CMD_WITH_MAIN[@]}"; do
  if ! contains "$handler" "${MAKEFILE_LAMBDAS[@]}"; then
    if contains "$handler" "${ALLOW_EXTRA_CMD[@]}"; then
      allowed_extra_cmd+=("$handler")
    else
      extra_cmd+=("$handler")
    fi
  fi
done

missing_zip=()
extra_zip=()
allowed_extra_zip=()
for lambda in "${MAKEFILE_LAMBDAS[@]}"; do
  if ! contains "$lambda" "${BIN_ZIPS[@]}"; then
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

echo "=== Lambda Set Verification (Spec 07 R1/R2) ==="
echo "Makefile LAMBDAS: ${#MAKEFILE_LAMBDAS[@]}"
echo "cmd/<name>/main.go with func main(): ${#CMD_WITH_MAIN[@]}"
echo "bin/*.zip artifacts: ${#BIN_ZIPS[@]}"
echo

echo "1) cmd/<lambda>/main.go presence + func main():"
if [ ${#missing_cmd_file[@]} -eq 0 ] && [ ${#missing_cmd_entrypoint[@]} -eq 0 ]; then
  echo "  ✓ All Makefile entries have cmd/<name>/main.go with func main()"
else
  [ ${#missing_cmd_file[@]} -gt 0 ] && printf '  ✗ Missing cmd/<name>/main.go: %s\n' "${missing_cmd_file[*]}"
  [ ${#missing_cmd_entrypoint[@]} -gt 0 ] && printf '  ✗ main.go missing func main(): %s\n' "${missing_cmd_entrypoint[*]}"
fi
if [ ${#extra_cmd[@]} -gt 0 ]; then
  printf '  ✗ Extra cmd handlers not in Makefile: %s\n' "${extra_cmd[*]}"
fi
if [ ${#allowed_extra_cmd[@]} -gt 0 ]; then
  printf '  • Allowed extra cmd handlers: %s\n' "${allowed_extra_cmd[*]}"
fi
echo

echo "2) bin/<lambda>.zip parity vs Makefile:"
if [ ${#missing_zip[@]} -eq 0 ] && [ ${#extra_zip[@]} -eq 0 ]; then
  echo "  ✓ bin/*.zip matches Makefile LAMBDAS (allowlist excluded)"
else
  [ ${#missing_zip[@]} -gt 0 ] && printf '  ✗ Missing zips: %s\n' "${missing_zip[*]}"
  [ ${#extra_zip[@]} -gt 0 ] && printf '  ✗ Extra zips not in Makefile: %s\n' "${extra_zip[*]}"
  [ ${#allowed_extra_zip[@]} -gt 0 ] && printf '  • Allowed extra zips: %s\n' "${allowed_extra_zip[*]}"
fi
echo

drift=0
if [ ${#missing_cmd_file[@]} -gt 0 ] || [ ${#missing_cmd_entrypoint[@]} -gt 0 ] || [ ${#extra_cmd[@]} -gt 0 ] || [ ${#missing_zip[@]} -gt 0 ] || [ ${#extra_zip[@]} -gt 0 ]; then
  drift=1
fi

if [ "$drift" -eq 0 ]; then
  echo "✅ Lambda set is consistent (Makefile ↔ cmd[/main.go+func main] ↔ bin/*.zip)"
  exit 0
else
  echo "❌ Drift detected. Resolve issues above."
  exit 1
fi