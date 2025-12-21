#!/usr/bin/env bash
set -euo pipefail

SMOKE_BASE_URL="${SMOKE_BASE_URL:-}"
SMOKE_USERNAME="${SMOKE_USERNAME:-}"
SMOKE_OBJECT_ID="${SMOKE_OBJECT_ID:-}"
SMOKE_ACCEPT_HEADER="${SMOKE_ACCEPT_HEADER:-application/activity+json}"
SMOKE_TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-15}"
SMOKE_INSECURE="${SMOKE_INSECURE:-0}"

if [[ -z "${SMOKE_BASE_URL}" ]]; then
  echo "SMOKE_BASE_URL is required (e.g., https://dev.lesser.host)" >&2
  exit 2
fi
if [[ -z "${SMOKE_USERNAME}" ]]; then
  echo "SMOKE_USERNAME is required (e.g., alice)" >&2
  exit 2
fi
if [[ -z "${SMOKE_OBJECT_ID}" ]]; then
  echo "SMOKE_OBJECT_ID is required (known object id for /objects/<id>)" >&2
  exit 2
fi

BASE="${SMOKE_BASE_URL%/}"
HOST="$(echo "${BASE}" | sed -E 's#^https?://##' | sed -E 's#/.*$##')"

curl_flags=(-sS -L --max-redirs 5 --connect-timeout 5 --max-time "${SMOKE_TIMEOUT_SECONDS}")
if [[ "${SMOKE_INSECURE}" == "1" ]]; then
  curl_flags+=(-k)
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

request_get() {
  local name="$1"; shift
  local url="$1"; shift
  local accept="${1:-}"

  local out="${tmpdir}/${name}.out"
  local code
  if [[ -n "${accept}" ]]; then
    code="$(curl "${curl_flags[@]}" -H "Accept: ${accept}" -o "${out}" -w "%{http_code}" "${url}" || true)"
  else
    code="$(curl "${curl_flags[@]}" -o "${out}" -w "%{http_code}" "${url}" || true)"
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

echo "=== Smoke Federation (Spec 07 R3) ==="
echo "Base: ${BASE}"
echo "User: ${SMOKE_USERNAME}"
echo "Host: ${HOST}"
echo

echo "1) GET /.well-known/webfinger"
webfinger_url="${BASE}/.well-known/webfinger?resource=acct:${SMOKE_USERNAME}@${HOST}"
resp="$(request_get webfinger "${webfinger_url}" "application/jrd+json, application/json")"
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

echo "2) GET /users/<username> (ActivityPub)"
resp="$(request_get actor "${BASE}/users/${SMOKE_USERNAME}" "${SMOKE_ACCEPT_HEADER}")"
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

echo "3) GET /users/<username>/followers (ActivityPub collection)"
resp="$(request_get followers "${BASE}/users/${SMOKE_USERNAME}/followers" "${SMOKE_ACCEPT_HEADER}")"
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

echo "4) GET /users/<username>/following (ActivityPub collection)"
resp="$(request_get following "${BASE}/users/${SMOKE_USERNAME}/following" "${SMOKE_ACCEPT_HEADER}")"
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

echo "5) GET /users/<username>/liked (ActivityPub collection)"
resp="$(request_get liked "${BASE}/users/${SMOKE_USERNAME}/liked" "${SMOKE_ACCEPT_HEADER}")"
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

echo "6) GET /objects/<id> (ActivityPub object)"
resp="$(request_get object "${BASE}/objects/${SMOKE_OBJECT_ID}" "${SMOKE_ACCEPT_HEADER}")"
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

if [[ "${failures}" -eq 0 ]]; then
  echo "✅ smoke-federation passed"
else
  echo "❌ smoke-federation failed (${failures} checks)"
fi

exit "${failures}"

