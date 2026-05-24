#!/usr/bin/env bash
set -euo pipefail

# CSR-032 regression probe: verify that processor_storm_recovery_snapshot.sh
# does not contain unsafe environment-variable dump patterns. Fails with a
# non-zero exit if any forbidden pattern is found on non-comment lines.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SNAPSHOT="$SCRIPT_DIR/processor_storm_recovery_snapshot.sh"

# Strip comment-only lines and blank lines from the script, then check
# the remaining active lines for forbidden patterns.
active_lines() {
  grep -v '^\s*#' "$1" | grep -v '^\s*$' || true
}

violations=0
report() {
  echo "  CSR-032 VIOLATION: $*" >&2
  violations=$((violations + 1))
}

# Rule 1: no printenv commands on active lines
if active_lines "$SNAPSHOT" | grep -qi '\bprintenv\b'; then
  report "printenv command found"
fi

# Rule 2: no 'env >' redirect (dumping env to file)
if active_lines "$SNAPSHOT" | grep -qE '\benv\b.*>' ; then
  report "env redirect found"
fi

# Rule 3: no declare -x or declare -p
if active_lines "$SNAPSHOT" | grep -qiE '\bdeclare\s+-[xp]'; then
  report "declare -x or declare -p found"
fi

# Rule 4: no compgen -v
if active_lines "$SNAPSHOT" | grep -qiE '\bcompgen\s+-v'; then
  report "compgen -v found"
fi

# Rule 5: lambda list-functions output must strip Environment via jq del
# Every list-functions call must have del(.Environment) within the same pipe
list_func_lines=$(grep -n 'list-functions' "$SNAPSHOT" | grep -v ':[[:space:]]*#' || true)
if [[ -n "$list_func_lines" ]]; then
  while IFS= read -r line; do
    line_no=$(echo "$line" | cut -d: -f1)
    has_del=""
    for offset in 0 1 2; do
      check_line=$((line_no + offset))
      if sed -n "${check_line}p" "$SNAPSHOT" | grep -q 'del(.Environment)'; then
        has_del="yes"
        break
      fi
    done
    if [[ -z "$has_del" ]]; then
      report "list-functions at line $line_no not followed by jq del(.Environment) within 2 lines"
    fi
  done <<< "$list_func_lines"
fi

# Summary
if [[ $violations -gt 0 ]]; then
  echo "CSR-032 PROBE FAILED: $violations unsafe pattern(s) found in $SNAPSHOT" >&2
  exit 1
fi

echo "CSR-032 probe passed: no unsafe env-dump patterns in $SNAPSHOT"
