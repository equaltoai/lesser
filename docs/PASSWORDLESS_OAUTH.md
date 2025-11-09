# Passwordless OAuth System

Lesser implements a comprehensive, passwordless OAuth 2.0 authorization system using WebAuthn (passkeys) and cryptocurrency wallets. **Passwords are deprecated and not supported.**

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│  OAuth Authorization Flow (Passwordless)                         │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  1. Client Application requests authorization                    │
│     GET https://dev.lesser.host/oauth/authorize                  │
│       ?client_id=xxx                                             │
│       &redirect_uri=http://localhost:4321/callback               │
│       &response_type=code                                        │
│       &scope=read+write                                          │
│                                                                   │
│  2. Lesser checks authentication status                          │
│     ├─ Authenticated → Go to step 4                             │
│     └─ Not authenticated → Redirect to auth UI                   │
│                                                                   │
│  3. Passwordless authentication at https://auth.dev.lesser.host  │
│     ┌───────────────────────────────────────┐                   │
│     │  ✨ Sign in with Passkey               │ ← WebAuthn        │
│     │     • Face ID / Touch ID                │                  │
│     │     • Windows Hello                     │                  │
│     │     • Hardware security keys            │                  │
│     ├───────────────────────────────────────┤                   │
│     │  🔐 Sign in with Crypto Wallet         │ ← Ethereum/EVM   │
│     │     • MetaMask                          │                  │
│     │     • WalletConnect                     │                  │
│     │     • Coinbase Wallet                   │                  │
│     │     • Rainbow, Trust, etc.              │                  │
│     └───────────────────────────────────────┘                   │
│                                                                   │
│  4. After authentication → Show consent screen                   │
│     https://auth.dev.lesser.host/consent                         │
│     ┌───────────────────────────────────────┐                   │
│     │  App: "My Fediverse Client"            │                  │
│     │  Requesting:                            │                  │
│     │    ✓ Read your profile and timeline    │                  │
│     │    ✓ Create and manage posts           │                  │
│     │    ✓ Follow and unfollow accounts      │                  │
│     │                                         │                  │
│     │  [Authorize]  [Deny]                   │                  │
│     └───────────────────────────────────────┘                   │
│                                                                   │
│  5. User approves → Generate authorization code                  │
│                                                                   │
│  6. Redirect back to client with code                           │
│     http://localhost:4321/callback?code=xxx&state=yyy          │
│                                                                   │
│  7. Client exchanges code for access token                       │
│     POST https://dev.lesser.host/oauth/token                     │
│       code=xxx                                                   │
│       client_id=xxx                                             │
│       client_secret=xxx                                         │
│       redirect_uri=xxx                                          │
│       grant_type=authorization_code                             │
│                                                                   │
│  8. Lesser returns access + refresh tokens                       │
│     {                                                            │
│       "access_token": "eyJ...",                                 │
│       "token_type": "Bearer",                                   │
│       "scope": "read write follow",                             │
│       "created_at": 1730352000                                  │
│     }                                                            │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

## Authentication Methods

### WebAuthn (Passkeys)

**What it is:**
- Modern passwordless authentication using device biometrics or security keys
- Standards-based (FIDO2/WebAuthn)
- Phishing-resistant
- Device-bound credentials

**Supported Authenticators:**
- **Biometrics**: Face ID (iOS/macOS), Touch ID (iOS/macOS), Windows Hello
- **Security Keys**: YubiKey, Google Titan, Feitian, etc.
- **Platform Authenticators**: Built-in device security chips (TPM, Secure Enclave)

**API Endpoints:**
```bash
# Begin login (get challenge)
POST /api/v1/auth/webauthn/login/begin
{
  "username": "alice"
}

# Complete login (verify credential)
POST /api/v1/auth/webauthn/login/finish
{
  "username": "alice",
  "challenge": "abc123...",
  "response": { ... }, # Credential assertion
  "device_name": "Chrome on MacBook"
}

# Manage credentials (authenticated)
GET    /api/v1/auth/webauthn/credentials
POST   /api/v1/auth/webauthn/register/begin
POST   /api/v1/auth/webauthn/register/finish
DELETE /api/v1/auth/webauthn/credentials/{id}
PUT    /api/v1/auth/webauthn/credentials/{id}
```

### Crypto Wallet Authentication

**What it is:**
- Sign-In with Ethereum (SIWE) protocol
- Message signing with private keys
- No passwords, no secrets stored
- Self-sovereign identity

**Supported Wallets:**
- **MetaMask**: Most popular Ethereum wallet
- **WalletConnect**: Protocol for mobile wallets
- **Coinbase Wallet**: Coinbase's self-custody wallet
- **Rainbow**: Beautiful wallet with ENS support
- **Trust Wallet**: Multi-chain mobile wallet
- **Phantom**: Solana + Ethereum wallet

**Supported Chains:**
- Ethereum (mainnet & testnets)
- Polygon
- Base
- Optimism
- Arbitrum
- Any EVM-compatible chain

**API Endpoints:**
```bash
# Create authentication challenge
POST /auth/wallet/challenge
{
  "address": "0x1234...",
  "chainId": 1
}

# Verify signature and create session
POST /auth/wallet/verify
{
  "challengeId": "uuid",
  "address": "0x1234...",
  "signature": "0xabcd...",
  "message": "Sign in to Lesser..."
}

# Link wallet to existing account (authenticated)
POST   /auth/wallet/link
DELETE /auth/wallet/unlink/{address}
GET    /auth/wallet/list
```

## UI Components (Greater-Components)

All UI is built with Greater Components for consistency, accessibility, and maintainability.

### Auth UI Project Structure

```
lesser/
├── auth-ui/                      # Passwordless OAuth UI
│   ├── package.json              # Astro + Greater-components
│   ├── astro.config.mjs          # Static build config
│   ├── src/
│   │   ├── layouts/
│   │   │   └── AuthLayout.astro  # Base layout
│   │   ├── pages/
│   │   │   ├── login.astro       # /login - Passwordless auth
│   │   │   └── consent.astro     # /consent - OAuth approval
│   │   ├── components/
│   │   │   ├── PasswordlessLogin.svelte
│   │   │   └── OAuthConsentScreen.svelte
│   │   └── styles/
│   │       └── global.css        # Lesser theme
│   └── dist/                     # Built static assets
│       ├── login/index.html
│       ├── consent/index.html
│       └── _assets/*.{js,css}
```

### Deployment

**Infrastructure (CDK):**
- S3 bucket: `lesser-auth-ui-{domain}`
- CloudFront distribution with SSL
- Custom domain: `auth.{domain}` (e.g., `auth.dev.lesser.host`)
- Origin Access Identity for S3 security

**Commands:**
```bash
# Build auth UI locally
make build-auth-ui

# Deploy to AWS
make deploy-auth-ui DOMAIN=dev.lesser.host AWS_PROFILE=Lesser

# Output:
# ✓ Auth UI built to auth-ui/dist/
# ✓ Uploaded to S3
# ✓ CloudFront cache invalidated
# ✓ Available at https://auth.dev.lesser.host
```

**CDK Resources Created:**
1. **S3 Bucket**: `lesser-auth-ui-dev.lesser.host`
   - Private access only
   - Versioning disabled (static assets)
   - S3-managed encryption
   - Auto-delete in dev (retain in prod)

2. **CloudFront Distribution**:
   - Origin: S3 bucket with OAI
   - SSL/TLS: Certificate from shared stack
   - Compression: Gzip + Brotli
   - Caching: Optimized for static assets
   - Error handling: 404/403 → /login

3. **Route53 DNS**:
   - A record: `auth.dev.lesser.host` → CloudFront
   - AAAA record: IPv6 support

## OAuth Flow Implementation

### Step 1: Authorization Request

Client redirects user to:
```
GET https://dev.lesser.host/oauth/authorize
  ?response_type=code
  &client_id=CLIENT_ID
  &redirect_uri=http://localhost:4321/callback
  &scope=read+write+follow
  &state=RANDOM_STATE
  &code_challenge=PKCE_CHALLENGE
  &code_challenge_method=S256
```

**Lesser validates:**
- `client_id` exists
- `redirect_uri` matches registered URI (exact match)
- `scopes` are valid
- `code_challenge` if PKCE is used

### Step 2: Authentication Check

**If not authenticated:**
- Store OAuth params in `OAuthState` (DynamoDB)
- Redirect to: `https://auth.dev.lesser.host/login?return_to=...&auth_request=...`

**If authenticated:**
- Skip to Step 4 (consent check)

### Step 3: Passwordless Login

User chooses authentication method at `https://auth.dev.lesser.host/login`:

**Option A: WebAuthn**
1. Enter username
2. Click "Sign in with Passkey"
3. Device prompts for biometric/security key
4. Credential verified
5. Session created with JWT token
6. Redirect back to `/oauth/authorize`

**Option B: Wallet**
1. Click "Sign in with Crypto Wallet"
2. Wallet extension prompts for connection
3. User approves connection
4. Sign authentication message
5. Signature verified (ECDSA)
6. Session created if wallet is linked
7. Redirect back to `/oauth/authorize`

### Step 4: Consent Check

**If user previously consented:**
- Skip to Step 6 (generate code)

**If first time or new scopes:**
- Store `OAuthState` in DynamoDB
- Redirect to: `https://auth.dev.lesser.host/consent?state=...&client_id=...`

### Step 5: User Consent

At `https://auth.dev.lesser.host/consent`, user sees:
- App name and website
- Requested scopes with descriptions
- Security notice
- [Authorize] and [Deny] buttons

**User clicks Authorize:**
```
POST https://dev.lesser.host/oauth/consent
  state=STATE_ID
  action=approve
```

**Lesser:**
1. Validates state exists and not expired
2. Generates authorization code
3. Stores code in DynamoDB (10 min TTL)
4. Stores user consent for future requests
5. Redirects to `client redirect_uri` with code

**User clicks Deny:**
```
POST https://dev.lesser.host/oauth/consent
  state=STATE_ID
  action=deny
```

**Lesser:**
1. Redirects to `client redirect_uri` with `error=access_denied`

### Step 6: Authorization Code

User redirected to client callback:
```
http://localhost:4321/callback
  ?code=AUTH_CODE
  &state=RANDOM_STATE
```

Client validates `state` matches original request (CSRF protection).

### Step 7: Token Exchange

Client exchanges code for tokens:
```bash
POST https://dev.lesser.host/oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&code=AUTH_CODE
&client_id=CLIENT_ID
&client_secret=CLIENT_SECRET
&redirect_uri=http://localhost:4321/callback
&code_verifier=PKCE_VERIFIER  # If PKCE was used
```

**Lesser validates:**
- Code exists and not expired
- Code belongs to this client
- Redirect URI matches
- PKCE verifier matches challenge (if used)
- Client secret matches (if provided)

**Response:**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "scope": "read write follow",
  "created_at": 1730352000,
  "refresh_token": "refresh_xxx"  // If requested
}
```

### Step 8: API Access

Client uses access token for API requests:
```bash
GET https://dev.lesser.host/api/graphql
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

{
  "query": "{ actor(username: \"alice\") { id username } }"
}
```

## Security Features

### OAuth 2.0 Security

✅ **PKCE (Proof Key for Code Exchange)**
- Prevents authorization code interception
- Required for public clients
- SHA-256 code challenge

✅ **State Parameter**
- CSRF protection
- Client-generated random value
- Validated on callback

✅ **Exact Redirect URI Matching**
- No prefix matching
- No wildcard domains
- Strict validation per Mastodon spec

✅ **Short-lived Authorization Codes**
- 10 minute expiration
- Single use only
- Deleted after exchange

✅ **Refresh Token Rotation**
- New refresh token issued on each use
- Old token invalidated
- Detects token theft

### WebAuthn Security

✅ **Phishing Resistant**
- Origin-bound credentials
- Cannot be used on fake domains

✅ **No Shared Secrets**
- Private keys never leave device
- Server only stores public keys

✅ **Attestation Support**
- Verify authenticator is genuine
- Detect cloned credentials
- Track sign counts

✅ **User Verification**
- Requires biometric or PIN
- "userVerification": "required"

### Wallet Security

✅ **SIWE (Sign-In with Ethereum)**
- EIP-4361 standard
- Nonce prevents replay
- Time-bound challenges

✅ **Message Signing**
- Private key proves ownership
- ECDSA signature validation
- No secrets transmitted

✅ **Address Normalization**
- Checksummed addresses
- Case-insensitive comparison
- ENS resolution support

✅ **Chain ID Validation**
- Prevents cross-chain attacks
- Validates network matches

## Development Workflow

### 1. Initial Setup

```bash
# Install dependencies for auth-ui
cd auth-ui
pnpm install

# Link to greater-components workspace (if in monorepo)
cd ..
pnpm install

# Start development server
cd auth-ui
pnpm dev
# Auth UI available at http://localhost:4322
```

### 2. Local Development

```bash
# Terminal 1: Run Lesser API
make dev

# Terminal 2: Run auth UI dev server
cd auth-ui && pnpm dev

# Terminal 3: Run test client (optional)
cd test-client && pnpm dev
```

**Configure OAuth client for localhost:**
```bash
# Register OAuth app with localhost redirect
curl -X POST https://dev.lesser.host/api/v1/apps \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Local Dev Client",
    "redirect_uris": "http://localhost:4321/auth/callback",
    "scopes": "read write follow",
    "website": "http://localhost:4321"
  }'
```

### 3. Testing

```bash
# Test WebAuthn registration (requires authenticated session)
curl -X POST https://dev.lesser.host/api/v1/auth/webauthn/register/begin \
  -H "Authorization: Bearer TOKEN"

# Test wallet challenge
curl -X POST https://dev.lesser.host/auth/wallet/challenge \
  -H "Content-Type: application/json" \
  -d '{
    "address": "0x1234567890123456789012345678901234567890",
    "chainId": 1
  }'

# Test full OAuth flow
open "https://dev.lesser.host/oauth/authorize?client_id=XXX&redirect_uri=http://localhost:4321/callback&response_type=code&scope=read+write"
```

### 4. Deployment

```bash
# Build and deploy auth UI
make build-auth-ui
make deploy-auth-ui DOMAIN=dev.lesser.host AWS_PROFILE=Lesser

# Deploy CDK stack (creates S3 + CloudFront + DNS)
make deploy-dev DOMAIN=dev.lesser.host AWS_PROFILE=Lesser

# Verify deployment
curl -I https://auth.dev.lesser.host/login
# Should return 200 with HTML
```

## Integration Guide

### For Client Applications

**1. Register your application:**
```javascript
const response = await fetch('https://dev.lesser.host/api/v1/apps', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    client_name: 'My Fediverse Client',
    redirect_uris: 'https://myapp.com/oauth/callback',
    scopes: 'read write follow',
    website: 'https://myapp.com'
  })
});

const { client_id, client_secret } = await response.json();
```

**2. Initiate OAuth flow:**
```javascript
// Generate PKCE challenge
const codeVerifier = generateRandomString(128);
const codeChallenge = await sha256(codeVerifier);

// Build authorization URL
const authUrl = new URL('https://dev.lesser.host/oauth/authorize');
authUrl.searchParams.set('client_id', client_id);
authUrl.searchParams.set('redirect_uri', 'https://myapp.com/oauth/callback');
authUrl.searchParams.set('response_type', 'code');
authUrl.searchParams.set('scope', 'read write follow');
authUrl.searchParams.set('state', generateRandomString(32));
authUrl.searchParams.set('code_challenge', codeChallenge);
authUrl.searchParams.set('code_challenge_method', 'S256');

// Redirect user
window.location.href = authUrl.toString();
```

**3. Handle callback:**
```javascript
// Extract code and state from URL
const params = new URLSearchParams(window.location.search);
const code = params.get('code');
const state = params.get('state');

// Validate state matches (CSRF protection)
if (state !== savedState) {
  throw new Error('State mismatch - possible CSRF attack');
}

// Exchange code for tokens
const tokenResponse = await fetch('https://dev.lesser.host/oauth/token', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    client_id,
    client_secret,
    redirect_uri: 'https://myapp.com/oauth/callback',
    code_verifier: codeVerifier
  })
});

const { access_token, refresh_token } = await tokenResponse.json();

// Store tokens securely
localStorage.setItem('access_token', access_token);
localStorage.setItem('refresh_token', refresh_token);
```

**4. Make authenticated requests:**
```javascript
const response = await fetch('https://dev.lesser.host/api/graphql', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${access_token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    query: '{ actor(username: "alice") { id username displayName } }'
  })
});
```

## Troubleshooting

### Common Issues

**"Passkeys not supported"**
- Use a modern browser (Chrome 67+, Safari 14+, Edge 18+, Firefox 60+)
- Enable biometrics in OS settings
- Try a hardware security key as fallback

**"No wallet detected"**
- Install MetaMask or another Web3 wallet extension
- Ensure wallet is unlocked
- Check browser compatibility

**"Wallet not linked to any account"**
- You need to register first with your wallet address
- Or link the wallet to an existing account in settings

**"Invalid redirect_uri"**
- Redirect URI must exactly match what you registered
- Check for trailing slashes
- Protocol must match (http vs https)
- `localhost` is allowed for development

**"Code expired"**
- Authorization codes expire after 10 minutes
- Restart the OAuth flow

**"Invalid PKCE verifier"**
- Code verifier must match the code challenge
- Use SHA-256 for challenge generation
- Store verifier securely during the flow

## Future Enhancements

- [ ] Social recovery (ActivityPub-based account recovery via trusted actors)
- [ ] Backup codes (emergency access if all auth methods are lost)
- [ ] Session management UI (view and revoke active sessions)
- [ ] OAuth app permissions dashboard
- [ ] Multi-factor combinations (passkey + wallet required)
- [ ] Hardware wallet support (Ledger, Trezor)
- [ ] Solana wallet support (Phantom, Solflare)

## References

- [WebAuthn Spec](https://www.w3.org/TR/webauthn-2/)
- [SIWE Spec (EIP-4361)](https://eips.ethereum.org/EIPS/eip-4361)
- [OAuth 2.0 RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749)
- [PKCE RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636)
- [Mastodon OAuth](https://docs.joinmastodon.org/client/token/)

---

**Security Note:** Lesser never stores passwords. All authentication is cryptographic (WebAuthn credentials or wallet signatures). This eliminates password breaches, credential stuffing, and phishing attacks.

