// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ModerationMLRepository defines the interface for ML moderation data storage operations.
// This handles moderation samples, model versions, and effectiveness metrics.
type ModerationMLRepository interface {
	// ===== Sample Operations =====

	// CreateSample creates a new moderation training sample
	CreateSample(ctx context.Context, sample *models.ModerationSample) error

	// GetSample retrieves a sample by ID
	GetSample(ctx context.Context, sampleID string) (*models.ModerationSample, error)

	// ListSamplesByLabel retrieves samples with a specific label
	ListSamplesByLabel(ctx context.Context, label string, limit int) ([]*models.ModerationSample, error)

	// ListSamplesByReviewer retrieves samples submitted by a specific reviewer
	ListSamplesByReviewer(ctx context.Context, reviewerID string, limit int) ([]*models.ModerationSample, error)

	// ===== Model Version Operations =====

	// CreateModelVersion creates a new model version record
	CreateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error

	// GetModelVersion retrieves a model version by ID
	GetModelVersion(ctx context.Context, versionID string) (*models.ModerationModelVersion, error)

	// GetActiveModelVersion retrieves the currently active model version
	GetActiveModelVersion(ctx context.Context) (*models.ModerationModelVersion, error)

	// UpdateModelVersion updates an existing model version
	UpdateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error

	// ===== Effectiveness Metrics Operations =====

	// CreateEffectivenessMetric creates a new effectiveness metric record
	CreateEffectivenessMetric(ctx context.Context, metric *models.ModerationEffectivenessMetric) error

	// GetEffectivenessMetric retrieves effectiveness metrics for a pattern/period
	GetEffectivenessMetric(ctx context.Context, patternID, period string, startTime time.Time) (*models.ModerationEffectivenessMetric, error)

	// ListEffectivenessMetricsByPattern retrieves all metrics for a pattern
	ListEffectivenessMetricsByPattern(ctx context.Context, patternID string, limit int) ([]*models.ModerationEffectivenessMetric, error)

	// ListEffectivenessMetricsByPeriod retrieves top-performing patterns for a period
	ListEffectivenessMetricsByPeriod(ctx context.Context, period string, limit int) ([]*models.ModerationEffectivenessMetric, error)
}
