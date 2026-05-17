#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

AUTH_UI_SOURCE_PATHS=(
  auth-ui/src
  auth-ui/public
  auth-ui/package.json
  auth-ui/pnpm-lock.yaml
  auth-ui/astro.config.mjs
  auth-ui/tsconfig.json
  auth-ui/components.json
)

CSP_HASH_PATHS=(
  infra/cdk/constructs/frontend_response_headers.go
)

format_commit() {
  git log -1 --format='%h %cs %s' -- "$@"
}

latest_commit() {
  git log -1 --format='%H' -- "$@"
}

untracked_auth_ui_sources() {
  git ls-files --others --exclude-standard -- "${AUTH_UI_SOURCE_PATHS[@]}"
}

ensure_history_for_ancestry_check() {
  if [[ "$(git rev-parse --is-shallow-repository)" != "true" ]]; then
    return
  fi

  echo "Repository is shallow; fetching history for auth UI CSP freshness check"
  git fetch --unshallow --quiet || git fetch --depth=1000 --quiet
}

fail_stale_hashes() {
  cat >&2 <<EOF
auth UI source is newer than the CloudFront CSP hash definitions.

Latest auth UI source change:
  $(format_commit "${AUTH_UI_SOURCE_PATHS[@]}")

Latest CSP hash definition change:
  $(format_commit "${CSP_HASH_PATHS[@]}")

Refresh the auth UI CSP hash definitions in:
  ${CSP_HASH_PATHS[*]}

Then rerun:
  bash scripts/verify_auth_ui_csp.sh
EOF
  exit 1
}

echo "==> Verifying auth UI CSP hash freshness"

ensure_history_for_ancestry_check

auth_ui_commit="$(latest_commit "${AUTH_UI_SOURCE_PATHS[@]}")"
csp_commit="$(latest_commit "${CSP_HASH_PATHS[@]}")"

if [[ -z "${auth_ui_commit}" ]]; then
  echo "could not find a git commit for auth UI source paths" >&2
  exit 1
fi
if [[ -z "${csp_commit}" ]]; then
  echo "could not find a git commit for auth UI CSP hash paths" >&2
  exit 1
fi

if ! git merge-base --is-ancestor "${auth_ui_commit}" "${csp_commit}"; then
  fail_stale_hashes
fi

untracked_sources="$(untracked_auth_ui_sources)"
if [[ -n "${untracked_sources}" ]]; then
  cat >&2 <<EOF
Untracked auth UI source/config files are present, so CSP freshness cannot be proven:
${untracked_sources}

Track or remove these files, and refresh CSP hash definitions if they can affect generated inline script/style snippets.
EOF
  exit 1
fi

if {
  ! git diff --quiet -- "${AUTH_UI_SOURCE_PATHS[@]}" ||
    ! git diff --cached --quiet -- "${AUTH_UI_SOURCE_PATHS[@]}"
} && {
  git diff --quiet -- "${CSP_HASH_PATHS[@]}" &&
    git diff --cached --quiet -- "${CSP_HASH_PATHS[@]}"
}; then
  cat >&2 <<EOF
Uncommitted auth UI source changes are present without corresponding CSP hash definition changes.

If these auth UI changes can affect generated inline script/style snippets, refresh:
  ${CSP_HASH_PATHS[*]}
EOF
  exit 1
fi

echo "✅ Auth UI CSP hashes are at least as new as auth-ui source"
