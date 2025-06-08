#!/bin/bash

# Lesser Test Suite Setup and Run Script
# This script will set up everything needed to run the Lesser tests

set -e  # Exit on error

echo "🚀 Lesser Test Suite Setup"
echo "========================="

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
LESSER_URL="${LESSER_URL:-http://localhost:8080}"
VENV_DIR="venv"

# Step 1: Create and activate virtual environment
echo -e "\n${YELLOW}Step 1: Setting up Python virtual environment...${NC}"
if [ ! -d "$VENV_DIR" ]; then
    python3 -m venv $VENV_DIR
    echo -e "${GREEN}✓ Virtual environment created${NC}"
else
    echo -e "${GREEN}✓ Virtual environment already exists${NC}"
fi

# Activate virtual environment
source $VENV_DIR/bin/activate
echo -e "${GREEN}✓ Virtual environment activated${NC}"

# Step 2: Install dependencies
echo -e "\n${YELLOW}Step 2: Installing dependencies...${NC}"
pip install --upgrade pip > /dev/null 2>&1
pip install -r requirements.txt
echo -e "${GREEN}✓ Dependencies installed${NC}"

# Step 3: Check if Lesser is running
echo -e "\n${YELLOW}Step 3: Checking Lesser instance...${NC}"
if curl -s -f "$LESSER_URL/api/v1/instance" > /dev/null; then
    echo -e "${GREEN}✓ Lesser is running at $LESSER_URL${NC}"
else
    echo -e "${RED}✗ Lesser is not running at $LESSER_URL${NC}"
    echo "Please start Lesser or set LESSER_URL environment variable"
    exit 1
fi

# Step 4: Generate authentication token
echo -e "\n${YELLOW}Step 4: Authentication Setup${NC}"
echo "Choose an option:"
echo "1) Create new test user and generate token"
echo "2) Use existing user credentials"
echo "3) Skip (I already have a token)"

read -p "Choice (1-3): " choice

case $choice in
    1)
        echo -e "\n${YELLOW}Creating new test user...${NC}"
        read -p "Username: " username
        read -p "Email: " email
        read -s -p "Password: " password
        echo
        
        # Run token generator in command line mode
        output=$(python utilities/generate_auth_token.py "$username" "$password" "$LESSER_URL" 2>&1)
        
        if echo "$output" | grep -q "Create new user? (y/N):"; then
            # User doesn't exist, create it
            echo "y" | python utilities/generate_auth_token.py "$username" "$password" "$LESSER_URL" > token_output.txt 2>&1
        else
            python utilities/generate_auth_token.py "$username" "$password" "$LESSER_URL" > token_output.txt 2>&1
        fi
        
        # Extract token from output
        export LESSER_AUTH_TOKEN=$(grep "Access Token:" token_output.txt | tail -1 | cut -d' ' -f3)
        rm -f token_output.txt
        
        if [ -n "$LESSER_AUTH_TOKEN" ]; then
            echo -e "${GREEN}✓ Token generated successfully${NC}"
        else
            echo -e "${RED}✗ Failed to generate token${NC}"
            exit 1
        fi
        ;;
    2)
        echo -e "\n${YELLOW}Using existing user...${NC}"
        read -p "Username: " username
        read -s -p "Password: " password
        echo
        
        # Generate token
        output=$(python utilities/generate_auth_token.py "$username" "$password" "$LESSER_URL" 2>&1)
        export LESSER_AUTH_TOKEN=$(echo "$output" | grep "Access Token:" | tail -1 | cut -d' ' -f3)
        
        if [ -n "$LESSER_AUTH_TOKEN" ]; then
            echo -e "${GREEN}✓ Token generated successfully${NC}"
        else
            echo -e "${RED}✗ Failed to generate token${NC}"
            echo "Error output: $output"
            exit 1
        fi
        ;;
    3)
        if [ -z "$LESSER_AUTH_TOKEN" ]; then
            read -p "Enter your access token: " LESSER_AUTH_TOKEN
            export LESSER_AUTH_TOKEN
        fi
        echo -e "${GREEN}✓ Using existing token${NC}"
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        exit 1
        ;;
esac

# Step 5: Run tests
echo -e "\n${YELLOW}Step 5: Running Tests${NC}"
echo "Choose test suite:"
echo "1) Run all tests (comprehensive)"
echo "2) API tests only"
echo "3) Federation tests only"
echo "4) Performance benchmark"
echo "5) Load test (requires k6)"

read -p "Choice (1-5): " test_choice

export LESSER_URL
export LESSER_AUTH_TOKEN

case $test_choice in
    1)
        echo -e "\n${YELLOW}Running comprehensive test suite...${NC}"
        ./run_comprehensive_validation.sh
        ;;
    2)
        echo -e "\n${YELLOW}Running API tests...${NC}"
        # First need to get OAuth credentials
        echo "Getting OAuth credentials..."
        
        # Create a simple Python script to extract client credentials
        cat > get_oauth_creds.py << 'EOF'
import os
import requests
import json

base_url = os.getenv('LESSER_URL', 'http://localhost:8080')
auth_token = os.getenv('LESSER_AUTH_TOKEN')

# Create OAuth app
response = requests.post(f"{base_url}/api/v1/apps", json={
    "client_name": "API Test Suite",
    "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
    "scopes": "read write follow push admin"
})

if response.status_code == 200:
    app = response.json()
    print(f"export LESSER_CLIENT_ID='{app['client_id']}'")
    print(f"export LESSER_CLIENT_SECRET='{app['client_secret']}'")
else:
    print(f"echo 'Failed to create OAuth app: {response.status_code}'")
EOF
        
        eval $(python get_oauth_creds.py)
        rm -f get_oauth_creds.py
        
        # Now run the test with credentials
        python integration/comprehensive_api_test.py "$LESSER_URL" "$LESSER_CLIENT_ID" "$LESSER_CLIENT_SECRET"
        ;;
    3)
        echo -e "\n${YELLOW}Running federation tests...${NC}"
        python integration/federation_validation_test.py
        ;;
    4)
        echo -e "\n${YELLOW}Running performance benchmark...${NC}"
        python integration/performance_benchmark.py
        ;;
    5)
        echo -e "\n${YELLOW}Running load test...${NC}"
        if ! command -v k6 &> /dev/null; then
            echo -e "${RED}✗ k6 is not installed${NC}"
            echo "Install with: brew install k6 (macOS) or see https://k6.io"
            exit 1
        fi
        k6 run integration/lesser_load_test.js
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        exit 1
        ;;
esac

echo -e "\n${GREEN}✓ Test execution complete!${NC}"
echo -e "\nYour environment variables are set:"
echo "LESSER_URL=$LESSER_URL"
echo "LESSER_AUTH_TOKEN=${LESSER_AUTH_TOKEN:0:20}..."

# Deactivate virtual environment
deactivate

echo -e "\n${YELLOW}To run tests again, you can use:${NC}"
echo "source venv/bin/activate"
echo "export LESSER_URL='$LESSER_URL'"
echo "export LESSER_AUTH_TOKEN='$LESSER_AUTH_TOKEN'"
echo "python integration/comprehensive_api_test.py" 