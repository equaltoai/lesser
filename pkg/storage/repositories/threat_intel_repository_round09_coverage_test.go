package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestThreatIntelRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	t.Run("ShareThreat stores threat and indicators (tolerates indicator failures)", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// First Create() call: store threat. Second: store indicator (force error). Remaining: succeed.
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		mockQuery.On("Create").Return(nil).Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		err := repo.ShareThreat(ctx, &ThreatIntel{
			ID:           "t1",
			ThreatType:   "malware",
			Indicators:   []string{"example.com", "bad.example"},
			Severity:     "high",
			Description:  "desc",
			SourceDomain: "src.example",
			FirstSeen:    baseTime.Add(-time.Hour),
			LastSeen:     baseTime,
			HitCount:     2,
			Confidence:   0.8,
			TTL:          0, // force default TTL path
		})
		require.NoError(t, err)
	})

	t.Run("Query conversions and map mapping", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mockMatchedByType[*[]map[string]interface{}]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]map[string]interface{})
				*out = append(*out,
					map[string]interface{}{
						"PK":           "THREATS",
						"SK":           "TIME#1",
						"ID":           "t2",
						"ThreatType":   "spam",
						"Severity":     "low",
						"Description":  "x",
						"Indicators":   []string{"i1"},
						"HitCount":     int64(1),
						"Confidence":   float64(0.5),
						"SourceDomain": "src",
						"FirstSeen":    baseTime.Add(-time.Hour),
						"LastSeen":     baseTime,
						"TTL":          time.Now().Add(10 * time.Minute).Unix(),
					},
				)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)

		shared, err := repo.GetSharedThreats(ctx, baseTime.Add(-time.Hour))
		require.NoError(t, err)
		require.Len(t, shared, 1)

		byType, err := repo.GetThreatsByType(ctx, "spam", 10)
		require.NoError(t, err)
		require.Len(t, byType, 1)
	})

	t.Run("UpdateThreatConfidence not found errors; IncrementHitCount ignores missing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.Error(t, repo.UpdateThreatConfidence(ctx, "missing", 0.9))
		require.NoError(t, repo.IncrementHitCount(ctx, "missing"))
	})

	t.Run("Update path ignores update errors when ignoreMissing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("First", mockMatchedByType[*models.ThreatIntel]()).
			Run(func(args mock.Arguments) {
				m := args.Get(0).(*models.ThreatIntel)
				m.ID = "t3"
				m.ThreatType = "spam"
				m.LastSeen = baseTime
				_ = m.UpdateKeys()
			}).
			Return(nil).
			Once()

		mockQuery.On("Update").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.IncrementHitCount(ctx, "t3"))
	})

	t.Run("LoadActiveThreats filters TTL and PK prefix", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("AllPaginated", mockMatchedByType[*[]models.ThreatIntel]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.ThreatIntel)
				expired := models.ThreatIntel{ID: "expired", TTL: time.Now().Add(-time.Hour).Unix()}
				_ = expired.UpdateKeys()
				expired.PK = "THREAT#expired"

				wrongPrefix := models.ThreatIntel{ID: "x", TTL: time.Now().Add(time.Hour).Unix()}
				_ = wrongPrefix.UpdateKeys()
				wrongPrefix.PK = "OTHER#x"

				active := models.ThreatIntel{ID: "active", TTL: time.Now().Add(time.Hour).Unix(), ThreatType: "spam"}
				_ = active.UpdateKeys()
				active.PK = "THREAT#active"

				*out = append(*out, expired, wrongPrefix, active)
			}).
			Return(&core.PaginatedResult{HasMore: false}, nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)
		threats, err := repo.LoadActiveThreats(ctx)
		require.NoError(t, err)
		require.Len(t, threats, 1)
	})

	t.Run("GetThreatByID and indicator lookup branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewThreatIntelRepository(mockDB, "test-table", zap.NewNop(), nil)

		got, err := repo.GetThreatByID(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, got)

		id, err := repo.GetIndicatorThreat(ctx, "indicator")
		require.NoError(t, err)
		require.Empty(t, id)
	})
}
