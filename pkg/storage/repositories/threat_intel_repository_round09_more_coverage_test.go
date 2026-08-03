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
	"go.uber.org/zap"
)

func TestThreatIntelRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("ShareThreat returns create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.ShareThreat(ctx, &ThreatIntel{
			ID: "t1", ThreatType: "spam", Indicators: []string{"i1"}, LastSeen: baseTime, FirstSeen: baseTime,
		}))
	})

	t.Run("GetSharedThreats and GetThreatsByType query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mockMatchedByType[*[]map[string]interface{}]()).Return(ErrTestMockError).Twice()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetSharedThreats(ctx, baseTime.Add(-time.Hour))
		require.Error(t, err)
		_, err = repo.GetThreatsByType(ctx, "spam", 10)
		require.Error(t, err)
	})

	t.Run("UpdateThreatConfidence success and updateThreat get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("First", mockMatchedByType[*models.ThreatIntel]()).
			Run(func(args mock.Arguments) {
				m := args.Get(0).(*models.ThreatIntel)
				m.ID = "t1"
				m.ThreatType = "spam"
				m.LastSeen = baseTime
				m.FirstSeen = baseTime.Add(-time.Hour)
				_ = m.UpdateKeys()
			}).
			Return(nil).
			Once()
		mockQuery.On("Update").Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.UpdateThreatConfidence(ctx, "t1", 0.9))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewThreatIntelRepository(mockDB2, "test-table", zap.NewNop(), nil)
		require.Error(t, repo2.UpdateThreatConfidence(ctx, "t1", 0.9))
	})

	t.Run("GetThreatByID success and indicator error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("First", mockMatchedByType[*models.ThreatIntel]()).
			Run(func(args mock.Arguments) {
				m := args.Get(0).(*models.ThreatIntel)
				m.ID = "t1"
				m.ThreatType = "spam"
				m.LastSeen = baseTime
				_ = m.UpdateKeys()
			}).
			Return(nil).
			Once()
		mockQuery.On("First", mockMatchedByType[*models.ThreatIndicator]()).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		got, err := repo.GetThreatByID(ctx, "t1")
		require.NoError(t, err)
		require.NotNil(t, got)

		// Unknown indicator uses not found -> empty id.
		id, err := repo.GetIndicatorThreat(ctx, "i1")
		require.NoError(t, err)
		require.Empty(t, id)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewThreatIntelRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err = repo2.GetIndicatorThreat(ctx, "i1")
		require.Error(t, err)
	})
}
