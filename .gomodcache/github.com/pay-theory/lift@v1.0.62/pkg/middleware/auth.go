package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/security"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	jwt.RegisteredClaims
	TenantID  string   `json:"tenant_id"`
	AccountID string   `json:"account_id"`
	Roles     []string `json:"roles"`
	Scopes    []string `json:"scopes"`
}

// JWTValidator handles JWT token validation
type JWTValidator struct {
	// 8-byte aligned fields
	publicKey *rsa.PublicKey
	secretKey []byte

	// Structs (varies)
	config security.JWTConfig
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(config security.JWTConfig) (*JWTValidator, error) {
	validator := &JWTValidator{
		config: config,
	}

	// Load keys based on signing method
	switch config.SigningMethod {
	case "RS256":
		if config.PublicKeyPath == "" {
			return nil, fmt.Errorf("public key path is required for RS256")
		}

		publicKey, err := loadRSAPublicKey(config.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w", err)
		}
		validator.publicKey = publicKey

	case "HS256":
		if config.SecretKey == "" {
			return nil, fmt.Errorf("secret key is required for HS256")
		}
		validator.secretKey = []byte(config.SecretKey)

	default:
		return nil, fmt.Errorf("unsupported signing method: %s", config.SigningMethod)
	}

	return validator, nil
}

// ValidateToken validates a JWT token and returns the claims
func (v *JWTValidator) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		// Verify signing method
		switch v.config.SigningMethod {
		case "RS256":
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return v.publicKey, nil
		case "HS256":
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return v.secretKey, nil
		default:
			return nil, fmt.Errorf("unsupported signing method: %s", v.config.SigningMethod)
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate standard claims
	if err := v.validateStandardClaims(claims); err != nil {
		return nil, err
	}

	// Validate custom claims
	if err := v.validateCustomClaims(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

// validateStandardClaims validates the standard JWT claims
func (v *JWTValidator) validateStandardClaims(claims *JWTClaims) error {
	validator := newStandardClaimsValidator(v, claims)
	return validator.validate()
}

// standardClaimsValidator validates JWT standard claims
type standardClaimsValidator struct {
	validator *JWTValidator
	claims    *JWTClaims
	now       time.Time
}

// newStandardClaimsValidator creates a new standard claims validator
func newStandardClaimsValidator(validator *JWTValidator, claims *JWTClaims) *standardClaimsValidator {
	return &standardClaimsValidator{
		validator: validator,
		claims:    claims,
		now:       time.Now(),
	}
}

// validate performs all standard claim validations
func (v *standardClaimsValidator) validate() error {
	if err := v.validateExpiration(); err != nil {
		return err
	}

	if err := v.validateNotBefore(); err != nil {
		return err
	}

	if err := v.validateMaxAge(); err != nil {
		return err
	}

	if err := v.validateIssuer(); err != nil {
		return err
	}

	return v.validateAudience()
}

// validateExpiration checks token expiration
func (v *standardClaimsValidator) validateExpiration() error {
	if v.claims.ExpiresAt == nil {
		return nil
	}

	if v.claims.ExpiresAt.Before(v.now) {
		return fmt.Errorf("token has expired")
	}

	return nil
}

// validateNotBefore checks token not-before time
func (v *standardClaimsValidator) validateNotBefore() error {
	if v.claims.NotBefore == nil {
		return nil
	}

	if v.claims.NotBefore.After(v.now) {
		return fmt.Errorf("token not valid yet")
	}

	return nil
}

// validateMaxAge checks token maximum age
func (v *standardClaimsValidator) validateMaxAge() error {
	if v.claims.IssuedAt == nil || v.validator.config.MaxAge <= 0 {
		return nil
	}

	maxAge := v.claims.IssuedAt.Add(v.validator.config.MaxAge)
	if v.now.After(maxAge) {
		return fmt.Errorf("token exceeds maximum age")
	}

	return nil
}

// validateIssuer checks token issuer
func (v *standardClaimsValidator) validateIssuer() error {
	if v.validator.config.Issuer == "" {
		return nil
	}

	if v.claims.Issuer != v.validator.config.Issuer {
		return fmt.Errorf("token validation failed: issuer mismatch")
	}

	return nil
}

// validateAudience checks token audience
func (v *standardClaimsValidator) validateAudience() error {
	if len(v.validator.config.Audience) == 0 {
		return nil
	}

	if v.isValidAudience() {
		return nil
	}

	return fmt.Errorf("token validation failed: audience mismatch")
}

// isValidAudience checks if any audience matches
func (v *standardClaimsValidator) isValidAudience() bool {
	for _, configAud := range v.validator.config.Audience {
		for _, claimAud := range v.claims.Audience {
			if configAud == claimAud {
				return true
			}
		}
	}
	return false
}

// validateCustomClaims validates custom claims specific to Pay Theory
func (v *JWTValidator) validateCustomClaims(claims *JWTClaims) error {
	// Validate tenant ID if required
	if v.config.RequireTenantID && claims.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}

	// Custom tenant validation
	if v.config.ValidateTenant != nil && claims.TenantID != "" {
		if err := v.config.ValidateTenant(claims.TenantID); err != nil {
			return fmt.Errorf("tenant validation failed: %w", err)
		}
	}

	// Validate subject (user ID)
	if claims.Subject == "" {
		return fmt.Errorf("subject (user_id) is required")
	}

	return nil
}

// JWT creates JWT authentication middleware
func JWT(config security.JWTConfig) lift.Middleware {
	validator, err := NewJWTValidator(config)
	if err != nil {
		// Return a middleware that always returns an error
		return func(_ lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(_ *lift.Context) error {
				return lift.SystemError("JWT middleware configuration error").
					WithDetail("error", "Failed to initialize JWT validator").
					WithDetail("cause", err.Error()).
					WithStackTrace()
			})
		}
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Create security context wrapper
			secCtx := lift.WithSecurity(ctx)

			// Extract token from Authorization header
			token := extractBearerToken(ctx)
			if token == "" {
				return lift.Unauthorized("Missing or invalid authorization token")
			}

			// Validate token
			claims, err := validator.ValidateToken(token)
			if err != nil {
				// Log detailed error internally but return generic message
				if ctx.Logger != nil {
					ctx.Logger.Error("Token validation failed", map[string]any{
						"error":      err.Error(),
						"error_type": fmt.Sprintf("%T", err),
					})
				}
				return lift.Unauthorized("Invalid or expired token")
			}

			// Multi-tenant validation
			if config.RequireTenantID && claims.TenantID == "" {
				return lift.AuthorizationError("Tenant ID is required")
			}

			// Create principal from claims
			principal := createPrincipalFromClaims(claims, ctx)

			// Set principal in security context
			secCtx.SetPrincipal(principal)

			// Add authentication info to logger
			if ctx.Logger != nil {
				ctx.Logger = ctx.Logger.WithField("user_id", principal.UserID).
					WithField("tenant_id", principal.TenantID).
					WithField("auth_method", "jwt")
			}

			return next.Handle(ctx)
		})
	}
}

// JWTOptional creates optional JWT authentication middleware
// If no token is provided, continues with anonymous principal
func JWTOptional(config security.JWTConfig) lift.Middleware {
	validator, err := NewJWTValidator(config)
	if err != nil {
		// Return a middleware that always returns an error
		return func(_ lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(_ *lift.Context) error {
				return lift.SystemError("JWT optional middleware configuration error").
					WithDetail("error", "Failed to initialize JWT validator").
					WithDetail("cause", err.Error()).
					WithStackTrace()
			})
		}
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Create security context wrapper
			secCtx := lift.WithSecurity(ctx)

			// Extract token from Authorization header
			token := extractBearerToken(ctx)

			var principal *security.Principal

			if token == "" {
				// No token provided, use anonymous principal
				principal = security.AnonymousPrincipal()
			} else {
				// Validate token
				claims, err := validator.ValidateToken(token)
				if err != nil {
					// Invalid token, use anonymous principal
					principal = security.AnonymousPrincipal()
				} else {
					// Valid token, create principal from claims
					principal = createPrincipalFromClaims(claims, ctx)
				}
			}

			// Set principal in security context
			secCtx.SetPrincipal(principal)

			// Add authentication info to logger
			if ctx.Logger != nil {
				ctx.Logger = ctx.Logger.WithField("user_id", principal.UserID).
					WithField("tenant_id", principal.TenantID).
					WithField("auth_method", principal.AuthMethod)
			}

			return next.Handle(ctx)
		})
	}
}

// RequireRole creates middleware that requires specific roles
func RequireRole(roles ...string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			secCtx := lift.WithSecurity(ctx)
			principal := secCtx.GetPrincipal()
			if principal == nil {
				return lift.Unauthorized("Authentication required")
			}

			if !principal.HasAnyRole(roles...) {
				// Log required roles internally but return generic message
				if ctx.Logger != nil {
					ctx.Logger.Warn("Authorization failed: missing required roles", map[string]any{
						"required_roles": roles,
						"user_roles":     principal.Roles,
						"user_id":        principal.UserID,
					})
				}
				return lift.AuthorizationError("Insufficient permissions")
			}

			return next.Handle(ctx)
		})
	}
}

// RequireScope creates middleware that requires specific scopes
func RequireScope(scopes ...string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			secCtx := lift.WithSecurity(ctx)
			principal := secCtx.GetPrincipal()
			if principal == nil {
				return lift.Unauthorized("Authentication required")
			}

			for _, scope := range scopes {
				if !principal.HasScope(scope) {
					// Log required scope internally but return generic message
					if ctx.Logger != nil {
						ctx.Logger.Warn("Authorization failed: missing required scope", map[string]any{
							"required_scope": scope,
							"user_scopes":    principal.Scopes,
							"user_id":        principal.UserID,
						})
					}
					return lift.AuthorizationError("Insufficient permissions")
				}
			}

			return next.Handle(ctx)
		})
	}
}

// RequireTenant creates middleware that validates tenant access
func RequireTenant(tenantID string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			secCtx := lift.WithSecurity(ctx)
			principal := secCtx.GetPrincipal()
			if principal == nil {
				return lift.Unauthorized("Authentication required")
			}

			if !principal.IsValidForTenant(tenantID) {
				return lift.AuthorizationError("Access denied for this tenant")
			}

			return next.Handle(ctx)
		})
	}
}

// extractBearerToken extracts the bearer token from the Authorization header
func extractBearerToken(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		return ""
	}

	// Check for Bearer token
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return ""
	}

	return strings.TrimPrefix(authHeader, bearerPrefix)
}

// createPrincipalFromClaims creates a Principal from JWT claims
func createPrincipalFromClaims(claims *JWTClaims, ctx *lift.Context) *security.Principal {
	var expiresAt time.Time
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	var issuedAt time.Time
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	} else {
		issuedAt = time.Now()
	}

	return &security.Principal{
		UserID:     claims.Subject,
		TenantID:   claims.TenantID,
		AccountID:  claims.AccountID,
		Roles:      claims.Roles,
		Scopes:     claims.Scopes,
		AuthMethod: "jwt",
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
		IPAddress:  ctx.Header("X-Real-IP"),
		UserAgent:  ctx.Header("User-Agent"),
		RequestID:  ctx.RequestID,
	}
}

// loadRSAPublicKey loads an RSA public key from a PEM file
func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	// Validate file path to prevent directory traversal
	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid key file path")
	}

	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA public key")
	}

	return rsaPub, nil
}
