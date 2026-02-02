package repositories

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type staticStatusRoundTripper struct {
	status int
	body   string
}

func (s *staticStatusRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return newMockResponse(s.status, s.body), nil
}

func TestFederationRepository_AcknowledgeSeverance_NotFoundAndUpdateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("not found maps to storage.ErrNotFound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.AcknowledgeSeverance(ctx, "user-1", "example.com")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("create error returns update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.AcknowledgeSeverance(ctx, "user-1", "example.com")
		require.Error(t, err)
	})
}

func TestFederationRepository_AttemptReconnection_FailureBranch(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = &staticStatusRoundTripper{status: http.StatusInternalServerError, body: ""}

	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.AttemptReconnection(ctx, "user-1", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_AttemptReconnection_CreateErrors(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("initial create fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.AttemptReconnection(ctx, "user-1", "example.com")
		require.Error(t, err)
	})

	t.Run("update create fails but reconnect error returned", func(t *testing.T) {
		originalTransport := http.DefaultTransport
		t.Cleanup(func() { http.DefaultTransport = originalTransport })
		http.DefaultTransport = &staticStatusRoundTripper{status: http.StatusInternalServerError, body: ""}

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.AttemptReconnection(ctx, "user-1", "example.com")
		require.Error(t, err)
	})
}

func TestFederationRepository_GetInstanceMetadata_NotFoundAndError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("not found maps to storage.ErrNotFound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		_, err := repo.GetInstanceMetadata(ctx, "example.com")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("real error maps to get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		_, err := repo.GetInstanceMetadata(ctx, "example.com")
		require.Error(t, err)
	})
}

func TestFederationRepository_RecordFederationActivity_CreateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// ValidateAndCreate flows through to a Create call; force it to fail.
	mockQuery.On("Create").Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.RecordFederationActivity(ctx, &storage.FederationActivity{
		Domain:       "example.com",
		Type:         "ingress",
		ActivityType: "Create",
		ByteSize:     123,
		Success:      true,
		ResponseTime: 10,
		Timestamp:    baseTime,
	})
	require.Error(t, err)
}

func TestFederationRepository_GetDomainHealthScore_NoRecentDataReturnsNeutral(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationAnalyticsTimeSeries")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.FederationAnalyticsTimeSeries)
		*target = []models.FederationAnalyticsTimeSeries{}
	}).Return(nil).Once()

	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	score, err := repo.GetDomainHealthScore(ctx, "example.com")
	require.NoError(t, err)
	require.Equal(t, 50.0, score)
}

func TestFederationRepository_AggregateFederationMetrics_NoData(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationAnalyticsTimeSeries")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.FederationAnalyticsTimeSeries)
		*target = []models.FederationAnalyticsTimeSeries{}
	}).Return(nil).Once()

	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	require.NoError(t, repo.AggregateFederationMetrics(ctx, "example.com", models.Period5Min, "5min", baseTime))
}

func TestFederationRepository_GetDeliveryStatus_NotFoundAndError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("not found maps to storage.ErrNotFound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		_, err := repo.GetDeliveryStatus(ctx, "activity-1", "example.com")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("real error maps to query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		_, err := repo.GetDeliveryStatus(ctx, "activity-1", "example.com")
		require.Error(t, err)
	})
}

func TestFederationRepository_RecordDeliveryAttempt_NotFoundBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.RecordDeliveryAttempt(ctx, "activity-1", "example.com", false, "failed")
	require.NoError(t, err)
}

func TestFederationRepository_InboxOutbox_CreateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})

	activity := &activitypub.Activity{}
	activity.ID = "act-1"
	activity.Type = "Create"

	require.Error(t, repo.AddToInbox(ctx, "actor-1", activity))
}

func TestFederationRepository_RetryDelivery_NotFoundBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.RetryDelivery(ctx, "activity-1", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_AggregateFederationMetrics_SwitchCases(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})

	for _, toPeriod := range []string{"5min", "hourly", models.PeriodDaily, models.PeriodMonthly} {
		require.NoError(t, repo.AggregateFederationMetrics(ctx, "example.com", models.Period5Min, toPeriod, baseTime))
	}

	err := repo.AggregateFederationMetrics(ctx, "example.com", models.Period5Min, "nope", baseTime)
	require.Error(t, err)
}

func TestFederationRepository_GetAffectedFollowerAndFollowingCounts_NotFoundReturnsZero(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Force GetSeveredRelationship's query.All to return not found so the caller treats it as "no severance".
	mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})

	count, err := repo.GetAffectedFollowersCount(ctx, "user-1", "example.com")
	require.NoError(t, err)
	require.Equal(t, 0, count)

	count, err = repo.GetAffectedFollowingCount(ctx, "user-1", "example.com")
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestFederationRepository_ReverseSeverance_NotReversible(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]models.SeveredRelationship")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.SeveredRelationship)
		*target = []models.SeveredRelationship{
			{
				ID:             "sev-1",
				LocalInstance:  "local.test",
				RemoteInstance: "example.com",
				Reversible:     false,
				DetectedAt:     baseTime,
				Status:         models.SeveranceStatusActive,
			},
		}
	}).Return(nil).Once()

	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.ReverseSeverance(ctx, "local.test", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_CreateErrors_NodeEdgeMetadata(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("UpdateFederationNode create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.UpdateFederationNode(ctx, &storage.FederationNode{Domain: "example.com"})
		require.Error(t, err)
	})

	t.Run("UpdateFederationEdge create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.UpdateFederationEdge(ctx, &storage.FederationEdge{SourceDomain: "a", TargetDomain: "b"})
		require.Error(t, err)
	})

	t.Run("UpdateInstanceMetadata create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.UpdateInstanceMetadata(ctx, &storage.InstanceMetadata{Domain: "example.com"})
		require.Error(t, err)
	})
}

func TestFederationRepository_CreateErrors_TimeSeriesAndMetrics(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("StoreFederationTimeSeries create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.StoreFederationTimeSeries(ctx, &storage.FederationTimeSeries{Domain: "example.com", Period: models.Period5Min})
		require.Error(t, err)
	})

	t.Run("StoreDetailedFederationMetrics create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.StoreDetailedFederationMetrics(ctx, &models.FederationAnalyticsTimeSeries{
			Domain:    "example.com",
			Period:    models.Period5Min,
			Timestamp: baseTime,
		})
		require.Error(t, err)
	})

	t.Run("CreateSeveredRelationship create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.CreateSeveredRelationship(ctx, &models.SeveredRelationship{
			LocalInstance:  "local.test",
			RemoteInstance: "example.com",
			Reason:         models.SeveranceReasonOther,
			DetectedAt:     baseTime,
		})
		require.Error(t, err)
	})

	t.Run("StoreInstanceCluster validation error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		err := repo.StoreInstanceCluster(ctx, &storage.InstanceCluster{})
		require.Error(t, err)
	})
}

func TestFederationRepository_GetSeveredRelationship_EmptyResult(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]models.SeveredRelationship")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.SeveredRelationship)
		*target = []models.SeveredRelationship{}
	}).Return(nil).Once()

	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetSeveredRelationship(ctx, "local.test", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_GetFederationEdges_EmptyDomainsReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()

	repo := &FederationRepository{}
	edges, err := repo.GetFederationEdges(ctx, []string{})
	require.NoError(t, err)
	require.Empty(t, edges)
}

func TestFederationRepository_GetFederationEdges_IgnoresNotFoundAndNonNotFoundErrors(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("not found errors are ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Twice()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		edges, err := repo.GetFederationEdges(ctx, []string{"a.example", "b.example"})
		require.NoError(t, err)
		require.Empty(t, edges)
	})

	t.Run("non-notfound errors are ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Twice()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		edges, err := repo.GetFederationEdges(ctx, []string{"a.example", "b.example"})
		require.NoError(t, err)
		require.Empty(t, edges)
	})
}

func TestFederationRepository_GetFederationNodes_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetFederationNodes(ctx, 1)
	require.Error(t, err)
}

func TestFederationRepository_GetFederationStatistics_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetFederationStatistics(ctx, baseTime.Add(-time.Hour), baseTime)
	require.Error(t, err)
}

func TestFederationRepository_GetInstanceStats_ActivityQueryErrorStillReturnsStats(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCostActivity")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	stats, err := repo.GetInstanceStats(ctx, "example.com")
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Equal(t, float64(0), stats.ErrorRate)
	require.Equal(t, float64(0), stats.AvgResponseTime)
}

func TestFederationRepository_AddToOutbox_CreateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})

	activity := &activitypub.Activity{}
	activity.ID = "act-1"
	activity.Type = "Create"
	require.Error(t, repo.AddToOutbox(ctx, "actor-1", activity, true))
}

func TestFederationRepository_GetStrongestConnectionsByType_LimitAndTypeBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	edges, err := repo.GetStrongestConnectionsByType(ctx, "follows", 0)
	require.NoError(t, err)
	require.Len(t, edges, 3)
}

func TestFederationRepository_FetchEdgePageWithCursor_CursorAndEmptyResultBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("cursor is applied", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		edges, nextCursor, err := repo.fetchEdgePageWithCursor(ctx, "cursor", 1)
		require.NoError(t, err)
		require.NotEmpty(t, edges)
		require.NotEmpty(t, nextCursor)
	})

	t.Run("no results returns empty slice and empty cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationEdge")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.FederationEdge)
			*target = []models.FederationEdge{}
		}).Return(nil).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		edges, nextCursor, err := repo.fetchEdgePageWithCursor(ctx, "", 10)
		require.NoError(t, err)
		require.Empty(t, edges)
		require.Empty(t, nextCursor)
	})
}

func TestFederationRepository_GetAllFederationEdges_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := &FederationRepository{logger: zap.NewNop()}
	edges, err := repo.GetAllFederationEdges(ctx, 10)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, edges)
}

func TestFederationRepository_GetFederationActivitiesByTimeRange_ErrorOnDayStopsEarly(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCostActivity")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	activities, err := repo.GetFederationActivitiesByTimeRange(ctx, baseTime, baseTime.Add(2*time.Hour), 100)
	require.NoError(t, err)
	require.Empty(t, activities)
}

func TestFederationRepository_GetFederationActivitiesByTimeRange_LimitReachedStopsFurtherDays(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	activities, err := repo.GetFederationActivitiesByTimeRange(ctx, baseTime.Add(-time.Minute), baseTime.Add(48*time.Hour), 1)
	require.NoError(t, err)
	require.Len(t, activities, 1)
}

func TestFederationRepository_GetAllFederationEdges_EmptyResultStops(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationEdge")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.FederationEdge)
		*target = []models.FederationEdge{}
	}).Return(nil).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	edges, err := repo.GetAllFederationEdges(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, edges)
}

func TestFederationRepository_GetAllFederationEdges_QueryError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationEdge")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetAllFederationEdges(ctx, 10)
	require.Error(t, err)
}

func TestFederationRepository_GetInstanceHealthReport_AdditionalBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("moderate error rate sets warning status", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCostActivity")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.FederationCostActivity)
			activities := make([]models.FederationCostActivity, 15)
			for i := range activities {
				activities[i] = models.FederationCostActivity{Success: true, ResponseTime: 1000}
			}
			activities[0].Success = false // 1/15 = 6.67% (warning)
			*target = activities
		}).Return(nil).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		report, err := repo.GetInstanceHealthReport(ctx, "example.com", time.Hour)
		require.NoError(t, err)
		require.Equal(t, StatusWarning, report.Status)
	})

	t.Run("slow responses elevate healthy to warning", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCostActivity")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.FederationCostActivity)
			*target = []models.FederationCostActivity{
				{Success: true, ResponseTime: 6001},
				{Success: true, ResponseTime: 6001},
			}
		}).Return(nil).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		report, err := repo.GetInstanceHealthReport(ctx, "example.com", time.Hour)
		require.NoError(t, err)
		require.Equal(t, StatusWarning, report.Status)
	})

	t.Run("query error propagates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCostActivity")).Return(ErrTestMockError).Once()
		setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
		_, err := repo.GetInstanceHealthReport(ctx, "example.com", time.Hour)
		require.Error(t, err)
	})
}

func TestFederationRepository_GetFederationNodesByHealth_DefaultLimitAndNoFilter(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	nodes, err := repo.GetFederationNodesByHealth(ctx, "", 0)
	require.NoError(t, err)
	require.NotEmpty(t, nodes)
}

func TestFederationRepository_AcknowledgeSeverance_FirstError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.AcknowledgeSeverance(ctx, "user-1", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_GetSeveranceHistory_QueryError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]models.SeveredRelationship")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetSeveranceHistory(ctx, "local.test", "example.com", 2)
	require.Error(t, err)
}

func TestFederationRepository_UpdateSeveredRelationship_UpdateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.UpdateSeveredRelationship(ctx, &models.SeveredRelationship{
		LocalInstance:  "local.test",
		RemoteInstance: "example.com",
		Reason:         models.SeveranceReasonOther,
		Reversible:     true,
		Details:        "details",
	})
	require.Error(t, err)
}

func TestFederationRepository_ReverseSeverance_CreateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.ReverseSeverance(ctx, "local.test", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_ListFailedDeliveries_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.DeliveryStatus")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.ListFailedDeliveries(ctx, 10)
	require.Error(t, err)
}

func TestFederationRepository_RetryDelivery_FirstError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.RetryDelivery(ctx, "activity-1", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_RetryDelivery_CreateError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	err := repo.RetryDelivery(ctx, "activity-1", "example.com")
	require.Error(t, err)
}

func TestFederationRepository_GetOutboxItems_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.OutboxItem")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, _, err := repo.GetOutboxItems(ctx, "actor-1", 10, "")
	require.Error(t, err)
}

func TestFederationRepository_GetFederationCostsByUser_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCost")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetFederationCostsByUser(ctx, "user-1", baseTime.Add(-time.Hour), baseTime, 10, 0)
	require.Error(t, err)
}

func TestFederationRepository_GetFederationNodesByHealth_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationNode")).Return(ErrTestMockError).Once()
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	_, err := repo.GetFederationNodesByHealth(ctx, "healthy", 10)
	require.Error(t, err)
}

func TestFederationRepository_GetAllFederationEdges_DefaultLimit(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, &appConfig.Config{})
	edges, err := repo.GetAllFederationEdges(ctx, 0)
	require.NoError(t, err)
	require.NotEmpty(t, edges)
}
