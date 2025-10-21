#!/usr/bin/env bash
set -euo pipefail

# Generate consolidated text documentation using go doc for embedding.
# Output: docs/godoc.txt

OUT="docs/godoc.txt"

# Use local caches (works in sandboxed CI)
export GOCACHE="$(pwd)/.gocache"
export GOMODCACHE="$(pwd)/.gomodcache"

echo "[godoc] generating $OUT"
mkdir -p docs
>"$OUT"

# Prefer go list to enumerate packages; fall back to scanning directories.
mapfile -t pkgs < <(go list -f '{{.Dir}}' ./pkg/... 2>/dev/null || true)
if [ ${#pkgs[@]} -eq 0 ]; then
  # Fallback: find directories with go files
  while IFS= read -r d; do
    if ls "$d"/*.go >/dev/null 2>&1; then
      pkgs+=("$d")
    fi
  done < <(find pkg -type d -not -path '*/vendor/*' -not -path '*/examples/*')
fi

for p in "${pkgs[@]}"; do
  echo "==== $p ====" >> "$OUT"
  # Summaries first (use local package path)
  (go doc "./$p" 2>/dev/null || true) | sed '/^$/d' >> "$OUT" || true
  echo "" >> "$OUT"
  # Full exported API
  (go doc -all "./$p" 2>/dev/null || true) >> "$OUT" || true
  echo -e "\n\n" >> "$OUT"
done

echo "[godoc] complete: $OUT"
