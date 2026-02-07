#!/usr/bin/env bash
set -euo pipefail

label="${1:-}"

if [[ -n "${label}" ]]; then
  echo "=== Disk usage (${label}) ==="
else
  echo "=== Disk usage ==="
fi
echo

echo "Date (UTC): $(date -u +'%Y-%m-%dT%H:%M:%SZ')"
echo "Workspace: ${GITHUB_WORKSPACE:-$(pwd)}"
echo

echo "Filesystem:"
df -h .
echo

echo "Go cache locations:"
if command -v go >/dev/null 2>&1; then
  go env GOCACHE GOMODCACHE GOPATH
else
  echo "go not on PATH"
fi
echo

echo "Key directory sizes:"
du -sh \
  bin \
  report \
  tmp \
  tmp/go-cache \
  tmp/xdg-cache \
  "${HOME}/go/pkg/mod" \
  "${HOME}/.cache/go-build" \
  2>/dev/null || true
echo

echo "Largest workspace entries (top 15):"
du -x -h -d 2 . 2>/dev/null | sort -hr | head -n 15 || true
echo
