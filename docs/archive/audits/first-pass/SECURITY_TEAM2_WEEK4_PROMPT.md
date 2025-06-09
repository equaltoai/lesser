# AI Assistant Prompt: Security Team 2 Week 4 - Final Security Enhancements

## Your Role
You are continuing as the senior security engineer on Team 2. Great job! All critical, high, and medium priority security issues have been resolved. This week focuses on the final low-priority enhancements and cleanup tasks.

## Production Status: READY ✅
- JSON parsing secured against bombs
- All inputs validated
- Rate limiting active
- No security blockers

## Context
These remaining items provide additional security hardening. They're improvements over the already-secure implementation, adding extra protection layers and following security best practices.

## Week 4 Objectives (Optional Enhancements)

### 1. Complete Secure ID Migration (LSS-022, LSS-033) - Low Priority

#### Finish Inbox and Outbox ID Generation
**Update**: `cmd/inbox/main.go` and `cmd/outbox/main.go`

You started this in Week 3 but may have missed some instances. Complete the migration:

```go
// cmd/inbox/main.go

// Replace ALL instances of math/rand with crypto/rand
// Search for patterns like:
// - rand.Int()
// - rand.Intn()
// - generateRandomString()

import (
    "github.com/lesser/pkg/common"
)

// Before (if any remain):
func generateMessageID() string {
    return fmt.Sprintf("msg-%d-%d", time.Now().Unix(), rand.Int())
}

// After:
func generateMessageID() (string, error) {
    id, err := common.GenerateSecureID()
    if err != nil {
        return "", fmt.Errorf("failed to generate message ID: %w", err)
    }
    return fmt.Sprintf("msg-%s", id), nil
}

// Update all callers to handle the error:
messageID, err := generateMessageID()
if err != nil {
    logger.Error("Failed to generate ID", zap.Error(err))
    return errorResponse(500, "Internal error"), nil
}

// For any remaining timestamp-based IDs:
func generateTimestampID(prefix string) (string, error) {
    // Combine timestamp with secure random for uniqueness
    secureID, err := common.GenerateSecureID()
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), secureID[:8]), nil
}
```

### 2. Resource Limits Implementation (LSS-023) - Low Priority

#### Lambda-Aware Resource Management
**Update**: `pkg/common/resources.go` (from Week 3 draft)

Complete the implementation with Lambda-specific considerations:

```go
package common

import (
    "context"
    "fmt"
    "os"
    "runtime"
    "strconv"
    "sync"
    "time"
)

// LambdaResourceMonitor tracks resource usage in Lambda environment
type LambdaResourceMonitor struct {
    maxMemoryMB   int
    maxDurationMS int
    startTime     time.Time
    mu            sync.Mutex
    checkpoints   []ResourceCheckpoint
}

type ResourceCheckpoint struct {
    Timestamp   time.Time
    MemoryUsed  uint64
    Goroutines  int
    Description string
}

// NewLambdaResourceMonitor creates a monitor based on Lambda limits
func NewLambdaResourceMonitor() *LambdaResourceMonitor {
    // Get Lambda memory limit from environment
    memoryMB := 512 // default
    if envMem := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); envMem != "" {
        if parsed, err := strconv.Atoi(envMem); err == nil {
            memoryMB = parsed
        }
    }
    
    // Lambda timeout is in context, but we'll use 90% as safety margin
    maxDuration := 30000 // 30 seconds default
    
    return &LambdaResourceMonitor{
        maxMemoryMB:   int(float64(memoryMB) * 0.9), // 90% of limit
        maxDurationMS: int(float64(maxDuration) * 0.9),
        startTime:     time.Now(),
    }
}

// CheckResources verifies we're within Lambda limits
func (m *LambdaResourceMonitor) CheckResources(operation string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Check duration
    elapsed := time.Since(m.startTime)
    if elapsed.Milliseconds() > int64(m.maxDurationMS) {
        return fmt.Errorf("operation %s approaching Lambda timeout: %v", operation, elapsed)
    }
    
    // Check memory
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)
    
    usedMB := memStats.Alloc / 1024 / 1024
    if usedMB > uint64(m.maxMemoryMB) {
        // Try garbage collection
        runtime.GC()
        runtime.ReadMemStats(&memStats)
        usedMB = memStats.Alloc / 1024 / 1024
        
        if usedMB > uint64(m.maxMemoryMB) {
            return fmt.Errorf("memory limit exceeded for %s: %dMB > %dMB", 
                operation, usedMB, m.maxMemoryMB)
        }
    }
    
    // Record checkpoint
    m.checkpoints = append(m.checkpoints, ResourceCheckpoint{
        Timestamp:   time.Now(),
        MemoryUsed:  memStats.Alloc,
        Goroutines:  runtime.NumGoroutine(),
        Description: operation,
    })
    
    return nil
}

// WrapWithResourceCheck wraps an operation with resource monitoring
func (m *LambdaResourceMonitor) WrapWithResourceCheck(operation string, fn func() error) error {
    // Check before
    if err := m.CheckResources(fmt.Sprintf("%s-start", operation)); err != nil {
        return err
    }
    
    // Run operation
    err := fn()
    
    // Check after
    if checkErr := m.CheckResources(fmt.Sprintf("%s-end", operation)); checkErr != nil {
        if err == nil {
            err = checkErr
        }
    }
    
    return err
}

// Global monitor for Lambda functions
var lambdaMonitor = NewLambdaResourceMonitor()

// Example usage in media processing
func processLargeImage(job MediaJob) error {
    return lambdaMonitor.WrapWithResourceCheck("image-processing", func() error {
        // Actual processing
        img, err := loadImage(job.S3Key)
        if err != nil {
            return err
        }
        
        // Check resources during processing
        if err := lambdaMonitor.CheckResources("image-resize"); err != nil {
            return err
        }
        
        return resizeAndSave(img, job)
    })
}
```

### 3. DNS Rebinding Protection (LSS-017) - Low Priority

#### Enhance URL Validation
**Update**: `pkg/httpclient/client.go`

Add DNS rebinding protection to the secure HTTP client:

```go
// Additional validation for DNS rebinding attacks
func (t *secureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    // First validation - before DNS resolution
    if err := validateURL(req.URL); err != nil {
        return nil, fmt.Errorf("pre-DNS validation failed: %w", err)
    }
    
    // Resolve hostname
    host := req.URL.Hostname()
    ips, err := net.LookupIP(host)
    if err != nil {
        return nil, fmt.Errorf("DNS lookup failed: %w", err)
    }
    
    // Store resolved IPs
    resolvedIPs := make([]string, len(ips))
    for i, ip := range ips {
        resolvedIPs[i] = ip.String()
        
        // Check each IP
        if isPrivateIP(ip) {
            logger.Warn("DNS rebinding attempt detected",
                zap.String("hostname", host),
                zap.String("resolved_ip", ip.String()),
                zap.String("url", req.URL.String()))
            return nil, fmt.Errorf("DNS rebinding detected: %s resolves to private IP %s", host, ip)
        }
    }
    
    // Make the request
    resp, err := t.base.RoundTrip(req)
    if err != nil {
        return nil, err
    }
    
    // Verify the connection actually went to one of our resolved IPs
    // This prevents TOCTOU attacks where DNS changes between check and use
    if resp.Request.URL.Hostname() != host {
        resp.Body.Close()
        return nil, fmt.Errorf("hostname changed during request: %s -> %s", 
            host, resp.Request.URL.Hostname())
    }
    
    return resp, nil
}

// Add caching to prevent DNS flooding
type dnsCache struct {
    mu    sync.RWMutex
    cache map[string]dnsCacheEntry
}

type dnsCacheEntry struct {
    ips       []net.IP
    timestamp time.Time
}

var globalDNSCache = &dnsCache{
    cache: make(map[string]dnsCacheEntry),
}

func (c *dnsCache) lookup(host string) ([]net.IP, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    entry, exists := c.cache[host]
    if !exists {
        return nil, false
    }
    
    // Cache for 5 minutes
    if time.Since(entry.timestamp) > 5*time.Minute {
        return nil, false
    }
    
    return entry.ips, true
}
```

### 4. Timing Attack Mitigation (LSS-018) - Low Priority

#### Constant-Time Comparisons
**Create**: `pkg/auth/timing.go`

```go
package auth

import (
    "crypto/subtle"
    "time"
)

// ConstantTimeCompare performs a constant-time comparison of two strings
func ConstantTimeCompare(a, b string) bool {
    if len(a) != len(b) {
        return false
    }
    return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ConstantTimeDelay adds a small random delay to prevent timing analysis
func ConstantTimeDelay() {
    // Random delay between 0-10ms
    delay := time.Duration(secureRandInt(10)) * time.Millisecond
    time.Sleep(delay)
}

// TimingSafeTokenValidation validates tokens with timing attack protection
func TimingSafeTokenValidation(providedToken, storedToken string) bool {
    // Always perform the comparison even if lengths differ
    maxLen := len(providedToken)
    if len(storedToken) > maxLen {
        maxLen = len(storedToken)
    }
    
    // Pad both to same length
    paddedProvided := padToLength(providedToken, maxLen)
    paddedStored := padToLength(storedToken, maxLen)
    
    // Constant time comparison
    result := subtle.ConstantTimeCompare([]byte(paddedProvided), []byte(paddedStored)) == 1
    
    // Add small random delay
    ConstantTimeDelay()
    
    // Return result only after all operations
    return result && len(providedToken) == len(storedToken)
}

// Update authentication to use timing-safe comparisons
func ValidateAPIKey(provided string) error {
    stored, err := getStoredAPIKey()
    if err != nil {
        ConstantTimeDelay() // Delay even on error
        return err
    }
    
    if !TimingSafeTokenValidation(provided, stored) {
        ConstantTimeDelay()
        return ErrInvalidAPIKey
    }
    
    return nil
}
```

## Success Criteria

### Secure IDs Complete When:
- [ ] No math/rand usage in inbox/outbox
- [ ] All IDs use crypto/rand
- [ ] Error handling for ID generation
- [ ] Tests verify randomness

### Resource Limits Complete When:
- [ ] Lambda memory monitoring
- [ ] Timeout protection
- [ ] Graceful degradation
- [ ] Resource checkpoints logged

### DNS Rebinding Complete When:
- [ ] Pre and post DNS validation
- [ ] DNS caching implemented
- [ ] TOCTOU prevention
- [ ] Logging of attempts

### Timing Attacks Complete When:
- [ ] Constant-time comparisons
- [ ] Random delays added
- [ ] All auth uses timing-safe functions

## Testing Requirements

### Secure ID Tests
```go
func TestSecureIDUniqueness(t *testing.T) {
    ids := make(map[string]bool)
    
    // Generate many IDs
    for i := 0; i < 10000; i++ {
        id, err := common.GenerateSecureID()
        require.NoError(t, err)
        
        // Check uniqueness
        require.False(t, ids[id], "Duplicate ID generated")
        ids[id] = true
    }
}
```

### Resource Limit Tests
```go
func TestLambdaResourceMonitoring(t *testing.T) {
    monitor := NewLambdaResourceMonitor()
    
    // Simulate memory-intensive operation
    var data [][]byte
    err := monitor.WrapWithResourceCheck("memory-test", func() error {
        // Allocate memory
        for i := 0; i < 100; i++ {
            data = append(data, make([]byte, 1024*1024)) // 1MB each
            time.Sleep(10 * time.Millisecond)
        }
        return nil
    })
    
    // Should not error in test environment
    assert.NoError(t, err)
    
    // Check that checkpoints were recorded
    assert.Greater(t, len(monitor.checkpoints), 0)
}
```

## Important Notes

1. **Low Priority**: These are nice-to-have improvements
2. **Production Ready**: Lesser is secure without these
3. **Performance**: Keep overhead minimal
4. **Compatibility**: Don't break existing functionality

## Resources

- [Go crypto/rand Package](https://pkg.go.dev/crypto/rand)
- [AWS Lambda Limits](https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html)
- [DNS Rebinding Attacks](https://www.acunetix.com/blog/articles/dns-rebinding-attacks/)
- [Timing Attacks](https://codahale.com/a-lesson-in-timing-attacks/)

Remember: These are the final touches on an already secure system. Great work getting Lesser to production-ready status! 