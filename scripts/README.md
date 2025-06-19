# Bootstrap User Script

This script creates a user in an empty Lesser database with everything needed to start using the API.

## What it creates:

1. **Actor (User)** - The ActivityPub actor with:
   - RSA key pair for federation
   - Profile information
   - Inbox/Outbox endpoints

2. **OAuth Client** - For authentication with:
   - Client ID and Secret
   - Default scopes (read, write, follow, push)

3. **Access Token** - Ready-to-use Bearer token (valid for 2 hours)

## Usage:

```bash
# Build the script
go build -o bootstrap_user scripts/bootstrap_user.go

# Run with defaults (creates user 'testuser')
./bootstrap_user

# Run with custom username
./bootstrap_user -username aron -display-name "Aron Price" -bio "Testing Lesser"

# Create an admin user
./bootstrap_user -username admin -display-name "Admin" -bio "Administrator account" -admin

# Specify table name and domain
./bootstrap_user -username aron -table my-dynamodb-table -domain lesser.host
```

## Environment Variables:

The script uses the same environment variables as Lesser:
- `LESSER_DOMAIN` - Your domain (e.g., lesser.host)
- `LESSER_DYNAMODB_TABLE` - DynamoDB table name
- `LESSER_JWT_SECRET` - JWT signing secret
- `AWS_REGION` - AWS region
- AWS credentials (via standard AWS SDK methods)

## After Running:

The script will output:
1. The created username and actor ID
2. OAuth client credentials
3. A Bearer token for immediate use
4. Example curl commands to test the API

Example output:
```
✓ Actor created successfully!
✓ OAuth client created successfully!

Client Credentials:
  Client ID:     AbCdEfGhIjKlMnOpQrStUvWx
  Client Secret: 1234567890abcdefghijklmnopqrstuvwxyzABCDEFG

✓ Access token created successfully!

Access Token: Bearer aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890AbCdE

You can now use the API with:
curl -H "Authorization: Bearer aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890AbCdE" https://lesser.host/api/v1/accounts/verify_credentials
```