package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ModerationMLRepository handles ML moderation data storage operations
type ModerationMLRepository struct {
	*EnhancedBaseRepository[*models.ModerationSample]
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewModerationMLRepository creates a new moderation ML repository
func NewModerationMLRepository(db core.DB, tableName string, logger *zap.Logger) *ModerationMLRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.ModerationSample](
		db, tableName, logger, nil, "ModerationMLRepository", "moderation_ml_sample",
	)

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &ModerationMLRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		tableName:              tableName,
		logger:                 logger,
	}
}

// CreateSample creates a new moderation training sample
func (r *ModerationMLRepository) CreateSample(ctx context.Context, sample *models.ModerationSample) error {
	if sample.ID == "" {
		sample.ID = uuid.New().String()
	}
	now := time.Now()
	sample.CreatedAt = now
	sample.UpdatedAt = now
	sample.Timestamp = now

	if err := sample.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	if err := r.Create(ctx, sample); err != nil {
		r.logger.Error("failed to create moderation sample",
			zap.Error(err),
			zap.String("sample_id", sample.ID))
		return err
	}

	r.logger.Debug("created moderation sample",
		zap.String("sample_id", sample.ID),
		zap.String("label", sample.Label))

	return nil
}

// GetSample retrieves a sample by ID
func (r *ModerationMLRepository) GetSample(ctx context.Context, sampleID string) (*models.ModerationSample, error) {
	var sample models.ModerationSample

	err := r.db.WithContext(ctx).Model(&sample).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("SAMPLEID#%s", sampleID)).
		First(&sample)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("sample not found: %s", sampleID)
		}
		return nil, err
	}

	return &sample, nil
}

// ListSamplesByLabel retrieves samples with a specific label
func (r *ModerationMLRepository) ListSamplesByLabel(ctx context.Context, label string, limit int) ([]*models.ModerationSample, error) {
	if limit <= 0 {
		limit = 100
	}

	var samples []*models.ModerationSample
	var results []models.ModerationSample

	query := r.db.WithContext(ctx).Model(&models.ModerationSample{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("LABEL#%s", label)).
		Limit(limit)

	err := query.Scan(&results)
	if err != nil {
		return nil, err
	}

	for i := range results {
		samples = append(samples, &results[i])
	}

	return samples, nil
}

// ListSamplesByReviewer retrieves samples submitted by a specific reviewer
func (r *ModerationMLRepository) ListSamplesByReviewer(ctx context.Context, reviewerID string, limit int) ([]*models.ModerationSample, error) {
	if limit <= 0 {
		limit = 100
	}

	var samples []*models.ModerationSample
	var results []models.ModerationSample

	query := r.db.WithContext(ctx).Model(&models.ModerationSample{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("REVIEWER#%s", reviewerID)).
		Limit(limit)

	err := query.Scan(&results)
	if err != nil {
		return nil, err
	}

	for i := range results {
		samples = append(samples, &results[i])
	}

	return samples, nil
}

// CreateModelVersion creates a new model version record
func (r *ModerationMLRepository) CreateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error {
	if version.VersionID == "" {
		version.VersionID = uuid.New().String()
	}
	now := time.Now()
	version.CreatedAt = now
	version.UpdatedAt = now

	if err := version.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Use direct DB operations for model version (not part of EnhancedBaseRepository type)
	err := r.db.WithContext(ctx).Model(version).Create()
	if err != nil {
		r.logger.Error("failed to create model version",
			zap.Error(err),
			zap.String("version_id", version.VersionID))
		return err
	}

	r.logger.Info("created model version",
		zap.String("version_id", version.VersionID),
		zap.Float64("accuracy", version.Accuracy))

	return nil
}

// GetModelVersion retrieves a model version by ID
func (r *ModerationMLRepository) GetModelVersion(ctx context.Context, versionID string) (*models.ModerationModelVersion, error) {
	var version models.ModerationModelVersion

	err := r.db.WithContext(ctx).Model(&version).
		Where("PK", "=", "MLMODEL#bedrock").
		Where("SK", "=", fmt.Sprintf("VERSION#%s", versionID)).
		First(&version)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("model version not found: %s", versionID)
		}
		return nil, err
	}

	return &version, nil
}

// GetActiveModelVersion retrieves the currently active model version
func (r *ModerationMLRepository) GetActiveModelVersion(ctx context.Context) (*models.ModerationModelVersion, error) {
	var version models.ModerationModelVersion

	query := r.db.WithContext(ctx).Model(&version).
		Index("gsi1").
		Where("gsi1PK", "=", "MLMODEL#ACTIVE")

	err := query.First(&version)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no active model version found")
		}
		return nil, err
	}

	return &version, nil
}

// UpdateModelVersion updates an existing model version
func (r *ModerationMLRepository) UpdateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error {
	version.UpdatedAt = time.Now()

	if err := version.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(version).Update()
	if err != nil {
		r.logger.Error("failed to update model version",
			zap.Error(err),
			zap.String("version_id", version.VersionID))
		return err
	}

	return nil
}

// CreateEffectivenessMetric creates a new effectiveness metric record
func (r *ModerationMLRepository) CreateEffectivenessMetric(ctx context.Context, metric *models.ModerationEffectivenessMetric) error {
	now := time.Now()
	metric.CreatedAt = now
	metric.UpdatedAt = now

	// Calculate metrics if not already set
	metric.CalculateMetrics()

	if err := metric.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(metric).Create()
	if err != nil {
		r.logger.Error("failed to create effectiveness metric",
			zap.Error(err),
			zap.String("pattern_id", metric.PatternID))
		return err
	}

	r.logger.Debug("created effectiveness metric",
		zap.String("pattern_id", metric.PatternID),
		zap.String("period", metric.Period),
		zap.Float64("f1_score", metric.F1Score))

	return nil
}

// GetEffectivenessMetric retrieves effectiveness metrics for a pattern/period
func (r *ModerationMLRepository) GetEffectivenessMetric(ctx context.Context, patternID, period string, startTime time.Time) (*models.ModerationEffectivenessMetric, error) {
	var metric models.ModerationEffectivenessMetric

	err := r.db.WithContext(ctx).Model(&metric).
		Where("PK", "=", fmt.Sprintf("MLMETRICS#%s", patternID)).
		Where("SK", "=", fmt.Sprintf("PERIOD#%s#%s", period, startTime.Format(time.RFC3339))).
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("effectiveness metric not found")
		}
		return nil, err
	}

	return &metric, nil
}

// ListEffectivenessMetricsByPattern retrieves all metrics for a pattern
func (r *ModerationMLRepository) ListEffectivenessMetricsByPattern(ctx context.Context, patternID string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	if limit <= 0 {
		limit = 50
	}

	var metrics []*models.ModerationEffectivenessMetric
	var results []models.ModerationEffectivenessMetric

	query := r.db.WithContext(ctx).Model(&models.ModerationEffectivenessMetric{}).
		Where("PK", "=", fmt.Sprintf("MLMETRICS#%s", patternID)).
		Limit(limit)

	err := query.Scan(&results)
	if err != nil {
		return nil, err
	}

	for i := range results {
		metrics = append(metrics, &results[i])
	}

	return metrics, nil
}

// ListEffectivenessMetricsByPeriod retrieves top-performing patterns for a period
func (r *ModerationMLRepository) ListEffectivenessMetricsByPeriod(ctx context.Context, period string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	if limit <= 0 {
		limit = 50
	}

	var metrics []*models.ModerationEffectivenessMetric
	var results []models.ModerationEffectivenessMetric

	query := r.db.WithContext(ctx).Model(&models.ModerationEffectivenessMetric{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("METRICS#%s", period)).
		Limit(limit)

	err := query.Scan(&results)
	if err != nil {
		return nil, err
	}

	for i := range results {
		metrics = append(metrics, &results[i])
	}

	return metrics, nil
}
