# AI Assistant Prompt: Security Team 2 - Input Validation & Data Protection

## Your Role
You are a senior security engineer on Team 2, responsible for implementing input validation, data protection, and federation security improvements for Lesser. You will work in parallel with Team 1, who is handling authentication infrastructure.

## Context
Lesser is currently in prototype phase with 33 identified security vulnerabilities. Your team focuses on preventing injection attacks, protecting user data, and securing federation mechanisms. You can assume Team 1's authentication middleware will provide user context when needed.

## Your Primary Objectives

### 1. Fix Critical XSS Vulnerability (Critical Priority)

#### HTML Sanitization (LSS-001)
**File**: `pkg/activitypub/validation.go`

The current HTML sanitizer is dangerously inadequate. Replace it completely:

```go
package activitypub

import (
    "github.com/microcosm-cc/bluemonday"
)

// Create a strict sanitizer for user-generated content
var strictSanitizer = bluemonday.UGCPolicy()

func init() {
    // Customize the policy if needed
    strictSanitizer.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("span")
    strictSanitizer.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
}

func SanitizeHTML(input string) string {
    // This replaces the dangerous string replacement approach
    return strictSanitizer.Sanitize(input)
}

// Add a separate sanitizer for more trusted content if needed
var relaxedSanitizer = bluemonday.UGCPolicy()

func SanitizeHTMLRelaxed(input string) string {
    return relaxedSanitizer.Sanitize(input)
}
```

Add comprehensive XSS tests:
```go
func TestSanitizeHTML(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "script tag",
            input:    `<script>alert('xss')</script>`,
            expected: ``,
        },
        {
            name:     "javascript protocol",
            input:    `<a href="javascript:alert('xss')">link</a>`,
            expected: `<a>link</a>`,
        },
        {
            name:     "event handler",
            input:    `<img src="x" onerror="alert('xss')">`,
            expected: `<img src="x">`,
        },
        {
            name:     "data uri script",
            input:    `<img src="data:text/html,<script>alert('xss')</script>">`,
            expected: `<img>`,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := SanitizeHTML(tt.input)
            if got != tt.expected {
                t.Errorf("SanitizeHTML() = %v, want %v", got, tt.expected)
            }
        })
    }
}
```

### 2. Prevent Data Leakage to Blocked Users (Critical Priority)

#### Block List Filtering (LSS-030)
**File**: `cmd/outbox/main.go`

Activities are currently delivered to blocked users. Fix the `deliverActivityRemotely` function:

```go
func deliverActivityRemotely(activity activitypub.Activity, actor activitypub.Actor) error {
    // Get the actor's block list
    blockedActors, err := store.GetBlockedActors(actor.ID)
    if err != nil {
        logger.Error("Failed to get blocked actors", zap.Error(err))
        // Fail closed - don't deliver if we can't check blocks
        return fmt.Errorf("failed to check block list: %w", err)
    }
    
    // Create a map for efficient lookup
    blockedMap := make(map[string]bool)
    for _, blocked := range blockedActors {
        blockedMap[blocked.ID] = true
        blockedMap[blocked.Inbox] = true
    }
    
    // Filter followers
    followers, err := store.GetFollowers(actor.ID)
    if err != nil {
        return fmt.Errorf("failed to get followers: %w", err)
    }
    
    filteredFollowers := []activitypub.Actor{}
    for _, follower := range followers {
        if !blockedMap[follower.ID] && !blockedMap[follower.Inbox] {
            filteredFollowers = append(filteredFollowers, follower)
        } else {
            logger.Info("Skipping delivery to blocked user",
                zap.String("blocked_actor", follower.ID),
                zap.String("activity", activity.ID))
        }
    }
    
    // Filter direct recipients
    recipients := extractRecipients(activity)
    filteredRecipients := []string{}
    for _, recipient := range recipients {
        if !blockedMap[recipient] {
            filteredRecipients = append(filteredRecipients, recipient)
        }
    }
    
    // Deliver only to non-blocked recipients
    return deliveryService.DeliverActivity(activity, filteredFollowers, filteredRecipients)
}
```

### 3. Implement Request Size Limits (High Priority)

#### Prevent DoS via Large Requests (LSS-021, LSS-029, LSS-032)

Create a utility function for safe body reading:
```go
// pkg/common/request.go
package common

import (
    "fmt"
    "io"
)

const (
    MaxRequestSize = 1 * 1024 * 1024 // 1MB default
    MaxMediaSize   = 50 * 1024 * 1024 // 50MB for media
)

func ReadRequestBody(body io.Reader, maxSize int64) ([]byte, error) {
    if maxSize <= 0 {
        maxSize = MaxRequestSize
    }
    
    limited := io.LimitReader(body, maxSize+1) // +1 to detect oversized
    data, err := io.ReadAll(limited)
    if err != nil {
        return nil, fmt.Errorf("failed to read body: %w", err)
    }
    
    if int64(len(data)) > maxSize {
        return nil, fmt.Errorf("request body too large: %d > %d", len(data), maxSize)
    }
    
    return data, nil
}
```

Update all handlers:
```go
// cmd/inbox/main.go
func handlePostInbox(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Replace: body := []byte(request.Body)
    body, err := common.ReadRequestBody(strings.NewReader(request.Body), common.MaxRequestSize)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 413, // Payload Too Large
            Body:       fmt.Sprintf(`{"error": "%s"}`, err.Error()),
        }, nil
    }
    // ... rest of handler
}
```

### 4. File Type Validation (High Priority)

#### Prevent Malicious File Processing (LSS-027)
**File**: `cmd/media-processor/main.go`

Never trust user-provided MIME types:

```go
import (
    "net/http"
)

var allowedMimeTypes = map[string]bool{
    "image/jpeg": true,
    "image/png":  true,
    "image/gif":  true,
    "image/webp": true,
    "video/mp4":  true,
    "video/webm": true,
}

func validateFileType(data []byte, claimedMimeType string) error {
    // Detect actual MIME type from file content
    detectedType := http.DetectContentType(data)
    
    // Check if detected type is allowed
    if !allowedMimeTypes[detectedType] {
        return fmt.Errorf("file type not allowed: %s", detectedType)
    }
    
    // Warn if claimed type doesn't match
    if claimedMimeType != "" && claimedMimeType != detectedType {
        logger.Warn("MIME type mismatch",
            zap.String("claimed", claimedMimeType),
            zap.String("detected", detectedType))
    }
    
    return nil
}

func processMediaJob(job MediaJob) error {
    // Download file with size limit
    data, err := downloadFromS3WithLimit(job.S3Key, common.MaxMediaSize)
    if err != nil {
        return fmt.Errorf("failed to download: %w", err)
    }
    
    // Validate file type before processing
    if err := validateFileType(data, job.MimeType); err != nil {
        logger.Error("Invalid file type",
            zap.String("job_id", job.ID),
            zap.Error(err))
        return err
    }
    
    // Safe to process
    switch detectedType := http.DetectContentType(data); detectedType {
    case "image/jpeg", "image/png", "image/gif", "image/webp":
        return processImage(data, job)
    case "video/mp4", "video/webm":
        return processVideo(data, job)
    default:
        return fmt.Errorf("unsupported type: %s", detectedType)
    }
}
```

### 5. Path Traversal Prevention (Medium Priority)

#### Sanitize S3 Keys (LSS-028)
**File**: `cmd/media-processor/main.go`

```go
import (
    "path"
    "regexp"
)

var safeUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func sanitizeUsername(username string) (string, error) {
    if username == "" {
        return "", fmt.Errorf("username cannot be empty")
    }
    
    // Check length
    if len(username) > 255 {
        return "", fmt.Errorf("username too long")
    }
    
    // Check against safe pattern
    if !safeUsernameRegex.MatchString(username) {
        return "", fmt.Errorf("username contains invalid characters")
    }
    
    // Additional safety: ensure no path traversal
    cleaned := path.Clean(username)
    if cleaned != username || cleaned == "." || cleaned == ".." {
        return "", fmt.Errorf("username contains path traversal")
    }
    
    return username, nil
}

func generateS3Key(username, mediaID, variant string) (string, error) {
    safeUsername, err := sanitizeUsername(username)
    if err != nil {
        return "", fmt.Errorf("invalid username: %w", err)
    }
    
    // Use path.Join for safe path construction
    key := path.Join("media", safeUsername, mediaID, variant)
    return key, nil
}
```

### 6. Secure Random ID Generation (Low Priority)

#### Replace Insecure Random (LSS-009, LSS-022, LSS-033)

Create a centralized secure ID generator:
```go
// pkg/common/id.go
package common

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
)

func GenerateSecureID() (string, error) {
    bytes := make([]byte, 16) // 128 bits
    _, err := rand.Read(bytes)
    if err != nil {
        return "", fmt.Errorf("failed to generate random ID: %w", err)
    }
    return hex.EncodeToString(bytes), nil
}

func GenerateActivityID(actorID string) (string, error) {
    id, err := GenerateSecureID()
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s/activities/%s", actorID, id), nil
}
```

Replace all instances of:
```go
// Replace this pattern:
rand.Seed(time.Now().UnixNano())
id := generateRandomString()

// With:
id, err := common.GenerateSecureID()
if err != nil {
    return err
}
```

## Success Criteria

### Critical Issues Fixed When:
- [ ] XSS test suite passes with bluemonday
- [ ] Blocked users never receive activities
- [ ] All request handlers enforce size limits
- [ ] File type validation prevents malicious uploads

### Additional Security When:
- [ ] Path traversal is impossible in S3 keys
- [ ] All IDs use cryptographically secure random
- [ ] Error messages don't leak sensitive info
- [ ] Security events are properly logged

## Testing Your Implementation

### XSS Testing
```go
// Run comprehensive XSS tests
go test ./pkg/activitypub -run TestSanitizeHTML -v
```

### Block List Testing
```go
// Test that blocked users don't receive activities
func TestBlockedUserDelivery(t *testing.T) {
    // Create test users
    alice := createTestActor("alice")
    bob := createTestActor("bob")
    
    // Alice blocks Bob
    store.BlockActor(alice.ID, bob.ID)
    
    // Alice creates a public post
    activity := createTestActivity(alice, "public")
    
    // Verify Bob doesn't receive it
    deliveries := captureDeliveries(activity)
    for _, delivery := range deliveries {
        if delivery.Recipient == bob.Inbox {
            t.Errorf("Activity delivered to blocked user")
        }
    }
}
```

### Size Limit Testing
```bash
# Generate large payload
dd if=/dev/zero bs=1M count=2 | base64 > large.json

# Should be rejected
curl -X POST http://localhost:3000/inbox \
  -H "Content-Type: application/json" \
  -d @large.json
```

## Important Security Principles

1. **Never Trust User Input**: Always validate, sanitize, and verify
2. **Fail Securely**: When in doubt, reject the request
3. **Defense in Depth**: Multiple layers of validation
4. **Log Security Events**: But don't log sensitive data

## Coordination with Team 1

Team 1 is implementing:
- Central authentication (you'll use their user context)
- Secure HTTP client (use it for any outbound requests)
- Core auth infrastructure

You may need to wait for their auth middleware before testing some authorization features.

## Resources

- [OWASP XSS Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)
- [bluemonday Documentation](https://github.com/microcosm-cc/bluemonday)
- [OWASP Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)

Remember: Input validation is the last line of defense. Be thorough, be paranoid, and test everything. 