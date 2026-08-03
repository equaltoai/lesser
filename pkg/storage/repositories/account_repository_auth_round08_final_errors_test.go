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
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepositoryAuth_FinalErrors(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CreatePasswordResetToken create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// CreatePasswordResetToken: db create fails.
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		_, err := repo.CreatePasswordResetToken(ctx, "user-1", "")
		require.Error(t, err)
	})

	t.Run("ResetPassword surfaces UpdateUser failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*models.PasswordReset)
			return ok
		})).Run(func(args mock.Arguments) {
			reset := args.Get(0).(*models.PasswordReset)
			reset.Username = "user-1"
			reset.Token = "tok"
			reset.Email = ""
			reset.ExpiresAt = baseTime.Add(time.Hour)
			reset.Used = false
		}).Return(nil).Once()

		// UpdateUser fails at Get (not found).
		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*models.User)
			return ok
		})).Return(dynamormErrors.ErrItemNotFound).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.Error(t, repo.ResetPassword(ctx, "tok", "new-hash"))
	})

	t.Run("session and device error branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(errors.New("get failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.InvalidateSession(ctx, "user-1", "sid"))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Run(func(args mock.Arguments) {
			s := args.Get(0).(*models.Session)
			s.SessionID = "sid"
			s.UserID = "USER#user-1"
			s.AccessToken = "at"
			s.RefreshToken = "rt"
			s.ExpiresAt = baseTime.Add(time.Hour).Unix()
			s.PK = "session#sid"
			s.SK = "session#sid"
		}).Return(nil).Once()
		mockQuery2.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo2.InvalidateSession(ctx, "user-1", "sid"))

		mockDB3 := new(mocks.MockDB)
		mockQuery3 := new(mocks.MockQuery)
		mockQuery3.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB3, mockQuery3, nil, baseTime)
		repo3 := NewAccountRepository(mockDB3, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo3.RecordLoginAttempt(ctx, "k", true))
	})

	t.Run("storage helpers error branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetDevice(ctx, "device-1")
		require.Error(t, err)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
		_, err = repo2.GetUserDevices(ctx, "user-1")
		require.Error(t, err)

		mockDB3 := new(mocks.MockDB)
		mockQuery3 := new(mocks.MockQuery)
		mockQuery3.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB3, mockQuery3, nil, baseTime)
		repo3 := NewAccountRepository(mockDB3, "test-table", "example.com", zaptest.NewLogger(t))
		_, err = repo3.GetRecoveryToken(ctx, "k")
		require.Error(t, err)

		mockDB4 := new(mocks.MockDB)
		mockQuery4 := new(mocks.MockQuery)
		mockQuery4.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB4, mockQuery4, nil, baseTime)
		repo4 := NewAccountRepository(mockDB4, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo4.DeleteRecoveryToken(ctx, "k"))
	})

	t.Run("WebAuthn and wallet/provider error branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.StoreWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
			Challenge: "c",
			UserID:    "user-1",
			ExpiresAt: baseTime.Add(time.Minute),
			Type:      "authentication",
		}))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo2.StoreWebAuthnCredential(ctx, &storage.WebAuthnCredential{
			ID:         "cred",
			UserID:     "user-1",
			PublicKey:  []byte("pk"),
			CreatedAt:  baseTime,
			LastUsedAt: baseTime,
		}))

		mockDB3 := new(mocks.MockDB)
		mockQuery3 := new(mocks.MockQuery)
		mockQuery3.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB3, mockQuery3, nil, baseTime)
		repo3 := NewAccountRepository(mockDB3, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo3.UpdateWalletLastUsed(ctx, "user-1", "0xabc"))

		mockDB4 := new(mocks.MockDB)
		mockQuery4 := new(mocks.MockQuery)
		mockQuery4.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB4, mockQuery4, nil, baseTime)
		repo4 := NewAccountRepository(mockDB4, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo4.GetLinkedProviders(ctx, "user-1")
		require.Error(t, err)
	})
}
