package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestTrendingCache_round09_basic(t *testing.T) {
	cache := NewTrendingCache(10 * time.Millisecond)

	cache.setTrendingResult("k1", &CachedTrendingResult{GeneratedAt: time.Now().Add(-time.Hour), Results: []*storage.TrendingHashtag{}})
	cache.setHashtagMetrics("tag", &CachedHashtagMetrics{ValidUntil: time.Now().Add(-time.Second)})

	assert.Nil(t, cache.getTrendingResult("k1"))
	assert.Nil(t, cache.getHashtagMetrics("tag"))
}

func TestTrendingEngine_round09_cache_hit(t *testing.T) {
	engine := NewTrendingEngine(new(mocks.MockDB), zap.NewNop())
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	key := engine.getCacheKey(since, 5)

	engine.cache.setTrendingResult(key, &CachedTrendingResult{
		Results:     []*storage.TrendingHashtag{{Name: "go"}},
		GeneratedAt: time.Now(),
		HitCount:    1,
	})

	out, err := engine.CalculateTrending(context.Background(), since, 5)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "go", out[0].Name)

	cached := engine.cache.getTrendingResult(key)
	require.NotNil(t, cached)
	assert.EqualValues(t, 2, cached.HitCount)
}

func TestTrendingEngine_round09_full_flow_and_helpers(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	hashtagQuery := new(mocks.MockQuery)
	usageQuery := new(mocks.MockQuery)
	statusQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(hashtagQuery)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQuery)
	// Status content lookup uses te.db.Model without WithContext.
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(statusQuery)

	engine := NewTrendingEngine(mockDB, zap.NewNop())
	engine.config.TimeWindows = map[string]TrendingTimeWindow{
		"1h":  {Name: "1h", Duration: time.Hour},
		"24h": {Name: "24h", Duration: 24 * time.Hour},
	}
	engine.config.MinimumUsage = 1
	engine.config.MinimumUsers = 1
	engine.config.TrustThreshold = 0
	engine.config.DiversityThreshold = 0
	engine.config.Scoring.ScoreThreshold = 0
	engine.config.CandidateLimit = 10

	// Candidate hashtags
	hashtagQuery.On("Where", "SK", "=", "METADATA").Return(hashtagQuery).Once()
	hashtagQuery.On("Filter", "LastUsed", ">=", mock.Anything).Return(hashtagQuery).Once()
	hashtagQuery.On("Filter", "UsageCount", ">=", int64(1)).Return(hashtagQuery).Once()
	hashtagQuery.On("OrderBy", "LastUsed", "DESC").Return(hashtagQuery).Once()
	hashtagQuery.On("Limit", 10).Return(hashtagQuery).Once()
	hashtagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Hashtag)
		*dest = []*models.Hashtag{{
			Name:       "go",
			FirstSeen:  time.Now().Add(-2 * time.Hour),
			LastUsed:   time.Now().Add(-5 * time.Minute),
			UsageCount: 5,
		}}
	}).Return(nil).Once()

	// Window metrics usage records (called once per window)
	usageQuery.On("Where", "PK", "=", "HASHTAG#go").Return(usageQuery)
	usageQuery.On("Where", "SK", ">=", mock.Anything).Return(usageQuery)
	usageQuery.On("Where", "SK", "<=", mock.Anything).Return(usageQuery)
	usageQuery.On("OrderBy", "SK", "DESC").Return(usageQuery)
	usageQuery.On("Limit", 1000).Return(usageQuery)
	usageQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.HashtagUsage)
		*dest = []models.HashtagUsage{
			{AuthorID: "user-1", Visibility: models.VisibilityPublic, StatusID: "s1"},
			{AuthorID: "user-2", Visibility: "private", StatusID: "s2"},
		}
	}).Return(nil).Twice()

	// Status content lookup: first success, then error.
	statusQuery.On("Where", "PK", "=", mock.Anything).Return(statusQuery)
	statusQuery.On("Where", "SK", "=", "METADATA").Return(statusQuery)
	statusQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Status)
		dest.Content = "the photo https://x.com #go"
	}).Return(nil)

	since := time.Now().Add(-time.Hour)
	out, err := engine.CalculateTrending(ctx, since, 5)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "go", out[0].Name)

	// Metrics caching branch
	m, err := engine.calculateEnhancedMetrics(ctx, &models.Hashtag{Name: "go"})
	require.NoError(t, err)
	m2, err := engine.calculateEnhancedMetrics(ctx, &models.Hashtag{Name: "go"})
	require.NoError(t, err)
	assert.Equal(t, m, m2)

	// Helper coverage: engagement + trust + content analysis
	assert.EqualValues(t, 3, engine.estimateEngagement(models.VisibilityPublic))
	assert.EqualValues(t, 1, engine.estimateEngagement("unknown"))

	trust := engine.calculateUserTrustScore("123456789012345", map[string]bool{"a": true, "b": true})
	assert.GreaterOrEqual(t, trust, 0.0)
	assert.LessOrEqual(t, trust, 1.0)
	assert.True(t, engine.detectSuspiciousUserPattern("123456789012"))
	assert.True(t, engine.detectSuspiciousUserPattern("abcabcabc"))
	assert.True(t, engine.detectSuspiciousUserPattern("ab"))

	q := engine.analyzeContentQuality("")
	assert.Equal(t, 0, q.TextLength)
	q = engine.analyzeContentQuality("photo http://example.com #tag the and")
	assert.NotZero(t, q.TextLength)
	assert.NotEqual(t, "", q.LanguageCode)
	assert.Equal(t, "en", engine.detectLanguage("the and for"))
	assert.Equal(t, "es", engine.detectLanguage("que por con"))
	assert.Equal(t, "unknown", engine.detectLanguage("zzzz"))

	mockDB.AssertExpectations(t)
	hashtagQuery.AssertExpectations(t)
	usageQuery.AssertExpectations(t)
	statusQuery.AssertExpectations(t)
}

func TestTrendingEngine_round09_db_error_paths(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	hashtagQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(hashtagQuery)

	engine := NewTrendingEngine(mockDB, zap.NewNop())
	engine.config.MinimumUsage = 1
	engine.config.CandidateLimit = 10

	hashtagQuery.On("Where", "SK", "=", "METADATA").Return(hashtagQuery)
	hashtagQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(hashtagQuery)
	hashtagQuery.On("OrderBy", mock.Anything, mock.Anything).Return(hashtagQuery)
	hashtagQuery.On("Limit", 10).Return(hashtagQuery)
	hashtagQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()

	_, err := engine.getCandidateHashtags(ctx, time.Now().Add(-time.Hour))
	assert.Error(t, err)

	usageQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQuery)
	usageQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(usageQuery)
	usageQuery.On("OrderBy", mock.Anything, mock.Anything).Return(usageQuery)
	usageQuery.On("Limit", 1000).Return(usageQuery)
	usageQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
	_, err = engine.calculateWindowMetrics(ctx, "go", time.Now().Add(-time.Hour), time.Now())
	assert.Error(t, err)

	// getStatusContentForAnalysis invalid ID branch
	assert.Equal(t, "", engine.getStatusContentForAnalysis(""))

	// getStatusContentForAnalysis fetch error branch
	statusQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(statusQuery)
	statusQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(statusQuery)
	statusQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	assert.Equal(t, "", engine.getStatusContentForAnalysis("s1"))
}

func TestTrendingEngine_round09_create_with_not_found_usage_records(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	usageQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQuery)

	engine := NewTrendingEngine(mockDB, zap.NewNop())

	usageQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(usageQuery)
	usageQuery.On("OrderBy", mock.Anything, mock.Anything).Return(usageQuery)
	usageQuery.On("Limit", 1000).Return(usageQuery)
	usageQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	m, err := engine.calculateWindowMetrics(ctx, "go", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 0, m.UsageCount)
}

func TestTrendingEngine_round09_calculate_trending_empty_and_trim_paths(t *testing.T) {
	ctx := context.Background()

	t.Run("no_candidates_returns_empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		q := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(q)

		engine := NewTrendingEngine(mockDB, zap.NewNop())
		engine.config.MinimumUsage = 1
		engine.config.CandidateLimit = 10

		q.On("Where", "SK", "=", "METADATA").Return(q).Once()
		q.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(q)
		q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Once()
		q.On("Limit", 10).Return(q).Once()
		q.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		out, err := engine.CalculateTrending(ctx, time.Now().Add(-time.Hour), 5)
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("trim_to_limit_and_score_threshold_skip", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		hashtagQuery := new(mocks.MockQuery)
		usageQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(hashtagQuery)
		mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQuery)
		mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(new(mocks.MockQuery))

		engine := NewTrendingEngine(mockDB, zap.NewNop())
		engine.config.TimeWindows = map[string]TrendingTimeWindow{
			"1h": {Name: "1h", Duration: time.Hour},
		}
		engine.config.MinimumUsage = 1
		engine.config.MinimumUsers = 0
		engine.config.TrustThreshold = 0
		engine.config.DiversityThreshold = 0
		engine.config.Scoring.ScoreThreshold = 0
		engine.config.CandidateLimit = 10

		hashtagQuery.On("Where", "SK", "=", "METADATA").Return(hashtagQuery).Once()
		hashtagQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(hashtagQuery)
		hashtagQuery.On("OrderBy", mock.Anything, mock.Anything).Return(hashtagQuery).Once()
		hashtagQuery.On("Limit", 10).Return(hashtagQuery).Once()
		hashtagQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Hashtag)
			*dest = []*models.Hashtag{
				{Name: "a", FirstSeen: time.Now().Add(-2 * time.Hour), LastUsed: time.Now().Add(-time.Minute), UsageCount: 5},
				{Name: "b", FirstSeen: time.Now().Add(-2 * time.Hour), LastUsed: time.Now().Add(-time.Minute), UsageCount: 6},
			}
		}).Return(nil).Once()

		usageQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(usageQuery)
		usageQuery.On("OrderBy", mock.Anything, mock.Anything).Return(usageQuery)
		usageQuery.On("Limit", 1000).Return(usageQuery)
		usageQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

		out, err := engine.CalculateTrending(ctx, time.Now().Add(-time.Hour), 1)
		require.NoError(t, err)
		assert.Len(t, out, 1)

		// Score threshold skip branch
		engine.cache = NewTrendingCache(0)
		engine.config.Scoring.ScoreThreshold = 9999
		hashtagQuery2 := new(mocks.MockQuery)
		usageQuery2 := new(mocks.MockQuery)
		mockDB2 := new(mocks.MockDB)
		mockDB2.On("WithContext", mock.Anything).Return(mockDB2)
		mockDB2.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(hashtagQuery2)
		mockDB2.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQuery2)
		mockDB2.On("Model", mock.AnythingOfType("*models.Status")).Return(new(mocks.MockQuery))
		engine.db = mockDB2

		hashtagQuery2.On("Where", "SK", "=", "METADATA").Return(hashtagQuery2).Once()
		hashtagQuery2.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(hashtagQuery2)
		hashtagQuery2.On("OrderBy", mock.Anything, mock.Anything).Return(hashtagQuery2).Once()
		hashtagQuery2.On("Limit", 10).Return(hashtagQuery2).Once()
		hashtagQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Hashtag)
			*dest = []*models.Hashtag{{Name: "a", FirstSeen: time.Now().Add(-2 * time.Hour), LastUsed: time.Now().Add(-time.Minute), UsageCount: 5}}
		}).Return(nil).Once()

		usageQuery2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(usageQuery2)
		usageQuery2.On("OrderBy", mock.Anything, mock.Anything).Return(usageQuery2)
		usageQuery2.On("Limit", 1000).Return(usageQuery2)
		usageQuery2.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

		out, err = engine.CalculateTrending(ctx, time.Now().Add(-time.Hour), 5)
		require.NoError(t, err)
		assert.Empty(t, out)
	})
}
