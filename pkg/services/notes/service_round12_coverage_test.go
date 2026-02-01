package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type MockMediaRepository struct {
	mock.Mock
}

func (m *MockMediaRepository) CreateMedia(ctx context.Context, media *models.Media) error {
	args := m.Called(ctx, media)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
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

func (m *MockMediaRepository) CreateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockMediaRepository) GetTranscodingJob(ctx context.Context, jobID string) (*models.TranscodingJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TranscodingJob), args.Error(1)
}

func (m *MockMediaRepository) UpdateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockMediaRepository) GetTranscodingJobsByUser(ctx context.Context, userID string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

func (m *MockMediaRepository) GetTranscodingJobsByMedia(ctx context.Context, mediaID string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, mediaID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

func (m *MockMediaRepository) DeleteTranscodingJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaByUser(ctx context.Context, userID string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

func (m *MockMediaRepository) GetMediaByStatus(ctx context.Context, status string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

func (m *MockMediaRepository) GetMediaByContentType(ctx context.Context, contentType string, limit int) ([]*models.Media, error) {
	args := m.Called(ctx, contentType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

func (m *MockMediaRepository) GetUserMediaLegacy(ctx context.Context, username string) ([]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockMediaRepository) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	args := m.Called(ctx, mediaID, updates)
	return args.Error(0)
}

func (m *MockMediaRepository) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockMediaRepository) CreateMediaJob(ctx context.Context, job *models.MediaJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaJob), args.Error(1)
}

func (m *MockMediaRepository) UpdateMediaJob(ctx context.Context, job *models.MediaJob) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockMediaRepository) DeleteMediaJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

func (m *MockMediaRepository) GetJobsByStatus(ctx context.Context, status string, limit int) ([]*models.MediaJob, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaJob), args.Error(1)
}

func (m *MockMediaRepository) GetJobsByUser(ctx context.Context, username string, limit int) ([]*models.MediaJob, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaJob), args.Error(1)
}

func (m *MockMediaRepository) CreateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockMediaRepository) GetUserMediaConfig(ctx context.Context, userID string) (*models.UserMediaConfig, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserMediaConfig), args.Error(1)
}

func (m *MockMediaRepository) GetUserMediaConfigByUsername(ctx context.Context, username string) (*models.UserMediaConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserMediaConfig), args.Error(1)
}

func (m *MockMediaRepository) UpdateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockMediaRepository) DeleteUserMediaConfig(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockMediaRepository) CreateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	args := m.Called(ctx, spending)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaSpending(ctx context.Context, userID, period string) (*models.MediaSpending, error) {
	args := m.Called(ctx, userID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaSpending), args.Error(1)
}

func (m *MockMediaRepository) UpdateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	args := m.Called(ctx, spending)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaSpendingByTimeRange(ctx context.Context, userID string, periodType string, limit int) ([]*models.MediaSpending, error) {
	args := m.Called(ctx, userID, periodType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaSpending), args.Error(1)
}

func (m *MockMediaRepository) GetOrCreateMediaSpending(ctx context.Context, userID, period, periodType string) (*models.MediaSpending, error) {
	args := m.Called(ctx, userID, period, periodType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaSpending), args.Error(1)
}

func (m *MockMediaRepository) CreateMediaSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

func (m *MockMediaRepository) GetMediaSpendingTransactions(ctx context.Context, userID string, limit int) ([]*models.MediaSpendingTransaction, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaSpendingTransaction), args.Error(1)
}

func (m *MockMediaRepository) AddSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	args := m.Called(ctx, transaction)
	return args.Error(0)
}

func (m *MockMediaRepository) GetTranscodingJobsByStatus(ctx context.Context, status string, limit int) ([]*models.TranscodingJob, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TranscodingJob), args.Error(1)
}

func (m *MockMediaRepository) GetTranscodingCostsByUser(ctx context.Context, userID string, timeRange string) (map[string]int64, error) {
	args := m.Called(ctx, userID, timeRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockMediaRepository) SetDependencies(deps map[string]interface{}) {
	m.Called(deps)
}

func TestService_buildActivityPubNote(t *testing.T) {
	service := &Service{domainName: "example.com"}

	author := &storage.Account{User: &storage.User{Username: "alice"}}
	cmd := &CreateNoteCommand{
		Content:      "hello",
		Visibility:   VisibilityPublic,
		ToRecipients: []string{"https://www.w3.org/ns/activitystreams#Public"},
	}

	note := service.buildActivityPubNote(cmd, "status-1", author)
	if assert.NotNil(t, note) {
		assert.Equal(t, "Note", note.Type)
		assert.Equal(t, "https://example.com/users/alice/statuses/status-1", note.ID)
		assert.Equal(t, "https://example.com/users/alice", note.AttributedTo)
		assert.Equal(t, "hello", note.Content)
		assert.Equal(t, VisibilityPublic, note.Visibility)
		assert.Equal(t, cmd.ToRecipients, note.To)
	}
}

func TestService_buildHashtagTags(t *testing.T) {
	service := &Service{domainName: "example.com"}

	tags, normalized := service.buildHashtagTags("Hello #World #world #GO")
	assert.NotEmpty(t, tags)
	assert.NotEmpty(t, normalized)
	assert.Contains(t, normalized, "world")
	assert.Contains(t, normalized, "go")

	for _, tag := range tags {
		assert.Equal(t, "Hashtag", tag.Type)
		assert.Contains(t, tag.Href, "https://example.com/tags/")
	}
}

func TestService_mediaBelongsToAuthor(t *testing.T) {
	service := &Service{}
	author := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
		},
	}

	assert.True(t, service.mediaBelongsToAuthor(&models.Media{UserID: "alice"}, author))
	assert.True(t, service.mediaBelongsToAuthor(&models.Media{UserID: "https://example.com/users/alice"}, author))
	assert.False(t, service.mediaBelongsToAuthor(&models.Media{UserID: "bob"}, author))
}

func TestService_mapMediaToAttachment(t *testing.T) {
	service := &Service{domainName: "example.com"}

	assert.Equal(t, activitypub.Attachment{}, service.mapMediaToAttachment(nil))

	media := &models.Media{
		MediaID:       "m1",
		ContentType:   "image/jpeg",
		MediaCategory: models.MediaCategoryImage,
		FileName:      "file.jpg",
		Description:   "alt",
		Width:         1,
		Height:        2,
	}
	attachment := service.mapMediaToAttachment(media)
	assert.Equal(t, "Image", attachment.Type)
	assert.Equal(t, "image/jpeg", attachment.MediaType)
	assert.Equal(t, "https://example.com/media/m1", attachment.URL)
	assert.Equal(t, "alt", attachment.Name)
	assert.Equal(t, "file.jpg", attachment.Value)
}

func Test_mapMediaCategoryToAttachmentType(t *testing.T) {
	assert.Equal(t, "Image", mapMediaCategoryToAttachmentType(models.MediaCategoryImage))
	assert.Equal(t, "Image", mapMediaCategoryToAttachmentType(models.MediaCategoryGifv))
	assert.Equal(t, "Video", mapMediaCategoryToAttachmentType(models.MediaCategoryVideo))
	assert.Equal(t, "Audio", mapMediaCategoryToAttachmentType(models.MediaCategoryAudio))
	assert.Equal(t, "Document", mapMediaCategoryToAttachmentType(models.MediaCategoryUnknown))
}

func Test_extractStatusIDFromObjectURL(t *testing.T) {
	assert.Equal(t, "", extractStatusIDFromObjectURL(""))
	assert.Equal(t, "123", extractStatusIDFromObjectURL("https://example.com/users/alice/statuses/123"))
	assert.Equal(t, "123", extractStatusIDFromObjectURL("https://example.com/users/alice/statuses/123/"))
}

func TestService_prepareMediaAttachments(t *testing.T) {
	ctx := context.Background()
	service := &Service{
		domainName: "example.com",
		logger:     zap.NewNop(),
	}

	author := &storage.Account{User: &storage.User{Username: "alice"}}

	t.Run("empty media_ids returns no-op", func(t *testing.T) {
		attachments, markIDs, err := service.prepareMediaAttachments(ctx, author, nil)
		assert.NoError(t, err)
		assert.Nil(t, attachments)
		assert.Nil(t, markIDs)
	})

	t.Run("too many media_ids returns validation error", func(t *testing.T) {
		mediaIDs := []string{"1", "2", "3", "4", "5"}
		_, _, err := service.prepareMediaAttachments(ctx, author, mediaIDs)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, pkgerrors.ErrValidationFailed))
	})

	t.Run("missing media repo returns retrieval error", func(t *testing.T) {
		_, _, err := service.prepareMediaAttachments(ctx, author, []string{"1"})
		assert.ErrorIs(t, err, pkgerrors.ErrRetrieveMediaAttachment)
	})

	t.Run("happy path maps and returns mark IDs", func(t *testing.T) {
		repo := &MockMediaRepository{}
		service.mediaRepo = repo

		repo.On("GetMedia", ctx, "m1").Return(&models.Media{
			MediaID:       "m1",
			UserID:        "alice",
			Status:        "ready",
			ContentType:   "image/jpeg",
			MediaCategory: models.MediaCategoryImage,
		}, nil)

		attachments, markIDs, err := service.prepareMediaAttachments(ctx, author, []string{"m1"})
		assert.NoError(t, err)
		assert.Len(t, attachments, 1)
		assert.Equal(t, []string{"m1"}, markIDs)
		repo.AssertExpectations(t)
	})

	t.Run("non-owner media returns not found", func(t *testing.T) {
		repo := &MockMediaRepository{}
		service.mediaRepo = repo

		repo.On("GetMedia", ctx, "m2").Return(&models.Media{
			MediaID: "m2",
			UserID:  "bob",
			Status:  "ready",
		}, nil)

		_, _, err := service.prepareMediaAttachments(ctx, author, []string{"m2"})
		assert.ErrorIs(t, err, pkgerrors.ErrMediaAttachmentNotFound)
		repo.AssertExpectations(t)
	})

	t.Run("expired media returns expired", func(t *testing.T) {
		repo := &MockMediaRepository{}
		service.mediaRepo = repo

		repo.On("GetMedia", ctx, "m3").Return(&models.Media{
			MediaID:     "m3",
			UserID:      "alice",
			Status:      "ready",
			ExpiresAt:   time.Now().Add(-time.Minute).Unix(),
			ContentType: "image/jpeg",
		}, nil)

		_, _, err := service.prepareMediaAttachments(ctx, author, []string{"m3"})
		assert.ErrorIs(t, err, pkgerrors.ErrMediaAttachmentExpired)
		repo.AssertExpectations(t)
	})
}

func TestService_validateCreateAndUpdateCommands(t *testing.T) {
	logger := zap.NewNop()
	mastodonConfig := common.DefaultMastodonConfig()
	mastodonConfig.Domain = "example.com"
	service := &Service{
		logger:        logger,
		mastodonLogic: common.NewMastodonBusinessLogic(mastodonConfig, logger),
		domainName:    "example.com",
	}

	t.Run("create command nil returns validation failed", func(t *testing.T) {
		err := service.validateCreateCommand(context.Background(), nil)
		assert.ErrorIs(t, err, ErrNotesValidationFailed)
	})

	t.Run("create command invalid visibility", func(t *testing.T) {
		err := service.validateCreateCommand(context.Background(), &CreateNoteCommand{
			AuthorID:     "alice",
			Content:      "hi",
			Visibility:   "nope",
			ToRecipients: []string{},
		})
		assert.Error(t, err)
	})

	t.Run("update command empty content", func(t *testing.T) {
		err := service.validateUpdateCommand(context.Background(), &UpdateNoteCommand{Content: "   "})
		assert.ErrorIs(t, err, ErrContentCannotBeEmpty)
	})
}

func TestService_shouldIncludeStatus(t *testing.T) {
	service := &Service{logger: zap.NewNop()}

	status := &models.Status{StatusID: "1", Deleted: true}
	assert.False(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer"}, false))

	status = &models.Status{StatusID: "2", Visibility: models.VisibilityDirect, AuthorID: "author"}
	assert.False(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer"}, false))
	assert.True(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer"}, true))

	status = &models.Status{StatusID: "3", Visibility: models.VisibilityPublic, MediaCount: 0}
	assert.False(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer", OnlyMedia: true}, true))

	status = &models.Status{StatusID: "4", Visibility: models.VisibilityPublic, InReplyToID: "x"}
	assert.False(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer", ExcludeReplies: true}, true))

	status = &models.Status{StatusID: "5", Visibility: models.VisibilityPublic, ReblogOfID: "x"}
	assert.False(t, service.shouldIncludeStatus(status, &ListNotesQuery{ViewerID: "viewer", ExcludeReblogs: true}, true))
}

func Test_buildNotesResult_trimStatusKey_safeKey_buildStatusURL(t *testing.T) {
	service := &Service{domainName: "example.com"}

	assert.Equal(t, "", trimStatusKey(""))
	assert.Equal(t, "123", trimStatusKey("status#123"))
	assert.Equal(t, "", trimStatusKey("x"))
	assert.Equal(t, "", safeKey(nil, true))

	status := &models.Status{PK: "pk", SK: "sk", StatusID: "123", AuthorUsername: "alice"}
	assert.Equal(t, "pk", safeKey(status, true))
	assert.Equal(t, "sk", safeKey(status, false))
	assert.Equal(t, "https://example.com/users/alice/statuses/123", service.buildStatusURL(status))
	assert.Equal(t, "", service.buildStatusURL(nil))
	assert.Equal(t, "", service.buildStatusURL(&models.Status{StatusID: ""}))
}

func TestStreamingEventEmitter_EmitEvents(t *testing.T) {
	publisher := streaming.NewMockPublisher()
	emitter := &streamingEventEmitter{publisher: publisher}

	events := []*common.StreamingEvent{
		{Type: "test", Timestamp: time.Now(), Metadata: map[string]interface{}{"k": "v"}},
	}

	err := emitter.EmitEvents(context.Background(), events)
	assert.NoError(t, err)

	if recorder, ok := publisher.(interface{ GetPublishedEventCount() int }); ok {
		assert.Equal(t, 1, recorder.GetPublishedEventCount())
	}
}
