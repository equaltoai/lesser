# Authentication Infrastructure Security

## Core Security Principles

Since Lesser is headless ActivityPub infrastructure, security must be built into the APIs that frontends consume. Every authentication method must be secure by default.

## Infrastructure Security Layers

### 1. API Gateway Security

**Rate Limiting**
```go
// DynamoDB-backed rate limiting
PK: RATELIMIT#endpoint#/api/v1/auth/login
SK: IP#1.2.3.4#2025-01-15T10:00:00Z
    requests: 5
    window: "1m"
    
// Per-user rate limiting
PK: RATELIMIT#user#alice
SK: ACTION#login#2025-01-15T10:00:00Z
    attempts: 3
```

**Request Validation**
- Input sanitization
- Request size limits
- Content-Type validation
- CORS configuration

### 2. Credential Storage Security

**Encryption at Rest**
```go
// All sensitive data encrypted in DynamoDB
type CredentialRecord struct {
    PK           string
    SK           string
    PublicKey    string    // Not encrypted
    PrivateData  string    // AES-256-GCM encrypted
    KeyVersion   int       // For key rotation
}

// KMS key rotation
aws kms create-key --description "lesser-auth-encryption"
```

**No Password Storage**
- Never store passwords (use passkeys/wallets)
- If legacy passwords needed, use Argon2id
- Immediate migration path to passkeys

### 3. Token Security

**JWT Best Practices**
```go
type TokenClaims struct {
    // Standard claims
    Subject   string    `json:"sub"`
    IssuedAt  int64     `json:"iat"`
    ExpiresAt int64     `json:"exp"`
    
    // Lesser-specific
    AuthMethod string   `json:"auth_method"`
    DeviceID   string   `json:"device_id"`
    IPAddress  string   `json:"ip"`
    Scopes     []string `json:"scopes"`
}

// Short-lived access tokens (15 min)
// Refresh tokens in database only
```

**Token Revocation**
```go
// Blacklist for emergency revocation
PK: REVOKED#TOKEN
SK: jti#token-id
    reason: "compromised"
    revokedAt: "2025-01-15T..."
    expiresAt: "2025-01-15T..." // Auto-cleanup
```

### 4. Transport Security

**HTTPS Everywhere**
- CloudFront enforces TLS 1.3
- HSTS headers
- Certificate pinning for mobile apps

**API Response Headers**
```go
headers := map[string]string{
    "Strict-Transport-Security": "max-age=31536000",
    "X-Content-Type-Options": "nosniff",
    "X-Frame-Options": "DENY",
    "X-XSS-Protection": "1; mode=block",
    "Referrer-Policy": "strict-origin-when-cross-origin",
}
```

### 5. Audit & Monitoring

**Authentication Events**
```go
// Store all auth events
PK: AUDIT#USER#alice
SK: 2025-01-15T10:00:00Z#event-id
    action: "login"
    method: "passkey"
    ip: "1.2.3.4"
    userAgent: "..."
    success: true
    deviceId: "credential-xyz"
```

**Anomaly Detection**
- New device/location alerts
- Unusual activity patterns
- Failed authentication spikes
- Concurrent session limits

**CloudWatch Metrics**
```go
// Track key security metrics
- AuthenticationAttempts
- AuthenticationFailures
- NewDeviceRegistrations
- TokenRevocations
- RateLimitHits
```

## Passkey-Specific Security

### Challenge Generation
```go
func generateChallenge() (string, error) {
    // Cryptographically secure random
    challenge := make([]byte, 32)
    _, err := rand.Read(challenge)
    if err != nil {
        return "", err
    }
    
    // Store with expiration
    // PK: CHALLENGE#base64(challenge)
    // TTL: 5 minutes
    
    return base64.URLEncoding.EncodeToString(challenge), nil
}
```

### Authenticator Validation
- Verify origin matches
- Check user verification flag
- Validate signature counter (replay protection)
- Store authenticator AAGUID

## Wallet Authentication Security

### Message Format (EIP-4361)
```
lesser.social wants you to sign in with your Ethereum account:
0x1234...abcd

Sign in to Lesser

URI: https://lesser.social
Version: 1
Chain ID: 1
Nonce: abc123
Issued At: 2025-01-15T10:00:00Z
Expiration Time: 2025-01-15T10:05:00Z
```

### Signature Verification
```go
func verifyWalletSignature(message, signature string, address string) error {
    // 1. Verify message format
    // 2. Check nonce hasn't been used
    // 3. Verify timestamp windows
    // 4. Recover address from signature
    // 5. Compare with claimed address
}
```

## Session Security

### Device Tracking
```go
PK: USER#alice
SK: DEVICE#device-id
    name: "Chrome on macOS"
    firstSeen: "2025-01-01T..."
    lastSeen: "2025-01-15T..."
    passkeys: ["credential-xyz"]
    trustScore: 100
```

### Concurrent Session Management
- Limit active sessions per user
- Show active sessions in UI
- One-click "logout everywhere"
- Automatic cleanup of stale sessions

## Privacy Considerations

### Data Minimization
- Don't store unnecessary PII
- No tracking beyond security needs
- Clear data retention policies
- User-controlled data export

### Anonymous Authentication
- Support privacy-preserving auth methods
- No email required for wallet auth
- Optional profile information
- Respect "Do Not Track"

## Implementation Checklist

### Phase 1: Foundation (Week 1)
- [ ] Rate limiting infrastructure
- [ ] JWT token generation/validation
- [ ] Audit logging system
- [ ] Basic security headers

### Phase 2: Passkeys (Week 2)  
- [ ] WebAuthn challenge/response
- [ ] Credential storage
- [ ] Device management
- [ ] Security event logging

### Phase 3: Wallets (Week 3)
- [ ] EIP-4361 message generation
- [ ] Signature verification
- [ ] Multi-chain support
- [ ] ENS integration

### Phase 4: Hardening (Week 4)
- [ ] Penetration testing
- [ ] Security monitoring dashboards
- [ ] Incident response playbooks
- [ ] Documentation

## Cost Impact

### Security Infrastructure
- KMS encryption: ~$1/month
- CloudWatch logs: ~$0.50/GB
- Additional DynamoDB: ~$0.0001/user
- **Total: <$0.001 per user per month**

## Security Best Practices for Frontends

### Example: Secure Passkey Implementation
```javascript
// Frontend should:
// 1. Use HTTPS only
// 2. Validate responses
// 3. Handle errors gracefully
// 4. Never log sensitive data

class LesserAuth {
  async loginWithPasskey() {
    try {
      // Get challenge from Lesser
      const { challenge, timeout } = await this.api.getChallenge();
      
      // Set timeout
      const controller = new AbortController();
      setTimeout(() => controller.abort(), timeout);
      
      // Use WebAuthn
      const credential = await navigator.credentials.get({
        publicKey: {
          challenge: base64ToBuffer(challenge),
          timeout,
          userVerification: "preferred"
        },
        signal: controller.signal
      });
      
      // Send to Lesser
      const { token } = await this.api.verifyCredential(credential);
      
      // Store securely
      this.secureStorage.setToken(token);
      
    } catch (error) {
      // Don't expose internal errors
      throw new Error('Authentication failed');
    }
  }
}
```

## Monitoring & Alerts

### Key Metrics to Watch
1. Failed auth ratio > 20%
2. New device registrations spike
3. Concurrent sessions > limit
4. Rate limit violations
5. Token revocations increase

### Alert Thresholds
```yaml
AuthFailureRate:
  threshold: 0.2
  period: 5m
  action: notify-on-call

NewDeviceSpike:
  threshold: 10x-baseline
  period: 1h
  action: investigate

RateLimitViolation:
  threshold: 100
  period: 1m
  action: auto-block-ip
```

## Conclusion

This security architecture provides:
- **Modern authentication** without passwords
- **Infrastructure-level security** that frontends inherit
- **Cost-effective** implementation (<$0.001/user)
- **Privacy-respecting** design
- **Standards-based** approach

Lesser becomes a secure foundation that any frontend can build upon with confidence. 