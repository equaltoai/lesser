#!/bin/bash

# Test commands for Lesser API
DOMAIN="dev.lesser.host"
TOKEN="$1"

if [ -z "$TOKEN" ]; then
    echo "Usage: ./test_commands.sh <jwt_token>"
    echo "Generate token using: JWT_SECRET=|UQ)}RtvjE[+s:$6QS?|kv[ZkOWAVs5680RpZh*[y,]$({dLT7bqPh;b[uPh>_V^ node create_token.js"
    exit 1
fi

echo "Testing with domain: $DOMAIN"
echo "Testing with token: $TOKEN"
echo ""

# Test authentication
echo "1. Testing authentication..."
curl -v -H "Authorization: Bearer $TOKEN" \
  "https://$DOMAIN/api/v1/accounts/verify_credentials"

echo -e "\n\n2. Creating test post..."
curl -v -X POST "https://$DOMAIN/api/v1/statuses" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "Hello from Lesser!", "visibility": "public"}'

echo -e "\n\n3. Getting home timeline..."
curl -v -H "Authorization: Bearer $TOKEN" \
  "https://$DOMAIN/api/v1/timelines/home"
