# AI Assistant Prompt for Lesser Development

You are an expert Go developer specializing in serverless architectures and federated social networking protocols. You will be helping to build **Lesser**, a cost-effective serverless ActivityPub implementation using AWS Lambda and DynamoDB.

## Project Overview

Lesser is a serverless ActivityPub server designed to minimize hosting costs while providing full ActivityPub compliance. Instead of traditional always-on servers, it uses:
- AWS Lambda for compute (pay per request)
- DynamoDB for storage (pay per use)
- API Gateway for HTTP endpoints
- S3 for media storage
- Pulumi for infrastructure as code

The goal is to make hosting an ActivityPub instance affordable for individuals and small communities (estimated ~$23/month for 100 users).

## Current Project State

### Completed ✅
1. **Architecture Design** (see DESIGN.md)
   - Single DynamoDB table design with composite keys
   - Lambda per endpoint pattern
   - Event-driven architecture with SQS

2. **Developer Guidelines** (see DEVELOPER_GUIDELINES.md)
   - Technology choices: zap for logging, no heavy frameworks
   - Naming conventions and code organization
   - Testing strategy with examples

3. **Core Packages**
   - `pkg/activitypub/` - ActivityPub types and validation
   - `pkg/config/` - Environment-based configuration
   - `pkg/common/` - Logging, errors, and response utilities

4. **DynamoDB Storage Layer** (NEW ✅)
   - `pkg/storage/dynamodb/client.go` - Connection pooling, initialization
   - `pkg/storage/dynamodb/actor.go` - Full actor CRUD operations
   - `pkg/storage/dynamodb/activity.go` - Activity storage with pagination
   - Comprehensive unit and integration tests
   - >80% test coverage

5. **First Lambda Function**
   - `cmd/webfinger/` - WebFinger discovery endpoint (needs storage connection)

### Partially Complete 🚧
1. **Storage Operations**
   - ✅ Actor operations (CRUD)
   - ✅ Activity operations (outbox/inbox with pagination)
   - ❌ Object operations (Notes, Articles, etc.)
   - ❌ Relationship operations (follows)
   - ❌ Collection operations

## Your Task

Continue development by implementing the **HTTP Signatures Package**, which is critical for federation. HTTP Signatures are required for:
- Authenticating incoming federation requests
- Signing outgoing federation requests
- Establishing trust between ActivityPub servers

### Important Prerequisites

⚠️ **Before implementing HTTP Signatures**, be aware of these architectural decisions (see ARCHITECTURE_DECISIONS.md):

1. **Private Key Encryption**: We need to implement AWS KMS encryption for private keys. Currently they're stored in plaintext.
2. **OAuth 2.0 Authentication**: We're using OAuth 2.0 (not JWT) for client authentication to ensure compatibility with existing ActivityPub clients.

### 1. Create HTTP Signatures Package Structure
Create `pkg/federation/httpsig.go` with:
- Signature verification for incoming requests
- Signature generation for outgoing requests
- Key management utilities
- Support for common algorithms (RSA-SHA256)

### 2. Implement Signature Verification
```go
// VerifyHTTPSignature verifies an incoming HTTP request's signature
func VerifyHTTPSignature(req *http.Request, publicKey *rsa.PublicKey) error
```

Key requirements:
- Parse the `Signature` header
- Validate required headers are included (date, host, digest)
- Verify the signature matches
- Check timestamp is within acceptable range (±5 minutes)
- Support for both RSA and Ed25519 keys

### 3. Implement Signature Generation
```go
// SignHTTPRequest signs an outgoing HTTP request
func SignHTTPRequest(req *http.Request, privateKey *rsa.PrivateKey, keyID string) error
```

Key requirements:
- Calculate digest for request body
- Build signature string from headers
- Sign using private key
- Add `Signature` header to request

### 4. Create Key Management Utilities
- RSA key generation (2048-bit minimum)
- Key serialization/deserialization
- PEM encoding/decoding
- Integration with storage layer for key retrieval

### 5. Write Comprehensive Tests
- Unit tests for signature parsing
- Verification tests with known-good signatures
- Generation tests with verification
- Interoperability tests with reference implementations
- Edge cases (expired signatures, missing headers, etc.)

## Technical Requirements

### HTTP Signature Specification
Follow the draft specification: [draft-cavage-http-signatures-12](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures-12)

Key headers to support:
- `(request-target)`: The HTTP method and path
- `host`: The target host
- `date`: Request timestamp
- `digest`: Body content digest (SHA-256)
- `content-type`: For POST requests

### Example Signature Header
```
Signature: keyId="https://example.com/users/alice#main-key",
           algorithm="rsa-sha256",
           headers="(request-target) host date digest",
           signature="base64-encoded-signature"
```

### Integration Points
1. The `inbox` handler will use this to verify incoming activities
2. The `activity-processor` will use this to sign outgoing activities
3. Actor profiles must include public keys for verification

## Example Implementation Pattern

```go
package federation

import (
    "crypto"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "net/http"
    "strings"
    "time"
    
    "github.com/lesser/lesser/pkg/common"
    "go.uber.org/zap"
)

// HTTPSignature represents a parsed HTTP signature
type HTTPSignature struct {
    KeyID     string
    Algorithm string
    Headers   []string
    Signature []byte
}

// ParseSignatureHeader parses the Signature header
func ParseSignatureHeader(header string) (*HTTPSignature, error) {
    // Implementation here
}

// VerifyHTTPSignature verifies an incoming request
func VerifyHTTPSignature(req *http.Request, publicKey crypto.PublicKey) error {
    log := common.Logger()
    
    // Parse signature header
    sigHeader := req.Header.Get("Signature")
    if sigHeader == "" {
        return common.AuthenticationError{Message: "missing signature header"}
    }
    
    sig, err := ParseSignatureHeader(sigHeader)
    if err != nil {
        return fmt.Errorf("failed to parse signature: %w", err)
    }
    
    // Verify timestamp
    date := req.Header.Get("Date")
    if err := verifyTimestamp(date); err != nil {
        return err
    }
    
    // Build signature string
    sigString, err := buildSignatureString(req, sig.Headers)
    if err != nil {
        return err
    }
    
    // Verify signature
    // ... verification logic
    
    log.Info("verified HTTP signature",
        zap.String("key_id", sig.KeyID),
        zap.String("method", req.Method),
        zap.String("path", req.URL.Path))
    
    return nil
}
```

## Testing Strategy

Create `pkg/federation/httpsig_test.go` with:
- Table-driven tests for signature parsing
- Mock HTTP requests for testing
- Known-good signatures from other implementations
- Integration tests with real keys

## Success Criteria

- [ ] HTTP signature verification working
- [ ] HTTP signature generation working  
- [ ] Key management utilities implemented
- [ ] Unit tests with >80% coverage
- [ ] Integration tests with reference signatures
- [ ] Documentation with examples
- [ ] Ready for use in inbox/outbox handlers

## Next Steps After This Task

1. **Connect Storage to Handlers**: Update WebFinger and create Actor endpoint
2. **Implement Inbox Handler**: Receive and verify federated activities
3. **Implement Activity Processor**: Background processing of activities
4. **Complete Remaining Storage**: Objects, relationships, collections

Begin by studying the HTTP Signatures specification and examining how other ActivityPub implementations handle federation authentication. 