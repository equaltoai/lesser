package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestPatternRepository_CrudAndQueries(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatePattern nil input returns error", func(t *testing.T) {
		repo := NewPatternRepository(nil, "table", zap.NewNop(), nil)
		err := repo.CreatePattern(ctx, nil)
		require.Error(t, err)
	})

	t.Run("CreatePattern success sets timestamps and calls create", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)

		start := time.Now()
		pattern := &models.ModerationPattern{
			PatternID: "p1",
			Type:      "keyword",
			Pattern:   "spam",
			Category:  "spam",
			Severity:  0.5,
			Active:    true,
			HitCount:  123,
		}
		err := repo.CreatePattern(ctx, pattern)
		require.NoError(t, err)
		assert.WithinDuration(t, start, pattern.CreatedAt, time.Second)
		assert.WithinDuration(t, start, pattern.UpdatedAt, time.Second)
		assert.Equal(t, int64(0), pattern.HitCount)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CreatePattern db error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.CreatePattern(ctx, &models.ModerationPattern{
			PatternID: "p1",
			Type:      "keyword",
			Pattern:   "spam",
			Category:  "spam",
			Severity:  0.5,
			Active:    true,
		})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdatePattern applies updates and calls update", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ModerationPattern)
			*dest = models.ModerationPattern{
				PatternID: "p1",
				Pattern:   "old",
				Type:      "keyword",
				Category:  "oldcat",
				Severity:  0.1,
				Active:    true,
				Flags:     []string{"a"},
				HitCount:  5,
			}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		updates := &models.ModerationPattern{
			Pattern:     "new",
			Type:        "regex",
			Category:    "spam",
			Severity:    0.9,
			Description: "desc",
			Flags:       []string{"x", "y"},
			Active:      false,
		}
		err := repo.UpdatePattern(ctx, "p1", updates)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdatePattern get error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.UpdatePattern(ctx, "p1", &models.ModerationPattern{Pattern: "x"})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("DeletePattern marks inactive and calls update", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ModerationPattern)
			*dest = models.ModerationPattern{PatternID: "p1", Active: true}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.DeletePattern(ctx, "p1")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("DeletePattern update error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ModerationPattern)
			*dest = models.ModerationPattern{PatternID: "p1", Active: true}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.DeletePattern(ctx, "p1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetPattern returns loaded pattern", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ModerationPattern)
			*dest = models.ModerationPattern{PatternID: "p1", Active: true}
			_ = dest.UpdateKeys()
		}).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		pattern, err := repo.GetPattern(ctx, "p1")
		require.NoError(t, err)
		assert.Equal(t, "p1", pattern.PatternID)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetPattern error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetPattern(ctx, "p1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetPatterns applies filters and returns results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "MODERATION_PATTERNS#ALL").Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = []models.ModerationPattern{
				{PatternID: "p1", Category: "spam", Active: true},
			}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		patterns, err := repo.GetPatterns(ctx, "spam", true)
		require.NoError(t, err)
		require.Len(t, patterns, 1)
		assert.Equal(t, "p1", patterns[0].PatternID)
		mockQuery.AssertNumberOfCalls(t, "Filter", 2)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetPatterns tracks cost when configured", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "MODERATION_PATTERNS#ALL").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = []models.ModerationPattern{}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), newTestCostService(t))
		_, err := repo.GetPatterns(ctx, "", false)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetPatterns db error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "MODERATION_PATTERNS#ALL").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetPatterns(ctx, "", false)
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("IncrementHitCount increments and updates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ModerationPattern)
			*dest = models.ModerationPattern{PatternID: "p1", HitCount: 3}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.IncrementHitCount(ctx, "p1")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("IncrementHitCount get error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.IncrementHitCount(ctx, "p1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("LoadActivePatterns delegates to GetPatterns", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "MODERATION_PATTERNS#ALL").Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = []models.ModerationPattern{}
		}).Once()

		repo := NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
		patterns, err := repo.LoadActivePatterns(ctx)
		require.NoError(t, err)
		assert.Empty(t, patterns)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}
