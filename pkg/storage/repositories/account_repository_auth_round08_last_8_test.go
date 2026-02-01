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
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepositoryAuth_LastMileCoverage(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("ClearLoginAttempts logs lockout delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.LoginAttempt)
			*out = append(*out, models.LoginAttempt{PK: "RATELIMIT#k", SK: "sk"})
		}).Return(nil).Once()
		mockQuery.On("Delete", mock.Anything).Return(nil).Once()                // delete attempt
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once() // delete lockout

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.ClearLoginAttempts(ctx, "k"))
	})

	t.Run("InvalidateAllSessions skips expired sessions", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Session)
			*out = append(*out, models.Session{
				SessionID: "sid",
				UserID:    "USER#user-1",
				ExpiresAt: baseTime.Add(-time.Minute).Unix(),
			})
		}).Return(nil).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.InvalidateAllSessions(ctx, "user-1"))
	})

	t.Run("UpdateWalletLastUsed not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateWalletLastUsed(ctx, "user-1", "0xabc"))
	})

	t.Run("UpdateDevice update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			device := args.Get(0).(*models.Device)
			device.Username = "user-1"
			device.DeviceID = "device-1"
			device.TrustLevel = "trusted"
			device.LastSeenAt = baseTime
			device.UpdateKeys()
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateDevice(ctx, &storage.Device{Username: "user-1", DeviceID: "device-1"}))
	})
}
