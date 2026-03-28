# OAuth for Mastodon Servers and Client Applications - Complete Overview

## Table of Contents
1. [Introduction](#introduction)
2. [OAuth 2.0 Fundamentals](#oauth-20-fundamentals)
3. [Mastodon-Specific OAuth Requirements](#mastodon-specific-oauth-requirements)
4. [Client Application Registration](#client-application-registration)
5. [Authorization Code Flow](#authorization-code-flow)
6. [PKCE (Proof Key for Code Exchange)](#pkce-proof-key-for-code-exchange)
7. [Token Exchange and Management](#token-exchange-and-management)
8. [Scopes and Permissions](#scopes-and-permissions)
9. [Security Features](#security-features)
10. [Error Handling](#error-handling)
11. [Client Implementation Guide](#client-implementation-guide)

---

## Introduction

OAuth 2.0 is the standard authorization framework used by Mastodon servers to allow third-party client applications to access user accounts securely. Unlike traditional password-based authentication, OAuth enables users to grant limited access to their accounts without sharing credentials directly with third-party applications.

This document provides a comprehensive overview of how OAuth works specifically in the context of Mastodon servers and client applications, based on the Mastodon API specification and common OAuth 2.0 practices.

---

## OAuth 2.0 Fundamentals

### Key Concepts

**OAuth 2.0 Roles:**
- **Resource Owner**: The user who owns the account on the Mastodon server
- **Client Application**: The third-party application requesting access (e.g., a mobile app, web client, or bot)
- **Authorization Server**: The Mastodon server that handles OAuth authorization
- **Resource Server**: The Mastodon server's API endpoints that provide protected resources

**OAuth 2.0 Flow Overview:**
1. Client registers with the server
2. Client redirects user to authorization endpoint
3. User authenticates and grants permission
4. Server redirects back with authorization code
5. Client exchanges code for access token
6. Client uses access token to access protected resources

---

## Mastodon-Specific OAuth Requirements

### Endpoint Structure

Mastodon servers commonly expose OAuth endpoints at the following paths:

- **App Registration**: `POST /oauth/register` (canonical RFC 7591) and `POST /api/v1/apps` (compatibility)
- **Authorization**: `GET /oauth/authorize`
- **Token Exchange**: `POST /oauth/token`
- **Token Verification**: `GET /api/v1/accounts/verify_credentials`

For actor-scoped remote MCP access, the canonical quickstart is documented in
[docs/mcp-remote-access.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/mcp-remote-access.md).
Lesser keeps the registration compatibility endpoint, but it does not currently publish a public
`GET /api/v1/apps/verify_credentials` route.

### Mastodon-Specific Rules

1. **Exact Redirect URI Matching**: Mastodon requires **exact** matching of redirect URIs - no prefix matching, no wildcards, no exceptions (except for the out-of-band URI).

2. **PKCE Support**: Mastodon 4.3.0+ requires PKCE (Proof Key for Code Exchange) for enhanced security, especially for public clients.

3. **Scope Validation**: Scopes must be validated against what the client registered during app registration.

4. **Out-of-Band (OOB) Support**: Mastodon supports the special redirect URI `urn:ietf:wg:oauth:2.0:oob` for applications that cannot handle redirects (e.g., desktop apps).

5. **Response Format**: Mastodon follows OAuth 2.0 RFC 6749 but adds Mastodon-specific fields like `created_at` in token responses.

---

## Client Application Registration

### Registration Request

Before a client can initiate OAuth flows, it must register with the Mastodon server:

**Endpoint**: `POST /api/v1/apps`

**Request Parameters:**
- `client_name` (required): Human-readable name of the application
- `redirect_uris` (required): Space or newline-separated list of redirect URIs
- `scopes` (optional): Space-separated list of requested scopes (defaults to "read")
- `website` (optional): URL of the application's website

**Request Format:**
The server accepts multiple content types:
- `application/json`
- `application/x-www-form-urlencoded`
- `multipart/form-data`

**Example Request:**
```
POST /api/v1/apps
Content-Type: application/json

{
  "client_name": "My Fediverse Client",
  "redirect_uris": "https://myapp.com/callback http://localhost:8080/callback",
  "scopes": "read write follow push",
  "website": "https://myapp.com"
}
```

### Registration Response

**Response Fields:**
- `id`: Unique identifier for the client (same as `client_id`)
- `name`: The client name
- `website`: The client website URL
- `redirect_uri`: The first redirect URI (for backward compatibility)
- `client_id`: Unique client identifier
- `client_secret`: Secret key for client authentication (keep secure!)
- `vapid_key`: VAPID public key for push notifications (if supported)

**Example Response:**
```json
{
  "id": "abc123xyz",
  "name": "My Fediverse Client",
  "website": "https://myapp.com",
  "redirect_uri": "https://myapp.com/callback",
  "client_id": "abc123xyz",
  "client_secret": "secret_key_here",
  "vapid_key": "VAPID_PUBLIC_KEY"
}
```

### Registration Security Notes

- **Client Secret Storage**: The `client_secret` must be stored securely and never exposed in client-side code (web apps, mobile apps with extractable code).
- **Public Clients**: For public clients (mobile apps, single-page apps), PKCE must be used instead of client secrets.
- **Redirect URI Validation**: Only registered redirect URIs can be used - exact matching is enforced.
- **Lesser implementation**: Lesser stores OAuth client secrets **hashed at rest** (bcrypt), returns them only at registration or explicit owned-client rotation time, and supports owned confidential-client rotation at `POST /api/v1/apps/{id}/rotate_secret`. Legacy plaintext-at-rest secrets auto-migrate (best-effort) on first successful client authentication.

---

## Authorization Code Flow

### Step 1: Build Authorization URL

The client constructs a URL pointing to the server's authorization endpoint with required parameters:

**Endpoint**: `GET /oauth/authorize`

**Required Parameters:**
- `response_type`: Must be `"code"` (authorization code flow)
- `client_id`: The client ID from registration
- `redirect_uri`: One of the registered redirect URIs (must match exactly)

**Optional Parameters:**
- `scope`: Space-separated list of scopes (defaults to "read")
- `state`: Random string for CSRF protection (recommended)
- `code_challenge`: PKCE code challenge (required for Mastodon 4.3.0+)
- `code_challenge_method`: Must be `"S256"` (SHA-256) for PKCE

**Example Authorization URL:**
```
https://mastodon.example/oauth/authorize?
  response_type=code&
  client_id=abc123xyz&
  redirect_uri=https://myapp.com/callback&
  scope=read+write+follow&
  state=random_state_string_12345&
  code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&
  code_challenge_method=S256
```

### Step 2: User Redirection

The client redirects the user's browser to the authorization URL. The server then:

1. **Checks Authentication**: If the user is not logged in, redirects to login page
2. **Checks Consent**: If the user hasn't consented to this app with these scopes, shows consent screen
3. **Validates Parameters**: Ensures `client_id`, `redirect_uri`, and scopes are valid

### Step 3: User Authentication

If the user is not authenticated, the server presents a login interface.

In Lesser, authentication is **passwordless** (WebAuthn/passkeys and cryptocurrency wallets). Password-based login is not supported.

After successful authentication, the user is redirected back to the authorization endpoint.

### Step 4: Consent Screen

If this is the first time authorizing this app, or if new scopes are requested, the user sees a consent screen showing:

- **App Name**: The registered client name
- **App Website**: Link to the application's website
- **Requested Scopes**: List of permissions the app is requesting
- **Actions**: "Authorize" or "Deny" buttons

**Consent Persistence**: If the user previously authorized this app with the same scopes, the consent step may be skipped (depending on server implementation).

### Step 5: Authorization Code Generation

If the user approves:
1. Server generates a short-lived authorization code
2. Server stores the code with associated metadata:
   - Client ID
   - User identity
   - Requested scopes
   - PKCE code challenge (if provided)
   - Expiration time (typically 5-10 minutes)

### Step 6: Redirect with Code

The server redirects the user back to the client's `redirect_uri`:

**Success Redirect:**
```
https://myapp.com/callback?
  code=AUTHORIZATION_CODE_HERE&
  state=random_state_string_12345
```

**Error Redirect (if user denies):**
```
https://myapp.com/callback?
  error=access_denied&
  error_description=The+user+denied+the+request&
  state=random_state_string_12345
```

**Out-of-Band (OOB) Flow:**
If `redirect_uri` is `urn:ietf:wg:oauth:2.0:oob`, the server displays the authorization code on a webpage instead of redirecting. The user must manually copy this code and paste it into the client application.

---

## PKCE (Proof Key for Code Exchange)

PKCE is a security extension for OAuth 2.0 that prevents authorization code interception attacks. Mastodon 4.3.0+ requires PKCE for public clients.

### How PKCE Works

1. **Code Verifier Generation** (Client):
   - Generate a random 32-byte value
   - Base64URL encode it (remove padding)
   - Store securely for later use

2. **Code Challenge Generation** (Client):
   - Compute SHA-256 hash of the code verifier
   - Base64URL encode the hash (remove padding)
   - Send as `code_challenge` in authorization request

3. **Authorization Request** (Client → Server):
   - Include `code_challenge` and `code_challenge_method=S256` in authorization URL

4. **Challenge Storage** (Server):
   - Store `code_challenge` with the authorization code

5. **Token Exchange** (Client → Server):
   - Include original `code_verifier` in token exchange request

6. **Verification** (Server):
   - Compute SHA-256 hash of provided `code_verifier`
   - Base64URL encode the hash
   - Compare with stored `code_challenge`
   - If match, proceed with token issuance

### PKCE Requirements

- **Method**: Only `S256` (SHA-256) is supported by Mastodon
- **Encoding**: Base64URL encoding (RFC 4648 Section 5) without padding
- **Verifier Length**: Typically 32-128 bytes (before encoding)
- **Storage**: Client must store verifier securely between authorization and token exchange

### Why PKCE Matters

- **Public Clients**: Mobile apps and SPAs cannot securely store client secrets - PKCE provides equivalent security
- **Code Interception Prevention**: Even if an attacker intercepts the authorization code, they cannot exchange it without the verifier
- **Man-in-the-Middle Protection**: Protects against MITM attacks on the authorization code

---

## Token Exchange and Management

### Token Exchange Request

After receiving the authorization code, the client exchanges it for access and refresh tokens:

**Endpoint**: `POST /oauth/token`

**Request Headers:**
```
Content-Type: application/x-www-form-urlencoded
```

**Request Parameters:**
- `grant_type`: Must be `"authorization_code"`
- `code`: The authorization code from the redirect
- `client_id`: The client ID
- `client_secret`: The client secret (if using confidential client)
- `redirect_uri`: Must match the redirect URI used in authorization request (exact match)
- `code_verifier`: The PKCE code verifier (if PKCE was used)

**Example Request:**
```
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&
code=AUTHORIZATION_CODE&
client_id=abc123xyz&
client_secret=secret_key_here&
redirect_uri=https://myapp.com/callback&
code_verifier=CODE_VERIFIER_HERE
```

### Token Exchange Response

**Success Response:**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "scope": "read write follow push",
  "created_at": 1730352000,
  "refresh_token": "refresh_token_here"
}
```

**Response Fields:**
- `access_token`: JWT token for API authentication
- `token_type`: Always `"Bearer"` for Mastodon
- `scope`: Space-separated list of granted scopes
- `created_at`: Unix timestamp of token creation
- `refresh_token`: Token for refreshing access token (optional)
- `expires_in`: Access token expiration in seconds (optional, may be omitted)

### Token Validation

The server validates:
1. **Authorization Code**: Exists, not expired, not previously used
2. **Client Credentials**: `client_id` and `client_secret` match registered client
3. **Redirect URI**: Matches exactly with authorization request
4. **PKCE**: Code verifier matches stored challenge (if PKCE was used)
5. **Code Ownership**: Authorization code belongs to the requesting client

### Access Token Structure

Mastodon access tokens are typically JWT (JSON Web Token) format with the following claims:

```json
{
  "sub": "username",
  "username": "username",
  "client_id": "abc123xyz",
  "scopes": ["read", "write", "follow", "push"],
  "iat": 1730352000,
  "exp": 1730355600,
  "jti": "unique_token_id"
}
```

**Token Claims:**
- `sub`: Subject (username)
- `username`: User identifier
- `client_id`: Client that issued the token
- `scopes`: Array of granted scopes
- `iat`: Issued at timestamp
- `exp`: Expiration timestamp
- `jti`: Unique token identifier (for revocation)

### Token Expiration

- **Access Tokens**: Typically expire after 1 hour (3600 seconds) or shorter in production
- **Refresh Tokens**: Typically expire after 7-30 days
- **Authorization Codes**: Expire after 5-10 minutes

### Using Access Tokens

Once the client has an access token, it includes it in API requests:

**Request Header:**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Example API Request:**
```
GET /api/v1/accounts/verify_credentials
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Refresh Token Flow

When access tokens expire, clients can request new tokens using refresh tokens:

**Endpoint**: `POST /oauth/token`

**Request Parameters:**
- `grant_type`: Must be `"refresh_token"`
- `refresh_token`: The refresh token from initial token exchange
- `client_id`: The client ID
- `client_secret`: The client secret (if using confidential client)
- `scope`: Optional - can request same or subset of original scopes

**Example Request:**
```
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&
refresh_token=REFRESH_TOKEN_HERE&
client_id=abc123xyz&
client_secret=secret_key_here
```

**Response:**
Same format as initial token exchange, with new `access_token` and potentially new `refresh_token`.

**Refresh Token Rotation**: Some servers implement refresh token rotation, where a new refresh token is issued with each refresh, and the old one is invalidated for security.

---

## Scopes and Permissions

Scopes define what permissions the client application has when accessing user resources.

### Standard Mastodon Scopes

- **`read`**: Read access to user data (timeline, profile, statuses, etc.)
- **`write`**: Write access to create/update/delete content (posts, favorites, etc.)
- **`follow`**: Permission to follow/unfollow accounts
- **`push`**: Permission to manage push subscriptions for notifications
- **`admin`**: Administrative access (server admin only)

### Scope Format

- Scopes are space-separated in requests: `"read write follow push"`
- Scopes are returned as space-separated in token responses
- Scopes are case-sensitive

### Scope Validation

1. **During Registration**: Client specifies requested scopes (stored with client registration)
2. **During Authorization**: User sees requested scopes on consent screen
3. **During Token Exchange**: Server validates requested scopes match registered scopes
4. **During API Access**: Server checks token scopes before allowing operations

### Scope Best Practices

- **Minimal Scope**: Request only the scopes your application needs
- **User Understanding**: Scopes should be understandable to users on consent screen
- **Scope Changes**: If requesting new scopes, user will need to re-authorize
- **Scope Inheritance**: Token scopes must be subset of client's registered scopes

---

## Security Features

### State Parameter (CSRF Protection)

The `state` parameter is a random string generated by the client and included in the authorization request. It must be:

1. **Generated**: Random, unpredictable value (e.g., UUID, cryptographically secure random)
2. **Stored**: Saved by client before redirect (e.g., session storage, cookie)
3. **Validated**: Compared on callback - must match exactly
4. **Opaque**: No sensitive information should be encoded in the state

**Purpose**: Prevents Cross-Site Request Forgery (CSRF) attacks by ensuring the callback request originated from the same client that initiated the flow.

### Redirect URI Validation

**Exact Matching**: Mastodon enforces exact matching of redirect URIs:
- No prefix matching
- No wildcard domains
- Protocol must match (`http` vs `https`)
- Port must match
- Path must match exactly
- Query parameters must match (if any)

**Special Cases**:
- `localhost` is allowed for development
- `urn:ietf:wg:oauth:2.0:oob` for out-of-band flows

### Authorization Code Security

- **Single Use**: Authorization codes can only be used once
- **Short Expiration**: Codes expire in 5-10 minutes
- **Client Binding**: Codes are bound to specific client
- **PKCE Binding**: Codes are bound to PKCE challenge (if used)

### Token Security

- **JWT Signing**: Access tokens are cryptographically signed
- **Expiration**: Tokens have limited lifetimes
- **Scope Limitation**: Tokens only grant requested scopes
- **Revocation**: Tokens can be revoked (server-dependent)
- **HTTPS Required**: Tokens should only be transmitted over HTTPS

### Client Secret Security

- **Confidential Clients**: Server-side applications should store client secrets securely
- **Public Clients**: Mobile apps and SPAs cannot securely store secrets - use PKCE instead
- **Never Expose**: Client secrets should never appear in client-side code, URLs, or logs
- **Lesser implementation**: secrets are hashed at rest and are only returned once when created or rotated; owned confidential clients can rotate in place with `POST /api/v1/apps/{id}/rotate_secret`, optionally using a grace window for zero-downtime cutover.

---

## Error Handling

### Authorization Errors

Errors during authorization are returned via redirect to `redirect_uri`:

**Error Parameters:**
- `error`: Error code (see below)
- `error_description`: Human-readable error description
- `error_uri`: Optional URI with more information
- `state`: The state parameter from original request (if provided)

**Common Error Codes:**
- `access_denied`: User denied authorization
- `invalid_request`: Malformed request (missing parameters, invalid values)
- `invalid_client`: Invalid client ID
- `invalid_scope`: Invalid or unauthorized scope
- `server_error`: Server encountered an error
- `temporarily_unavailable`: Server temporarily unavailable

**Example Error Redirect:**
```
https://myapp.com/callback?
  error=access_denied&
  error_description=The+user+denied+the+request&
  state=random_state_string_12345
```

### Token Exchange Errors

Errors during token exchange are returned as JSON:

**Error Response Format:**
```json
{
  "error": "invalid_grant",
  "error_description": "Authorization code expired or invalid"
}
```

**Common Error Codes:**
- `invalid_request`: Malformed request
- `invalid_client`: Invalid client credentials
- `invalid_grant`: Invalid authorization code, expired, or already used
- `invalid_scope`: Requested scope is invalid or not authorized
- `unauthorized_client`: Client not authorized for this grant type
- `unsupported_grant_type`: Grant type not supported

### API Request Errors

When using access tokens in API requests:

**401 Unauthorized**: Invalid or expired token
```json
{
  "error": "invalid_token",
  "error_description": "The access token is invalid or expired"
}
```

**403 Forbidden**: Valid token but insufficient scope
```json
{
  "error": "insufficient_scope",
  "error_description": "The access token does not have required scope"
}
```

### Error Handling Best Practices

1. **Validate State**: Always validate `state` parameter on callback
2. **Handle Errors Gracefully**: Show user-friendly error messages
3. **Retry Logic**: Implement retry for transient errors
4. **Token Refresh**: Automatically refresh expired tokens when possible
5. **Logging**: Log errors for debugging (but not sensitive data)

---

## Client Implementation Guide

### Step-by-Step Implementation

#### 1. Application Registration

```javascript
// Register your application (one-time, or cache client_id/client_secret)
const registrationResponse = await fetch('https://mastodon.example/api/v1/apps', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    client_name: 'My Fediverse Client',
    redirect_uris: 'https://myapp.com/callback',
    scopes: 'read write follow push',
    website: 'https://myapp.com'
  })
});

const { client_id, client_secret } = await registrationResponse.json();
```

#### 2. Generate PKCE Parameters

```javascript
// Generate code verifier (store securely)
function generateCodeVerifier() {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return base64URLEncode(array);
}

// Generate code challenge
async function generateCodeChallenge(verifier) {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const hash = await crypto.subtle.digest('SHA-256', data);
  return base64URLEncode(new Uint8Array(hash));
}

function base64URLEncode(buffer) {
  return btoa(String.fromCharCode(...buffer))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

const codeVerifier = generateCodeVerifier();
const codeChallenge = await generateCodeChallenge(codeVerifier);
```

#### 3. Initiate Authorization

```javascript
// Generate state parameter
const state = generateRandomString(32);

// Store state and verifier securely (sessionStorage, secure cookie, etc.)
sessionStorage.setItem('oauth_state', state);
sessionStorage.setItem('oauth_verifier', codeVerifier);

// Build authorization URL
const authUrl = new URL('https://mastodon.example/oauth/authorize');
authUrl.searchParams.set('response_type', 'code');
authUrl.searchParams.set('client_id', client_id);
authUrl.searchParams.set('redirect_uri', 'https://myapp.com/callback');
authUrl.searchParams.set('scope', 'read write follow push');
authUrl.searchParams.set('state', state);
authUrl.searchParams.set('code_challenge', codeChallenge);
authUrl.searchParams.set('code_challenge_method', 'S256');

// Redirect user
window.location.href = authUrl.toString();
```

#### 4. Handle Callback

```javascript
// On callback page (https://myapp.com/callback)
const params = new URLSearchParams(window.location.search);
const code = params.get('code');
const state = params.get('state');
const error = params.get('error');

// Validate state
const savedState = sessionStorage.getItem('oauth_state');
if (state !== savedState) {
  throw new Error('State mismatch - possible CSRF attack');
}

if (error) {
  // Handle error
  console.error('Authorization error:', error);
  return;
}

// Exchange code for tokens
const codeVerifier = sessionStorage.getItem('oauth_verifier');
// ... continue to token exchange
```

#### 5. Exchange Code for Tokens

```javascript
const tokenResponse = await fetch('https://mastodon.example/oauth/token', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: new URLSearchParams({
    grant_type: 'authorization_code',
    code: code,
    client_id: client_id,
    client_secret: client_secret, // Only for confidential clients
    redirect_uri: 'https://myapp.com/callback',
    code_verifier: codeVerifier
  })
});

const { access_token, refresh_token, expires_in } = await tokenResponse.json();

// Store tokens securely
localStorage.setItem('access_token', access_token);
localStorage.setItem('refresh_token', refresh_token);
sessionStorage.removeItem('oauth_state');
sessionStorage.removeItem('oauth_verifier');
```

#### 6. Make Authenticated Requests

```javascript
const response = await fetch('https://mastodon.example/api/v1/accounts/verify_credentials', {
  headers: {
    'Authorization': `Bearer ${access_token}`
  }
});

const user = await response.json();
```

#### 7. Refresh Expired Tokens

```javascript
async function refreshAccessToken() {
  const refreshToken = localStorage.getItem('refresh_token');
  
  const response = await fetch('https://mastodon.example/oauth/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'refresh_token',
      refresh_token: refreshToken,
      client_id: client_id,
      client_secret: client_secret
    })
  });
  
  const { access_token, refresh_token } = await response.json();
  
  // Update stored tokens
  localStorage.setItem('access_token', access_token);
  localStorage.setItem('refresh_token', refresh_token);
  
  return access_token;
}
```

### Client Types

#### Confidential Clients (Server-Side)

- Can securely store `client_secret`
- Typically web applications with backend servers
- Use client secret authentication in token exchange

#### Public Clients (Client-Side)

- Cannot securely store `client_secret`
- Includes mobile apps, SPAs, desktop apps
- Must use PKCE (no client secret in token exchange)
- Client secret may be omitted or empty

### Platform-Specific Considerations

#### Web Applications

- Use HTTPS for all redirect URIs
- Store tokens securely (httpOnly cookies preferred)
- Implement proper CORS handling
- Use secure session storage for temporary OAuth data

#### Mobile Applications

- Use deep links or custom URL schemes for redirects
- Use secure storage (Keychain, Keystore) for tokens
- Implement certificate pinning for API calls
- Use PKCE (always required for mobile)

#### Desktop Applications

- Consider using out-of-band flow (`urn:ietf:wg:oauth:2.0:oob`)
- Use localhost redirect URIs with random ports
- Store tokens securely in OS credential store
- Use PKCE for security

---

## Summary

OAuth 2.0 for Mastodon provides a secure, standardized way for client applications to access user accounts without requiring users to share passwords. Key points:

1. **Registration**: Clients register once to get `client_id` and `client_secret`
2. **Authorization**: Users authorize apps via browser redirect flow
3. **PKCE**: Required for public clients, enhances security for all clients
4. **Tokens**: Short-lived access tokens for API calls, refresh tokens for renewal
5. **Security**: Multiple layers (state parameter, exact redirect matching, PKCE, token expiration)
6. **Scopes**: Granular permission system for user control

Understanding these components enables developers to build secure, compliant Mastodon client applications that respect user privacy and security.
