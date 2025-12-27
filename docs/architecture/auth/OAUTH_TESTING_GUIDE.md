# OAuth Testing Guide

Comprehensive testing guide for Lesser's passwordless OAuth system.

## Prerequisites

### 1. WebAuthn Testing

**Browser Requirements:**
- Chrome 67+, Safari 14+, Edge 18+, or Firefox 60+
- macOS: Touch ID or Face ID enabled
- Windows: Windows Hello configured
- Linux: Security key (YubiKey, etc.)

**Setup:**
```bash
# Ensure you have a WebAuthn credential registered
# This typically happens during account registration
# Or use the settings page to add a passkey
```

### 2. Wallet Testing

**Required:**
- MetaMask browser extension (or other Web3 wallet)
- Test wallet with any EVM network
- Test ETH for gas (not needed for signing, but good practice)

**Setup:**
```bash
# Install MetaMask
# https://metamask.io/download/

# Create or import a test wallet
# Get test ETH from faucet (optional):
# - Sepolia: https://sepoliafaucet.com/
# - Goerli: https://goerlifaucet.com/
```

### 3. Local Development Environment

```bash
# Terminal 1: Run Lesser API
cd /path/to/lesser
./lesser dev

# Terminal 2: Run Auth UI (development)
cd auth-ui
pnpm install
pnpm dev
# Available at http://localhost:4322/auth

# Note:
# - Auth UI builds under /auth (e.g., http://localhost:4322/auth/login)
# - If the API is on a different origin, set PUBLIC_LESSER_API_ORIGIN (e.g., PUBLIC_LESSER_API_ORIGIN=http://localhost:8080 pnpm dev)
```

## Test Scenarios

### Scenario 1: WebAuthn Login Flow

**Steps:**
1. Register OAuth client:
```bash
curl -X POST http://localhost:8080/api/v1/apps \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Test Client",
    "redirect_uris": "http://localhost:4321/callback",
    "scopes": "read write follow"
  }'
# Save the client_id and client_secret
```

2. Initiate OAuth flow:
```
http://localhost:8080/oauth/authorize?client_id=CLIENT_ID&redirect_uri=http://localhost:4321/callback&response_type=code&scope=read+write&state=random123
```

3. You should be redirected to auth UI (http://localhost:4322/auth/login or https://dev.lesser.host/auth/login)

4. Enter your username

5. Click "Sign in with Passkey"

6. Browser prompts for biometric/security key

7. After successful authentication, redirected back to `/oauth/authorize`

8. Consent screen shows (if first time)

9. Click "Authorize"

10. Redirected to callback with authorization code

**Expected Result:**
```
http://localhost:4321/callback?code=AUTHORIZATION_CODE&state=random123
```

**Validation:**
```bash
# Exchange code for token
curl -X POST http://localhost:8080/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=AUTHORIZATION_CODE&client_id=CLIENT_ID&client_secret=CLIENT_SECRET&redirect_uri=http://localhost:4321/callback"

# Should return:
# {
#   "access_token": "eyJ...",
#   "token_type": "Bearer",
#   "scope": "read write follow",
#   "created_at": 1730352000
# }
```

### Scenario 2: Wallet Login Flow

**Steps:**
1. Register OAuth client (same as above if not done)

2. Initiate OAuth flow (same URL)

3. Redirected to auth UI

4. Click "Sign in with Crypto Wallet"

5. MetaMask (or other wallet) prompts for connection

6. Approve connection

7. Wallet shows message to sign

8. Sign message in wallet

9. After verification, redirected to `/oauth/authorize`

10. Consent screen shows

11. Authorize app

**Expected Result:**
Authorization code returned to callback URL

**Validation:**
```bash
# Verify wallet is linked
TOKEN="eyJ..."  # From previous login

curl -X GET http://localhost:8080/auth/wallet/list \
  -H "Authorization: Bearer $TOKEN"

# Should list linked wallets including the one just used
```

### Scenario 3: PKCE Flow (Public Clients)

**Setup:**
```javascript
// Client-side JavaScript for PKCE
function generateCodeVerifier() {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return base64URLEncode(array);
}

async function sha256(plain) {
  const encoder = new TextEncoder();
  const data = encoder.encode(plain);
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
const codeChallenge = await sha256(codeVerifier);

// Store verifier for later
sessionStorage.setItem('pkce_verifier', codeVerifier);
```

**OAuth URL with PKCE:**
```
http://localhost:8080/oauth/authorize
  ?client_id=CLIENT_ID
  &redirect_uri=http://localhost:4321/callback
  &response_type=code
  &scope=read+write
  &state=random123
  &code_challenge=CODE_CHALLENGE
  &code_challenge_method=S256
```

**Token Exchange with PKCE:**
```bash
curl -X POST http://localhost:8080/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=AUTH_CODE&client_id=CLIENT_ID&redirect_uri=http://localhost:4321/callback&code_verifier=CODE_VERIFIER"
```

**Expected:** Token issued successfully (PKCE prevents code interception)

### Scenario 4: Consent Persistence

**Test:** Previously consented apps skip consent screen

**Steps:**
1. Complete OAuth flow once (Scenario 1 or 2)
2. Initiate OAuth flow again with same client and scopes
3. Should skip consent screen and immediately issue authorization code

**Expected:** No consent screen shown on second authorization

### Scenario 5: Scope Changes

**Test:** New scopes trigger consent screen again

**Steps:**
1. Complete OAuth flow with `scope=read`
2. Initiate OAuth flow with `scope=read+write+follow`
3. Should show consent screen with new scopes highlighted

**Expected:** Consent screen shown with all requested scopes

### Scenario 6: Invalid Redirect URI

**Test:** Security validation rejects mismatched redirect URIs

**Steps:**
```
http://localhost:8080/oauth/authorize
  ?client_id=CLIENT_ID
  &redirect_uri=https://evil.com/callback  # Different from registered
  &response_type=code
  &scope=read
```

**Expected:** Error response (not redirect to evil.com)

### Scenario 7: Expired Challenge

**Test:** WebAuthn/Wallet challenges expire after 5 minutes

**Steps:**
1. Begin WebAuthn login
2. Wait 6 minutes without completing
3. Try to finish login with old challenge

**Expected:** Error: "challenge expired" or "challenge not found"

### Scenario 8: Multiple Authenticators

**Test:** Users with multiple passkeys/wallets can choose

**Steps:**
1. Register 2+ passkeys (e.g., laptop + phone)
2. Begin WebAuthn login
3. Browser/OS shows authenticator selection

**Expected:** User can choose which passkey to use

### Scenario 9: Cross-Device Flow

**Test:** Start on mobile, complete on desktop

**Steps:**
1. Mobile: Navigate to OAuth authorize URL
2. Mobile: Choose "Sign in with Passkey"
3. Mobile: Scan QR code to continue on desktop (if supported by authenticator)
4. Desktop: Complete authentication

**Expected:** Flow continues across devices

### Scenario 10: Error Handling

**Test Cases:**

**A. User cancels authentication:**
- Click "Sign in with Passkey"
- Cancel the biometric prompt
- Expected: Error message, can retry

**B. Wallet rejects signature:**
- Click "Sign in with Crypto Wallet"
- Connect wallet
- Reject signature request
- Expected: Error message, can retry

**C. Network failure:**
- Disconnect internet mid-flow
- Expected: Graceful error, retry button

**D. Invalid state parameter:**
```
POST http://localhost:8080/oauth/consent
  state=INVALID_STATE
  action=approve
```
- Expected: 400 Bad Request - invalid state

## Automated Testing

### Unit Tests (Go)

```bash
# Test OAuth service layer
go test ./pkg/auth/... -run TestOAuth -v

# Test WebAuthn service
go test ./pkg/auth/... -run TestWebAuthn -v

# Test wallet auth
go test ./pkg/auth/... -run TestWallet -v
```

### Integration Tests (Python)

```bash
# End-to-end OAuth flow
python3 tests/auth/test_oauth.py

# WebAuthn flow
python3 tests/auth/test_webauthn.py

# Wallet flow
python3 tests/auth/test_wallet.py
```

### Browser Tests (Playwright)

```bash
# Install Playwright
cd auth-ui
pnpm add -D @playwright/test

# Run E2E tests
pnpm test:e2e
```

**Sample Playwright test:**
```typescript
// tests/oauth-flow.spec.ts
import { test, expect } from '@playwright/test';

test('complete OAuth flow with WebAuthn', async ({ page, context }) => {
  // Navigate to OAuth authorize
  await page.goto('http://localhost:8080/oauth/authorize?...');
  
  // Should redirect to login page
  await expect(page).toHaveURL(/auth\.dev\.lesser\.host\/login/);
  
  // Enter username
  await page.fill('#username', 'testuser');
  
  // Click WebAuthn button
  await page.click('button:has-text("Sign in with Passkey")');
  
  // WebAuthn dialog appears (mocked in Playwright with virtual authenticator)
  // ...
  
  // Should redirect to consent
  await expect(page).toHaveURL(/auth\.dev\.lesser\.host\/consent/);
  
  // Approve authorization
  await page.click('button:has-text("Authorize")');
  
  // Should redirect to callback with code
  await expect(page).toHaveURL(/localhost:4321\/callback\?code=/);
  
  // Extract authorization code
  const url = new URL(page.url());
  const code = url.searchParams.get('code');
  expect(code).toBeTruthy();
});
```

## Performance Testing

### Load Test OAuth Endpoints

```bash
# Using k6
cd tests/k6
k6 run oauth-load-test.js
```

**Metrics to monitor:**
- Authorization requests/second
- Token exchange latency
- WebAuthn challenge generation time
- Wallet signature verification time
- DynamoDB read/write latency
- CloudFront cache hit ratio

### Expected Performance

- **Authorization request**: < 500ms
- **Token exchange**: < 300ms
- **WebAuthn begin**: < 200ms
- **WebAuthn finish**: < 400ms
- **Wallet challenge**: < 150ms
- **Wallet verify**: < 300ms

## Security Testing

### Penetration Testing Checklist

- [ ] CSRF protection (state parameter)
- [ ] Redirect URI validation (no open redirects)
- [ ] PKCE validation (code verifier/challenge)
- [ ] Token expiration (authorization codes, access tokens)
- [ ] Scope escalation prevention
- [ ] WebAuthn origin validation
- [ ] Wallet signature verification
- [ ] Rate limiting on auth endpoints
- [ ] Brute force protection
- [ ] Session fixation prevention

### Security Scan

```bash
# OAuth endpoint security
./lesser sec-scan

# Check for vulnerabilities
./lesser vuln-check
```

## Monitoring & Debugging

### CloudWatch Logs

```bash
# CloudFront access logs (if enabled)
# Note: bucket naming is deployment-specific; use your stack outputs / console to find the log bucket.
AWS_PROFILE=<profile> aws s3 ls s3://<cloudfront-log-bucket>/

# API Lambda logs (OAuth endpoints)
AWS_PROFILE=<profile> aws logs tail /aws/lambda/<app>-dev-api \
  --since 10m \
  --filter-pattern "oauth"

# WebAuthn logs
AWS_PROFILE=<profile> aws logs tail /aws/lambda/<app>-dev-api \
  --since 10m \
  --filter-pattern "webauthn"

# Wallet auth logs
AWS_PROFILE=<profile> aws logs tail /aws/lambda/<app>-dev-api \
  --since 10m \
  --filter-pattern "wallet"
```

### Metrics to Track

**OAuth Metrics:**
- Authorization requests (count, p50, p99)
- Token exchanges (count, p50, p99)
- Consent approvals vs denials
- Error rate by type

**Auth Metrics:**
- WebAuthn success rate
- Wallet auth success rate
- Most used wallet providers
- Authenticator distribution

**Security Metrics:**
- Failed login attempts
- Invalid redirect_uri attempts
- Expired challenge rate
- PKCE usage rate

## Troubleshooting

### Common Errors

**"route not found: GET /auth/login"**
- **Cause**: Auth UI not deployed or DNS not configured
- **Fix**: Ensure the Auth UI assets are deployed under `https://<stage-domain>/auth/*` (see `docs/architecture/auth/PASSWORDLESS_OAUTH.md`).

**"redirect to external host not allowed: localhost"**
- **Cause**: Over-strict redirect validation
- **Fix**: Already fixed - OAuth validates via registered redirect URIs

**"service registry not initialized"**
- **Cause**: Registry initialization failed on Lambda cold start
- **Fix**: Check CloudWatch logs for initialization errors, verify DynamoDB permissions

**"WebAuthn not supported"**
- **Cause**: Old browser or HTTP (not HTTPS)
- **Fix**: Use modern browser, ensure HTTPS (or localhost exception)

**"No wallet detected"**
- **Cause**: No Web3 provider installed
- **Fix**: Install MetaMask or use passkey option

**"Wallet not linked to any account"**
- **Cause**: Wallet address not associated with user
- **Fix**: Register new account with wallet, or link wallet in account settings

### Debug Mode

Enable verbose logging:
```bash
# Set environment variable on Lambda
AWS_PROFILE=Lesser aws lambda update-function-configuration \
  --function-name <app>-dev-api \
  --environment Variables={DEBUG=true,LOG_LEVEL=debug,...}
```

## Next Steps After Testing

1. **Security Audit**: Third-party review of OAuth implementation
2. **Load Testing**: Stress test with realistic user loads
3. **Accessibility Audit**: WCAG 2.1 AA compliance for auth UI
4. **Mobile Testing**: iOS Safari, Android Chrome
5. **Internationalization**: Add multi-language support
6. **Analytics**: Track conversion rates through OAuth funnel

---

**Note**: This system eliminates password-related vulnerabilities (breaches, credential stuffing, phishing). All authentication is cryptographic (WebAuthn or wallet signatures).
