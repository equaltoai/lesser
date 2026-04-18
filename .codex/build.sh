#!/usr/bin/env bash
# Compose the stewardship prompt from layered sources.
#
# Authoring happens in stack/*.md. This script concatenates them in filename
# order into steward.md, which is the file Codex actually loads via
# model_instructions_file in config.toml.
#
# Rebuild after editing any stack layer:
#   ./.codex/build.sh

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
stack_dir="${here}/stack"
out_file="${here}/steward.md"

if [[ ! -d "${stack_dir}" ]]; then
  echo "build.sh: no stack directory at ${stack_dir}" >&2
  exit 1
fi

shopt -s nullglob
layers=("${stack_dir}"/*.md)
if (( ${#layers[@]} == 0 )); then
  echo "build.sh: no stack layers found in ${stack_dir}" >&2
  exit 1
fi

{
  for layer in "${layers[@]}"; do
    cat "${layer}"
    printf '\n'
  done
} > "${out_file}"

echo "build.sh: wrote ${out_file} from ${#layers[@]} layers"
