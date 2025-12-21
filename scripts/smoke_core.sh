#!/usr/bin/env bash
set -euo pipefail

SMOKE_BASE_URL="${SMOKE_BASE_URL:-}"
SMOKE_TOKEN="${SMOKE_TOKEN:-}"
SMOKE_TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-15}"
SMOKE_INSECURE="${SMOKE_INSECURE:-0}"

if [[ -z "${SMOKE_BASE_URL}" ]]; then
  echo "SMOKE_BASE_URL is required (e.g., https://dev.lesser.host)" >&2
  exit 2
fi

BASE="${SMOKE_BASE_URL%/}"

curl_flags=(-sS -L --max-redirs 5 --connect-timeout 5 --max-time "${SMOKE_TIMEOUT_SECONDS}")
if [[ "${SMOKE_INSECURE}" == "1" ]]; then
  curl_flags+=(-k)
fi

auth_header=()
if [[ -n "${SMOKE_TOKEN}" ]]; then
  auth_header=(-H "Authorization: ${SMOKE_TOKEN}")
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

request() {
  local name="$1"; shift
  local method="$1"; shift
  local url="$1"; shift
  local body="${1:-}"

  local out="${tmpdir}/${name}.out"
  local code

  if [[ "${method}" == "GET" ]]; then
    code="$(curl "${curl_flags[@]}" "${auth_header[@]}" -o "${out}" -w "%{http_code}" "${url}" || true)"
  else
    code="$(curl "${curl_flags[@]}" "${auth_header[@]}" -H "Content-Type: application/json" -X "${method}" -d "${body}" -o "${out}" -w "%{http_code}" "${url}" || true)"
  fi

  echo "${code}:${out}"
}

require_json() {
  local file="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -e '.' "${file}" >/dev/null 2>&1
    return
  fi
  grep -Eq '^[[:space:]]*[{[]' "${file}"
}

failures=0

echo "=== Smoke Core (Spec 07 R4) ==="
echo "Base: ${BASE}"
echo

echo "1) GET /health"
resp="$(request health GET "${BASE}/health")"
code="${resp%%:*}"
file="${resp#*:}"
if [[ "${code}" != "200" ]]; then
  echo "  ✗ expected 200, got ${code}"
  failures=$((failures + 1))
else
  if ! require_json "${file}"; then
    echo "  ✗ expected JSON body"
    failures=$((failures + 1))
  else
    echo "  ✓ ok"
  fi
fi
echo

echo "2) GET /api/v1/instance"
resp="$(request instance GET "${BASE}/api/v1/instance")"
code="${resp%%:*}"
file="${resp#*:}"
if [[ "${code}" != "200" ]]; then
  echo "  ✗ expected 200, got ${code}"
  failures=$((failures + 1))
else
  if ! require_json "${file}"; then
    echo "  ✗ expected JSON body"
    failures=$((failures + 1))
  else
    echo "  ✓ ok"
  fi
fi
echo

echo "3) POST /api/graphql"
graphql_body='{"query":"query { __typename }"}'
resp="$(request graphql POST "${BASE}/api/graphql" "${graphql_body}")"
code="${resp%%:*}"
file="${resp#*:}"

case "${code}" in
  200|401|403)
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body (got ${code})"
      failures=$((failures + 1))
    else
      echo "  ✓ ok (http ${code})"
    fi
    ;;
  404)
    echo "  ✗ route missing (404)"
    failures=$((failures + 1))
    ;;
  5*)
    echo "  ✗ server error (${code})"
    failures=$((failures + 1))
    ;;
  *)
    echo "  ✗ unexpected status (${code})"
    failures=$((failures + 1))
    ;;
esac
echo

if [[ "${failures}" -eq 0 ]]; then
  echo "✅ smoke-core passed"
else
  echo "❌ smoke-core failed (${failures} checks)"
fi

exit "${failures}"

