#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AUTH_UI_DIR="${ROOT_DIR}/auth-ui"
CSP_SOURCE="${ROOT_DIR}/infra/cdk/constructs/frontend_response_headers.go"

for tool in node corepack; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "${tool} is required to build auth-ui and verify its CSP hashes" >&2
    exit 1
  fi
done

node -e '
  const major = Number(process.versions.node.split(".")[0]);
  if (major < 20) {
    console.error(`auth-ui requires Node >=20 (found ${process.version})`);
    process.exit(1);
  }
'

node --test "${ROOT_DIR}/scripts/verify_auth_ui_csp.test.mjs"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
expected_hashes="${tmp_dir}/expected"
built_hashes="${tmp_dir}/built"

echo "==> Building auth UI from the pinned pnpm lockfile"
rm -rf "${AUTH_UI_DIR}/dist"
(
  cd "${AUTH_UI_DIR}"
  corepack pnpm install --frozen-lockfile
  corepack pnpm build
)

echo "==> Comparing emitted inline content with CloudFront CSP hashes"
node - "${ROOT_DIR}" "${CSP_SOURCE}" "${expected_hashes}" "${built_hashes}" <<'NODE'
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const [rootDir, cspSourcePath, expectedPath, builtPath] = process.argv.slice(2);

function uniqueSorted(values) {
  return [...new Set(values)].sort();
}

function hashesFromGoFunction(source, functionName) {
  const signature = `func ${functionName}() []string {`;
  const start = source.indexOf(signature);
  if (start === -1) {
    throw new Error(`could not find ${functionName} in ${cspSourcePath}`);
  }
  const end = source.indexOf("\n}\n", start);
  if (end === -1) {
    throw new Error(`could not find the end of ${functionName} in ${cspSourcePath}`);
  }
  return uniqueSorted(source.slice(start, end).match(/sha256-[A-Za-z0-9+/]+={0,2}/g) || []);
}

function htmlFiles(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const entryPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      return htmlFiles(entryPath);
    }
    return entry.isFile() && entry.name.endsWith(".html") ? [entryPath] : [];
  });
}

function sha256CSP(content) {
  return `sha256-${crypto.createHash("sha256").update(content, "utf8").digest("base64")}`;
}

const goSource = fs.readFileSync(cspSourcePath, "utf8");
const expected = [
  ...hashesFromGoFunction(goSource, "authUIInlineScriptHashes").map((hash) => `script\t${hash}`),
  ...hashesFromGoFunction(goSource, "authUIInlineStyleHashes").map((hash) => `style\t${hash}`),
].sort();

const distDir = path.join(rootDir, "auth-ui", "dist");
if (!fs.existsSync(distDir)) {
  throw new Error(`auth-ui build did not create ${distDir}`);
}

const scripts = [];
const styles = [];
for (const htmlPath of htmlFiles(distDir)) {
  const html = fs.readFileSync(htmlPath, "utf8");
  for (const match of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    if (!/(^|\s)src\s*=/i.test(match[1])) {
      scripts.push(sha256CSP(match[2]));
    }
  }
  for (const match of html.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/gi)) {
    styles.push(sha256CSP(match[1]));
  }
}

const built = [
  ...uniqueSorted(scripts).map((hash) => `script\t${hash}`),
  ...uniqueSorted(styles).map((hash) => `style\t${hash}`),
].sort();

if (built.length === 0) {
  throw new Error("auth-ui build emitted no inline script or style content to verify");
}

fs.writeFileSync(expectedPath, `${expected.join("\n")}\n`);
fs.writeFileSync(builtPath, `${built.join("\n")}\n`);
NODE

if ! diff -u \
  --label "CloudFront CSP hashes (frontend_response_headers.go)" \
  --label "auth-ui production build hashes" \
  "${expected_hashes}" "${built_hashes}"; then
  cat >&2 <<'EOF'

auth-ui's production build does not match the CloudFront CSP allowlist.
Update authUIInlineScriptHashes/authUIInlineStyleHashes in
infra/cdk/constructs/frontend_response_headers.go to exactly match the build,
then rerun: bash scripts/verify_auth_ui_csp.sh
EOF
  exit 1
fi

echo "✅ Auth UI production build matches the CloudFront CSP hash allowlist"
