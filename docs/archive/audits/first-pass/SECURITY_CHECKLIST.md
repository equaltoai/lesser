# Lesser Security Implementation Checklist

## 🔴 Critical Issues (Fix Immediately)

### Authentication
- [ ] **GraphQL**: Add auth middleware to `cmd/graphql/main.go`
- [ ] **REST API**: Replace manual routing with router + auth middleware in `cmd/api/main.go`
- [ ] **Outbox**: Add auth to GET endpoint in `cmd/outbox/main.go`

### Data Protection
- [ ] **XSS**: Replace HTML sanitizer with bluemonday in `pkg/activitypub/validation.go`
- [ ] **Blocked Users**: Filter recipients in `cmd/outbox/main.go`

### SSRF Prevention
- [ ] **Create Secure HTTP Client**: New package `pkg/httpclient`
- [ ] **Update All Services**: Replace direct HTTP calls
  - [ ] `pkg/federation/delivery.go`
  - [ ] `pkg/federation/authorized_fetch.go`
  - [ ] `cmd/inbox/main.go`

## 🟠 High Priority Issues

### Input Validation
- [ ] **Size Limits**: Add `io.LimitReader` to:
  - [ ] `cmd/inbox/main.go` - handlePostInbox
  - [ ] `cmd/outbox/main.go` - handlePostOutbox
  - [ ] `cmd/media-processor/main.go` - downloadFromS3
- [ ] **File Validation**: Check file types in `cmd/media-processor/main.go`

### Security Improvements
- [ ] **Path Traversal**: Sanitize usernames in S3 keys
- [ ] **Secure IDs**: Replace math/rand with crypto/rand
- [ ] **Rate Limiting**: Implement lockout records

## 🟡 Medium Priority Issues

### Code Quality
- [ ] **Type Safety**: Custom UnmarshalJSON for ActivityPub types
- [ ] **Logging**: Replace fmt.Printf with structured logging
- [ ] **Session Security**: Detect refresh token reuse

### Federation
- [ ] **Delivery Retries**: Add exponential backoff
- [ ] **HTTP Signatures**: Support multiple algorithms
- [ ] **JSON Limits**: Prevent DoS on federation endpoints

## 🟢 Low Priority Issues

### Validation
- [ ] **Error Messages**: Fix address validation errors
- [ ] **Coverage**: Add validation for all ActivityPub types
- [ ] **Fail Closed**: Change security check behavior

### WebAuthn
- [ ] **User Verification**: Make configurable
- [ ] **Multiple Origins**: Support via config
- [ ] **Resident Keys**: Add support

## Quick Implementation Guide

### 1. Adding Authentication Middleware

```go
// Example for any Lambda handler
func lambdaHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // First thing: Check auth
    user, err := validateAuth(request.Headers["Authorization"])
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 401,
            Body: "Unauthorized",
        }, nil
    }
    
    // Continue with authenticated user
    ctx = context.WithValue(ctx, "user", user)
    // ... rest of handler
}
```

### 2. Secure HTTP Client Usage

```go
// Replace this:
resp, err := http.Get(url)

// With this:
client := httpclient.NewSecureClient()
resp, err := client.Get(url)
```

### 3. Request Size Limiting

```go
// Replace this:
body, err := io.ReadAll(request.Body)

// With this:
const maxSize = 1 << 20 // 1MB
body, err := io.ReadAll(io.LimitReader(request.Body, maxSize))
```

### 4. HTML Sanitization

```go
// Replace custom sanitizer with:
import "github.com/microcosm-cc/bluemonday"

var sanitizer = bluemonday.UGCPolicy()
clean := sanitizer.Sanitize(userInput)
```

### 5. Secure ID Generation

```go
// Replace this:
id := generateRandomString() // uses math/rand

// With this:
import "crypto/rand"
b := make([]byte, 16)
rand.Read(b)
id := hex.EncodeToString(b)
```

## Testing Your Security Fixes

### Auth Testing
```bash
# Should return 401
curl -X POST https://api.lesser.app/graphql

# Should work with valid token
curl -X POST https://api.lesser.app/graphql \
  -H "Authorization: Bearer $TOKEN"
```

### SSRF Testing
```bash
# These should all be rejected:
# - http://169.254.169.254/
# - http://localhost/
# - http://127.0.0.1/
# - http://10.0.0.1/
```

### Size Limit Testing
```bash
# Should reject large payloads
dd if=/dev/zero bs=1M count=10 | \
  curl -X POST https://api.lesser.app/inbox \
    -H "Content-Type: application/json" \
    --data-binary @-
```

## Security Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [Go Security Best Practices](https://github.com/OWASP/Go-SCP)
- [AWS Security Best Practices](https://aws.amazon.com/architecture/security-identity-compliance/)

---

Remember: **When in doubt, fail closed!** It's better to reject a legitimate request than to allow a malicious one. 