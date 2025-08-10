# Modern Auth Implementation Quick Start

## Overview: Email-Free Authentication

Lesser implements **passwordless authentication only** - no email verification, no SMS, no traditional passwords. Our modern auth stack supports:

🔑 **Passkeys (WebAuthn/FIDO2)** - Primary method, backed by Apple/Google/Microsoft  
🦊 **Crypto Wallets** - MetaMask, Phantom, WalletConnect integration  
🛡️ **Zero Knowledge** - No PII collection, no verification emails  
📱 **Cross-Device Sync** - Automatic via iCloud Keychain, Google Password Manager  

## Priority 1: Core JWT Infrastructure (3 days)

### Day 1: JWT & Session Management

```go
// cmd/auth/handlers/token.go
package handlers

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type TokenService struct {
    signingKey []byte
    issuer     string
}

func (s *TokenService) GenerateAccessToken(userID string, authMethod string) (string, error) {
    claims := jwt.MapClaims{
        "sub": userID,
        "iss": s.issuer,
        "iat": time.Now().Unix(),
        "exp": time.Now().Add(15 * time.Minute).Unix(),
        "auth_method": authMethod,
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(s.signingKey)
}

// Store refresh tokens in DynamoDB
type RefreshToken struct {
    PK        string    `dynamodbav:"PK"`        // USER#username
    SK        string    `dynamodbav:"SK"`        // SESSION#session-id
    Token     string    `dynamodbav:"Token"`     // hashed
    ExpiresAt time.Time `dynamodbav:"ExpiresAt"`
    DeviceID  string    `dynamodbav:"DeviceID"`
}
```

### Day 2: Rate Limiting

```go
// pkg/auth/ratelimit.go
func (rl *RateLimiter) CheckLimit(ctx context.Context, key string, limit int) error {
    now := time.Now()
    windowStart := now.Truncate(time.Minute)
    
    pk := fmt.Sprintf("RATELIMIT#%s", key)
    sk := windowStart.Format(time.RFC3339)
    
    // Atomic increment
    update := expression.Add(expression.Name("count"), expression.Value(1))
    
    _, err := rl.dynamo.UpdateItem(ctx, &dynamodb.UpdateItemInput{
        TableName: aws.String(rl.table),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: pk},
            "SK": &types.AttributeValueMemberS{Value: sk},
        },
        UpdateExpression: expr.Update(),
        // ... 
    })
    
    // Check if over limit
    // Set TTL for auto-cleanup
}
```

### Day 3: Basic Password Auth (Migration Path)

```go
// Temporary for migration only
func (a *AuthHandler) PasswordLogin(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    
    // 1. Rate limit check
    // 2. Verify password with Argon2id
    // 3. Generate tokens
    // 4. Prompt to set up passkey!
    
    respond(w, map[string]any{
        "access_token": accessToken,
        "refresh_token": refreshToken,
        "upgrade_required": true,
        "upgrade_url": "/auth/passkey/setup",
    })
}
```

## Priority 2: Passkeys (Week 2)

### Quick WebAuthn Setup

```bash
go get github.com/go-webauthn/webauthn
```

```go
// cmd/auth/handlers/passkey.go
import "github.com/go-webauthn/webauthn/webauthn"

func NewPasskeyHandler(domain string) (*PasskeyHandler, error) {
    wconfig := &webauthn.Config{
        RPDisplayName: "Lesser",
        RPID:          domain,
        RPOrigin:      fmt.Sprintf("https://%s", domain),
    }
    
    webAuthn, err := webauthn.New(wconfig)
    return &PasskeyHandler{webAuthn: webAuthn}, err
}

// Registration flow
func (h *PasskeyHandler) BeginRegistration(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r.Context())
    
    options, session, err := h.webAuthn.BeginRegistration(user)
    
    // Store session in cache with TTL
    h.cache.Set(sessionKey, session, 5*time.Minute)
    
    respond(w, options)
}
```

### Frontend Integration Example

```javascript
// Simple passkey registration
async function registerPasskey() {
    // 1. Get options from Lesser
    const options = await fetch('/api/v1/auth/webauthn/register/begin', {
        headers: { 'Authorization': `Bearer ${token}` }
    }).then(r => r.json());
    
    // 2. Create credential
    const credential = await navigator.credentials.create({
        publicKey: options
    });
    
    // 3. Send to Lesser
    await fetch('/api/v1/auth/webauthn/register/finish', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` },
        body: JSON.stringify(credential)
    });
}
```

## Priority 3: Wallet Auth (Week 3)

### EIP-4361 Implementation

```go
// pkg/auth/siwe/siwe.go
type SIWEMessage struct {
    Domain         string
    Address        string
    Statement      string
    URI            string
    Version        string
    ChainId        int
    Nonce          string
    IssuedAt       time.Time
    ExpirationTime time.Time
}

func (m *SIWEMessage) String() string {
    return fmt.Sprintf(`%s wants you to sign in with your Ethereum account:
%s

%s

URI: %s
Version: %s
Chain ID: %d
Nonce: %s
Issued At: %s
Expiration Time: %s`, 
        m.Domain, m.Address, m.Statement, 
        m.URI, m.Version, m.ChainId, 
        m.Nonce, m.IssuedAt.Format(time.RFC3339),
        m.ExpirationTime.Format(time.RFC3339))
}

func VerifySignature(message, signature string, address string) error {
    // 1. Recover address from signature
    // 2. Verify it matches claimed address
    // 3. Check message format
    // 4. Validate nonce and timestamps
}
```

### Multi-Chain Support

```go
type WalletAuth struct {
    chains map[int]ChainConfig
}

type ChainConfig struct {
    Name      string
    ChainID   int
    RPCUrl    string
    VerifyFn  func(message, sig, addr string) error
}

// Support Ethereum, Polygon, BSC, etc.
var DefaultChains = map[int]ChainConfig{
    1:     {Name: "Ethereum", ChainID: 1},
    137:   {Name: "Polygon", ChainID: 137},
    56:    {Name: "BSC", ChainID: 56},
}
```

## Quick Deployment Guide

### 1. Environment Variables

```bash
# .env
JWT_SIGNING_KEY=your-secret-key
DOMAIN=lesser.social
AWS_REGION=us-east-1
DYNAMO_TABLE=lesser-main
```

### 2. Update API Gateway

```go
// Add auth endpoints to API
authGroup := api.Group("/auth")
{
    // Core auth
    authGroup.POST("/login", handlers.PasswordLogin)
    authGroup.POST("/refresh", handlers.RefreshToken)
    authGroup.POST("/logout", handlers.Logout)
    
    // Passkeys
    authGroup.POST("/webauthn/register/begin", handlers.BeginRegistration)
    authGroup.POST("/webauthn/register/finish", handlers.FinishRegistration)
    authGroup.POST("/webauthn/login/begin", handlers.BeginLogin)
    authGroup.POST("/webauthn/login/finish", handlers.FinishLogin)
    
    // Wallet
    authGroup.POST("/wallet/challenge", handlers.GetWalletChallenge)
    authGroup.POST("/wallet/verify", handlers.VerifyWalletSignature)
}
```

### 3. Middleware

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        if token == "" {
            respondError(w, "Unauthorized", 401)
            return
        }
        
        claims, err := validateToken(token)
        if err != nil {
            respondError(w, "Invalid token", 401)
            return
        }
        
        ctx := context.WithValue(r.Context(), "user_id", claims["sub"])
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## Testing Checklist

### Week 1 Tests
- [ ] JWT generation and validation
- [ ] Refresh token flow
- [ ] Rate limiting works
- [ ] Session management
- [ ] Password migration path

### Week 2 Tests
- [ ] Passkey registration
- [ ] Passkey authentication
- [ ] Multiple devices
- [ ] Device management
- [ ] Challenge expiration

### Week 3 Tests
- [ ] Wallet signature verification
- [ ] Multi-chain support
- [ ] Nonce validation
- [ ] Message format compliance
- [ ] Account linking

## Monitoring Setup

```go
// CloudWatch metrics
func trackAuthEvent(action string, method string, success bool) {
    metric := cloudwatch.PutMetricDataInput{
        Namespace: aws.String("Lesser/Auth"),
        MetricData: []types.MetricDatum{
            {
                MetricName: aws.String("AuthenticationAttempts"),
                Value:      aws.Float64(1),
                Dimensions: []types.Dimension{
                    {Name: aws.String("Action"), Value: aws.String(action)},
                    {Name: aws.String("Method"), Value: aws.String(method)},
                    {Name: aws.String("Success"), Value: aws.String(fmt.Sprint(success))},
                },
            },
        },
    }
}
```

## Success Criteria

### Week 1 ✓
- Users can get JWT tokens
- Rate limiting prevents abuse
- Sessions are managed properly
- Old passwords still work (for migration)

### Week 2 ✓
- Users can register passkeys
- Passkey login works
- Multiple devices supported
- No passwords needed for new users

### Week 3 ✓
- Ethereum wallet login works
- Multiple chains supported
- Wallet linking works
- No email required

## Next Steps

1. **Start with Week 1** - Get JWT infrastructure solid
2. **Add Passkeys** - Modern passwordless auth
3. **Add Wallets** - Web3 native support
4. **Remove passwords** - After migration period

This gives Lesser modern, secure authentication that costs <$0.001 per user! 