# Lesser Security Features

Lesser implements comprehensive security measures designed for production-grade federated social media. This document details all security implementations across authentication, federation, infrastructure, and data protection.

## Authentication Security

### Multi-Factor Authentication Options

Lesser provides multiple authentication methods without requiring traditional passwords or email verification:

#### 1. WebAuthn (Passkeys)
- **Implementation**: `pkg/auth/webauthn.go`
- **Features**: 
  - FIDO2/WebAuthn compliant passkey authentication
  - Hardware security keys (YubiKey, etc.)
  - Platform authenticators (Touch ID, Windows Hello)
  - Maximum 10 credentials per user
  - 5-minute challenge expiration
- **Configuration**:
  ```go
  wconfig := &webauthn.Config{
      RPDisplayName: displayName,
      RPID:          domain,
      RPOrigins:     []string{"https://" + domain},
  }
  ```

#### 2. OAuth 2.0 with PKCE
- **Implementation**: `pkg/auth/oauth_test.go`, `pkg/auth/providers/provider.go`
- **Security Features**:
  - PKCE (Proof Key for Code Exchange) mandatory
  - State parameter validation
  - Secure token storage with refresh rotation
  - Multiple provider support (GitHub, Google, Twitter, etc.)

#### 3. Crypto Wallet Authentication
- **Implementation**: Web3 wallet integration
- **Features**:
  - Ethereum wallet signature verification
  - Message signing for authentication
  - Decentralized identity without centralized providers

### Token Management
- **Implementation**: `pkg/auth/refresh_tokens_test.go`
- **Features**:
  - JWT with secure refresh token rotation
  - Configurable expiration times
  - Token revocation and blacklisting
  - Rate limiting on token endpoints

### Session Security
- **CSRF Protection**: Implemented in `pkg/auth/csrf_test.go`
- **Secure Headers**: Auto-configured security headers
- **Rate Limiting**: Per-user and per-IP limits

## Federation Security

### HTTP Signature Verification

Lesser implements both legacy and modern HTTP signature verification:

#### Enhanced HTTP Signatures
- **Implementation**: `pkg/federation/httpsig_enhanced.go`
- **Standards Supported**:
  - Draft-ietf-httpbis-message-signatures (modern)
  - Legacy HTTP Signatures (for compatibility)
- **Key Features**:
  ```go
  type EnhancedHTTPSignature struct {
      HTTPSignature
      Created int64 // Unix timestamp (for hs2019)
      Expires int64 // Unix timestamp (for hs2019)
  }
  ```

#### Signature Verification Process
- **Clock Skew Tolerance**: Configurable window for time-based signatures
- **Public Key Caching**: TTL-based caching with `pkg/storage/models/public_key_cache.go`
- **Retry Logic**: Automatic retry on key fetch failures
- **Algorithm Support**: RSA, ECDSA, Ed25519

### Digest Verification
- **SHA-256 Content Verification**: All incoming ActivityPub messages
- **Body Integrity**: Prevents tampering in transit
- **Error Classification**: Clear error taxonomy for debugging

### Actor Verification
- **Actor Discovery**: Secure WebFinger and Actor document fetching
- **Key Rotation**: Automatic handling of public key updates
- **Domain Validation**: Anti-spoofing measures for actor domains

## Infrastructure Security

### Serverless Security Model

Lesser leverages AWS Lambda's inherent security advantages:

#### Function Isolation
- **Process Isolation**: Each request runs in isolated environment
- **No Persistent State**: Eliminates many attack vectors
- **Automatic Scaling**: DDoS mitigation through Lambda scaling

#### Network Security
- **API Gateway Integration**: Built-in DDoS protection
- **CloudFront CDN**: Additional DDoS mitigation and caching
- **VPC Optional**: Can run with VPC for additional isolation

### Rate Limiting

Comprehensive rate limiting across all endpoints:

#### Implementation Details
- **File**: `pkg/ratelimit/middleware_test.go`
- **Multi-Level Limiting**:
  - Per-user limits (authenticated requests)
  - Per-IP limits (unauthenticated requests)  
  - Per-domain limits (federation)
- **Sliding Window**: Precise rate control with time windows
- **Headers**: Standard `X-RateLimit-*` headers

#### Rate Limit Categories
```go
// Example rate limits
const (
    PostRateLimit      = 100 // posts per hour
    FollowRateLimit    = 200 // follows per hour
    MediaUploadLimit   = 50  // uploads per hour
    FederationLimit    = 1000 // inbox per hour per domain
)
```

### Data Security

#### Encryption at Rest
- **DynamoDB**: Server-side encryption with AWS KMS
- **S3 Media**: AES-256 encryption for all media files
- **Secrets Management**: AWS Secrets Manager for sensitive config

#### Encryption in Transit
- **TLS 1.3**: All API endpoints require HTTPS
- **HTTP/2**: Modern protocol with security improvements
- **Certificate Management**: Automatic SSL/TLS via AWS Certificate Manager

### Content Security

#### Media Processing Security
- **File Type Validation**: Strict MIME type checking
- **Content Scanning**: AWS Rekognition integration for harmful content
- **Size Limits**: Configurable upload limits
- **Virus Scanning**: Optional integration with AWS Macie

#### Content Moderation Security
- **Implementation**: `pkg/moderation/advanced/engine.go`
- **Features**:
  - AI-powered content analysis
  - Trust network for distributed moderation
  - Appeal process with audit trails
  - Rate limiting on moderation actions

## Access Control

### Role-Based Access Control (RBAC)
- **User Roles**: User, Moderator, Admin, Instance Admin
- **Permission Granularity**: Fine-grained permissions per endpoint
- **Scope Validation**: OAuth scopes enforce API access limits

### Activity Authorization
- **Actor Matching**: Ensures users can only act as themselves
- **Ownership Validation**: Content modification requires ownership
- **Visibility Checks**: Respect privacy settings and followers-only content

## Security Headers

Lesser automatically sets comprehensive security headers:

```http
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000; includeSubDomains
Content-Security-Policy: default-src 'self'
Referrer-Policy: strict-origin-when-cross-origin
```

## Monitoring and Incident Response

### Security Monitoring
- **Implementation**: `pkg/observability/constants.go`
- **Metrics Tracked**:
  - Authentication failures
  - Signature verification failures
  - Rate limit violations
  - Suspicious patterns

### Alert Configuration
```go
// Security alert thresholds
const (
    AlertP0ErrorRatePercent = 10.0  // Critical error rate
    AlertP1ErrorRatePercent = 5.0   // High error rate
    AlertP2ErrorRatePercent = 2.0   // Warning error rate
)
```

### Incident Response
- **Automated Responses**: Rate limiting, IP blocking
- **Manual Escalation**: Alerts for security team
- **Audit Logging**: Comprehensive security event logging

## Data Protection

### Privacy Controls
- **Account Privacy**: Public, unlisted, followers-only, direct
- **Content Warnings**: Sensitive content handling
- **Block/Mute**: User-controlled visibility
- **Data Retention**: Configurable content expiration

### GDPR Compliance
- **Data Export**: Complete account data export
- **Data Deletion**: Right to be forgotten implementation
- **Consent Management**: Clear privacy controls
- **Data Minimization**: Only required data collected

## Vulnerability Management

### Security Scanning
- **Dependency Scanning**: Automated security updates
- **Code Analysis**: Static analysis integration
- **Penetration Testing**: Regular security assessments

### Update Process
- **Security Patches**: Rapid deployment for critical fixes
- **Version Control**: Git-based change tracking
- **Rollback Capability**: Immediate rollback on security issues

## Configuration Security

### Environment Security
```bash
# Required secure configuration
PRIVATE_KEY_SECRET=<secure-key>    # ActivityPub signing key
JWT_SECRET=<secure-random>         # JWT signing key
OAUTH_CLIENT_SECRET=<provider>     # OAuth provider secrets
```

### Secrets Management
- **AWS Secrets Manager**: Production secret storage
- **Environment Variables**: Development configuration
- **Rotation**: Automated key rotation capabilities

## Federation Security Best Practices

### Incoming Activity Validation
1. **HTTP Signature Verification**: All activities must be cryptographically signed
2. **Actor Verification**: Confirm activity sender matches signature
3. **Content Validation**: Sanitize and validate all incoming content
4. **Rate Limiting**: Prevent federation abuse

### Outgoing Activity Security
1. **Signature Generation**: Sign all outbound activities
2. **Retry Logic**: Secure retry with exponential backoff
3. **Error Handling**: Don't leak internal information
4. **Delivery Tracking**: Monitor delivery success/failure

## Implementation Files Reference

### Core Security Files
- `pkg/auth/webauthn.go` - WebAuthn/Passkey implementation
- `pkg/auth/oauth_test.go` - OAuth 2.0 with PKCE
- `pkg/federation/httpsig_enhanced.go` - HTTP signature verification
- `pkg/ratelimit/middleware_test.go` - Rate limiting implementation
- `cmd/inbox/main.go` - Federation security (lines 1-200)

### Security Testing
- `pkg/auth/csrf_test.go` - CSRF protection tests
- `pkg/auth/refresh_tokens_test.go` - Token security tests
- `pkg/federation/httpsig_test.go` - Signature verification tests

### Configuration
- `pkg/observability/constants.go` - Security monitoring constants
- `pkg/common/security_logger.go` - Security event logging

## Troubleshooting Security Issues

### Authentication Problems
```bash
# Check WebAuthn configuration
curl -X POST https://your-instance.com/auth/webauthn/register/begin

# Verify OAuth configuration
curl -X GET https://your-instance.com/.well-known/oauth-authorization-server
```

### Federation Security Debugging
```bash
# Test HTTP signature verification
curl -X POST https://your-instance.com/inbox \
  -H "Signature: keyId=...,signature=..." \
  -H "Digest: SHA-256=..." \
  -d @activity.json

# Check actor key cache
curl -X GET https://your-instance.com/users/username
```

### Rate Limiting
```bash
# Check rate limit headers
curl -I https://your-instance.com/api/v1/statuses
# Look for: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
```

## Security Checklist for Deployment

- [ ] Configure HTTPS with valid SSL certificate
- [ ] Set strong JWT and private keys in Secrets Manager
- [ ] Enable CloudWatch logging for security events
- [ ] Configure rate limiting thresholds for your instance size
- [ ] Test WebAuthn registration and authentication flows
- [ ] Verify HTTP signature verification with test federation
- [ ] Set up monitoring alerts for security metrics
- [ ] Review and configure content moderation settings
- [ ] Enable automated security scanning
- [ ] Document incident response procedures

Lesser's security architecture provides defense in depth while maintaining the simplicity and cost-effectiveness of serverless deployment. All security features are production-tested and designed to handle the unique challenges of federated social media.