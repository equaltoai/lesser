package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func newPermissiveDynamORM(t *testing.T) (*dynamormmocks.MockDB, *dynamormmocks.MockQuery) {
	t.Helper()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()

	q.On("Create").Return(nil).Maybe()
	q.On("CreateOrUpdate").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	q.On("Count").Return(int64(2), nil).Maybe()
	q.On("BatchCreate", mock.Anything).Return(nil).Maybe()

	return db, q
}

func TestHashtagRepository_PureHelpers(t *testing.T) {
	assert.Equal(t, "golang", normalizeHashtagName("#GoLang"))
	assert.Equal(t, "golang", normalizeHashtagName("  golang "))
	assert.Equal(t, "", normalizeHashtagName("#"))
	assert.Equal(t, "", normalizeHashtagName(""))

	assert.Nil(t, convertHashtagFollowModel(nil))

	follow := convertHashtagFollowModel(&models.HashtagFollow{
		PK:                   "user#alice",
		SK:                   "hashtag#go",
		UserID:               "alice",
		Hashtag:              "go",
		NotificationsEnabled: true,
		Muted:                false,
		CreatedAt:            time.Date(2024, 12, 28, 0, 0, 0, 0, time.UTC),
		UpdatedAt:            time.Date(2024, 12, 28, 0, 0, 0, 0, time.UTC),
	})
	require.NotNil(t, follow)
	assert.Equal(t, "go", follow.Hashtag)

	assert.Nil(t, convertHashtagNotificationSettingsModel(nil))
	settings := convertHashtagNotificationSettingsModel(&models.HashtagNotificationSettings{
		PK:      "user#alice",
		SK:      "settings#go",
		UserID:  "alice",
		Hashtag: "go",
		Level:   "all",
		Muted:   false,
		Filters: []models.NotificationFilter{{Types: []string{"mention"}, Limit: 5}},
	})
	require.NotNil(t, settings)
	require.Len(t, settings.Filters, 1)
	assert.Equal(t, 5, settings.Filters[0].Limit)

	modelFilters := convertNotificationFiltersToModel([]*storage.NotificationFilter{
		{
			Types:        []string{"mention"},
			AccountID:    "acct1",
			ExcludeTypes: []string{"follow"},
			Limit:        1,
		},
		nil,
	})
	require.Len(t, modelFilters, 2)
	backToStorage := convertNotificationFiltersToStorage(modelFilters)
	require.Len(t, backToStorage, 2)
	assert.Equal(t, "acct1", backToStorage[0].AccountID)
	assert.Nil(t, convertNotificationFiltersToModel(nil))
	assert.Nil(t, convertNotificationFiltersToStorage(nil))
}

func TestHashtagRepository_SkipsNonPublicIndexing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")

	require.NoError(t, repo.IndexHashtag(ctx, "#Secret", "status-private", "alice", models.VisibilityPrivate))
	require.NoError(t, repo.IndexStatusHashtags(ctx, "status-private", "alice", "alice", "https://example.com/s/private", "secret", []string{"#Secret"}, time.Now(), models.VisibilityPrivate))

	db.AssertNotCalled(t, "WithContext", mock.Anything)
	db.AssertNotCalled(t, "Model", mock.Anything)
	q.AssertNotCalled(t, "Create")
}

func TestHashtagRepository_Sweep_IndexTimelineStatsAndCleanup(t *testing.T) {
	ctx := context.Background()
	db, q := newPermissiveDynamORM(t)
	logger := zap.NewNop()

	repo := NewHashtagRepository(db, "test-table", logger, "example.com")

	// Make BaseRepository.Get populate hashtag metadata when called.
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.Hashtag:
			dest.Name = "golang"
			dest.URL = "https://example.com/tags/golang"
			dest.UsageCount = 42
			dest.FirstSeen = time.Now().Add(-24 * time.Hour)
			dest.LastUsed = time.Now().Add(-1 * time.Hour)
		default:
		}
	}).Return(nil).Maybe()

	// Populate timeline/usage queries with some results so conversion paths run.
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.HashtagStatusIndex:
			*dest = []models.HashtagStatusIndex{
				{
					PK:           "HASHTAG_TIMELINE#golang",
					SK:           "TS#1",
					StatusID:     "s1",
					AuthorID:     "a1",
					AuthorHandle: "alice",
					StatusURL:    "https://example.com/s/1",
					Content:      "hello",
					Visibility:   models.VisibilityPublic,
					Published:    time.Now().Add(-1 * time.Hour),
					GSI2SK:       "TS#1",
				},
				{
					PK:           "HASHTAG_TIMELINE#golang",
					SK:           "TS#2",
					StatusID:     "s2",
					AuthorID:     "a2",
					AuthorHandle: "bob",
					StatusURL:    "https://example.com/s/2",
					Content:      "private",
					Visibility:   models.VisibilityPrivate,
					Published:    time.Now().Add(-2 * time.Hour),
					GSI2SK:       "TS#2",
				},
			}
		case *[]*models.HashtagUsage:
			*dest = []*models.HashtagUsage{
				{StatusID: "s1", AuthorID: "alice", UsedAt: time.Now()},
				{StatusID: "s2", AuthorID: "alice", UsedAt: time.Now()},
				{StatusID: "s3", AuthorID: "bob", UsedAt: time.Now()},
			}
		case *[]*models.HashtagTrend:
			*dest = []*models.HashtagTrend{
				{Name: "golang", URL: "https://example.com/tags/golang", UsageCount: 10, UniqueUsers: 2, FirstSeen: time.Now().Add(-2 * time.Hour), LastUsed: time.Now(), UpdatedAt: time.Now()},
			}
		default:
		}
	}).Return(nil).Maybe()

	// Wave #1469 page-capped walks (RemoveStatusFromHashtagIndex) iterate with
	// AllPaginated instead of a bare All.
	q.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.HashtagStatusIndex:
			*dest = []models.HashtagStatusIndex{
				{
					PK:           "HASHTAG_TIMELINE#golang",
					SK:           "TS#1",
					StatusID:     "s1",
					AuthorID:     "alice",
					AuthorHandle: "alice",
					StatusURL:    "https://example.com/s/1",
					Content:      "hello",
					Visibility:   models.VisibilityPublic,
					Published:    time.Now().Add(-1 * time.Hour),
					GSI2SK:       "TS#1",
				},
			}
		default:
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Maybe()

	// Basic helpers.
	assert.Equal(t, "short", repo.truncateContent("short", 200))
	assert.Contains(t, repo.truncateContent("word1 word2 word3", 10), "...")

	// Indexing: empty hashtags noops.
	require.NoError(t, repo.IndexStatusHashtags(ctx, "status1", "author1", "alice", "https://example.com/s/1", "hello", nil, time.Now(), models.VisibilityPublic))

	// Indexing: creates index entries.
	require.NoError(t, repo.IndexStatusHashtags(ctx, "status1", "author1", "alice", "https://example.com/s/1", "hello "+time.Now().String(), []string{"#GoLang", "#Rust"}, time.Now(), models.VisibilityPublic))

	// Reverse index deletion.
	require.NoError(t, repo.RemoveStatusFromHashtagIndex(ctx, "status1"))

	// Info + stats.
	info, err := repo.GetHashtagInfo(ctx, "#GoLang")
	require.NoError(t, err)
	require.NotNil(t, info)

	stats, err := repo.GetHashtagStats(ctx, "#GoLang")
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Timelines.
	publicResults, err := repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", nil, 2, models.VisibilityPublic)
	require.NoError(t, err)
	require.Len(t, publicResults, 1)
	require.Equal(t, "s1", publicResults[0].StatusID)

	privateVis := "private"
	privateResults, err := repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", nil, 2, privateVis)
	require.NoError(t, err)
	require.Empty(t, privateResults)

	// Multi-hashtag merge path.
	merged, err := repo.GetMultiHashtagTimeline(ctx, []string{"#GoLang"}, nil, 1, privateVis)
	require.NoError(t, err)
	require.Empty(t, merged)

	// Trend storage + batch.
	require.NoError(t, repo.StoreHashtagTrend(ctx, &storage.TrendingHashtag{Name: "golang", URL: "https://example.com/tags/golang"}))
	assert.Error(t, repo.StoreHashtagTrend(ctx, 123))

	require.NoError(t, repo.BatchCreateHashtagTrends(ctx, nil))
	require.NoError(t, repo.BatchCreateHashtagTrends(ctx, []*storage.TrendingHashtag{{Name: "golang", URL: "https://example.com/tags/golang", CreatedAt: time.Now()}}))

	// Delete-old trend orchestration.
	require.NoError(t, repo.DeleteOldHashtagTrends(ctx, time.Now().Add(-24*time.Hour)))
}

func TestHashtagRepository_GetHashtagTrendsByScore_NotFoundReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Index", "gsi8").Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("OrderBy", "gsi8SK", mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)

	q.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	results, err := repo.GetHashtagTrendsByScore(ctx, time.Now(), 10, true)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHashtagRepository_CoverageSweep_Exports(t *testing.T) {
	ctx := context.Background()
	db, q := newPermissiveDynamORM(t)

	// Specific not-found branches.
	q.On("First", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(dynamormerrors.ErrItemNotFound).Maybe()
	q.On("First", mock.AnythingOfType("*models.HashtagFollow")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.Hashtag:
			dest.Name = "golang"
			dest.URL = "https://example.com/tags/golang"
			dest.UsageCount = 2
			dest.FirstSeen = time.Now().Add(-24 * time.Hour)
			dest.LastUsed = time.Now().Add(-1 * time.Hour)
		case *models.HashtagFollow:
			dest.PK = "user#alice"
			dest.SK = "hashtag#golang"
			dest.UserID = "alice"
			dest.Hashtag = "golang"
			dest.NotificationsEnabled = true
		case *models.HashtagMute:
			dest.PK = "user#alice"
			dest.SK = "mute#golang"
			dest.Username = "alice"
			dest.Hashtag = "golang"
			dest.TTL = time.Now().Add(-time.Minute).Unix()
		case *models.HashtagNotificationSettings:
			// Default: treat as missing so callers cover not-found branches.
			// Callers that need a concrete record use Create.
		default:
		}
	}).Return(nil).Maybe()

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.HashtagStatusIndex:
			*dest = []models.HashtagStatusIndex{
				{
					PK:           "HASHTAG_TIMELINE#golang",
					SK:           "TS#1",
					StatusID:     "s1",
					AuthorID:     "alice",
					AuthorHandle: "alice",
					StatusURL:    "https://example.com/s/1",
					Content:      "hi",
					Published:    time.Now().Add(-1 * time.Hour),
					GSI2SK:       "TS#1",
				},
				{
					PK:           "HASHTAG_TIMELINE#rust",
					SK:           "TS#2",
					StatusID:     "s2",
					AuthorID:     "bob",
					AuthorHandle: "bob",
					StatusURL:    "https://example.com/s/2",
					Content:      "hi2",
					Published:    time.Now().Add(-30 * time.Minute),
					GSI2SK:       "TS#2",
				},
			}
		case *[]*models.HashtagUsage:
			*dest = []*models.HashtagUsage{
				{StatusID: "s1", AuthorID: "alice", UsedAt: time.Now()},
			}
		case *[]*models.HashtagFollow:
			*dest = []*models.HashtagFollow{
				{PK: "user#alice", SK: "hashtag#golang", UserID: "alice", Hashtag: "golang"},
				{PK: "user#alice", SK: "hashtag#rust", UserID: "alice", Hashtag: "rust"},
			}
		case *[]*models.HashtagTrend:
			*dest = []*models.HashtagTrend{
				{Name: "golang", URL: "https://example.com/tags/golang", UsageCount: 5, UniqueUsers: 2, FirstSeen: time.Now().Add(-2 * time.Hour), LastUsed: time.Now(), UpdatedAt: time.Now()},
			}
		default:
		}
	}).Return(nil).Maybe()

	// Wave #1469 page-capped walks (RemoveStatusFromHashtagIndex) iterate with
	// AllPaginated instead of a bare All.
	q.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.HashtagStatusIndex:
			*dest = []models.HashtagStatusIndex{
				{
					PK:           "HASHTAG_TIMELINE#golang",
					SK:           "TS#1",
					StatusID:     "s1",
					AuthorID:     "alice",
					AuthorHandle: "alice",
					StatusURL:    "https://example.com/s/1",
					Content:      "hi",
					Published:    time.Now().Add(-1 * time.Hour),
					GSI2SK:       "TS#1",
				},
			}
		default:
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Maybe()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")

	require.NoError(t, repo.IndexHashtag(ctx, "#GoLang", "s1", "alice", models.VisibilityPublic))
	require.NoError(t, repo.IndexStatusHashtags(ctx, "s1", "alice", "alice", "https://example.com/s/1", "hello", []string{"#GoLang"}, time.Now(), models.VisibilityPublic))
	require.NoError(t, repo.RemoveStatusFromHashtagIndex(ctx, "s1"))

	_, _ = repo.GetHashtagUsageHistory(ctx, "#GoLang", 2)
	_, _ = repo.GetHashtagActivity(ctx, "#GoLang", time.Now().Add(-24*time.Hour))
	_, _ = repo.GetHashtagInfo(ctx, "#GoLang")
	_, _ = repo.GetHashtagStats(ctx, "#GoLang")

	maxID := "TS#0"
	_, _ = repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", &maxID, 2, models.VisibilityPublic)
	_, _ = repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", &maxID, 2, "private")

	_, _ = repo.GetMultiHashtagTimeline(ctx, []string{"#GoLang", "#Rust"}, nil, 1, "private")

	_ = repo.FollowHashtag(ctx, "alice", "#GoLang")
	_ = repo.UnfollowHashtag(ctx, "alice", "#GoLang")
	_, _ = repo.IsFollowingHashtag(ctx, "alice", "#GoLang")
	_, _ = repo.GetHashtagFollow(ctx, "alice", "#GoLang")

	_, _ = repo.GetHashtagMute(ctx, "alice", "#GoLang")
	_, _ = repo.IsHashtagMuted(ctx, "alice", "#GoLang")
	_ = repo.MuteHashtag(ctx, "alice", "#GoLang", nil)
	_ = repo.UnmuteHashtag(ctx, "alice", "#GoLang")

	_, _, _ = repo.GetFollowedHashtags(ctx, "alice", 1, "")

	_, _ = repo.GetHashtagNotificationSettings(ctx, "alice", "#GoLang")
	_ = repo.UpdateHashtagNotificationSettings(ctx, "alice", "#GoLang", &storage.HashtagNotificationSettings{
		Level: "none",
		Muted: true,
	})

	_ = repo.DeleteOldHashtagTrends(ctx, time.Now().Add(-24*time.Hour))
	_ = repo.StoreHashtagTrend(ctx, &storage.TrendingHashtag{Name: "golang", URL: "https://example.com/tags/golang", CreatedAt: time.Now()})

	_, _ = repo.GetHashtagTrendsByScore(ctx, time.Now(), 1, false)
	_ = repo.BatchCreateHashtagTrends(ctx, []*storage.TrendingHashtag{{Name: "golang", URL: "https://example.com/tags/golang", CreatedAt: time.Now()}})
}

func TestHashtagRepository_BatchCreateHashtagTrends_ReturnsErrorWhenBatchCreateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("Model", mock.Anything).Return(q)
	q.On("BatchCreate", mock.Anything).Return(errors.New("batch create failed"))

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	err := repo.BatchCreateHashtagTrends(ctx, []*storage.TrendingHashtag{
		{Name: "golang", URL: "https://example.com/tags/golang", UsageCount: 1, CreatedAt: time.Now()},
		{Name: "rust", URL: "https://example.com/tags/rust", UsageCount: 1, CreatedAt: time.Now()},
	})
	assert.Error(t, err)
}

func TestHashtagRepository_StoreHashtagTrend_TrendingScoreType(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("Model", mock.Anything).Return(q)
	q.On("CreateOrUpdate").Return(nil)

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")

	now := time.Now()
	err := repo.StoreHashtagTrend(ctx, &TrendingScore{
		HashtagName:  "golang",
		OverallScore: 1.23,
		Metrics: &TrendingMetrics{
			HashtagName: "golang",
			TotalUsage:  5,
			UniqueUsers: 3,
			FirstSeen:   now.Add(-2 * time.Hour),
			LastUsed:    now,
		},
		Timestamp: now,
	})
	assert.NoError(t, err)
}

func TestHashtagRepository_FollowMuteUnmute_Branches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	// Follow: condition failed -> treated as already exists and returns nil.
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	require.NoError(t, repo.FollowHashtag(ctx, "alice", "#GoLang"))

	// Follow: generic error -> returns error.
	q.On("Create").Return(errors.New("create failed")).Once()
	assert.Error(t, repo.FollowHashtag(ctx, "alice", "#GoLang"))

	// Mute: set TTL when provided; condition failed -> nil.
	until := time.Now().Add(1 * time.Hour).UTC()
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	require.NoError(t, repo.MuteHashtag(ctx, "alice", "#GoLang", &until))

	// Unmute: not found -> nil.
	q.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()
	require.NoError(t, repo.UnmuteHashtag(ctx, "alice", "#GoLang"))
}

func TestHashtagRepository_UpdateHashtagNotificationSettings_DuplicateCreateFallsBackToUpdate(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	settingsQuery := new(dynamormmocks.MockQuery)
	followQuery := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	// Load existing settings -> not found.
	db.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(settingsQuery).Maybe()
	settingsQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(settingsQuery).Maybe()
	settingsQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	// Create settings fails with duplicate, Update succeeds.
	settingsQuery.On("Create").Return(errors.New("already exists")).Once()
	settingsQuery.On("Update", mock.Anything).Return(nil).Once()

	// updateHashtagFollowSetting path: fail to load follow so the wrapper logs and continues.
	db.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followQuery).Maybe()
	followQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(followQuery).Maybe()
	followQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Maybe()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	err := repo.UpdateHashtagNotificationSettings(ctx, "alice", "#GoLang", &storage.HashtagNotificationSettings{
		Level: "none",
		Muted: false,
	})
	assert.NoError(t, err)
}

func TestHashtagRepository_GetHashtagNotificationSettings_Success(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Twice()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.HashtagNotificationSettings)
		dest.PK = "user#alice"
		dest.SK = "settings#golang"
		dest.UserID = "alice"
		dest.Hashtag = "golang"
		dest.Level = "all"
		dest.Muted = false
		dest.Filters = []models.NotificationFilter{{Types: []string{"mention"}, Limit: 5}}
		dest.CreatedAt = time.Now()
		dest.UpdatedAt = time.Now()
	}).Return(nil).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	settings, err := repo.GetHashtagNotificationSettings(ctx, "alice", "#GoLang")
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, "all", settings.Level)
	require.Len(t, settings.Filters, 1)
	assert.Equal(t, 5, settings.Filters[0].Limit)
}

func TestHashtagRepository_GetHashtagMute_AndIsHashtagMuted_NotExpired(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.HashtagMute)
		dest.PK = "user#alice"
		dest.SK = "mute#golang"
		dest.Username = "alice"
		dest.Hashtag = "golang"
		dest.TTL = time.Now().Add(1 * time.Hour).Unix()
	}).Return(nil).Maybe()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	mute, err := repo.GetHashtagMute(ctx, "alice", "#GoLang")
	require.NoError(t, err)
	require.NotNil(t, mute)

	isMuted, err := repo.IsHashtagMuted(ctx, "alice", "#GoLang")
	require.NoError(t, err)
	assert.True(t, isMuted)
}

func TestHashtagRepository_UpdateHashtagNotificationSettings_CreateFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(errors.New("boom")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.UpdateHashtagNotificationSettings(ctx, "alice", "#GoLang", &storage.HashtagNotificationSettings{Level: "all"}))
	assert.Error(t, repo.UpdateHashtagNotificationSettings(ctx, "alice", "#GoLang", nil))
}

func TestHashtagRepository_UpdateHashtagNotificationSettings_UpdateFailureAfterDuplicate(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(nil).Once()
	q.On("Create").Return(errors.New("duplicate")).Once()
	q.On("Update", mock.Anything).Return(errors.New("update failed")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.UpdateHashtagNotificationSettings(ctx, "alice", "#GoLang", &storage.HashtagNotificationSettings{Level: "none"}))
}

func TestHashtagRepository_TimelineQueries_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagStatusIndex")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", nil, 1, models.VisibilityPublic)
	assert.Error(t, err)
}

func TestHashtagRepository_MiscErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	hashtagQuery := new(dynamormmocks.MockQuery)
	followQuery := new(dynamormmocks.MockQuery)
	muteQuery := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	// GetHashtagInfo not found path.
	db.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(hashtagQuery).Once()
	hashtagQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(hashtagQuery).Twice()
	hashtagQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	info, err := repo.GetHashtagInfo(ctx, "#GoLang")
	assert.Error(t, err)
	assert.Nil(t, info)

	// IsFollowingHashtag: non-notfound error.
	db.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followQuery).Once()
	followQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(followQuery).Twice()
	followQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.IsFollowingHashtag(ctx, "alice", "#GoLang")
	assert.Error(t, err)

	// GetHashtagFollow: success.
	db.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followQuery).Once()
	followQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(followQuery).Twice()
	followQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.HashtagFollow)
		dest.PK = "user#alice"
		dest.SK = "hashtag#golang"
		dest.UserID = "alice"
		dest.Hashtag = "golang"
	}).Return(nil).Once()
	follow, err := repo.GetHashtagFollow(ctx, "alice", "#GoLang")
	require.NoError(t, err)
	require.NotNil(t, follow)

	// IsHashtagMuted: not found.
	db.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(muteQuery).Once()
	muteQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(muteQuery).Twice()
	muteQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	muted, err := repo.IsHashtagMuted(ctx, "alice", "#GoLang")
	require.NoError(t, err)
	assert.False(t, muted)

	// UnfollowHashtag: delete error (not not-found) -> returns error.
	db.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followQuery).Once()
	followQuery.On("Delete").Return(errors.New("delete failed")).Once()
	assert.Error(t, repo.UnfollowHashtag(ctx, "alice", "#GoLang"))

	// deleteOldHashtagRecordsBatch unknown type.
	_, err = repo.deleteOldHashtagRecordsBatch(ctx, time.Now(), "nope")
	assert.Error(t, err)
}

func TestHashtagRepository_IndexHashtag_NotFoundCreatesMetadataAndUsage(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	getQ := new(dynamormmocks.MockQuery)
	createQ := new(dynamormmocks.MockQuery)
	usageQ := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	// BaseRepository.Get -> not found.
	db.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(getQ).Once()
	getQ.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(getQ).Twice()
	getQ.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	// ValidateAndCreate -> BaseRepository.Create
	db.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(createQ).Once()
	createQ.On("Create").Return(nil).Once()

	// Usage record create (non-context path).
	db.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQ).Once()
	usageQ.On("Create").Return(nil).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.NoError(t, repo.IndexHashtag(ctx, "#GoLang", "s1", "alice", models.VisibilityPublic))
}

func TestHashtagRepository_IndexHashtag_UsageCreateFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	getQ := new(dynamormmocks.MockQuery)
	createQ := new(dynamormmocks.MockQuery)
	usageQ := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	// BaseRepository.Get -> existing metadata.
	db.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(getQ).Once()
	getQ.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(getQ).Twice()
	getQ.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Hashtag)
		dest.UsageCount = 1
		dest.FirstSeen = time.Now().Add(-time.Hour)
	}).Return(nil).Once()

	// ValidateAndCreate -> BaseRepository.Create
	db.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(createQ).Once()
	createQ.On("Create").Return(nil).Once()

	// Usage record create fails.
	db.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(usageQ).Once()
	usageQ.On("Create").Return(errors.New("create failed")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.IndexHashtag(ctx, "#GoLang", "s1", "alice", models.VisibilityPublic))
}

func TestHashtagRepository_GetHashtagMute_InvalidAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(errors.New("boom")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagMute(ctx, "alice", "")
	assert.Error(t, err)
	_, err = repo.GetHashtagMute(ctx, "alice", "#GoLang")
	assert.Error(t, err)
}

func TestHashtagRepository_GetHashtagNotificationSettings_InvalidAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(errors.New("boom")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagNotificationSettings(ctx, "alice", "")
	assert.Error(t, err)
	_, err = repo.GetHashtagNotificationSettings(ctx, "alice", "#GoLang")
	assert.Error(t, err)
}

func TestHashtagRepository_UnmuteHashtag_InvalidAndDeleteErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(q).Maybe()
	q.On("Delete").Return(errors.New("delete failed")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.UnmuteHashtag(ctx, "alice", ""))
	assert.Error(t, repo.UnmuteHashtag(ctx, "alice", "#GoLang"))
}

func TestHashtagRepository_StoreHashtagTrend_UpsertFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("Model", mock.Anything).Return(q)
	q.On("CreateOrUpdate").Return(errors.New("create failed"))

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.StoreHashtagTrend(ctx, &storage.TrendingHashtag{Name: "golang", URL: "https://example.com/tags/golang"}))
}

func TestHashtagRepository_TimelineVisibility_NotFoundReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.HashtagStatusIndex")).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	results, err := repo.GetHashtagTimelineAdvanced(ctx, "#GoLang", nil, 1, "private")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestHashtagRepository_GetHashtagFollowAndMute_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagFollow(ctx, "alice", "")
	assert.Error(t, err)
	assert.Error(t, repo.MuteHashtag(ctx, "alice", "", nil))
}

func TestHashtagRepository_MuteHashtag_AndGetHashtagFollow_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	muteQuery := new(dynamormmocks.MockQuery)
	followQuery := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	db.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(muteQuery).Once()
	muteQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewHashtagRepository(db, "test-table", zap.NewNop(), "example.com")
	assert.Error(t, repo.MuteHashtag(ctx, "alice", "#GoLang", nil))

	db.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followQuery).Once()
	followQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(followQuery).Twice()
	followQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()

	_, err := repo.GetHashtagFollow(ctx, "alice", "#GoLang")
	assert.Error(t, err)
}

func TestHashtagRepository_EarlyValidationBranches(t *testing.T) {
	ctx := context.Background()

	repo := NewHashtagRepository(nil, "test-table", zap.NewNop(), "example.com")

	_, err := repo.IsFollowingHashtag(ctx, "alice", "")
	assert.Error(t, err)
	assert.Error(t, repo.UnfollowHashtag(ctx, "alice", ""))
	_, err = repo.IsHashtagMuted(ctx, "alice", "")
	assert.Error(t, err)
}
