#!/bin/bash

# Script to check for stub implementations in the codebase
# Usage: ./check_stub_implementations.sh

echo "=== Stub Implementation Checker ==="
echo "Checking for incomplete implementations in the codebase..."
echo ""

# Create temporary file for results
RESULTS_FILE=$(mktemp)

# Function to count occurrences
count_pattern() {
    local pattern="$1"
    local description="$2"
    local count=$(grep -r "$pattern" --include="*.go" --include="*.ts" --include="*.js" . 2>/dev/null | wc -l)
    echo "- $description: $count instances"
    echo "$description|$count" >> "$RESULTS_FILE"
}

echo "## Pattern Analysis"
echo ""

echo "### 'For now' patterns:"
count_pattern "// For now.*return.*empty" "Empty returns with 'For now' comment"
count_pattern "// For now.*return" "Any 'For now' returns"
count_pattern "For now.*empty" "General 'For now' empty patterns"

echo ""
echo "### TODO patterns:"
count_pattern "// TODO:" "TODO comments"
count_pattern "TODO:" "All TODO markers"
count_pattern "// TODO.*[Ii]mplement" "TODO: Implement patterns"

echo ""
echo "### Not implemented patterns:"
count_pattern "not implemented" "Not implemented (case insensitive)"
count_pattern "NotImplemented" "NotImplemented errors"
count_pattern "panic.*not implemented" "Panic with not implemented"

echo ""
echo "### Placeholder/Stub patterns:"
count_pattern "placeholder" "Placeholder mentions"
count_pattern "dummy" "Dummy implementations"
count_pattern "stub" "Stub mentions"
count_pattern "would normally" "Would normally patterns"

echo ""
echo "### Empty return patterns:"
count_pattern "return \[\]map\[string\]any{}, nil" "Empty map slice returns"
count_pattern "return \[\]string{}, nil" "Empty string slice returns"
count_pattern "return \[\]any{}, nil" "Empty interface slice returns"
count_pattern "return nil, nil" "Double nil returns"

echo ""
echo "## File-specific Analysis"
echo ""

echo "### Critical files with stub implementations:"
echo ""

# Check specific critical files
check_file() {
    local file="$1"
    local description="$2"
    if [ -f "$file" ]; then
        local stubs=$(grep -n "For now\|TODO\|not implemented\|placeholder\|would normally" "$file" 2>/dev/null | wc -l)
        if [ $stubs -gt 0 ]; then
            echo "- $description ($file): $stubs potential stubs"
            grep -n "For now\|TODO\|not implemented\|placeholder\|would normally" "$file" 2>/dev/null | head -5 | sed 's/^/    Line /'
            if [ $stubs -gt 5 ]; then
                echo "    ... and $((stubs - 5)) more"
            fi
            echo ""
        fi
    fi
}

check_file "cmd/api/handlers/imports.go" "Import Handler"
check_file "cmd/api/handlers/exports.go" "Export Handler"
check_file "cmd/export-generator/main.go" "Export Generator"
check_file "graph/schema.resolvers.go" "GraphQL Resolvers"
check_file "pkg/storage/dynamodb/trends.go" "Trends Storage"

echo ""
echo "## Summary Statistics"
echo ""

# Calculate totals
total=0
while IFS='|' read -r description count; do
    total=$((total + count))
done < "$RESULTS_FILE"

echo "Total potential stub implementations found: $total"
echo ""

# Find files with most stubs
echo "### Top 10 files with most stub indicators:"
grep -r "For now\|TODO\|not implemented\|placeholder\|would normally\|return \[\].*{}, nil" \
    --include="*.go" --include="*.ts" --include="*.js" . 2>/dev/null | \
    cut -d: -f1 | sort | uniq -c | sort -rn | head -10 | \
    while read count file; do
        echo "  $count stubs: $file"
    done

echo ""
echo "### Recommended Actions:"
echo "1. Review files with high stub counts"
echo "2. Prioritize fixing functions that return empty data"
echo "3. Replace panics with proper error handling"
echo "4. Implement TODO items based on priority"

# Cleanup
rm -f "$RESULTS_FILE"

echo ""
echo "Report generated at: $(date)" 