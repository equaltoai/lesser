package moderation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeEnhancedPatternRepository struct {
	mu sync.Mutex

	patterns map[string]*models.EnhancedModerationPattern
	caches   map[string]*models.PatternCache

	recordCalls int
	recordErr   error
}

func newFakeEnhancedPatternRepository() *fakeEnhancedPatternRepository {
	return &fakeEnhancedPatternRepository{
		patterns: make(map[string]*models.EnhancedModerationPattern),
		caches:   make(map[string]*models.PatternCache),
	}
}

func (f *fakeEnhancedPatternRepository) CreatePattern(_ context.Context, pattern *models.EnhancedModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patterns[pattern.PatternID] = pattern
	return nil
}

func (f *fakeEnhancedPatternRepository) GetPattern(_ context.Context, patternID string) (*models.EnhancedModerationPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.patterns[patternID]
	if !ok {
		return nil, errors.New("not found")
	}
	return p, nil
}

func (f *fakeEnhancedPatternRepository) UpdatePattern(_ context.Context, pattern *models.EnhancedModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patterns[pattern.PatternID] = pattern
	return nil
}

func (f *fakeEnhancedPatternRepository) DeletePattern(_ context.Context, patternID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.patterns, patternID)
	return nil
}

func (f *fakeEnhancedPatternRepository) GetActivePatterns(_ context.Context, limit int) ([]*models.EnhancedModerationPattern, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]*models.EnhancedModerationPattern, 0)
	for _, p := range f.patterns {
		if !p.Active {
			continue
		}
		out = append(out, p)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeEnhancedPatternRepository) RecordMatch(_ context.Context, _ string, _ bool, _ bool, _ float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordCalls++
	return f.recordErr
}

func (f *fakeEnhancedPatternRepository) GetPatternStatistics(_ context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{"ok": true}, nil
}

func (f *fakeEnhancedPatternRepository) GetPatternCache(_ context.Context, patternID, patternType string) (*models.PatternCache, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.caches[patternID+":"+patternType], nil
}

func (f *fakeEnhancedPatternRepository) SetPatternCache(_ context.Context, cache *models.PatternCache) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.caches[cache.PatternID+":"+cache.PatternType] = cache
	return nil
}

func (f *fakeEnhancedPatternRepository) InvalidatePatternCache(_ context.Context, patternID, patternType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.caches, patternID+":"+patternType)
	return nil
}

func TestPatternManager_AnalyzePatternEffectiveness_AndOptimizePatterns(t *testing.T) {
	storage := newFakePatternStorage()
	pm := &PatternManager{storage: storage}

	now := time.Now()
	patterns := []*ModerationPattern{
		{ID: "p1", Name: "p1", Type: "keyword", Content: "bad", Severity: "low", Active: true, MatchCount: 10, FalsePositiveCount: 9, LastMatch: now},
		{ID: "p2", Name: "p2", Type: "regex", Content: "x", Severity: "medium", Active: true, MatchCount: 10, FalsePositiveCount: 8, LastMatch: now},
		{ID: "p3", Name: "p3", Type: "regex", Content: "y", Severity: "high", Active: true, MatchCount: 0, CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "p4", Name: "p4", Type: "regex", Content: "z", Severity: "high", Active: true, MatchCount: 100, FalsePositiveCount: 0, LastMatch: now},
	}
	for _, p := range patterns {
		require.NoError(t, storage.CreateModerationPattern(context.Background(), p))
	}

	report, err := pm.AnalyzePatternEffectiveness(context.Background())
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, 4, report.TotalPatterns)
	assert.NotEmpty(t, report.PatternAnalysis)
	assert.NotEmpty(t, report.Recommendations)
	assert.Contains(t, report.Recommendations, "High number of ineffective patterns - consider pattern cleanup")
	assert.Contains(t, report.Recommendations, "High number of regex patterns - consider performance impact")
	assert.Contains(t, report.Recommendations, "Consider adding more patterns for comprehensive moderation")

	optimizations, err := pm.OptimizePatterns(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, optimizations)
}

func TestPatternManager_MatchDomainAndIP_EnhancedAndFallback(t *testing.T) {
	pm := &PatternManager{enhancedEnabled: false}
	matched, txt := pm.matchDomain("example.com", "visit example.com now")
	assert.True(t, matched)
	assert.Equal(t, "example.com", txt)

	matched, txt = pm.matchIP("192.168.1.1", "blocked ip 192.168.1.1")
	assert.True(t, matched)
	assert.Equal(t, "192.168.1.1", txt)

	// Enhanced path uses the cache manager and requires a parseable URL/IP string.
	cfg := &CacheConfig{CleanupInterval: 0, EnablePersistentCache: false}
	cacheManager := NewPatternCacheManager(nil, cfg, zap.NewNop())
	pm = &PatternManager{enhancedEnabled: true, cacheManager: cacheManager, logger: zap.NewNop()}

	matched, txt = pm.matchDomain("example.com", "https://example.com/path")
	assert.True(t, matched)
	assert.Equal(t, "example.com", txt)

	matched, txt = pm.matchIP("192.168.1.1", "192.168.1.1")
	assert.True(t, matched)
	assert.Equal(t, "192.168.1.1", txt)
}

func TestPatternManager_EnhancedCRUD_AndMatchContentEnhanced(t *testing.T) {
	repo := newFakeEnhancedPatternRepository()
	cacheManager := NewPatternCacheManager(repo, &CacheConfig{CleanupInterval: 0, EnablePersistentCache: false}, zap.NewNop())

	pm := &PatternManager{
		enhancedRepo:    repo,
		cacheManager:    cacheManager,
		enhancedEnabled: true,
		logger:          zap.NewNop(),
	}

	urlPattern := &models.EnhancedModerationPattern{
		PatternID:       "u1",
		Name:            "url",
		PatternType:     "url_domain",
		PatternContent:  "example.com",
		Category:        "spam",
		Severity:        "high",
		Action:          "block",
		Priority:        10,
		Active:          true,
		ConfidenceScore: 0.9,
		Effectiveness:   0.9,
	}
	ipPattern := &models.EnhancedModerationPattern{
		PatternID:       "i1",
		Name:            "ip",
		PatternType:     "ip_single",
		PatternContent:  "192.168.1.1",
		Category:        "spam",
		Severity:        "medium",
		Action:          "flag",
		Priority:        5,
		Active:          true,
		ConfidenceScore: 0.8,
		Effectiveness:   0.8,
	}
	require.NoError(t, repo.CreatePattern(context.Background(), urlPattern))
	require.NoError(t, repo.CreatePattern(context.Background(), ipPattern))

	require.NoError(t, pm.CreateEnhancedPattern(context.Background(), &models.EnhancedModerationPattern{
		PatternID:      "c1",
		Name:           "created",
		PatternType:    "url_domain",
		PatternContent: "created.example",
		Active:         false,
	}))

	matches, err := pm.MatchContentEnhanced(context.Background(), &ContentToModerate{
		Text: "check https://example.com/path and 192.168.1.1",
	})
	require.NoError(t, err)
	require.Len(t, matches, 3)

	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.recordCalls >= 2
	}, time.Second, 10*time.Millisecond)

	// CRUD helpers.
	require.NotNil(t, pm.GetCacheStatistics())
	_, err = pm.GetActiveEnhancedPatterns(context.Background(), 10)
	require.NoError(t, err)

	_, err = pm.ValidateEnhancedPattern(context.Background(), urlPattern)
	require.ErrorIs(t, err, ErrEnhancedPatternValidationNotAvailable)

	stats, err := pm.GetPatternStatistics(context.Background())
	require.NoError(t, err)
	assert.Equal(t, true, stats["ok"])

	got, err := pm.GetEnhancedPattern(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, "u1", got.PatternID)

	require.NoError(t, pm.UpdateEnhancedPattern(context.Background(), got))
	require.NoError(t, pm.DeleteEnhancedPattern(context.Background(), "u1"))

	constructed := NewEnhancedPatternManager(nil, repo, zap.NewNop())
	assert.True(t, constructed.IsEnhancedEnabled())
}

func TestPatternManager_EnhancedGuardRails(t *testing.T) {
	pm := NewPatternManager()

	_, err := pm.GetEnhancedPattern(context.Background(), "p1")
	require.ErrorIs(t, err, ErrEnhancedPatternsNotAvailable)

	err = pm.CreateEnhancedPattern(context.Background(), &models.EnhancedModerationPattern{})
	require.ErrorIs(t, err, ErrEnhancedPatternsNotAvailable)
}

func TestPatternManager_CreateEnhancedPattern_ValidationAndMetadata(t *testing.T) {
	repo := newFakeEnhancedPatternRepository()
	pm := NewEnhancedPatternManager(nil, repo, zap.NewNop())

	okPattern := &models.EnhancedModerationPattern{
		PatternID:      "p1",
		Name:           "p1",
		PatternType:    URLPatternDomainStr,
		PatternContent: "example.com",
		Active:         true,
	}
	require.NoError(t, pm.CreateEnhancedPattern(context.Background(), okPattern))
	assert.Greater(t, okPattern.ValidationScore, 0.0)

	result, err := pm.ValidateEnhancedPattern(context.Background(), okPattern)
	require.NoError(t, err)
	assert.True(t, result.Valid)

	badPattern := &models.EnhancedModerationPattern{
		PatternID:      "p2",
		Name:           "p2",
		PatternType:    URLPatternDomainStr,
		PatternContent: "",
		Active:         true,
	}
	err = pm.CreateEnhancedPattern(context.Background(), badPattern)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPatternValidationFailed)
}
