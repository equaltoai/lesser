#!/bin/bash
set -e

TABLE_NAME="lesser-lab"

echo "Deploying to table: $TABLE_NAME"

# Deploy items
echo "Creating actor..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://actor.json

echo "Creating user..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://user.json

echo "Creating OAuth client..."
aws dynamodb put-item --table-name "$TABLE_NAME" --item file://oauth_client.json

echo "✅ Deployment complete!"
