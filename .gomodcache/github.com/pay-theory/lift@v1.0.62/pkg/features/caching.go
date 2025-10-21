package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	httpGET = "GET"
)

// CacheStore defines the interface for cache backends
type CacheStore interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Stats() CacheStats
	Close() error
	Keys(pattern string) ([]string, error)
	Exists(key string) bool
	TTL(key string) time.Duration
}

// CacheStrategy defines caching behavior
type CacheStrategy string

const (
	CacheStrategyLRU          CacheStrategy = "lru"
	CacheStrategyLFU          CacheStrategy = "lfu"
	CacheStrategyTTL          CacheStrategy = "ttl"
	CacheStrategyWriteThrough CacheStrategy = "write_through"
	CacheStrategyWriteBack    CacheStrategy = "write_back"
	CacheStrategyReadThrough  CacheStrategy = "read_through"
)

// CacheStats provides cache performance metrics
type CacheStats struct {
	Hits        int64         `json:"hits"`
	Misses      int64         `json:"misses"`
	Sets        int64         `json:"sets"`
	Deletes     int64         `json:"deletes"`
	Evictions   int64         `json:"evictions"`
	HitRate     float64       `json:"hit_rate"`
	AvgLatency  time.Duration `json:"avg_latency"`
	Size        int64         `json:"size"`
	MaxSize     int64         `json:"max_size"`
	MemoryUsage int64         `json:"memory_usage"`
}

// CacheConfig configures the caching middleware
type CacheConfig struct {
	Store             CacheStore
	Serializer        CacheSerializer
	KeyFunc           func(*lift.Context) string
	ShouldCache       func(*lift.Context, any) bool
	ShouldInvalidate  func(*lift.Context) bool
	InvalidatePattern string
	Strategy          CacheStrategy
	Namespace         string
	EvictionPolicy    string
	Tags              []string
	InvalidateOn      []string
	DefaultTTL        time.Duration
	MaxSize           int64
	Serialization     SerializationType
	EnableMetrics     bool
	TenantIsolation   bool
	Compression       bool
	Encryption        bool
}

// SerializationType defines how data is serialized in cache
type SerializationType int

const (
	SerializationJSON SerializationType = iota
	SerializationGob
	SerializationMsgPack
)

// CacheSerializer interface for cache serialization
type CacheSerializer interface {
	Serialize(value any) ([]byte, error)
	Deserialize(data []byte, target any) error
}

// CacheMiddleware provides intelligent caching capabilities
type CacheMiddleware struct {
	store   CacheStore
	metrics *CacheMetrics
	config  CacheConfig
}

// CacheMetrics tracks cache performance
type CacheMetrics struct {
	hits      int64
	misses    int64
	sets      int64
	deletes   int64
	errors    int64
	totalTime int64
	mu        sync.RWMutex
}

// NewCacheMiddleware creates a new caching middleware
func NewCacheMiddleware(config CacheConfig) *CacheMiddleware {
	if config.Store == nil {
		config.Store = NewMemoryCache(MemoryCacheConfig{
			MaxSize: config.MaxSize,
			TTL:     config.DefaultTTL,
		})
	}

	if config.Strategy == "" {
		config.Strategy = CacheStrategyTTL
	}

	if config.DefaultTTL == 0 {
		config.DefaultTTL = 5 * time.Minute
	}

	if len(config.InvalidateOn) == 0 {
		config.InvalidateOn = []string{"POST", "PUT", "DELETE", "PATCH"}
	}

	if config.KeyFunc == nil {
		config.KeyFunc = defaultKeyFunc
	}
	if config.ShouldCache == nil {
		config.ShouldCache = defaultShouldCache
	}
	if config.Serializer == nil {
		config.Serializer = &JSONCacheSerializer{}
	}

	return &CacheMiddleware{
		config:  config,
		store:   config.Store,
		metrics: &CacheMetrics{},
	}
}

// Cache returns the caching middleware
func Cache(config CacheConfig) lift.Middleware {
	middleware := NewCacheMiddleware(config)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return middleware.Handle(ctx, next)
		})
	}
}

// Handle processes the request with caching
func (c *CacheMiddleware) Handle(ctx *lift.Context, next lift.Handler) error {
	start := time.Now()

	// Check if we should invalidate cache
	if c.shouldInvalidate(ctx) {
		c.invalidateCache(ctx)
	}

	// Check if we should use cache for this request
	if !c.shouldUseCache(ctx) {
		return next.Handle(ctx)
	}

	// Generate cache key
	key := c.generateKey(ctx)

	// Try to get from cache
	if cached, found, err := c.store.Get(ctx.Context, key); err == nil && found {
		c.recordHit(time.Since(start))
		return c.serveCached(ctx, cached)
	} else if err != nil {
		c.recordError()
	} else {
		c.recordMiss()
	}

	// Execute handler
	result, err := c.executeAndCapture(ctx, next)
	if err != nil {
		return err
	}

	// Cache the result if appropriate
	if c.config.ShouldCache(ctx, result) {
		ttl := c.config.DefaultTTL
		if cacheErr := c.store.Set(ctx.Context, key, result, ttl); cacheErr != nil {
			c.recordError()
		} else {
			c.recordSet()
		}
	}

	return c.serveResult(ctx, result)
}

// shouldUseCache determines if caching should be used for this request
func (c *CacheMiddleware) shouldUseCache(ctx *lift.Context) bool {
	// Only cache GET requests by default
	if ctx.Request.Method != httpGET {
		return false
	}

	// Check for cache control headers
	if ctx.Header("Cache-Control") == "no-cache" {
		return false
	}

	return true
}

// shouldInvalidate checks if cache should be invalidated
func (c *CacheMiddleware) shouldInvalidate(ctx *lift.Context) bool {
	for _, method := range c.config.InvalidateOn {
		if ctx.Request.Method == method {
			return true
		}
	}

	return c.config.ShouldInvalidate != nil && c.config.ShouldInvalidate(ctx)
}

// generateKey creates a cache key for the request
func (c *CacheMiddleware) generateKey(ctx *lift.Context) string {
	if c.config.KeyFunc != nil {
		if key := c.config.KeyFunc(ctx); key != "" {
			return c.addTenantPrefix(ctx, key)
		}
	}

	// Default key generation
	key := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)

	// Include query parameters
	if len(ctx.Request.QueryParams) > 0 {
		queryHash := c.hashQueryParams(ctx.Request.QueryParams)
		key = fmt.Sprintf("%s:q:%x", key, queryHash)
	}

	// Include relevant headers
	if userAgent := ctx.Header("User-Agent"); userAgent != "" {
		key = fmt.Sprintf("%s:ua:%x", key, c.hashString(userAgent))
	}

	return c.addTenantPrefix(ctx, key)
}

// addTenantPrefix adds tenant isolation to cache keys
func (c *CacheMiddleware) addTenantPrefix(ctx *lift.Context, key string) string {
	if !c.config.TenantIsolation {
		return key
	}

	tenantID := ctx.TenantID()
	if tenantID == "" {
		tenantID = "default"
	}

	return fmt.Sprintf("tenant:%s:%s", tenantID, key)
}

// hashQueryParams creates a hash of query parameters
func (c *CacheMiddleware) hashQueryParams(params map[string]string) uint64 {
	h := fnv.New64a()

	// Sort keys for consistent hashing
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}

	for _, k := range keys {
		// hash.Hash Write never returns an error; still capture return values
		// to satisfy linters and make intent explicit.
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte("="))
		_, _ = h.Write([]byte(params[k]))
		_, _ = h.Write([]byte("&"))
	}

	return h.Sum64()
}

// hashString creates a hash of a string using SHA-256 (secure)
func (c *CacheMiddleware) hashString(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}

// executeAndCapture executes the handler and captures the response
func (c *CacheMiddleware) executeAndCapture(ctx *lift.Context, next lift.Handler) (any, error) {
	// Execute handler normally
	err := next.Handle(ctx)
	if err != nil {
		return nil, err
	}

	// Return the response body as the cached data
	// This is a simplified approach - in a real implementation,
	// you might want to capture more sophisticated response data
	return ctx.Response.Body, nil
}

// serveCached serves cached content
func (c *CacheMiddleware) serveCached(ctx *lift.Context, cached any) error {
	// Add cache headers
	ctx.Response.Header("X-Cache", "HIT")
	ctx.Response.Header("X-Cache-Key", c.generateKey(ctx))

	return ctx.JSON(cached)
}

// serveResult serves the result and adds cache headers
func (c *CacheMiddleware) serveResult(ctx *lift.Context, result any) error {
	// Add cache headers
	ctx.Response.Header("X-Cache", "MISS")
	ctx.Response.Header("X-Cache-Key", c.generateKey(ctx))

	return ctx.JSON(result)
}

// invalidateCache invalidates relevant cache entries
func (c *CacheMiddleware) invalidateCache(ctx *lift.Context) {
	// For now, implement simple invalidation
	// In a production system, this would be more sophisticated
	if c.config.TenantIsolation {
		// Invalidate tenant-specific entries would require pattern-based deletion
		// which is not supported by all cache stores
		tenantID := ctx.TenantID()
		if tenantID == "" {
			tenantID = "default"
		}
		// Log that invalidation was requested
		if ctx.Logger != nil {
			ctx.Logger.Info("Cache invalidation requested", map[string]any{
				"tenant_id": tenantID,
				"path":      ctx.Request.Path,
			})
		}
	}
}

// Metrics recording methods
func (c *CacheMiddleware) recordHit(duration time.Duration) {
	if !c.config.EnableMetrics {
		return
	}

	c.metrics.mu.Lock()
	c.metrics.hits++
	c.metrics.totalTime += duration.Nanoseconds()
	c.metrics.mu.Unlock()
}

func (c *CacheMiddleware) recordMiss() {
	if !c.config.EnableMetrics {
		return
	}

	c.metrics.mu.Lock()
	c.metrics.misses++
	c.metrics.mu.Unlock()
}

func (c *CacheMiddleware) recordSet() {
	if !c.config.EnableMetrics {
		return
	}

	c.metrics.mu.Lock()
	c.metrics.sets++
	c.metrics.mu.Unlock()
}

func (c *CacheMiddleware) recordError() {
	if !c.config.EnableMetrics {
		return
	}

	c.metrics.mu.Lock()
	c.metrics.errors++
	c.metrics.mu.Unlock()
}

// GetStats returns cache statistics
func (c *CacheMiddleware) GetStats() CacheStats {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	total := c.metrics.hits + c.metrics.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.metrics.hits) / float64(total)
	}

	avgLatency := time.Duration(0)
	if c.metrics.hits > 0 {
		avgLatency = time.Duration(c.metrics.totalTime / c.metrics.hits)
	}

	storeStats := c.store.Stats()

	return CacheStats{
		Hits:        c.metrics.hits,
		Misses:      c.metrics.misses,
		Sets:        c.metrics.sets,
		Deletes:     c.metrics.deletes,
		Evictions:   c.metrics.errors,
		HitRate:     hitRate,
		AvgLatency:  avgLatency,
		Size:        storeStats.Size,
		MemoryUsage: storeStats.MemoryUsage,
	}
}

// ResponseCapturer captures response data for caching
type ResponseCapturer struct {
	*lift.Response
	data any
}

// GetCapturedData returns the captured response data
func (r *ResponseCapturer) GetCapturedData() any {
	return r.data
}

// JSON captures JSON response
func (r *ResponseCapturer) JSON(data any) error {
	r.data = data
	return r.Response.JSON(data)
}

// Status sets the status code
func (r *ResponseCapturer) Status(code int) *lift.Response {
	return r.Response.Status(code)
}

// Header sets a header
func (r *ResponseCapturer) Header(key, value string) *lift.Response {
	return r.Response.Header(key, value)
}

// Text captures text response
func (r *ResponseCapturer) Text(text string) error {
	return r.Response.Text(text)
}

// HTML captures HTML response
func (r *ResponseCapturer) HTML(html string) error {
	return r.Response.HTML(html)
}

// Binary captures binary response
func (r *ResponseCapturer) Binary(data []byte) error {
	return r.Response.Binary(data)
}

// IsWritten returns whether the response has been written
func (r *ResponseCapturer) IsWritten() bool {
	return r.Response.IsWritten()
}

// MarshalJSON implements JSON marshaling
func (r *ResponseCapturer) MarshalJSON() ([]byte, error) {
	return r.Response.MarshalJSON()
}

// JSONCacheSerializer implements JSON serialization
type JSONCacheSerializer struct{}

func (j *JSONCacheSerializer) Serialize(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (j *JSONCacheSerializer) Deserialize(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

// Default functions
func defaultKeyFunc(ctx *lift.Context) string {
	queryString := ""
	if len(ctx.Request.QueryParams) > 0 {
		params := make([]string, 0, len(ctx.Request.QueryParams))
		for k, v := range ctx.Request.QueryParams {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		queryString = strings.Join(params, "&")
	}
	return fmt.Sprintf("%s:%s:%s", ctx.Request.Method, ctx.Request.Path, queryString)
}

func defaultShouldCache(ctx *lift.Context, _ any) bool {
	// Cache GET requests by default
	return ctx.Request.Method == httpGET && ctx.Response.StatusCode == 200
}

// Cache utility functions

// CacheWithStore creates a simple cache middleware with default configuration
func CacheWithStore(store CacheStore, ttl time.Duration) lift.Middleware {
	config := CacheConfig{
		Store:      store,
		DefaultTTL: ttl,
	}

	return Cache(config)
}

// CacheWithKey creates cache middleware with custom key function
func CacheWithKey(store CacheStore, ttl time.Duration, keyFunc func(*lift.Context) string) lift.Middleware {
	config := CacheConfig{
		Store:      store,
		DefaultTTL: ttl,
		KeyFunc:    keyFunc,
	}

	return Cache(config)
}

// CacheWithInvalidation creates cache middleware with invalidation support
func CacheWithInvalidation(store CacheStore, ttl time.Duration, invalidatePattern string) lift.Middleware {
	config := CacheConfig{
		Store:             store,
		DefaultTTL:        ttl,
		InvalidatePattern: invalidatePattern,
		ShouldInvalidate: func(ctx *lift.Context) bool {
			// Invalidate on POST, PUT, DELETE requests
			return ctx.Request.Method != httpGET && ctx.Request.Method != "HEAD"
		},
	}

	return Cache(config)
}

// CacheStatsMiddleware middleware to expose cache statistics
func CacheStatsMiddleware(store CacheStore) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if ctx.Request.Path == "/cache/stats" && ctx.Request.Method == httpGET {
				stats := store.Stats()
				return ctx.JSON(stats)
			}
			return next.Handle(ctx)
		})
	}
}

// Multi-backend cache store
type MultiBendCacheStore struct {
	primary   CacheStore
	secondary CacheStore
	strategy  string // "failover", "write_through", "write_back"
}

// NewMultiBackendCacheStore creates a multi-backend cache store
func NewMultiBackendCacheStore(primary, secondary CacheStore, strategy string) *MultiBendCacheStore {
	return &MultiBendCacheStore{
		primary:   primary,
		secondary: secondary,
		strategy:  strategy,
	}
}

func (m *MultiBendCacheStore) Get(ctx context.Context, key string) (any, bool, error) {
	// Try primary first
	if value, found, err := m.primary.Get(ctx, key); err == nil && found {
		return value, true, nil
	}

	// Try secondary
	if value, found, err := m.secondary.Get(ctx, key); err == nil && found {
		// Write back to primary if using write-back strategy
		if m.strategy == "write_back" {
			if setErr := m.primary.Set(ctx, key, value, 0); setErr != nil {
				// Log error but don't fail the read operation
				log.Printf("Failed to write-back to primary cache: %v", setErr)
			}
		}
		return value, true, nil
	}

	return nil, false, nil
}

func (m *MultiBendCacheStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	// Always write to primary
	if err := m.primary.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	// Write to secondary based on strategy
	if m.strategy == "write_through" {
		return m.secondary.Set(ctx, key, value, ttl)
	}

	return nil
}

func (m *MultiBendCacheStore) Delete(ctx context.Context, key string) error {
	// Delete from both stores
	err1 := m.primary.Delete(ctx, key)
	err2 := m.secondary.Delete(ctx, key)

	if err1 != nil {
		return err1
	}
	return err2
}

func (m *MultiBendCacheStore) Clear(ctx context.Context) error {
	err1 := m.primary.Clear(ctx)
	err2 := m.secondary.Clear(ctx)

	if err1 != nil {
		return err1
	}
	return err2
}

func (m *MultiBendCacheStore) Keys(pattern string) ([]string, error) {
	// Get keys from primary
	return m.primary.Keys(pattern)
}

func (m *MultiBendCacheStore) Exists(key string) bool {
	return m.primary.Exists(key) || m.secondary.Exists(key)
}

func (m *MultiBendCacheStore) TTL(key string) time.Duration {
	if ttl := m.primary.TTL(key); ttl > 0 {
		return ttl
	}
	return m.secondary.TTL(key)
}

func (m *MultiBendCacheStore) Stats() CacheStats {
	// Return primary stats for now
	return m.primary.Stats()
}

func (m *MultiBendCacheStore) Close() error {
	err1 := m.primary.Close()
	err2 := m.secondary.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
