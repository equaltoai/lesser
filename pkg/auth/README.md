# Auth Package

This package provides OAuth 2.0 authentication for Lesser, enabling secure access to local user accounts.

## Overview

The auth package implements:
- OAuth 2.0 with PKCE (Proof Key for Code Exchange) for enhanced security
- JWT (JSON Web Token) based access tokens
- Refresh token support for long-lived sessions
- Middleware for protecting endpoints
- Scope-based authorization

Lesser is **passwordless** by design: users authenticate via WebAuthn/passkeys and/or wallet signatures. OAuth tokens
are still presented as `Authorization: Bearer …`, but Lesser does not support password-based login or password grant
flows.

## Components

### OAuthService

The main OAuth service that handles:
- Client validation
- Authorization code generation
- PKCE verification
- Token generation and validation
- JWT signing and verification

### Middleware

Authentication middleware that:
- Extracts and validates JWT tokens from requests
- Provides user context to handlers
- Enforces scope requirements
- Validates user ownership of resources

## Usage

### Basic Setup

```go
import "github.com/equaltoai/lesser/pkg/auth"

// Create OAuth service
oauthSvc := auth.NewOAuthService("your-jwt-secret")

// Register a client
oauthSvc.RegisterClient(&auth.Client{
    ID:          "my-app",
    Secret:      "client-secret",
    Name:        "My Application",
    RedirectURI: "https://myapp.com/callback",
})
```

### Protecting Endpoints

```go
// Create middleware
authMiddleware := auth.NewMiddleware()

// In your Lambda handler
func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Verify authentication
    claims, err := authMiddleware.RequireAuth(ctx, request)
    if err != nil {
        return common.Unauthorized(err), nil
    }
    
    // Check user matches resource owner
    if err := authMiddleware.RequireUser(claims, username); err != nil {
        return common.Forbidden(err), nil
    }
    
    // Check required scope
    if err := authMiddleware.RequireScope(claims, auth.ScopeWrite); err != nil {
        return common.Forbidden(err), nil
    }
    
    // User is authenticated and authorized
    // ... handle request ...
}
```

### OAuth Flow

1. **Authorization Request**
   ```
   GET /oauth/authorize?
     response_type=code&
     client_id=my-app&
     redirect_uri=https://myapp.com/callback&
     code_challenge=CHALLENGE&
     code_challenge_method=S256&
     scope=read write&
     state=RANDOM_STATE
   ```

2. **User Authorization**
   - Users authenticate via the passwordless auth UI (WebAuthn/passkeys and/or wallet challenge + signature).
   - See `docs/architecture/auth/PASSWORDLESS_OAUTH.md` for the end-to-end flow and endpoints.

3. **Authorization Code Response**
   ```
   HTTP/1.1 302 Found
   Location: https://myapp.com/callback?code=AUTH_CODE&state=RANDOM_STATE
   ```

4. **Token Exchange**
   ```
   POST /oauth/token
   Content-Type: application/x-www-form-urlencoded
   
   grant_type=authorization_code&
   code=AUTH_CODE&
   client_id=my-app&
   client_secret=client-secret&
   code_verifier=VERIFIER&
   redirect_uri=https://myapp.com/callback
   ```

5. **Token Response**
   ```json
   {
     "access_token": "eyJ0eXAiOiJKV1QiLCJhbGc...",
     "token_type": "Bearer",
     "expires_in": 3600,
     "refresh_token": "REFRESH_TOKEN",
     "scope": "read write"
   }
   ```

## Security Considerations

### PKCE Required
All authorization requests must include PKCE parameters:
- `code_challenge`: Base64URL encoded SHA256 hash of the code verifier
- `code_challenge_method`: Must be "S256"
- `code_verifier`: Random string used in token exchange

### Token Expiration
- Access tokens expire after 1 hour
- Refresh tokens expire after 30 days
- Authorization codes expire after 10 minutes

### JWT Claims
Access tokens include:
```json
{
  "sub": "username",
  "username": "username",
  "client_id": "my-app",
  "scopes": ["read", "write"],
  "iat": 1234567890,
  "exp": 1234571490
}
```

## Scopes

- `read`: Read access to user data
- `write`: Write access to create/update activities

## Storage

OAuth data is stored in DynamoDB:
- Authorization codes: `PK=AUTHCODE#{code}`, `SK=CODE`
- Refresh tokens: `PK=REFRESHTOKEN#{token}`, `SK=TOKEN`

Both include automatic expiration handling.

## Configuration

Environment variables:
- `JWT_SECRET`: Secret key for signing JWT tokens (required in production)

## Testing

```bash
# Run OAuth package tests
go test ./pkg/auth/...

# Run OAuth storage tests
GO_ENV=test go test ./pkg/storage/dynamodb/... -run OAuth
```

## Future Enhancements

1. **User Authentication**
   - Expand passwordless login UI (passkeys/wallets)
   - Account recovery UX (recovery codes / social recovery)
   - Multi-factor authentication (passwordless-friendly)

2. **Client Management**
   - Dynamic client registration
   - Client credentials from database
   - Multiple redirect URIs per client

3. **Advanced OAuth Features**
   - OpenID Connect support
   - Token introspection endpoint
   - Token revocation endpoint
   - Client credentials grant type

4. **Security Enhancements**
   - Rate limiting
   - Brute force protection
   - Token rotation on refresh
   - Encrypted token storage 
