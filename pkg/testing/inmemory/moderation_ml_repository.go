// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// ModerationMLRepository is a thread-safe in-memory implementation of interfaces.ModerationMLRepository.
type ModerationMLRepository struct {
	mu sync.RWMutex

	// Samples by ID: sampleID -> ModerationSample
	samplesByID map[string]*models.ModerationSample

	// Samples by label: label -> []ModerationSample
	samplesByLabel map[string][]*models.ModerationSample

	// Samples by reviewer: reviewerID -> []ModerationSample
	samplesByReviewer map[string][]*models.ModerationSample

	// Model versions by ID: versionID -> ModerationModelVersion
	modelVersions map[string]*models.ModerationModelVersion

	// Active model version ID
	activeModelVersionID string

	// Effectiveness metrics: patternID_period_startTime -> ModerationEffectivenessMetric
	effectivenessMetrics map[string]*models.ModerationEffectivenessMetric

	// Metrics by pattern: patternID -> []ModerationEffectivenessMetric
	metricsByPattern map[string][]*models.ModerationEffectivenessMetric

	// Metrics by period: period -> []ModerationEffectivenessMetric
	metricsByPeriod map[string][]*models.ModerationEffectivenessMetric
}

// NewModerationMLRepository creates a new in-memory moderation ML repository
func NewModerationMLRepository() *ModerationMLRepository {
	return &ModerationMLRepository{
		samplesByID:          make(map[string]*models.ModerationSample),
		samplesByLabel:       make(map[string][]*models.ModerationSample),
		samplesByReviewer:    make(map[string][]*models.ModerationSample),
		modelVersions:        make(map[string]*models.ModerationModelVersion),
		effectivenessMetrics: make(map[string]*models.ModerationEffectivenessMetric),
		metricsByPattern:     make(map[string][]*models.ModerationEffectivenessMetric),
		metricsByPeriod:      make(map[string][]*models.ModerationEffectivenessMetric),
	}
}

// ===== Sample Operations =====

// CreateSample creates a new moderation training sample
func (r *ModerationMLRepository) CreateSample(_ context.Context, sample *models.ModerationSample) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sample.ID == "" {
		sample.ID = uuid.New().String()
	}

	now := time.Now()
	sample.CreatedAt = now
	sample.UpdatedAt = now
	sample.Timestamp = now

	r.samplesByID[sample.ID] = sample
	r.samplesByLabel[sample.Label] = append(r.samplesByLabel[sample.Label], sample)
	r.samplesByReviewer[sample.ReviewerID] = append(r.samplesByReviewer[sample.ReviewerID], sample)

	return nil
}

// GetSample retrieves a sample by ID
func (r *ModerationMLRepository) GetSample(_ context.Context, sampleID string) (*models.ModerationSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sample, exists := r.samplesByID[sampleID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return sample, nil
}

// ListSamplesByLabel retrieves samples with a specific label
func (r *ModerationMLRepository) ListSamplesByLabel(_ context.Context, label string, limit int) ([]*models.ModerationSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	samples := r.samplesByLabel[label]
	if len(samples) > limit {
		samples = samples[:limit]
	}

	result := make([]*models.ModerationSample, len(samples))
	copy(result, samples)
	return result, nil
}

// ListSamplesByReviewer retrieves samples submitted by a specific reviewer
func (r *ModerationMLRepository) ListSamplesByReviewer(_ context.Context, reviewerID string, limit int) ([]*models.ModerationSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	samples := r.samplesByReviewer[reviewerID]
	if len(samples) > limit {
		samples = samples[:limit]
	}

	result := make([]*models.ModerationSample, len(samples))
	copy(result, samples)
	return result, nil
}

// ===== Model Version Operations =====

// CreateModelVersion creates a new model version record
func (r *ModerationMLRepository) CreateModelVersion(_ context.Context, version *models.ModerationModelVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if version.VersionID == "" {
		version.VersionID = uuid.New().String()
	}

	now := time.Now()
	version.CreatedAt = now
	version.UpdatedAt = now

	r.modelVersions[version.VersionID] = version

	// If this is marked as active, update the active version
	if version.IsActive {
		// Deactivate previous active version
		if r.activeModelVersionID != "" {
			if prev, exists := r.modelVersions[r.activeModelVersionID]; exists {
				prev.IsActive = false
			}
		}
		r.activeModelVersionID = version.VersionID
	}

	return nil
}

// GetModelVersion retrieves a model version by ID
func (r *ModerationMLRepository) GetModelVersion(_ context.Context, versionID string) (*models.ModerationModelVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	version, exists := r.modelVersions[versionID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return version, nil
}

// GetActiveModelVersion retrieves the currently active model version
func (r *ModerationMLRepository) GetActiveModelVersion(_ context.Context) (*models.ModerationModelVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.activeModelVersionID == "" {
		return nil, fmt.Errorf("no active model version found")
	}

	version, exists := r.modelVersions[r.activeModelVersionID]
	if !exists {
		return nil, fmt.Errorf("no active model version found")
	}
	return version, nil
}

// UpdateModelVersion updates an existing model version
func (r *ModerationMLRepository) UpdateModelVersion(_ context.Context, version *models.ModerationModelVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.modelVersions[version.VersionID]; !exists {
		return storage.ErrNotFound
	}

	version.UpdatedAt = time.Now()
	r.modelVersions[version.VersionID] = version

	// Handle active status changes
	if version.IsActive && r.activeModelVersionID != version.VersionID {
		// Deactivate previous active version
		if r.activeModelVersionID != "" {
			if prev, exists := r.modelVersions[r.activeModelVersionID]; exists {
				prev.IsActive = false
			}
		}
		r.activeModelVersionID = version.VersionID
	} else if !version.IsActive && r.activeModelVersionID == version.VersionID {
		r.activeModelVersionID = ""
	}

	return nil
}

// ===== Effectiveness Metrics Operations =====

// CreateEffectivenessMetric creates a new effectiveness metric record
func (r *ModerationMLRepository) CreateEffectivenessMetric(_ context.Context, metric *models.ModerationEffectivenessMetric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	metric.CreatedAt = now
	metric.UpdatedAt = now
	metric.CalculateMetrics()

	key := fmt.Sprintf("%s_%s_%s", metric.PatternID, metric.Period, metric.StartTime.Format(time.RFC3339))
	r.effectivenessMetrics[key] = metric
	r.metricsByPattern[metric.PatternID] = append(r.metricsByPattern[metric.PatternID], metric)
	r.metricsByPeriod[metric.Period] = append(r.metricsByPeriod[metric.Period], metric)

	return nil
}

// GetEffectivenessMetric retrieves effectiveness metrics for a pattern/period
func (r *ModerationMLRepository) GetEffectivenessMetric(_ context.Context, patternID, period string, startTime time.Time) (*models.ModerationEffectivenessMetric, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s_%s_%s", patternID, period, startTime.Format(time.RFC3339))
	metric, exists := r.effectivenessMetrics[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return metric, nil
}

// ListEffectivenessMetricsByPattern retrieves all metrics for a pattern
func (r *ModerationMLRepository) ListEffectivenessMetricsByPattern(_ context.Context, patternID string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	metrics := r.metricsByPattern[patternID]

	// Sort by start time descending
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].StartTime.After(metrics[j].StartTime)
	})

	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	result := make([]*models.ModerationEffectivenessMetric, len(metrics))
	copy(result, metrics)
	return result, nil
}

// ListEffectivenessMetricsByPeriod retrieves top-performing patterns for a period
func (r *ModerationMLRepository) ListEffectivenessMetricsByPeriod(_ context.Context, period string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	metrics := r.metricsByPeriod[period]

	// Sort by F1 score descending
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].F1Score > metrics[j].F1Score
	})

	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	result := make([]*models.ModerationEffectivenessMetric, len(metrics))
	copy(result, metrics)
	return result, nil
}

// Clear clears all data (test helper)
func (r *ModerationMLRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.samplesByID = make(map[string]*models.ModerationSample)
	r.samplesByLabel = make(map[string][]*models.ModerationSample)
	r.samplesByReviewer = make(map[string][]*models.ModerationSample)
	r.modelVersions = make(map[string]*models.ModerationModelVersion)
	r.activeModelVersionID = ""
	r.effectivenessMetrics = make(map[string]*models.ModerationEffectivenessMetric)
	r.metricsByPattern = make(map[string][]*models.ModerationEffectivenessMetric)
	r.metricsByPeriod = make(map[string][]*models.ModerationEffectivenessMetric)
}

// Ensure ModerationMLRepository implements interfaces.ModerationMLRepository
var _ interfaces.ModerationMLRepository = (*ModerationMLRepository)(nil)
