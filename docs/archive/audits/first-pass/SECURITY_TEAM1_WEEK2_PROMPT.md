# AI Assistant Prompt: Security Team 1 Week 2 - Error Handling & Advanced Protection

## Your Role
You are continuing as the senior security engineer on Team 1. In Week 1, you successfully implemented authentication infrastructure and SSRF protection. Now you'll focus on preventing information disclosure, implementing CSRF protection, and strengthening password policies.

## Week 1 Accomplishments ✅
- Central authentication for GraphQL and REST APIs
- Secure HTTP client with SSRF protection
- Outbox visibility controls
- All critical auth issues resolved

## Context
Lesser runs as pure serverless (no VPC), so all security is application-level. The codebase now has:
- Working JWT authentication middleware
- Secure HTTP client blocking private IPs
- User context available in all handlers

## Week 2 Objectives

### 1. Fix Information Disclosure in Errors (LSS-002) - Medium Priority

#### Create Secure Error Handler
**File**: `pkg/common/errors.go`

Current errors leak sensitive information. You must:
1. Create standardized error types
2. Separate internal errors from user-facing messages
3. Log detailed errors internally
4. Return generic messages to users

```go
package common

import (
    "fmt"
    "go.uber.org/zap"
)

type AppError struct {
    Code          string  // Internal error code
    UserMessage   string  // Safe message for users
    InternalError error   // Detailed error for logging
    StatusCode    int     // HTTP status code
}

func (e AppError) Error() string {
    return e.UserMessage
}

// Error constructors
func ErrUnauthorized(internal error) AppError {
    return AppError{
        Code:          "AUTH_FAILED",
        UserMessage:   "Authentication failed",
        InternalError: internal,
        StatusCode:    401,
    }
}

func ErrNotFound(resource string) AppError {
    return AppError{
        Code:          "NOT_FOUND",
        UserMessage:   "Resource not found",
        InternalError: fmt.Errorf("%s not found", resource),
        StatusCode:    404,
    }
}

func ErrInternal(internal error) AppError {
    return AppError{
        Code:          "INTERNAL_ERROR",
        UserMessage:   "An error occurred processing your request",
        InternalError: internal,
        StatusCode:    500,
    }
}

// Safe error response
func HandleError(logger *zap.Logger, err error) (int, string) {
    if appErr, ok := err.(AppError); ok {
        logger.Error("Request failed",
            zap.String("code", appErr.Code),
            zap.Error(appErr.InternalError))
        return appErr.StatusCode, appErr.UserMessage
    }
    
    // Unknown errors get generic message
    logger.Error("Unexpected error", zap.Error(err))
    return 500, "An error occurred processing your request"
}
```

#### Update All Error Responses
Replace patterns like:
```go
// BAD - Leaks internal details
return errorResponse(500, fmt.Sprintf("database error: %v", err))

// GOOD - Generic message, log details
logger.Error("Database query failed", zap.Error(err))
return errorResponse(500, "An error occurred")
```

### 2. Implement CSRF Protection (LSS-005) - Medium Priority

#### CSRF Token Package
**Create**: `pkg/auth/csrf.go`

```go
package auth

import (
    "crypto/rand"
    "encoding/base64"
    "time"
)

type CSRFToken struct {
    Token     string
    ExpiresAt time.Time
    UserID    string
}

var csrfStore = make(map[string]CSRFToken) // Use DynamoDB in production

func GenerateCSRFToken(userID string) (string, error) {
    // Generate secure random token
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    
    token := base64.URLEncoding.EncodeToString(b)
    
    // Store with expiration
    csrfStore[token] = CSRFToken{
        Token:     token,
        ExpiresAt: time.Now().Add(1 * time.Hour),
        UserID:    userID,
    }
    
    return token, nil
}

func ValidateCSRFToken(token string, userID string) error {
    stored, exists := csrfStore[token]
    if !exists {
        return ErrInvalidCSRF
    }
    
    if stored.UserID != userID {
        return ErrInvalidCSRF
    }
    
    if time.Now().After(stored.ExpiresAt) {
        delete(csrfStore, token)
        return ErrExpiredCSRF
    }
    
    // Single use - delete after validation
    delete(csrfStore, token)
    return nil
}

// Middleware for state-changing operations
func CSRFMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Skip for safe methods
        if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
            next(w, r)
            return
        }
        
        // Extract CSRF token
        csrfToken := r.Header.Get("X-CSRF-Token")
        if csrfToken == "" {
            http.Error(w, "Missing CSRF token", http.StatusForbidden)
            return
        }
        
        // Get user from context (set by auth middleware)
        user := r.Context().Value(UserContextKey).(User)
        
        // Validate token
        if err := ValidateCSRFToken(csrfToken, user.ID); err != nil {
            http.Error(w, "Invalid CSRF token", http.StatusForbidden)
            return
        }
        
        next(w, r)
    }
}
```

#### Apply to State-Changing Endpoints
Update REST API router:
```go
// Routes that modify data need CSRF protection
router.Route("/api/v1", func(r chi.Router) {
    r.Use(AuthMiddleware)
    r.Use(CSRFMiddleware)
    
    r.Post("/statuses", handleCreateStatus)
    r.Delete("/statuses/{id}", handleDeleteStatus)
    r.Post("/accounts/{id}/follow", handleFollow)
    // ... other state-changing endpoints
})
```

### 3. Implement Strong Password Policy (LSS-019) - Medium Priority

#### Password Validation
**File**: `pkg/auth/password.go`

```go
package auth

import (
    "fmt"
    "regexp"
    "strings"
    "unicode"
)

type PasswordPolicy struct {
    MinLength            int
    RequireUppercase     bool
    RequireLowercase     bool
    RequireNumbers       bool
    RequireSpecialChars  bool
    PreventCommonPasswords bool
}

var DefaultPolicy = PasswordPolicy{
    MinLength:            12,
    RequireUppercase:     true,
    RequireLowercase:     true,
    RequireNumbers:       true,
    RequireSpecialChars:  true,
    PreventCommonPasswords: true,
}

// Common passwords to block (load from file in production)
var commonPasswords = map[string]bool{
    "password123": true,
    "qwerty123":   true,
    "123456789":   true,
    // ... more
}

func ValidatePassword(password string, username string) error {
    // Check minimum length
    if len(password) < DefaultPolicy.MinLength {
        return fmt.Errorf("password must be at least %d characters", DefaultPolicy.MinLength)
    }
    
    // Check character requirements
    var hasUpper, hasLower, hasNumber, hasSpecial bool
    for _, char := range password {
        switch {
        case unicode.IsUpper(char):
            hasUpper = true
        case unicode.IsLower(char):
            hasLower = true
        case unicode.IsNumber(char):
            hasNumber = true
        case unicode.IsPunct(char) || unicode.IsSymbol(char):
            hasSpecial = true
        }
    }
    
    if DefaultPolicy.RequireUppercase && !hasUpper {
        return fmt.Errorf("password must contain at least one uppercase letter")
    }
    if DefaultPolicy.RequireLowercase && !hasLower {
        return fmt.Errorf("password must contain at least one lowercase letter")
    }
    if DefaultPolicy.RequireNumbers && !hasNumber {
        return fmt.Errorf("password must contain at least one number")
    }
    if DefaultPolicy.RequireSpecialChars && !hasSpecial {
        return fmt.Errorf("password must contain at least one special character")
    }
    
    // Check against username
    if strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
        return fmt.Errorf("password cannot contain username")
    }
    
    // Check common passwords
    if DefaultPolicy.PreventCommonPasswords && commonPasswords[strings.ToLower(password)] {
        return fmt.Errorf("password is too common")
    }
    
    return nil
}

// Password strength meter
func PasswordStrength(password string) int {
    score := 0
    
    // Length bonus
    if len(password) >= 8 { score++ }
    if len(password) >= 12 { score++ }
    if len(password) >= 16 { score++ }
    
    // Complexity bonus
    if regexp.MustCompile(`[a-z]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[A-Z]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[0-9]`).MatchString(password) { score++ }
    if regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) { score++ }
    
    // Penalty for patterns
    if regexp.MustCompile(`(.)\1{2,}`).MatchString(password) { score-- }
    if regexp.MustCompile(`(012|123|234|345|456|567|678|789|890)`).MatchString(password) { score-- }
    
    if score < 0 { score = 0 }
    if score > 5 { score = 5 }
    
    return score
}
```

#### Update Registration Endpoint
**File**: `cmd/auth/main.go`

```go
func handleRegister(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    var req RegisterRequest
    json.Unmarshal([]byte(request.Body), &req)
    
    // Validate password
    if err := ValidatePassword(req.Password, req.Username); err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       fmt.Sprintf(`{"error": "%s"}`, err.Error()),
        }, nil
    }
    
    // Check password strength
    strength := PasswordStrength(req.Password)
    if strength < 3 {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       `{"error": "Password is too weak"}`,
        }, nil
    }
    
    // ... rest of registration
}
```

### 4. Complete REST API Router Migration

Finish the router migration started in Week 1:

```go
// cmd/api/main.go
func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Initialize router
    r := chi.NewRouter()
    
    // Global middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(30 * time.Second))
    
    // Public routes
    r.Group(func(r chi.Router) {
        r.Post("/oauth/token", handleOAuthToken)
        r.Post("/oauth/authorize", handleOAuthAuthorize)
        r.Get("/.well-known/webfinger", handleWebFinger)
    })
    
    // API v1 routes (authenticated)
    r.Route("/api/v1", func(r chi.Router) {
        r.Use(AuthMiddleware)
        
        // Read-only routes
        r.Get("/accounts/{id}", handleGetAccount)
        r.Get("/statuses/{id}", handleGetStatus)
        
        // State-changing routes (need CSRF)
        r.Group(func(r chi.Router) {
            r.Use(CSRFMiddleware)
            
            r.Post("/statuses", handleCreateStatus)
            r.Delete("/statuses/{id}", handleDeleteStatus)
            r.Post("/media", handleUploadMedia)
        })
    })
    
    // Convert Lambda request to http.Request
    httpReq := lambdaRequestToHTTP(request)
    
    // Execute router
    w := httptest.NewRecorder()
    r.ServeHTTP(w, httpReq)
    
    // Convert response back to Lambda format
    return httpResponseToLambda(w)
}
```

## Success Criteria

### Error Handling Complete When:
- [ ] No internal errors in user-facing responses
- [ ] All errors logged with context
- [ ] Consistent error format across APIs
- [ ] Security errors are generic

### CSRF Protection Complete When:
- [ ] CSRF tokens required for state changes
- [ ] Tokens are single-use
- [ ] Token validation in middleware
- [ ] Safe methods excluded

### Password Policy Complete When:
- [ ] 12+ character minimum enforced
- [ ] Complexity requirements checked
- [ ] Common passwords blocked
- [ ] Strength meter implemented

## Testing Requirements

### Error Handling Tests
```go
func TestErrorHandling(t *testing.T) {
    tests := []struct {
        name         string
        internalErr  error
        expectedCode int
        expectedMsg  string
    }{
        {
            name:         "database error",
            internalErr:  sql.ErrNoRows,
            expectedCode: 404,
            expectedMsg:  "Resource not found",
        },
        {
            name:         "auth error",
            internalErr:  ErrInvalidToken,
            expectedCode: 401,
            expectedMsg:  "Authentication failed",
        },
    }
    // Verify no internal details leak
}
```

### CSRF Tests
```bash
# Should fail without token
curl -X POST $API/statuses \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"content": "test"}'

# Should succeed with valid token
curl -X POST $API/statuses \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d '{"content": "test"}'
```

## Important Notes

1. **No VPC**: Remember all security is application-level
2. **Logging**: Use structured logging for security events
3. **Performance**: CSRF tokens should use caching (DynamoDB)
4. **Coordination**: Team 2 may need your error types

## Resources

- [OWASP CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [NIST Password Guidelines](https://pages.nist.gov/800-63-3/sp800-63b.html)
- [Chi Router Documentation](https://github.com/go-chi/chi)

Remember: These fixes prevent attackers from gathering information about your system and strengthen authentication security. 