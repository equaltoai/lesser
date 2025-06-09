# AI Assistant Prompt: Security Team 1 Week 4 - Optional Security Enhancements

## Your Role
You are continuing as the senior security engineer on Team 1. Congratulations! All critical, high, and medium priority security issues have been resolved. Lesser is now production-ready. This week focuses on optional low-priority enhancements that provide defense-in-depth.

## Production Status: READY ✅
- All significant vulnerabilities resolved
- CSRF protection working in serverless
- Authentication comprehensive
- No blockers for deployment

## Context
These remaining items are "nice-to-have" security enhancements. They're not required for secure operation but add extra layers of protection and follow security best practices.

## Week 4 Objectives (Optional Enhancements)

### 1. Cookie Security Headers (LSS-013) - Low Priority

#### Implement Secure Cookie Settings
**Update**: `cmd/api/handlers/oauth.go` and auth-related handlers

```go
package handlers

import (
    "net/http"
    "time"
)

// SetSecureCookie sets a cookie with all security flags
func SetSecureCookie(w http.ResponseWriter, name, value string, maxAge int) {
    cookie := &http.Cookie{
        Name:     name,
        Value:    value,
        Path:     "/",
        Domain:   "", // Let browser handle domain
        MaxAge:   maxAge,
        Secure:   true,  // HTTPS only
        HttpOnly: true,  // No JavaScript access
        SameSite: http.SameSiteStrictMode, // CSRF protection
    }
    
    http.SetCookie(w, cookie)
}

// Example usage in OAuth handler
func handleOAuthToken(w http.ResponseWriter, r *http.Request) {
    // ... generate tokens ...
    
    // Set secure session cookie
    SetSecureCookie(w, "session_token", sessionToken, 3600) // 1 hour
    
    // For refresh tokens, use longer expiry
    SetSecureCookie(w, "refresh_token", refreshToken, 86400*30) // 30 days
    
    // ... rest of handler
}

// Add security headers middleware
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Security headers
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        
        // HSTS for HTTPS enforcement
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### 2. CORS Headers Configuration (LSS-016) - Low Priority

#### Implement Proper CORS Policy
**Create**: `pkg/middleware/cors.go`

```go
package middleware

import (
    "net/http"
    "strings"
)

type CORSConfig struct {
    AllowedOrigins   []string
    AllowedMethods   []string
    AllowedHeaders   []string
    ExposedHeaders   []string
    AllowCredentials bool
    MaxAge           int
}

// DefaultCORSConfig provides secure defaults
var DefaultCORSConfig = CORSConfig{
    AllowedOrigins: []string{
        "https://lesser.example.com",
        "https://app.lesser.example.com",
    },
    AllowedMethods: []string{
        http.MethodGet,
        http.MethodPost,
        http.MethodPut,
        http.MethodDelete,
        http.MethodOptions,
    },
    AllowedHeaders: []string{
        "Authorization",
        "Content-Type",
        "X-CSRF-Token",
    },
    ExposedHeaders: []string{
        "X-RateLimit-Limit",
        "X-RateLimit-Remaining",
        "X-RateLimit-Reset",
    },
    AllowCredentials: true,
    MaxAge:          86400, // 24 hours
}

func CORS(config CORSConfig) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            
            // Check if origin is allowed
            allowed := false
            for _, allowedOrigin := range config.AllowedOrigins {
                if allowedOrigin == "*" || allowedOrigin == origin {
                    allowed = true
                    break
                }
            }
            
            if allowed && origin != "" {
                w.Header().Set("Access-Control-Allow-Origin", origin)
            }
            
            // Handle preflight
            if r.Method == http.MethodOptions {
                w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
                w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
                w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))
                
                if config.AllowCredentials {
                    w.Header().Set("Access-Control-Allow-Credentials", "true")
                }
                
                w.WriteHeader(http.StatusNoContent)
                return
            }
            
            // Actual request
            if len(config.ExposedHeaders) > 0 {
                w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
            }
            
            if config.AllowCredentials {
                w.Header().Set("Access-Control-Allow-Credentials", "true")
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

### 3. HTTP Signature Validation Enhancements (LSS-012) - Low Priority

#### Already Partially Implemented
Team 2 worked on this in Week 3. Your task is to review and enhance:

**Update**: `pkg/federation/httpsig.go`

```go
// Add support for draft-ietf-httpbis-message-signatures
func VerifyHTTPSignatureV2(r *http.Request, publicKey crypto.PublicKey) error {
    // New draft specification support
    signatureInput := r.Header.Get("Signature-Input")
    signature := r.Header.Get("Signature")
    
    if signatureInput == "" || signature == "" {
        // Fall back to legacy verification
        return VerifyHTTPSignature(r, publicKey)
    }
    
    // Parse structured field values
    // Implementation follows draft-ietf-httpbis-message-signatures
    
    // ... implementation details ...
}

// Add signature algorithm negotiation
func NegotiateSignatureAlgorithm(acceptedAlgorithms []string, keyType string) string {
    // Order of preference
    preferences := []string{
        "hs2019",       // Recommended
        "rsa-sha256",   // Legacy support
        "ecdsa-sha256", // ECDSA support
        "ed25519",      // EdDSA support
    }
    
    for _, pref := range preferences {
        for _, accepted := range acceptedAlgorithms {
            if pref == accepted && isCompatible(pref, keyType) {
                return pref
            }
        }
    }
    
    return "rsa-sha256" // Default fallback
}
```

## Success Criteria

### Cookie Security Complete When:
- [ ] All auth cookies use Secure flag
- [ ] HttpOnly prevents XSS access
- [ ] SameSite prevents CSRF
- [ ] Security headers on all responses

### CORS Complete When:
- [ ] Allowed origins configured
- [ ] Credentials handled properly
- [ ] Preflight requests optimized
- [ ] No wildcard with credentials

### HTTP Signatures Complete When:
- [ ] Multiple algorithms supported
- [ ] Draft spec compatibility
- [ ] Algorithm negotiation
- [ ] Backward compatibility maintained

## Testing Requirements

### Cookie Security Tests
```go
func TestSecureCookies(t *testing.T) {
    req := httptest.NewRequest("POST", "/oauth/token", nil)
    w := httptest.NewRecorder()
    
    handleOAuthToken(w, req)
    
    cookies := w.Result().Cookies()
    for _, cookie := range cookies {
        assert.True(t, cookie.Secure, "Cookie must be secure")
        assert.True(t, cookie.HttpOnly, "Cookie must be httpOnly")
        assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
    }
}
```

### CORS Tests
```go
func TestCORSPreflight(t *testing.T) {
    req := httptest.NewRequest("OPTIONS", "/api/v1/statuses", nil)
    req.Header.Set("Origin", "https://app.lesser.example.com")
    req.Header.Set("Access-Control-Request-Method", "POST")
    
    w := httptest.NewRecorder()
    handler := CORS(DefaultCORSConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
    
    handler.ServeHTTP(w, req)
    
    assert.Equal(t, 204, w.Code)
    assert.Equal(t, "https://app.lesser.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}
```

## Important Notes

1. **Optional**: These are enhancements, not requirements
2. **Backward Compatible**: Don't break existing functionality
3. **Performance**: Minimal impact on response times
4. **Configuration**: Make security headers configurable for different environments

## Resources

- [OWASP Secure Headers](https://owasp.org/www-project-secure-headers/)
- [MDN CORS Guide](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)
- [HTTP Message Signatures Draft](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-message-signatures)

Remember: These enhancements add extra security layers but Lesser is already production-ready without them! 