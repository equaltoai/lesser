#!/bin/bash
# Script to configure CloudFront signed URL keys after CDK deployment
# Usage: ./scripts/configure-cloudfront-keys.sh <environment>
# Example: ./scripts/configure-cloudfront-keys.sh production

set -e

ENVIRONMENT=${1:-development}
STACK_NAME="LesserStack-${ENVIRONMENT}"

if [ "$ENVIRONMENT" = "live" ]; then
    ENVIRONMENT="production"
    STACK_NAME="LesserStack-production"
fi

echo "=== Configuring CloudFront Keys for ${ENVIRONMENT} ==="
echo

# Step 1: Extract public key from stack outputs
echo "Step 1: Extracting public key from CloudFormation stack..."
PUBLIC_KEY=$(aws cloudformation describe-stacks \
  --stack-name "$STACK_NAME" \
  --query 'Stacks[0].Outputs[?OutputKey==`CloudFrontKeyPairPublicKey`].OutputValue' \
  --output text)

if [ -z "$PUBLIC_KEY" ]; then
    echo "❌ Error: Could not find CloudFrontKeyPairPublicKey output in stack $STACK_NAME"
    echo "   Make sure the stack has been deployed with the CloudFront key pair construct."
    exit 1
fi

echo "✓ Public key extracted (${#PUBLIC_KEY} bytes)"
echo

# Step 2: Upload public key to CloudFront
echo "Step 2: Uploading public key to CloudFront..."
KEY_RESPONSE=$(aws cloudfront create-public-key \
  --public-key-config \
    Name=lesser-${ENVIRONMENT}-key,EncodedKey="$PUBLIC_KEY",CallerReference=$(date +%s) \
  --output json)

KEY_ID=$(echo "$KEY_RESPONSE" | jq -r '.PublicKey.Id')
KEY_ETAG=$(echo "$KEY_RESPONSE" | jq -r '.ETag')

echo "✓ Public key uploaded"
echo "  Key ID: $KEY_ID"
echo "  ETag: $KEY_ETAG"
echo

# Step 3: Create CloudFront key group
echo "Step 3: Creating CloudFront key group..."
KEYGROUP_RESPONSE=$(aws cloudfront create-key-group \
  --key-group-config \
    Name=lesser-${ENVIRONMENT}-keygroup,Items=$KEY_ID \
  --output json)

KEYGROUP_ID=$(echo "$KEYGROUP_RESPONSE" | jq -r '.KeyGroup.Id')

echo "✓ Key group created"
echo "  Key Group ID: $KEYGROUP_ID"
echo

# Step 4: Update CDK config file
echo "Step 4: Updating CDK config file..."
CONFIG_FILE="infra/cdk/config/${ENVIRONMENT}.yaml"

if [ -f "$CONFIG_FILE" ]; then
    # Use sed to update the cloudfrontKeyPairId value
    sed -i.bak "s/cloudfrontKeyPairId: .*/cloudfrontKeyPairId: \"$KEY_ID\"/" "$CONFIG_FILE"
    rm -f "${CONFIG_FILE}.bak"
    echo "✓ Updated $CONFIG_FILE with cloudfrontKeyPairId: $KEY_ID"
else
    echo "⚠️  Config file not found: $CONFIG_FILE"
    echo "   You'll need to manually update the CloudFront key pair ID in your config."
fi
echo

# Step 5: Update Lambda environment variables (temporary until next deploy)
echo "Step 5: Updating Lambda environment variables..."
LAMBDA_FUNCTIONS=$(aws lambda list-functions \
  --query "Functions[?starts_with(FunctionName, 'lesser-${ENVIRONMENT}')].FunctionName" \
  --output text)

UPDATED=0
SKIPPED=0

for func in $LAMBDA_FUNCTIONS; do
    # Get current environment variables
    CURRENT_ENV=$(aws lambda get-function-configuration \
      --function-name "$func" \
      --query 'Environment.Variables' \
      --output json)
    
    # Add or update CLOUDFRONT_KEY_PAIR_ID
    NEW_ENV=$(echo "$CURRENT_ENV" | jq --arg keyid "$KEY_ID" '. + {CLOUDFRONT_KEY_PAIR_ID: $keyid}')
    
    # Update the function
    aws lambda update-function-configuration \
      --function-name "$func" \
      --environment "Variables=$NEW_ENV" \
      --no-cli-pager > /dev/null 2>&1 && \
      echo "  ✓ Updated $func" && UPDATED=$((UPDATED + 1)) || \
      echo "  ⚠️  Skipped $func" && SKIPPED=$((SKIPPED + 1))
done

echo
echo "Updated $UPDATED Lambda functions, skipped $SKIPPED"
echo

# Summary
echo "=== Configuration Complete ==="
echo
echo "CloudFront Key Pair ID: $KEY_ID"
echo "CloudFront Key Group ID: $KEYGROUP_ID"
echo
echo "✓ Public key uploaded to CloudFront"
echo "✓ Key group created"
echo "✓ Lambda environment variables updated (temporary)"
echo "✓ CDK config file updated: $CONFIG_FILE"
echo
echo "⚠️  IMPORTANT: Run 'cd infra/cdk && cdk deploy' to persist the configuration!"
echo

