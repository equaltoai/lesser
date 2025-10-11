#!/bin/bash

# Script to generate a secure JWT secret for Lesser ActivityPub server
# Usage: ./generate-jwt-secret.sh

set -e

echo "Generating secure JWT secret for Lesser..."
echo ""

# Generate a 64-character random string (512 bits of entropy)
JWT_SECRET=$(openssl rand -base64 48 | tr -d '\n')

echo "Generated JWT secret:"
echo "====================="
echo "$JWT_SECRET"
echo ""

echo "To use this secret, set it as an environment variable:"
echo "export JWT_SECRET='$JWT_SECRET'"
echo ""

echo "Or add it to your .env file:"
echo "JWT_SECRET=$JWT_SECRET"
echo ""

echo "Security notes:"
echo "- This secret is 64 characters long (512 bits of entropy)"
echo "- Store it securely and never commit it to version control"
echo "- Use different secrets for different environments (dev, staging, prod)"
echo "- Rotate secrets periodically for enhanced security"