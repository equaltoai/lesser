# Actor Profile Endpoint

This Lambda function handles requests for ActivityPub actor profiles, supporting both human-readable HTML and machine-readable ActivityStreams JSON formats through content negotiation.

## Endpoint

```
GET /users/{username}
```

## Features

- **Content Negotiation**: Returns JSON for ActivityPub clients and HTML for browsers
- **Public Key Serving**: Includes public keys for HTTP signature verification
- **Storage Integration**: Connects to DynamoDB to fetch actor data
- **Error Handling**: Proper error responses with appropriate status codes

## Content Types

### ActivityStreams JSON

When the `Accept` header contains:
- `application/activity+json`
- `application/ld+json`
- `application/json`

Returns an ActivityPub actor object:

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice",
  "type": "Person",
  "preferredUsername": "alice",
  "name": "Alice Smith",
  "summary": "A test user",
  "inbox": "https://example.com/users/alice/inbox",
  "outbox": "https://example.com/users/alice/outbox",
  "followers": "https://example.com/users/alice/followers",
  "following": "https://example.com/users/alice/following",
  "publicKey": {
    "id": "https://example.com/users/alice#main-key",
    "owner": "https://example.com/users/alice",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\n..."
  }
}
```

### HTML

When the `Accept` header contains `text/html` or is missing, returns a human-readable profile page with:
- Profile information
- Avatar (if available)
- Bio/summary
- Links to followers/following
- ActivityPub discovery metadata

## Environment Variables

Required:
- `DOMAIN` - The domain name of the instance
- `TABLE_NAME` - DynamoDB table name
- `JWT_SECRET` - Secret for JWT token generation

Optional:
- `LOG_LEVEL` - Logging level (debug, info, warn, error)
- `AWS_REGION` - AWS region for DynamoDB

## Testing

Run unit tests:

```bash
JWT_SECRET=test DOMAIN=example.com TABLE_NAME=test go test -v
```

The tests use a mock storage implementation to avoid requiring actual DynamoDB access.

## Manual Testing

Test JSON response:
```bash
curl -H "Accept: application/activity+json" https://example.com/users/alice
```

Test HTML response:
```bash
curl https://example.com/users/alice
```

## Integration with WebFinger

The WebFinger endpoint (`/.well-known/webfinger`) returns links to this actor endpoint, enabling discovery through the standard WebFinger flow:

1. Remote server queries: `/.well-known/webfinger?resource=acct:alice@example.com`
2. WebFinger returns actor URL: `https://example.com/users/alice`
3. Remote server fetches actor profile with `Accept: application/activity+json`
4. Actor profile includes public key for HTTP signature verification

## Error Responses

- **400 Bad Request**: Missing username parameter
- **404 Not Found**: Actor doesn't exist
- **500 Internal Server Error**: Database or other internal errors

All error responses follow the common error format:
```json
{
  "error": "Not Found",
  "message": "actor not found: alice",
  "code": "ACTOR_NOT_FOUND"
}
```

## Deployment

This function is designed to be deployed as an AWS Lambda function behind API Gateway. The AWS CDK infrastructure (`infra/cdk`) will:

1. Create the Lambda function
2. Configure API Gateway routes
3. Set up appropriate IAM roles
4. Configure environment variables

## Future Enhancements

- [ ] Support for actor attachments (profile fields)
- [ ] Custom CSS themes
- [ ] Profile metadata (pronouns, website, etc.)
- [ ] Integration with media storage for avatars
- [ ] Caching headers optimization 
