package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func newReplayRefreshDB(t *testing.T, model any) (core.DB, *fakedb.Fake) {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(model))
	return db, fake
}

func TestAggregateRepositoriesRefreshDeterministicWindowsOnReplay(t *testing.T) {
	ctx := context.Background()
	windowStart := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Minute)

	t.Run("dynamodb cost", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.DynamoDBCostAggregation{})
		repo := NewTrackingRepository(db, models.MainTableName, zap.NewNop(), nil)
		agg := &models.DynamoDBCostAggregation{Period: "minute", OperationType: "PutItem", Table: "all", WindowStart: windowStart, WindowEnd: windowEnd, TotalOperations: 1}
		require.NoError(t, repo.CreateAggregated(ctx, agg))
		agg.TotalOperations = 2
		require.NoError(t, repo.CreateAggregated(ctx, agg))
		require.Len(t, fake.Items(models.MainTableName), 1)

		var got models.DynamoDBCostAggregation
		require.NoError(t, db.Model(&models.DynamoDBCostAggregation{}).Where("PK", "=", agg.PK).Where("SK", "=", agg.SK).First(&got))
		require.EqualValues(t, 2, got.TotalOperations)
	})

	t.Run("service metrics", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.AggregatedMetrics{})
		repo := NewMetricsRepository(db, models.MainTableName, zap.NewNop(), nil)
		agg := &models.AggregatedMetrics{Period: "minute", Type: "latency", Service: "api", WindowStart: windowStart, WindowEnd: windowEnd, TotalCount: 1}
		require.NoError(t, repo.CreateAggregated(ctx, agg))
		agg.TotalCount = 2
		require.NoError(t, repo.CreateAggregated(ctx, agg))
		require.Len(t, fake.Items(models.MainTableName), 1)

		var got models.AggregatedMetrics
		require.NoError(t, db.Model(&models.AggregatedMetrics{}).Where("PK", "=", agg.PK).Where("SK", "=", agg.SK).First(&got))
		require.EqualValues(t, 2, got.TotalCount)
	})
}

func TestRoutingMetricWindowsRefreshOnReplay(t *testing.T) {
	ctx := context.Background()
	db, fake := newReplayRefreshDB(t, &models.RouteMetricsWindow{})
	repo := NewRoutingMetricsRepository(db, models.MainTableName, zap.NewNop(), nil)
	start := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	route := &models.RouteMetricsWindow{RouteID: "route-1", WindowStart: start, WindowSize: 5, MessageCount: 1}
	global := &models.GlobalMetricsWindow{WindowStart: start, WindowSize: 5, TotalMessages: 1}
	instance := &models.InstanceMetricsWindow{InstanceID: "remote.example", WindowStart: start, WindowSize: 5, TotalMessages: 1}

	for _, write := range []func() error{
		func() error { return repo.StoreRouteMetricsWindow(ctx, route) },
		func() error { return repo.StoreGlobalMetricsWindow(ctx, global) },
		func() error { return repo.StoreInstanceMetricsWindow(ctx, instance) },
	} {
		require.NoError(t, write())
		require.NoError(t, write())
	}
	require.Len(t, fake.Items(models.MainTableName), 3)

	route.MessageCount = 2
	global.TotalMessages = 2
	instance.TotalMessages = 2
	require.NoError(t, repo.BatchStoreMetrics(ctx, []*models.RouteMetricsWindow{route}, []*models.InstanceMetricsWindow{instance}, global))
	require.NoError(t, repo.BatchStoreMetrics(ctx, []*models.RouteMetricsWindow{route}, []*models.InstanceMetricsWindow{instance}, global))
	require.Len(t, fake.Items(models.MainTableName), 3)
}

func TestConfigurationCacheAndSummaryWritesRefreshOnReplay(t *testing.T) {
	ctx := context.Background()

	t.Run("instance rules description and daily metrics", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.InstanceConfig{})
		repo := NewInstanceRepository(db, models.MainTableName, zap.NewNop())
		rules := []storage.InstanceRule{{ID: "1", Text: "be kind"}}
		require.NoError(t, repo.SetInstanceRules(ctx, rules))
		rules[0].Text = "be kinder"
		require.NoError(t, repo.SetInstanceRules(ctx, rules))
		require.NoError(t, repo.SetExtendedDescription(ctx, "first"))
		require.NoError(t, repo.SetExtendedDescription(ctx, "second"))
		require.Len(t, fake.Items(models.MainTableName), 2)

		description, _, err := repo.GetExtendedDescription(ctx)
		require.NoError(t, err)
		require.Equal(t, "second", description)
	})

	t.Run("daily history snapshot", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.InstanceHistory{})
		repo := NewInstanceRepository(db, models.MainTableName, zap.NewNop())
		metrics := map[string]interface{}{
			"total_users": int64(10), "active_users": int64(4),
			"storage_bytes": int64(20), "total_posts": int64(30),
			"known_instances": int64(40),
		}
		require.NoError(t, repo.RecordDailyMetrics(ctx, "2026-08-20", metrics))
		metrics["total_users"] = int64(11)
		require.NoError(t, repo.RecordDailyMetrics(ctx, "2026-08-20", metrics))
		require.Len(t, fake.Items(models.MainTableName), 4)
	})

	t.Run("instance health summary", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.InstanceHealthSummary{})
		repo := NewInstanceHealthRepository(db, models.MainTableName, zap.NewNop(), nil)
		summary := models.NewInstanceHealthSummary("remote.example", time.Hour)
		summary.SampleCount = 1
		summary.HealthScore = 100
		summary.Availability = 1
		require.NoError(t, repo.SaveHealthSummary(ctx, summary))
		summary.SampleCount = 2
		require.NoError(t, repo.SaveHealthSummary(ctx, summary))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("query cache", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.QueryCacheEntry{})
		repo := NewQueryCacheRepository(db, models.MainTableName, zap.NewNop(), nil, nil, nil)
		require.NoError(t, repo.SetCachedValue(ctx, "timeline:alice", map[string]any{"value": 1}, 1, time.Hour))
		require.NoError(t, repo.SetCachedValue(ctx, "timeline:alice", map[string]any{"value": 2}, 1, time.Hour))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("public key rotation cache", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.PublicKeyCache{})
		repo := NewPublicKeyCacheRepository(db, models.MainTableName, zap.NewNop(), nil)
		_, err := repo.Store(ctx, "https://remote.example/users/alice", "#old", "old-pem", "rsa-sha256")
		require.NoError(t, err)
		_, err = repo.Store(ctx, "https://remote.example/users/alice", "#new", "new-pem", "rsa-sha256")
		require.NoError(t, err)
		require.Len(t, fake.Items(models.MainTableName), 1)
		got, err := repo.GetByActorURL(ctx, "https://remote.example/users/alice")
		require.NoError(t, err)
		require.Equal(t, "#new", got.KeyID)
		require.Equal(t, "new-pem", got.PublicKeyPEM)
	})

	t.Run("compiled pattern cache", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.PatternCache{})
		repo := NewEnhancedPatternRepository(db, models.MainTableName, zap.NewNop(), nil)
		cache := &models.PatternCache{PatternID: "pattern-1", PatternType: "spam", CompilationHash: "old", CompiledData: map[string]interface{}{"version": "old"}}
		require.NoError(t, repo.SetPatternCache(ctx, cache))
		cache.CompilationHash = "new"
		cache.CompiledData = map[string]interface{}{"version": "new"}
		require.NoError(t, repo.SetPatternCache(ctx, cache))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})
}

func TestDuplicateClientStateCreatesAreIdempotent(t *testing.T) {
	ctx := context.Background()

	t.Run("announcement dismissal", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.AnnouncementDismissal{})
		repo := NewAnnouncementRepository(db, models.MainTableName, zap.NewNop())
		require.NoError(t, repo.DismissAnnouncement(ctx, "alice", "announcement-1"))
		require.NoError(t, repo.DismissAnnouncement(ctx, "alice", "announcement-1"))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("conversation mute", func(t *testing.T) {
		db, fake := newReplayRefreshDB(t, &models.ConversationMute{})
		repo := NewConversationRepository(db, models.MainTableName, zap.NewNop(), nil)
		createdAt := time.Now().UTC()
		firstExpiry := createdAt.Add(time.Hour)
		secondExpiry := createdAt.Add(24 * time.Hour)
		mute := &storage.ConversationMute{
			Username:       "alice",
			ConversationID: "conversation-1",
			CreatedAt:      createdAt,
			ExpiresAt:      firstExpiry,
		}
		require.NoError(t, repo.CreateConversationMute(ctx, mute))
		mute.ExpiresAt = secondExpiry
		require.NoError(t, repo.CreateConversationMute(ctx, mute))
		require.Len(t, fake.Items(models.MainTableName), 1)

		var persisted models.ConversationMute
		require.NoError(t, db.Model(&models.ConversationMute{}).
			Where("PK", "=", "USER#alice").
			Where("SK", "=", "CONVERSATION_MUTE#conversation-1").
			First(&persisted))
		require.Equal(t, secondExpiry, persisted.ExpiresAt)
		require.Equal(t, secondExpiry.Unix(), persisted.TTL)
	})
}

func TestAccountRemoteActorCacheRefreshesOnReplay(t *testing.T) {
	ctx := context.Background()
	db, fake := newReplayRefreshDB(t, &models.Actor{})
	repo := NewAccountRepository(db, models.MainTableName, "example.com", zap.NewNop())
	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: activitypub.PersonType},
		PreferredUsername: "alice",
		Name:              "Old name",
	}
	require.NoError(t, repo.CacheRemoteActor(ctx, actor))
	actor.Name = "New name"
	require.NoError(t, repo.CacheRemoteActor(ctx, actor))
	require.Len(t, fake.Items(models.MainTableName), 1)

	var got models.Actor
	require.NoError(t, db.Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#alice@remote.example").
		Where("SK", "=", models.SKProfile).
		First(&got))
	require.NotNil(t, got.Actor)
	require.Equal(t, "New name", got.Actor.Name)
}
