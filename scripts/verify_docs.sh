#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

readarray -t MAKEFILE_LAMBDAS < <(awk '
  $0 ~ /^LAMBDAS :=/ { in_lambdas=1; next }
  in_lambdas {
    if ($0 ~ /^[[:space:]]*$/) { exit }
    gsub(/\\/, "", $0)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", $0)
    if ($0 != "") print $0
  }
' Makefile)
LAMBDA_COUNT=${#MAKEFILE_LAMBDAS[@]}

status=0

echo "=== Doc Drift Verification (Spec 07 R7) ==="
echo "Expected Lambda count (from Makefile): ${LAMBDA_COUNT}"
echo

pulumi_hits=$(grep -Rni --binary-files=without-match "Pulumi" docs || true)
if [[ -n "${pulumi_hits}" ]]; then
  echo "✗ Pulumi references found (CDK is sole IaC path):"
  echo "${pulumi_hits}"
  echo "  -> Remove or rewrite Pulumi references to align with CDK-only deploy path."
  status=1
else
  echo "✓ No Pulumi references detected in docs/"
fi
echo

count_mismatches=()
while IFS= read -r line; do
  [[ -z "${line}" ]] && continue
  file="${line%%:*}"
  rest="${line#*:}"
  lineno="${rest%%:*}"
  text="${rest#*:}"
  found=$(echo "${text}" | sed -E 's/.*\b([0-9]+)[[:space:]]+Lambdas?\b.*/\1/')
  if [[ -n "${found}" && "${found}" != "${LAMBDA_COUNT}" ]]; then
    count_mismatches+=("${file}:${lineno}:${found}:${text}")
  fi
done < <(grep -RniE '\b[0-9]+[[:space:]]+Lambdas?\b' docs || true)

if [[ ${#count_mismatches[@]} -gt 0 ]]; then
  echo "✗ Lambda count mismatches:"
  for entry in "${count_mismatches[@]}"; do
    IFS=":" read -r file lineno found text <<< "${entry}"
    echo "  ${file}:${lineno} claims ${found} Lambdas (expected ${LAMBDA_COUNT})"
    echo "    -> Update to ${LAMBDA_COUNT} or replace with a templated reference (e.g., {{LAMBDA_COUNT}})."
  done
  status=1
else
  echo "✓ No stale Lambda count claims detected"
fi
echo

if [[ "${status}" -eq 0 ]]; then
  echo "✅ Doc drift check passed"
else
  echo "❌ Doc drift detected. See remediation hints above."
fi

exit "${status}"
