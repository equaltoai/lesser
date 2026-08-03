package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestSeveranceRepository_Round09_FinalPush(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 2, 3, 4, 0, time.UTC)

	t.Run("CreateAffectedRelationship create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.CreateAffectedRelationship(ctx, models.NewAffectedRelationship("sev", "actor-1", "@a1", "example.com", "follower", baseTime)))
	})

	t.Run("CreateReconnectionAttempt and UpdateReconnectionAttempt create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Twice()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.CreateReconnectionAttempt(ctx, models.NewSeveranceReconnectionAttempt("sev", "admin")))
		require.Error(t, repo.UpdateReconnectionAttempt(ctx, models.NewSeveranceReconnectionAttempt("sev", "admin")))
	})

	t.Run("GetReconnectionAttempt other error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetReconnectionAttempt(ctx, "sev", "attempt-1")
		require.Error(t, err)
	})

	t.Run("UpdateSeveranceStatus get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.UpdateSeveranceStatus(ctx, "local_remote_1", models.SeveranceStatusActive))
	})
}
