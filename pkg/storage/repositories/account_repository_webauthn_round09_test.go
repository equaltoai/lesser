package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"

	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestRound09_AccountRepository_WebAuthnCredentials(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	cred := &storage.WebAuthnCredential{
		ID:         "cred-1",
		UserID:     "user-1",
		PublicKey:  []byte("pk"),
		SignCount:  1,
		CreatedAt:  baseTime,
		LastUsedAt: baseTime,
		Name:       "key",
	}
	require.NoError(t, repo.CreateWebAuthnCredential(ctx, cred))

	got, err := repo.GetWebAuthnCredential(ctx, cred.ID)
	require.NoError(t, err)
	require.Equal(t, cred.ID, got.ID)

	list, err := repo.GetUserWebAuthnCredentials(ctx, cred.UserID)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	require.NoError(t, repo.DeleteWebAuthnCredential(ctx, cred.ID))

	mockDBNotFound := new(mocks.MockDB)
	mockQueryNotFound := new(mocks.MockQuery)
	mockQueryNotFound.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBNotFound, mockQueryNotFound, nil, baseTime)

	repoNotFound := NewAccountRepository(mockDBNotFound, "test-table", "example.com", zap.NewNop())
	repoNotFound.SetValidationService(nil)
	repoNotFound.SetPermissionService(nil)
	repoNotFound.SetEventService(nil)
	repoNotFound.SetCachingService(nil)
	require.NoError(t, repoNotFound.DeleteWebAuthnCredential(ctx, "missing"))

	mockDBMissing := new(mocks.MockDB)
	mockQueryMissing := new(mocks.MockQuery)
	mockQueryMissing.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBMissing, mockQueryMissing, nil, baseTime)

	repoMissing := NewAccountRepository(mockDBMissing, "test-table", "example.com", zap.NewNop())
	repoMissing.SetValidationService(nil)
	repoMissing.SetPermissionService(nil)
	repoMissing.SetEventService(nil)
	repoMissing.SetCachingService(nil)
	require.Error(t, repoMissing.UpdateWebAuthnLastUsed(ctx, "missing", 10))

	mockDBUpdateErr := new(mocks.MockDB)
	mockQueryUpdateErr := new(mocks.MockQuery)
	mockUpdateBuilderErr := new(mocks.MockUpdateBuilder)
	mockUpdateBuilderErr.On("Execute").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUpdateErr, mockQueryUpdateErr, mockUpdateBuilderErr, baseTime)

	repoUpdateErr := NewAccountRepository(mockDBUpdateErr, "test-table", "example.com", zap.NewNop())
	repoUpdateErr.SetValidationService(nil)
	repoUpdateErr.SetPermissionService(nil)
	repoUpdateErr.SetEventService(nil)
	repoUpdateErr.SetCachingService(nil)
	require.Error(t, repoUpdateErr.UpdateWebAuthnLastUsed(ctx, cred.ID, 2))
}

func TestRound09_AccountRepository_WebAuthnChallenges(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	challenge := &storage.WebAuthnChallenge{
		Challenge:   "challenge-1",
		UserID:      "user-1",
		SessionData: []byte("session"),
		Type:        "authentication",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	require.NoError(t, repo.CreateWebAuthnChallenge(ctx, challenge))

	got, err := repo.GetWebAuthnChallenge(ctx, challenge.Challenge)
	require.NoError(t, err)
	require.Equal(t, challenge.Challenge, got.Challenge)

	require.NoError(t, repo.DeleteWebAuthnChallenge(ctx, challenge.Challenge))

	mockDBExpired := new(mocks.MockDB)
	mockQueryExpired := new(mocks.MockQuery)
	mockQueryExpired.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if target, ok := args.Get(0).(*models.WebAuthnChallenge); ok {
			target.Challenge = "expired"
			target.UserID = "user-1"
			target.Type = "authentication"
			target.SessionData = []byte("s")
			target.ExpiresAt = time.Now().Add(-1 * time.Minute)
		}
	}).Return(nil).Once()
	mockQueryExpired.On("Delete").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBExpired, mockQueryExpired, nil, baseTime)

	repoExpired := NewAccountRepository(mockDBExpired, "test-table", "example.com", zap.NewNop())
	repoExpired.SetValidationService(nil)
	repoExpired.SetPermissionService(nil)
	repoExpired.SetEventService(nil)
	repoExpired.SetCachingService(nil)
	_, err = repoExpired.GetWebAuthnChallenge(ctx, "expired")
	require.Error(t, err)
}
