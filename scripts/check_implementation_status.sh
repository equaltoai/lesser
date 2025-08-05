#!/bin/bash

# Script to check implementation status of incomplete features

echo "=== Lesser Implementation Status Check ==="
echo "Date: $(date)"
echo

# Check for "not implemented" errors
echo "1. Checking for 'not implemented' errors..."
NOT_IMPL_COUNT=$(grep -r "not implemented" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" | wc -l)
echo "   Found: $NOT_IMPL_COUNT instances"
echo

# Check for TODO comments
echo "2. Checking for TODO comments..."
TODO_COUNT=$(grep -r "TODO" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" | wc -l)
echo "   Found: $TODO_COUNT instances"
echo

# Check for context.TODO()
echo "3. Checking for context.TODO() usage..."
CONTEXT_TODO_COUNT=$(grep -r "context.TODO()" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" | wc -l)
echo "   Found: $CONTEXT_TODO_COUNT instances"
echo

# Check authentication repository
echo "4. Checking authentication repository methods..."
AUTH_NOT_IMPL=$(grep -c "not implemented" pkg/storage/repositories/account_repository_auth.go 2>/dev/null || echo 0)
echo "   Not implemented methods: $AUTH_NOT_IMPL"
echo

# Check for cursor pagination TODOs
echo "5. Checking for pagination TODOs..."
PAGINATION_TODO=$(grep -r "cursor-based pagination" --include="*.go" pkg/storage/repositories/ 2>/dev/null | wc -l)
echo "   Found: $PAGINATION_TODO instances"
echo

# Check GraphQL resolvers
echo "6. Checking GraphQL resolver TODOs..."
GRAPHQL_TODO=$(grep -c "TODO" graph/schema.resolvers.go graph/phase2_resolvers.go 2>/dev/null | grep -v "total" | paste -sd+ | bc)
echo "   Found: $GRAPHQL_TODO instances"
echo

# Check for return nil, nil patterns (potential incomplete implementations)
echo "7. Checking for 'return nil, nil' patterns..."
RETURN_NIL_NIL=$(grep -r "return nil, nil" --include="*.go" pkg/storage/repositories/ 2>/dev/null | wc -l)
echo "   Found: $RETURN_NIL_NIL instances (review needed)"
echo

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
if [ -f .implementation_status_last ]; then
    LAST_TOTAL=$(cat .implementation_status_last)
    if [ $TOTAL_ISSUES -lt $LAST_TOTAL ]; then
        echo "✅ Progress! Reduced from $LAST_TOTAL to $TOTAL_ISSUES issues"
    elif [ $TOTAL_ISSUES -gt $LAST_TOTAL ]; then
        echo "⚠️  Warning! Increased from $LAST_TOTAL to $TOTAL_ISSUES issues"
    else
        echo "No change from last check ($LAST_TOTAL issues)"
    fi
fi

# Save current status
echo $TOTAL_ISSUES > .implementation_status_last

echo
echo "For detailed list, see INCOMPLETE_IMPLEMENTATIONS.md"