package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type refreshTokenRepoStub struct {
	createToken *RefreshToken
	getToken    *RefreshToken
	rotateToken *RefreshToken
	tokens      []RefreshToken

	createErr  error
	getErr     error
	rotateErr  error
	familyErr  error
	userErr    error
	userTokErr error
	famTokErr  error
}

func (s *refreshTokenRepoStub) CreateRefreshToken(_ context.Context, _ string, _ string, _ string) (*RefreshToken, error) {
	return s.createToken, s.createErr
}

func (s *refreshTokenRepoStub) GetRefreshToken(_ context.Context, _ string) (*RefreshToken, error) {
	return s.getToken, s.getErr
}

func (s *refreshTokenRepoStub) RotateRefreshToken(_ context.Context, _ string, _ string) (*RefreshToken, error) {
	return s.rotateToken, s.rotateErr
}

func (s *refreshTokenRepoStub) RevokeTokenFamily(_ context.Context, _ string, _ string) error {
	return s.familyErr
}

func (s *refreshTokenRepoStub) RevokeUserTokens(_ context.Context, _ string, _ string) error {
	return s.userErr
}

func (s *refreshTokenRepoStub) GetTokensByUser(_ context.Context, _ string) ([]RefreshToken, error) {
	return s.tokens, s.userTokErr
}

func (s *refreshTokenRepoStub) GetTokensByFamily(_ context.Context, _ string) ([]RefreshToken, error) {
	return s.tokens, s.famTokErr
}

func TestRefreshTokenStore_ErrorMappingAndDelegation(t *testing.T) {
	ctx := context.Background()
	stub := &refreshTokenRepoStub{
		getErr:    errors.New("invalid refresh token"),
		rotateErr: errors.New("refresh token reuse detected"),
	}
	store := &RefreshTokenStore{repo: stub}

	_, err := store.GetRefreshToken(ctx, "t")
	require.ErrorIs(t, err, ErrInvalidRefreshToken)

	stub.getErr = errors.New("refresh token expired")
	_, err = store.GetRefreshToken(ctx, "t")
	require.ErrorIs(t, err, ErrExpiredRefreshToken)

	stub.getErr = errors.New("other")
	_, err = store.GetRefreshToken(ctx, "t")
	require.Error(t, err)

	_, err = store.RotateRefreshToken(ctx, "old", "ip")
	require.ErrorIs(t, err, ErrTokenReuse)

	stub.rotateErr = errors.New("other")
	_, err = store.RotateRefreshToken(ctx, "old", "ip")
	require.Error(t, err)

	nowToken := models.AuthRefreshToken{Token: "new"}
	stub.rotateErr = nil
	stub.rotateToken = &nowToken
	out, err := store.RotateRefreshToken(ctx, "old", "ip")
	require.NoError(t, err)
	assert.Equal(t, "new", out.Token)

	stub.familyErr = nil
	require.NoError(t, store.RevokeTokenFamily(ctx, "fam", "reason"))

	// Straight delegation methods.
	token := models.AuthRefreshToken{Token: "created"}
	stub.createToken = &token
	out2, err := store.CreateRefreshToken(ctx, "user", "device", "ip")
	require.NoError(t, err)
	require.Equal(t, "created", out2.Token)

	stub.getErr = nil
	stub.getToken = &models.AuthRefreshToken{Token: "ok"}
	out3, err := store.GetRefreshToken(ctx, "t")
	require.NoError(t, err)
	require.Equal(t, "ok", out3.Token)

	stub.userErr = nil
	require.NoError(t, store.RevokeUserTokens(ctx, "user", "reason"))

	stub.tokens = []RefreshToken{{Token: "t1"}}
	byUser, err := store.GetTokensByUser(ctx, "user")
	require.NoError(t, err)
	require.Len(t, byUser, 1)

	byFamily, err := store.GetTokensByFamily(ctx, "family")
	require.NoError(t, err)
	require.Len(t, byFamily, 1)
}
