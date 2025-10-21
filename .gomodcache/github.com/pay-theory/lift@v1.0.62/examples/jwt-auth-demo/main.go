package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

func main() {
	// Get JWT secret from environment or generate a secure one
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// In production, this should fail or use a secure secret management service
		fmt.Println("WARNING: JWT_SECRET not set. Using a default value for demo purposes only.")
		jwtSecret = "demo-secret-key-change-in-production"
	}

	// Create app and add JWT authentication middleware
	app := lift.New(
		lift.WithSecurityMiddleware(lift.SecurityConfig{
			EnableSecurityHeaders: true,
			AuditLogger: func(_ *lift.Context, event string, data map[string]any) {
				fmt.Printf("Audit: %s - %v\n", event, data)
			},
		}),
	)

	app.Use(middleware.JWTAuth(middleware.JWTConfig{
		Secret:      jwtSecret,
		Algorithm:   "HS256",
		TokenLookup: "header:Authorization",
		SkipPaths:   []string{"/health", "/login"},
		Validator: func(claims jwt.MapClaims) error {
			// Validate token expiration
			if exp, ok := claims["exp"].(float64); ok {
				if time.Now().Unix() > int64(exp) {
					return fmt.Errorf("token expired")
				}
			}
			return nil
		},
	}))

	// Small helpers to cut down repetitive error checks
	mustGET := func(path string, h func(*lift.Context) error) {
		if err := app.GET(path, h); err != nil {
			log.Fatalf("Failed to register GET %s: %v", path, err)
		}
	}
	mustPOST := func(path string, h func(*lift.Context) error) {
		if err := app.POST(path, h); err != nil {
			log.Fatalf("Failed to register POST %s: %v", path, err)
		}
	}

	// Public endpoints
	mustGET("/health", func(ctx *lift.Context) error {
		return ctx.OK(map[string]string{"status": "healthy"})
	})

	mustPOST("/login", func(ctx *lift.Context) error {
		// In real app, validate credentials
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := ctx.ParseRequest(&req); err != nil {
			return err
		}

		// Create JWT token
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":   "user-123",
			"tenant_id": "tenant-456",
			"username":  req.Username,
			"roles":     []string{"user", "admin"},
			"exp":       time.Now().Add(24 * time.Hour).Unix(),
		})

		tokenString, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			return ctx.SystemError("Failed to create token", err)
		}

		return ctx.OK(map[string]string{
			"token": tokenString,
		})
	})

	// Protected endpoints
	mustGET("/me", func(ctx *lift.Context) error {
		// Access JWT claims
		claims := ctx.Claims()
		if claims == nil {
			return lift.Unauthorized("No claims found")
		}

		return ctx.OK(map[string]any{
			"user_id":          ctx.UserID(),
			"tenant_id":        ctx.TenantID(),
			"username":         claims["username"],
			"roles":            claims["roles"],
			"is_authenticated": ctx.IsAuthenticated(),
		})
	})

	mustGET("/admin", func(ctx *lift.Context) error {
		// Check for admin role
		claims := ctx.Claims()
		if claims == nil {
			return lift.Unauthorized("Authentication required")
		}

		roles, ok := claims["roles"].([]any)
		if !ok {
			return lift.AuthorizationError("No roles found")
		}

		hasAdmin := false
		for _, role := range roles {
			if role == "admin" {
				hasAdmin = true
				break
			}
		}

		if !hasAdmin {
			return lift.AuthorizationError("Admin role required")
		}

		return ctx.OK(map[string]string{
			"message": "Welcome admin!",
			"user_id": ctx.UserID(),
		})
	})

	// Multi-tenant endpoint
	mustGET("/tenant/:tenantId/data", func(ctx *lift.Context) error {
		requestedTenant := ctx.Param("tenantId")
		userTenant := ctx.TenantID()

		if userTenant != requestedTenant {
			return ctx.Forbidden("Cannot access other tenant's data", nil)
		}

		return ctx.OK(map[string]any{
			"tenant_id": requestedTenant,
			"data":      "Tenant specific data",
			"user_id":   ctx.UserID(),
		})
	})

	// In Lambda, you would start with:
	// lambda.Start(app.HandleRequest)
}
