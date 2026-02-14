#!/bin/bash
# Diagnostic script for Lesser test failures

echo "=== Diagnosing Lesser Test Issues ==="
echo

# Check AWS Lambda account concurrency (optional)
if [ -n "$AWS_PROFILE" ] && command -v aws &> /dev/null; then
    echo "0. Checking AWS Lambda concurrency quota (AWS_PROFILE=$AWS_PROFILE):"
    AWS_REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}"
    LIMIT=$(aws lambda get-account-settings --region "$AWS_REGION" --query 'AccountLimit.ConcurrentExecutions' --output text 2>/dev/null || true)
    if [[ "$LIMIT" =~ ^[0-9]+$ ]]; then
        echo "   ✓ Account concurrency limit: $LIMIT (region: $AWS_REGION)"
        if [ "$LIMIT" -lt 50 ]; then
            echo "   ⚠️  This is unusually low; Lambda throttling can show up as intermittent 5xx responses."
        fi
    else
        echo "   ✗ Unable to read account concurrency limit (check AWS credentials/region permissions)"
    fi
    echo
fi

# Check if bc is installed
echo "1. Checking for bc command (needed for calculations):"
if command -v bc &> /dev/null; then
    echo "   ✓ bc is installed"
else
    echo "   ✗ bc is NOT installed - install with: brew install bc (macOS) or apt-get install bc (Linux)"
fi
echo

# Test instance URL
INSTANCE_URL="${LESSER_URL:-https://lesser.host}"
echo "2. Testing instance URL: $INSTANCE_URL"

# Test basic connectivity
echo "   Testing /api/v1/instance endpoint:"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$INSTANCE_URL/api/v1/instance")
if [ "$HTTP_CODE" = "200" ]; then
    echo "   ✓ Instance is reachable (HTTP $HTTP_CODE)"
else
    echo "   ✗ Instance returned HTTP $HTTP_CODE"
fi
echo

# Test WebFinger
echo "3. Testing WebFinger endpoint:"
WEBFINGER_URL="$INSTANCE_URL/.well-known/webfinger?resource=acct:aron@${INSTANCE_URL#https://}"
echo "   URL: $WEBFINGER_URL"
WEBFINGER_RESPONSE=$(curl -s "$WEBFINGER_URL")
echo "   Response: $WEBFINGER_RESPONSE"
if echo "$WEBFINGER_RESPONSE" | jq -e '.subject' > /dev/null 2>&1; then
    echo "   ✓ WebFinger is working"
else
    echo "   ✗ WebFinger failed or returned invalid JSON"
fi
echo

# Test content negotiation
echo "4. Testing content negotiation (ActivityPub):"
ACTOR_URL="$INSTANCE_URL/users/aron"
echo "   URL: $ACTOR_URL"
ACTOR_RESPONSE=$(curl -s -H 'Accept: application/activity+json' "$ACTOR_URL")
echo "   Response: $ACTOR_RESPONSE"
if echo "$ACTOR_RESPONSE" | jq -e '.type' > /dev/null 2>&1; then
    echo "   ✓ Content negotiation is working"
else
    echo "   ✗ Content negotiation failed or returned invalid JSON"
fi
echo

# Test cost headers (if token is available)
if [ -n "$LESSER_TOKEN" ]; then
    echo "5. Testing cost tracking headers:"
    HEADERS=$(curl -s -H "Authorization: Bearer $LESSER_TOKEN" "$INSTANCE_URL/api/v1/timelines/home" -I)
    echo "   Headers:"
    echo "$HEADERS" | grep -i "x-cost"
    if echo "$HEADERS" | grep -q "X-Cost-Total-Microcents"; then
        echo "   ✓ Cost headers are present"
    else
        echo "   ✗ Cost headers are missing"
    fi
else
    echo "5. Skipping cost header test (no token provided)"
fi
echo

echo "=== Diagnosis Complete ===" 
