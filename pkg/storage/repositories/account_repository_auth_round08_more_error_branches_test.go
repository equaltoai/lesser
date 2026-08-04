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

func TestRound08_AccountRepositoryAuth_MoreErrorBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetRecentLoginAttempts query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetRecentLoginAttempts(ctx, "user-1", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("ValidatePasswordResetToken expired and used branches", func(t *testing.T) {
		t.Run("expired", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				reset := args.Get(0).(*models.PasswordReset)
				reset.Username = "user-1"
				reset.Token = "tok"
				reset.Email = ""
				reset.ExpiresAt = baseTime.Add(-time.Minute)
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			_, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.Error(t, err)
		})

		t.Run("used", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				reset := args.Get(0).(*models.PasswordReset)
				reset.Username = "user-1"
				reset.Token = "tok"
				reset.Email = ""
				reset.ExpiresAt = baseTime.Add(time.Hour)
				reset.Used = true
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			_, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.Error(t, err)
		})
	})

	t.Run("GetUserSessions query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetUserSessions(ctx, "user-1")
		require.Error(t, err)
	})

	t.Run("UpdateSession not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.UpdateSession(ctx, "sid", "rt", "ip", time.Now(), time.Now().Add(time.Hour))
		require.Error(t, err)
	})

	t.Run("DeleteSession not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.DeleteSession(ctx, "sid")
		require.Error(t, err)
	})

	t.Run("GetSessionByRefreshToken empty results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetSessionByRefreshToken(ctx, "rt")
		require.Error(t, err)
	})

	t.Run("CreateDevice create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.Error(t, repo.CreateDevice(ctx, &storage.Device{DeviceID: "d", Username: "u", CreatedAt: baseTime, LastSeenAt: baseTime}))
	})
}
