#!/bin/bash

# Script to check implementation status of incomplete features

echo "=== Lesser Implementation Status Check ==="
echo "Date: $(date)"
echo

# Report file for detailed results
DETAIL_FILE="report/incomplete_implementations.md"
STATUS_FILE="report/.implementation_status_last"
mkdir -p "$(dirname "$DETAIL_FILE")"

# Truncate report file and write header
{
    echo "# Incomplete Implementations Report"
    echo
    echo "_Generated on $(date)_"
    echo
} > "$DETAIL_FILE"

# Check for "not implemented" errors
echo "1. Checking for 'not implemented' errors..."
mapfile -t NOT_IMPL_LIST < <(grep -r -n "not implemented" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" || true)
NOT_IMPL_COUNT=${#NOT_IMPL_LIST[@]}
echo "   Found: $NOT_IMPL_COUNT instances"
echo
{
    echo "## \"not implemented\" occurrences ($NOT_IMPL_COUNT)"
    if [ "$NOT_IMPL_COUNT" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${NOT_IMPL_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Check for TODO comments
echo "2. Checking for TODO comments..."
mapfile -t TODO_LIST < <(grep -r -n "TODO" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" || true)
TODO_COUNT=${#TODO_LIST[@]}
echo "   Found: $TODO_COUNT instances"
echo
{
    echo "## TODO comments ($TODO_COUNT)"
    if [ "$TODO_COUNT" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${TODO_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Check for context.TODO()
echo "3. Checking for context.TODO() usage..."
mapfile -t CONTEXT_TODO_LIST < <(grep -r -n "context.TODO()" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" || true)
CONTEXT_TODO_COUNT=${#CONTEXT_TODO_LIST[@]}
echo "   Found: $CONTEXT_TODO_COUNT instances"
echo
{
    echo "## context.TODO() occurrences ($CONTEXT_TODO_COUNT)"
    if [ "$CONTEXT_TODO_COUNT" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${CONTEXT_TODO_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Check authentication repository
echo "4. Checking authentication repository methods..."
if [ -f "pkg/storage/repositories/account_repository_auth.go" ]; then
    mapfile -t AUTH_NOT_IMPL_LIST < <(grep -n "not implemented" pkg/storage/repositories/account_repository_auth.go 2>/dev/null || true)
    AUTH_NOT_IMPL=${#AUTH_NOT_IMPL_LIST[@]}
else
    AUTH_NOT_IMPL=0
    AUTH_NOT_IMPL_LIST=()
fi
echo "   Not implemented methods: $AUTH_NOT_IMPL"
echo
{
    echo "## Authentication repository gaps ($AUTH_NOT_IMPL)"
    if [ "$AUTH_NOT_IMPL" -eq 0 ]; then
        echo "_None found._"
    else
        printf 'pkg/storage/repositories/account_repository_auth.go:%s\n' "${AUTH_NOT_IMPL_LIST[@]}"
    fi
    echo
} >> "$DETAIL_FILE"

# Check for cursor pagination TODOs
echo "5. Checking for pagination TODOs..."
mapfile -t PAGINATION_LIST < <(grep -r -n "cursor-based pagination" --include="*.go" pkg/storage/repositories/ 2>/dev/null || true)
PAGINATION_TODO=${#PAGINATION_LIST[@]}
echo "   Found: $PAGINATION_TODO instances"
echo
{
    echo "## Pagination TODO markers ($PAGINATION_TODO)"
    if [ "$PAGINATION_TODO" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${PAGINATION_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Check GraphQL resolvers
echo "6. Checking GraphQL resolver TODOs..."
GRAPHQL_TODO=0
GRAPHQL_LIST=()
GRAPHQL_FILES=(
    "graph/schema.resolvers.go"
    "graph/phase2_resolvers.go"
)
for file in "${GRAPHQL_FILES[@]}"; do
    if [ -f "$file" ]; then
        mapfile -t CURRENT_GRAPHQL_LIST < <(grep -nH "TODO" "$file" 2>/dev/null || true)
        GRAPHQL_TODO=$((GRAPHQL_TODO + ${#CURRENT_GRAPHQL_LIST[@]}))
        GRAPHQL_LIST+=("${CURRENT_GRAPHQL_LIST[@]}")
    fi
done
echo "   Found: $GRAPHQL_TODO instances"
echo
{
    echo "## GraphQL TODOs ($GRAPHQL_TODO)"
    if [ "$GRAPHQL_TODO" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${GRAPHQL_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Check for return nil, nil patterns (potential incomplete implementations)
echo "7. Checking for 'return nil, nil' patterns..."
mapfile -t RETURN_NIL_LIST < <(grep -r -n "return nil, nil" --include="*.go" pkg/storage/repositories/ 2>/dev/null || true)
RETURN_NIL_NIL=${#RETURN_NIL_LIST[@]}
echo "   Found: $RETURN_NIL_NIL instances (review needed)"
echo
{
    echo "## \"return nil, nil\" patterns ($RETURN_NIL_NIL)"
    if [ "$RETURN_NIL_NIL" -eq 0 ]; then
        echo "_None found._"
    else
        printf '%s\n' "${RETURN_NIL_LIST[@]}" | sort
    fi
    echo
} >> "$DETAIL_FILE"

# Summary
echo "=== SUMMARY ==="
TOTAL_ISSUES=$((NOT_IMPL_COUNT + TODO_COUNT + CONTEXT_TODO_COUNT + PAGINATION_TODO))
echo "Total incomplete implementations: ~$TOTAL_ISSUES"
echo
echo "Priority areas:"
echo "- Authentication methods: $AUTH_NOT_IMPL not implemented"
echo "- Pagination: $PAGINATION_TODO instances need implementation"
echo "- GraphQL: $GRAPHQL_TODO TODOs"
echo "- Context: $CONTEXT_TODO_COUNT context.TODO() to fix"
echo

# Check if getting better or worse
if [ -f "$STATUS_FILE" ]; then
    LAST_TOTAL=$(cat "$STATUS_FILE")
    if [ $TOTAL_ISSUES -lt $LAST_TOTAL ]; then
        echo "✅ Progress! Reduced from $LAST_TOTAL to $TOTAL_ISSUES issues"
    elif [ $TOTAL_ISSUES -gt $LAST_TOTAL ]; then
        echo "⚠️  Warning! Increased from $LAST_TOTAL to $TOTAL_ISSUES issues"
    else
        echo "No change from last check ($LAST_TOTAL issues)"
    fi
fi

# Save current status
echo $TOTAL_ISSUES > "$STATUS_FILE"

echo
echo "For detailed list, see $DETAIL_FILE"
