package lift

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
)

// JWTConfig represents the configuration for JWT middleware
type JWTConfig struct {
	// Secret key for JWT validation
	SecretKey string

	// JWT token lookup places (default: "header:Authorization")
	// Format: "header:<name>" or "query:<name>"
	TokenLookup string

	// JWT token prefix (default: "Bearer")
	TokenPrefix string

	// Required claims
	RequiredClaims []string

	// Require tenant ID in claims
	RequireTenantID bool

	// Tenant ID claim key (default: "tenant_id")
	TenantIDClaim string

	// User ID claim key (default: "sub")
	UserIDClaim string
}

// DefaultJWTConfig returns the default JWT configuration
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		TokenLookup:     "header:Authorization",
		TokenPrefix:     "Bearer",
		RequiredClaims:  []string{},
		RequireTenantID: false,
		TenantIDClaim:   "tenant_id",
		UserIDClaim:     "sub",
	}
}

// JWT creates a middleware that validates JWT tokens
func JWT(config JWTConfig) Middleware {
	// Use default config if not provided
	if config.TokenLookup == "" {
		config.TokenLookup = "header:Authorization"
	}
	if config.TokenPrefix == "" {
		config.TokenPrefix = "Bearer"
	}
	if config.TenantIDClaim == "" {
		config.TenantIDClaim = "tenant_id"
	}
	if config.UserIDClaim == "" {
		config.UserIDClaim = "sub"
	}

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			// Extract token
			token, err := extractToken(ctx, config.TokenLookup, config.TokenPrefix)
			if err != nil {
				return Unauthorized(err.Error())
			}

			// Parse token
			claims := jwt.MapClaims{}
			parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}

				return []byte(config.SecretKey), nil
			})

			if err != nil {
				return Unauthorized("Invalid token: " + err.Error())
			}

			if !parsedToken.Valid {
				return Unauthorized("Invalid token")
			}

			// Validate required claims
			for _, claim := range config.RequiredClaims {
				if _, ok := claims[claim]; !ok {
					return Unauthorized(fmt.Sprintf("Missing required claim: %s", claim))
				}
			}

			// Validate tenant ID if required
			if config.RequireTenantID {
				tenantID, ok := claims[config.TenantIDClaim].(string)
				if !ok || tenantID == "" {
					return Unauthorized("Missing tenant ID in token")
				}

				// Set tenant ID in context
				ctx.SetTenantID(tenantID)
			}

			// Set user ID in context
			if userID, ok := claims[config.UserIDClaim].(string); ok {
				ctx.SetUserID(userID)
			}

			// Set claims in context
			ctx.Set("jwt_claims", claims)
			ctx.Set("jwt_token", parsedToken)

			// Call next handler
			return next.Handle(ctx)
		})
	}
}

// extractToken extracts the token from the request
func extractToken(ctx *Context, tokenLookup, tokenPrefix string) (string, error) {
	parts := strings.Split(tokenLookup, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token lookup format: %s", tokenLookup)
	}

	var tokenString string

	switch parts[0] {
	case "header":
		// Get token from header
		authHeader := ctx.Header(parts[1])
		if authHeader == "" {
			return "", fmt.Errorf("missing auth header: %s", parts[1])
		}

		// Remove token prefix
		if tokenPrefix != "" {
			if !strings.HasPrefix(authHeader, tokenPrefix) {
				return "", fmt.Errorf("invalid token format")
			}
			tokenString = strings.TrimPrefix(authHeader, tokenPrefix)
			tokenString = strings.TrimSpace(tokenString)
		} else {
			tokenString = authHeader
		}

	case "query":
		// Get token from query parameter
		tokenString = ctx.Query(parts[1])
		if tokenString == "" {
			return "", fmt.Errorf("missing auth query parameter: %s", parts[1])
		}

	default:
		return "", fmt.Errorf("unsupported token lookup: %s", parts[0])
	}

	if tokenString == "" {
		return "", fmt.Errorf("empty token")
	}

	return tokenString, nil
}
