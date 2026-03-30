#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

RELEASE_DIR="${1:-}"
if [[ -n "${RELEASE_DIR}" ]]; then
  for asset in \
    lesser-lambda-bundle.tar.gz \
    lesser-lambda-bundle.json \
    lesser-release.json \
    checksums.txt
  do
    if [[ ! -f "${RELEASE_DIR}/${asset}" ]]; then
      echo "missing required release asset: ${RELEASE_DIR}/${asset}" >&2
      exit 1
    fi
  done
fi

echo "==> Artifact-driven deploy certification (cmd/lesser)"
go test ./cmd/lesser -run 'TestInstallReleaseLambdaAssets_AcceptsCanonicalInventoryDeclaredOutOfOrder|TestRunUp_UsesVerifiedReleaseDirWithoutBuildingLambdas|TestRunUp_ReleaseDirPropagatesArtifactRootAcrossSharedAndStageDeploys|TestRunUp_ReleaseDirUsesRealCdkDeployPreparationWithoutRepoBin' -count=1

echo "==> Artifact-driven deploy certification (infra/cdk)"
(
  cd infra/cdk
  go test ./stacks -run 'TestLesserApiStack(LambdaFunctionsPropsUsesConfiguredLambdaAssetRoot|SynthUsesConfiguredLambdaAssetRootWithoutRepoBin)' -count=1
)

echo "✅ Artifact-driven deploy certification passed"
