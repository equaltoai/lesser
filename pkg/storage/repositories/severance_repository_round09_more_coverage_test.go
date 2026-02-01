package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestSeveranceRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("NewSeveranceRepository defaults nil logger", func(t *testing.T) {
		repo := NewSeveranceRepository(new(mocks.MockDB), "test-table", nil)
		require.NotNil(t, repo)
		require.NotNil(t, repo.logger)
	})

	t.Run("CreateSeveredRelationship create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		require.Error(t, repo.CreateSeveredRelationship(ctx, models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)))
	})

	t.Run("GetSeveredRelationship scan error and not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		_, err := repo.GetSeveredRelationship(ctx, "local_remote_1")
		require.Error(t, err)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Scan", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewSeveranceRepository(mockDB2, "test-table", nil)
		_, err = repo2.GetSeveredRelationship(ctx, "local_remote_1")
		require.Error(t, err)
	})

	t.Run("ListSeveredRelationships query error and reason filter empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		_, _, err := repo.ListSeveredRelationships(ctx, "local", SeveranceFilters{}, 0, "")
		require.Error(t, err)
	})

	t.Run("UpdateSeveranceStatus save error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("Scan", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.SeveredRelationship)
				s := models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)
				s.DetectedAt = baseTime
				s.ID = "local_remote_1"
				_ = s.UpdateKeys()
				*out = append(*out, s)
			}).
			Return(nil).
			Once()

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		require.Error(t, repo.UpdateSeveranceStatus(ctx, "local_remote_1", models.SeveranceStatusRestored))
	})

	t.Run("GetAffectedRelationships not found and error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		items, next, err := repo.GetAffectedRelationships(ctx, "sev", 2, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, next)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewSeveranceRepository(mockDB2, "test-table", nil)
		_, _, err = repo2.GetAffectedRelationships(ctx, "sev", 2, "")
		require.Error(t, err)
	})

	t.Run("Reconnection attempts not found and error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", nil)
		attempts, err := repo.GetReconnectionAttempts(ctx, "sev")
		require.NoError(t, err)
		require.Empty(t, attempts)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewSeveranceRepository(mockDB2, "test-table", nil)
		_, err = repo2.GetReconnectionAttempts(ctx, "sev")
		require.Error(t, err)
	})
}
