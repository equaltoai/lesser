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

type fakePatternCacheRepository struct {
	mu                 sync.Mutex
	cacheByKey         map[string]*models.PatternCache
	invalidateCalls    []string
	setCalls           int
	getErr             error
	setErr             error
	invalidateErr      error
	invalidateCallChan chan string
}

func newFakePatternCacheRepository() *fakePatternCacheRepository {
	return &fakePatternCacheRepository{
		cacheByKey:         make(map[string]*models.PatternCache),
		invalidateCallChan: make(chan string, 10),
	}
}

func (f *fakePatternCacheRepository) GetPatternCache(_ context.Context, patternID, patternType string) (*models.PatternCache, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getErr != nil {
		return nil, f.getErr
	}
	cache, ok := f.cacheByKey[patternID+":"+patternType]
	if !ok {
		return nil, errors.New("pattern cache not found")
	}
	return cache, nil
}

func (f *fakePatternCacheRepository) SetPatternCache(_ context.Context, cache *models.PatternCache) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls++
	f.cacheByKey[cache.PatternID+":"+cache.PatternType] = cache
	return nil
}

func (f *fakePatternCacheRepository) InvalidatePatternCache(_ context.Context, patternID, patternType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.invalidateErr != nil {
		return f.invalidateErr
	}
	delete(f.cacheByKey, patternID+":"+patternType)
	f.invalidateCalls = append(f.invalidateCalls, patternID+":"+patternType)
	select {
	case f.invalidateCallChan <- patternID + ":" + patternType:
	default:
	}
	return nil
}

func countSyncMapEntries(m *sync.Map) int {
	count := 0
	m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func TestPatternCacheManager_DefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, 1000, cfg.MaxMemoryPatterns)
	assert.Equal(t, 30*time.Minute, cfg.MemoryCacheTimeout)
	assert.Equal(t, 24*time.Hour, cfg.PersistentCacheTimeout)
	assert.True(t, cfg.PreloadActivePatterns)
	assert.True(t, cfg.EnableStatistics)
	assert.Equal(t, 5*time.Minute, cfg.CleanupInterval)
	assert.True(t, cfg.EnablePersistentCache)
}

func TestPatternCacheManager_GetCompiledPattern_MemoryHitAndExpiry(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{
		MaxMemoryPatterns:      10,
		MemoryCacheTimeout:     time.Second,
		PersistentCacheTimeout: time.Hour,
		EnableStatistics:       true,
		CleanupInterval:        0,
		EnablePersistentCache:  false,
	}

	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	first, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(2), second.HitCount)

	// Force expiry for the cached entry, then ensure we recompile.
	cacheKey := manager.generateCacheKey("p1", "example.com", "url_domain")
	value, ok := manager.urlCache.Load(cacheKey)
	require.True(t, ok)
	cached := value.(*CachedPattern)
	cached.CachedAt = time.Now().Add(-2 * cfg.MemoryCacheTimeout)
	manager.urlCache.Store(cacheKey, cached)

	third, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, int64(1), third.HitCount)
}

func TestPatternCacheManager_GetCompiledPattern_PersistentHitAndExpiry(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{
		MaxMemoryPatterns:      10,
		MemoryCacheTimeout:     time.Minute,
		PersistentCacheTimeout: time.Minute,
		EnableStatistics:       true,
		CleanupInterval:        0,
		EnablePersistentCache:  true,
	}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	repo.cacheByKey["p1:url_domain"] = &models.PatternCache{
		PatternID:       "p1",
		PatternType:     "url_domain",
		CompilationHash: "hash",
		CompiledData:    map[string]any{"data": "x"},
		CreatedAt:       time.Now(),
		LastUsed:        time.Now(),
		CacheHits:       10,
		CompileTime:     1.0,
	}

	got, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "p1", got.PatternID)
	assert.Equal(t, "url_domain", got.PatternType)

	// Expired persistent cache triggers invalidation and recompilation.
	repo.cacheByKey["p2:url_domain"] = &models.PatternCache{
		PatternID:       "p2",
		PatternType:     "url_domain",
		CompilationHash: "hash2",
		CompiledData:    map[string]any{"data": "y"},
		CreatedAt:       time.Now().Add(-2 * cfg.PersistentCacheTimeout),
		LastUsed:        time.Now(),
		CacheHits:       1,
		CompileTime:     1.0,
	}
	got, err = manager.GetCompiledPattern(context.Background(), "p2", "example.com", "url_domain")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.invalidateCalls) > 0
	}, time.Second, 10*time.Millisecond)
}

func TestPatternCacheManager_compilePattern_UnsupportedAndErrors(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{CleanupInterval: 0, EnablePersistentCache: false}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	_, err := manager.compilePattern("x", "unknown")
	require.Error(t, err)

	_, err = manager.compileURLPattern("x", "bad_type")
	require.Error(t, err)

	_, err = manager.compileIPPattern("x", "bad_type")
	require.Error(t, err)
}

func TestPatternCacheManager_InvalidatePattern_RemovesCachesAndPersistent(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{
		MaxMemoryPatterns:      10,
		MemoryCacheTimeout:     time.Minute,
		PersistentCacheTimeout: time.Minute,
		EnableStatistics:       true,
		CleanupInterval:        0,
		EnablePersistentCache:  true,
	}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	_, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	assert.Equal(t, 1, countSyncMapEntries(manager.urlCache))

	require.NoError(t, manager.InvalidatePattern(context.Background(), "p1", "example.com", "url_domain"))
	assert.Equal(t, 0, countSyncMapEntries(manager.urlCache))

	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.invalidateCalls) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestPatternCacheManager_EvictIfNecessary_AndCleanupCache(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{
		MaxMemoryPatterns:      1,
		MemoryCacheTimeout:     time.Millisecond,
		PersistentCacheTimeout: time.Hour,
		EnableStatistics:       true,
		CleanupInterval:        0,
		EnablePersistentCache:  false,
	}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	_, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)
	_, err = manager.GetCompiledPattern(context.Background(), "p2", "example.org", "url_domain")
	require.NoError(t, err)
	assert.Equal(t, 1, countSyncMapEntries(manager.urlCache))

	// Force an expired entry and ensure cleanup removes it.
	manager.urlCache.Range(func(key, value any) bool {
		cached := value.(*CachedPattern)
		cached.CachedAt = time.Now().Add(-10 * cfg.MemoryCacheTimeout)
		manager.urlCache.Store(key, cached)
		return false
	})
	manager.cleanupCache(manager.urlCache)
	assert.Equal(t, 0, countSyncMapEntries(manager.urlCache))
}

func TestPatternCacheManager_cleanup_RemovesExpiredFromBothCaches(t *testing.T) {
	repo := newFakePatternCacheRepository()
	cfg := &CacheConfig{
		MaxMemoryPatterns:      10,
		MemoryCacheTimeout:     time.Hour,
		PersistentCacheTimeout: time.Hour,
		EnableStatistics:       false,
		CleanupInterval:        0,
		EnablePersistentCache:  false,
	}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	expired := &CachedPattern{CachedAt: time.Now().Add(-10 * cfg.MemoryCacheTimeout)}
	fresh := &CachedPattern{CachedAt: time.Now()}

	manager.urlCache.Store("u-expired", expired)
	manager.urlCache.Store("u-fresh", fresh)
	manager.ipCache.Store("ip-expired", expired)
	manager.ipCache.Store("ip-fresh", fresh)

	manager.cleanup()

	assert.Equal(t, 1, countSyncMapEntries(manager.urlCache))
	assert.Equal(t, 1, countSyncMapEntries(manager.ipCache))
}

func TestPatternCacheManager_GetCompiledPattern_CompilesSupportedTypes(t *testing.T) {
	cfg := &CacheConfig{
		MaxMemoryPatterns:      100,
		MemoryCacheTimeout:     time.Hour,
		PersistentCacheTimeout: time.Hour,
		EnableStatistics:       false,
		CleanupInterval:        0,
		EnablePersistentCache:  false,
	}
	manager := NewPatternCacheManager(newFakePatternCacheRepository(), cfg, zap.NewNop())
	ctx := context.Background()

	for _, tt := range []struct {
		id          string
		content     string
		patternType string
	}{
		{id: "u1", content: "https://example.com", patternType: "url_exact"},
		{id: "u2", content: "example.com", patternType: "url_domain"},
		{id: "u3", content: "*.example.com", patternType: "url_subdomain"},
		{id: "u4", content: "/api/*", patternType: "url_path"},
		{id: "u5", content: "param=test", patternType: "url_query"},
		{id: "u6", content: `^https?://.*\\.example\\.com/.*$`, patternType: "url_regex"},
		{id: "ip1", content: "192.168.1.1", patternType: "ip_single"},
		{id: "ip2", content: "192.168.1.0/24", patternType: "ip_cidr"},
		{id: "ip3", content: "192.168.1.1-192.168.1.10", patternType: "ip_range"},
		{id: "ip4", content: `^192\\.168\\.[0-9]+\\.[0-9]+$`, patternType: "ip_regex"},
	} {
		_, err := manager.GetCompiledPattern(ctx, tt.id, tt.content, tt.patternType)
		require.NoError(t, err)
	}
}

func TestPatternCacheManager_StatisticsAndHelpers(t *testing.T) {
	repo := newFakePatternCacheRepository()
	repo.setErr = errors.New("set boom")
	cfg := &CacheConfig{
		MaxMemoryPatterns:      10,
		MemoryCacheTimeout:     time.Minute,
		PersistentCacheTimeout: time.Hour,
		EnableStatistics:       true,
		CleanupInterval:        0,
		EnablePersistentCache:  true,
	}

	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())
	_, err := manager.GetCompiledPattern(context.Background(), "p1", "example.com", "url_domain")
	require.NoError(t, err)

	stats := manager.GetStatistics()
	require.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalLookups, int64(1))

	manager.ResetStatistics()
	stats = manager.GetStatistics()
	assert.Equal(t, int64(0), stats.TotalLookups)

	assert.True(t, isURLPatternType("url_domain"))
	assert.False(t, isURLPatternType("ip_single"))
	assert.True(t, isIPPatternType("ip_single"))
	assert.False(t, isIPPatternType("url_domain"))
}

func TestPatternCacheManager_getFromPersistentCache_RepositoryError(t *testing.T) {
	repo := newFakePatternCacheRepository()
	repo.getErr = errors.New("get boom")
	cfg := &CacheConfig{CleanupInterval: 0, EnableStatistics: true, EnablePersistentCache: true, PersistentCacheTimeout: time.Minute}
	manager := NewPatternCacheManager(repo, cfg, zap.NewNop())

	got := manager.getFromPersistentCache(context.Background(), "p1", "url_domain")
	assert.Nil(t, got)
}
