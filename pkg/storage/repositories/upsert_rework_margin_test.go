package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func upsertFailureDB(t *testing.T, failure error) core.DB {
	t.Helper()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("First", mock.Anything).Return(failure).Maybe()
	query.On("CreateOrUpdate").Return(failure).Maybe()
	query.On("Update", mock.Anything).Return(failure).Maybe()
	return db
}

func TestDirectMessageTombstoneReplayIsIdempotentAndQueryable(t *testing.T) {
	ctx := context.Background()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DirectMessageTombstone{}))
	repo := NewDirectMessageTombstoneRepository(db, models.MainTableName, nil)

	require.ErrorIs(t, repo.CreateTombstone(ctx, "", "status-1"), storage.ErrInvalidInput)
	require.ErrorIs(t, repo.CreateTombstone(ctx, "alice", " "), storage.ErrInvalidInput)
	require.NoError(t, repo.CreateTombstone(ctx, " alice ", " status-1 "))
	require.NoError(t, repo.CreateTombstone(ctx, "alice", "status-1"), "a replay must tolerate the conditional conflict")
	require.Len(t, fake.Items(models.MainTableName), 1)

	empty, err := repo.TombstonesByStatusID(ctx, "alice", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.ErrorIs(t, func() error {
		_, queryErr := repo.TombstonesByStatusID(ctx, "", []string{"status-1"})
		return queryErr
	}(), storage.ErrInvalidInput)

	got, err := repo.TombstonesByStatusID(ctx, "alice", []string{" ", "status-1", "missing"})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"status-1": true}, got)
}

func TestDraftReviewProjectionQueriesAndImmutableVerdicts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", ctx).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Count").Return(int64(2), nil).Once()
	query.On("All", mock.AnythingOfType("*[]models.DraftReviewGrant")).Run(func(args mock.Arguments) {
		rows := args.Get(0).(*[]models.DraftReviewGrant)
		*rows = []models.DraftReviewGrant{{OwnerID: "alice", DraftID: "draft-1", Reviewer: "bob"}}
	}).Return(nil).Once()
	query.On("Create").Return(nil).Once()
	query.On("All", mock.AnythingOfType("*[]models.DraftReviewVerdict")).Run(func(args mock.Arguments) {
		rows := args.Get(0).(*[]models.DraftReviewVerdict)
		*rows = []models.DraftReviewVerdict{{OwnerID: "alice", DraftID: "draft-1", Reviewer: "bob", Verdict: "approve", RecordedAt: now}}
	}).Return(nil).Once()

	repo := NewDraftRepository(db, models.MainTableName, zap.NewNop(), nil)
	count, err := repo.CountActiveDraftReviewGrants(ctx, "bob")
	require.NoError(t, err)
	require.Equal(t, 2, count)

	grants, err := repo.ListDraftReviewGrants(ctx, "alice", "draft-1")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "bob", grants[0].Reviewer)

	verdict := &models.DraftReviewVerdict{OwnerID: "alice", DraftID: "draft-1", Reviewer: "bob", Verdict: "approve", RecordedAt: now}
	require.NoError(t, repo.CreateDraftReviewVerdict(ctx, verdict))
	require.NotEmpty(t, verdict.PK)
	require.NotEmpty(t, verdict.SK)

	verdicts, err := repo.ListDraftReviewVerdicts(ctx, "alice", "draft-1")
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, "bob", verdicts[0].Reviewer)
}

func TestInstanceWellKnownProofCreateThenCacheRefresh(t *testing.T) {
	ctx := context.Background()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.InstanceWellKnownLesserSoulAgent{}))

	var nilRepo *InstanceRepository
	require.NoError(t, nilRepo.SetWellKnownLesserSoulAgent(ctx, nil))
	missing, err := nilRepo.GetWellKnownLesserSoulAgent(ctx)
	require.NoError(t, err)
	require.Nil(t, missing)

	repo := NewInstanceRepository(db, models.MainTableName, zap.NewNop())
	require.Error(t, repo.SetWellKnownLesserSoulAgent(ctx, nil))
	proof := models.NewInstanceWellKnownLesserSoulAgent()
	proof.ProofValue = "  proof-v1  "
	require.NoError(t, repo.SetWellKnownLesserSoulAgent(ctx, proof))
	require.Equal(t, "proof-v1", proof.ProofValue)
	require.Len(t, fake.Items(models.MainTableName), 1)

	cached, err := repo.GetWellKnownLesserSoulAgent(ctx)
	require.NoError(t, err)
	require.Same(t, proof, cached)

	freshRepo := NewInstanceRepository(db, models.MainTableName, zap.NewNop())
	persisted, err := freshRepo.GetWellKnownLesserSoulAgent(ctx)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, "proof-v1", persisted.ProofValue)
}

func TestBatchRelationshipProjectionsDeduplicateAndReturnMatches(t *testing.T) {
	ctx := context.Background()

	t.Run("status pins", func(t *testing.T) {
		fake := fakedb.New()
		db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
		require.NoError(t, err)
		require.NoError(t, db.CreateTable(&models.StatusPin{}))
		pin := &models.StatusPin{Username: "alice", StatusID: "status-1"}
		require.NoError(t, pin.BeforeCreate())
		require.NoError(t, db.Model(pin).Create())

		repo := NewSocialRepository(db, models.MainTableName, zap.NewNop(), nil)
		empty, err := repo.CheckPinnedStatuses(ctx, "", []string{"status-1"})
		require.NoError(t, err)
		require.Empty(t, empty)
		got, err := repo.CheckPinnedStatuses(ctx, "alice", []string{"", "status-1", "status-1", "missing"})
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"status-1": true}, got)
	})

	t.Run("likes", func(t *testing.T) {
		fake := fakedb.New()
		db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
		require.NoError(t, err)
		require.NoError(t, db.CreateTable(&models.Like{}))
		like := models.NewLike("alice", "status-1", "bob")
		require.NoError(t, db.Model(like).Create())

		repo := NewLikeRepository(db, models.MainTableName, zap.NewNop())
		empty, err := repo.CheckLikesForStatuses(ctx, "", []string{"status-1"})
		require.NoError(t, err)
		require.Empty(t, empty)
		got, err := repo.CheckLikesForStatuses(ctx, "alice", []string{"", "status-1", "status-1", "missing"})
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"status-1": true}, got)
	})
}

func TestRateLimitLockoutProjectionReplayNeverShortensExpiry(t *testing.T) {
	ctx := context.Background()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.RateLimitLockout{}))
	repo := NewRateLimitRepository(db, models.MainTableName, zap.NewNop(), nil)

	var nilRepo *RateLimitRepository
	require.Error(t, nilRepo.ImposeLockout(ctx, "alice", time.Minute))
	require.ErrorIs(t, nilRepo.imposeLockoutUntil(ctx, "alice", time.Now()), storage.ErrDatabaseConnectionFailed)
	require.Error(t, repo.ImposeLockout(ctx, " ", time.Minute))
	require.NoError(t, repo.ImposeLockout(ctx, "alice", 0))
	limited, unlockAt, err := repo.IsRateLimited(ctx, "alice")
	require.NoError(t, err)
	require.True(t, limited)
	require.True(t, unlockAt.After(time.Now().Add(50*time.Minute)))

	longer := time.Now().Add(3 * time.Hour).UTC()
	require.NoError(t, repo.imposeLockoutUntil(ctx, "alice", longer))
	shorter := time.Now().Add(2 * time.Hour).UTC()
	require.NoError(t, repo.imposeLockoutUntil(ctx, "alice", shorter))
	limited, persisted, err := repo.IsRateLimited(ctx, "alice")
	require.NoError(t, err)
	require.True(t, limited)
	require.WithinDuration(t, longer, persisted, time.Millisecond)

	require.ErrorIs(t, repo.imposeLockoutUntil(ctx, "", longer), storage.ErrInvalidInput)
	require.ErrorIs(t, repo.imposeLockoutUntil(ctx, "alice", time.Time{}), storage.ErrInvalidInput)
}

func TestReworkedUpsertsSurfaceStorageFailures(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	failure := errors.New("upsert unavailable")
	logger := zap.NewNop()

	tests := []struct {
		name string
		call func(core.DB) error
	}{
		{
			name: "AI aggregate",
			call: func(db core.DB) error {
				return NewAICostRepository(db, models.MainTableName, logger, nil).
					CreateOrUpdateAggregatedCost(ctx, &models.AIAggregatedCost{Period: "day", OperationType: "summarize", ModelName: "model"})
			},
		},
		{
			name: "DynamoDB aggregate",
			call: func(db core.DB) error {
				return NewTrackingRepository(db, models.MainTableName, logger, nil).
					CreateAggregated(ctx, &models.DynamoDBCostAggregation{Period: "minute", OperationType: "PutItem", Table: "all", WindowStart: now, WindowEnd: now.Add(time.Minute)})
			},
		},
		{
			name: "service aggregate",
			call: func(db core.DB) error {
				return NewMetricsRepository(db, models.MainTableName, logger, nil).
					CreateAggregated(ctx, &models.AggregatedMetrics{Period: "minute", Type: "latency", Service: "api", WindowStart: now, WindowEnd: now.Add(time.Minute)})
			},
		},
		{
			name: "route window",
			call: func(db core.DB) error {
				return NewRoutingMetricsRepository(db, models.MainTableName, logger, nil).
					StoreRouteMetricsWindow(ctx, &models.RouteMetricsWindow{RouteID: "route-1", WindowStart: now, WindowSize: 5})
			},
		},
		{
			name: "health summary",
			call: func(db core.DB) error {
				summary := models.NewInstanceHealthSummary("remote.example", time.Hour)
				summary.LastUpdated = now
				return NewInstanceHealthRepository(db, models.MainTableName, logger, nil).SaveHealthSummary(ctx, summary)
			},
		},
		{
			name: "trust score",
			call: func(db core.DB) error {
				return NewTrustRepository(db, models.MainTableName, logger, nil).UpdateTrustScore(ctx, &storage.TrustScore{
					ActorID: "https://remote.example/users/alice", Category: "general", Score: 0.5, LastCalculated: now,
				})
			},
		},
		{
			name: "account note",
			call: func(db core.DB) error {
				return NewSocialRepository(db, models.MainTableName, logger, nil).CreateAccountNote(ctx, &storage.AccountNote{
					Username: "alice", TargetActorID: "https://remote.example/users/bob", Note: "note", CreatedAt: now, UpdatedAt: now,
				})
			},
		},
		{
			name: "marker",
			call: func(db core.DB) error {
				return NewMarkerRepository(db, models.MainTableName, logger, nil).SaveMarker(ctx, "alice", "home", "status-1", 1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call(upsertFailureDB(t, failure))
			require.Error(t, err)
		})
	}
}

func TestFederationReplayUpsertsRejectIncompleteInputs(t *testing.T) {
	repo := NewFederationRepository(upsertFailureDB(t, errors.New("not reached")), models.MainTableName, zap.NewNop(), nil, nil)
	ctx := context.Background()

	for name, call := range map[string]func() error{
		"acknowledge missing user":   func() error { return repo.AcknowledgeSeverance(ctx, "", "remote.example") },
		"acknowledge missing domain": func() error { return repo.AcknowledgeSeverance(ctx, "alice", "") },
		"reconnect missing user":     func() error { return repo.AttemptReconnection(ctx, "", "remote.example") },
		"reconnect missing domain":   func() error { return repo.AttemptReconnection(ctx, "alice", "") },
		"incomplete federation node": func() error { return repo.UpdateFederationNode(ctx, &storage.FederationNode{}) },
		"incomplete federation edge": func() error { return repo.UpdateFederationEdge(ctx, &storage.FederationEdge{}) },
		"federation edge missing target": func() error {
			return repo.UpdateFederationEdge(ctx, &storage.FederationEdge{SourceDomain: "source.example"})
		},
		"incomplete instance metadata":      func() error { return repo.UpdateInstanceMetadata(ctx, &storage.InstanceMetadata{}) },
		"incomplete federation time series": func() error { return repo.StoreFederationTimeSeries(ctx, &storage.FederationTimeSeries{}) },
		"federation time series missing period": func() error {
			return repo.StoreFederationTimeSeries(ctx, &storage.FederationTimeSeries{Domain: "remote.example"})
		},
		"incomplete detailed federation metrics": func() error { return repo.StoreDetailedFederationMetrics(ctx, &models.FederationAnalyticsTimeSeries{}) },
		"detailed metrics missing period": func() error {
			return repo.StoreDetailedFederationMetrics(ctx, &models.FederationAnalyticsTimeSeries{Domain: "remote.example"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			var validationErr common.ValidationError
			require.ErrorAs(t, call(), &validationErr)
		})
	}
}

func TestThreadReplayWritesExerciseFailureAndContinueSemantics(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("thread projection unavailable")
	db := upsertFailureDB(t, failure)
	repo := NewThreadRepository(db, zap.NewNop())

	require.Error(t, repo.SaveThreadSync(ctx, models.NewThreadSync("status-1")))
	require.Error(t, repo.SaveThreadNode(ctx, models.NewThreadNode("root-1", "status-1", "parent-1", 1, "alice")))
	require.Error(t, repo.SaveMissingReply(ctx, models.NewMissingReply("root-1", "parent-1", "reply-1")))
	require.NoError(t, repo.MarkMissingReplies(ctx, "root-1", "parent-1", []string{"reply-1", "reply-2"}), "individual replay failures are best effort")
	require.NoError(t, repo.BulkSaveThreadNodes(ctx, []*models.ThreadNode{
		models.NewThreadNode("root-1", "status-1", "parent-1", 1, "alice"),
		models.NewThreadNode("root-1", "status-2", "parent-1", 1, "bob"),
	}), "batch projection writes continue after individual failures")
}

func TestDraftReviewProjectionErrorsAreSurfaced(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("draft projection unavailable")
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", ctx).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Count").Return(int64(0), failure).Once()
	query.On("All", mock.AnythingOfType("*[]models.DraftReviewGrant")).Return(failure).Once()
	query.On("Create").Return(failure).Once()
	query.On("All", mock.AnythingOfType("*[]models.DraftReviewVerdict")).Return(failure).Once()

	repo := NewDraftRepository(db, models.MainTableName, zap.NewNop(), nil)
	_, err := repo.CountActiveDraftReviewGrants(ctx, "bob")
	require.ErrorIs(t, err, failure)
	_, err = repo.ListDraftReviewGrants(ctx, "alice", "draft-1")
	require.ErrorIs(t, err, failure)
	verdict := &models.DraftReviewVerdict{OwnerID: "alice", DraftID: "draft-1", Reviewer: "bob", Verdict: "approve", RecordedAt: time.Now().UTC()}
	require.ErrorIs(t, repo.CreateDraftReviewVerdict(ctx, verdict), failure)
	_, err = repo.ListDraftReviewVerdicts(ctx, "alice", "draft-1")
	require.ErrorIs(t, err, failure)
	require.Error(t, repo.CreateDraftReviewVerdict(ctx, &models.DraftReviewVerdict{}))
}

func TestAggregateReplayPathsSurfaceEachProjectionFailure(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("aggregate projection unavailable")
	db := upsertFailureDB(t, failure)
	logger := zap.NewNop()

	instanceRepo := NewInstanceRepository(db, models.MainTableName, logger)
	require.Error(t, instanceRepo.SetInstanceRules(ctx, []storage.InstanceRule{{ID: "1", Text: "be kind"}}))
	for name, metrics := range map[string]map[string]interface{}{
		"users":      {"total_users": int64(1)},
		"storage":    {"storage_bytes": int64(1)},
		"posts":      {"total_posts": int64(1)},
		"federation": {"known_instances": int64(1)},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, instanceRepo.RecordDailyMetrics(ctx, "2026-08-21", metrics))
		})
	}

	trendingRepo := NewTrendingRepository(db, logger, nil)
	require.Error(t, trendingRepo.UpdateTrendingHashtag(ctx, "golang", "2026-08-21", 2, 1))
	require.Error(t, trendingRepo.RecordModerationAction(ctx, "2026-08-21", "spam", &storage.ModerationAction{}))

	federationRepo := NewFederationRepository(db, models.MainTableName, logger, nil, nil)
	require.Error(t, federationRepo.StoreInstanceCluster(ctx, &storage.InstanceCluster{ClusterID: "cluster-1", Instances: []string{"remote.example"}}))
}

func TestRemoteActorReplayValidationAndConditionalFallback(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	invalidRepo := NewUserRepository(upsertFailureDB(t, errors.New("not reached")), models.MainTableName, logger)
	require.Error(t, invalidRepo.CacheRemoteActor(ctx, "alice@remote.example", nil, time.Hour))
	require.Error(t, invalidRepo.CacheRemoteActor(ctx, " ", &activitypub.Actor{}, time.Hour))

	failure := errors.New("conditional fallback update failed")
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", ctx).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("CreateOrUpdate").Return(dynamormerrors.ErrConditionFailed).Once()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Update", mock.Anything).Return(failure).Once()

	repo := NewUserRepository(db, models.MainTableName, logger)
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/alice"}, PreferredUsername: "alice"}
	err := repo.CacheRemoteActor(ctx, "alice@remote.example", actor, time.Hour)
	require.Error(t, err)
}

func TestFederationInstanceReplayReportsBothWriteFailures(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("instance upsert failed")
	db := upsertFailureDB(t, failure)
	repo := NewFederationActivityRepository(db, models.MainTableName, zap.NewNop(), nil)

	err := repo.UpdateInstanceInfo(ctx, &models.InstanceInfo{Domain: "remote.example"})
	require.Error(t, err)
}
