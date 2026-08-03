package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// CreateMediaMetadata Tests
// ============================================================================

func TestCreateMediaMetadata_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	metadata := &models.MediaMetadata{
		MediaID: "media-123",
		Status:  "pending",
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := repo.CreateMediaMetadata(ctx, metadata)
	require.NoError(t, err)

	// Verify BeforeCreate was called (keys should be set)
	assert.NotEmpty(t, metadata.PK)
	assert.NotEmpty(t, metadata.SK)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateMediaMetadata_BeforeCreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	// Create an invalid MediaMetadata that will fail BeforeCreate validation
	// MediaMetadata requires MediaID to be non-empty
	metadata := &models.MediaMetadata{
		MediaID: "", // Empty - will fail validation
		Status:  "pending",
	}

	err := repo.CreateMediaMetadata(ctx, metadata)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "for creation")
}

func TestCreateMediaMetadata_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	metadata := &models.MediaMetadata{
		MediaID: "media-123",
		Status:  "pending",
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.CreateMediaMetadata(ctx, metadata)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetMediaMetadata Tests
// ============================================================================

func TestGetMediaMetadata_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	expectedPK := "MEDIA#media-123"
	expectedSK := "METADATA"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", expectedPK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", expectedSK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaMetadata)
		record.MediaID = "media-123"
		record.Status = "complete"
		record.Width = 1920
		record.Height = 1080
	}).Return(nil)

	result, err := repo.GetMediaMetadata(ctx, "media-123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "media-123", result.MediaID)
	assert.Equal(t, "complete", result.Status)
	assert.Equal(t, 1920, result.Width)
	assert.Equal(t, 1080, result.Height)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetMediaMetadata_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#not-found").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Return(errors.ErrItemNotFound)

	result, err := repo.GetMediaMetadata(ctx, "not-found")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrMediaMetadataNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetMediaMetadata_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#error-media").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Return(ErrTestMockError)

	result, err := repo.GetMediaMetadata(ctx, "error-media")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrMediaMetadataQueryFailed)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// UpdateMediaMetadata Tests
// ============================================================================

func TestUpdateMediaMetadata_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	metadata := &models.MediaMetadata{
		MediaID: "media-123",
		Status:  "complete",
		Width:   1920,
		Height:  1080,
	}
	// Set up keys as if BeforeCreate had been called
	metadata.PK = "MEDIA#media-123"
	metadata.SK = "METADATA"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.UpdateMediaMetadata(ctx, metadata)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpdateMediaMetadata_BeforeUpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	// Create invalid MediaMetadata that will fail BeforeUpdate validation
	metadata := &models.MediaMetadata{
		MediaID: "", // Empty - will fail validation
		Status:  "",
	}

	err := repo.UpdateMediaMetadata(ctx, metadata)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "for update")
}

func TestUpdateMediaMetadata_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	metadata := &models.MediaMetadata{
		MediaID: "media-123",
		Status:  "complete",
	}
	metadata.PK = "MEDIA#media-123"
	metadata.SK = "METADATA"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError)

	err := repo.UpdateMediaMetadata(ctx, metadata)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetMediaMetadataByStatus Tests
// ============================================================================

func TestGetMediaMetadataByStatus_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	status := "processing"
	limit := 10

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#processing").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{
			{MediaID: "media-1", Status: status},
			{MediaID: "media-2", Status: status},
		}
	}).Return(nil)

	result, err := repo.GetMediaMetadataByStatus(ctx, status, limit)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetMediaMetadataByStatus_NoLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	status := "pending"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#pending").Return(mockQuery)
	// No Limit call when limit <= 0
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{}
	}).Return(nil)

	result, err := repo.GetMediaMetadataByStatus(ctx, status, 0)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetMediaMetadataByStatus_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#failed").Return(mockQuery)
	mockQuery.On("Limit", 5).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Return(ErrTestMockError)

	result, err := repo.GetMediaMetadataByStatus(ctx, "failed", 5)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrMediaMetadataStatusQueryFailed)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// MarkProcessingStarted Tests
// ============================================================================

func TestMarkProcessingStarted_CreatePath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "new-media-123"

	// GetMediaMetadata returns not found -> create new record
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#new-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Return(errors.ErrItemNotFound)

	// Create new record
	mockQuery.On("Create").Return(nil)

	err := repo.MarkProcessingStarted(ctx, mediaID)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestMarkProcessingStarted_UpdatePath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "existing-media-123"

	// GetMediaMetadata returns existing record
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#existing-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaMetadata)
		record.MediaID = mediaID
		record.Status = "pending"
		record.PK = "MEDIA#existing-media-123"
		record.SK = "METADATA"
	}).Return(nil)

	// Update record
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.MarkProcessingStarted(ctx, mediaID)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// MarkProcessingComplete Tests
// ============================================================================

func TestMarkProcessingComplete_CreatePath_PKEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "new-media-123"
	result := ProcessingResult{
		Width:    1920,
		Height:   1080,
		Duration: 5000, // 5 seconds in ms
		FileSize: 1000000,
		Blurhash: "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		Sizes: map[string]SizeInfo{
			"1080p": {Width: 1920, Height: 1080, S3Key: "path/1080p.mp4"},
			"720p":  {Width: 1280, Height: 720, S3Key: "path/720p.mp4"},
		},
	}

	// GetMediaMetadata returns not found
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#new-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Return(errors.ErrItemNotFound)

	// Create new record
	mockQuery.On("Create").Return(nil)

	err := repo.MarkProcessingComplete(ctx, mediaID, result)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestMarkProcessingComplete_UpdatePath_PKPresent(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "existing-media-123"
	result := ProcessingResult{
		Width:    1920,
		Height:   1080,
		Duration: 0,
		FileSize: 500000,
		Blurhash: "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		Sizes: map[string]SizeInfo{
			"1080p": {Width: 1920, Height: 1080, S3Key: "path/1080p.jpg"},
		},
	}

	// GetMediaMetadata returns existing record with PK set
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#existing-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaMetadata)
		record.MediaID = mediaID
		record.Status = "processing"
		record.PK = "MEDIA#existing-media-123"
		record.SK = "METADATA"
		record.AvailableQualities = []string{"720p"} // Existing quality
	}).Return(nil)

	// Update record
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.MarkProcessingComplete(ctx, mediaID, result)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestMarkProcessingComplete_QualityMerge_NoDuplicates(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "media-with-qualities"
	result := ProcessingResult{
		Width:  1920,
		Height: 1080,
		Sizes: map[string]SizeInfo{
			"1080p": {Width: 1920, Height: 1080},
			"720p":  {Width: 1280, Height: 720}, // This already exists
		},
	}

	var capturedMetadata *models.MediaMetadata

	// GetMediaMetadata returns existing record with some qualities
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#media-with-qualities").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaMetadata)
		record.MediaID = mediaID
		record.PK = "MEDIA#media-with-qualities"
		record.SK = "METADATA"
		record.AvailableQualities = []string{"720p", "480p"} // 720p already exists
		capturedMetadata = record
	}).Return(nil)

	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.MarkProcessingComplete(ctx, mediaID, result)
	require.NoError(t, err)

	// Verify 720p wasn't duplicated and 1080p was added
	found720p := 0
	found1080p := 0
	for _, q := range capturedMetadata.AvailableQualities {
		if q == "720p" {
			found720p++
		}
		if q == "1080p" {
			found1080p++
		}
	}
	assert.Equal(t, 1, found720p, "720p should appear exactly once")
	assert.Equal(t, 1, found1080p, "1080p should be added")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// MarkProcessingFailed Tests
// ============================================================================

func TestMarkProcessingFailed_CreatePath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "new-failed-media-123"
	errorMsg := "transcoding failed: unsupported codec"

	// GetMediaMetadata returns not found
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#new-failed-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Return(errors.ErrItemNotFound)

	// Create new record
	mockQuery.On("Create").Return(nil)

	err := repo.MarkProcessingFailed(ctx, mediaID, errorMsg)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestMarkProcessingFailed_UpdatePath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mediaID := "existing-failed-media-123"
	errorMsg := "processing timeout"

	// GetMediaMetadata returns existing record with PK set
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#existing-failed-media-123").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaMetadata")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaMetadata)
		record.MediaID = mediaID
		record.Status = "processing"
		record.PK = "MEDIA#existing-failed-media-123"
		record.SK = "METADATA"
	}).Return(nil)

	// Update record
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.MarkProcessingFailed(ctx, mediaID, errorMsg)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// DeleteMediaMetadata Tests
// ============================================================================

func TestDeleteMediaMetadata_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#media-to-delete").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteMediaMetadata(ctx, "media-to-delete")
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestDeleteMediaMetadata_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA#error-media").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := repo.DeleteMediaMetadata(ctx, "error-media")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// CleanupExpiredMetadata Tests
// ============================================================================

func TestCleanupExpiredMetadata_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#failed").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Return(ErrTestMockError)

	err := repo.CleanupExpiredMetadata(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExpiredMediaMetadataQueryFailed)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCleanupExpiredMetadata_DeleteLoopContinuesOnError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	// Query returns some expired records
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#failed").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{
			{MediaID: "expired-1", Status: "failed"},
			{MediaID: "expired-2", Status: "failed"},
			{MediaID: "expired-3", Status: "failed"},
		}
	}).Return(nil)

	// Delete calls - first fails, others succeed
	mockQuery.On("Where", "PK", "=", "MEDIA#expired-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError).Once() // First delete fails

	mockQuery.On("Where", "PK", "=", "MEDIA#expired-2").Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once() // Second delete succeeds

	mockQuery.On("Where", "PK", "=", "MEDIA#expired-3").Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once() // Third delete succeeds

	// The method should return nil even if some deletes fail (it continues the loop)
	err := repo.CleanupExpiredMetadata(ctx)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestCleanupExpiredMetadata_Success_NoExpiredRecords(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#failed").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{} // No expired records
	}).Return(nil)

	err := repo.CleanupExpiredMetadata(ctx)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetPendingMediaMetadata and GetProcessingMediaMetadata Tests
// ============================================================================

func TestGetPendingMediaMetadata_DelegatesToGetByStatus(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#pending").Return(mockQuery)
	mockQuery.On("Limit", 25).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{
			{MediaID: "pending-1", Status: "pending"},
		}
	}).Return(nil)

	result, err := repo.GetPendingMediaMetadata(ctx, 25)
	require.NoError(t, err)
	assert.Len(t, result, 1)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetProcessingMediaMetadata_DelegatesToGetByStatus(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#processing").Return(mockQuery)
	mockQuery.On("Limit", 50).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{
			{MediaID: "processing-1", Status: "processing"},
			{MediaID: "processing-2", Status: "processing"},
		}
	}).Return(nil)

	result, err := repo.GetProcessingMediaMetadata(ctx, 50)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// ProcessingResult and SizeInfo Struct Tests (ensure fields work correctly)
// ============================================================================

func TestProcessingResult_Fields(t *testing.T) {
	result := ProcessingResult{
		Width:    1920,
		Height:   1080,
		Duration: 120000, // 120 seconds in ms
		FileSize: 50000000,
		Blurhash: "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		Sizes: map[string]SizeInfo{
			"1080p": {
				Width:  1920,
				Height: 1080,
				S3Key:  "media/1080p/video.mp4",
				URL:    "https://cdn.example.com/media/1080p/video.mp4",
			},
			"720p": {
				Width:  1280,
				Height: 720,
				S3Key:  "media/720p/video.mp4",
				URL:    "https://cdn.example.com/media/720p/video.mp4",
			},
		},
	}

	assert.Equal(t, 1920, result.Width)
	assert.Equal(t, 1080, result.Height)
	assert.Equal(t, 120000, result.Duration)
	assert.Equal(t, 50000000, result.FileSize)
	assert.Equal(t, "LKO2?U%2Tw=w]~RBVZRi};RPxuwH", result.Blurhash)
	assert.Len(t, result.Sizes, 2)

	size1080 := result.Sizes["1080p"]
	assert.Equal(t, 1920, size1080.Width)
	assert.Equal(t, 1080, size1080.Height)
	assert.Equal(t, "media/1080p/video.mp4", size1080.S3Key)
	assert.Equal(t, "https://cdn.example.com/media/1080p/video.mp4", size1080.URL)
}

func TestSizeInfo_EmptyValues(t *testing.T) {
	size := SizeInfo{}
	assert.Equal(t, 0, size.Width)
	assert.Equal(t, 0, size.Height)
	assert.Empty(t, size.S3Key)
	assert.Empty(t, size.URL)
}
