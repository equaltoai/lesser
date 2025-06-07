# Infrastructure Security Enhancements for Lesser

## Overview

Lesser is headless ActivityPub infrastructure - an API layer that handles federation, storage, and compute. Security enhancements should focus on:

1. **API Security**: Protecting the infrastructure endpoints
2. **Federation Security**: Securing ActivityPub communications
3. **Storage Security**: Protecting data at rest and in transit
4. **Compute Security**: Lambda and serverless-specific protections
5. **Privacy APIs**: Exposing privacy primitives for frontends to use

## Core Infrastructure Security

### 1. API Gateway Security Layer

#### 1.1 Request Signing & Verification
```go
// pkg/security/request_signing.go
type RequestSigner struct {
    algorithm string // "AWS-HMAC-SHA256" or "ED25519"
}

type SignedRequest struct {
    Method    string
    Path      string
    Headers   map[string]string
    Body      []byte
    Signature string
    Timestamp time.Time
}

// Require signature on all mutation endpoints
func (s *RequestSigner) VerifyRequest(r *http.Request) error {
    // Extract signature headers
    sig := r.Header.Get("X-Lesser-Signature")
    timestamp := r.Header.Get("X-Lesser-Timestamp")
    
    // Verify timestamp (prevent replay)
    if time.Since(timestamp) > 5*time.Minute {
        return ErrRequestTooOld
    }
    
    // Verify signature
    canonical := s.canonicalizeRequest(r)
    if !s.verify(canonical, sig) {
        return ErrInvalidSignature
    }
    
    return nil
}
```

#### 1.2 Rate Limiting with Token Buckets
```go
// pkg/security/rate_limiter.go
type RateLimiter struct {
    store DynamoDBStorage
}

type RateLimitKey struct {
    Type   string // "ip", "user", "app"
    Value  string
    Window string // "minute", "hour", "day"
}

func (r *RateLimiter) CheckLimit(ctx context.Context, key RateLimitKey) error {
    // Store in DynamoDB with TTL
    // PK: RATELIMIT#type#value
    // SK: WINDOW#window#timestamp
    
    count, err := r.store.IncrementCounter(ctx, key)
    if err != nil {
        return err
    }
    
    limit := r.getLimit(key)
    if count > limit {
        return ErrRateLimitExceeded
    }
    
    // Return remaining quota in headers
    SetRateLimitHeaders(ctx, limit, count, resetTime)
    
    return nil
}
```

#### 1.3 OAuth App Restrictions
```go
// pkg/security/oauth_security.go
type OAuthSecurity struct {
    store DynamoDBStorage
}

type AppRestrictions struct {
    AppID           string
    AllowedScopes   []string
    AllowedIPs      []string
    RateLimits      map[string]int
    RequirePKCE     bool
    MaxTokens       int
    AllowedDomains  []string // For CORS
}

func (o *OAuthSecurity) ValidateApp(ctx context.Context, appID string) error {
    restrictions, err := o.store.GetAppRestrictions(ctx, appID)
    if err != nil {
        return err
    }
    
    // Validate current request against restrictions
    // Check IP, scopes, rate limits, etc.
    
    return nil
}
```

### 2. Federation Security Enhancements

#### 2.1 Enhanced HTTP Signature Verification
```go
// pkg/federation/security/signatures.go
type FederationSecurity struct {
    keyCache      *KeyCache
    instanceTrust map[string]float64
}

func (f *FederationSecurity) VerifyIncomingActivity(r *http.Request) error {
    // 1. Verify HTTP Signature (already implemented)
    actor, err := f.verifyHTTPSignature(r)
    if err != nil {
        return err
    }
    
    // 2. Check instance trust score
    instance := extractInstance(actor)
    if trust := f.instanceTrust[instance]; trust < 0.1 {
        return ErrUntrustedInstance
    }
    
    // 3. Verify activity signature (if present)
    activity := parseActivity(r.Body)
    if activity.Signature != nil {
        if err := f.verifyActivitySignature(activity); err != nil {
            return err
        }
    }
    
    // 4. Check for replay attacks
    if err := f.checkReplay(activity.ID); err != nil {
        return err
    }
    
    return nil
}
```

#### 2.2 Outbound Federation Security
```go
// pkg/federation/security/delivery.go
type SecureDelivery struct {
    signer  RequestSigner
    encrypt bool // Optional E2E encryption
}

func (s *SecureDelivery) DeliverActivity(
    ctx context.Context,
    activity Activity,
    inbox string,
) error {
    // 1. Sign the activity object itself
    signedActivity := s.signActivity(activity)
    
    // 2. Optional: Encrypt for recipient
    if s.encrypt && recipientKey := s.getPublicKey(inbox) {
        signedActivity = s.encryptActivity(signedActivity, recipientKey)
    }
    
    // 3. Create HTTP signature
    req := s.createRequest(inbox, signedActivity)
    s.signer.SignRequest(req)
    
    // 4. Deliver with retry logic
    return s.deliverWithRetry(ctx, req)
}
```

### 3. Storage Layer Security

#### 3.1 Field-Level Encryption
```go
// pkg/storage/encryption/field_encryption.go
type FieldEncryptor struct {
    kmsClient *kms.Client
    dataKey   []byte
}

type EncryptedField struct {
    Algorithm string `dynamodbav:"algorithm"`
    KeyID     string `dynamodbav:"key_id"`
    Nonce     []byte `dynamodbav:"nonce"`
    Data      []byte `dynamodbav:"data"`
}

// Encrypt sensitive fields before storage
func (f *FieldEncryptor) EncryptSensitiveFields(item interface{}) error {
    v := reflect.ValueOf(item)
    t := v.Type()
    
    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        if tag := field.Tag.Get("encrypt"); tag == "true" {
            // Encrypt this field
            plaintext := v.Field(i).Bytes()
            encrypted := f.Encrypt(plaintext)
            v.Field(i).Set(reflect.ValueOf(encrypted))
        }
    }
    
    return nil
}

// Example usage in types
type DirectMessage struct {
    ID        string `dynamodbav:"id"`
    Sender    string `dynamodbav:"sender"`
    Recipient string `dynamodbav:"recipient"`
    Content   string `dynamodbav:"content" encrypt:"true"`
    CreatedAt time.Time `dynamodbav:"created_at"`
}
```

#### 3.2 Audit Logging
```go
// pkg/storage/audit/logger.go
type AuditLogger struct {
    stream   *kinesis.Client
    streamName string
}

type AuditEvent struct {
    ID          string
    Timestamp   time.Time
    ActorID     string
    ActorIP     string
    Action      string
    Resource    string
    Result      string
    Details     map[string]interface{}
    Signature   string // Sign audit logs
}

func (a *AuditLogger) LogDataAccess(ctx context.Context, event AuditEvent) error {
    // Sign the audit event
    event.Signature = a.signEvent(event)
    
    // Send to Kinesis Firehose for durable storage
    data, _ := json.Marshal(event)
    
    _, err := a.stream.PutRecord(ctx, &kinesis.PutRecordInput{
        StreamName: &a.streamName,
        Data:       data,
        PartitionKey: aws.String(event.ActorID),
    })
    
    return err
}
```

### 4. Lambda Security Patterns

#### 4.1 Function-Level Permissions
```go
// pkg/lambda/security/permissions.go
type LambdaPermissions struct {
    functionName string
    permissions  map[string][]string // action -> resources
}

// Validate Lambda has minimum required permissions
func (l *LambdaPermissions) ValidatePermissions(ctx context.Context) error {
    // Use STS to assume role and check permissions
    creds := stscreds.NewAssumeRoleProvider(sts.New(sess), roleARN)
    
    // Test each required permission
    for action, resources := range l.permissions {
        if err := l.testPermission(ctx, action, resources); err != nil {
            return fmt.Errorf("missing permission %s: %w", action, err)
        }
    }
    
    return nil
}

// Implement least-privilege by function
func GetLambdaPermissions(functionName string) []iam.PolicyStatement {
    switch functionName {
    case "inbox-processor":
        return []iam.PolicyStatement{
            {
                Effect: "Allow",
                Action: []string{"dynamodb:PutItem"},
                Resource: []string{"arn:aws:dynamodb:*:*:table/lesser-*"},
            },
        }
    // ... other functions
    }
}
```

#### 4.2 Environment Variable Encryption
```go
// pkg/lambda/security/env.go
type SecureEnvironment struct {
    kmsKeyID string
}

func (s *SecureEnvironment) GetSecret(key string) (string, error) {
    encrypted := os.Getenv(key)
    if encrypted == "" {
        return "", fmt.Errorf("secret %s not found", key)
    }
    
    // Decrypt using KMS
    decoded, _ := base64.StdEncoding.DecodeString(encrypted)
    
    result, err := s.kmsClient.Decrypt(context.Background(), &kms.DecryptInput{
        CiphertextBlob: decoded,
    })
    
    return string(result.Plaintext), err
}
```

### 5. Privacy Infrastructure APIs

#### 5.1 Privacy Preferences API
```go
// pkg/privacy/preferences.go
type PrivacyPreference struct {
    UserID              string
    DefaultVisibility   string
    IndexInSearch       bool
    AllowAnonymousViews bool
    RequireFollowApproval bool
    BlockedInstances    []string
    EncryptDMs         bool
    DataRetention      RetentionPolicy
}

type PrivacyAPI struct {
    store DynamoDBStorage
}

// Expose privacy controls via API
func (p *PrivacyAPI) GetPrivacyControls(ctx context.Context, userID string) (*PrivacyControls, error) {
    // Return all privacy options with current settings
    // Frontend can build UI from this
}

func (p *PrivacyAPI) ValidateAction(ctx context.Context, action Action) error {
    // Check if action is allowed based on privacy settings
    prefs := p.getPreferences(ctx, action.ActorID)
    
    switch action.Type {
    case "view_profile":
        if !prefs.AllowAnonymousViews && !action.IsAuthenticated {
            return ErrPrivacyRestriction
        }
    case "send_dm":
        if prefs.BlockedInstances.Contains(action.ActorInstance) {
            return ErrInstanceBlocked
        }
    }
    
    return nil
}
```

#### 5.2 Data Export API
```go
// pkg/privacy/export.go
type DataExporter struct {
    storage DynamoDBStorage
    s3      S3Client
}

func (d *DataExporter) ExportUserData(ctx context.Context, userID string, format string) (*ExportJob, error) {
    // Create export job
    job := &ExportJob{
        ID:        uuid.New().String(),
        UserID:    userID,
        Format:    format, // "activitypub", "mastodon", "json"
        Status:    "pending",
        CreatedAt: time.Now(),
    }
    
    // Queue for Lambda processing
    d.queueExportJob(ctx, job)
    
    // Return job for status tracking
    return job, nil
}

// Lambda function processes export
func processExport(ctx context.Context, job ExportJob) error {
    // Gather all user data
    data := d.gatherUserData(ctx, job.UserID)
    
    // Format according to spec
    formatted := d.formatData(data, job.Format)
    
    // Encrypt the export
    encrypted := d.encryptExport(formatted, job.UserID)
    
    // Upload to S3 with expiry
    url := d.uploadToS3(encrypted, 24*time.Hour)
    
    // Notify user
    d.notifyExportReady(job.UserID, url)
    
    return nil
}
```

### 6. Cryptographic Infrastructure

#### 6.1 Key Management Service
```go
// pkg/crypto/kms/service.go
type KeyManagementService struct {
    kms      *kms.Client
    keyAlias string
}

// Hierarchical key derivation
func (k *KeyManagementService) DeriveKey(ctx context.Context, purpose string, userID string) ([]byte, error) {
    // Generate data key for specific purpose
    dataKeySpec := types.DataKeySpecAes256
    
    result, err := k.kms.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
        KeyId:         &k.keyAlias,
        KeySpec:       &dataKeySpec,
        EncryptionContext: map[string]string{
            "purpose": purpose,
            "user_id": userID,
            "service": "lesser",
        },
    })
    
    // Cache the plaintext key in memory
    // Store the ciphertext key
    
    return result.Plaintext, nil
}
```

#### 6.2 Content Integrity Service
```go
// pkg/crypto/integrity/service.go
type IntegrityService struct {
    hasher hash.Hash
}

type ContentHash struct {
    Algorithm string `json:"algorithm"`
    Hash      string `json:"hash"`
    Timestamp time.Time `json:"timestamp"`
}

// Hash content for integrity verification
func (i *IntegrityService) HashContent(content interface{}) (*ContentHash, error) {
    // Canonicalize content
    canonical, err := i.canonicalize(content)
    if err != nil {
        return nil, err
    }
    
    // Generate hash
    i.hasher.Reset()
    i.hasher.Write(canonical)
    hash := i.hasher.Sum(nil)
    
    return &ContentHash{
        Algorithm: "SHA256",
        Hash:      hex.EncodeToString(hash),
        Timestamp: time.Now(),
    }, nil
}
```

### 7. Infrastructure Monitoring & Detection

#### 7.1 Anomaly Detection Service
```go
// pkg/security/detection/anomaly.go
type AnomalyDetector struct {
    metrics   CloudWatchClient
    threshold float64
}

type SecurityMetric struct {
    Name      string
    Value     float64
    Timestamp time.Time
    Dimensions map[string]string
}

func (a *AnomalyDetector) MonitorAPIUsage(ctx context.Context) error {
    // Track unusual patterns
    metrics := []SecurityMetric{
        {Name: "FailedAuthentications", Value: failedAuths},
        {Name: "UnusualAPICallPattern", Value: patternScore},
        {Name: "DataExfiltrationRisk", Value: exfilScore},
    }
    
    for _, metric := range metrics {
        if metric.Value > a.threshold {
            a.triggerAlert(ctx, metric)
        }
    }
    
    return nil
}
```

## Implementation Priorities for Infrastructure

### Phase 1: API Security (Weeks 1-2)
1. Request signing and verification
2. Enhanced rate limiting
3. OAuth app restrictions
4. Audit logging infrastructure

### Phase 2: Federation Security (Weeks 3-4)
1. Enhanced signature verification
2. Instance trust scoring
3. Replay attack prevention
4. Secure delivery with retry

### Phase 3: Storage Security (Weeks 5-6)
1. Field-level encryption for PII
2. Key rotation automation
3. Backup encryption
4. Access audit trails

### Phase 4: Privacy APIs (Weeks 7-8)
1. Privacy preference management
2. Data export/import APIs
3. Consent management
4. Right to deletion implementation

## Cost Analysis for Infrastructure

### Per-Request Costs
- Request signing: +0.1ms Lambda time
- Field encryption: +$0.00001/request (KMS)
- Audit logging: +$0.00002/request (Kinesis)
- **Total overhead**: ~3% increase in cost

### Storage Costs
- Encrypted fields: +20% storage size
- Audit logs: +$0.023/GB/month (S3)
- Key management: +$1/month/key

### Operational Benefits
- Reduced fraud/abuse saves money
- Better compliance reduces legal risk
- Trust increases platform value
- Security incidents are costly

## Success Metrics

### Technical Metrics
- API authentication success rate
- Federation signature verification rate
- Encryption/decryption latency
- Key rotation frequency

### Security Metrics
- Unauthorized access attempts blocked
- Data breaches prevented
- Compliance audit pass rate
- Instance trust accuracy

### Business Metrics
- Developer adoption of security features
- Instance-to-instance trust scores
- Privacy API usage
- Security incident reduction

## Conclusion

By focusing on infrastructure-level security, Lesser can provide:

1. **Secure APIs** that frontends can trust
2. **Federation security** that protects the network
3. **Privacy primitives** that enable innovation
4. **Compliance tools** for global operation
5. **Trust infrastructure** for the ecosystem

These enhancements make Lesser the most secure choice for building ActivityPub applications, regardless of the frontend implementation.

---

*"Security is not a feature, it's the foundation."* - Lesser Infrastructure Philosophy 