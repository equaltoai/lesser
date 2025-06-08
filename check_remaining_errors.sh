#!/bin/bash
# Script to analyze remaining API test failures

echo "🔍 Analyzing Remaining API Errors"
echo "================================="
echo ""

# Find the most recent test results file in any directory
LATEST_RESULTS=$(find . -name "api-test-results-*.json" -type f -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null | head -1)

if [ -z "$LATEST_RESULTS" ]; then
    echo "❌ No test results found. Run ./run_api_tests.sh first."
    exit 1
fi

echo "📄 Analyzing: $LATEST_RESULTS"
echo ""

# Extract and categorize failures
echo "❌ Failed Tests by Status Code:"
echo "==============================="

# 404 errors (Not Implemented)
echo ""
echo "📍 404 - Not Implemented:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 404) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# 403 errors (Forbidden)
echo ""
echo "🚫 403 - Forbidden:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 403) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# 422 errors (Unprocessable Entity)
echo ""
echo "⚠️  422 - Unprocessable Entity:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 422) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# 400 errors (Bad Request)
echo ""
echo "❗ 400 - Bad Request:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 400) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# 500 errors (Server Error)
echo ""
echo "💥 500 - Server Error:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 500) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# 501 errors (Not Implemented)
echo ""
echo "🚧 501 - Not Implemented:"
jq -r '.results[] | select(.result == "FAIL" and .status_code == 501) | "  - \(.endpoint): \(.method) \(.endpoint)"' "$LATEST_RESULTS" | sort | uniq

# Summary
echo ""
echo "📊 Summary:"
echo "==========="
TOTAL=$(jq '.results | length' "$LATEST_RESULTS")
PASSED=$(jq '.results | map(select(.result == "PASS")) | length' "$LATEST_RESULTS")
FAILED=$(jq '.results | map(select(.result == "FAIL")) | length' "$LATEST_RESULTS")
SKIPPED=$(jq '.results | map(select(.result == "SKIP")) | length' "$LATEST_RESULTS")

echo "Total: $TOTAL"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "Skipped: $SKIPPED"
echo "Success Rate: $(( $PASSED * 100 / ($PASSED + $FAILED) ))%"

# Show which files likely need fixing
echo ""
echo "📝 Files that likely need fixes:"
echo "================================"
echo "Based on the errors above, check these handler files:"
echo ""
echo "500 Errors (Runtime crashes):"
echo "- moderation.go (GET /moderation/trust)"
echo "- ai.go (GET /ai/stats)"
echo "- reputation.go (GET /reputation/:actor_id)"
echo "- notes.go (GET /accounts/:username/notes)"
echo ""
echo "404 Errors (Not implemented features):"
echo "- bookmarks.go (needs implementation)"
echo "- search.go (suggestions endpoint)"
echo ""
echo "403/422 Errors (Logic/validation issues):"
echo "- statuses.go (pin/unpin permissions)"
echo "- filters.go (permission checks)"
echo "- translation.go (configuration needed)" 