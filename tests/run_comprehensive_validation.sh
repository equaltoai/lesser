#!/bin/bash
#
# Comprehensive Validation Suite for Lesser
# This script runs all validation tests to ensure Lesser is working correctly
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTANCE_URL="${LESSER_URL:-https://lesser.example.com}"
ACCESS_TOKEN="${LESSER_AUTH_TOKEN:-}"
REPORT_DIR="test-reports-$(date +%Y%m%d-%H%M%S)"

# Check for virtual environment and activate if available
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="$SCRIPT_DIR/../test_venv"

if [ -d "$VENV_DIR" ]; then
    echo -e "${BLUE}Activating virtual environment...${NC}"
    source "$VENV_DIR/bin/activate"
elif [ -d "$SCRIPT_DIR/venv" ]; then
    echo -e "${BLUE}Activating virtual environment...${NC}"
    source "$SCRIPT_DIR/venv/bin/activate"
else
    echo -e "${YELLOW}⚠ No virtual environment found. Python tests may fail if dependencies are not installed globally.${NC}"
    echo -e "${YELLOW}  Run 'python3 -m venv test_venv && test_venv/bin/pip install -r tests/requirements.txt' to set up.${NC}"
fi

# Create report directory
mkdir -p "$REPORT_DIR"

echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║          Lesser Comprehensive Validation Suite              ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}Instance URL:${NC} $INSTANCE_URL"
echo -e "${YELLOW}Report Directory:${NC} $REPORT_DIR"
echo ""

# Function to run a test and capture results
run_test() {
    local test_name=$1
    local test_command=$2
    local report_file="$REPORT_DIR/${test_name}.log"
    
    echo -e "${BLUE}Running:${NC} $test_name"
    
    if eval "$test_command" > "$report_file" 2>&1; then
        echo -e "${GREEN}✓${NC} $test_name passed"
        return 0
    else
        echo -e "${RED}✗${NC} $test_name failed (see $report_file)"
        return 1
    fi
}

# Track overall results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 1. Check instance is reachable
echo -e "\n${YELLOW}1. Basic Connectivity Tests${NC}"
echo "================================"

TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "instance-reachable" "curl -s -o /dev/null -w '%{http_code}' '$INSTANCE_URL/api/v1/instance' | grep -q 200"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
    echo -e "${RED}Cannot reach instance. Aborting.${NC}"
    exit 1
fi

# 2. Run API tests
echo -e "\n${YELLOW}2. API Endpoint Tests${NC}"
echo "================================"

if [ -n "$ACCESS_TOKEN" ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if run_test "api-comprehensive" "python3 tests/api/comprehensive_api_test.py '$INSTANCE_URL' --token '$ACCESS_TOKEN'"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
else
    echo -e "${YELLOW}⚠ Skipping authenticated API tests (no token provided)${NC}"
fi

# 3. Federation tests
echo -e "\n${YELLOW}3. Federation Compliance Tests${NC}"
echo "================================"

TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "federation-validation" "python3 tests/federation/test_federation_validation.py '$INSTANCE_URL'"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 4. WebFinger tests
echo -e "\n${YELLOW}4. WebFinger Tests${NC}"
echo "================================"

TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "webfinger-test" "curl -s '$INSTANCE_URL/.well-known/webfinger?resource=acct:aron@${INSTANCE_URL#https://}' | jq -e '.subject' > /dev/null"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 5. Cost tracking validation
echo -e "\n${YELLOW}5. Cost Tracking Tests${NC}"
echo "================================"

if [ -n "$ACCESS_TOKEN" ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    COST_TEST_CMD="curl -s -H 'Authorization: Bearer $ACCESS_TOKEN' '$INSTANCE_URL/api/v1/timelines/home' -I | grep -qi 'x-cost-total-microcents'"
    if run_test "cost-headers" "$COST_TEST_CMD"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
fi

# 6. Performance baseline
echo -e "\n${YELLOW}6. Performance Baseline Tests${NC}"
echo "================================"

ENDPOINTS=(
    "/api/v1/instance"
    "/api/v2/instance"
    "/.well-known/nodeinfo"
)

for endpoint in "${ENDPOINTS[@]}"; do
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    PERF_CMD="curl -s -o /dev/null -w '%{time_total}' '$INSTANCE_URL$endpoint' | awk '{if (\$1 < 0.5) exit 0; else exit 1}'"
    if run_test "performance-${endpoint//\//-}" "$PERF_CMD"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
done

# 7. Search functionality
echo -e "\n${YELLOW}7. Search Tests${NC}"
echo "================================"

if [ -n "$ACCESS_TOKEN" ]; then
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    SEARCH_CMD="curl -s -H 'Authorization: Bearer $ACCESS_TOKEN' '$INSTANCE_URL/api/v2/search?q=test' | jq -e '.accounts' > /dev/null"
    if run_test "search-v2" "$SEARCH_CMD"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
fi

# 8. Media type negotiation
echo -e "\n${YELLOW}8. Content Negotiation Tests${NC}"
echo "================================"

TOTAL_TESTS=$((TOTAL_TESTS + 1))
CONTENT_NEG_CMD="curl -s -H 'Accept: application/activity+json' '$INSTANCE_URL/users/aron' | jq -e '.type' > /dev/null"
if run_test "content-negotiation" "$CONTENT_NEG_CMD"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
else
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 9. Generate summary report
echo -e "\n${YELLOW}Generating Summary Report${NC}"
echo "================================"

SUMMARY_FILE="$REPORT_DIR/summary.txt"
{
    echo "Lesser Validation Summary"
    echo "========================"
    echo "Instance: $INSTANCE_URL"
    echo "Date: $(date)"
    echo ""
    echo "Results:"
    echo "--------"
    echo "Total Tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $FAILED_TESTS"
    if [ $TOTAL_TESTS -gt 0 ]; then
        SUCCESS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
        echo "Success Rate: ${SUCCESS_RATE}%"
    else
        echo "Success Rate: N/A"
    fi
    echo ""
    
    if [ $FAILED_TESTS -gt 0 ]; then
        echo "Failed Tests:"
        echo "-------------"
        grep -l "failed" "$REPORT_DIR"/*.log 2>/dev/null | while read -r log; do
            basename "$log" .log
        done
    fi
} > "$SUMMARY_FILE"

# Display summary
echo ""
cat "$SUMMARY_FILE"

# 10. Optional load test
if command -v k6 &> /dev/null && [ -n "$ACCESS_TOKEN" ]; then
    echo -e "\n${YELLOW}10. Load Test (Optional)${NC}"
    echo "================================"
    read -p "Run load test? This will create test data (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        K6_CMD="k6 run --env LESSER_URL='$INSTANCE_URL' --env LESSER_TOKEN='$ACCESS_TOKEN' tests/load/lesser_load_test.js"
        run_test "load-test" "$K6_CMD"
    fi
else
    echo -e "\n${YELLOW}Load test skipped (k6 not installed or no token)${NC}"
fi

# Final result
echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${BLUE}║${GREEN}              ✅ ALL TESTS PASSED! ✅                      ${BLUE}║${NC}"
    echo -e "${BLUE}║${GREEN}         Lesser instance is fully operational!              ${BLUE}║${NC}"
else
    echo -e "${BLUE}║${RED}              ❌ SOME TESTS FAILED ❌                      ${BLUE}║${NC}"
    echo -e "${BLUE}║${RED}         Check $REPORT_DIR for details              ${BLUE}║${NC}"
fi
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"

# Exit with appropriate code
exit $FAILED_TESTS 