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

# Keep these versions pinned and in sync with CI + local tooling.
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1
go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
