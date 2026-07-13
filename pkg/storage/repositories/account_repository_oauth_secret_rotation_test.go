package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestAccountRepository_RotateOAuthClientSecret_Success(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	repo := &AccountRepository{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	ctx := context.Background()
	rotatedAt := time.Date(2026, time.March, 19, 15, 4, 0, 0, time.UTC)
	graceExpiresAt := rotatedAt.Add(24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#client-1").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", oauthClientSortKey).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("Set", "ClientSecret", "hash-new").Return(mockUpdate)
	mockUpdate.On("Set", "PreviousClientSecret", "hash-old").Return(mockUpdate)
	mockUpdate.On("Set", "PreviousClientSecretGraceExpiresAt", graceExpiresAt).Return(mockUpdate)
	mockUpdate.On("Set", "SecretRotatedAt", rotatedAt).Return(mockUpdate)
	mockUpdate.On("Set", "SecretRotatedBy", "owner").Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(nil)

	err := repo.RotateOAuthClientSecret(ctx, "client-1", storage.OAuthClientSecretRotation{
		ActiveClientSecretHash:             "hash-new",
		PreviousClientSecretHash:           "hash-old",
		PreviousClientSecretGraceExpiresAt: graceExpiresAt,
		RotatedAt:                          rotatedAt,
		RotatedBy:                          "owner",
	})
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func TestAccountRepository_GetOAuthClient_MapsSecretRotationState(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &AccountRepository{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	ctx := context.Background()
	rotatedAt := time.Date(2026, time.March, 19, 15, 4, 0, 0, time.UTC)
	graceExpiresAt := rotatedAt.Add(24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#client-1").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", oauthClientSortKey).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.OAuthClient)
		model.ClientID = "client-1"
		model.ClientSecret = "hash-new"
		model.PreviousClientSecret = "hash-old"
		model.PreviousClientSecretGraceExpiresAt = graceExpiresAt
		model.SecretRotatedAt = rotatedAt
		model.SecretRotatedBy = "owner"
		model.Name = "Agent Connector"
		model.RedirectURIs = []string{"https://example.com/callback"}
	}).Return(nil)

	client, err := repo.GetOAuthClient(ctx, "client-1")
	require.NoError(t, err)
	require.Equal(t, "hash-new", client.ClientSecretHash)
	require.Equal(t, "hash-old", client.PreviousClientSecretHash)
	require.Equal(t, graceExpiresAt, client.PreviousClientSecretGraceExpiresAt)
	require.Equal(t, rotatedAt, client.SecretRotatedAt)
	require.Equal(t, "owner", client.SecretRotatedBy)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_RotateOAuthClientSecret_NormalizesZeroRotationState(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	repo := &AccountRepository{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#client-1").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", oauthClientSortKey).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("Set", "ClientSecret", "hash-new").Return(mockUpdate)
	mockUpdate.On("Set", "PreviousClientSecret", "").Return(mockUpdate)
	mockUpdate.On("Set", "PreviousClientSecretGraceExpiresAt", time.Time{}).Return(mockUpdate)
	mockUpdate.On("Set", "SecretRotatedAt", mock.AnythingOfType("time.Time")).Return(mockUpdate)
	mockUpdate.On("Set", "SecretRotatedBy", "owner").Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.AnythingOfType("time.Time")).Return(mockUpdate)
	mockUpdate.On("Execute").Return(nil)

	err := repo.RotateOAuthClientSecret(ctx, "client-1", storage.OAuthClientSecretRotation{
		ActiveClientSecretHash:             "hash-new",
		PreviousClientSecretGraceExpiresAt: time.Now().Add(24 * time.Hour),
		RotatedBy:                          " owner ",
	})
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}
