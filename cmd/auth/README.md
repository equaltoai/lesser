# Auth Lambda Function

This Lambda function provides OAuth 2.0 endpoints for Lesser, enabling third-party applications and users to authenticate.

## Endpoints

### Authorization Endpoint
`GET/POST /oauth/authorize`

Initiates the OAuth 2.0 authorization code flow with PKCE.

**Query Parameters:**
- `response_type` (required): Must be "code"
- `client_id` (required): The client application ID
- `redirect_uri` (required): Where to redirect after authorization
- `code_challenge` (required): PKCE code challenge
- `code_challenge_method` (optional): Must be "S256" if provided
- `scope` (optional): Space-separated list of scopes (defaults to "read write")
- `state` (optional): Opaque value to maintain state

**Response:**
- 302 redirect to `redirect_uri` with authorization code
- Error responses redirect with `error` and `error_description` parameters

**Example:**
```
GET /oauth/authorize?
  response_type=code&
  client_id=lesser-web&
  redirect_uri=https://app.example.com/callback&
  code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&
  code_challenge_method=S256&
  scope=read write&
  state=xyz123
```

### Token Endpoint
`POST /oauth/token`

Exchanges authorization codes for access tokens or refreshes existing tokens.

**Content-Type:** `application/x-www-form-urlencoded` or `application/json`

**Parameters:**

For authorization code grant:
- `grant_type`: "authorization_code"
- `code`: The authorization code received
- `redirect_uri`: Must match the original request
- `client_id`: The client application ID
- `client_secret`: The client secret
- `code_verifier`: PKCE code verifier

For refresh token grant:
- `grant_type`: "refresh_token"
- `refresh_token`: The refresh token
- `client_id`: The client application ID
- `client_secret`: The client secret
- `scope` (optional): Requested scopes (must be subset of original)

**Response:**
```json
{
  "access_token": "eyJ0eXAiOiJKV1QiLCJhbGc...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "dGVzdC1yZWZyZXNo...",
  "scope": "read write"
}
```

**Error Response:**
```json
{
  "error": "invalid_grant",
  "error_description": "Invalid authorization code"
}
```

### Discovery Endpoint
`GET /oauth/.well-known/oauth-authorization-server`

Returns OAuth 2.0 server metadata for automatic configuration.

**Response:**
```json
{
  "issuer": "https://lesser.example.com",
  "authorization_endpoint": "https://lesser.example.com/oauth/authorize",
  "token_endpoint": "https://lesser.example.com/oauth/token",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "code_challenge_methods_supported": ["S256"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "client_secret_basic"],
  "scopes_supported": ["read", "write"]
}
```

## Configuration

### Environment Variables
- `JWT_SECRET`: Secret key for signing JWT tokens (defaults to development key)
- Standard Lambda/DynamoDB configuration

### Default Client
For development, a default client is registered:
- Client ID: `lesser-web`
- Client Secret: `development-client-secret`
- Redirect URI: `{BASE_URL}/auth/callback`

## Security

### PKCE (Proof Key for Code Exchange)
All authorization requests must use PKCE to prevent authorization code interception attacks.

1. Generate a code verifier (43-128 characters)
2. Calculate code challenge: `BASE64URL(SHA256(code_verifier))`
3. Include `code_challenge` and `code_challenge_method=S256` in authorization request
4. Include `code_verifier` in token exchange

### Token Security
- Access tokens are JWT signed with HS256
- Access tokens expire after 1 hour
- Refresh tokens expire after 30 days
- Authorization codes expire after 10 minutes

## Testing

```bash
# Unit tests
go test ./cmd/auth/...

# Manual testing with curl
# 1. Get authorization code (normally done via browser)
curl "http://localhost:3000/oauth/authorize?\
response_type=code&\
client_id=lesser-web&\
redirect_uri=http://localhost:3000/auth/callback&\
code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&\
code_challenge_method=S256"

# 2. Exchange code for token
curl -X POST http://localhost:3000/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&\
code=AUTHORIZATION_CODE&\
client_id=lesser-web&\
client_secret=development-client-secret&\
code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk&\
redirect_uri=http://localhost:3000/auth/callback"

# 3. Refresh token
curl -X POST http://localhost:3000/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token&\
refresh_token=REFRESH_TOKEN&\
client_id=lesser-web&\
client_secret=development-client-secret"
```

## Current Limitations

1. **No User Authentication**: Currently hardcoded to "testuser"
2. **No Consent Screen**: Authorization is automatic
3. **Single Client**: Only one hardcoded client is supported
4. **No Token Revocation**: Tokens cannot be revoked before expiration

## Future Improvements

1. Implement user authentication (login page)
2. Add consent screen for scope approval
3. Dynamic client registration
4. Token revocation endpoint
5. OpenID Connect support
6. Rate limiting and abuse prevention 