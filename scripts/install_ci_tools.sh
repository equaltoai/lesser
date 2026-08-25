#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

toolchain="$(awk '$1 == "toolchain" { print $2; exit }' go.mod || true)"
go_version="$(awk '$1 == "go" { print $2; exit }' go.mod || true)"
if [[ -n "${toolchain}" ]]; then
	export GOTOOLCHAIN="${toolchain}"
elif [[ -n "${go_version}" ]]; then
	export GOTOOLCHAIN="go${go_version}"
fi

# Keep these versions pinned and in sync with CI + local tooling. The gosec
# version is read from cmd/lesser/security_cmd.go (pinnedGosecVersion) so the
# installer and the sec-scan version assertion share one source of truth.
gosec_version="$(awk '$1 == "const" && $2 == "pinnedGosecVersion" || ($1 == "pinnedGosecVersion" && $2 == "=") { v = $NF; gsub(/"/, "", v); print v; exit }' cmd/lesser/security_cmd.go)"
if [[ -z "${gosec_version}" ]]; then
	echo "error: could not read pinnedGosecVersion from cmd/lesser/security_cmd.go" >&2
	exit 1
fi
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
go install github.com/securego/gosec/v2/cmd/gosec@"${gosec_version}"
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
