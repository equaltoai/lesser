#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

bash scripts/verify_auth_ui_csp.sh
bash scripts/install_ci_tools.sh
go build -o lesser ./cmd/lesser
./lesser build lambdas
./lesser verify ci
bash scripts/verify_artifact_deploy.sh
