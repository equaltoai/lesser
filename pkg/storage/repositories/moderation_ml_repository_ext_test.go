package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================
// Constants verification tests
// ============================================

func TestModerationML_ReviewedConstants(t *testing.T) {
	assert.Equal(t, "true", ReviewedTrue)
	assert.Equal(t, "false", ReviewedFalse)
}

// ============================================
// CreatePollRequest tests
// ============================================

func TestModerationMLRepository_CreatePollRequest_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	before := time.Now()
	pollRequest := &models.MLPollRequest{
		JobID:   "job-123",
		JobName: "test-job",
		Status:  "PENDING",
	}

	err := repo.CreatePollRequest(ctx, pollRequest)

	assert.NoError(t, err)
	// Verify timestamps were set
	assert.True(t, pollRequest.CreatedAt.After(before) || pollRequest.CreatedAt.Equal(before))
	assert.True(t, pollRequest.UpdatedAt.After(before) || pollRequest.UpdatedAt.Equal(before))
	// Verify TTL was set to ~48 hours from now
	expectedTTL := time.Now().Add(48 * time.Hour).Unix()
	assert.InDelta(t, expectedTTL, pollRequest.TTL, 10) // Within 10 seconds
}

func TestModerationMLRepository_CreatePollRequest_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Create").Return(dbError)

	pollRequest := &models.MLPollRequest{
		JobID:     "job-123",
		Status:    "PENDING",
		CreatedAt: time.Now(), // Set CreatedAt for UpdateKeys
	}

	err := repo.CreatePollRequest(ctx, pollRequest)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create poll request")
}

// ============================================
// UpdatePollRequest tests
// ============================================

func TestModerationMLRepository_UpdatePollRequest_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil)

	before := time.Now()
	pollRequest := &models.MLPollRequest{
		JobID:     "job-123",
		Status:    "PROCESSING",
		CreatedAt: time.Now(),
	}

	err := repo.UpdatePollRequest(ctx, pollRequest)

	assert.NoError(t, err)
	// Verify UpdatedAt was set
	assert.True(t, pollRequest.UpdatedAt.After(before) || pollRequest.UpdatedAt.Equal(before))
}

func TestModerationMLRepository_UpdatePollRequest_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(dbError)

	pollRequest := &models.MLPollRequest{
		JobID:     "job-123",
		Status:    "PROCESSING",
		CreatedAt: time.Now(),
	}

	err := repo.UpdatePollRequest(ctx, pollRequest)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update poll request")
}

// ============================================
// GetPollRequest tests
// ============================================

func TestModerationMLRepository_GetPollRequest_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	timestamp := int64(1609459200000000000) // Example timestamp

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Where", "PK = ? AND SK = ?", "MLPOLL#job-123", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MLPollRequest")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.MLPollRequest)
		out.JobID = "job-123"
		out.Status = "PENDING"
	}).Return(nil)

	result, err := repo.GetPollRequest(ctx, "job-123", timestamp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "job-123", result.JobID)
}

func TestModerationMLRepository_GetPollRequest_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("item not found")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPollRequest")).Return(mockQuery)
	mockQuery.On("Where", "PK = ? AND SK = ?", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dbError)

	result, err := repo.GetPollRequest(ctx, "job-unknown", 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get poll request")
}

// ============================================
// CreatePrediction tests
// ============================================

func TestModerationMLRepository_CreatePrediction_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	before := time.Now()
	prediction := &models.MLPrediction{
		PredictionID:   "pred-123",
		ObjectID:       "obj-456",
		ModelVersion:   "v1.0",
		PredictedLabel: "spam",
		Confidence:     0.95,
	}

	err := repo.CreatePrediction(ctx, prediction)

	assert.NoError(t, err)
	// Verify timestamps were set
	assert.True(t, prediction.CreatedAt.After(before) || prediction.CreatedAt.Equal(before))
	assert.True(t, prediction.UpdatedAt.After(before) || prediction.UpdatedAt.Equal(before))
	assert.True(t, prediction.Timestamp.After(before) || prediction.Timestamp.Equal(before))
	// Verify TTL was set to ~90 days from now
	expectedTTL := time.Now().Add(90 * 24 * time.Hour).Unix()
	assert.InDelta(t, expectedTTL, prediction.TTL, 10) // Within 10 seconds
}

func TestModerationMLRepository_CreatePrediction_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Create").Return(dbError)

	prediction := &models.MLPrediction{
		PredictionID:   "pred-123",
		ObjectID:       "obj-456",
		ModelVersion:   "v1.0",
		PredictedLabel: "spam",
		Confidence:     0.95,
	}

	err := repo.CreatePrediction(ctx, prediction)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create prediction")
}

// ============================================
// GetPredictionsByModelVersion tests
// ============================================

func TestModerationMLRepository_GetPredictionsByModelVersion_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "MODEL#v1.0").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.MLPrediction)
		*out = []*models.MLPrediction{
			{PredictionID: "pred-1", ModelVersion: "v1.0"},
			{PredictionID: "pred-2", ModelVersion: "v1.0"},
		}
	}).Return(nil)

	results, err := repo.GetPredictionsByModelVersion(ctx, "v1.0", startTime, endTime, 10)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestModerationMLRepository_GetPredictionsByModelVersion_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(dbError)

	results, err := repo.GetPredictionsByModelVersion(ctx, "v1.0", time.Now(), time.Now(), 10)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to get predictions")
}

// ============================================
// GetPredictionsByReviewStatus tests
// ============================================

func TestModerationMLRepository_GetPredictionsByReviewStatus_Reviewed(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "REVIEW#true").Return(mockQuery) // reviewed=true uses "true"
	mockQuery.On("Where", "gsi2SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Limit", 5).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.MLPrediction)
		*out = []*models.MLPrediction{
			{PredictionID: "pred-1", Reviewed: true},
		}
	}).Return(nil)

	results, err := repo.GetPredictionsByReviewStatus(ctx, true, startTime, endTime, 5)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestModerationMLRepository_GetPredictionsByReviewStatus_Unreviewed(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "REVIEW#false").Return(mockQuery) // reviewed=false uses "false"
	mockQuery.On("Where", "gsi2SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Limit", 5).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.MLPrediction)
		*out = []*models.MLPrediction{
			{PredictionID: "pred-2", Reviewed: false},
			{PredictionID: "pred-3", Reviewed: false},
		}
	}).Return(nil)

	results, err := repo.GetPredictionsByReviewStatus(ctx, false, startTime, endTime, 5)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestModerationMLRepository_GetPredictionsByReviewStatus_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(dbError)

	results, err := repo.GetPredictionsByReviewStatus(ctx, true, time.Now(), time.Now(), 5)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "failed to get predictions")
}

// ============================================
// CreateTrainingJob tests
// ============================================

func TestModerationMLRepository_CreateTrainingJob_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	before := time.Now()
	job := &models.ModelTrainingJob{
		JobID:     "job-456",
		JobName:   "training-job-1",
		Status:    "SUBMITTED",
		StartedAt: time.Now(),
	}

	err := repo.CreateTrainingJob(ctx, job)

	assert.NoError(t, err)
	// Verify timestamps were set
	assert.True(t, job.CreatedAt.After(before) || job.CreatedAt.Equal(before))
	assert.True(t, job.UpdatedAt.After(before) || job.UpdatedAt.Equal(before))
	// Verify TTL was set to ~90 days from now
	expectedTTL := time.Now().Add(90 * 24 * time.Hour).Unix()
	assert.InDelta(t, expectedTTL, job.TTL, 10) // Within 10 seconds
}

func TestModerationMLRepository_CreateTrainingJob_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Create").Return(dbError)

	job := &models.ModelTrainingJob{
		JobID:     "job-456",
		Status:    "SUBMITTED",
		StartedAt: time.Now(),
	}

	err := repo.CreateTrainingJob(ctx, job)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create training job")
}

// ============================================
// GetTrainingJob tests
// ============================================

func TestModerationMLRepository_GetTrainingJob_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Where", "PK = ? AND SK = ?", "MLJOB#job-456", "JOB").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.ModelTrainingJob")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.ModelTrainingJob)
		out.JobID = "job-456"
		out.Status = "COMPLETED"
	}).Return(nil)

	result, err := repo.GetTrainingJob(ctx, "job-456")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "job-456", result.JobID)
	assert.Equal(t, "COMPLETED", result.Status)
}

func TestModerationMLRepository_GetTrainingJob_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("item not found")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Where", "PK = ? AND SK = ?", "MLJOB#job-unknown", "JOB").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dbError)

	result, err := repo.GetTrainingJob(ctx, "job-unknown")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get training job")
}

// ============================================
// UpdateTrainingJob tests
// ============================================

func TestModerationMLRepository_UpdateTrainingJob_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil)

	before := time.Now()
	job := &models.ModelTrainingJob{
		JobID:     "job-456",
		Status:    "IN_PROGRESS",
		StartedAt: time.Now(),
	}

	err := repo.UpdateTrainingJob(ctx, job)

	assert.NoError(t, err)
	// Verify UpdatedAt was set
	assert.True(t, job.UpdatedAt.After(before) || job.UpdatedAt.Equal(before))
}

func TestModerationMLRepository_UpdateTrainingJob_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewModerationMLRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	dbError := errors.New("database error")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModelTrainingJob")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(dbError)

	job := &models.ModelTrainingJob{
		JobID:     "job-456",
		Status:    "IN_PROGRESS",
		StartedAt: time.Now(),
	}

	err := repo.UpdateTrainingJob(ctx, job)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update training job")
}
