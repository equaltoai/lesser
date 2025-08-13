package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// CSRF-related errors
var (
	ErrInvalidCSRF = errors.New("invalid CSRF token")
	ErrExpiredCSRF = errors.New("expired CSRF token")
	ErrMissingCSRF = errors.New("missing CSRF token")
)

// CSRFToken represents a CSRF token with metadata
type CSRFToken struct {
	Token     string
	ExpiresAt time.Time
	UserID    string
}

// CSRFStore interface for token storage
type CSRFStore interface {
	Store(token string, csrf CSRFToken) error
	Get(token string) (*CSRFToken, error)
	Delete(token string) error
	CleanExpired() error
}

// MemoryCSRFStore is an in-memory implementation (use DynamoDB in production)
type MemoryCSRFStore struct {
	mu     sync.RWMutex
	tokens map[string]CSRFToken
}

// NewMemoryCSRFStore creates a new in-memory CSRF store
func NewMemoryCSRFStore() *MemoryCSRFStore {
	store := &MemoryCSRFStore{
		tokens: make(map[string]CSRFToken),
	}

	// Note: Removed polling goroutine - cleanup is now handled manually or by TTL in production stores

	return store
}

// Store saves a CSRF token
func (s *MemoryCSRFStore) Store(token string, csrf CSRFToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = csrf
	return nil
}

// Get retrieves a CSRF token
func (s *MemoryCSRFStore) Get(token string) (*CSRFToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	csrf, exists := s.tokens[token]
	if !exists {
		return nil, ErrInvalidCSRF
	}

	if time.Now().After(csrf.ExpiresAt) {
		return nil, ErrExpiredCSRF
	}

	return &csrf, nil
}

// Delete removes a CSRF token
func (s *MemoryCSRFStore) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

// CleanExpired removes expired tokens
func (s *MemoryCSRFStore) CleanExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, csrf := range s.tokens {
		if now.After(csrf.ExpiresAt) {
			delete(s.tokens, token)
		}
	}
	return nil
}

// CSRFManager handles CSRF token operations
type CSRFManager struct {
	store CSRFStore
}

// NewCSRFManager creates a new CSRF manager
func NewCSRFManager(store CSRFStore) *CSRFManager {
	return &CSRFManager{store: store}
}

// GenerateToken creates a new CSRF token for a user
func (m *CSRFManager) GenerateToken(userID string) (string, error) {
	// Generate secure random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(b)

	// Store with expiration
	csrf := CSRFToken{
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		UserID:    userID,
	}

	if err := m.store.Store(token, csrf); err != nil {
		return "", err
	}

	return token, nil
}

// ValidateToken checks if a CSRF token is valid for a user
func (m *CSRFManager) ValidateToken(token string, userID string) error {
	if token == "" {
		return ErrMissingCSRF
	}

	// For DynamORM store, use ValidateAndConsume for atomic operation
	if dynamoStore, ok := m.store.(*DynamORMCSRFStore); ok {
		return dynamoStore.ValidateAndConsume(token, userID)
	}

	// Fallback for in-memory store (testing)
	csrf, err := m.store.Get(token)
	if err != nil {
		return err
	}

	// Verify user matches
	if csrf.UserID != userID {
		return ErrInvalidCSRF
	}

	// Single use - delete after validation
	_ = m.store.Delete(token)

	return nil
}

// CSRFMiddleware creates middleware for CSRF protection
func CSRFMiddleware(manager *CSRFManager) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Skip for safe methods
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next(w, r)
				return
			}

			// Extract CSRF token from header or form
			csrfToken := r.Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				csrfToken = r.FormValue("csrf_token")
			}

			if csrfToken == "" {
				// Log CSRF failure
				claims, _ := GetClaims(r.Context())
				userID := ""
				if claims != nil {
					userID = claims.Username
				}
				common.LogCSRFFailure("missing_token", userID, r.RemoteAddr, r.URL.Path)

				http.Error(w, "Missing CSRF token", http.StatusForbidden)
				return
			}

			// Get claims from context (set by auth middleware)
			claims, ok := GetClaims(r.Context())
			if !ok || claims == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Validate token
			if err := manager.ValidateToken(csrfToken, claims.Username); err != nil {
				// Log CSRF failure
				reason := "invalid_token"
				if err == ErrExpiredCSRF {
					reason = "expired_token"
				}
				common.LogCSRFFailure(reason, claims.Username, r.RemoteAddr, r.URL.Path)

				if err == ErrExpiredCSRF {
					http.Error(w, "CSRF token expired", http.StatusForbidden)
				} else {
					http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				}
				return
			}

			next(w, r)
		}
	}
}

// GenerateCSRFTokenHandler creates an endpoint to get CSRF tokens
func GenerateCSRFTokenHandler(manager *CSRFManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get claims from context
		claims, ok := GetClaims(r.Context())
		if !ok || claims == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Generate token
		token, err := manager.GenerateToken(claims.Username)
		if err != nil {
			statusCode, message := common.HandleError(nil, common.ErrInternal(err))
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(message))
			return
		}

		// Return token
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"csrf_token": "` + token + `"}`))
	}
}

// NewCSRFManagerWithDynamORM creates a CSRF manager with DynamORM store
func NewCSRFManagerWithDynamORM(db core.DB, tableName string, logger *zap.Logger) *CSRFManager {
	store := NewDynamORMCSRFStore(db, tableName, logger)
	return &CSRFManager{store: store}
}
