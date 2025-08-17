package moderation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// PatternRepository defines the interface for pattern storage operations needed by the cache
type PatternRepository interface {
	GetPatternCache(ctx context.Context, patternID, patternType string) (*models.PatternCache, error)
	SetPatternCache(ctx context.Context, cache *models.PatternCache) error
	InvalidatePatternCache(ctx context.Context, patternID, patternType string) error
}

// PatternCacheManager manages in-memory and persistent pattern caching for performance
type PatternCacheManager struct {
	// In-memory cache
	urlCache   *sync.Map // map[string]*CompiledURLPattern
	ipCache    *sync.Map // map[string]*CompiledIPPattern

	// Pattern managers
	urlMatcher *EnhancedURLMatcher
	ipMatcher  *EnhancedIPMatcher

	// Persistent storage
	repository PatternRepository

	// Cache configuration
	config *CacheConfig

	// Statistics
	stats *CacheStatistics

	logger *zap.Logger
}

// CacheConfig defines cache behavior parameters
type CacheConfig struct {
	MaxMemoryPatterns    int           `json:"max_memory_patterns"`     // Maximum patterns in memory
	MemoryCacheTimeout   time.Duration `json:"memory_cache_timeout"`    // How long to keep patterns in memory
	PersistentCacheTimeout time.Duration `json:"persistent_cache_timeout"` // How long to keep in DynamoDB
	PreloadActivePatterns bool          `json:"preload_active_patterns"` // Preload active patterns on startup
	EnableStatistics     bool          `json:"enable_statistics"`       // Track cache statistics
	CleanupInterval      time.Duration `json:"cleanup_interval"`        // How often to cleanup cache
	EnablePersistentCache bool         `json:"enable_persistent_cache"` // Enable DynamoDB caching
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxMemoryPatterns:      1000,
		MemoryCacheTimeout:     30 * time.Minute,
		PersistentCacheTimeout: 24 * time.Hour,
		PreloadActivePatterns:  true,
		EnableStatistics:       true,
		CleanupInterval:        5 * time.Minute,
		EnablePersistentCache:  true,
	}
}

// CacheStatistics tracks cache performance metrics
type CacheStatistics struct {
	MemoryHits           int64 `json:"memory_hits"`
	MemoryMisses         int64 `json:"memory_misses"`
	PersistentHits       int64 `json:"persistent_hits"`
	PersistentMisses     int64 `json:"persistent_misses"`
	CompilationCount     int64 `json:"compilation_count"`
	CacheEvictions       int64 `json:"cache_evictions"`
	TotalLookups         int64 `json:"total_lookups"`
	AverageCompileTime   float64 `json:"average_compile_time"`
	AverageRetrievalTime float64 `json:"average_retrieval_time"`
	LastReset            time.Time `json:"last_reset"`
	mutex                sync.RWMutex
}

// CachedPattern represents a cached compiled pattern with metadata
type CachedPattern struct {
	PatternID        string                 `json:"pattern_id"`
	PatternContent   string                 `json:"pattern_content"`
	PatternType      string                 `json:"pattern_type"`
	CompiledData     interface{}            `json:"compiled_data"`
	CompilationHash  string                 `json:"compilation_hash"`
	CachedAt         time.Time              `json:"cached_at"`
	LastUsed         time.Time              `json:"last_used"`
	HitCount         int64                  `json:"hit_count"`
	CompileTime      float64                `json:"compile_time"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// NewPatternCacheManager creates a new pattern cache manager
func NewPatternCacheManager(repository PatternRepository, config *CacheConfig, logger *zap.Logger) *PatternCacheManager {
	if config == nil {
		config = DefaultCacheConfig()
	}

	manager := &PatternCacheManager{
		urlCache:   &sync.Map{},
		ipCache:    &sync.Map{},
		urlMatcher: NewEnhancedURLMatcher(),
		ipMatcher:  NewEnhancedIPMatcher(),
		repository: repository,
		config:     config,
		stats:      &CacheStatistics{LastReset: time.Now()},
		logger:     logger,
	}

	// Start cleanup routine
	if config.CleanupInterval > 0 {
		go manager.startCleanupRoutine()
	}

	return manager
}

// GetCompiledPattern retrieves a compiled pattern from cache or compiles it
func (c *PatternCacheManager) GetCompiledPattern(ctx context.Context, patternID, patternContent, patternType string) (*CachedPattern, error) {
	start := time.Now()
	defer func() {
		if c.config.EnableStatistics {
			c.updateRetrievalTime(float64(time.Since(start).Nanoseconds()) / 1e6)
		}
	}()

	// Generate cache key
	cacheKey := c.generateCacheKey(patternID, patternContent, patternType)

	// Try memory cache first
	if cached := c.getFromMemoryCache(cacheKey, patternType); cached != nil {
		if c.config.EnableStatistics {
			c.stats.mutex.Lock()
			c.stats.MemoryHits++
			c.stats.TotalLookups++
			c.stats.mutex.Unlock()
		}
		cached.LastUsed = time.Now()
		cached.HitCount++
		return cached, nil
	}

	if c.config.EnableStatistics {
		c.stats.mutex.Lock()
		c.stats.MemoryMisses++
		c.stats.TotalLookups++
		c.stats.mutex.Unlock()
	}

	// Try persistent cache
	if c.config.EnablePersistentCache {
		if cached := c.getFromPersistentCache(ctx, patternID, patternType); cached != nil {
			// Store in memory cache for faster access
			c.storeInMemoryCache(cacheKey, patternType, cached)
			if c.config.EnableStatistics {
				c.stats.mutex.Lock()
				c.stats.PersistentHits++
				c.stats.mutex.Unlock()
			}
			return cached, nil
		}

		if c.config.EnableStatistics {
			c.stats.mutex.Lock()
			c.stats.PersistentMisses++
			c.stats.mutex.Unlock()
		}
	}

	// Compile pattern
	compiled, err := c.compilePattern(patternContent, patternType)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern: %w", err)
	}

	// Create cached pattern
	cached := &CachedPattern{
		PatternID:       patternID,
		PatternContent:  patternContent,
		PatternType:     patternType,
		CompiledData:    compiled.CompiledData,
		CompilationHash: cacheKey,
		CachedAt:        time.Now(),
		LastUsed:        time.Now(),
		HitCount:        1,
		CompileTime:     compiled.CompileTime,
	}

	// Store in caches
	c.storeInMemoryCache(cacheKey, patternType, cached)
	if c.config.EnablePersistentCache {
		go c.storeInPersistentCache(context.Background(), cached)
	}

	if c.config.EnableStatistics {
		c.stats.mutex.Lock()
		c.stats.CompilationCount++
		c.stats.mutex.Unlock()
		c.updateCompileTime(cached.CompileTime)
	}

	return cached, nil
}

// generateCacheKey generates a unique cache key for a pattern
func (c *PatternCacheManager) generateCacheKey(patternID, patternContent, patternType string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", patternID, patternContent, patternType)))
	return fmt.Sprintf("%x", hash)
}

// getFromMemoryCache retrieves a pattern from memory cache
func (c *PatternCacheManager) getFromMemoryCache(cacheKey, patternType string) *CachedPattern {
	var cache *sync.Map
	if isURLPatternType(patternType) {
		cache = c.urlCache
	} else if isIPPatternType(patternType) {
		cache = c.ipCache
	} else {
		return nil
	}

	if value, exists := cache.Load(cacheKey); exists {
		if cached, ok := value.(*CachedPattern); ok {
			// Check if cache entry is still valid
			if time.Since(cached.CachedAt) < c.config.MemoryCacheTimeout {
				return cached
			}
			// Remove expired entry
			cache.Delete(cacheKey)
		}
	}

	return nil
}

// storeInMemoryCache stores a pattern in memory cache
func (c *PatternCacheManager) storeInMemoryCache(cacheKey, patternType string, pattern *CachedPattern) {
	var cache *sync.Map
	if isURLPatternType(patternType) {
		cache = c.urlCache
	} else if isIPPatternType(patternType) {
		cache = c.ipCache
	} else {
		return
	}

	// Check cache size and evict if necessary
	c.evictIfNecessary(cache, patternType)

	cache.Store(cacheKey, pattern)
}

// getFromPersistentCache retrieves a pattern from persistent cache
func (c *PatternCacheManager) getFromPersistentCache(ctx context.Context, patternID, patternType string) *CachedPattern {
	cacheEntry, err := c.repository.GetPatternCache(ctx, patternID, patternType)
	if err != nil {
		return nil
	}

	// Check if cache entry is still valid
	if time.Since(cacheEntry.CreatedAt) > c.config.PersistentCacheTimeout {
		// Expired, remove it
		go func() {
			_ = c.repository.InvalidatePatternCache(context.Background(), patternID, patternType)
		}()
		return nil
	}

	// Convert to CachedPattern
	cached := &CachedPattern{
		PatternID:       patternID,
		PatternContent:  "", // Will need to be set by caller
		PatternType:     patternType,
		CompiledData:    cacheEntry.CompiledData,
		CompilationHash: cacheEntry.CompilationHash,
		CachedAt:        cacheEntry.CreatedAt,
		LastUsed:        cacheEntry.LastUsed,
		HitCount:        cacheEntry.CacheHits,
		CompileTime:     cacheEntry.CompileTime,
	}

	return cached
}

// storeInPersistentCache stores a pattern in persistent cache
func (c *PatternCacheManager) storeInPersistentCache(ctx context.Context, pattern *CachedPattern) {
	cacheEntry := &models.PatternCache{
		PatternID:       pattern.PatternID,
		PatternType:     pattern.PatternType,
		CompilationHash: pattern.CompilationHash,
		CompiledData:    map[string]interface{}{
			"data":         pattern.CompiledData,
			"content":      pattern.PatternContent,
			"compile_time": pattern.CompileTime,
		},
		CompileTime: pattern.CompileTime,
		CacheHits:   pattern.HitCount,
		LastUsed:    pattern.LastUsed,
	}

	if err := c.repository.SetPatternCache(ctx, cacheEntry); err != nil {
		c.logger.Warn("failed to store pattern in persistent cache",
			zap.String("pattern_id", pattern.PatternID),
			zap.Error(err))
	}
}

// compilePattern compiles a pattern and returns compiled data with timing
func (c *PatternCacheManager) compilePattern(patternContent, patternType string) (*CachedPattern, error) {
	start := time.Now()

	var compiledData interface{}
	var err error

	switch {
	case isURLPatternType(patternType):
		compiledData, err = c.compileURLPattern(patternContent, patternType)
	case isIPPatternType(patternType):
		compiledData, err = c.compileIPPattern(patternContent, patternType)
	default:
		return nil, fmt.Errorf("unsupported pattern type: %s", patternType)
	}

	if err != nil {
		return nil, err
	}

	compileTime := float64(time.Since(start).Nanoseconds()) / 1e6

	return &CachedPattern{
		CompiledData: compiledData,
		CompileTime:  compileTime,
	}, nil
}

// compileURLPattern compiles a URL pattern
func (c *PatternCacheManager) compileURLPattern(patternContent, patternType string) (interface{}, error) {
	var urlPatternType URLPatternType
	switch patternType {
	case "url_exact":
		urlPatternType = URLPatternExact
	case "url_domain":
		urlPatternType = URLPatternDomain
	case "url_subdomain":
		urlPatternType = URLPatternSubdomain
	case "url_path":
		urlPatternType = URLPatternPath
	case "url_query":
		urlPatternType = URLPatternQuery
	case "url_regex":
		urlPatternType = URLPatternRegex
	default:
		return nil, fmt.Errorf("invalid URL pattern type: %s", patternType)
	}

	err := c.urlMatcher.CompileURLPattern(patternContent, urlPatternType)
	if err != nil {
		return nil, err
	}

	// Return the pattern content as compiled data (the actual compilation is stored in the matcher)
	return map[string]interface{}{
		"pattern":      patternContent,
		"pattern_type": urlPatternType,
		"compiled":     true,
	}, nil
}

// compileIPPattern compiles an IP pattern
func (c *PatternCacheManager) compileIPPattern(patternContent, patternType string) (interface{}, error) {
	var ipPatternType IPPatternType
	switch patternType {
	case "ip_single":
		ipPatternType = IPPatternSingle
	case "ip_cidr":
		ipPatternType = IPPatternCIDR
	case "ip_range":
		ipPatternType = IPPatternRange
	case "ip_regex":
		ipPatternType = IPPatternRegex
	default:
		return nil, fmt.Errorf("invalid IP pattern type: %s", patternType)
	}

	err := c.ipMatcher.CompileIPPattern(patternContent, ipPatternType)
	if err != nil {
		return nil, err
	}

	// Return the pattern content as compiled data (the actual compilation is stored in the matcher)
	return map[string]interface{}{
		"pattern":      patternContent,
		"pattern_type": ipPatternType,
		"compiled":     true,
	}, nil
}

// MatchURL matches a URL using cached patterns
func (c *PatternCacheManager) MatchURL(ctx context.Context, urlStr string, patterns []*models.EnhancedModerationPattern) (bool, *models.EnhancedModerationPattern, error) {
	// Prepare pattern list for matching
	patternStrings := make([]string, 0, len(patterns))
	patternMap := make(map[string]*models.EnhancedModerationPattern)

	for _, pattern := range patterns {
		if !isURLPatternType(pattern.PatternType) {
			continue
		}

		// Ensure pattern is compiled and cached
		_, err := c.GetCompiledPattern(ctx, pattern.PatternID, pattern.PatternContent, pattern.PatternType)
		if err != nil {
			c.logger.Warn("failed to compile pattern",
				zap.String("pattern_id", pattern.PatternID),
				zap.Error(err))
			continue
		}

		patternStrings = append(patternStrings, pattern.PatternContent)
		patternMap[pattern.PatternContent] = pattern
	}

	// Perform matching
	matched, matchedPattern, err := c.urlMatcher.MatchURL(urlStr, patternStrings)
	if err != nil {
		return false, nil, err
	}

	if matched && matchedPattern != "" {
		if pattern, exists := patternMap[matchedPattern]; exists {
			return true, pattern, nil
		}
	}

	return false, nil, nil
}

// MatchIP matches an IP using cached patterns
func (c *PatternCacheManager) MatchIP(ctx context.Context, ipStr string, patterns []*models.EnhancedModerationPattern) (bool, *models.EnhancedModerationPattern, error) {
	// Prepare pattern list for matching
	patternStrings := make([]string, 0, len(patterns))
	patternMap := make(map[string]*models.EnhancedModerationPattern)

	for _, pattern := range patterns {
		if !isIPPatternType(pattern.PatternType) {
			continue
		}

		// Ensure pattern is compiled and cached
		_, err := c.GetCompiledPattern(ctx, pattern.PatternID, pattern.PatternContent, pattern.PatternType)
		if err != nil {
			c.logger.Warn("failed to compile pattern",
				zap.String("pattern_id", pattern.PatternID),
				zap.Error(err))
			continue
		}

		patternStrings = append(patternStrings, pattern.PatternContent)
		patternMap[pattern.PatternContent] = pattern
	}

	// Perform matching
	matched, matchedPattern, err := c.ipMatcher.MatchIP(ipStr, patternStrings)
	if err != nil {
		return false, nil, err
	}

	if matched && matchedPattern != "" {
		if pattern, exists := patternMap[matchedPattern]; exists {
			return true, pattern, nil
		}
	}

	return false, nil, nil
}

// InvalidatePattern removes a pattern from all caches
func (c *PatternCacheManager) InvalidatePattern(ctx context.Context, patternID, patternContent, patternType string) error {
	cacheKey := c.generateCacheKey(patternID, patternContent, patternType)

	// Remove from memory cache
	if isURLPatternType(patternType) {
		c.urlCache.Delete(cacheKey)
	} else if isIPPatternType(patternType) {
		c.ipCache.Delete(cacheKey)
	}

	// Remove from persistent cache
	if c.config.EnablePersistentCache {
		return c.repository.InvalidatePatternCache(ctx, patternID, patternType)
	}

	return nil
}

// evictIfNecessary evicts old patterns if cache is full
func (c *PatternCacheManager) evictIfNecessary(cache *sync.Map, _ string) {
	// Count current entries
	count := 0
	cache.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	if count >= c.config.MaxMemoryPatterns {
		// Evict oldest entries
		toEvict := make([]string, 0)
		oldestTime := time.Now()
		
		cache.Range(func(key, value interface{}) bool {
			if cached, ok := value.(*CachedPattern); ok {
				if cached.LastUsed.Before(oldestTime) {
					if err := common.ValidateSliceNotEmpty("toEvict", toEvict); err != nil {
						toEvict = append(toEvict, key.(string))
						oldestTime = cached.LastUsed
					} else if cached.LastUsed.Before(oldestTime) {
						toEvict[0] = key.(string)
						oldestTime = cached.LastUsed
					}
				}
			}
			return true
		})

		for _, key := range toEvict {
			cache.Delete(key)
			if c.config.EnableStatistics {
				c.stats.mutex.Lock()
				c.stats.CacheEvictions++
				c.stats.mutex.Unlock()
			}
		}
	}
}

// startCleanupRoutine starts the cache cleanup routine
func (c *PatternCacheManager) startCleanupRoutine() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries from memory cache
func (c *PatternCacheManager) cleanup() {
	c.cleanupCache(c.urlCache)
	c.cleanupCache(c.ipCache)
}

// cleanupCache removes expired entries from a specific cache
func (c *PatternCacheManager) cleanupCache(cache *sync.Map) {
	toDelete := make([]string, 0)

	cache.Range(func(key, value interface{}) bool {
		if cached, ok := value.(*CachedPattern); ok {
			if time.Since(cached.CachedAt) > c.config.MemoryCacheTimeout {
				toDelete = append(toDelete, key.(string))
			}
		}
		return true
	})

	for _, key := range toDelete {
		cache.Delete(key)
	}

	if len(toDelete) > 0 {
		c.logger.Debug("cleaned up expired cache entries",
			zap.Int("count", len(toDelete)))
	}
}

// GetStatistics returns cache performance statistics
func (c *PatternCacheManager) GetStatistics() *CacheStatistics {
	c.stats.mutex.RLock()
	defer c.stats.mutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	return &CacheStatistics{
		MemoryHits:           c.stats.MemoryHits,
		MemoryMisses:         c.stats.MemoryMisses,
		PersistentHits:       c.stats.PersistentHits,
		PersistentMisses:     c.stats.PersistentMisses,
		CompilationCount:     c.stats.CompilationCount,
		CacheEvictions:       c.stats.CacheEvictions,
		TotalLookups:         c.stats.TotalLookups,
		AverageCompileTime:   c.stats.AverageCompileTime,
		AverageRetrievalTime: c.stats.AverageRetrievalTime,
		LastReset:            c.stats.LastReset,
	}
}

// ResetStatistics resets cache statistics
func (c *PatternCacheManager) ResetStatistics() {
	c.stats.mutex.Lock()
	defer c.stats.mutex.Unlock()

	c.stats.MemoryHits = 0
	c.stats.MemoryMisses = 0
	c.stats.PersistentHits = 0
	c.stats.PersistentMisses = 0
	c.stats.CompilationCount = 0
	c.stats.CacheEvictions = 0
	c.stats.TotalLookups = 0
	c.stats.AverageCompileTime = 0
	c.stats.AverageRetrievalTime = 0
	c.stats.LastReset = time.Now()
}

// updateCompileTime updates the average compilation time
func (c *PatternCacheManager) updateCompileTime(compileTime float64) {
	c.stats.mutex.Lock()
	defer c.stats.mutex.Unlock()

	if c.stats.AverageCompileTime == 0 {
		c.stats.AverageCompileTime = compileTime
	} else {
		c.stats.AverageCompileTime = (c.stats.AverageCompileTime + compileTime) / 2
	}
}

// updateRetrievalTime updates the average retrieval time
func (c *PatternCacheManager) updateRetrievalTime(retrievalTime float64) {
	c.stats.mutex.Lock()
	defer c.stats.mutex.Unlock()

	if c.stats.AverageRetrievalTime == 0 {
		c.stats.AverageRetrievalTime = retrievalTime
	} else {
		c.stats.AverageRetrievalTime = (c.stats.AverageRetrievalTime + retrievalTime) / 2
	}
}

// Helper functions

// isURLPatternType checks if pattern type is a URL pattern
func isURLPatternType(patternType string) bool {
	return patternType == "url_exact" || patternType == "url_domain" || 
		   patternType == "url_subdomain" || patternType == "url_path" || 
		   patternType == "url_query" || patternType == "url_regex"
}

// isIPPatternType checks if pattern type is an IP pattern
func isIPPatternType(patternType string) bool {
	return patternType == "ip_single" || patternType == "ip_cidr" || 
		   patternType == "ip_range" || patternType == "ip_regex"
}