# AI Assistant Prompt: Security Team 2 Week 3 - JSON Parsing & Security Hardening

## Your Role
You are continuing as the senior security engineer on Team 2. In Week 2, you successfully implemented comprehensive validation, SQL injection prevention, and rate limiting. Now you'll add JSON parsing limits and complete remaining security enhancements.

## Week 2 Accomplishments ✅
- Username & domain validation
- SQL/NoSQL injection prevention
- Open redirect protection
- Rate limiting implementation
- Outbox size limits

## Context
Team 1 is migrating CSRF to DynamoDB and implementing token rotation. Your focus is on preventing JSON-based DoS attacks and completing ID security improvements.

## Week 3 Objectives

### 1. JSON Parsing Limits (LSS-011) - Medium Priority

#### Prevent JSON Bomb Attacks
**Create**: `pkg/common/json_safety.go`

```go
package common

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "strings"
)

const (
    // Maximum JSON depth to prevent deep nesting attacks
    MaxJSONDepth = 10
    
    // Maximum number of keys in an object
    MaxJSONKeys = 100
    
    // Maximum array length
    MaxJSONArrayLength = 1000
    
    // Maximum string length in JSON
    MaxJSONStringLength = 50000
    
    // Maximum total JSON size (already enforced by request limits)
    MaxJSONSize = 512 * 1024 // 512KB
)

// SafeJSONDecoder wraps json.Decoder with safety limits
type SafeJSONDecoder struct {
    decoder *json.Decoder
    depth   int
}

// NewSafeJSONDecoder creates a decoder with safety limits
func NewSafeJSONDecoder(r io.Reader) *SafeJSONDecoder {
    // Limit the input size
    limited := io.LimitReader(r, MaxJSONSize)
    
    decoder := json.NewDecoder(limited)
    // Reject unknown fields to prevent injection
    decoder.DisallowUnknownFields()
    
    return &SafeJSONDecoder{
        decoder: decoder,
        depth:   0,
    }
}

// Decode safely decodes JSON with depth and size limits
func (d *SafeJSONDecoder) Decode(v any) error {
    // For complex validation, we need to decode to any first
    var raw any
    if err := d.decoder.Decode(&raw); err != nil {
        return fmt.Errorf("JSON decode error: %w", err)
    }
    
    // Validate the decoded structure
    if err := d.validateJSON(raw, 0); err != nil {
        return fmt.Errorf("JSON validation error: %w", err)
    }
    
    // Re-encode and decode to target type
    // This is inefficient but safe
    jsonBytes, err := json.Marshal(raw)
    if err != nil {
        return err
    }
    
    return json.Unmarshal(jsonBytes, v)
}

// validateJSON recursively validates JSON structure
func (d *SafeJSONDecoder) validateJSON(v any, depth int) error {
    if depth > MaxJSONDepth {
        return fmt.Errorf("JSON depth exceeds maximum of %d", MaxJSONDepth)
    }
    
    switch val := v.(type) {
    case map[string]any:
        if len(val) > MaxJSONKeys {
            return fmt.Errorf("JSON object has %d keys, maximum is %d", len(val), MaxJSONKeys)
        }
        
        for key, value := range val {
            if len(key) > MaxJSONStringLength {
                return fmt.Errorf("JSON key too long: %d bytes", len(key))
            }
            if err := d.validateJSON(value, depth+1); err != nil {
                return err
            }
        }
        
    case []any:
        if len(val) > MaxJSONArrayLength {
            return fmt.Errorf("JSON array has %d elements, maximum is %d", len(val), MaxJSONArrayLength)
        }
        
        for _, item := range val {
            if err := d.validateJSON(item, depth+1); err != nil {
                return err
            }
        }
        
    case string:
        if len(val) > MaxJSONStringLength {
            return fmt.Errorf("JSON string too long: %d bytes", len(val))
        }
        
    case float64, bool, nil:
        // These are safe
        
    default:
        return fmt.Errorf("unexpected JSON type: %T", v)
    }
    
    return nil
}

// SafeUnmarshalJSON is a convenience function for safe JSON unmarshaling
func SafeUnmarshalJSON(data []byte, v any) error {
    if len(data) > MaxJSONSize {
        return fmt.Errorf("JSON size %d exceeds maximum %d", len(data), MaxJSONSize)
    }
    
    decoder := NewSafeJSONDecoder(bytes.NewReader(data))
    return decoder.Decode(v)
}

// DetectJSONBomb checks for obvious JSON bomb patterns
func DetectJSONBomb(data []byte) error {
    dataStr := string(data)
    
    // Check for excessive repetition (compression bomb indicator)
    if detectRepetition(dataStr) {
        return fmt.Errorf("possible JSON bomb: excessive repetition detected")
    }
    
    // Check for exponential expansion patterns
    if strings.Count(dataStr, "[") > 100 || strings.Count(dataStr, "{") > 100 {
        return fmt.Errorf("possible JSON bomb: excessive nesting detected")
    }
    
    return nil
}

func detectRepetition(s string) bool {
    // Simple repetition detection
    // In production, use more sophisticated algorithm
    for length := 10; length < 100 && length < len(s)/10; length++ {
        pattern := s[:length]
        count := strings.Count(s, pattern)
        if count > len(s)/length/2 {
            return true
        }
    }
    return false
}
```

#### Update All JSON Parsing
Replace all `json.Unmarshal` calls with safe version:

```go
// cmd/inbox/main.go
func handlePostInbox(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Existing size limit check
    body, err := common.ReadRequestBody(strings.NewReader(request.Body), common.MaxRequestSize)
    if err != nil {
        return errorResponse(413, "Request too large"), nil
    }
    
    // Check for JSON bombs
    if err := common.DetectJSONBomb(body); err != nil {
        logger.Warn("Possible JSON bomb detected", 
            zap.String("error", err.Error()),
            zap.Int("size", len(body)))
        return errorResponse(400, "Invalid JSON"), nil
    }
    
    // Safe JSON parsing
    var activity activitypub.Activity
    if err := common.SafeUnmarshalJSON(body, &activity); err != nil {
        return errorResponse(400, fmt.Sprintf("Invalid activity: %v", err)), nil
    }
    
    // ... rest of handler
}
```

### 2. Secure ID Generation for Remaining Services (LSS-022, LSS-033) - Low Priority

#### Update Inbox and Outbox ID Generation
**Update**: `cmd/inbox/main.go` and `cmd/outbox/main.go`

```go
// cmd/inbox/main.go
import (
    "github.com/lesser/pkg/common"
)

func generateActivityID(actorID string) (string, error) {
    // Use the secure ID generator from Week 1
    secureID, err := common.GenerateSecureID()
    if err != nil {
        return "", fmt.Errorf("failed to generate activity ID: %w", err)
    }
    
    return fmt.Sprintf("%s/activities/%s", actorID, secureID), nil
}

// Replace any remaining uses of math/rand
// Before:
// activityID := fmt.Sprintf("%s/activities/%d", actor.ID, rand.Int63())

// After:
activityID, err := generateActivityID(actor.ID)
if err != nil {
    return errorResponse(500, "Failed to generate ID"), nil
}
```

### 3. HTTP Signature Validation Improvements (LSS-012) - Low Priority

#### Enhanced Signature Verification
**Update**: `pkg/federation/httpsig.go`

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
)

// VerifyHTTPSignature verifies the HTTP signature on a request
func VerifyHTTPSignature(r *http.Request, publicKey *rsa.PublicKey) error {
    signature := r.Header.Get("Signature")
    if signature == "" {
        return fmt.Errorf("missing signature header")
    }
    
    // Parse signature header
    sigParams := parseSignatureHeader(signature)
    
    // Verify required fields
    if sigParams["keyId"] == "" || sigParams["signature"] == "" {
        return fmt.Errorf("missing required signature parameters")
    }
    
    // Verify algorithm
    algorithm := sigParams["algorithm"]
    if algorithm == "" {
        algorithm = "rsa-sha256" // default
    }
    
    if algorithm != "rsa-sha256" && algorithm != "hs2019" {
        return fmt.Errorf("unsupported algorithm: %s", algorithm)
    }
    
    // Check signature age (prevent replay attacks)
    if dateStr := r.Header.Get("Date"); dateStr != "" {
        date, err := http.ParseTime(dateStr)
        if err == nil {
            age := time.Since(date)
            if age > 5*time.Minute || age < -5*time.Minute {
                return fmt.Errorf("signature too old or clock skew too large: %v", age)
            }
        }
    }
    
    // Build the signing string
    headers := strings.Split(sigParams["headers"], " ")
    if len(headers) == 0 {
        headers = []string{"(request-target)", "host", "date"}
    }
    
    signingString := buildSigningString(r, headers)
    
    // Verify the signature
    signatureBytes, err := base64.StdEncoding.DecodeString(sigParams["signature"])
    if err != nil {
        return fmt.Errorf("invalid signature encoding: %w", err)
    }
    
    hash := sha256.Sum256([]byte(signingString))
    
    err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signatureBytes)
    if err != nil {
        return fmt.Errorf("signature verification failed: %w", err)
    }
    
    return nil
}

// buildSigningString constructs the string to be signed
func buildSigningString(r *http.Request, headers []string) string {
    var parts []string
    
    for _, header := range headers {
        var value string
        
        switch header {
        case "(request-target)":
            value = fmt.Sprintf("%s %s", strings.ToLower(r.Method), r.URL.RequestURI())
        case "host":
            value = r.Host
        case "date":
            value = r.Header.Get("Date")
        case "digest":
            value = r.Header.Get("Digest")
        case "content-length":
            value = r.Header.Get("Content-Length")
        default:
            value = r.Header.Get(header)
        }
        
        parts = append(parts, fmt.Sprintf("%s: %s", header, value))
    }
    
    return strings.Join(parts, "\n")
}

// VerifyDigest verifies the Digest header matches the body
func VerifyDigest(r *http.Request, body []byte) error {
    digestHeader := r.Header.Get("Digest")
    if digestHeader == "" {
        return nil // Digest is optional
    }
    
    // Parse digest header (e.g., "SHA-256=base64...")
    parts := strings.SplitN(digestHeader, "=", 2)
    if len(parts) != 2 {
        return fmt.Errorf("invalid digest format")
    }
    
    algorithm := strings.ToUpper(parts[0])
    expectedDigest := parts[1]
    
    var actualDigest string
    switch algorithm {
    case "SHA-256":
        hash := sha256.Sum256(body)
        actualDigest = base64.StdEncoding.EncodeToString(hash[:])
    default:
        return fmt.Errorf("unsupported digest algorithm: %s", algorithm)
    }
    
    if actualDigest != expectedDigest {
        return fmt.Errorf("digest mismatch")
    }
    
    return nil
}
```

### 4. Resource Limits (LSS-023) - Low Priority

#### Memory and CPU Limits
**Create**: `pkg/common/resources.go`

```go
package common

import (
    "context"
    "runtime"
    "time"
)

// ResourceLimiter prevents resource exhaustion
type ResourceLimiter struct {
    maxGoroutines int
    maxMemoryMB   int
    semaphore     chan struct{}
}

func NewResourceLimiter(maxGoroutines, maxMemoryMB int) *ResourceLimiter {
    return &ResourceLimiter{
        maxGoroutines: maxGoroutines,
        maxMemoryMB:   maxMemoryMB,
        semaphore:     make(chan struct{}, maxGoroutines),
    }
}

// RunWithLimit executes a function with resource limits
func (rl *ResourceLimiter) RunWithLimit(ctx context.Context, f func() error) error {
    // Check memory before starting
    if err := rl.checkMemory(); err != nil {
        return err
    }
    
    // Acquire semaphore slot
    select {
    case rl.semaphore <- struct{}{}:
        defer func() { <-rl.semaphore }()
    case <-ctx.Done():
        return ctx.Err()
    default:
        return fmt.Errorf("too many concurrent operations")
    }
    
    // Run with timeout
    done := make(chan error, 1)
    go func() {
        done <- f()
    }()
    
    select {
    case err := <-done:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (rl *ResourceLimiter) checkMemory() error {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    usedMB := int(m.Alloc / 1024 / 1024)
    if usedMB > rl.maxMemoryMB {
        // Force GC and check again
        runtime.GC()
        runtime.ReadMemStats(&m)
        usedMB = int(m.Alloc / 1024 / 1024)
        
        if usedMB > rl.maxMemoryMB {
            return fmt.Errorf("memory limit exceeded: %dMB > %dMB", usedMB, rl.maxMemoryMB)
        }
    }
    
    return nil
}

// Example usage in media processing
var mediaLimiter = NewResourceLimiter(10, 256) // 10 concurrent, 256MB

func processMediaJob(job MediaJob) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    return mediaLimiter.RunWithLimit(ctx, func() error {
        // Media processing logic here
        return processImage(job)
    })
}
```

## Success Criteria

### JSON Parsing Limits Complete When:
- [ ] All JSON parsing uses safe decoder
- [ ] Depth limits enforced
- [ ] Size limits enforced
- [ ] JSON bomb detection implemented
- [ ] Tests verify attack prevention

### Secure IDs Complete When:
- [ ] Inbox uses crypto/rand IDs
- [ ] Outbox uses crypto/rand IDs
- [ ] No math/rand usage remains
- [ ] ID generation centralized

### HTTP Signatures Complete When:
- [ ] Multiple algorithms supported
- [ ] Signature age validated
- [ ] Digest verification implemented
- [ ] Clock skew handled

### Resource Limits Complete When:
- [ ] Goroutine limits enforced
- [ ] Memory monitoring active
- [ ] Timeouts on all operations
- [ ] Graceful degradation

## Testing Requirements

### JSON Bomb Tests
```go
func TestJSONBombPrevention(t *testing.T) {
    tests := []struct {
        name string
        json string
        shouldFail bool
    }{
        {
            name: "deep nesting",
            json: strings.Repeat(`{"a":`, 20) + `null` + strings.Repeat(`}`, 20),
            shouldFail: true,
        },
        {
            name: "large array",
            json: `{"items":[` + strings.Repeat(`"x",`, 2000) + `"x"]}`,
            shouldFail: true,
        },
        {
            name: "compression bomb",
            json: `{"` + strings.Repeat("a", 1000) + `":"` + strings.Repeat("b", 1000) + `"}`,
            shouldFail: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var result any
            err := SafeUnmarshalJSON([]byte(tt.json), &result)
            
            if tt.shouldFail {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

## Important Notes

1. **Performance**: JSON validation adds overhead - benchmark critical paths
2. **Compatibility**: Some valid ActivityPub objects might exceed limits
3. **Monitoring**: Track rejected requests to tune limits
4. **Lambda Limits**: AWS Lambda has built-in memory/time limits too

## Resources

- [JSON Bomb Attack](https://www.acunetix.com/blog/web-security-zone/what-is-json-bomb/)
- [HTTP Signatures Spec](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures)
- [Go Memory Management](https://go.dev/doc/gc-guide)

Remember: These are the final security hardening steps before production readiness! 