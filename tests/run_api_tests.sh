#!/bin/bash
#
# Run Comprehensive API Tests for Lesser
#

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
INSTANCE_URL="${LESSER_URL:-https://lesser.host}"
TEST_USER="${LESSER_TEST_USER:-testuser}"
TEST_PASS="${LESSER_TEST_PASS:-testpass123}"

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="$SCRIPT_DIR/../test_venv"

echo -e "${BLUE}🧪 Lesser Comprehensive API Test Runner${NC}"
echo -e "${BLUE}=====================================>${NC}"
echo ""

# Check if instance URL is provided as argument
if [ $# -ge 1 ]; then
    INSTANCE_URL="$1"
fi

echo -e "${YELLOW}Instance URL:${NC} $INSTANCE_URL"
if [ -n "$LESSER_TOKEN" ]; then
    echo -e "${YELLOW}Auth Method:${NC} Using existing LESSER_TOKEN"
else
    echo -e "${YELLOW}Auth Method:${NC} OAuth flow with user/pass"
    echo -e "${YELLOW}Test User:${NC} $TEST_USER"
fi
echo ""

# Setup virtual environment if needed
if [ ! -d "$VENV_DIR" ]; then
    echo -e "${YELLOW}Setting up test environment...${NC}"
    "$SCRIPT_DIR/setup_test_env.sh"
fi

# Activate virtual environment
echo -e "${BLUE}Activating virtual environment...${NC}"
source "$VENV_DIR/bin/activate"

# Export environment variables
export LESSER_URL="$INSTANCE_URL"
export LESSER_TEST_USER="$TEST_USER"
export LESSER_TEST_PASS="$TEST_PASS"

# Create reports directory
REPORT_DIR="test-reports-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$REPORT_DIR"

echo -e "${BLUE}Running comprehensive API tests...${NC}"
echo ""

# Run the comprehensive test
python "$SCRIPT_DIR/api/comprehensive_api_test.py" "$INSTANCE_URL" 2>&1 | tee "$REPORT_DIR/api-comprehensive.log"

# Check exit code
EXIT_CODE=${PIPESTATUS[0]}

# Move result files to report directory
if ls api-test-results-*.json 1> /dev/null 2>&1; then
    mv api-test-results-*.json "$REPORT_DIR/"
fi

echo ""
echo -e "${BLUE}Test reports saved to: ${YELLOW}$REPORT_DIR${NC}"

# Run analysis on the results
echo ""
echo -e "${BLUE}Analyzing test results...${NC}"
if ls "$REPORT_DIR"/api-test-results-*.json 1> /dev/null 2>&1; then
    python "$SCRIPT_DIR/analyze_test_results.py" "$REPORT_DIR"/api-test-results-*.json
fi

# Summary
if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✅ All API tests passed!${NC}"
else
    echo -e "${RED}❌ Some API tests failed. Check the logs for details.${NC}"
fi

# Deactivate virtual environment
deactivate

exit $EXIT_CODE 