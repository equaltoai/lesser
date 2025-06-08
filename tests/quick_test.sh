#!/bin/bash
# Quick test runner for Lesser

echo "🚀 Lesser Quick Test Runner"
echo "=========================="

# Setup
LESSER_URL="${LESSER_URL:-http://localhost:8080}"
cd "$(dirname "$0")"

# Create venv if needed
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
fi

# Activate venv and install deps
source venv/bin/activate
pip install -q -r requirements.txt

# Check if we have a token
if [ -z "$LESSER_AUTH_TOKEN" ]; then
    echo ""
    echo "No auth token found. Let's create one:"
    echo "1) Use existing user"
    echo "2) Skip (run without auth)"
    read -p "Choice (1-2): " choice
    
    if [ "$choice" = "1" ]; then
        read -p "Username: " username
        read -s -p "Password: " password
        echo ""
        
        # Get token
        token=$(python utilities/generate_auth_token.py "$username" "$password" "$LESSER_URL" 2>&1 | grep "Access Token:" | tail -1 | awk '{print $3}')
        
        if [ -n "$token" ]; then
            export LESSER_AUTH_TOKEN="$token"
            echo "✅ Token generated!"
        else
            echo "❌ Failed to generate token"
            exit 1
        fi
    fi
fi

# Run tests
echo ""
echo "Select test to run:"
echo "1) API tests (requires auth)"
echo "2) Federation tests" 
echo "3) Performance benchmark"
echo "4) All tests"
read -p "Choice (1-4): " test_choice

case $test_choice in
    1)
        echo "Running API tests..."
        # Create OAuth app and get credentials
        python -c "
import os, requests, sys
url = os.getenv('LESSER_URL', 'http://localhost:8080')
r = requests.post(f'{url}/api/v1/apps', json={'client_name': 'Test', 'redirect_uris': 'urn:ietf:wg:oauth:2.0:oob', 'scopes': 'read write follow push'})
if r.status_code == 200:
    app = r.json()
    sys.exit(os.system(f'python integration/comprehensive_api_test.py \"{url}\" \"{app[\"client_id\"]}\" \"{app[\"client_secret\"]}\"'))
else:
    print(f'Failed to create app: {r.status_code}')
    sys.exit(1)
"
        ;;
    2)
        python integration/federation_validation_test.py
        ;;
    3)
        python integration/performance_benchmark.py
        ;;
    4)
        ./run_comprehensive_validation.sh
        ;;
esac

deactivate 