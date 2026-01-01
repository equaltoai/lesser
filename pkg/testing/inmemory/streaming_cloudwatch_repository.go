// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StreamingCloudWatchRepository is a thread-safe in-memory implementation of interfaces.StreamingCloudWatchRepository.
type StreamingCloudWatchRepository struct {
	mu      sync.RWMutex
	metrics map[string]map[string]*models.StreamingCloudWatchMetrics // mediaID -> metricType -> metrics
}

// NewStreamingCloudWatchRepository creates a new in-memory streaming CloudWatch repository.
func NewStreamingCloudWatchRepository() *StreamingCloudWatchRepository {
	return &StreamingCloudWatchRepository{
		metrics: make(map[string]map[string]*models.StreamingCloudWatchMetrics),
	}
}

// getOrCreateMediaMetrics gets or creates the metrics map for a media ID.
func (r *StreamingCloudWatchRepository) getOrCreateMediaMetrics(mediaID string) map[string]*models.StreamingCloudWatchMetrics {
	if _, exists := r.metrics[mediaID]; !exists {
		r.metrics[mediaID] = make(map[string]*models.StreamingCloudWatchMetrics)
	}
	return r.metrics[mediaID]
}

// ===== Quality Metrics Operations =====

// GetQualityBreakdown retrieves cached quality breakdown metrics for a media item.
func (r *StreamingCloudWatchRepository) GetQualityBreakdown(_ context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mediaMetrics, exists := r.metrics[mediaID]; exists {
		if metrics, ok := mediaMetrics["quality"]; ok {
			return metrics, nil
		}
	}
	return nil, storage.ErrNotFound
}

// CacheQualityBreakdown stores quality breakdown metrics in cache.
func (r *StreamingCloudWatchRepository) CacheQualityBreakdown(_ context.Context, mediaID string, qualityMetrics map[string]models.QualityMetric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaMetrics := r.getOrCreateMediaMetrics(mediaID)
	metrics := &models.StreamingCloudWatchMetrics{}
	metrics.SetQualityBreakdown(mediaID, qualityMetrics)
	mediaMetrics["quality"] = metrics
	return nil
}

// ===== Geographic Metrics Operations =====

// GetGeographicData retrieves cached geographic distribution metrics.
func (r *StreamingCloudWatchRepository) GetGeographicData(_ context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mediaMetrics, exists := r.metrics[mediaID]; exists {
		if metrics, ok := mediaMetrics["geographic"]; ok {
			return metrics, nil
		}
	}
	return nil, storage.ErrNotFound
}

// CacheGeographicData stores geographic distribution metrics in cache.
func (r *StreamingCloudWatchRepository) CacheGeographicData(_ context.Context, mediaID string, geoMetrics map[string]models.GeographicMetric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaMetrics := r.getOrCreateMediaMetrics(mediaID)
	metrics := &models.StreamingCloudWatchMetrics{}
	metrics.SetGeographicData(mediaID, geoMetrics)
	mediaMetrics["geographic"] = metrics
	return nil
}

// ===== Concurrent Viewer Operations =====

// GetConcurrentViewers retrieves cached concurrent viewer metrics.
func (r *StreamingCloudWatchRepository) GetConcurrentViewers(_ context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mediaMetrics, exists := r.metrics[mediaID]; exists {
		if metrics, ok := mediaMetrics["concurrent"]; ok {
			return metrics, nil
		}
	}
	return nil, storage.ErrNotFound
}

// CacheConcurrentViewers stores concurrent viewer metrics in cache.
func (r *StreamingCloudWatchRepository) CacheConcurrentViewers(_ context.Context, mediaID string, concurrentMetrics models.ConcurrentViewerMetrics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaMetrics := r.getOrCreateMediaMetrics(mediaID)
	metrics := &models.StreamingCloudWatchMetrics{}
	metrics.SetConcurrentViewers(mediaID, concurrentMetrics)
	mediaMetrics["concurrent"] = metrics
	return nil
}

// ===== Performance Metrics Operations =====

// GetPerformanceMetrics retrieves cached performance metrics.
func (r *StreamingCloudWatchRepository) GetPerformanceMetrics(_ context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mediaMetrics, exists := r.metrics[mediaID]; exists {
		if metrics, ok := mediaMetrics["performance"]; ok {
			return metrics, nil
		}
	}
	return nil, storage.ErrNotFound
}

// CachePerformanceMetrics stores performance metrics in cache.
func (r *StreamingCloudWatchRepository) CachePerformanceMetrics(_ context.Context, mediaID string, perfMetrics models.StreamingPerformanceMetrics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaMetrics := r.getOrCreateMediaMetrics(mediaID)
	metrics := &models.StreamingCloudWatchMetrics{}
	metrics.SetPerformanceMetrics(mediaID, perfMetrics)
	mediaMetrics["performance"] = metrics
	return nil
}

// ===== Aggregate Operations =====

// GetAllCachedMetrics retrieves all cached metrics for a media item.
func (r *StreamingCloudWatchRepository) GetAllCachedMetrics(_ context.Context, mediaID string) (map[string]*models.StreamingCloudWatchMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mediaMetrics, exists := r.metrics[mediaID]; exists {
		// Return a copy to avoid race conditions
		result := make(map[string]*models.StreamingCloudWatchMetrics)
		for k, v := range mediaMetrics {
			result[k] = v
		}
		return result, nil
	}
	return make(map[string]*models.StreamingCloudWatchMetrics), nil
}

// ===== Cleanup Operations =====

// CleanupExpiredMetrics removes expired metrics from cache.
func (r *StreamingCloudWatchRepository) CleanupExpiredMetrics(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for mediaID, mediaMetrics := range r.metrics {
		for metricType, metrics := range mediaMetrics {
			if metrics.IsExpired() {
				delete(mediaMetrics, metricType)
			}
		}
		if len(mediaMetrics) == 0 {
			delete(r.metrics, mediaID)
		}
	}
	return nil
}

// ===== Test Helper Methods =====

// Clear clears all cached data.
func (r *StreamingCloudWatchRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = make(map[string]map[string]*models.StreamingCloudWatchMetrics)
}

// Count returns the total number of cached metrics across all media items.
func (r *StreamingCloudWatchRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, mediaMetrics := range r.metrics {
		count += len(mediaMetrics)
	}
	return count
}

// MediaCount returns the number of media items with cached metrics.
func (r *StreamingCloudWatchRepository) MediaCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.metrics)
}

// Ensure StreamingCloudWatchRepository implements interfaces.StreamingCloudWatchRepository
var _ interfaces.StreamingCloudWatchRepository = (*StreamingCloudWatchRepository)(nil)
