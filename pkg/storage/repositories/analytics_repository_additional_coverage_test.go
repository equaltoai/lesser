package repositories

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type stubStatusRepository struct {
	interfaces.StatusRepository
	searchFn func(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
}

func (s *stubStatusRepository) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	if s.searchFn != nil {
		return s.searchFn(ctx, query, opts)
	}
	return &interfaces.PaginatedResult[*models.Status]{}, nil
}

func setupPermissiveAnalyticsRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateAnalyticsSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateAnalyticsSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateAnalyticsStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()

	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()
}

func populateAnalyticsSliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// Avoid interface slices to prevent type assertion pitfalls.
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseType := elemType
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}

	count := 2
	switch baseType {
	case reflect.TypeOf(models.SearchQuery{}):
		count = 3
	case reflect.TypeOf(models.MediaAnalytics{}):
		count = 3
	}

	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseType)
			populateAnalyticsStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseType)
			populateAnalyticsStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateAnalyticsStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)
	date := baseTime.Format(common.DateFormat)

	switch model := target.(type) {
	case *models.SearchQuery:
		model.UserID = fmt.Sprintf("user-%d", (idx%2)+1)
		model.ResultCount = 10 + idx
		model.SearchedAt = now
		switch idx {
		case 0:
			model.Query = "golang"
		case 1:
			model.Query = "golang"
		default:
			model.Query = "go test"
		}
		_ = model.UpdateKeys()

	case *models.EngagementMetrics:
		model.Date = date
		model.MetricType = "status"
		model.TargetID = fmt.Sprintf("status-%d", idx+1)
		model.Views = 10 + int64(idx)
		model.Likes = 5 + int64(idx)
		model.Shares = 2 + int64(idx)
		model.Replies = 1 + int64(idx)
		model.UniqueUsers = 2
		model.StatusID = "status-1"
		model.LikeCount = model.Likes
		model.BoostCount = 1
		model.ReplyCount = model.Replies
		model.Score = float64(model.Views)
		model.EngagementBucket = "high"
		model.UpdatedAt = now
		model.PK = fmt.Sprintf("METRICS#status#%s", date)
		model.SK = fmt.Sprintf("target#%s", model.TargetID)
		_ = model.UpdateKeys()

	case *models.HashtagTrend:
		model.Name = "golang"
		model.URL = "https://localhost/tags/golang"
		model.UsageCount = 42
		model.UniqueUsers = 3
		model.LastUsed = now
		model.FirstSeen = baseTime.Add(-24 * time.Hour)
		model.TrendScore = float64(model.UsageCount)
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.StatusTrend:
		model.ID = "status-1"
		model.URL = "https://localhost/@alice/1"
		model.AuthorID = "alice"
		model.Content = "hello world"
		model.Engagements = 5
		model.PublishedAt = now
		model.TrendScore = float64(model.Engagements)
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.LinkTrend:
		model.URL = "https://example.com"
		model.Title = "Example"
		model.Description = "Example description"
		model.Type = LinkTypeVideo
		model.AuthorName = "alice"
		model.Image = "https://example.com/example.png"
		model.ShareCount = 3
		model.TrendScore = float64(model.ShareCount)
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.TrendingHashtag:
		model.Date = date
		model.Hashtag = "golang"
		model.Score = 1.0
		model.UseCount = 10
		model.UserCount = 5
		model.History = []float64{1, 2, 3}
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.InstanceMetrics:
		model.Date = date
		model.MetricType = "total_users"
		model.Value = 100
		model.Delta = 5
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.PopularQueryCounter:
		model.QueryHash = "hash"
		model.Query = "golang"
		model.TimeBucket = "daily"
		model.Date = date
		model.Count = 10
		model.UserCount = 2
		model.AvgResults = 5.0
		model.FirstQueried = baseTime.Add(-24 * time.Hour)
		model.LastQueried = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.MediaAnalytics:
		model.Format = "hls"
		model.MediaID = "media-1"
		if idx == 0 {
			model.MediaID = "media-2"
		}
		model.UserID = fmt.Sprintf("user-%d", idx+1)
		model.Duration = 10.0
		if idx == 2 {
			model.Duration = 0
		}
		model.Timestamp = now
		model.Date = date
		model.EventType = "session_start"
		model.StreamingSessions = 1
		model.QualityDistribution = map[string]int{"720p": 1}
		model.VariantBandwidth = map[string]int64{"720p": 1024}

	case *models.ModerationAnalytics:
		model.Date = date
		model.ReportType = "spam"
		model.Count = 5
		model.ResolvedCount = 1
		model.AverageResolutionTime = 2.0
		model.ModeratorActions = map[string]int64{"mod-1": 2}
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Activity:
		model.CreatedAt = now
		if idx%2 == 0 {
			model.Activity = &activitypub.Activity{}
			model.Activity.Actor = "https://example.com/users/alice"
			model.Activity.ID = fmt.Sprintf("activity-%d", idx)
		}
		_ = model.UpdateKeys()

	case *models.Actor:
		model.Username = fmt.Sprintf("user-%d", idx+1)
		model.NumericID = fmt.Sprintf("%d", idx+1)
		if idx%2 == 0 {
			model.Actor = &activitypub.Actor{}
			model.Actor.ID = "https://example.com/users/alice"
		}
		_ = model.UpdateKeys()

	case *models.StatusEngagement:
		model.StatusID = "status-1"
		model.UserID = fmt.Sprintf("user-%d", idx+1)
		model.EngagementType = "like"
		model.EngagedAt = now
		_ = model.UpdateKeys()

	case *models.Object:
		model.Type = "Note"

	case *models.User:
		model.Username = fmt.Sprintf("user-%d", idx+1)
	}
}

func TestTrendingRepository_AnalyticsAdditionalCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().Add(-1 * time.Hour)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveAnalyticsRepoMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	t.Run("engagement metrics store/get", func(t *testing.T) {
		require.NoError(t, repo.StoreEngagementMetrics(ctx, &storage.EngagementMetrics{
			StatusID:         "status-1",
			LikeCount:        1,
			BoostCount:       2,
			ReplyCount:       3,
			Score:            4,
			EngagementBucket: "high",
		}))

		metrics, err := repo.GetEngagementMetrics(ctx, "status-1")
		require.NoError(t, err)
		require.NotNil(t, metrics)
	})

	t.Run("delete old trends", func(t *testing.T) {
		require.NoError(t, repo.DeleteOldHashtagTrends(ctx, baseTime))
		require.NoError(t, repo.DeleteOldLinkTrends(ctx, baseTime))
		require.NoError(t, repo.DeleteOldStatusTrends(ctx, baseTime))
	})

	t.Run("search analytics", func(t *testing.T) {
		require.NoError(t, repo.TrackSearchQuery(ctx, "user-1", "Golang", 10))
		require.NoError(t, repo.IndexByEngagement(ctx, "status-1", "high"))

		popular, err := repo.GetPopularSearchQueries(ctx, 1, 24*time.Hour)
		require.NoError(t, err)
		require.NotEmpty(t, popular)

		history, err := repo.GetUserSearchHistory(ctx, "user-1", 5)
		require.NoError(t, err)
		require.NotEmpty(t, history)

		suggestions, err := repo.GenerateSearchSuggestions(ctx, "user-1", "go", 5)
		require.NoError(t, err)
		require.NotEmpty(t, suggestions)
	})

	t.Run("statuses by link", func(t *testing.T) {
		_, err := repo.GetStatusesByLink(ctx, "https://example.com", 200)
		require.Error(t, err)

		statusRepo := &stubStatusRepository{
			searchFn: func(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
				return &interfaces.PaginatedResult[*models.Status]{
					Items: []*models.Status{
						{StatusID: "public-1", Visibility: models.VisibilityPublic, Content: "check this out https://example.com"},
						{StatusID: "public-2", Visibility: models.VisibilityPublic, Content: "no link here"},
					},
				}, nil
			},
		}
		repo.SetStatusRepository(statusRepo)

		results, err := repo.GetStatusesByLink(ctx, "https://example.com", 0)
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("statuses by link filters non-public matches", func(t *testing.T) {
		statusRepo := &stubStatusRepository{
			searchFn: func(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
				return &interfaces.PaginatedResult[*models.Status]{
					Items: []*models.Status{
						{StatusID: "public-1", Visibility: models.VisibilityPublic, Content: "see https://example.com/private-topic"},
						{StatusID: "private-1", Visibility: models.VisibilityPrivate, Content: "see https://example.com/private-topic"},
						{StatusID: "direct-1", Visibility: models.VisibilityDirect, Content: "see https://example.com/private-topic"},
						{StatusID: "unlisted-1", Visibility: models.VisibilityUnlisted, Content: "see https://example.com/private-topic"},
						{StatusID: "deleted-1", Visibility: models.VisibilityPublic, Deleted: true, Content: "see https://example.com/private-topic"},
					},
				}, nil
			},
		}
		repo.SetStatusRepository(statusRepo)

		results, err := repo.GetStatusesByLink(ctx, "https://example.com/private-topic", 20)
		require.NoError(t, err)
		require.Len(t, results, 1)
		result, ok := results[0].(*storage.TrendingStatus)
		require.True(t, ok)
		require.Equal(t, "public-1", result.StatusID)
	})

	t.Run("engagement ranking and aggregation", func(t *testing.T) {
		require.NoError(t, repo.RecordEngagement(ctx, "status", "status-1", baseTime.Format(common.DateFormat), &storage.EngagementData{
			Views:       10,
			Likes:       5,
			Shares:      2,
			Replies:     1,
			UniqueUsers: 2,
		}))

		_, _ = repo.GetEngagementMetricsData(ctx, "status", "status-1", baseTime.Format(common.DateFormat))
		_, _ = repo.GetEngagementByDateRange(ctx, "status", baseTime.Add(-24*time.Hour).Format(common.DateFormat), baseTime.Format(common.DateFormat), 5)
		_, _ = repo.GetTopEngagedContent(ctx, "status", baseTime.Format(common.DateFormat), 1)
		_, _ = repo.AggregateEngagementMetrics(ctx, "status", []string{baseTime.Format(common.DateFormat), baseTime.Add(-24 * time.Hour).Format(common.DateFormat)})
	})

	t.Run("trending hashtag methods", func(t *testing.T) {
		require.NoError(t, repo.UpdateTrendingHashtag(ctx, "golang", baseTime.Format(common.DateFormat), 10, 3))
		_, _ = repo.GetTrendingHashtagsForDate(ctx, baseTime.Format(common.DateFormat), 5)
		_, _ = repo.GetHashtagTrend(ctx, "golang", 2)
		require.NoError(t, repo.PruneStaleTrends(ctx, baseTime.Add(-48*time.Hour)))
	})

	t.Run("instance metrics", func(t *testing.T) {
		require.NoError(t, repo.RecordInstanceMetric(ctx, baseTime.Format(common.DateFormat), "total_users", 100))
		_, _ = repo.GetInstanceMetrics(ctx, baseTime.Format(common.DateFormat), "total_users")
		_, _ = repo.GetMetricHistory(ctx, "total_users", 2)
		_, _ = repo.CalculateGrowthRate(ctx, "total_users", baseTime.Add(-24*time.Hour).Format(common.DateFormat), baseTime.Format(common.DateFormat))
	})

	t.Run("media analytics", func(t *testing.T) {
		require.NoError(t, repo.RecordManifestGeneration(ctx, "media-1", "hls", 1.25))
		require.NoError(t, repo.RecordQualityChange(ctx, "media-1", "user-1", "720p", "1080p"))
		require.NoError(t, repo.RecordMediaEvent(ctx, "session_start", "media-1", "user-1"))

		_, _ = repo.GetManifestGenerationStats(ctx, "hls", baseTime.Add(-24*time.Hour).Format(common.DateFormat), baseTime.Format(common.DateFormat))
		_, _ = repo.GetMediaEventStats(ctx, "session_start", baseTime.Add(-24*time.Hour).Format(common.DateFormat), baseTime.Format(common.DateFormat))
		_, _ = repo.GetStreamingAnalytics(ctx, "media-1")
	})

	t.Run("moderation analytics", func(t *testing.T) {
		require.NoError(t, repo.RecordModerationAction(ctx, baseTime.Format(common.DateFormat), "spam", &storage.ModerationAction{
			Resolved:       true,
			ResolutionTime: 2.0,
			ModeratorID:    "mod-1",
		}))

		_, _ = repo.GetModerationAnalytics(ctx, baseTime.Format(common.DateFormat), "spam")
		_, _ = repo.GetModeratorStats(ctx, "mod-1", 2)
		_, _ = repo.GetReportTrends(ctx, []string{"spam"}, 2)
	})

	t.Run("counts and counters", func(t *testing.T) {
		_, _ = repo.GetActiveUserCount(ctx, 7)
		_, _ = repo.GetTotalUserCount(ctx)
		_, _ = repo.GetTotalStatusCount(ctx)
		_, _ = repo.GetTotalDomainCount(ctx)

		require.NoError(t, repo.IncrementQueryCount(ctx, "golang", 1))
		_, _ = repo.GetQueryCount(ctx, "golang")
		_, _ = repo.GetTopQueries(ctx, 5, 24*time.Hour)

		_, _ = repo.GetActorInteraction(ctx, "https://example.com/users/a", "https://example.com/users/b")
	})
}

func TestTrendingRepository_IncrementQueryCount_InvalidParameters(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	require.ErrorIs(t, repo.IncrementQueryCount(ctx, "", 1), ErrInvalidQueryParameters)
	require.ErrorIs(t, repo.IncrementQueryCount(ctx, "golang", 0), ErrInvalidQueryParameters)
}

func TestTrendingRepository_GetEngagementMetrics_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	_, err := repo.GetEngagementMetrics(ctx, "status-1")
	require.Error(t, err)
}

func TestTrendingRepository_IndexByEngagement_CreateError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Create").Return(fmt.Errorf("create failed")).Once()

	err := repo.IndexByEngagement(ctx, "status-1", "high")
	require.Error(t, err)
}

func TestTrendingRepository_updatePopularQueries_InvalidQuery(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	repo.updatePopularQueries(ctx, "")
}

func TestTrendingRepository_RecordInstanceMetric_ConditionalUpdatePath(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	date := now.Format(common.DateFormat)
	metricType := "total_users"

	mockDB := new(mocks.MockDB)
	genericQuery := new(mocks.MockQuery)
	createQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.MatchedBy(func(arg any) bool {
		metrics, ok := arg.(*models.InstanceMetrics)
		if !ok {
			return false
		}
		return metrics.Date == date && metrics.MetricType == metricType
	})).Return(createQuery).Maybe()
	mockDB.On("Model", mock.Anything).Return(genericQuery).Maybe()

	genericQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(genericQuery).Maybe()
	genericQuery.On("ConsistentRead").Return(genericQuery).Maybe()
	genericQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*models.InstanceMetrics)
		if !ok {
			return
		}
		dest.Date = date
		dest.MetricType = metricType
		dest.Value = 10
		dest.Delta = 1
		dest.UpdatedAt = now
	}).Return(nil).Maybe()

	genericQuery.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Execute").Return(nil).Once()

	createQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()

	require.NoError(t, repo.RecordInstanceMetric(ctx, date, metricType, 5))
}

func TestTrendingRepository_DeleteOldTrends_IdentifierClosureCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().Add(-1 * time.Hour)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateAnalyticsSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Delete").Return(fmt.Errorf("delete failed")).Maybe()

	require.NoError(t, repo.DeleteOldHashtagTrends(ctx, baseTime))
	require.NoError(t, repo.DeleteOldLinkTrends(ctx, baseTime))
	require.NoError(t, repo.DeleteOldStatusTrends(ctx, baseTime))
}

func TestTrendingRepository_GetQueryCount_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PopularQueryCounter")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	count, err := repo.GetQueryCount(ctx, "golang")
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestTrendingRepository_GetActorInteraction_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Twice()

	_, err := repo.GetActorInteraction(ctx, "actor-1", "actor-2")
	require.Error(t, err)
}

func TestTrendingRepository_GetTotalUserCount_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("db down")).Once()

	_, err := repo.GetTotalUserCount(ctx)
	require.Error(t, err)
}

func TestTrendingRepository_getMediaAnalyticsStatsGeneric_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()

	_, err := repo.getMediaAnalyticsStatsGeneric(ctx, "MEDIA_EVENT", "session_start", "2025-01-01", "2025-01-02")
	require.Error(t, err)
}

func TestTrendingRepository_hashQuery_TrimsLongValues(t *testing.T) {
	repo := &TrendingRepository{}

	hash := repo.hashQuery(strings.Repeat("a", 20)) // 40 hex chars
	require.Len(t, hash, 32)
}

func TestTrendingRepository_getTrendingItemsGeneric_NotFoundAndErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("not found returns empty slice", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		result, err := repo.getTrendingItemsGeneric(ctx, "HASHTAG", 10, &models.HashtagTrend{}, func(model interface{}) interface{} {
			m := model.(*models.HashtagTrend)
			return &storage.TrendingHashtag{Name: m.Name}
		})
		require.NoError(t, err)
		require.Empty(t, result.([]*storage.TrendingHashtag))
	})

	t.Run("query error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("db error")).Once()

		_, err := repo.getTrendingItemsGeneric(ctx, "HASHTAG", 10, &models.HashtagTrend{}, func(model interface{}) interface{} {
			m := model.(*models.HashtagTrend)
			return &storage.TrendingHashtag{Name: m.Name}
		})
		require.Error(t, err)
	})
}

func TestTrendingRepository_getTrendingItemsGeneric_Success(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.HashtagTrend)
		*dest = []*models.HashtagTrend{
			{Name: "golang", URL: "https://localhost/tags/golang", UsageCount: 10, UniqueUsers: 5, LastUsed: time.Now()},
			{Name: "testing", URL: "https://localhost/tags/testing", UsageCount: 5, UniqueUsers: 2, LastUsed: time.Now()},
		}
	}).Return(nil).Once()

	result, err := repo.getTrendingItemsGeneric(ctx, "HASHTAG", 10, &models.HashtagTrend{}, func(model interface{}) interface{} {
		m := model.(*models.HashtagTrend)
		return &storage.TrendingHashtag{
			Name:        m.Name,
			URL:         m.URL,
			UsageCount:  m.UsageCount,
			UniqueUsers: m.UniqueUsers,
			LastUsed:    m.LastUsed,
			FirstSeen:   m.FirstSeen,
		}
	})
	require.NoError(t, err)
	require.Len(t, result.([]*storage.TrendingHashtag), 2)
}

type failingTrendModel struct{}

func (f *failingTrendModel) UpdateKeys() error {
	return errors.New("update keys failed")
}

func TestTrendingRepository_storeTrendInternal_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateKeys error", func(t *testing.T) {
		repo := &TrendingRepository{}
		require.Error(t, repo.storeTrendInternal(ctx, &failingTrendModel{}, "hashtag", "golang"))
	})

	t.Run("Create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := &TrendingRepository{db: mockDB, logger: zap.NewNop()}

		model := &models.HashtagTrend{
			Name:       "golang",
			TrendScore: 1,
			UpdatedAt:  time.Now(),
		}

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.storeTrendInternal(ctx, model, "hashtag", model.Name))
	})
}

func TestTrendingRepository_TrackSearchQuery_ValidationAndCreateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid query returns validation error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		require.Error(t, repo.TrackSearchQuery(ctx, "user-1", " ", 10))
	})

	t.Run("create error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.SearchQuery")).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.TrackSearchQuery(ctx, "user-1", "golang", 10))
	})
}

func TestTrendingRepository_GetPopularSearchQueries_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchQuery")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	results, err := repo.GetPopularSearchQueries(ctx, 10, 24*time.Hour)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestTrendingRepository_GetUserSearchHistory_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchQuery")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	results, err := repo.GetUserSearchHistory(ctx, "user-1", 10)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestTrendingRepository_RecordEngagement_CreateError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	err := repo.RecordEngagement(ctx, "status", "status-1", "2025-01-01", &storage.EngagementData{Views: 1})
	require.Error(t, err)
}

func TestTrendingRepository_GetEngagementMetricsData_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	_, err := repo.GetEngagementMetricsData(ctx, "status", "status-1", "2025-01-01")
	require.Error(t, err)
}

func TestTrendingRepository_GetInstanceMetrics_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	_, err := repo.GetInstanceMetrics(ctx, "2025-01-01", "total_users")
	require.Error(t, err)
}

func TestTrendingRepository_GetModerationAnalytics_NotFound(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	_, err := repo.GetModerationAnalytics(ctx, "2025-01-01", "spam")
	require.Error(t, err)
}

func TestTrendingRepository_GetTopQueries_NotFoundAndErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("not found returns empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.PopularQueryCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		results, err := repo.GetTopQueries(ctx, 0, 24*time.Hour)
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("query error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.PopularQueryCounter")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

		_, err := repo.GetTopQueries(ctx, 10, 24*time.Hour)
		require.Error(t, err)
	})
}

func TestTrendingRepository_incrementCounterForBucket_Branches(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	date := now.Format(common.DateFormat)

	t.Run("create new counter when not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(nil).Once()

		require.NoError(t, repo.incrementCounterForBucket(ctx, "hash", "golang", "daily", date, 1, now))
	})

	t.Run("update existing counter when found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PopularQueryCounter)
			dest.PK = "POPULAR_QUERY#hash"
			dest.SK = "COUNTER#daily"
			dest.Count = 10
			dest.UserCount = 2
			dest.AvgResults = 1.0
			dest.FirstQueried = now.Add(-24 * time.Hour)
			dest.LastQueried = now.Add(-time.Hour)
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		require.NoError(t, repo.incrementCounterForBucket(ctx, "hash", "golang", "daily", date, 1, now))
	})

	t.Run("db error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("db error")).Once()

		require.Error(t, repo.incrementCounterForBucket(ctx, "hash", "golang", "daily", date, 1, now))
	})
}

func TestTrendingRepository_GetTotalStatusCount_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetTotalStatusCount(ctx)
	require.Error(t, err)
}

func TestTrendingRepository_RecordModerationAction_NotFoundAndCreateError(t *testing.T) {
	ctx := context.Background()

	t.Run("not found initializes map and still saves", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ModerationAnalytics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(nil).Once()

		require.NoError(t, repo.RecordModerationAction(ctx, "2025-01-01", "spam", &storage.ModerationAction{
			Resolved:       false,
			ResolutionTime: 0,
			ModeratorID:    "mod-1",
		}))
	})

	t.Run("create error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ModerationAnalytics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.RecordModerationAction(ctx, "2025-01-01", "spam", &storage.ModerationAction{
			Resolved:    true,
			ModeratorID: "mod-1",
		}))
	})
}

func TestTrendingRepository_querySessionEvents_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.querySessionEvents(ctx, time.Now().Add(-24*time.Hour))
	require.Error(t, err)
}

func TestTrendingRepository_StoreEngagementMetrics_CreateError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	err := repo.StoreEngagementMetrics(ctx, &storage.EngagementMetrics{
		StatusID:         "status-1",
		EngagementBucket: "high",
	})
	require.Error(t, err)
}

func TestTrendingRepository_GetEngagementByDateRange_NotFoundAndError(t *testing.T) {
	ctx := context.Background()

	t.Run("not found returns empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Twice()

		results, err := repo.GetEngagementByDateRange(ctx, "status", "2025-01-01", "2025-01-02", 10)
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("query error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

		_, err := repo.GetEngagementByDateRange(ctx, "status", "2025-01-01", "2025-01-02", 10)
		require.Error(t, err)
	})
}

func TestTrendingRepository_GetEngagementMetricsData_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetEngagementMetricsData(ctx, "status", "status-1", "2025-01-01")
	require.Error(t, err)
}

func TestTrendingRepository_PruneStaleTrends_NotFoundAndDeleteError(t *testing.T) {
	ctx := context.Background()
	before := time.Now().Add(-24 * time.Hour)

	t.Run("not found returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.TrendingHashtag")).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		require.NoError(t, repo.PruneStaleTrends(ctx, before))
	})

	t.Run("delete errors are logged and skipped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		baseTime := time.Now().Add(-1 * time.Hour)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			populateAnalyticsSliceForCoverage(args.Get(0), baseTime)
		}).Return(nil).Maybe()
		mockQuery.On("Delete").Return(errors.New("delete failed")).Maybe()

		require.NoError(t, repo.PruneStaleTrends(ctx, before))
	})
}

func TestTrendingRepository_deleteOldTrendsGeneric_TTLNoops(t *testing.T) {
	ctx := context.Background()
	repo := &TrendingRepository{logger: zap.NewNop()}

	repo.db = nil

	require.NoError(t, repo.deleteOldTrendsGeneric(ctx, time.Now(), "hashtag", &models.HashtagTrend{}, func(trend interface{}) string {
		return trend.(*models.HashtagTrend).Name
	}))
}

func TestTrendingRepository_GetStatusesByLink_WrongTypeAndSearchError(t *testing.T) {
	ctx := context.Background()

	t.Run("wrong type returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		repo.SetStatusRepository(struct{}{})
		_, err := repo.GetStatusesByLink(ctx, "https://example.com", 10)
		require.Error(t, err)
	})

	t.Run("search error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		statusRepo := &stubStatusRepository{
			searchFn: func(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
				return nil, errors.New("search failed")
			},
		}
		repo.SetStatusRepository(statusRepo)

		_, err := repo.GetStatusesByLink(ctx, "https://example.com", 10)
		require.Error(t, err)
	})
}

func TestTrendingRepository_GetQueryCount_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PopularQueryCounter")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetQueryCount(ctx, "golang")
	require.Error(t, err)
}

func TestTrendingRepository_GetActorInteraction_QueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetActorInteraction(ctx, "actor-1", "actor-2")
	require.Error(t, err)
}

func TestTrendingRepository_scoreHashtagSuggestions_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("query too short returns early", func(t *testing.T) {
		repo := &TrendingRepository{}
		suggestions := map[string]float64{"keep": 1}
		repo.scoreHashtagSuggestions(ctx, "a", suggestions)
		require.Equal(t, float64(1), suggestions["keep"])
	})

	t.Run("no matching hashtags continues", func(t *testing.T) {
		baseTime := time.Now().Add(-1 * time.Hour)
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			populateAnalyticsSliceForCoverage(args.Get(0), baseTime)
		}).Return(nil).Maybe()

		suggestions := make(map[string]float64)
		repo.scoreHashtagSuggestions(ctx, "zz", suggestions)
		require.Empty(t, suggestions)
	})

	t.Run("query error returns early", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

		suggestions := make(map[string]float64)
		repo.scoreHashtagSuggestions(ctx, "go", suggestions)
		require.Empty(t, suggestions)
	})
}

func TestTrendingRepository_queryBufferingEvents_ErrorIgnored(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	count, err := repo.queryBufferingEvents(ctx, "media-1", time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestTrendingRepository_queryQualityChangeEvents_ErrorIgnored(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	count, err := repo.queryQualityChangeEvents(ctx, "media-1", time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestTrendingRepository_GetStreamingAnalytics_SessionQueryError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetStreamingAnalytics(ctx, "media-1")
	require.Error(t, err)
}

func TestTrendingRepository_RecordMediaAnalytics_CreateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("manifest generation create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.RecordManifestGeneration(ctx, "media-1", "hls", 1.2))
	})

	t.Run("quality change create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.RecordQualityChange(ctx, "media-1", "user-1", "720p", "1080p"))
	})

	t.Run("media event create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.RecordMediaEvent(ctx, "session_start", "media-1", "user-1"))
	})
}

func TestTrendingRepository_IncrementQueryCount_InvalidQueryForCounting(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	require.Error(t, repo.IncrementQueryCount(ctx, " ", 1))
}
