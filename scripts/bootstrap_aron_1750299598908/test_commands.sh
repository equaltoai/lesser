#!/bin/bash

# Test commands for Lesser API
DOMAIN="lesser.host"
TOKEN="$1"

if [ -z "$TOKEN" ]; then
    echo "Usage: ./test_commands.sh <jwt_token>"
    echo "Generate token using: JWT_SECRET=seNNAR+4jKG6vSoxGBZ9GYuBx7scopVcS1fE6enobEI= node create_token.js"
    exit 1
fi

echo "Testing with token: $TOKEN"

# Test authentication
echo "Testing authentication..."
curl -H "Authorization: Bearer $TOKEN" \
  "https://$DOMAIN/api/v1/accounts/verify_credentials"

# Create a test post
echo -e "\n\nCreating test post..."
curl -X POST "https://$DOMAIN/api/v1/statuses" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "Hello from Lesser!", "visibility": "public"}'
