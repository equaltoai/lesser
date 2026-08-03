package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
)

func TestRound08_AccountRepositoryAuth_RecoveryCodeNotFoundAndInvalidPK(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("code"), bcrypt.MinCost)
	require.NoError(t, err)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.RecoveryCode)
		usedAt := baseTime.Add(-time.Minute)
		*out = append(*out,
			models.RecoveryCode{
				PK:       "USER#user-1",
				SK:       "RECOVERY_CODE#0",
				Username: "user-1",
				CodeHash: string(hash),
				UsedAt:   &usedAt, // skipped
			},
			models.RecoveryCode{
				PK:       "BAD",
				SK:       "RECOVERY_CODE#1",
				Username: "user-1",
				CodeHash: string(hash), // matches, but invalid PK format triggers continue
				UsedAt:   nil,
			},
		)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, err = repo.GetUserByRecoveryCode(ctx, "code")
	require.Error(t, err)
}

func TestRound08_AccountRepositoryAuth_DeleteRecoveryTokenNotFound(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Delete", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.DeleteRecoveryToken(ctx, "k"))
}

func TestRound08_AccountRepositoryAuth_GetSessionAndWebAuthnErrors(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetSession non-notfound error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetSession(ctx, "sid")
		require.Error(t, err)
	})

	t.Run("UpdateWebAuthnCredential query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateWebAuthnCredential(ctx, "cred-1", 1))
	})
}
