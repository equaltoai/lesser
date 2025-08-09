package auth

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DynamORMCSRFStore implements CSRFStore using DynamORM patterns
type DynamORMCSRFStore struct {
	repo *repositories.CSRFRepository
}

// NewDynamORMCSRFStore creates a new DynamORM-backed CSRF store
func NewDynamORMCSRFStore(db core.DB, tableName string, logger *zap.Logger) *DynamORMCSRFStore {
	return &DynamORMCSRFStore{
		repo: repositories.NewCSRFRepository(db, tableName, logger),
	}
}

// Store saves a CSRF token with expiration
func (s *DynamORMCSRFStore) Store(token string, csrf CSRFToken) error {
	ctx := context.Background()
	return s.repo.Store(ctx, token, csrf.UserID, csrf.ExpiresAt)
}

// Get retrieves a CSRF token
func (s *DynamORMCSRFStore) Get(token string) (*CSRFToken, error) {
	ctx := context.Background()
	retrievedToken, userID, expiresAt, valid, err := s.repo.Get(ctx, token)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, ErrInvalidCSRF
	}

	return &CSRFToken{
		Token:     retrievedToken,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}, nil
}

// Delete removes a CSRF token
func (s *DynamORMCSRFStore) Delete(token string) error {
	ctx := context.Background()
	return s.repo.Delete(ctx, token)
}

// CleanExpired removes expired tokens (DynamoDB TTL handles this automatically)
func (s *DynamORMCSRFStore) CleanExpired() error {
	ctx := context.Background()
	return s.repo.CleanExpired(ctx)
}

// ValidateAndConsume validates a token and marks it as used atomically
// This method provides the atomic operation that the legacy DynamoDB store had
func (s *DynamORMCSRFStore) ValidateAndConsume(token string, userID string) error {
	ctx := context.Background()
	err := s.repo.ValidateAndConsume(ctx, token, userID)
	if err != nil {
		// Map repository errors to auth errors
		if err.Error() == "invalid CSRF token" {
			return ErrInvalidCSRF
		}
		if err.Error() == "expired CSRF token" {
			return ErrExpiredCSRF
		}
		return fmt.Errorf("CSRF validation failed: %w", err)
	}
	return nil
}

// GetUserActiveTokenCount returns the number of active tokens for a user
// This is used for rate limiting to prevent DoS attacks
func (s *DynamORMCSRFStore) GetUserActiveTokenCount(userID string) (int, error) {
	ctx := context.Background()
	return s.repo.GetUserActiveTokenCount(ctx, userID)
}

// CleanupUserTokens removes old/used tokens for a user
// This is called automatically when a user hits the token limit
func (s *DynamORMCSRFStore) CleanupUserTokens(userID string) error {
	ctx := context.Background()
	return s.repo.CleanupUserTokens(ctx, userID)
}
