package middleware

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/pay-theory/lift/pkg/security"
)

func TestJWTValidator(t *testing.T) {
	config := security.JWTConfig{
		SigningMethod: "HS256",
		// #nosec G101 -- Test secret for unit testing only
		SecretKey: "test-secret",
		Issuer:    "test-issuer",
		Audience:  []string{"test-audience"},
		MaxAge:    time.Hour,
	}

	validator, err := NewJWTValidator(config)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	t.Run("Valid Token", func(t *testing.T) {
		token := createTestToken(t, config, map[string]any{
			"sub": "user123",
		})

		claims, err := validator.ValidateToken(token)
		if err != nil {
			t.Errorf("Expected valid token, got error: %v", err)
		}

		if claims.Subject != "user123" {
			t.Errorf("Expected subject 'user123', got %s", claims.Subject)
		}
	})

	t.Run("Invalid Signature", func(t *testing.T) {
		// Create token with different secret
		wrongConfig := config
		// #nosec G101 -- Test secret for unit testing only
		wrongConfig.SecretKey = "wrong-secret"
		token := createTestToken(t, wrongConfig, map[string]any{
			"sub": "user123",
		})

		_, err := validator.ValidateToken(token)
		if err == nil {
			t.Error("Expected error for invalid signature")
		}
	})

	t.Run("Malformed Token", func(t *testing.T) {
		_, err := validator.ValidateToken("not.a.jwt")
		if err == nil {
			t.Error("Expected error for malformed token")
		}
	})

	t.Run("Expired Token", func(t *testing.T) {
		token := createTestToken(t, config, map[string]any{
			"sub": "user123",
			"exp": time.Now().Add(-time.Hour).Unix(), // Expired
		})

		_, err := validator.ValidateToken(token)
		if err == nil {
			t.Error("Expected error for expired token")
		}
	})

	t.Run("Wrong Issuer", func(t *testing.T) {
		token := createTestToken(t, config, map[string]any{
			"sub": "user123",
			"iss": "wrong-issuer",
		})

		_, err := validator.ValidateToken(token)
		if err == nil {
			t.Error("Expected error for wrong issuer")
		}
	})

	t.Run("Wrong Audience", func(t *testing.T) {
		token := createTestToken(t, config, map[string]any{
			"sub": "user123",
			"aud": []string{"wrong-audience"},
		})

		_, err := validator.ValidateToken(token)
		if err == nil {
			t.Error("Expected error for wrong audience")
		}
	})
}

func TestExtractBearerToken(t *testing.T) {
	t.Run("Valid Bearer Token", func(t *testing.T) {
		// Create a proper request using the NewRequest constructor
		adapterReq := &adapters.Request{
			Headers: map[string]string{
				"Authorization": "Bearer test-token-123",
			},
		}
		req := lift.NewRequest(adapterReq)
		ctx := &lift.Context{
			Request: req,
		}

		token := extractBearerToken(ctx)
		if token != "test-token-123" {
			t.Errorf("Expected 'test-token-123', got '%s'", token)
		}
	})

	t.Run("Missing Authorization Header", func(t *testing.T) {
		adapterReq := &adapters.Request{
			Headers: map[string]string{},
		}
		req := lift.NewRequest(adapterReq)
		ctx := &lift.Context{
			Request: req,
		}

		token := extractBearerToken(ctx)
		if token != "" {
			t.Errorf("Expected empty string, got '%s'", token)
		}
	})

	t.Run("Non-Bearer Authorization", func(t *testing.T) {
		adapterReq := &adapters.Request{
			Headers: map[string]string{
				"Authorization": "Basic dXNlcjpwYXNz",
			},
		}
		req := lift.NewRequest(adapterReq)
		ctx := &lift.Context{
			Request: req,
		}

		token := extractBearerToken(ctx)
		if token != "" {
			t.Errorf("Expected empty string, got '%s'", token)
		}
	})
}

// Helper function to create test JWT tokens
func createTestToken(t *testing.T, config security.JWTConfig, claims map[string]any) string {
	// Set default claims
	now := time.Now()
	tokenClaims := jwt.MapClaims{
		"iss": config.Issuer,
		"aud": config.Audience,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}

	// Add custom claims
	for k, v := range claims {
		tokenClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)
	tokenString, err := token.SignedString([]byte(config.SecretKey))
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}

	return tokenString
}
