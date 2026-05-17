#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! -f "${ROOT_DIR}/auth-ui/dist/index.html" ]]; then
  echo "auth-ui/dist is missing; run: corepack pnpm --dir auth-ui -s build" >&2
  exit 1
fi

echo "==> Verifying auth UI CSP hashes against built auth-ui/dist"
(
  cd "${ROOT_DIR}/infra/cdk"
  LESSER_REQUIRE_AUTH_UI_CSP_DIST=1 \
    go test ./stacks -run 'TestFrontendStaticCSP(IsStrictAndBehaviorScoped|MatchesBuiltAuthUIDist)' -count=1
)

echo "✅ Auth UI CSP hashes match built auth-ui/dist"
