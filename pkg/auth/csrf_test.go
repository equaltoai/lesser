package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCSRFTokenGeneration(t *testing.T) {
	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)

	// Generate token for user
	userID := "test-user-123"
	token, err := manager.GenerateToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Verify token is not empty
	if token == "" {
		t.Error("Generated token is empty")
	}

	// Verify token can be retrieved
	stored, err := store.Get(token)
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if stored.UserID != userID {
		t.Errorf("Expected userID %s, got %s", userID, stored.UserID)
	}
}

func TestCSRFTokenValidation(t *testing.T) {
	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)

	tests := []struct {
		name      string
		setupFunc func() (string, string) // returns token, userID
		expectErr bool
	}{
		{
			name: "valid token",
			setupFunc: func() (string, string) {
				userID := "user1"
				token, _ := manager.GenerateToken(userID)
				return token, userID
			},
			expectErr: false,
		},
		{
			name: "invalid token",
			setupFunc: func() (string, string) {
				return "invalid-token", "user1"
			},
			expectErr: true,
		},
		{
			name: "wrong user",
			setupFunc: func() (string, string) {
				token, _ := manager.GenerateToken("user1")
				return token, "user2"
			},
			expectErr: true,
		},
		{
			name: "empty token",
			setupFunc: func() (string, string) {
				return "", "user1"
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, userID := tt.setupFunc()
			err := manager.ValidateToken(token, userID)

			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateToken() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestCSRFTokenSingleUse(t *testing.T) {
	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)

	userID := "user1"
	token, err := manager.GenerateToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// First validation should succeed
	err = manager.ValidateToken(token, userID)
	if err != nil {
		t.Errorf("First validation failed: %v", err)
	}

	// Second validation should fail (single use)
	err = manager.ValidateToken(token, userID)
	if err == nil {
		t.Error("Second validation succeeded, expected failure")
	}
}

func TestCSRFTokenExpiration(t *testing.T) {
	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)

	userID := "user1"
	token, _ := manager.GenerateToken(userID)

	// Manually expire the token
	csrf, _ := store.Get(token)
	csrf.ExpiresAt = time.Now().Add(-1 * time.Hour)
	_ = store.Store(token, *csrf)

	// Validation should fail
	err := manager.ValidateToken(token, userID)
	if err != ErrExpiredCSRF {
		t.Errorf("Expected ErrExpiredCSRF, got %v", err)
	}
}

func TestCSRFMiddleware(t *testing.T) {
	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)
	middleware := CSRFMiddleware(manager)

	// Mock handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Success"))
	})

	tests := []struct {
		name           string
		method         string
		setupRequest   func(*http.Request)
		expectedStatus int
	}{
		{
			name:   "GET request bypasses CSRF",
			method: "GET",
			setupRequest: func(r *http.Request) {
				// No CSRF token needed
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "POST without token",
			method: "POST",
			setupRequest: func(r *http.Request) {
				// Set claims context but no CSRF token
				claims := &Claims{Username: "user1"}
				ctx := WithClaims(r.Context(), claims)
				*r = *r.WithContext(ctx)
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:   "POST with valid token",
			method: "POST",
			setupRequest: func(r *http.Request) {
				// Set claims context
				claims := &Claims{Username: "user1"}
				ctx := WithClaims(r.Context(), claims)
				*r = *r.WithContext(ctx)

				// Generate and set CSRF token
				token, _ := manager.GenerateToken(claims.Username)
				r.Header.Set("X-CSRF-Token", token)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			tt.setupRequest(req)

			rec := httptest.NewRecorder()
			middleware(handler).ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

func TestMemoryCSRFStoreCleanup(t *testing.T) {
	store := &MemoryCSRFStore{
		tokens: make(map[string]CSRFToken),
	}

	// Add expired token
	expiredToken := CSRFToken{
		Token:     "expired",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		UserID:    "user1",
	}
	_ = store.Store("expired", expiredToken)

	// Add valid token
	validToken := CSRFToken{
		Token:     "valid",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		UserID:    "user2",
	}
	_ = store.Store("valid", validToken)

	// Clean expired
	err := store.CleanExpired()
	if err != nil {
		t.Errorf("CleanExpired failed: %v", err)
	}

	// Expired token should be gone
	_, err = store.Get("expired")
	if err != ErrInvalidCSRF {
		t.Error("Expired token still exists")
	}

	// Valid token should remain
	_, err = store.Get("valid")
	if err != nil {
		t.Error("Valid token was removed")
	}
}
