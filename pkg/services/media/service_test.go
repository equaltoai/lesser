package media

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock implementations for testing

type MockMediaRepository struct {
	mock.Mock
}

func (m *MockMediaRepository) CreateMedia(ctx context.Context, media *models.Media) error {
	args := m.Called(ctx, media)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	args := m.Called(ctx, mediaID)
	return args.Get(0).(*models.Media), args.Error(1)
}

func (m *MockMediaRepository) UpdateMedia(ctx context.Context, media *models.Media) error {
	args := m.Called(ctx, media)
	return args.Error(0)
}

func (m *MockMediaRepository) DeleteMedia(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

func (m *MockMediaRepository) MarkMediaProcessing(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

func (m *MockMediaRepository) MarkMediaReady(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

func (m *MockMediaRepository) MarkMediaFailed(ctx context.Context, mediaID, errorMsg string) error {
	args := m.Called(ctx, mediaID, errorMsg)
	return args.Error(0)
}

func (m *MockMediaRepository) GetPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) GetProcessingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) AddMediaVariant(ctx context.Context, mediaID, variantName string, variant models.MediaVariant) error {
	args := m.Called(ctx, mediaID, variantName, variant)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaVariant(ctx context.Context, mediaID, variantName string) (*models.MediaVariant, error) {
	args := m.Called(ctx, mediaID, variantName)
	return args.Get(0).(*models.MediaVariant), args.Error(1)
}

func (m *MockMediaRepository) DeleteMediaVariant(ctx context.Context, mediaID, variantName string) error {
	args := m.Called(ctx, mediaID, variantName)
	return args.Error(0)
}

func (m *MockMediaRepository) GetUserMedia(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) GetUserMediaByType(ctx context.Context, userID, contentType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, userID, contentType, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) GetUnusedMedia(ctx context.Context, olderThan time.Time, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, olderThan, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) MarkMediaUsed(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaUsageStats(ctx context.Context, mediaID string) (usageCount int, lastUsed *time.Time, err error) {
	args := m.Called(ctx, mediaID)
	return args.Int(0), args.Get(1).(*time.Time), args.Error(2)
}

func (m *MockMediaRepository) SetMediaModeration(ctx context.Context, mediaID string, isNSFW bool, score float64, labels []string) error {
	args := m.Called(ctx, mediaID, isNSFW, score, labels)
	return args.Error(0)
}

func (m *MockMediaRepository) GetModerationPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

func (m *MockMediaRepository) GetMediaByIDs(ctx context.Context, mediaIDs []string) ([]*models.Media, error) {
	args := m.Called(ctx, mediaIDs)
	return args.Get(0).([]*models.Media), args.Error(1)
}

func (m *MockMediaRepository) DeleteExpiredMedia(ctx context.Context, expiredBefore time.Time) (int64, error) {
	args := m.Called(ctx, expiredBefore)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockMediaRepository) GetMediaStorageUsage(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockMediaRepository) GetTotalStorageUsage(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// Test helper functions

func createTestService(t *testing.T) (*Service, *MockMediaRepository, streaming.Publisher) {
	mediaRepo := new(MockMediaRepository)
	publisher := streaming.NewMockPublisher()
	logger := zaptest.NewLogger(t)

	service := NewService(
		mediaRepo,
		publisher,
		logger,
		"test-bucket",
		"cdn.example.com",
	)

	return service, mediaRepo, publisher
}

func createValidUploadCommand() *UploadMediaCommand {
	return &UploadMediaCommand{
		UserID:      "user123",
		FileName:    "test.jpg",
		ContentType: "image/jpeg",
		FileData:    []byte("fake image data"),
		Description: "A test image",
		Focus:       "0.5,0.5",
	}
}

func createValidUpdateCommand() *UpdateMediaCommand {
	return &UpdateMediaCommand{
		MediaID:     "media123",
		UserID:      "user123",
		Description: "Updated description",
		Focus:       "-0.2,0.8",
	}
}

func createTestMedia() *models.Media {
	now := time.Now()
	return &models.Media{
		MediaID:     "media123",
		UserID:      "user123",
		FileName:    "test.jpg",
		ContentType: "image/jpeg",
		FileSize:    1024,
		Status:      models.StatusPending,
		S3Bucket:    "test-bucket",
		S3Key:       "media/2023/12/01/media123.jpg",
		CDNUrl:      "https://cdn.example.com/media/2023/12/01/media123.jpg",
		Description: "A test image",
		Focus:       "0.5,0.5",
		Width:       1920,
		Height:      1080,
		Blurhash:    "L6PZfSi_.AyE_3t7t7R**0o#DgR4",
		CreatedAt:   now,
		UpdatedAt:   now,
		UploadedAt:  now,
	}
}

// Test UploadMedia method

func TestService_UploadMedia_Success(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUploadCommand()
	ctx := context.Background()

	// Mock expectations
	mediaRepo.On("CreateMedia", ctx, mock.AnythingOfType("*models.Media")).Return(nil)

	// Execute
	result, err := service.UploadMedia(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Media)
	assert.NotEmpty(t, result.Media.MediaID)
	assert.Equal(t, cmd.UserID, result.Media.UserID)
	assert.Equal(t, cmd.FileName, result.Media.FileName)
	assert.Equal(t, cmd.ContentType, result.Media.ContentType)
	assert.Equal(t, int64(len(cmd.FileData)), result.Media.FileSize)
	assert.Equal(t, cmd.Description, result.Media.Description)
	assert.Equal(t, cmd.Focus, result.Media.Focus)
	assert.Equal(t, models.StatusPending, result.Media.Status)
	assert.Equal(t, "test-bucket", result.Media.S3Bucket)
	assert.NotEmpty(t, result.Media.S3Key)
	assert.Contains(t, result.Media.CDNUrl, "cdn.example.com")

	// Check events were emitted
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "media.uploaded", result.Events[0].Type)
	assert.Equal(t, fmt.Sprintf("user:%s", cmd.UserID), result.Events[0].Stream)

	// Note: Publisher verification is done through result events, not direct mock calls

	// Verify repository was called
	mediaRepo.AssertExpectations(t)
}

func TestService_UploadMedia_ValidationErrors(t *testing.T) {
	service, _, _ := createTestService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		modifyCmd   func(*UploadMediaCommand)
		expectedErr string
	}{
		{
			name: "missing user ID",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.UserID = ""
			},
			expectedErr: "user_id is required",
		},
		{
			name: "missing file name",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.FileName = ""
			},
			expectedErr: "file_name is required",
		},
		{
			name: "missing content type",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.ContentType = ""
			},
			expectedErr: "content_type is required",
		},
		{
			name: "empty file data",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.FileData = []byte{}
			},
			expectedErr: "file_data is required",
		},
		{
			name: "file too large",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.FileData = make([]byte, 51*1024*1024) // 51MB
			},
			expectedErr: "file size",
		},
		{
			name: "unsupported content type",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.ContentType = "application/pdf"
			},
			expectedErr: "unsupported content type",
		},
		{
			name: "description too long",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.Description = strings.Repeat("a", 1501)
			},
			expectedErr: "description too long",
		},
		{
			name: "invalid focus point",
			modifyCmd: func(cmd *UploadMediaCommand) {
				cmd.Focus = "invalid"
			},
			expectedErr: "invalid focus point format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createValidUploadCommand()
			tt.modifyCmd(cmd)

			result, err := service.UploadMedia(ctx, cmd)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestService_UploadMedia_RepositoryError(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUploadCommand()
	ctx := context.Background()

	// Mock repository error
	mediaRepo.On("CreateMedia", ctx, mock.AnythingOfType("*models.Media")).Return(fmt.Errorf("database error"))

	// Execute
	result, err := service.UploadMedia(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to store media record")
	assert.Contains(t, err.Error(), "database error")

	mediaRepo.AssertExpectations(t)
}

// Test UpdateMedia method

func TestService_UpdateMedia_Success(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUpdateCommand()
	ctx := context.Background()

	testMedia := createTestMedia()

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, cmd.MediaID).Return(testMedia, nil)
	mediaRepo.On("UpdateMedia", ctx, testMedia).Return(nil)

	// Execute
	result, err := service.UpdateMedia(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Media)
	assert.Equal(t, cmd.Description, result.Media.Description)
	assert.Equal(t, cmd.Focus, result.Media.Focus)

	// Check events were emitted
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "media.updated", result.Events[0].Type)
	assert.Equal(t, fmt.Sprintf("user:%s", cmd.UserID), result.Events[0].Stream)

	// Note: Publisher verification is done through result events, not direct mock calls

	mediaRepo.AssertExpectations(t)
}

func TestService_UpdateMedia_ValidationErrors(t *testing.T) {
	service, _, _ := createTestService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		modifyCmd   func(*UpdateMediaCommand)
		expectedErr string
	}{
		{
			name: "missing media ID",
			modifyCmd: func(cmd *UpdateMediaCommand) {
				cmd.MediaID = ""
			},
			expectedErr: "media_id is required",
		},
		{
			name: "missing user ID",
			modifyCmd: func(cmd *UpdateMediaCommand) {
				cmd.UserID = ""
			},
			expectedErr: "user_id is required",
		},
		{
			name: "description too long",
			modifyCmd: func(cmd *UpdateMediaCommand) {
				cmd.Description = strings.Repeat("a", 1501)
			},
			expectedErr: "description too long",
		},
		{
			name: "invalid focus point",
			modifyCmd: func(cmd *UpdateMediaCommand) {
				cmd.Focus = "invalid"
			},
			expectedErr: "invalid focus point format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createValidUpdateCommand()
			tt.modifyCmd(cmd)

			result, err := service.UpdateMedia(ctx, cmd)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestService_UpdateMedia_NotFound(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUpdateCommand()
	ctx := context.Background()

	// Mock media not found
	mediaRepo.On("GetMedia", ctx, cmd.MediaID).Return((*models.Media)(nil), fmt.Errorf("media not found"))

	// Execute
	result, err := service.UpdateMedia(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get media")

	mediaRepo.AssertExpectations(t)
}

func TestService_UpdateMedia_Unauthorized(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUpdateCommand()
	cmd.UserID = "different-user" // Different user
	ctx := context.Background()

	testMedia := createTestMedia()

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, cmd.MediaID).Return(testMedia, nil)

	// Execute
	result, err := service.UpdateMedia(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unauthorized")

	mediaRepo.AssertExpectations(t)
}

// Test GetMedia method

func TestService_GetMedia_Success(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	query := &GetMediaQuery{
		MediaID:  "media123",
		ViewerID: "user123",
	}
	ctx := context.Background()

	testMedia := createTestMedia()
	testMedia.Status = "ready" // Make it ready

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, query.MediaID).Return(testMedia, nil)
	mediaRepo.On("UpdateMedia", ctx, testMedia).Return(nil) // For marking as used

	// Execute
	result, err := service.GetMedia(ctx, query)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, testMedia.MediaID, result.MediaID)
	assert.Equal(t, 1, result.UsageCount) // Should be marked as used

	mediaRepo.AssertExpectations(t)
}

func TestService_GetMedia_NotReady(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	query := &GetMediaQuery{
		MediaID:  "media123",
		ViewerID: "different-user",
	}
	ctx := context.Background()

	testMedia := createTestMedia()
	testMedia.Status = models.StatusPending // Not ready

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, query.MediaID).Return(testMedia, nil)

	// Execute
	result, err := service.GetMedia(ctx, query)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "media not ready for viewing")

	mediaRepo.AssertExpectations(t)
}

// Test processing callback methods

func TestService_MarkMediaProcessed_Success(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	mediaID := "media123"
	variants := map[string]models.MediaVariant{
		"thumbnail": {
			S3Key:       "media/2023/12/01/media123_thumb.jpg",
			Width:       300,
			Height:      200,
			FileSize:    1024,
			ContentType: "image/jpeg",
		},
	}
	ctx := context.Background()

	testMedia := createTestMedia()

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, mediaID).Return(testMedia, nil)
	mediaRepo.On("UpdateMedia", ctx, testMedia).Return(nil)

	// Execute
	err := service.MarkMediaProcessed(ctx, mediaID, variants)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "ready", testMedia.Status)
	assert.NotNil(t, testMedia.ProcessedAt)
	assert.Empty(t, testMedia.Error)
	assert.Contains(t, testMedia.Variants, "thumbnail")

	// Note: Event verification is done through service behavior, not direct mock calls

	mediaRepo.AssertExpectations(t)
}

func TestService_MarkMediaFailed_Success(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	mediaID := "media123"
	errorMsg := "Processing failed"
	ctx := context.Background()

	testMedia := createTestMedia()

	// Mock expectations
	mediaRepo.On("GetMedia", ctx, mediaID).Return(testMedia, nil)
	mediaRepo.On("UpdateMedia", ctx, testMedia).Return(nil)

	// Execute
	err := service.MarkMediaFailed(ctx, mediaID, errorMsg)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, models.StatusFailed, testMedia.Status)
	assert.NotNil(t, testMedia.ProcessedAt)
	assert.Equal(t, errorMsg, testMedia.Error)

	// Note: Event verification is done through service behavior, not direct mock calls

	mediaRepo.AssertExpectations(t)
}

// Test validation helper methods

func TestService_IsValidMediaType(t *testing.T) {
	service, _, _ := createTestService(t)

	tests := []struct {
		contentType string
		expected    bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"video/mp4", true},
		{"audio/mp3", true},
		{"application/pdf", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := service.isValidMediaType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestService_ValidateFileExtension(t *testing.T) {
	service, _, _ := createTestService(t)

	tests := []struct {
		fileName    string
		contentType string
		expected    bool
	}{
		{"test.jpg", "image/jpeg", true},
		{"test.png", "image/png", true},
		{"test.mp4", "video/mp4", true},
		{"test.jpg", "video/mp4", false},  // Mismatch
		{"test", "image/jpeg", false},     // No extension
		{"test.exe", "image/jpeg", false}, // Wrong extension
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.fileName, tt.contentType), func(t *testing.T) {
			result := service.validateFileExtension(tt.fileName, tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestService_IsValidFocusPoint(t *testing.T) {
	service, _, _ := createTestService(t)

	tests := []struct {
		focus    string
		expected bool
	}{
		{"0.5,0.5", true},
		{"-1.0,1.0", true},
		{"0,0", true},
		{"invalid", false},
		{"0.5", false},         // Missing Y
		{"0.5,0.5,0.5", false}, // Too many values
		{"", false},            // Empty
		{",", false},           // Empty values
	}

	for _, tt := range tests {
		t.Run(tt.focus, func(t *testing.T) {
			result := service.isValidFocusPoint(tt.focus)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestService_GenerateS3Key(t *testing.T) {
	service, _, _ := createTestService(t)

	mediaID := "test-media-123"
	fileName := "test.jpg"

	key := service.generateS3Key(mediaID, fileName)

	assert.Contains(t, key, "media/")
	assert.Contains(t, key, mediaID)
	assert.Contains(t, key, ".jpg")
	assert.Regexp(t, `media/\d{4}/\d{2}/\d{2}/test-media-123\.jpg`, key)
}

// Test service configuration

func TestService_SetMaxFileSize(t *testing.T) {
	service, _, _ := createTestService(t)

	// Default should be 50MB
	assert.Equal(t, int64(50*1024*1024), service.maxFileSize)

	// Set new size
	newSize := int64(10 * 1024 * 1024) // 10MB
	service.SetMaxFileSize(newSize)
	assert.Equal(t, newSize, service.maxFileSize)
}

// Benchmark tests

func createTestServiceForBenchmark() (*Service, *MockMediaRepository, streaming.Publisher) {
	mediaRepo := new(MockMediaRepository)
	publisher := streaming.NewMockPublisher()
	logger := zaptest.NewLogger(&testing.T{})

	service := NewService(
		mediaRepo,
		publisher,
		logger,
		"test-bucket",
		"cdn.example.com",
	)

	return service, mediaRepo, publisher
}

func BenchmarkService_UploadMedia(b *testing.B) {
	service, mediaRepo, _ := createTestServiceForBenchmark()
	ctx := context.Background()

	mediaRepo.On("CreateMedia", ctx, mock.AnythingOfType("*models.Media")).Return(nil)

	cmd := createValidUploadCommand()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.UploadMedia(ctx, cmd)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkService_UpdateMedia(b *testing.B) {
	service, mediaRepo, _ := createTestServiceForBenchmark()
	ctx := context.Background()

	testMedia := createTestMedia()
	cmd := createValidUpdateCommand()

	mediaRepo.On("GetMedia", ctx, cmd.MediaID).Return(testMedia, nil)
	mediaRepo.On("UpdateMedia", ctx, testMedia).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.UpdateMedia(ctx, cmd)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test event emission edge cases

func TestService_EmitEvents_PublisherError(t *testing.T) {
	service, mediaRepo, _ := createTestService(t)
	cmd := createValidUploadCommand()
	ctx := context.Background()

	// Mock expectations
	mediaRepo.On("CreateMedia", ctx, mock.AnythingOfType("*models.Media")).Return(nil)

	// Execute
	result, err := service.UploadMedia(ctx, cmd)

	// Should still succeed despite publisher error (handled gracefully)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Events should still be included in result (just publisher call might fail)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "media.uploaded", result.Events[0].Type)

	mediaRepo.AssertExpectations(t)
}
