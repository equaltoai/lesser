# API Security Quick Start for Lesser Infrastructure

## Overview

As headless ActivityPub infrastructure, Lesser's security focus should be on:
- Protecting API endpoints
- Securing federation channels
- Providing privacy primitives
- Enabling compliance

This guide provides practical first steps that align with Lesser's serverless architecture.

## Priority 1: API Request Security

### 1.1 HMAC Request Signing

Every API request should be signed to prevent tampering and replay attacks.

```go
// pkg/api/security/hmac.go
package security

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "net/http"
    "strings"
    "time"
)

type HMACValidator struct {
    secrets map[string]string // appID -> secret
}

func (h *HMACValidator) ValidateRequest(r *http.Request) error {
    // Extract auth header
    // Format: HMAC-SHA256 AppId=xxx,Timestamp=xxx,Signature=xxx
    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "HMAC-SHA256 ") {
        return fmt.Errorf("missing HMAC authorization")
    }
    
    parts := parseAuthHeader(auth)
    appID := parts["AppId"]
    timestamp := parts["Timestamp"]
    signature := parts["Signature"]
    
    // Verify timestamp (5 minute window)
    ts, _ := time.Parse(time.RFC3339, timestamp)
    if time.Since(ts) > 5*time.Minute {
        return fmt.Errorf("request expired")
    }
    
    // Recreate signature
    secret, ok := h.secrets[appID]
    if !ok {
        return fmt.Errorf("unknown app")
    }
    
    canonical := fmt.Sprintf("%s\n%s\n%s\n%s",
        r.Method,
        r.URL.Path,
        timestamp,
        r.Header.Get("Content-Hash"),
    )
    
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(canonical))
    expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
    
    if signature != expected {
        return fmt.Errorf("invalid signature")
    }
    
    return nil
}
```

### 1.2 Rate Limiting in DynamoDB

Use DynamoDB to track API usage across Lambda invocations.

```go
// pkg/api/security/rate_limiter.go
package security

type RateLimiter struct {
    table string
    db    dynamodbiface.DynamoDBAPI
}

type RateLimit struct {
    Key       string    // RATE#user#endpoint
    Window    string    // 2024-01-01T12:00
    Count     int
    ExpiresAt int64     // TTL
}

func (r *RateLimiter) CheckAndIncrement(ctx context.Context, userID, endpoint string) error {
    now := time.Now()
    window := now.Truncate(time.Minute).Format(time.RFC3339)
    key := fmt.Sprintf("RATE#%s#%s", userID, endpoint)
    
    // Atomic increment
    update := expression.Add(expression.Name("count"), expression.Value(1))
    expr, _ := expression.NewBuilder().WithUpdate(update).Build()
    
    result, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
        TableName: &r.table,
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: key},
            "SK": &types.AttributeValueMemberS{Value: window},
        },
        UpdateExpression: expr.Update(),
        ExpressionAttributeNames: expr.Names(),
        ExpressionAttributeValues: expr.Values(),
        ReturnValues: types.ReturnValueAllNew,
    })
    
    // Check limit
    var item RateLimit
    attributevalue.UnmarshalMap(result.Attributes, &item)
    
    limit := r.getLimit(userID, endpoint)
    if item.Count > limit {
        return ErrRateLimitExceeded
    }
    
    return nil
}
```

### 1.3 API Key Management

Store API keys securely with scopes and metadata.

```go
// pkg/storage/dynamodb/api_keys.go
type APIKey struct {
    ID          string   `dynamodbav:"id"`
    AppID       string   `dynamodbav:"app_id"`
    HashedKey   string   `dynamodbav:"hashed_key"`
    Scopes      []string `dynamodbav:"scopes"`
    CreatedAt   time.Time `dynamodbav:"created_at"`
    LastUsed    time.Time `dynamodbav:"last_used"`
    ExpiresAt   *time.Time `dynamodbav:"expires_at,omitempty"`
    
    // Security metadata
    AllowedIPs  []string `dynamodbav:"allowed_ips,omitempty"`
    RateLimit   int      `dynamodbav:"rate_limit"`
    Permissions map[string]any `dynamodbav:"permissions"`
}

// Storage pattern
// PK: APP#appID
// SK: APIKEY#keyID
```

## Priority 2: Federation Security

### 2.1 Instance Trust Scoring

Track trust metrics for federated instances.

```go
// pkg/federation/trust/scorer.go
type InstanceTrustScorer struct {
    storage DynamoDBStorage
}

type InstanceTrust struct {
    Domain        string    `dynamodbav:"domain"`
    TrustScore    float64   `dynamodbav:"trust_score"`
    LastUpdated   time.Time `dynamodbav:"last_updated"`
    
    // Metrics
    TotalActivities   int `dynamodbav:"total_activities"`
    ValidSignatures   int `dynamodbav:"valid_signatures"`
    FailedSignatures  int `dynamodbav:"failed_signatures"`
    SpamReports       int `dynamodbav:"spam_reports"`
    UserReports       int `dynamodbav:"user_reports"`
    
    // Decisions
    AutoApprove      bool `dynamodbav:"auto_approve"`
    RequireModeration bool `dynamodbav:"require_moderation"`
    Blocked          bool `dynamodbav:"blocked"`
}

func (i *InstanceTrustScorer) UpdateTrust(ctx context.Context, domain string, event TrustEvent) error {
    // Update trust based on event
    trust, _ := i.storage.GetInstanceTrust(ctx, domain)
    
    switch event.Type {
    case "valid_signature":
        trust.ValidSignatures++
        trust.TrustScore += 0.01
    case "failed_signature":
        trust.FailedSignatures++
        trust.TrustScore -= 0.05
    case "spam_report":
        trust.SpamReports++
        trust.TrustScore -= 0.1
    }
    
    // Normalize score between 0 and 1
    trust.TrustScore = math.Max(0, math.Min(1, trust.TrustScore))
    
    // Update auto-moderation rules
    if trust.TrustScore > 0.8 {
        trust.AutoApprove = true
        trust.RequireModeration = false
    } else if trust.TrustScore < 0.3 {
        trust.AutoApprove = false
        trust.RequireModeration = true
    }
    
    return i.storage.UpdateInstanceTrust(ctx, trust)
}
```

### 2.2 Activity Validation

Enhanced validation for incoming ActivityPub objects.

```go
// pkg/federation/validator/activity.go
type ActivityValidator struct {
    verifier SignatureVerifier
    trust    InstanceTrustScorer
}

func (a *ActivityValidator) ValidateIncoming(ctx context.Context, r *http.Request) (*Activity, error) {
    // 1. Parse activity
    activity, err := ParseActivity(r.Body)
    if err != nil {
        return nil, fmt.Errorf("parse activity: %w", err)
    }
    
    // 2. Verify HTTP signature
    actor, err := a.verifier.VerifyHTTPSignature(r)
    if err != nil {
        // Update trust score
        domain := extractDomain(activity.Actor)
        a.trust.UpdateTrust(ctx, domain, TrustEvent{
            Type: "failed_signature",
        })
        return nil, fmt.Errorf("signature verification: %w", err)
    }
    
    // 3. Validate activity structure
    if err := a.validateStructure(activity); err != nil {
        return nil, fmt.Errorf("invalid structure: %w", err)
    }
    
    // 4. Check instance trust
    domain := extractDomain(actor)
    trust, _ := a.trust.GetInstanceTrust(ctx, domain)
    
    if trust.Blocked {
        return nil, fmt.Errorf("instance blocked")
    }
    
    if trust.RequireModeration {
        activity.RequiresModeration = true
    }
    
    // 5. Update trust (successful validation)
    a.trust.UpdateTrust(ctx, domain, TrustEvent{
        Type: "valid_signature",
    })
    
    return activity, nil
}
```

## Priority 3: Privacy APIs

### 3.1 Visibility Enforcement

Centralized visibility checking for all content access.

```go
// pkg/privacy/visibility.go
type VisibilityEnforcer struct {
    storage DynamoDBStorage
}

func (v *VisibilityEnforcer) CanView(ctx context.Context, viewer *Actor, content Content) (bool, error) {
    // Public content
    if content.GetVisibility() == "public" {
        return true, nil
    }
    
    // Unlisted - need direct link
    if content.GetVisibility() == "unlisted" {
        // Check if viewer came via direct link (in context)
        return ctx.Value("direct_access") == true, nil
    }
    
    // Followers only
    if content.GetVisibility() == "followers" {
        if viewer == nil {
            return false, nil
        }
        return v.storage.IsFollower(ctx, viewer.ID, content.GetAuthorID())
    }
    
    // Direct/Mentioned only
    if content.GetVisibility() == "direct" {
        if viewer == nil {
            return false, nil
        }
        return content.IsMentioned(viewer.ID), nil
    }
    
    return false, nil
}

// Middleware to enforce visibility
func (v *VisibilityEnforcer) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract content ID from request
        contentID := chi.URLParam(r, "id")
        
        // Get content
        content, err := v.storage.GetContent(r.Context(), contentID)
        if err != nil {
            http.Error(w, "Not found", 404)
            return
        }
        
        // Check visibility
        viewer := GetActorFromContext(r.Context())
        canView, err := v.CanView(r.Context(), viewer, content)
        if err != nil || !canView {
            http.Error(w, "Not found", 404) // Don't leak existence
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### 3.2 Data Retention API

Allow users to set retention policies for their data.

```go
// pkg/privacy/retention.go
type RetentionPolicy struct {
    UserID         string `dynamodbav:"user_id"`
    
    // Retention periods
    StatusRetention   int  `dynamodbav:"status_retention_days"`   // -1 = forever
    MediaRetention    int  `dynamodbav:"media_retention_days"`
    MessageRetention  int  `dynamodbav:"message_retention_days"`
    
    // Options
    DeleteAfterDeactivation bool `dynamodbav:"delete_after_deactivation"`
    AnonymizeOldContent     bool `dynamodbav:"anonymize_old_content"`
}

// Lambda function to enforce retention
func RetentionEnforcer(ctx context.Context) error {
    // Query for content past retention
    items, err := storage.GetExpiredContent(ctx, time.Now())
    if err != nil {
        return err
    }
    
    for _, item := range items {
        policy, _ := storage.GetRetentionPolicy(ctx, item.UserID)
        
        if policy.AnonymizeOldContent {
            // Replace content with [deleted]
            item.Content = "[deleted]"
            storage.UpdateContent(ctx, item)
        } else {
            // Hard delete
            storage.DeleteContent(ctx, item.ID)
        }
    }
    
    return nil
}
```

### 3.3 Consent Management

Track and enforce user consent for data processing.

```go
// pkg/privacy/consent.go
type ConsentRecord struct {
    UserID      string    `dynamodbav:"user_id"`
    Purpose     string    `dynamodbav:"purpose"`
    Granted     bool      `dynamodbav:"granted"`
    GrantedAt   time.Time `dynamodbav:"granted_at"`
    ExpiresAt   *time.Time `dynamodbav:"expires_at,omitempty"`
    
    // Audit trail
    IP          string    `dynamodbav:"ip"`
    UserAgent   string    `dynamodbav:"user_agent"`
    Version     string    `dynamodbav:"policy_version"`
}

type ConsentManager struct {
    storage DynamoDBStorage
}

func (c *ConsentManager) RequireConsent(purpose string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := GetUserFromContext(r.Context())
            if user == nil {
                http.Error(w, "Unauthorized", 401)
                return
            }
            
            consent, _ := c.storage.GetConsent(r.Context(), user.ID, purpose)
            if consent == nil || !consent.Granted || consent.IsExpired() {
                // Return consent required error
                respondJSON(w, 403, map[string]any{
                    "error": "consent_required",
                    "purpose": purpose,
                    "policy_url": "/privacy/policy",
                })
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## Implementation Checklist

### Week 1: Core API Security
- [ ] Implement HMAC request signing
- [ ] Add rate limiting with DynamoDB
- [ ] Create API key management system
- [ ] Add security headers middleware

### Week 2: Federation Security
- [ ] Build instance trust scoring
- [ ] Enhance activity validation
- [ ] Add replay attack prevention
- [ ] Implement federation audit logs

### Week 3: Privacy Infrastructure
- [ ] Create visibility enforcement layer
- [ ] Add retention policy API
- [ ] Implement consent management
- [ ] Build privacy preference API

### Week 4: Monitoring & Testing
- [ ] Add security metrics to CloudWatch
- [ ] Create penetration test suite
- [ ] Document security APIs
- [ ] Security audit preparation

## Cost Impact

### Minimal Overhead
- HMAC validation: <1ms per request
- Rate limiting: 1 DynamoDB write per request
- Trust scoring: 1 DynamoDB update per activity
- Total: ~3% increase in infrastructure cost

### High Value
- Prevents abuse (saves money)
- Enables compliance (reduces risk)
- Builds trust (increases adoption)
- Differentiates Lesser (competitive advantage)

## Developer Experience

### For Frontend Developers
```javascript
// Using Lesser's secure API
const lesser = new LesserClient({
    appId: 'your-app-id',
    secret: 'your-secret',
    
    // Automatic request signing
    signRequests: true,
    
    // Automatic retry with backoff
    retryStrategy: 'exponential'
});

// Privacy-aware operations
const posts = await lesser.posts.list({
    // Automatically filtered by visibility
    viewer: currentUser
});

// Consent management
try {
    await lesser.analytics.track(event);
} catch (e) {
    if (e.code === 'consent_required') {
        // Show consent dialog
        await showConsentDialog(e.purpose);
    }
}
```

### For Instance Admins
```bash
# View instance trust scores
lesser-cli federation trust list

# Update retention policy
lesser-cli privacy retention set --status-days 365 --media-days 90

# Export audit logs
lesser-cli security audit export --start 2024-01-01 --format json
```

## Conclusion

By implementing these infrastructure-level security features, Lesser provides:

1. **Secure by Default**: Every API call is authenticated and rate-limited
2. **Privacy Primitives**: Frontends can build privacy-respecting features
3. **Federation Trust**: Automatic protection from bad actors
4. **Compliance Ready**: GDPR, CCPA, and other regulations

This positions Lesser as the most secure and privacy-respecting ActivityPub infrastructure available.

---

*"Infrastructure security enables application innovation."* - Lesser Security Principles 