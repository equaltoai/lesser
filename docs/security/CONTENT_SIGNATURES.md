# Quick Start: Implementing Content Signatures in Lesser

## Overview

This guide walks through implementing cryptographic content signatures as your first security enhancement. This feature provides immediate value with minimal complexity.

## Why Start Here?

1. **Visible Impact**: Users see "Verified ✓" badges
2. **Federation Ready**: Other servers can verify authenticity
3. **Foundation**: Enables future features (C2PA, edit history)
4. **Low Risk**: Doesn't break existing functionality
5. **Marketing Win**: "Every post cryptographically signed"

## Implementation Steps

### Step 1: Create Signature Types

```go
// pkg/crypto/signatures/types.go
package signatures

import (
    "crypto/ecdsa"
    "time"
)

type ContentSignature struct {
    // Required fields
    Type       string    `json:"type"` // "DataSignature2022"
    Creator    string    `json:"creator"` // Actor ID who signed
    Created    time.Time `json:"created"`
    
    // Signature data
    Algorithm  string    `json:"signatureAlgorithm"` // "ES256"
    Value      string    `json:"signatureValue"` // Base64 encoded
    
    // Optional proof fields
    Domain     string    `json:"domain,omitempty"` // Instance domain
    Nonce      string    `json:"nonce,omitempty"` // Replay prevention
    ProofPurpose string  `json:"proofPurpose,omitempty"` // "assertionMethod"
}

type SignableContent interface {
    GetID() string
    GetType() string
    GetAuthorID() string
    GetContent() map[string]interface{}
}
```

### Step 2: Implement Signing Service

```go
// pkg/crypto/signatures/signer.go
package signatures

import (
    "crypto/ecdsa"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "math/big"
)

type Signer struct {
    instanceDomain string
    // In production, load from KMS
    privateKey     *ecdsa.PrivateKey
}

func NewSigner(domain string, privateKey *ecdsa.PrivateKey) *Signer {
    return &Signer{
        instanceDomain: domain,
        privateKey:     privateKey,
    }
}

func (s *Signer) SignContent(content SignableContent) (*ContentSignature, error) {
    // 1. Create canonical representation
    canonical, err := s.canonicalize(content)
    if err != nil {
        return nil, fmt.Errorf("canonicalize: %w", err)
    }
    
    // 2. Hash the canonical form
    hash := sha256.Sum256(canonical)
    
    // 3. Sign with ECDSA
    r, sig_s, err := ecdsa.Sign(rand.Reader, s.privateKey, hash[:])
    if err != nil {
        return nil, fmt.Errorf("sign: %w", err)
    }
    
    // 4. Encode signature
    signature := append(r.Bytes(), sig_s.Bytes()...)
    
    return &ContentSignature{
        Type:         "DataSignature2022",
        Creator:      content.GetAuthorID(),
        Created:      time.Now().UTC(),
        Algorithm:    "ES256",
        Value:        base64.StdEncoding.EncodeToString(signature),
        Domain:       s.instanceDomain,
        ProofPurpose: "assertionMethod",
    }, nil
}

func (s *Signer) canonicalize(content SignableContent) ([]byte, error) {
    // Create deterministic JSON representation
    data := map[string]interface{}{
        "@context": "https://www.w3.org/ns/activitystreams",
        "id":       content.GetID(),
        "type":     content.GetType(),
        "actor":    content.GetAuthorID(),
        "content":  content.GetContent(),
    }
    
    // Sort keys for deterministic output
    return json.Marshal(data)
}
```

### Step 3: Add Verification

```go
// pkg/crypto/signatures/verifier.go
package signatures

import (
    "crypto/ecdsa"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "math/big"
)

type Verifier struct {
    // Cache of public keys
    keyCache map[string]*ecdsa.PublicKey
}

func (v *Verifier) VerifySignature(
    content SignableContent, 
    sig *ContentSignature,
) error {
    // 1. Fetch public key for creator
    publicKey, err := v.getPublicKey(sig.Creator)
    if err != nil {
        return fmt.Errorf("get public key: %w", err)
    }
    
    // 2. Recreate canonical form
    canonical, err := canonicalize(content)
    if err != nil {
        return fmt.Errorf("canonicalize: %w", err)
    }
    
    // 3. Hash it
    hash := sha256.Sum256(canonical)
    
    // 4. Decode signature
    sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
    if err != nil {
        return fmt.Errorf("decode signature: %w", err)
    }
    
    // 5. Extract r and s
    r := new(big.Int).SetBytes(sigBytes[:32])
    s := new(big.Int).SetBytes(sigBytes[32:])
    
    // 6. Verify
    if !ecdsa.Verify(publicKey, hash[:], r, s) {
        return fmt.Errorf("signature verification failed")
    }
    
    return nil
}
```

### Step 4: Integrate with Status Creation

```go
// cmd/api/handlers/statuses.go
// Update CreateStatus handler

func (h *Handler) CreateStatus(w http.ResponseWriter, r *http.Request) {
    // ... existing status creation code ...
    
    // Sign the status before storing
    if h.signer != nil {
        signature, err := h.signer.SignContent(&StatusWrapper{status})
        if err != nil {
            h.logger.Error("Failed to sign status", "error", err)
            // Continue without signature - don't break posting
        } else {
            status.Signature = signature
        }
    }
    
    // ... store status ...
}

// Wrapper to implement SignableContent
type StatusWrapper struct {
    *models.Status
}

func (s *StatusWrapper) GetContent() map[string]interface{} {
    return map[string]interface{}{
        "content":     s.Content,
        "visibility":  s.Visibility,
        "sensitive":   s.Sensitive,
        "spoilerText": s.SpoilerText,
    }
}
```

### Step 5: Add Storage Fields

```go
// pkg/storage/types.go
// Update Status struct

type Status struct {
    // ... existing fields ...
    
    // Signature information
    Signature *ContentSignature `dynamodbav:"signature,omitempty"`
    
    // For edit history
    PreviousVersions []string `dynamodbav:"previous_versions,omitempty"`
    EditedAt        *time.Time `dynamodbav:"edited_at,omitempty"`
}
```

### Step 6: Federation Integration

```go
// pkg/activitypub/objects.go
// Add signature to ActivityPub objects

func StatusToActivity(status *Status) map[string]interface{} {
    activity := map[string]interface{}{
        "@context": "https://www.w3.org/ns/activitystreams",
        "id":       status.ID,
        "type":     "Note",
        "content":  status.Content,
        // ... other fields ...
    }
    
    // Include signature if present
    if status.Signature != nil {
        activity["signature"] = status.Signature
    }
    
    return activity
}
```

### Step 7: Add Verification Endpoint

```go
// cmd/api/handlers/verify.go
package handlers

func (h *Handler) VerifyContent(w http.ResponseWriter, r *http.Request) {
    contentID := chi.URLParam(r, "id")
    
    // Fetch content
    status, err := h.storage.GetStatus(r.Context(), contentID)
    if err != nil {
        respondError(w, http.StatusNotFound, "Content not found")
        return
    }
    
    // Verify signature
    if status.Signature == nil {
        respondJSON(w, http.StatusOK, map[string]interface{}{
            "verified": false,
            "reason":   "No signature present",
        })
        return
    }
    
    err = h.verifier.VerifySignature(&StatusWrapper{status}, status.Signature)
    if err != nil {
        respondJSON(w, http.StatusOK, map[string]interface{}{
            "verified": false,
            "reason":   err.Error(),
        })
        return
    }
    
    respondJSON(w, http.StatusOK, map[string]interface{}{
        "verified":   true,
        "signer":     status.Signature.Creator,
        "signed_at":  status.Signature.Created,
        "algorithm":  status.Signature.Algorithm,
    })
}
```

### Step 8: Lambda for Key Management

```go
// cmd/key-manager/main.go
// Lambda function to generate and rotate keys

package main

import (
    "context"
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/x509"
    "encoding/pem"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go/service/kms"
    "github.com/aws/aws-sdk-go/service/ssm"
)

func generateActorKeys(ctx context.Context, actorID string) error {
    // 1. Generate ECDSA key pair
    privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return err
    }
    
    // 2. Encode private key
    privDER, err := x509.MarshalECPrivateKey(privateKey)
    if err != nil {
        return err
    }
    
    privPEM := pem.EncodeToMemory(&pem.Block{
        Type:  "EC PRIVATE KEY",
        Bytes: privDER,
    })
    
    // 3. Store in Parameter Store (encrypted)
    ssmClient := ssm.New(session.Must(session.NewSession()))
    _, err = ssmClient.PutParameter(&ssm.PutParameterInput{
        Name:  aws.String(fmt.Sprintf("/lesser/keys/%s/private", actorID)),
        Value: aws.String(string(privPEM)),
        Type:  aws.String("SecureString"),
    })
    
    // 4. Store public key in DynamoDB
    // ... store public key for federation ...
    
    return nil
}
```

## Testing

### Unit Tests

```go
// pkg/crypto/signatures/signer_test.go
func TestSignAndVerify(t *testing.T) {
    // Generate test key
    privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    
    // Create signer
    signer := NewSigner("example.com", privateKey)
    
    // Create test content
    content := &TestContent{
        ID:      "https://example.com/status/123",
        Type:    "Note",
        Author:  "https://example.com/users/alice",
        Content: map[string]interface{}{"text": "Hello, world!"},
    }
    
    // Sign
    signature, err := signer.SignContent(content)
    assert.NoError(t, err)
    
    // Verify
    verifier := &Verifier{}
    err = verifier.VerifySignature(content, signature)
    assert.NoError(t, err)
}
```

### Integration Tests

```python
# test_content_signatures.py
def test_signed_status_creation():
    # Create status
    response = client.post("/api/v1/statuses", json={
        "status": "This is a signed post!"
    })
    
    status_id = response.json()["id"]
    
    # Verify signature
    verify_response = client.get(f"/api/v1/verify/{status_id}")
    
    assert verify_response.json()["verified"] == True
    assert "signer" in verify_response.json()
```

## Rollout Plan

### Week 1: Development
- [ ] Implement core signing/verification
- [ ] Add storage fields
- [ ] Create key management Lambda

### Week 2: Integration
- [ ] Update status creation flow
- [ ] Add verification endpoint
- [ ] Federation integration

### Week 3: Testing
- [ ] Unit tests
- [ ] Integration tests
- [ ] Performance testing

### Week 4: Launch
- [ ] Documentation
- [ ] UI indicators (if Greater is ready)
- [ ] Marketing announcement

## UI/UX Considerations

For the Greater frontend:

```jsx
// Verified badge component
function VerifiedBadge({ status }) {
    if (!status.signature) return null;
    
    return (
        <Tooltip content={`Verified ${formatDate(status.signature.created)}`}>
            <span className="verified-badge">✓</span>
        </Tooltip>
    );
}

// In status display
<div className="status-header">
    <span className="author">{status.account.display_name}</span>
    <VerifiedBadge status={status} />
</div>
```

## Success Metrics

- **Technical**: % of posts signed, verification success rate
- **Performance**: Signing latency (<10ms target)
- **Adoption**: User awareness of verification
- **Security**: Tampering attempts detected

## Next Steps

After content signatures are working:

1. **Edit History**: Link edits cryptographically
2. **Media Signatures**: Implement C2PA for images
3. **Cross-Instance Verification**: Verify federated content
4. **Key Rotation**: Automated key management

---

This implementation provides a solid foundation for Lesser's security enhancements while being straightforward to implement and visible to users. 