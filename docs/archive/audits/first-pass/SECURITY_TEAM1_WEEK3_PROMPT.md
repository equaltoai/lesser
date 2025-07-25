# AI Assistant Prompt: Security Team 1 Week 3 - Token Management & Production Hardening

## Your Role
You are continuing as the senior security engineer on Team 1. In Week 2, you successfully implemented CSRF protection (with in-memory store), error handling, and password policies. Now you'll migrate the CSRF store to DynamoDB for production use and implement token rotation/revocation.

## Week 2 Accomplishments ✅
- Secure error handling preventing info disclosure
- CSRF protection with in-memory store
- Strong password policy enforcement
- Chi router migration completed

## Context
Lesser runs as pure serverless - Lambda functions don't share memory between invocations. The in-memory CSRF store from Week 2 won't work in production. You need to migrate to DynamoDB for distributed token storage.

## Week 3 Objectives

### 1. Migrate CSRF Store to DynamoDB (Production Critical) 🔥

#### Why This Is Urgent
The current in-memory CSRF store has critical limitations:
- **Lambda Isolation**: Each Lambda invocation has its own memory - tokens aren't shared
- **No Persistence**: Tokens are lost when Lambda container recycles
- **No Distribution**: Multiple Lambda instances can't share tokens
- **Production Blocker**: CSRF protection is effectively broken in serverless

#### DynamoDB CSRF Store Implementation
**Create**: `pkg/auth/csrf_dynamodb.go`

```go
package auth

import (
    "fmt"
    "time"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/dynamodb"
    "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

// CSRFStore interface for token storage
type CSRFStore interface {
    Store(token string, userID string, expiresIn time.Duration) error
    Validate(token string, userID string) error
    Delete(token string) error
    GetUserActiveTokenCount(userID string) (int, error)
}

// DynamoDBCSRFStore implements distributed CSRF storage
type DynamoDBCSRFStore struct {
    db        *dynamodb.DynamoDB
    tableName string
}

type CSRFTokenRecord struct {
    Token     string `dynamodbav:"token"`      // Partition key
    UserID    string `dynamodbav:"user_id"`
    CreatedAt int64  `dynamodbav:"created_at"`
    ExpiresAt int64  `dynamodbav:"expires_at"` // TTL field
    Used      bool   `dynamodbav:"used"`
}

func NewDynamoDBCSRFStore(db *dynamodb.DynamoDB, tableName string) *DynamoDBCSRFStore {
    return &DynamoDBCSRFStore{
        db:        db,
        tableName: tableName,
    }
}

func (s *DynamoDBCSRFStore) Store(token string, userID string, expiresIn time.Duration) error {
    // Check token limit per user (prevent DoS)
    count, err := s.GetUserActiveTokenCount(userID)
    if err == nil && count >= 10 {
        return fmt.Errorf("too many active CSRF tokens")
    }
    
    now := time.Now()
    record := CSRFTokenRecord{
        Token:     token,
        UserID:    userID,
        CreatedAt: now.Unix(),
        ExpiresAt: now.Add(expiresIn).Unix(),
        Used:      false,
    }
    
    item, err := dynamodbattribute.MarshalMap(record)
    if err != nil {
        return fmt.Errorf("failed to marshal token: %w", err)
    }
    
    input := &dynamodb.PutItemInput{
        TableName: aws.String(s.tableName),
        Item:      item,
        // Prevent duplicate tokens
        ConditionExpression: aws.String("attribute_not_exists(#token)"),
        ExpressionAttributeNames: map[string]*string{
            "#token": aws.String("token"),
        },
    }
    
    _, err = s.db.PutItem(input)
    return err
}

func (s *DynamoDBCSRFStore) Validate(token string, userID string) error {
    // Get and validate in a single operation for atomicity
    input := &dynamodb.UpdateItemInput{
        TableName: aws.String(s.tableName),
        Key: map[string]*dynamodb.AttributeValue{
            "token": {S: aws.String(token)},
        },
        // Only update if: exists, belongs to user, not expired, not used
        ConditionExpression: aws.String(
            "attribute_exists(#token) AND " +
            "user_id = :user_id AND " +
            "expires_at > :now AND " +
            "used = :false"),
        UpdateExpression: aws.String("SET used = :true"),
        ExpressionAttributeNames: map[string]*string{
            "#token": aws.String("token"),
        },
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":user_id": {S: aws.String(userID)},
            ":now":     {N: aws.String(fmt.Sprintf("%d", time.Now().Unix()))},
            ":false":   {BOOL: aws.Bool(false)},
            ":true":    {BOOL: aws.Bool(true)},
        },
    }
    
    _, err := s.db.UpdateItem(input)
    if err != nil {
        if aerr, ok := err.(awserr.Error); ok {
            if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
                return ErrInvalidCSRF
            }
        }
        return fmt.Errorf("failed to validate token: %w", err)
    }
    
    return nil
}
```

#### Update CSRF Package
**Modify**: `pkg/auth/csrf.go`

```go
// Global store (initialized at startup)
var store CSRFStore

// InitializeStore sets up the CSRF store (called from main)
func InitializeStore(s CSRFStore) {
    store = s
}

// Update GenerateCSRFToken to use interface
func GenerateCSRFToken(userID string) (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    
    token := base64.URLEncoding.EncodeToString(b)
    
    // Use the configured store
    if err := store.Store(token, userID, 1*time.Hour); err != nil {
        return "", err
    }
    
    return token, nil
}

// Update ValidateCSRFToken to use interface
func ValidateCSRFToken(token string, userID string) error {
    return store.Validate(token, userID)
}
```

#### Table Creation
Add to infrastructure setup:

```go
// infra/tables/csrf_tokens.go
func CreateCSRFTable(db *dynamodb.DynamoDB) error {
    input := &dynamodb.CreateTableInput{
        TableName: aws.String("lesser-csrf-tokens"),
        KeySchema: []*dynamodb.KeySchemaElement{
            {
                AttributeName: aws.String("token"),
                KeyType:       aws.String(dynamodb.KeyTypeHash),
            },
        },
        AttributeDefinitions: []*dynamodb.AttributeDefinition{
            {
                AttributeName: aws.String("token"),
                AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
            },
        },
        BillingMode: aws.String(dynamodb.BillingModePayPerRequest),
        TimeToLiveSpecification: &dynamodb.TimeToLiveSpecification{
            Enabled:       aws.Bool(true),
            AttributeName: aws.String("expires_at"),
        },
    }
    
    _, err := db.CreateTable(input)
    return err
}
```

### 2. Token Rotation & Revocation (LSS-014) - Medium Priority

#### Refresh Token Management
**Create**: `pkg/auth/refresh_tokens.go`

```go
package auth

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "time"
)

type RefreshToken struct {
    Token         string `dynamodbav:"token"`          // Partition key
    UserID        string `dynamodbav:"user_id"`        // GSI partition key
    Family        string `dynamodbav:"family"`         // Token family for rotation
    Generation    int    `dynamodbav:"generation"`     // Rotation generation
    CreatedAt     int64  `dynamodbav:"created_at"`
    ExpiresAt     int64  `dynamodbav:"expires_at"`
    LastUsedAt    int64  `dynamodbav:"last_used_at"`
    Revoked       bool   `dynamodbav:"revoked"`
    RevokedReason string `dynamodbav:"revoked_reason"`
}

type RefreshTokenStore struct {
    db        *dynamodb.DynamoDB
    tableName string
}

// CreateRefreshToken generates a new refresh token
func (s *RefreshTokenStore) CreateRefreshToken(userID string) (*RefreshToken, error) {
    tokenBytes := make([]byte, 32)
    familyBytes := make([]byte, 16)
    
    rand.Read(tokenBytes)
    rand.Read(familyBytes)
    
    token := &RefreshToken{
        Token:      base64.URLEncoding.EncodeToString(tokenBytes),
        UserID:     userID,
        Family:     base64.URLEncoding.EncodeToString(familyBytes),
        Generation: 1,
        CreatedAt:  time.Now().Unix(),
        ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days
        Revoked:    false,
    }
    
    // Store in DynamoDB
    item, _ := dynamodbattribute.MarshalMap(token)
    _, err := s.db.PutItem(&dynamodb.PutItemInput{
        TableName: aws.String(s.tableName),
        Item:      item,
    })
    
    return token, err
}

// RotateRefreshToken implements secure rotation with reuse detection
func (s *RefreshTokenStore) RotateRefreshToken(oldToken string) (*RefreshToken, error) {
    // Get the old token
    oldRefresh, err := s.GetRefreshToken(oldToken)
    if err != nil {
        return nil, err
    }
    
    // Check if token was already used (reuse detection)
    if oldRefresh.Revoked {
        // SECURITY ALERT: Token reuse detected!
        // Revoke entire family
        s.RevokeTokenFamily(oldRefresh.Family, "Token reuse detected")
        
        logger.Error("Refresh token reuse detected",
            zap.String("user_id", oldRefresh.UserID),
            zap.String("family", oldRefresh.Family))
        
        return nil, ErrTokenReuse
    }
    
    // Create new token in same family
    newTokenBytes := make([]byte, 32)
    rand.Read(newTokenBytes)
    
    newToken := &RefreshToken{
        Token:      base64.URLEncoding.EncodeToString(newTokenBytes),
        UserID:     oldRefresh.UserID,
        Family:     oldRefresh.Family,
        Generation: oldRefresh.Generation + 1,
        CreatedAt:  time.Now().Unix(),
        ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Unix(),
        Revoked:    false,
    }
    
    // Transaction: revoke old, create new
    err = s.db.TransactWriteItems(&dynamodb.TransactWriteItemsInput{
        TransactItems: []*dynamodb.TransactWriteItem{
            {
                Update: &dynamodb.Update{
                    TableName: aws.String(s.tableName),
                    Key: map[string]*dynamodb.AttributeValue{
                        "token": {S: aws.String(oldToken)},
                    },
                    UpdateExpression: aws.String("SET revoked = :true, revoked_reason = :reason"),
                    ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
                        ":true":   {BOOL: aws.Bool(true)},
                        ":reason": {S: aws.String("Rotated")},
                    },
                },
            },
            {
                Put: &dynamodb.Put{
                    TableName: aws.String(s.tableName),
                    Item:      mustMarshal(newToken),
                },
            },
        },
    })
    
    if err != nil {
        return nil, fmt.Errorf("failed to rotate token: %w", err)
    }
    
    return newToken, nil
}

// RevokeTokenFamily revokes all tokens in a family (security breach response)
func (s *RefreshTokenStore) RevokeTokenFamily(family string, reason string) error {
    // Query all tokens in family using GSI
    expr, _ := expression.NewBuilder().
        WithKeyCondition(expression.Key("family").Equal(expression.Value(family))).
        Build()
    
    result, err := s.db.Query(&dynamodb.QueryInput{
        TableName:                 aws.String(s.tableName),
        IndexName:                 aws.String("family-index"),
        KeyConditionExpression:    expr.KeyCondition(),
        ExpressionAttributeNames:  expr.Names(),
        ExpressionAttributeValues: expr.Values(),
    })
    
    if err != nil {
        return err
    }
    
    // Revoke each token
    for _, item := range result.Items {
        token := item["token"].S
        if token != nil {
            s.db.UpdateItem(&dynamodb.UpdateItemInput{
                TableName: aws.String(s.tableName),
                Key: map[string]*dynamodb.AttributeValue{
                    "token": {S: token},
                },
                UpdateExpression: aws.String("SET revoked = :true, revoked_reason = :reason"),
                ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
                    ":true":   {BOOL: aws.Bool(true)},
                    ":reason": {S: aws.String(reason)},
                },
            })
        }
    }
    
    return nil
}

// RevokeUserTokens revokes all tokens for a user (logout all devices)
func (s *RefreshTokenStore) RevokeUserTokens(userID string, reason string) error {
    // Similar to RevokeTokenFamily but query by user_id GSI
    // Implementation follows same pattern
}
```

### 3. Security Logging Enhancement (LSS-015) - Low Priority

#### Structured Security Logging
**Create**: `pkg/common/security_logger.go`

```go
package common

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

var SecurityLogger *zap.Logger

func InitSecurityLogger() {
    config := zap.NewProductionConfig()
    config.OutputPaths = []string{"stdout"}
    
    // Add security-specific fields
    config.InitialFields = map[string]any{
        "service": "lesser",
        "type":    "security",
    }
    
    // Ensure sensitive data is not logged
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    
    SecurityLogger, _ = config.Build()
}

// Security event types
const (
    EventAuthFailure      = "auth_failure"
    EventCSRFFailure      = "csrf_failure"
    EventTokenReuse       = "token_reuse"
    EventRateLimitExceed  = "rate_limit_exceed"
    EventSSRFBlocked      = "ssrf_blocked"
    EventSuspiciousActivity = "suspicious_activity"
)

// LogSecurityEvent logs a security-relevant event
func LogSecurityEvent(event string, fields ...zap.Field) {
    // Always include timestamp and event type
    allFields := append([]zap.Field{
        zap.String("event", event),
        zap.Int64("timestamp", time.Now().Unix()),
    }, fields...)
    
    SecurityLogger.Warn("Security event", allFields...)
}

// Example usage in auth middleware
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        user, err := validateToken(token)
        
        if err != nil {
            LogSecurityEvent(EventAuthFailure,
                zap.String("ip", r.RemoteAddr),
                zap.String("path", r.URL.Path),
                zap.String("error", err.Error()),
                zap.String("user_agent", r.UserAgent()),
            )
            http.Error(w, "Unauthorized", 401)
            return
        }
        
        // ... rest of middleware
    })
}
```

## Success Criteria

### CSRF DynamoDB Migration Complete When:
- [ ] DynamoDB table created with TTL
- [ ] All CSRF operations use DynamoDB
- [ ] Token limit per user enforced
- [ ] Atomic validate-and-mark-used operation
- [ ] Tests verify distributed operation

### Token Rotation Complete When:
- [ ] Refresh tokens support rotation
- [ ] Reuse detection revokes family
- [ ] User can revoke all sessions
- [ ] Generation tracking implemented
- [ ] Security alerts on reuse

### Security Logging Complete When:
- [ ] All auth failures logged
- [ ] CSRF failures logged
- [ ] Rate limit events logged
- [ ] Structured format for analysis
- [ ] No sensitive data in logs

## Testing Requirements

### DynamoDB CSRF Tests
```go
func TestCSRFTokenAcrossLambdas(t *testing.T) {
    // Simulate two different Lambda instances
    store1 := NewDynamoDBCSRFStore(db, "csrf-tokens")
    store2 := NewDynamoDBCSRFStore(db, "csrf-tokens")
    
    // Create token in "Lambda 1"
    token := generateToken()
    err := store1.Store(token, "user123", 1*time.Hour)
    require.NoError(t, err)
    
    // Validate in "Lambda 2"
    err = store2.Validate(token, "user123")
    require.NoError(t, err)
    
    // Second validation should fail (single use)
    err = store2.Validate(token, "user123")
    require.Error(t, err)
}
```

### Token Rotation Tests
```go
func TestRefreshTokenReuse(t *testing.T) {
    store := NewRefreshTokenStore(db, "refresh-tokens")
    
    // Create initial token
    token1, _ := store.CreateRefreshToken("user123")
    
    // Rotate once
    token2, _ := store.RotateRefreshToken(token1.Token)
    
    // Try to use old token again (reuse attack)
    _, err := store.RotateRefreshToken(token1.Token)
    
    // Should fail and revoke entire family
    require.Equal(t, ErrTokenReuse, err)
    
    // New token should also be revoked
    token2Fresh, _ := store.GetRefreshToken(token2.Token)
    require.True(t, token2Fresh.Revoked)
}
```

## Important Notes

1. **Production Critical**: The in-memory CSRF store MUST be replaced before deployment
2. **TTL Configuration**: DynamoDB TTL may take up to 48 hours to delete expired items
3. **Cost Optimization**: Use on-demand billing for CSRF tokens (sporadic access pattern)
4. **Monitoring**: Set up CloudWatch alarms for token reuse detection

## Resources

- [DynamoDB Best Practices](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/best-practices.html)
- [OWASP Token Storage](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html#token-storage-on-client-side)
- [OAuth 2.0 Security Best Practices](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics) 