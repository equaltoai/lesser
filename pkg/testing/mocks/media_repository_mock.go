// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockMediaRepository is a mock implementation of interfaces.MediaRepository
// using testify/mock for expectation-based testing.
type MockMediaRepository struct {
	mock.Mock
}

// NewMockMediaRepository creates a new mock media repository
func NewMockMediaRepository() *MockMediaRepository {
	return &MockMediaRepository{}
}

// CreateMedia mocks the CreateMedia method
func (m *MockMediaRepository) CreateMedia(ctx context.Context, media *models.Media) error {
	args := m.Called(ctx, media)
	return args.Error(0)
}

// GetMedia mocks the GetMedia method
func (m *MockMediaRepository) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Media), args.Error(1)
}

// UpdateMedia mocks the UpdateMedia method
func (m *MockMediaRepository) UpdateMedia(ctx context.Context, media *models.Media) error {
	args := m.Called(ctx, media)
	return args.Error(0)
}

// DeleteMedia mocks the DeleteMedia method
func (m *MockMediaRepository) DeleteMedia(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// GetMediaByUser mocks the GetMediaByUser method
func (m *MockMediaRepository) GetMediaByUser(ctx context.Context, userID string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

// GetMediaByStatus mocks the GetMediaByStatus method
func (m *MockMediaRepository) GetMediaByStatus(ctx context.Context, status string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

// GetMediaByContentType mocks the GetMediaByContentType method
func (m *MockMediaRepository) GetMediaByContentType(ctx context.Context, contentType string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, contentType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

// GetUserMediaLegacy mocks the GetUserMediaLegacy method
func (m *MockMediaRepository) GetUserMediaLegacy(ctx context.Context, username string) ([]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// MarkMediaProcessing mocks the MarkMediaProcessing method
func (m *MockMediaRepository) MarkMediaProcessing(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// MarkMediaReady mocks the MarkMediaReady method
func (m *MockMediaRepository) MarkMediaReady(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// MarkMediaFailed mocks the MarkMediaFailed method
func (m *MockMediaRepository) MarkMediaFailed(ctx context.Context, mediaID, errorMsg string) error {
	args := m.Called(ctx, mediaID, errorMsg)
	return args.Error(0)
}

// GetPendingMedia mocks the GetPendingMedia method
func (m *MockMediaRepository) GetPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// GetProcessingMedia mocks the GetProcessingMedia method
func (m *MockMediaRepository) GetProcessingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// AddMediaVariant mocks the AddMediaVariant method
func (m *MockMediaRepository) AddMediaVariant(ctx context.Context, mediaID, variantName string, variant models.MediaVariant) error {
	args := m.Called(ctx, mediaID, variantName, variant)
	return args.Error(0)
}

// GetMediaVariant mocks the GetMediaVariant method
func (m *MockMediaRepository) GetMediaVariant(ctx context.Context, mediaID, variantName string) (*models.MediaVariant, error) {
	args := m.Called(ctx, mediaID, variantName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaVariant), args.Error(1)
}

// DeleteMediaVariant mocks the DeleteMediaVariant method
func (m *MockMediaRepository) DeleteMediaVariant(ctx context.Context, mediaID, variantName string) error {
	args := m.Called(ctx, mediaID, variantName)
	return args.Error(0)
}

// UpdateMediaAttachment mocks the UpdateMediaAttachment method
func (m *MockMediaRepository) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	args := m.Called(ctx, mediaID, updates)
	return args.Error(0)
}

// UnmarkAllMediaAsSensitive mocks the UnmarkAllMediaAsSensitive method
func (m *MockMediaRepository) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// UpdateMediaEditorialState mocks the UpdateMediaEditorialState method
func (m *MockMediaRepository) UpdateMediaEditorialState(ctx context.Context, mediaID string, state models.EditorialLifecycle, supersededByMediaID string) error {
	args := m.Called(ctx, mediaID, state, supersededByMediaID)
	return args.Error(0)
}

// UpdateMediaPublishedState mocks the UpdateMediaPublishedState method
func (m *MockMediaRepository) UpdateMediaPublishedState(ctx context.Context, mediaID string, publishedS3Key, publishedURL string, publishedAt time.Time) error {
	args := m.Called(ctx, mediaID, publishedS3Key, publishedURL, publishedAt)
	return args.Error(0)
}

// CreateMediaJob mocks the CreateMediaJob method
func (m *MockMediaRepository) CreateMediaJob(ctx context.Context, job *models.MediaJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

// GetMediaJob mocks the GetMediaJob method
func (m *MockMediaRepository) GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaJob), args.Error(1)
}

// UpdateMediaJob mocks the UpdateMediaJob method
func (m *MockMediaRepository) UpdateMediaJob(ctx context.Context, job *models.MediaJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

// DeleteMediaJob mocks the DeleteMediaJob method
func (m *MockMediaRepository) DeleteMediaJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

// GetJobsByStatus mocks the GetJobsByStatus method
func (m *MockMediaRepository) GetJobsByStatus(ctx context.Context, status string, limit int) ([]*models.MediaJob, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaJob), args.Error(1)
}

// GetJobsByUser mocks the GetJobsByUser method
func (m *MockMediaRepository) GetJobsByUser(ctx context.Context, username string, limit int) ([]*models.MediaJob, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaJob), args.Error(1)
}

// CreateUserMediaConfig mocks the CreateUserMediaConfig method
func (m *MockMediaRepository) CreateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

// GetUserMediaConfig mocks the GetUserMediaConfig method
func (m *MockMediaRepository) GetUserMediaConfig(ctx context.Context, userID string) (*models.UserMediaConfig, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserMediaConfig), args.Error(1)
}

// GetUserMediaConfigByUsername mocks the GetUserMediaConfigByUsername method
func (m *MockMediaRepository) GetUserMediaConfigByUsername(ctx context.Context, username string) (*models.UserMediaConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserMediaConfig), args.Error(1)
}

// UpdateUserMediaConfig mocks the UpdateUserMediaConfig method
func (m *MockMediaRepository) UpdateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

// DeleteUserMediaConfig mocks the DeleteUserMediaConfig method
func (m *MockMediaRepository) DeleteUserMediaConfig(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// CreateMediaSpending mocks the CreateMediaSpending method
func (m *MockMediaRepository) CreateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	args := m.Called(ctx, spending)
	return args.Error(0)
}

// GetMediaSpending mocks the GetMediaSpending method
func (m *MockMediaRepository) GetMediaSpending(ctx context.Context, userID, period string) (*models.MediaSpending, error) {
	args := m.Called(ctx, userID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaSpending), args.Error(1)
}

// UpdateMediaSpending mocks the UpdateMediaSpending method
func (m *MockMediaRepository) UpdateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	args := m.Called(ctx, spending)
	return args.Error(0)
}

// GetMediaSpendingByTimeRange mocks the GetMediaSpendingByTimeRange method
func (m *MockMediaRepository) GetMediaSpendingByTimeRange(ctx context.Context, userID string, periodType string, limit int) ([]*models.MediaSpending, error) {
	args := m.Called(ctx, userID, periodType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaSpending), args.Error(1)
}

// GetOrCreateMediaSpending mocks the GetOrCreateMediaSpending method
func (m *MockMediaRepository) GetOrCreateMediaSpending(ctx context.Context, userID, period, periodType string) (*models.MediaSpending, error) {
	args := m.Called(ctx, userID, period, periodType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaSpending), args.Error(1)
}

// CreateMediaSpendingTransaction mocks the CreateMediaSpendingTransaction method
func (m *MockMediaRepository) CreateMediaSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

// GetMediaSpendingTransactions mocks the GetMediaSpendingTransactions method
func (m *MockMediaRepository) GetMediaSpendingTransactions(ctx context.Context, userID string, limit int) ([]*models.MediaSpendingTransaction, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaSpendingTransaction), args.Error(1)
}

// AddSpendingTransaction mocks the AddSpendingTransaction method
func (m *MockMediaRepository) AddSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

// CreateTranscodingJob mocks the CreateTranscodingJob method
func (m *MockMediaRepository) CreateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

// GetTranscodingJob mocks the GetTranscodingJob method
func (m *MockMediaRepository) GetTranscodingJob(ctx context.Context, jobID string) (*models.TranscodingJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TranscodingJob), args.Error(1)
}

// UpdateTranscodingJob mocks the UpdateTranscodingJob method
func (m *MockMediaRepository) UpdateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

// GetTranscodingJobsByUser mocks the GetTranscodingJobsByUser method
func (m *MockMediaRepository) GetTranscodingJobsByUser(ctx context.Context, userID string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

// GetTranscodingJobsByMedia mocks the GetTranscodingJobsByMedia method
func (m *MockMediaRepository) GetTranscodingJobsByMedia(ctx context.Context, mediaID string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, mediaID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

// GetTranscodingJobsByStatus mocks the GetTranscodingJobsByStatus method
func (m *MockMediaRepository) GetTranscodingJobsByStatus(ctx context.Context, status string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

// DeleteTranscodingJob mocks the DeleteTranscodingJob method
func (m *MockMediaRepository) DeleteTranscodingJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

// GetTranscodingCostsByUser mocks the GetTranscodingCostsByUser method
func (m *MockMediaRepository) GetTranscodingCostsByUser(ctx context.Context, userID string, timeRange string) (map[string]int64, error) {
	args := m.Called(ctx, userID, timeRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

// GetUserMedia mocks the GetUserMedia method
func (m *MockMediaRepository) GetUserMedia(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// GetUserMediaByType mocks the GetUserMediaByType method
func (m *MockMediaRepository) GetUserMediaByType(ctx context.Context, userID, contentType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, userID, contentType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// GetUnusedMedia mocks the GetUnusedMedia method
func (m *MockMediaRepository) GetUnusedMedia(ctx context.Context, olderThan time.Time, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, olderThan, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// MarkMediaUsed mocks the MarkMediaUsed method
func (m *MockMediaRepository) MarkMediaUsed(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// GetMediaUsageStats mocks the GetMediaUsageStats method
func (m *MockMediaRepository) GetMediaUsageStats(ctx context.Context, mediaID string) (int, *time.Time, error) {
	args := m.Called(ctx, mediaID)
	var lastUsed *time.Time
	if args.Get(1) != nil {
		lastUsed = args.Get(1).(*time.Time)
	}
	return args.Int(0), lastUsed, args.Error(2)
}

// SetMediaModeration mocks the SetMediaModeration method
func (m *MockMediaRepository) SetMediaModeration(ctx context.Context, mediaID string, isNSFW bool, score float64, labels []string) error {
	args := m.Called(ctx, mediaID, isNSFW, score, labels)
	return args.Error(0)
}

// GetModerationPendingMedia mocks the GetModerationPendingMedia method
func (m *MockMediaRepository) GetModerationPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Media]), args.Error(1)
}

// GetMediaByIDs mocks the GetMediaByIDs method
func (m *MockMediaRepository) GetMediaByIDs(ctx context.Context, mediaIDs []string) ([]*models.Media, error) {
	args := m.Called(ctx, mediaIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

// DeleteExpiredMedia mocks the DeleteExpiredMedia method
func (m *MockMediaRepository) DeleteExpiredMedia(ctx context.Context, expiredBefore time.Time) (int64, error) {
	args := m.Called(ctx, expiredBefore)
	return args.Get(0).(int64), args.Error(1)
}

// GetMediaStorageUsage mocks the GetMediaStorageUsage method
func (m *MockMediaRepository) GetMediaStorageUsage(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalStorageUsage mocks the GetTotalStorageUsage method
func (m *MockMediaRepository) GetTotalStorageUsage(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// SetDependencies mocks the SetDependencies method
func (m *MockMediaRepository) SetDependencies(deps map[string]interface{}) {
	m.Called(deps)
}

// Ensure MockMediaRepository implements interfaces.MediaRepository
var _ interfaces.MediaRepository = (*MockMediaRepository)(nil)
