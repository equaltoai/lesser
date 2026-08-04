package auth

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

var (
	// ErrTokenReuse indicates a refresh token was reused (security breach)
	ErrTokenReuse = errors.New("refresh token reuse detected")
	// ErrExpiredRefreshToken indicates the refresh token has expired
	ErrExpiredRefreshToken = errors.New("refresh token expired")
)

// RefreshToken represents a refresh token with rotation support
// This is an alias to the models.AuthRefreshToken for backward compatibility
type RefreshToken = models.AuthRefreshToken

// RefreshTokenStore manages refresh tokens using DynamORM
type RefreshTokenStore struct {
	repo   authRefreshTokenRepository
	logger *zap.Logger
}

type authRefreshTokenRepository interface {
	CreateRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*RefreshToken, error)
	GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldToken string, ipAddress string) (*RefreshToken, error)
	RevokeTokenFamily(ctx context.Context, family string, reason string) error
	RevokeUserTokens(ctx context.Context, userID string, reason string) error
	GetTokensByUser(ctx context.Context, userID string) ([]RefreshToken, error)
	GetTokensByFamily(ctx context.Context, family string) ([]RefreshToken, error)
}

// NewRefreshTokenStore creates a new refresh token store
func NewRefreshTokenStore(db core.DB) *RefreshTokenStore {
	return &RefreshTokenStore{
		repo:   repositories.NewAuthRefreshTokenRepository(db, "auth_tokens", common.Logger(), nil),
		logger: common.Logger(),
	}
}

// NewRefreshTokenStoreFromRepo creates a new refresh token store from repository
func NewRefreshTokenStoreFromRepo(repo *repositories.AuthRefreshTokenRepository) *RefreshTokenStore {
	return &RefreshTokenStore{
		repo:   repo,
		logger: common.Logger(),
	}
}

// CreateRefreshToken generates a new refresh token
func (s *RefreshTokenStore) CreateRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*RefreshToken, error) {
	return s.repo.CreateRefreshToken(ctx, userID, deviceName, ipAddress)
}

// GetRefreshToken retrieves a refresh token
func (s *RefreshTokenStore) GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error) {
	refreshToken, err := s.repo.GetRefreshToken(ctx, token)
	if err != nil {
		// Map repository errors to auth package errors
		if err.Error() == "invalid refresh token" {
			return nil, ErrInvalidRefreshToken
		}
		if err.Error() == "refresh token expired" {
			return nil, ErrExpiredRefreshToken
		}
		return nil, err
	}
	return refreshToken, nil
}

// RotateRefreshToken implements secure rotation with reuse detection
func (s *RefreshTokenStore) RotateRefreshToken(ctx context.Context, oldToken string, ipAddress string) (*RefreshToken, error) {
	newToken, err := s.repo.RotateRefreshToken(ctx, oldToken, ipAddress)
	if err != nil {
		// Map repository errors to auth package errors
		if err.Error() == "refresh token reuse detected" {
			return nil, ErrTokenReuse
		}
		return nil, err
	}
	return newToken, nil
}

// RevokeTokenFamily revokes all tokens in a family (security breach response)
func (s *RefreshTokenStore) RevokeTokenFamily(ctx context.Context, family string, reason string) error {
	return s.repo.RevokeTokenFamily(ctx, family, reason)
}

// RevokeUserTokens revokes all tokens for a user (logout all devices)
func (s *RefreshTokenStore) RevokeUserTokens(ctx context.Context, userID string, reason string) error {
	return s.repo.RevokeUserTokens(ctx, userID, reason)
}

// GetTokensByUser retrieves all active tokens for a user
func (s *RefreshTokenStore) GetTokensByUser(ctx context.Context, userID string) ([]RefreshToken, error) {
	return s.repo.GetTokensByUser(ctx, userID)
}

// GetTokensByFamily retrieves all tokens in a family
func (s *RefreshTokenStore) GetTokensByFamily(ctx context.Context, family string) ([]RefreshToken, error) {
	return s.repo.GetTokensByFamily(ctx, family)
}
