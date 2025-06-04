# Lesser Architecture Decisions

This document records key architectural decisions for the Lesser project. Each decision includes context, options considered, and rationale.

## Decision Log

### 1. Private Key Storage Encryption
**Status:** 🚨 PENDING IMPLEMENTATION  
**Decision:** Use AWS KMS for private key encryption

**Context:**
- ActivityPub requires private keys for HTTP signatures
- Currently storing private keys in plaintext (security risk)
- Need secure, auditable encryption

**Options Considered:**
1. **AWS KMS** ✅ SELECTED
   - Use a single Customer Master Key (CMK)
   - Encrypt/decrypt private keys on demand
   - ~$1/month + API calls
2. Application-level encryption
   - Would require secure master key management
   - More complex key rotation

**Rationale:**
- KMS provides hardware security modules (HSM)
- Automatic key rotation
- CloudTrail audit logs
- No key management burden

**Implementation:**
```go
// In pkg/storage/dynamodb/actor.go
func (s *dynamoDBStorage) encryptPrivateKey(ctx context.Context, plaintext string) (string, error) {
    input := &kms.EncryptInput{
        KeyId:     aws.String(s.kmsKeyID),
        Plaintext: []byte(plaintext),
    }
    result, err := s.kmsClient.Encrypt(ctx, input)
    if err != nil {
        return "", fmt.Errorf("failed to encrypt private key: %w", err)
    }
    return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
}
```

---

### 2. HTTP Signatures for Federation
**Status:** ✅ IMPLEMENTED  
**Decision:** Implement HTTP Signatures following draft-cavage-http-signatures-12

**Context:**
- ActivityPub requires HTTP signatures for server-to-server authentication
- Need to verify incoming federation requests
- Need to sign outgoing federation requests

**Implementation Completed:**
- `pkg/federation/httpsig.go` - Core implementation
- RSA-SHA256 algorithm support
- Timestamp validation (±5 minutes window)
- Digest calculation and verification
- Key management utilities (RSA key generation, PEM encoding)
- 87.4% test coverage
- Comprehensive documentation in `pkg/federation/README.md`

**Features:**
- `VerifyHTTPSignature()` - Verify incoming requests
- `SignHTTPRequest()` - Sign outgoing requests
- `GenerateRSAKeyPair()` - Generate RSA keys (2048-bit minimum)
- PEM encoding/decoding utilities

**Future Enhancements:**
- Ed25519 support for more efficient signatures
- Integration with AWS KMS for private key encryption
- Public key caching to reduce lookups

---

### 3. Client Authentication Strategy
**Status:** 📋 PLANNED  
**Decision:** OAuth 2.0 with PKCE

**Context:**
- Need secure client-to-server authentication
- Most ActivityPub clients expect OAuth 2.0
- Must support third-party apps

**Options Considered:**
1. JWT with refresh tokens
   - Simpler to implement
   - Would require migration later
2. **OAuth 2.0** ✅ SELECTED
   - Industry standard
   - Existing client support
   - PKCE for mobile app security

**Rationale:**
- Avoid future migration from JWT to OAuth
- Immediate compatibility with Mastodon apps
- Better security with authorization codes
- Supports scopes for fine-grained permissions

**Implementation Plan:**
```go
// pkg/auth/oauth/
├── server.go        // OAuth 2.0 server implementation
├── tokens.go        // Token generation and validation
├── clients.go       // Client app registration
├── scopes.go        // Permission scopes
└── pkce.go          // PKCE support
```

**OAuth 2.0 Endpoints:**
- `/oauth/authorize` - Authorization endpoint
- `/oauth/token` - Token endpoint
- `/oauth/revoke` - Token revocation
- `/api/v1/apps` - Client registration (Mastodon API compatible)

---

### 4. Activity Delivery Architecture
**Status:** 📋 PLANNED  
**Decision:** DynamoDB Streams → Lambda

**Context:**
- Need to deliver activities to remote servers
- Must handle retries and failures gracefully
- Should scale automatically

**Options Considered:**
1. **DynamoDB Streams** ✅ SELECTED
   - Automatic triggering
   - Built-in retry logic
   - No additional infrastructure
2. SQS Queue
   - More control but more complexity
   - Additional component to manage

**Rationale:**
- Leverages existing DynamoDB infrastructure
- Automatic scaling with Lambda
- Built-in error handling and DLQ
- Can add SQS later if needed

---

### 5. GetActivity Performance
**Status:** 🔧 OPTIMIZATION NEEDED  
**Decision:** Add GSI2 for activity lookups

**Context:**
- Current implementation uses Scan (inefficient)
- Need O(1) activity lookups by ID
- Activities can be in any user's outbox

**Solution:**
Add a second Global Secondary Index:
```
GSI2PK: ACTIVITY#{activity_id}
GSI2SK: METADATA
```

**Rationale:**
- Enables efficient GetActivity queries
- Minimal storage overhead
- Can be added without data migration

---

### 6. Shared Inbox Strategy
**Status:** 🟢 DEFERRED  
**Decision:** Implement individual inboxes first

**Context:**
- Shared inbox improves efficiency for multiple followers
- Adds routing complexity
- Not required for MVP

**Rationale:**
- Start simple with individual delivery
- Can add shared inbox endpoint later
- No breaking changes required
- Measure actual performance first

---

### 7. Media Storage Architecture
**Status:** 🟢 DEFERRED  
**Decision:** S3 with CloudFront CDN

**Context:**
- Need to store images, videos, avatars
- Must serve media efficiently
- Cost optimization important

**Planned Implementation:**
- S3 bucket with lifecycle policies
- CloudFront for global CDN
- Presigned URLs for uploads
- Image processing Lambda for thumbnails

---

### 8. Federation Protocol Support
**Status:** 📋 PLANNED  
**Decision:** ActivityPub S2S only initially

**Context:**
- Multiple federation protocols exist
- ActivityPub is the modern standard
- Some servers support legacy protocols

**Rationale:**
- Focus on ActivityPub for MVP
- Most active development in Fediverse
- Can add OStatus/Diaspora later if needed

---

### 9. Database Design
**Status:** ✅ IMPLEMENTED  
**Decision:** Single-table DynamoDB design

**Context:**
- Need efficient queries at scale
- Cost-effective for serverless
- Must support various access patterns

**Implementation:**
- Single table with composite keys
- GSI1 for inbox queries
- GSI2 (planned) for activity lookups
- Optimized for common queries

---

### 10. Lambda Architecture
**Status:** ✅ IMPLEMENTED  
**Decision:** One Lambda per endpoint

**Context:**
- Need fast cold starts
- Independent scaling per endpoint
- Clear separation of concerns

**Benefits:**
- Minimal cold start time
- Independent deployment
- Easier debugging
- Per-endpoint metrics

---

### 11. Infrastructure as Code
**Status:** 📋 PLANNED  
**Decision:** Pulumi with TypeScript

**Context:**
- Need reproducible deployments
- Version control for infrastructure
- Support for multiple environments

**Rationale:**
- Type safety with TypeScript
- Better than CloudFormation/SAM
- Supports all AWS resources
- Great Go SDK integration

---

## Decision Process

When making architecture decisions:

1. **Document the context** - Why is this decision needed?
2. **List options** - What alternatives were considered?
3. **Explain rationale** - Why was this option chosen?
4. **Plan implementation** - How will it be built?
5. **Set status** - Is it implemented, planned, or pending?

## Status Key

- 🚨 **PENDING IMPLEMENTATION** - Blocking progress
- 📋 **PLANNED** - Scheduled for implementation
- 🔧 **OPTIMIZATION NEEDED** - Working but needs improvement
- 🟢 **DEFERRED** - Intentionally postponed
- ✅ **IMPLEMENTED** - Completed 