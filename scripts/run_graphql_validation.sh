#!/bin/bash
# Helper script to run GraphQL validation with proper tokens

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
ENDPOINT="${GRAPHQL_ENDPOINT:-https://dev.lesser.host/api/graphql}"
AWS_PROFILE="${AWS_PROFILE:-Lesser}"

# Find bootstrap directories
BOOTSTRAP_ROOT="$PROJECT_ROOT"
ADMIN_DIR=$(find "$BOOTSTRAP_ROOT" -maxdepth 1 -type d -name "bootstrap_admin_*" | head -1)
MEMBER_DIR=$(find "$BOOTSTRAP_ROOT" -maxdepth 1 -type d -name "bootstrap_member_*" | head -1)
MOD_DIR=$(find "$BOOTSTRAP_ROOT" -maxdepth 1 -type d -name "bootstrap_mod_*" | head -1)

# Function to extract JWT secret from credentials file
extract_jwt_secret() {
    local cred_file="$1/credentials.txt"
    if [ -f "$cred_file" ]; then
        grep "^JWT Secret:" "$cred_file" | cut -d: -f2 | sed 's/^[[:space:]]*//'
    fi
}

# Function to extract username from credentials file
extract_username() {
    local cred_file="$1/credentials.txt"
    if [ -f "$cred_file" ]; then
        grep "^Username:" "$cred_file" | cut -d: -f2 | sed 's/^[[:space:]]*//'
    fi
}

# Function to extract client ID from credentials file
extract_client_id() {
    local cred_file="$1/credentials.txt"
    if [ -f "$cred_file" ]; then
        grep "Client ID:" "$cred_file" | cut -d: -f2 | sed 's/^[[:space:]]*//'
    fi
}

# Function to generate token using Python
generate_token() {
    local username="$1"
    local secret="$2"
    local client_id="$3"
    
    python3 -c "
import sys
import jwt
import time

secret = '$secret'
username = '$username'
client_id = '$client_id'

now = int(time.time())
payload = {
    'sub': username,
    'iat': now,
    'exp': now + 3600,
    'username': username,
    'scopes': ['read', 'write', 'follow', 'push'],
    'client_id': client_id,
}

token = jwt.encode(payload, secret, algorithm='HS256')
print(f'Bearer {token}')
"
}

echo "=== GraphQL Validation Setup ==="
echo "Endpoint: $ENDPOINT"
echo "AWS Profile: $AWS_PROFILE"
echo ""

# Try to get JWT secret from AWS Secrets Manager if not in bootstrap
JWT_SECRET=""
if [ -n "$ADMIN_DIR" ]; then
    JWT_SECRET=$(extract_jwt_secret "$ADMIN_DIR")
fi

if [ -z "$JWT_SECRET" ]; then
    echo "Attempting to get JWT_SECRET from AWS Secrets Manager..."
    JWT_SECRET=$(aws secretsmanager get-secret-value \
        --secret-id "lesser/dev/jwt-secret" \
        --profile "$AWS_PROFILE" \
        --query "SecretString" \
        --output text 2>/dev/null || echo "")
    
    # If secret is JSON, parse it
    if [[ "$JWT_SECRET" =~ ^\{.*\}$ ]]; then
        JWT_SECRET=$(echo "$JWT_SECRET" | python3 -c "import sys, json; print(json.load(sys.stdin).get('secret', ''))")
    fi
fi

if [ -z "$JWT_SECRET" ]; then
    echo "ERROR: Could not find JWT_SECRET"
    echo "Please set JWT_SECRET environment variable or ensure bootstrap directories exist"
    exit 1
fi

echo "✓ JWT Secret found"
echo ""

# Generate tokens
ADMIN_TOKEN=""
MEMBER_TOKEN=""
MOD_TOKEN=""

if [ -n "$ADMIN_DIR" ]; then
    ADMIN_USER=$(extract_username "$ADMIN_DIR")
    ADMIN_CLIENT=$(extract_client_id "$ADMIN_DIR")
    ADMIN_TOKEN=$(generate_token "$ADMIN_USER" "$JWT_SECRET" "$ADMIN_CLIENT")
    echo "✓ Admin token generated"
fi

if [ -n "$MEMBER_DIR" ]; then
    MEMBER_USER=$(extract_username "$MEMBER_DIR")
    MEMBER_CLIENT=$(extract_client_id "$MEMBER_DIR")
    MEMBER_TOKEN=$(generate_token "$MEMBER_USER" "$JWT_SECRET" "$MEMBER_CLIENT")
    echo "✓ Member token generated"
fi

if [ -n "$MOD_DIR" ]; then
    MOD_USER=$(extract_username "$MOD_DIR")
    MOD_CLIENT=$(extract_client_id "$MOD_DIR")
    MOD_TOKEN=$(generate_token "$MOD_USER" "$JWT_SECRET" "$MOD_CLIENT")
    echo "✓ Mod token generated"
fi

echo ""
echo "=== Running GraphQL Validation ==="
echo ""

# Run validation script
export GRAPHQL_ENDPOINT="$ENDPOINT"
export ADMIN_TOKEN="$ADMIN_TOKEN"
export MEMBER_TOKEN="$MEMBER_TOKEN"
export MOD_TOKEN="$MOD_TOKEN"

python3 "$SCRIPT_DIR/validate_graphql_comprehensive.py"

