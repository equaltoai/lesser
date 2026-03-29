#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <version> [out-dir]" >&2
  exit 1
fi

VERSION="$1"
OUT_DIR="${2:-dist/release}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

if [[ -z "${VERSION}" ]]; then
  echo "version is required" >&2
  exit 1
fi
if [[ "${VERSION}" != v* ]]; then
  echo "version must start with 'v' (for example: v1.0.0)" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"

GIT_SHA="$(git rev-parse --verify HEAD)"
GO_VERSION="$(go env GOVERSION)"

CDK_MAJOR="$(awk '
  /github.com\/aws\/aws-cdk-go\/awscdk\/v[0-9]+/ {
    if (match($0, /\/v[0-9]+/)) {
      v = substr($0, RSTART + 2, RLENGTH - 2)
      print v
      exit
    }
  }
' infra/cdk/go.mod)"
if [[ -z "${CDK_MAJOR}" ]]; then
  echo "failed to resolve CDK major version from infra/cdk/go.mod" >&2
  exit 1
fi

RECEIPT_SCHEMA_VERSION="$(awk '
  /const receiptSchemaVersion = [0-9]+/ {
    print $4
    exit
  }
' cmd/lesser/state.go)"
if [[ -z "${RECEIPT_SCHEMA_VERSION}" ]]; then
  echo "failed to resolve receipt schema version from cmd/lesser/state.go" >&2
  exit 1
fi

TARGETS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

echo "Building canonical Lambda zip artifacts"
go run ./cmd/lesser build lambdas --rebuild

for target in "${TARGETS[@]}"; do
  read -r GOOS GOARCH <<< "${target}"
  OUTPUT_PATH="${OUT_DIR}/lesser-${GOOS}-${GOARCH}"
  echo "Building ${OUTPUT_PATH}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -trimpath -ldflags="-s -w" -o "${OUTPUT_PATH}" ./cmd/lesser
done

go run ./tools/release_assets \
  --repo-root "${ROOT_DIR}" \
  --out-dir "${OUT_DIR}" \
  --version "${VERSION}" \
  --git-sha "${GIT_SHA}" \
  --go-version "${GO_VERSION}" \
  --cdk-major "${CDK_MAJOR}" \
  --receipt-schema-version "${RECEIPT_SCHEMA_VERSION}"

echo "Wrote release assets to ${OUT_DIR}"
