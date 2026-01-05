#!/usr/bin/env bash
set -euo pipefail

# Keep these versions in sync with .github/workflows/ci.yml.
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4

