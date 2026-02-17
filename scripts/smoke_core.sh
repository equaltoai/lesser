#!/usr/bin/env bash
set -euo pipefail

SMOKE_BASE_URL="${SMOKE_BASE_URL:-}"
SMOKE_TOKEN="${SMOKE_TOKEN:-}"
SMOKE_TIMEOUT_SECONDS="${SMOKE_TIMEOUT_SECONDS:-15}"
SMOKE_INSECURE="${SMOKE_INSECURE:-0}"
SMOKE_TRANSLATE_STATUS_ID="${SMOKE_TRANSLATE_STATUS_ID:-}"

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

translation_enabled_from_v2_instance() {
  local file="$1"

  if command -v jq >/dev/null 2>&1; then
    jq -r '.configuration.translation.enabled // empty' "${file}" 2>/dev/null
    return
  fi

  # jq-less fallback: look for a translation.enabled boolean near the translation block.
  local compact
  compact="$(tr -d '\n' <"${file}")"
  if [[ "${compact}" =~ \"translation\"[[:space:]]*:[[:space:]]*\\{[^\\}]*\"enabled\"[[:space:]]*:[[:space:]]*(true|false) ]]; then
    echo "${BASH_REMATCH[1]}"
    return
  fi

  echo ""
}

trust_enabled_from_v2_instance() {
  local file="$1"

  if command -v jq >/dev/null 2>&1; then
    jq -r '.configuration.trust.enabled // empty' "${file}" 2>/dev/null
    return
  fi

  # jq-less fallback: look for a trust.enabled boolean near the trust block.
  local compact
  compact="$(tr -d '\n' <"${file}")"
  if [[ "${compact}" =~ \"trust\"[[:space:]]*:[[:space:]]*\\{[^\\}]*\"enabled\"[[:space:]]*:[[:space:]]*(true|false) ]]; then
    echo "${BASH_REMATCH[1]}"
    return
  fi

  echo ""
}

graphql_response_has_errors() {
  local file="$1"

  if command -v jq >/dev/null 2>&1; then
    jq -e '((.errors // []) | length) > 0' "${file}" >/dev/null 2>&1
    return
  fi

  grep -q '"errors"' "${file}"
}

json_first_public_status_id() {
  local file="$1"

  if command -v jq >/dev/null 2>&1; then
    jq -r '.[0].id // empty' "${file}" 2>/dev/null
    return
  fi

  # jq-less fallback (best-effort): extract first "id":"..." from a JSON array.
  grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' "${file}" | head -n 1 | sed -E 's/^"id"[[:space:]]*:[[:space:]]*"([^"]+)".*$/\1/'
}

json_status_id() {
  local file="$1"

  if command -v jq >/dev/null 2>&1; then
    jq -r '.id // empty' "${file}" 2>/dev/null
    return
  fi

  grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' "${file}" | head -n 1 | sed -E 's/^"id"[[:space:]]*:[[:space:]]*"([^"]+)".*$/\1/'
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

echo "4) GET /api/v2/instance (translation + trust config)"
resp="$(request instance_v2 GET "${BASE}/api/v2/instance")"
code="${resp%%:*}"
file="${resp#*:}"
translation_enabled=""
trust_enabled=""

if [[ "${code}" != "200" ]]; then
  echo "  ✗ expected 200, got ${code}"
  failures=$((failures + 1))
else
  if ! require_json "${file}"; then
    echo "  ✗ expected JSON body"
    failures=$((failures + 1))
  else
    translation_enabled="$(translation_enabled_from_v2_instance "${file}")"
    if [[ "${translation_enabled}" != "true" && "${translation_enabled}" != "false" ]]; then
      echo "  ✗ expected .configuration.translation.enabled boolean"
      failures=$((failures + 1))
    else
      trust_enabled="$(trust_enabled_from_v2_instance "${file}")"
      if [[ "${trust_enabled}" != "true" && "${trust_enabled}" != "false" ]]; then
        echo "  ✗ expected .configuration.trust.enabled boolean"
        failures=$((failures + 1))
      else
        echo "  ✓ ok (translation.enabled=${translation_enabled}, trust.enabled=${trust_enabled})"
      fi
    fi
  fi
fi
echo

echo "5) GET /api/v1/instance/translation_languages"
resp="$(request translation_languages GET "${BASE}/api/v1/instance/translation_languages")"
code="${resp%%:*}"
file="${resp#*:}"

if [[ "${translation_enabled}" == "true" ]]; then
  if [[ "${code}" != "200" ]]; then
    echo "  ✗ expected 200 (translation enabled), got ${code}"
    failures=$((failures + 1))
  else
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      if command -v jq >/dev/null 2>&1; then
        if ! jq -e 'type == "array"' "${file}" >/dev/null 2>&1; then
          echo "  ✗ expected JSON array body"
          failures=$((failures + 1))
        else
          echo "  ✓ ok"
        fi
      else
        echo "  ✓ ok"
      fi
    fi
  fi
else
  if [[ "${code}" != "422" ]]; then
    echo "  ✗ expected 422 (translation disabled), got ${code}"
    failures=$((failures + 1))
  else
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      echo "  ✓ ok"
    fi
  fi
fi
echo

echo "6) POST /api/v1/statuses/:id/translate"
if [[ "${translation_enabled}" != "true" ]]; then
  resp="$(request translate POST "${BASE}/api/v1/statuses/smoke-translation/translate" "")"
  code="${resp%%:*}"
  file="${resp#*:}"
  if [[ "${code}" != "422" ]]; then
    echo "  ✗ expected 422 (translation disabled), got ${code}"
    failures=$((failures + 1))
  else
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      echo "  ✓ ok (disabled)"
    fi
  fi
else
  if [[ -z "${SMOKE_TOKEN}" ]]; then
    echo "  ✗ translation enabled but SMOKE_TOKEN is not set"
    failures=$((failures + 1))
  else
    status_id="${SMOKE_TRANSLATE_STATUS_ID}"
    created_status_id=""

    if [[ -z "${status_id}" ]]; then
      timeline_resp="$(request public_timeline GET "${BASE}/api/v1/timelines/public?limit=1")"
      timeline_code="${timeline_resp%%:*}"
      timeline_file="${timeline_resp#*:}"
      if [[ "${timeline_code}" == "200" ]] && require_json "${timeline_file}"; then
        status_id="$(json_first_public_status_id "${timeline_file}")"
      fi
    fi

    if [[ -z "${status_id}" ]]; then
      create_body='{"status":"Smoke translation check (hola mundo)","visibility":"unlisted","sensitive":false,"language":"es"}'
      create_resp="$(request create_status POST "${BASE}/api/v1/statuses" "${create_body}")"
      create_code="${create_resp%%:*}"
      create_file="${create_resp#*:}"

      case "${create_code}" in
        200|201)
          if ! require_json "${create_file}"; then
            echo "  ✗ expected JSON body from status create"
            failures=$((failures + 1))
          else
            status_id="$(json_status_id "${create_file}")"
            created_status_id="${status_id}"
          fi
          ;;
        *)
          echo "  ✗ expected 200/201 from status create, got ${create_code}"
          failures=$((failures + 1))
          ;;
      esac
    fi

    if [[ -z "${status_id}" ]]; then
      echo "  ✗ unable to find or create a status id to translate"
      failures=$((failures + 1))
    else
      resp="$(request translate POST "${BASE}/api/v1/statuses/${status_id}/translate" "")"
      code="${resp%%:*}"
      file="${resp#*:}"
      if [[ "${code}" != "200" ]]; then
        echo "  ✗ expected 200 (translation enabled), got ${code}"
        failures=$((failures + 1))
      else
        if ! require_json "${file}"; then
          echo "  ✗ expected JSON body"
          failures=$((failures + 1))
        else
          if command -v jq >/dev/null 2>&1; then
            if ! jq -e '.content | type == "string"' "${file}" >/dev/null 2>&1; then
              echo "  ✗ expected translation response with .content string"
              failures=$((failures + 1))
            else
              echo "  ✓ ok"
            fi
          else
            echo "  ✓ ok"
          fi
        fi
      fi
    fi

    if [[ -n "${created_status_id}" ]]; then
      _="$(request delete_status DELETE "${BASE}/api/v1/statuses/${created_status_id}" "")"
    fi
  fi
fi
echo

echo "7) GET /api/v1/trust/jwks.json"
resp="$(request trust_jwks GET "${BASE}/api/v1/trust/jwks.json")"
code="${resp%%:*}"
file="${resp#*:}"

if [[ "${trust_enabled}" == "true" ]]; then
  if [[ "${code}" != "200" ]]; then
    echo "  ✗ expected 200 (trust enabled), got ${code}"
    failures=$((failures + 1))
  else
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      if command -v jq >/dev/null 2>&1; then
        if ! jq -e '.keys | type == "array"' "${file}" >/dev/null 2>&1; then
          echo "  ✗ expected JWKS JSON with .keys array"
          failures=$((failures + 1))
        else
          echo "  ✓ ok"
        fi
      else
        echo "  ✓ ok"
      fi
    fi
  fi
else
  case "${code}" in
    200)
      if ! require_json "${file}"; then
        echo "  ✗ expected JSON body"
        failures=$((failures + 1))
      else
        echo "  ✓ ok (trust disabled; jwks reachable)"
      fi
      ;;
    422|409)
      if ! require_json "${file}"; then
        echo "  ✗ expected JSON body"
        failures=$((failures + 1))
      else
        echo "  ✓ ok (trust disabled)"
      fi
      ;;
    *)
      echo "  ✗ unexpected status (${code})"
      failures=$((failures + 1))
      ;;
  esac
fi
echo

echo "8) GET /api/v1/trust/attestations"
resp="$(request trust_attestations GET "${BASE}/api/v1/trust/attestations")"
code="${resp%%:*}"
file="${resp#*:}"

if [[ "${trust_enabled}" == "true" ]]; then
  if [[ "${code}" != "200" ]]; then
    echo "  ✗ expected 200 (trust enabled), got ${code}"
    failures=$((failures + 1))
  else
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      echo "  ✓ ok"
    fi
  fi
else
  case "${code}" in
    200)
      if ! require_json "${file}"; then
        echo "  ✗ expected JSON body"
        failures=$((failures + 1))
      else
        echo "  ✓ ok (trust disabled; attestations reachable)"
      fi
      ;;
    422|409)
      if ! require_json "${file}"; then
        echo "  ✗ expected JSON body"
        failures=$((failures + 1))
      else
        echo "  ✓ ok (trust disabled)"
      fi
      ;;
    *)
      echo "  ✗ unexpected status (${code})"
      failures=$((failures + 1))
      ;;
  esac
fi
echo

echo "9) POST /api/graphql (search)"
graphql_search_body='{"query":"query SmokeSearch($q: String!){ search(query: $q, first: 1){ accounts { id } statuses { id } hashtags { name } } }","variables":{"q":"smoke"}}'
resp="$(request graphql_search POST "${BASE}/api/graphql" "${graphql_search_body}")"
code="${resp%%:*}"
file="${resp#*:}"

case "${code}" in
  200)
    if ! require_json "${file}"; then
      echo "  ✗ expected JSON body"
      failures=$((failures + 1))
    else
      if graphql_response_has_errors "${file}"; then
        echo "  ✗ graphql search returned errors"
        failures=$((failures + 1))
      else
        if command -v jq >/dev/null 2>&1; then
          if ! jq -e '.data.search.accounts | type == "array"' "${file}" >/dev/null 2>&1; then
            echo "  ✗ expected .data.search.accounts array"
            failures=$((failures + 1))
          elif ! jq -e '.data.search.statuses | type == "array"' "${file}" >/dev/null 2>&1; then
            echo "  ✗ expected .data.search.statuses array"
            failures=$((failures + 1))
          elif ! jq -e '.data.search.hashtags | type == "array"' "${file}" >/dev/null 2>&1; then
            echo "  ✗ expected .data.search.hashtags array"
            failures=$((failures + 1))
          else
            echo "  ✓ ok"
          fi
        else
          echo "  ✓ ok"
        fi
      fi
    fi
    ;;
  401|403)
    if [[ -n "${SMOKE_TOKEN}" ]]; then
      echo "  ✗ expected 200 (token provided), got ${code}"
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

echo "10) WebSockets (GraphQL subscriptions + streaming)"
ws_args=(--base-url="${BASE}" --timeout-seconds="${SMOKE_TIMEOUT_SECONDS}")
if [[ -n "${SMOKE_TOKEN}" ]]; then
  ws_args+=(--token="${SMOKE_TOKEN}")
fi
if [[ "${SMOKE_INSECURE}" == "1" ]]; then
  ws_args+=(--insecure)
fi

if go run ./tools/ws_smoke "${ws_args[@]}"; then
  echo "  ✓ ok"
else
  echo "  ✗ websocket smoke failed"
  failures=$((failures + 1))
fi
echo

if [[ "${failures}" -eq 0 ]]; then
  echo "✅ smoke-core passed"
else
  echo "❌ smoke-core failed (${failures} checks)"
fi

exit "${failures}"
