# Federation Package

This package implements HTTP Signatures for ActivityPub federation authentication, following the [draft-cavage-http-signatures-12](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures-12) specification.

## Overview

HTTP Signatures provide a way to authenticate HTTP requests between ActivityPub servers. This package provides:

- **Signature verification** for incoming federation requests
- **Signature generation** for outgoing federation requests
- **RSA key management** utilities
- **Digest calculation and verification** for request bodies

## Usage

### Verifying Incoming Requests

```go
import (
    "net/http"
    "github.com/aron23/lesser/pkg/federation"
)

func handleInbox(w http.ResponseWriter, r *http.Request) {
    // Fetch the sender's public key (usually from their actor profile)
    publicKey := fetchActorPublicKey(r)
    
    // Verify the HTTP signature
    err := federation.VerifyHTTPSignature(r, publicKey)
    if err != nil {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }
    
    // Optionally verify the digest if there's a body
    if r.Body != nil {
        body, _ := io.ReadAll(r.Body)
        err = federation.VerifyDigest(r, body)
        if err != nil {
            http.Error(w, "Invalid digest", http.StatusBadRequest)
            return
        }
    }
    
    // Process the authenticated request...
}
```

### Signing Outgoing Requests

```go
func sendActivity(activity interface{}, recipientInbox string) error {
    // Serialize the activity
    body, _ := json.Marshal(activity)
    
    // Create the request
    req, _ := http.NewRequest("POST", recipientInbox, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/activity+json")
    
    // Load your actor's private key
    privateKey := loadPrivateKey()
    keyID := "https://example.com/users/alice#main-key"
    
    // Sign the request
    err := federation.SignHTTPRequest(req, privateKey, keyID)
    if err != nil {
        return err
    }
    
    // Send the request
    client := &http.Client{}
    resp, err := client.Do(req)
    // Handle response...
}
```

### Key Management

#### Generating Keys

```go
// Generate a new RSA key pair (2048-bit minimum)
privateKey, err := federation.GenerateRSAKeyPair(2048)
if err != nil {
    log.Fatal(err)
}

// Get the public key
publicKey := &privateKey.PublicKey

// Encode keys to PEM format for storage
publicPEM, _ := federation.EncodePublicKeyPEM(publicKey)
privatePEM, _ := federation.EncodePrivateKeyPEM(privateKey)
```

#### Parsing Keys

```go
// Parse public key from PEM
publicKey, err := federation.ParsePublicKeyPEM(publicPEM)

// Parse private key from PEM
privateKey, err := federation.ParsePrivateKeyPEM(privatePEM)
```

## Headers Signed

By default, the following headers are included in signatures:

- `(request-target)`: The HTTP method and path
- `host`: The target host
- `date`: Request timestamp
- `digest`: Body content digest (for requests with bodies)
- `content-type`: For requests with bodies

## Security Considerations

1. **Timestamp Validation**: Requests are rejected if the timestamp is more than 5 minutes in the past or future to prevent replay attacks.

2. **Minimum Key Size**: RSA keys must be at least 2048 bits for security.

3. **Digest Verification**: Always verify the digest header for requests with bodies to ensure content integrity.

4. **Key Storage**: Private keys should be stored securely and encrypted (future enhancement will use AWS KMS).

## Integration with ActivityPub

In ActivityPub, actors include their public key in their profile:

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice",
  "type": "Person",
  "preferredUsername": "alice",
  "publicKey": {
    "id": "https://example.com/users/alice#main-key",
    "owner": "https://example.com/users/alice",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\n..."
  }
}
```

The `keyId` in HTTP signatures should match the `id` field of the public key.

## Example Signature Header

```
Signature: keyId="https://example.com/users/alice#main-key",
           algorithm="rsa-sha256",
           headers="(request-target) host date digest",
           signature="base64-encoded-signature"
```

## Testing

The package includes comprehensive tests:

```bash
go test ./pkg/federation -v
```

Tests cover:
- Signature parsing and validation
- Signature generation and verification
- Key operations
- Digest calculation and verification
- Edge cases and error conditions

## Future Enhancements

1. **AWS KMS Integration**: Encrypt private keys using AWS KMS for better security
2. **Ed25519 Support**: Add support for Ed25519 signatures (more efficient than RSA)
3. **Caching**: Cache public keys to reduce lookup overhead
4. **Metrics**: Add CloudWatch metrics for signature verification failures 