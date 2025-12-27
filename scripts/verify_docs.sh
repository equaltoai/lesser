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

DOC_SEARCH_PATHS=(docs README.md CONTRIBUTING.md CLAUDE.md AGENTS.md infra/cdk/README.md infra/cdk/inventory/README.md)

pulumi_hits=$(grep -Rni --exclude-dir=archive --binary-files=without-match "Pulumi" "${DOC_SEARCH_PATHS[@]}" || true)
if [[ -n "${pulumi_hits}" ]]; then
  echo "✗ Pulumi references found (CDK is sole IaC path):"
  echo "${pulumi_hits}"
  echo "  -> Remove or rewrite Pulumi references to align with CDK-only deploy path."
  status=1
else
  echo "✓ No Pulumi references detected"
fi
echo

make_hits=$(grep -RniE --exclude-dir=archive --binary-files=without-match '(^|[^[:alnum:]_])make[[:space:]]+(build|rebuild|dev|test|lint|fmt|deploy|local|verify|generate|schema|gqlgen|tidy|sec-scan|vuln-check)([-[:alnum:]_]+)?' "${DOC_SEARCH_PATHS[@]}" || true)
if [[ -n "${make_hits}" ]]; then
  echo "✗ Make workflow commands found (document ./lesser ... instead):"
  echo "${make_hits}"
  status=1
else
  echo "✓ No Make workflow commands detected (CLI-first docs)"
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
done < <(grep -RniE --exclude-dir=archive '\b[0-9]+[[:space:]]+Lambdas?\b' docs || true)

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

echo "=== Deployment + Naming Drift Checks ==="

config_refs_all=$(grep -Rni --exclude-dir=archive --binary-files=without-match "infra/cdk/config/" "${DOC_SEARCH_PATHS[@]}" || true)
config_refs=$(
  # Allow explicit "do not use" disclaimers to reference the path.
  echo "${config_refs_all}" | grep -viE "not loaded|reference template" || true
)
if [[ -n "${config_refs}" ]]; then
  echo "✗ infra/cdk/config references found (CDK app does not load these templates):"
  echo "${config_refs}"
  echo "  -> Replace with infra/cdk/stacks + infra/cdk/inventory references, or treat as archived/internal-only material."
  status=1
else
  echo "✓ No infra/cdk/config guidance detected (only allowlisted disclaimers at most)"
fi
echo

bad_cdk_rollback=$(grep -Rni --exclude-dir=archive --binary-files=without-match "cdk deploy --rollback" "${DOC_SEARCH_PATHS[@]}" || true)
if [[ -n "${bad_cdk_rollback}" ]]; then
  echo "✗ Invalid CDK rollback guidance found:"
  echo "${bad_cdk_rollback}"
  echo "  -> CDK does not support 'cdk deploy --rollback'; use CloudFormation cancel-update-stack or redeploy a previous git SHA."
  status=1
else
  echo "✓ No invalid 'cdk deploy --rollback' guidance detected"
fi
echo

legacy_env_names=$(grep -RniE --exclude-dir=archive --binary-files=without-match "/aws/lambda/[^[:space:]]+-(development|production)-" "${DOC_SEARCH_PATHS[@]}" || true)
if [[ -n "${legacy_env_names}" ]]; then
  echo "✗ Legacy Lambda naming found (use <app>-<stage>-<function>, stage is dev|staging|live):"
  echo "${legacy_env_names}"
  status=1
else
  echo "✓ No legacy Lambda naming detected"
fi
echo

echo "=== Primary Docs Navigation (docs/README.md) ==="
readme="docs/README.md"
if [[ ! -f "${readme}" ]]; then
  echo "✗ Missing: ${readme}"
  status=1
else
  archive_refs=$(
    {
      grep -nF "](archive/" "${readme}" || true
      grep -nF "](./archive/" "${readme}" || true
    } | sort -u
  )
  if [[ -n "${archive_refs}" ]]; then
    echo "✗ docs/README.md references archived docs (should link only to current documentation):"
    echo "${archive_refs}"
    status=1
  fi

  broken_links=()
  readme_dir="$(dirname "${readme}")"
  while IFS= read -r link; do
    [[ -z "${link}" ]] && continue
    if [[ "${link}" =~ ^https?:// ]] || [[ "${link}" =~ ^mailto: ]] || [[ "${link}" =~ ^# ]]; then
      continue
    fi

    path="${link%%#*}"
    path="${path%%\?*}"
    if [[ "${path}" != *.md ]]; then
      continue
    fi

    target="${readme_dir}/${path}"
    if [[ ! -f "${target}" ]]; then
      broken_links+=("${link}")
    fi
  done < <(grep -oE '\\[[^]]+\\]\\([^)]+\\)' "${readme}" | sed -E 's/.*\\(([^)]+)\\).*/\\1/' | sort -u)

  if [[ ${#broken_links[@]} -gt 0 ]]; then
    echo "✗ Broken links in docs/README.md:"
    for link in "${broken_links[@]}"; do
      echo "  - ${link}"
    done
    status=1
  elif [[ -z "${archive_refs}" ]]; then
    echo "✓ docs/README.md links resolve and do not reference docs/archive/"
  fi
fi
echo

if [[ "${status}" -eq 0 ]]; then
  echo "✅ Doc drift check passed"
else
  echo "❌ Doc drift detected. See remediation hints above."
fi

exit "${status}"
