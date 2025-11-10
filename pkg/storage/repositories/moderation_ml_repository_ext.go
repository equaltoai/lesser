// Package repositories provides repository extensions for Moderation ML
package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	// ReviewedTrue represents the string value for reviewed status
	ReviewedTrue = "true"
	// ReviewedFalse represents the string value for not reviewed status
	ReviewedFalse = "false"
)

// CreatePollRequest creates a new poll request
func (r *ModerationMLRepository) CreatePollRequest(ctx context.Context, pollRequest *models.MLPollRequest) error {
	now := time.Now()
	pollRequest.CreatedAt = now
	pollRequest.UpdatedAt = now
	pollRequest.TTL = now.Add(48 * time.Hour).Unix() // Poll requests expire after 48 hours

	if err := pollRequest.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update poll request keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(pollRequest).Create(); err != nil {
		r.logger.Error("failed to create poll request",
			zap.String("job_id", pollRequest.JobID),
			zap.Error(err))
		return fmt.Errorf("failed to create poll request: %w", err)
	}

	return nil
}

// UpdatePollRequest updates an existing poll request
func (r *ModerationMLRepository) UpdatePollRequest(ctx context.Context, pollRequest *models.MLPollRequest) error {
	pollRequest.UpdatedAt = time.Now()

	if err := pollRequest.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update poll request keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(pollRequest).Update(); err != nil {
		r.logger.Error("failed to update poll request",
			zap.String("job_id", pollRequest.JobID),
			zap.Error(err))
		return fmt.Errorf("failed to update poll request: %w", err)
	}

	return nil
}

// GetPollRequest retrieves a poll request by job ID
func (r *ModerationMLRepository) GetPollRequest(ctx context.Context, jobID string, timestamp int64) (*models.MLPollRequest, error) {
	var pollRequest models.MLPollRequest
	pk := fmt.Sprintf("MLPOLL#%s", jobID)
	sk := fmt.Sprintf("REQUEST#%d", timestamp)

	if err := r.db.WithContext(ctx).Model(&pollRequest).Where("PK = ? AND SK = ?", pk, sk).First(&pollRequest); err != nil {
		return nil, fmt.Errorf("failed to get poll request: %w", err)
	}

	return &pollRequest, nil
}

// CreatePrediction creates a new ML prediction record
func (r *ModerationMLRepository) CreatePrediction(ctx context.Context, prediction *models.MLPrediction) error {
	now := time.Now()
	prediction.CreatedAt = now
	prediction.UpdatedAt = now
	prediction.Timestamp = now
	prediction.TTL = now.Add(90 * 24 * time.Hour).Unix() // Predictions expire after 90 days

	if err := prediction.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update prediction keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(prediction).Create(); err != nil {
		r.logger.Error("failed to create prediction",
			zap.String("prediction_id", prediction.PredictionID),
			zap.String("object_id", prediction.ObjectID),
			zap.Error(err))
		return fmt.Errorf("failed to create prediction: %w", err)
	}

	return nil
}

// GetPredictionsByModelVersion retrieves predictions for a specific model version within a time range
func (r *ModerationMLRepository) GetPredictionsByModelVersion(ctx context.Context, modelVersion string, startTime, endTime time.Time, limit int) ([]*models.MLPrediction, error) {
	var predictions []*models.MLPrediction

	gsi1pk := fmt.Sprintf("MODEL#%s", modelVersion)
	startSK := fmt.Sprintf("TIME#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIME#%s", endTime.Format(time.RFC3339))

	var pred models.MLPrediction
	query := r.db.WithContext(ctx).
		Model(&pred).
		Where("gsi1PK", "=", gsi1pk).
		Where("gsi1SK", ">=", startSK).
		Where("gsi1SK", "<=", endSK).
		Index("gsi1").
		Limit(limit)

	if err := query.Scan(&predictions); err != nil {
		r.logger.Error("failed to get predictions by model version",
			zap.String("model_version", modelVersion),
			zap.String("start_time", startTime.Format(time.RFC3339)),
			zap.String("end_time", endTime.Format(time.RFC3339)),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get predictions: %w", err)
	}

	r.logger.Debug("retrieved predictions by model version",
		zap.String("model_version", modelVersion),
		zap.Int("count", len(predictions)),
		zap.String("start_time", startTime.Format(time.RFC3339)),
		zap.String("end_time", endTime.Format(time.RFC3339)))

	return predictions, nil
}

// GetPredictionsByReviewStatus retrieves predictions by review status within a time range
func (r *ModerationMLRepository) GetPredictionsByReviewStatus(ctx context.Context, reviewed bool, startTime, endTime time.Time, limit int) ([]*models.MLPrediction, error) {
	var predictions []*models.MLPrediction

	reviewStatus := ReviewedFalse
	if reviewed {
		reviewStatus = ReviewedTrue
	}

	gsi2pk := fmt.Sprintf("REVIEW#%s", reviewStatus)
	startSK := fmt.Sprintf("TIME#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIME#%s", endTime.Format(time.RFC3339))

	var pred models.MLPrediction
	query := r.db.WithContext(ctx).
		Model(&pred).
		Where("gsi2PK", "=", gsi2pk).
		Where("gsi2SK", ">=", startSK).
		Where("gsi2SK", "<=", endSK).
		Index("gsi2").
		Limit(limit)

	if err := query.Scan(&predictions); err != nil {
		r.logger.Error("failed to get predictions by review status",
			zap.Bool("reviewed", reviewed),
			zap.String("start_time", startTime.Format(time.RFC3339)),
			zap.String("end_time", endTime.Format(time.RFC3339)),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get predictions: %w", err)
	}

	r.logger.Debug("retrieved predictions by review status",
		zap.Bool("reviewed", reviewed),
		zap.Int("count", len(predictions)),
		zap.String("start_time", startTime.Format(time.RFC3339)),
		zap.String("end_time", endTime.Format(time.RFC3339)))

	return predictions, nil
}

// CreateTrainingJob creates a new training job record
func (r *ModerationMLRepository) CreateTrainingJob(ctx context.Context, job *models.ModelTrainingJob) error {
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now
	job.TTL = now.Add(90 * 24 * time.Hour).Unix() // Jobs expire after 90 days

	if err := job.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update training job keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(job).Create(); err != nil {
		r.logger.Error("failed to create training job",
			zap.String("job_id", job.JobID),
			zap.Error(err))
		return fmt.Errorf("failed to create training job: %w", err)
	}

	return nil
}

// GetTrainingJob retrieves a training job by ID
func (r *ModerationMLRepository) GetTrainingJob(ctx context.Context, jobID string) (*models.ModelTrainingJob, error) {
	var job models.ModelTrainingJob
	pk := fmt.Sprintf("MLJOB#%s", jobID)
	sk := "JOB"

	if err := r.db.WithContext(ctx).Model(&job).Where("PK = ? AND SK = ?", pk, sk).First(&job); err != nil {
		return nil, fmt.Errorf("failed to get training job: %w", err)
	}

	return &job, nil
}

// UpdateTrainingJob updates an existing training job
func (r *ModerationMLRepository) UpdateTrainingJob(ctx context.Context, job *models.ModelTrainingJob) error {
	job.UpdatedAt = time.Now()

	if err := job.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update training job keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(job).Update(); err != nil {
		r.logger.Error("failed to update training job",
			zap.String("job_id", job.JobID),
			zap.Error(err))
		return fmt.Errorf("failed to update training job: %w", err)
	}

	return nil
}
