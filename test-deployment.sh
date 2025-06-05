#!/bin/bash

# Test script for Lesser ActivityPub server
DOMAIN="lesser.host"
TEST_USER="testuser"

echo "🧪 Testing Lesser ActivityPub Server at $DOMAIN"
echo "================================================"

# Test WebFinger
echo -e "\n📍 Testing WebFinger..."
curl -s -H "Accept: application/json" \
  "https://$DOMAIN/.well-known/webfinger?resource=acct:$TEST_USER@$DOMAIN" \
  | jq '.' || echo "❌ WebFinger failed"

# Test Actor
echo -e "\n👤 Testing Actor endpoint..."
curl -s -H "Accept: application/activity+json" \
  "https://$DOMAIN/users/$TEST_USER" \
  | jq '.' || echo "❌ Actor endpoint failed"

# Test Inbox (expect 202 Accepted or 404 if user doesn't exist)
echo -e "\n📥 Testing Inbox..."
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  -H "Content-Type: application/activity+json" \
  -H "Accept: application/activity+json" \
  -d '{
    "@context": "https://www.w3.org/ns/activitystreams",
    "id": "https://example.com/test/'$(date +%s)'",
    "type": "Follow",
    "actor": "https://example.com/users/test",
    "object": "https://'$DOMAIN'/users/'$TEST_USER'"
  }' \
  "https://$DOMAIN/users/$TEST_USER/inbox")

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" == "202" ]; then
  echo "✅ Inbox accepted activity (202)"
elif [ "$HTTP_CODE" == "404" ]; then
  echo "⚠️  Inbox returned 404 (user might not exist yet)"
else
  echo "❌ Inbox failed with status $HTTP_CODE"
  echo "$BODY" | jq '.' 2>/dev/null || echo "$BODY"
fi

# Test Outbox
echo -e "\n📤 Testing Outbox..."
curl -s -H "Accept: application/activity+json" \
  "https://$DOMAIN/users/$TEST_USER/outbox" \
  | jq '.' || echo "❌ Outbox failed"

# Test OAuth metadata
echo -e "\n🔐 Testing OAuth metadata..."
curl -s -H "Accept: application/json" \
  "https://$DOMAIN/.well-known/oauth-authorization-server" \
  | jq '.' || echo "❌ OAuth metadata failed"

echo -e "\n✅ Basic connectivity test complete!" 