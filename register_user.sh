#!/bin/bash

# Register a new user on Lesser
# Usage: ./register_user.sh [username] [email] [password]

BASE_URL="${LESSER_URL:-https://lesser.host}"
USERNAME="${1:-testuser}"
EMAIL="${2:-testuser@example.com}"
PASSWORD="${3:-SecurePassword123!}"

echo "Registering user on $BASE_URL..."
echo "Username: $USERNAME"
echo "Email: $EMAIL"

# First, create an app to get client credentials
echo -e "\n1. Creating OAuth app..."
APP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/apps" \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Registration Script",
    "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
    "scopes": "read write",
    "website": "https://github.com"
  }')

CLIENT_ID=$(echo "$APP_RESPONSE" | grep -o '"client_id":"[^"]*' | cut -d'"' -f4)
CLIENT_SECRET=$(echo "$APP_RESPONSE" | grep -o '"client_secret":"[^"]*' | cut -d'"' -f4)

if [ -z "$CLIENT_ID" ]; then
  echo "Failed to create app. Response:"
  echo "$APP_RESPONSE"
  exit 1
fi

echo "App created successfully!"
echo "Client ID: ${CLIENT_ID:0:20}..."

# Now register the user
echo -e "\n2. Registering user account..."
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d "{
    \"username\": \"$USERNAME\",
    \"email\": \"$EMAIL\",
    \"password\": \"$PASSWORD\",
    \"agreement\": true,
    \"locale\": \"en\"
  }")

# Check if registration was successful
if echo "$REGISTER_RESPONSE" | grep -q '"id"'; then
  echo -e "\n✅ User registered successfully!"
  USER_ID=$(echo "$REGISTER_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
  echo "User ID: $USER_ID"
  
  # Save credentials for later use
  echo -e "\nSaving credentials to .env.lesser..."
  cat > .env.lesser << EOF
# Lesser OAuth Credentials
LESSER_URL=$BASE_URL
LESSER_CLIENT_ID=$CLIENT_ID
LESSER_CLIENT_SECRET=$CLIENT_SECRET
LESSER_USERNAME=$USERNAME
LESSER_PASSWORD=$PASSWORD
EOF
  
  echo -e "\nNext steps:"
  echo "1. Visit: $BASE_URL/oauth/authorize?client_id=$CLIENT_ID&response_type=code&redirect_uri=urn:ietf:wg:oauth:2.0:oob&scope=read+write"
  echo "2. Log in with username: $USERNAME and password: $PASSWORD"
  echo "3. Authorize the app and copy the code"
  echo "4. Exchange the code for an access token"
  
else
  echo -e "\n❌ Registration failed. Response:"
  echo "$REGISTER_RESPONSE"
  
  # Common error cases
  if echo "$REGISTER_RESPONSE" | grep -q "already taken"; then
    echo -e "\nUsername or email already exists."
  elif echo "$REGISTER_RESPONSE" | grep -q "registrations are not open"; then
    echo -e "\nRegistrations are disabled on this instance."
  fi
fi 