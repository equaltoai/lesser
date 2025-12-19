#!/bin/bash

echo "=== Lambda Functions Verification ==="
echo

set -euo pipefail

readarray -t makefile_lambdas < <(awk '
  $0 ~ /^LAMBDAS :=/ { in_lambdas=1; next }
  in_lambdas {
    if ($0 ~ /^[[:space:]]*$/) { exit }
    gsub(/\\/, "", $0)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0)
    if ($0 != "") print $0
  }
' Makefile)

readarray -t cmd_handlers < <(for dir in cmd/*/; do
  main_go="${dir}main.go"
  if [ -f "$main_go" ] && rg -q "^[[:space:]]*func main\\(\\)" "$main_go"; then
    basename "$dir"
  fi
done | sort)

echo "1. Handlers in cmd/ (main.go + func main):"
echo "-----------------------------------------"
printf '✓ %s\n' "${cmd_handlers[@]}"
echo

echo "2. Functions packaged by make build-lambdas (Makefile LAMBDAS):"
echo "--------------------------------------------------------------"
printf '✓ %s\n' "${makefile_lambdas[@]}"
echo

echo "3. cmd/ handlers missing from Makefile LAMBDAS (utilities/stubs?):"
echo "---------------------------------------------------------------"
missing_from_makefile=()
for handler in "${cmd_handlers[@]}"; do
  if ! printf '%s\n' "${makefile_lambdas[@]}" | rg -qx "$handler"; then
    missing_from_makefile+=("$handler")
  fi
done
if [ "${#missing_from_makefile[@]}" -eq 0 ]; then
  echo "✓ None"
else
  printf '✗ %s\n' "${missing_from_makefile[@]}"
fi
echo

echo "4. Makefile LAMBDAS missing cmd/<name>/main.go:"
echo "----------------------------------------------"
missing_cmd=()
for lambda in "${makefile_lambdas[@]}"; do
  if [ ! -f "cmd/${lambda}/main.go" ]; then
    missing_cmd+=("$lambda")
  fi
done
if [ "${#missing_cmd[@]}" -eq 0 ]; then
  echo "✓ None"
else
  printf '✗ %s\n' "${missing_cmd[@]}"
fi
echo

echo "5. Build artifacts in bin/ (zip per Makefile LAMBDAS):"
echo "-----------------------------------------------------"
missing_zip=()
for lambda in "${makefile_lambdas[@]}"; do
  if [ -f "bin/${lambda}.zip" ]; then
    echo "✓ ${lambda}.zip"
  else
    missing_zip+=("$lambda")
    echo "✗ ${lambda}.zip (missing - run 'make build-lambdas')"
  fi
done
echo

echo "6. Extra zip artifacts in bin/ (not in Makefile LAMBDAS):"
echo "--------------------------------------------------------"
extra_zip=()
if [ -d "bin" ]; then
  while IFS= read -r zip; do
    name="$(basename "$zip" .zip)"
    if ! printf '%s\n' "${makefile_lambdas[@]}" | rg -qx "$name"; then
      extra_zip+=("$name")
    fi
  done < <(ls -1 bin/*.zip 2>/dev/null || true)
fi
if [ "${#extra_zip[@]}" -eq 0 ]; then
  echo "✓ None"
else
  printf '• %s.zip\n' "${extra_zip[@]}"
fi

echo
echo "=== Summary ==="
echo "To build packaged functions: 'make build-lambdas'"
echo "To deploy with CDK: see 'make deploy-dev|deploy-test|deploy-live' or infra/cdk/README.md"
