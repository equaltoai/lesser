package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func newEnhancedPatternRepo(t *testing.T, mockDB *mocks.MockDB) *EnhancedPatternRepository {
	t.Helper()

	repo := NewEnhancedPatternRepository(mockDB, "table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)
	return repo
}

func TestEnhancedPatternRepository_CRUDAndQueries(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatePattern nil input", func(t *testing.T) {
		repo := NewEnhancedPatternRepository(nil, "table", zap.NewNop(), nil)
		err := repo.CreatePattern(ctx, nil)
		require.Error(t, err)
	})

	t.Run("CreatePattern success populates defaults and creates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		repo := newEnhancedPatternRepo(t, mockDB)
		pattern := &models.EnhancedModerationPattern{
			PatternID:      "p1",
			PatternType:    "text",
			PatternContent: "hello",
			Category:       "spam",
			Severity:       StatusLow,
			Priority:       3,
			Active:         true,
		}
		err := repo.CreatePattern(ctx, pattern)
		require.NoError(t, err)
		assert.Equal(t, 1, pattern.Version)
		assert.Equal(t, int64(0), pattern.MatchCount)
		assert.Equal(t, 0.5, pattern.ConfidenceScore)
	})

	t.Run("GetPattern wraps get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(assert.AnError)

		repo := newEnhancedPatternRepo(t, mockDB)
		_, err := repo.GetPattern(ctx, "missing")
		require.Error(t, err)
	})

	t.Run("UpdatePattern nil input", func(t *testing.T) {
		repo := NewEnhancedPatternRepository(nil, "table", zap.NewNop(), nil)
		err := repo.UpdatePattern(ctx, nil)
		require.Error(t, err)
	})

	t.Run("UpdatePattern success calls update", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Update", mock.Anything).Return(nil)

		repo := newEnhancedPatternRepo(t, mockDB)
		pattern := &models.EnhancedModerationPattern{
			PatternID:      "p1",
			PatternType:    "text",
			PatternContent: "hello",
			Category:       "spam",
			Severity:       StatusLow,
			Priority:       3,
			Active:         true,
		}
		err := repo.UpdatePattern(ctx, pattern)
		require.NoError(t, err)
	})

	t.Run("DeletePattern loads then updates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{PatternID: "p1", Active: true}
			_ = dest.UpdateKeys()
		})
		mockQuery.On("Update", mock.Anything).Return(nil)

		repo := newEnhancedPatternRepo(t, mockDB)
		err := repo.DeletePattern(ctx, "p1")
		require.NoError(t, err)
	})

	t.Run("GetActivePatterns / GetPatternsByType / GetPatternsByCategory", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.EnhancedModerationPattern)
			*dest = []*models.EnhancedModerationPattern{
				{PatternID: "p1", Active: true, Effectiveness: 0.9, Category: "spam", PatternType: "text"},
			}
		})

		repo := newEnhancedPatternRepo(t, mockDB)
		_, err := repo.GetActivePatterns(ctx, 10)
		require.NoError(t, err)
		_, err = repo.GetPatternsByType(ctx, "text", 10)
		require.NoError(t, err)
		_, err = repo.GetPatternsByCategory(ctx, "spam", 10)
		require.NoError(t, err)
	})
}

func TestEnhancedPatternRepository_AnalysisAndFeedback(t *testing.T) {
	ctx := context.Background()

	t.Run("AnalyzeContentPatterns processes patterns and records matches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{PatternID: "p1", Active: true, ConfidenceScore: 0.9}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		content := "https://example.com"
		patterns := []*models.EnhancedModerationPattern{
			{
				PatternID:       "p1",
				PatternType:     "url_exact",
				PatternContent:  content,
				Category:        "spam",
				Active:          true,
				Effectiveness:   0.9,
				ConfidenceScore: 0.9,
			},
			{PatternID: "p2", Active: false},
		}

		analysis, err := repo.AnalyzeContentPatterns(ctx, content, patterns)
		require.NoError(t, err)
		require.Len(t, analysis.Matches, 1)
	})

	t.Run("DetectSpamPatterns pulls patterns and computes spam score", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.EnhancedModerationPattern)
			*dest = []*models.EnhancedModerationPattern{
				{
					PatternID:       "p1",
					PatternType:     "text",
					PatternContent:  "spam",
					Category:        "spam",
					Active:          true,
					Effectiveness:   0.8,
					ConfidenceScore: 0.9,
				},
				{PatternID: "p2", Active: true, Effectiveness: 0.6}, // filtered out
			}
		}).Once()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{PatternID: "p1", Active: true, Effectiveness: 0.8, ConfidenceScore: 0.9}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		out, err := repo.DetectSpamPatterns(ctx, "spam content", &SenderInfo{AccountAge: 3, FollowerCount: 1, ViolationCount: 1})
		require.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("UpdatePatternEffectiveness updates based on feedback and saves", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		// GetPattern
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{
				PatternID:         "p1",
				Active:            true,
				ConfidenceScore:   0.5,
				ValidationScore:   0.1,
				MatchCount:        10,
				TruePositiveCount: 5,
			}
			_ = dest.UpdateKeys()
		}).Once()

		// GetPerformanceMetrics
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.PatternPerformanceMetric)
			*dest = []*models.PatternPerformanceMetric{
				{TruePositives: 10, FalsePositives: 5},
			}
		}).Once()

		// UpdatePattern
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		err := repo.UpdatePatternEffectiveness(ctx, "p1", &PatternFeedback{PatternID: "p1", FeedbackType: "false_negative"})
		require.NoError(t, err)
	})

	t.Run("GetOptimalPatterns filters and sorts", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.EnhancedModerationPattern)
			*dest = []*models.EnhancedModerationPattern{
				{PatternID: "best", Active: true, Effectiveness: 0.9, ConfidenceScore: 0.9, Priority: 10},
				{PatternID: "skip_inactive", Active: false, Effectiveness: 0.9},
				{PatternID: "skip_low", Active: true, Effectiveness: 0.2},
			}
		})

		repo := newEnhancedPatternRepo(t, mockDB)
		items, err := repo.GetOptimalPatterns(ctx, "spam", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "best", items[0].PatternID)
	})

	t.Run("LearnFromFeedback early return and normal path", func(t *testing.T) {
		repo := newEnhancedPatternRepo(t, new(mocks.MockDB))
		require.NoError(t, repo.LearnFromFeedback(ctx, nil))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{PatternID: "p1", Active: true, MatchCount: 1, TruePositiveCount: 1, ConfidenceScore: 0.5}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.PatternPerformanceMetric)
			*dest = []*models.PatternPerformanceMetric{}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo = newEnhancedPatternRepo(t, mockDB)
		err := repo.LearnFromFeedback(ctx, []*PatternFeedback{
			{PatternID: "p1", FeedbackType: "false_positive"},
		})
		require.NoError(t, err)
	})
}

func TestEnhancedPatternRepository_CacheAndMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("GetPatternCache updates cache stats and tolerates update errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PatternCache)
			*dest = models.PatternCache{PatternID: "p1", PatternType: "text", CacheHits: 2}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		cache, err := repo.GetPatternCache(ctx, "p1", "text")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, cache.CacheHits, int64(3))
	})

	t.Run("SetPatternCache nil input and upsert success", func(t *testing.T) {
		repo := newEnhancedPatternRepo(t, new(mocks.MockDB))
		require.Error(t, repo.SetPatternCache(ctx, nil))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("CreateOrUpdate").Return(nil).Once()

		repo = newEnhancedPatternRepo(t, mockDB)
		err := repo.SetPatternCache(ctx, &models.PatternCache{PatternID: "p1", PatternType: "text"})
		require.NoError(t, err)
	})

	t.Run("InvalidatePatternCache always returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		require.NoError(t, repo.InvalidatePatternCache(ctx, "p1", "text"))
	})

	t.Run("RecordPerformanceMetric create and update paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		// Create path: First errors -> Create succeeds
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		metric := &models.PatternPerformanceMetric{PK: "PATTERN_METRICS#p1", SK: "TIME#2025-01-01#00"}
		require.NoError(t, repo.RecordPerformanceMetric(ctx, metric))

		// Update path: First succeeds -> Update succeeds
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PatternPerformanceMetric)
			*dest = models.PatternPerformanceMetric{
				PK:             metric.PK,
				SK:             metric.SK,
				MatchAttempts:  10,
				TotalMatchTime: 10.0,
			}
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		metric2 := &models.PatternPerformanceMetric{PK: metric.PK, SK: metric.SK, MatchAttempts: 5, TotalMatchTime: 5.0, MinMatchTime: 1.0, MaxMatchTime: 2.0}
		require.NoError(t, repo.RecordPerformanceMetric(ctx, metric2))
	})

	t.Run("CreateTestResult and GetTestResults + GetLatestTestResult", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)

		repo := newEnhancedPatternRepo(t, mockDB)
		require.Error(t, repo.CreateTestResult(ctx, nil))

		mockQuery.On("Create").Return(nil).Once()
		require.NoError(t, repo.CreateTestResult(ctx, &models.PatternTestResult{PatternID: "p1", TestType: "unit"}))

		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.PatternTestResult)
			*dest = []*models.PatternTestResult{{PatternID: "p1", TestType: "unit"}}
		}).Once()
		_, err := repo.GetTestResults(ctx, "p1", "unit", 1)
		require.NoError(t, err)

		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.PatternTestResult)
			*dest = []*models.PatternTestResult{}
		}).Once()
		_, err = repo.GetLatestTestResult(ctx, "p1", "unit")
		require.Error(t, err)
	})

	t.Run("GetPerformanceMetrics filters by date", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.PatternPerformanceMetric)
			*dest = []*models.PatternPerformanceMetric{{PK: "PATTERN_METRICS#p1"}}
		})

		repo := newEnhancedPatternRepo(t, mockDB)
		metrics, err := repo.GetPerformanceMetrics(ctx, "p1", "2025-01-01", "2025-01-31")
		require.NoError(t, err)
		require.Len(t, metrics, 1)
	})
}

func TestEnhancedPatternRepository_Maintenance(t *testing.T) {
	ctx := context.Background()

	t.Run("CleanupExpiredPatterns returns cleaned count", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		// CleanupExpiredPatterns initial scan
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.EnhancedModerationPattern)
			*dest = []*models.EnhancedModerationPattern{
				{PatternID: "expired", Active: true, ExpiresAt: time.Now().Add(-time.Hour)},
				{PatternID: "active", Active: true},
			}
		}).Once()

		// DeletePattern -> GetPattern
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.EnhancedModerationPattern)
			*dest = models.EnhancedModerationPattern{PatternID: "expired", Active: true}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		count, err := repo.CleanupExpiredPatterns(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("GetPatternStatistics aggregates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.EnhancedModerationPattern)
			*dest = []*models.EnhancedModerationPattern{
				{PatternID: "p1", Active: true, Effectiveness: 0.2, MatchCount: 10, FalsePositiveCount: 1, TruePositiveCount: 9, Category: "spam", PatternType: "text", Severity: StatusLow},
				{PatternID: "p2", Active: false, Effectiveness: 0.8, MatchCount: 0, Category: "spam", PatternType: "text", Severity: StatusHigh},
			}
		}).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		stats, err := repo.GetPatternStatistics(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, stats["total_patterns"])
		assert.Equal(t, 1, stats["active_patterns"])
	})
}

func TestEnhancedPatternRepository_GetLatestTestResult_NotFoundError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

	repo := newEnhancedPatternRepo(t, mockDB)
	_, err := repo.GetLatestTestResult(ctx, "p1", "unit")
	require.Error(t, err)
}

func TestEnhancedPatternRepository_RecordMatch_AverageMatchTime(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.EnhancedModerationPattern)
		*dest = models.EnhancedModerationPattern{PatternID: "p1", Active: true, AverageMatchTime: 0}
		_ = dest.UpdateKeys()
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := newEnhancedPatternRepo(t, mockDB)
	err := repo.RecordMatch(ctx, "p1", true, true, 10.0)
	require.NoError(t, err)
}

func TestEnhancedPatternRepository_SetPatternCache_NilCache(t *testing.T) {
	ctx := context.Background()

	repo := newEnhancedPatternRepo(t, new(mocks.MockDB))
	err := repo.SetPatternCache(ctx, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrPatternCacheCreateFailed)
}

func TestEnhancedPatternRepository_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatePattern wraps create failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		err := repo.CreatePattern(ctx, &models.EnhancedModerationPattern{PatternID: "p1", PatternType: "text", PatternContent: "x", Active: true})
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternCreateFailed)
	})

	t.Run("GetPatternCache returns not found error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		_, err := repo.GetPatternCache(ctx, "p1", "text")
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternCacheNotFound)
	})

	t.Run("SetPatternCache upsert failure is surfaced", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("CreateOrUpdate").Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		err := repo.SetPatternCache(ctx, &models.PatternCache{PatternID: "p1", PatternType: "text"})
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternCacheUpdateFailed)
	})

	t.Run("RecordPerformanceMetric nil metric and create failures", func(t *testing.T) {
		repo := newEnhancedPatternRepo(t, new(mocks.MockDB))
		require.Error(t, repo.RecordPerformanceMetric(ctx, nil))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo = newEnhancedPatternRepo(t, mockDB)
		err := repo.RecordPerformanceMetric(ctx, &models.PatternPerformanceMetric{PK: "PATTERN_METRICS#p1", SK: "TIME#2025-01-01#00"})
		require.Error(t, err)
	})

	t.Run("CreateTestResult and GetTestResults errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		err := repo.CreateTestResult(ctx, &models.PatternTestResult{PatternID: "p1", TestType: "unit"})
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternTestResultCreateFailed)

		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()
		_, err = repo.GetTestResults(ctx, "p1", "", 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternTestResultQueryFailed)
	})

	t.Run("GetPerformanceMetrics query failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		_, err := repo.GetPerformanceMetrics(ctx, "p1", "", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternMetricsQueryFailed)
	})

	t.Run("CleanupExpiredPatterns and GetPatternStatistics query failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := newEnhancedPatternRepo(t, mockDB)
		_, err := repo.CleanupExpiredPatterns(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternQueryFailed)

		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()
		_, err = repo.GetPatternStatistics(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, storage.ErrPatternQueryFailed)
	})
}
