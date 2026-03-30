#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

RELEASE_DIR="${1:-}"

resolve_host_release_binary() {
  local host_os host_arch

  case "$(uname -s)" in
    Linux) host_os="linux" ;;
    Darwin) host_os="darwin" ;;
    *)
      echo "unsupported host OS for release certification: $(uname -s)" >&2
      return 1
      ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) host_arch="amd64" ;;
    arm64|aarch64) host_arch="arm64" ;;
    *)
      echo "unsupported host architecture for release certification: $(uname -m)" >&2
      return 1
      ;;
  esac

  printf '%s/lesser-%s-%s\n' "${RELEASE_DIR}" "${host_os}" "${host_arch}"
}

if [[ -n "${RELEASE_DIR}" ]]; then
  HOST_RELEASE_BINARY="$(resolve_host_release_binary)"
  if [[ ! -f "${HOST_RELEASE_BINARY}" ]]; then
    echo "missing host release binary: ${HOST_RELEASE_BINARY}" >&2
    exit 1
  fi

  echo "==> Artifact-driven deploy certification (published release)"
  "${HOST_RELEASE_BINARY}" verify artifact-deploy --release-dir "${RELEASE_DIR}"
fi

echo "==> Artifact-driven deploy certification (cmd/lesser)"
go test ./cmd/lesser -run 'TestInstallReleaseLambdaAssets_AcceptsCanonicalInventoryDeclaredOutOfOrder|TestRunUp_UsesVerifiedReleaseDirWithoutBuildingLambdas|TestRunUp_ReleaseDirPropagatesArtifactRootAcrossSharedAndStageDeploys|TestRunUp_ReleaseDirUsesRealCdkDeployPreparationWithoutRepoBin' -count=1

echo "==> Artifact-driven deploy certification (infra/cdk)"
(
  cd infra/cdk
  go test ./stacks -run 'TestLesserApiStack(LambdaFunctionsPropsUsesConfiguredLambdaAssetRoot|SynthUsesConfiguredLambdaAssetRootWithoutRepoBin)' -count=1
)

echo "✅ Artifact-driven deploy certification passed"
