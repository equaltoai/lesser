package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeStorageAdapterRepos struct {
	actor        actorRepository
	object       objectRepository
	activity     activityRepository
	relationship relationshipRepository
	like         likeRepository
	analytics    analyticsRepository
	user         userRepository
	timeline     timelineRepository
	notification notificationRepository
	scheduled    scheduledStatusRepository
	instance     instanceRepository
	dlq          dlqRepository
	cloudwatch   cloudWatchMetricsRepository
	cost         costRepository
	domainBlock  domainBlockRepository
	federation   federationRepository

	db     dynamormDB
	rawDB  interface{}
	table  string
	logger *zap.Logger
}

func (f fakeStorageAdapterRepos) Actor() actorRepository                     { return f.actor }
func (f fakeStorageAdapterRepos) Object() objectRepository                   { return f.object }
func (f fakeStorageAdapterRepos) Activity() activityRepository               { return f.activity }
func (f fakeStorageAdapterRepos) Relationship() relationshipRepository       { return f.relationship }
func (f fakeStorageAdapterRepos) Like() likeRepository                       { return f.like }
func (f fakeStorageAdapterRepos) Analytics() analyticsRepository             { return f.analytics }
func (f fakeStorageAdapterRepos) User() userRepository                       { return f.user }
func (f fakeStorageAdapterRepos) Timeline() timelineRepository               { return f.timeline }
func (f fakeStorageAdapterRepos) Notification() notificationRepository       { return f.notification }
func (f fakeStorageAdapterRepos) ScheduledStatus() scheduledStatusRepository { return f.scheduled }
func (f fakeStorageAdapterRepos) Instance() instanceRepository               { return f.instance }
func (f fakeStorageAdapterRepos) DLQ() dlqRepository                         { return f.dlq }
func (f fakeStorageAdapterRepos) CloudWatchMetrics() cloudWatchMetricsRepository {
	return f.cloudwatch
}
func (f fakeStorageAdapterRepos) Cost() costRepository               { return f.cost }
func (f fakeStorageAdapterRepos) DomainBlock() domainBlockRepository { return f.domainBlock }
func (f fakeStorageAdapterRepos) Federation() federationRepository   { return f.federation }
func (f fakeStorageAdapterRepos) DB() dynamormDB                     { return f.db }
func (f fakeStorageAdapterRepos) GetDB() interface{}                 { return f.rawDB }
func (f fakeStorageAdapterRepos) GetTableName() string               { return f.table }
func (f fakeStorageAdapterRepos) GetLogger() *zap.Logger             { return f.logger }

type fakeDB struct {
	firstFn func(model any, dest any) error
}

func (f *fakeDB) WithContext(context.Context) dynamormDB { return f }
func (f *fakeDB) Model(model any) dynamormQuery          { return &fakeQuery{db: f, model: model} }

type fakeQuery struct {
	db    *fakeDB
	model any
}

func (q *fakeQuery) Where(string, string, any) dynamormQuery { return q }
func (q *fakeQuery) Limit(int) dynamormQuery                 { return q }
func (q *fakeQuery) First(dest any) error {
	if q.db.firstFn == nil {
		return nil
	}
	return q.db.firstFn(q.model, dest)
}

type fakeActorRepo struct {
	actor *activitypub.Actor
	err   error
}

func (f fakeActorRepo) GetActor(context.Context, string) (*activitypub.Actor, error) {
	return f.actor, f.err
}

type fakeObjectRepo struct {
	createErr    error
	getValue     any
	getErr       error
	tombstoneErr error
	incrementErr error
}

func (f fakeObjectRepo) CreateObject(context.Context, any) error               { return f.createErr }
func (f fakeObjectRepo) GetObject(context.Context, string) (any, error)        { return f.getValue, f.getErr }
func (f fakeObjectRepo) TombstoneObject(context.Context, string, string) error { return f.tombstoneErr }
func (f fakeObjectRepo) IncrementReplyCount(context.Context, string) error     { return f.incrementErr }

type fakeActivityRepo struct{ err error }

func (f fakeActivityRepo) CreateActivity(context.Context, *activitypub.Activity) error { return f.err }

type fakeRelationshipRepo struct {
	getRelationshipFn func(ctx context.Context, followerUsername, followingID string) (*models.RelationshipRecord, error)
	getFollowersFn    func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	createErr         error
	deleteErr         error
}

func (f fakeRelationshipRepo) CreateRelationship(context.Context, string, string, string) error {
	return f.createErr
}
func (f fakeRelationshipRepo) DeleteRelationship(context.Context, string, string) error {
	return f.deleteErr
}
func (f fakeRelationshipRepo) GetRelationship(ctx context.Context, followerUsername, followingID string) (*models.RelationshipRecord, error) {
	if f.getRelationshipFn == nil {
		return nil, errors.New("GetRelationship not configured")
	}
	return f.getRelationshipFn(ctx, followerUsername, followingID)
}
func (f fakeRelationshipRepo) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if f.getFollowersFn == nil {
		return nil, "", errors.New("GetFollowers not configured")
	}
	return f.getFollowersFn(ctx, username, limit, cursor)
}

type fakeLikeRepo struct {
	hasLiked bool
	err      error
}

func (f fakeLikeRepo) CreateLike(context.Context, string, string, string) (*models.Like, error) {
	return &models.Like{}, f.err
}
func (f fakeLikeRepo) DeleteLike(context.Context, string, string) error { return f.err }
func (f fakeLikeRepo) HasLiked(context.Context, string, string) (bool, error) {
	return f.hasLiked, f.err
}

type fakeAnalyticsRepo struct {
	totalUsersErr  error
	totalUsers     int
	activeUsersErr error
	activeUsers    int

	recordMetricFn func(ctx context.Context, date, metricType string, value int64) error
}

func (f fakeAnalyticsRepo) RecordInstanceMetric(ctx context.Context, date, metricType string, value int64) error {
	if f.recordMetricFn == nil {
		return nil
	}
	return f.recordMetricFn(ctx, date, metricType, value)
}
func (f fakeAnalyticsRepo) RecordHashtagUsage(context.Context, string, string, string) error {
	return nil
}
func (f fakeAnalyticsRepo) RecordLinkShare(context.Context, string, string, string) error { return nil }
func (f fakeAnalyticsRepo) RecordStatusEngagement(context.Context, string, string, string) error {
	return nil
}
func (f fakeAnalyticsRepo) GetTotalUserCount(context.Context) (int, error) {
	return f.totalUsers, f.totalUsersErr
}
func (f fakeAnalyticsRepo) GetActiveUserCount(context.Context, int) (int, error) {
	return f.activeUsers, f.activeUsersErr
}

type fakeUserRepo struct{ err error }

func (f fakeUserRepo) FanOutPost(context.Context, *activitypub.Activity) error { return f.err }

type fakeTimelineRepo struct{ err error }

func (f fakeTimelineRepo) RemoveFromTimelines(context.Context, string) error { return f.err }

type fakeNotificationRepo struct {
	createErr error
	deleteErr error
}

func (f fakeNotificationRepo) CreateNotification(context.Context, *models.Notification) error {
	return f.createErr
}
func (f fakeNotificationRepo) DeleteNotificationsByObject(context.Context, string) error {
	return f.deleteErr
}

type fakeScheduledStatusRepo struct{}

func (fakeScheduledStatusRepo) CreateScheduledStatus(context.Context, *storage.ScheduledStatus) error {
	return nil
}
func (fakeScheduledStatusRepo) GetScheduledStatus(context.Context, string) (*storage.ScheduledStatus, error) {
	return nil, nil
}
func (fakeScheduledStatusRepo) GetScheduledStatuses(context.Context, string, int, string) ([]*storage.ScheduledStatus, string, error) {
	return nil, "", nil
}
func (fakeScheduledStatusRepo) UpdateScheduledStatus(context.Context, *storage.ScheduledStatus) error {
	return nil
}
func (fakeScheduledStatusRepo) DeleteScheduledStatus(context.Context, string) error { return nil }
func (fakeScheduledStatusRepo) GetDueScheduledStatuses(context.Context, time.Time, int) ([]*storage.ScheduledStatus, error) {
	return nil, nil
}
func (fakeScheduledStatusRepo) MarkScheduledStatusPublished(context.Context, string) error {
	return nil
}

type fakeInstanceRepo struct {
	sleep time.Duration
	err   error
}

func (f fakeInstanceRepo) GetInstanceRules(context.Context) ([]storage.InstanceRule, error) {
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	return nil, f.err
}

type fakeDLQRepo struct {
	messages []*models.DLQMessage
	err      error
}

func (f fakeDLQRepo) GetDLQMessagesForReprocessing(context.Context, string, string, int, string) ([]*models.DLQMessage, string, error) {
	return f.messages, "", f.err
}

type fakeCloudWatchRepo struct {
	metrics *repositories.ServiceMetrics
	err     error
}

func (f fakeCloudWatchRepo) GetServiceMetrics(context.Context, string, time.Duration) (*repositories.ServiceMetrics, error) {
	return f.metrics, f.err
}

type fakeCostRepo struct {
	monthly       *repositories.MonthlyAggregate
	monthlyErr    error
	projection    *storage.CostProjection
	projectionErr error
}

func (f fakeCostRepo) GetMonthlyAggregate(context.Context, int, int) (*repositories.MonthlyAggregate, error) {
	return f.monthly, f.monthlyErr
}
func (f fakeCostRepo) GetCostProjections(context.Context, string) (*storage.CostProjection, error) {
	return f.projection, f.projectionErr
}

type fakeDomainBlockRepo struct {
	blocked bool
	err     error
}

func (f fakeDomainBlockRepo) IsInstanceDomainBlocked(context.Context, string) (bool, *storage.InstanceDomainBlock, error) {
	if f.err != nil {
		return false, nil, f.err
	}
	if !f.blocked {
		return false, nil, nil
	}
	return true, &storage.InstanceDomainBlock{Domain: "blocked.example"}, nil
}

type fakeFederationRepo struct {
	score float64
	err   error
}

func (f fakeFederationRepo) GetDomainHealthScore(context.Context, string) (float64, error) {
	return f.score, f.err
}

func TestRepositoryStorageAdapter_BasicOperations(t *testing.T) {
	ctx := context.Background()

	var recordedDate string
	var recordedMetric string

	repos := fakeStorageAdapterRepos{
		actor:    fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}, err: nil},
		object:   fakeObjectRepo{getValue: "obj", getErr: nil},
		activity: fakeActivityRepo{},
		relationship: fakeRelationshipRepo{getRelationshipFn: func(context.Context, string, string) (*models.RelationshipRecord, error) {
			return &models.RelationshipRecord{}, nil
		}},
		like: fakeLikeRepo{hasLiked: true},
		analytics: fakeAnalyticsRepo{
			recordMetricFn: func(_ context.Context, date, metricType string, _ int64) error {
				recordedDate = date
				recordedMetric = metricType
				return nil
			},
		},
		user:         fakeUserRepo{},
		timeline:     fakeTimelineRepo{},
		notification: fakeNotificationRepo{},
		scheduled:    fakeScheduledStatusRepo{},
		rawDB:        "raw-db",
		table:        "tbl",
		logger:       zap.NewNop(),
	}

	a := &repositoryStorageAdapter{repos: repos}

	actor, err := a.GetActor(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "alice", actor.PreferredUsername)

	require.NoError(t, a.CreateObject(ctx, "obj"))
	obj, err := a.GetObject(ctx, "id")
	require.NoError(t, err)
	require.Equal(t, "obj", obj)

	require.NoError(t, a.TombstoneObject(ctx, "id", "actor"))
	require.NoError(t, a.IncrementReplyCount(ctx, "id"))
	require.NoError(t, a.CreateActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1"}}))

	require.NoError(t, a.CreateRelationship(ctx, "alice", "bob", "act"))
	require.NoError(t, a.RemoveRelationship(ctx, "alice", "bob"))

	isFollowing, err := a.IsFollowing(ctx, "alice", "bob")
	require.NoError(t, err)
	require.True(t, isFollowing)

	require.NoError(t, a.CreateLike(ctx, "alice", "obj", "act"))
	require.NoError(t, a.RemoveLike(ctx, "alice", "obj"))

	liked, err := a.HasLiked(ctx, "alice", "obj")
	require.NoError(t, err)
	require.True(t, liked)

	ts := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, a.RecordActivity(ctx, "create", "ignored", ts))
	require.Equal(t, "2025-01-02", recordedDate)
	require.Equal(t, "create", recordedMetric)

	require.NoError(t, a.RecordInstanceActivity(ctx, "login", ts))
	require.NoError(t, a.RecordHashtagUsage(ctx, "#tag", "obj", "actor"))
	require.NoError(t, a.RecordLinkShare(ctx, "https://example.com", "obj", "actor"))
	require.NoError(t, a.RecordStatusEngagement(ctx, "obj", "like", "actor"))

	require.NoError(t, a.FanOutPost(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a1"}}))
	require.NoError(t, a.RemoveFromTimelines(ctx, "obj"))

	repos.relationship = fakeRelationshipRepo{
		getRelationshipFn: func(context.Context, string, string) (*models.RelationshipRecord, error) {
			return nil, errors.New("not found")
		},
		getFollowersFn: func(context.Context, string, int, string) ([]string, string, error) {
			return []string{"a", "b"}, "cursor", nil
		},
	}
	a.repos = repos

	isFollowing, err = a.IsFollowing(ctx, "alice", "bob")
	require.NoError(t, err)
	require.False(t, isFollowing)

	followers, cursor, err := a.GetFollowers(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, followers)
	require.Equal(t, "cursor", cursor)

	require.NotNil(t, a.ScheduledStatus())
	require.Equal(t, "raw-db", a.GetDB())
	require.Equal(t, "tbl", a.GetTableName())

	// Notification type conversion branch.
	require.ErrorIs(t, a.CreateNotification(ctx, &struct{}{}), ErrInvalidNotificationType)
	require.NoError(t, a.CreateNotification(ctx, &models.Notification{}))
	require.NoError(t, a.DeleteNotificationsByObject(ctx, "obj"))
}

func TestRepositoryStorageAdapter_HealthChecksAndAlerts(t *testing.T) {
	ctx := context.Background()

	t.Run("database_nil_is_down", func(t *testing.T) {
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{table: "tbl", logger: zap.NewNop(), db: nil}}
		dbs, healthy := a.checkDatabaseHealth(ctx)
		require.False(t, healthy)
		require.Len(t, dbs, 1)
		require.Equal(t, model.HealthStatusDown, dbs[0].Status)
	})

	t.Run("database_not_found_is_healthy_and_latency_thresholds_work", func(t *testing.T) {
		origDegraded := databaseHealthDegradedThreshold
		origDown := databaseHealthDownThreshold
		t.Cleanup(func() {
			databaseHealthDegradedThreshold = origDegraded
			databaseHealthDownThreshold = origDown
		})

		databaseHealthDegradedThreshold = 0
		databaseHealthDownThreshold = time.Hour

		db := &fakeDB{firstFn: func(_ any, _ any) error {
			time.Sleep(1 * time.Millisecond)
			return errors.New("not found")
		}}

		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			table:  "tbl",
			logger: zap.NewNop(),
			db:     db,
		}}

		dbs, healthy := a.checkDatabaseHealth(ctx)
		require.True(t, healthy)
		require.Equal(t, model.HealthStatusDegraded, dbs[0].Status)
	})

	t.Run("database_real_error_is_down", func(t *testing.T) {
		db := &fakeDB{firstFn: func(_ any, _ any) error { return errors.New("boom") }}
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{table: "tbl", logger: zap.NewNop(), db: db}}
		dbs, healthy := a.checkDatabaseHealth(ctx)
		require.False(t, healthy)
		require.Equal(t, model.HealthStatusDown, dbs[0].Status)
	})

	t.Run("service_health_latency_and_error_rate", func(t *testing.T) {
		origServiceThreshold := serviceHealthDownLatencyThreshold
		t.Cleanup(func() { serviceHealthDownLatencyThreshold = origServiceThreshold })
		serviceHealthDownLatencyThreshold = 0

		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger:     zap.NewNop(),
			instance:   fakeInstanceRepo{sleep: 1 * time.Millisecond, err: errors.New("repo down")},
			cloudwatch: fakeCloudWatchRepo{metrics: &repositories.ServiceMetrics{RequestCount: 10, ErrorCount: 2, DynamoDBReads: 100, DynamoDBWrites: 50, EstimatedCostUSD: 20.0}},
		}}

		svcs, healthy := a.checkServiceHealth(ctx)
		require.False(t, healthy)
		require.Len(t, svcs, 2)
		require.Equal(t, model.HealthStatusDown, svcs[0].Status) // API
		require.Equal(t, model.HealthStatusDown, svcs[1].Status) // Database (error rate > 0.1)
	})

	t.Run("queue_health_sets_oldest_message", func(t *testing.T) {
		now := time.Now()
		msg1 := &models.DLQMessage{FirstSeenAt: now.Add(-2 * time.Hour)}
		msg2 := &models.DLQMessage{FirstSeenAt: now.Add(-1 * time.Hour)}

		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger: zap.NewNop(),
			dlq:    fakeDLQRepo{messages: []*models.DLQMessage{msg2, msg1}},
		}}

		queues, healthy := a.checkQueueHealth(ctx)
		require.True(t, healthy)
		require.Len(t, queues, 1)
		require.Equal(t, 2, queues[0].Depth)
		require.NotNil(t, queues[0].OldestMessage)
	})

	t.Run("generate_infrastructure_alerts_covers_all_categories", func(t *testing.T) {
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger: zap.NewNop(),
			cloudwatch: fakeCloudWatchRepo{metrics: &repositories.ServiceMetrics{
				EstimatedCostUSD: 20.0,
				RequestCount:     100,
				ErrorCount:       0,
			}},
		}}

		alerts := a.generateInfrastructureAlerts(ctx,
			[]*model.DatabaseStatus{
				{Name: "db", Status: model.HealthStatusDown, Latency: model.Duration(time.Second)},
				{Name: "db2", Status: model.HealthStatusDegraded, Latency: model.Duration(2 * time.Second)},
			},
			[]*model.ServiceStatus{
				{Name: "svc", Status: model.HealthStatusDown, ErrorRate: 0.2},
				{Name: "svc2", Status: model.HealthStatusHealthy, ErrorRate: 0.06},
			},
			[]*model.QueueStatus{
				{Name: "q1", Depth: 2000},
				{Name: "q2", Depth: 200},
			},
		)

		require.NotEmpty(t, alerts)
	})
}

func TestRepositoryStorageAdapter_BudgetsHealthReportAndRelationships(t *testing.T) {
	ctx := context.Background()

	t.Run("instance_budgets_default_on_missing_data", func(t *testing.T) {
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger: zap.NewNop(),
			cost:   fakeCostRepo{monthlyErr: errors.New("no data")},
		}}

		budgets, err := a.GetInstanceBudgets(ctx, nil)
		require.NoError(t, err)
		require.Len(t, budgets, 1)
		require.Equal(t, 0.0, budgets[0].CurrentSpendUsd)
	})

	t.Run("instance_budgets_projection_and_exceeded_filter", func(t *testing.T) {
		proj := &storage.CostProjection{ProjectedCost: 150.0, Period: "monthly"}
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger: zap.NewNop(),
			cost: fakeCostRepo{
				monthly:    &repositories.MonthlyAggregate{TotalCostDollars: 110.0},
				projection: proj,
			},
		}}

		exceeded := false
		budgets, err := a.GetInstanceBudgets(ctx, &exceeded)
		require.NoError(t, err)
		require.Empty(t, budgets)

		exceeded = true
		budgets, err = a.GetInstanceBudgets(ctx, &exceeded)
		require.NoError(t, err)
		require.Len(t, budgets, 1)
		require.NotNil(t, budgets[0].ProjectedOverspend)
	})

	t.Run("instance_health_report_localhost_defaults_and_recommendations", func(t *testing.T) {
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger:    zap.NewNop(),
			analytics: fakeAnalyticsRepo{totalUsers: 10, activeUsers: 1},
			cost:      fakeCostRepo{monthlyErr: errors.New("no data")},
			db:        &fakeDB{},
		}}

		report, err := a.GetInstanceHealthReport(ctx, "localhost")
		require.NoError(t, err)
		require.Equal(t, model.InstanceHealthStatusHealthy, report.Status)
		require.NotEmpty(t, report.Recommendations)
	})

	t.Run("instance_health_report_remote_with_metrics_triggers_issues", func(t *testing.T) {
		db := &fakeDB{firstFn: func(model any, dest any) error {
			if _, ok := model.(*models.FederationInstanceHealthTracking); !ok {
				return nil
			}

			out, ok := dest.(**models.FederationInstanceHealthTracking)
			require.True(t, ok)
			*out = &models.FederationInstanceHealthTracking{
				AverageResponseTime: 3000,
				SuccessRate:         0.80,
				ResponseTimeP95:     6000,
				ConsecutiveFails:    11,
				IsHealthy:           false,
			}
			return nil
		}}

		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger:    zap.NewNop(),
			analytics: fakeAnalyticsRepo{totalUsers: 10, activeUsers: 10},
			cost:      fakeCostRepo{monthly: &repositories.MonthlyAggregate{TotalCostDollars: 20.0}},
			db:        db,
		}}

		report, err := a.GetInstanceHealthReport(ctx, "remote.example")
		require.NoError(t, err)
		require.Equal(t, model.InstanceHealthStatusCritical, report.Status)
		require.NotEmpty(t, report.Issues)
	})

	t.Run("calculate_federation_score_bounds_and_error", func(t *testing.T) {
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger:     zap.NewNop(),
			federation: fakeFederationRepo{err: errors.New("boom")},
		}}
		_, err := a.calculateFederationScore(ctx, "remote.example")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGetDomainHealthScore)

		a.repos = fakeStorageAdapterRepos{logger: zap.NewNop(), federation: fakeFederationRepo{score: -10}}
		score, err := a.calculateFederationScore(ctx, "remote.example")
		require.NoError(t, err)
		require.Equal(t, 0.0, score)

		a.repos = fakeStorageAdapterRepos{logger: zap.NewNop(), federation: fakeFederationRepo{score: 120}}
		score, err = a.calculateFederationScore(ctx, "remote.example")
		require.NoError(t, err)
		require.Equal(t, 1.0, score)
	})

	t.Run("instance_relationships_recommendations", func(t *testing.T) {
		db := &fakeDB{firstFn: func(model any, dest any) error {
			if _, ok := model.(*models.FederationInstanceHealthTracking); !ok {
				return nil
			}
			out, ok := dest.(**models.FederationInstanceHealthTracking)
			require.True(t, ok)
			*out = &models.FederationInstanceHealthTracking{
				IsHealthy:        false,
				ConsecutiveFails: 6,
				ResponseTimeP95:  11000,
			}
			return nil
		}}

		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			logger:      zap.NewNop(),
			db:          db,
			federation:  fakeFederationRepo{score: 95},
			domainBlock: fakeDomainBlockRepo{blocked: true},
		}}

		rel, err := a.GetInstanceRelationships(ctx, "remote.example")
		require.NoError(t, err)
		require.Equal(t, "remote.example", rel.Domain)
		require.NotEmpty(t, rel.Recommendations)
	})
}

func TestIsNotFoundErrorAndCreateStorageAdapter(t *testing.T) {
	require.False(t, IsNotFoundError(nil))
	require.True(t, IsNotFoundError(errors.New("not found")))
	require.True(t, IsNotFoundError(errors.New("item not found")))
	require.True(t, IsNotFoundError(errors.New("record not found")))
	require.False(t, IsNotFoundError(errors.New("other")))

	require.Panics(t, func() {
		_ = CreateStorageAdapter(fmt.Sprintf("%s", "not repos"))
	})
}

func TestStorageAdapter_WrappersAndInfrastructureHealth(t *testing.T) {
	t.Run("repository_storage_wrapper_calls", func(t *testing.T) {
		// `mockStorage` is defined in `pkg/services/registry_test.go` and implements `core.RepositoryStorage`.
		var repos core.RepositoryStorage = newMockStorage()

		w := repositoryStorageWrapper{repos: repos}

		_ = w.Actor()
		_ = w.Object()
		_ = w.Activity()
		_ = w.Relationship()
		_ = w.Like()
		_ = w.Analytics()
		_ = w.User()
		_ = w.Timeline()
		_ = w.Notification()
		_ = w.ScheduledStatus()
		_ = w.Instance()
		_ = w.DLQ()
		_ = w.CloudWatchMetrics()
		_ = w.Cost()
		_ = w.DomainBlock()
		_ = w.Federation()

		require.Nil(t, w.DB())
		require.Nil(t, w.GetDB())
		require.Equal(t, "test-table", w.GetTableName())
		require.NotNil(t, w.GetLogger())
	})

	t.Run("dynamorm_wrappers_nil_branches", func(t *testing.T) {
		db := dynamormDBWrapper{}
		require.Nil(t, db.WithContext(context.Background()))

		query := db.Model(&struct{}{})
		query = query.Where("PK", "=", "x").Limit(1)
		err := query.First(&struct{}{})
		require.Error(t, err)
	})

	t.Run("get_infrastructure_health_runs_end_to_end", func(t *testing.T) {
		db := &fakeDB{firstFn: func(_ any, _ any) error { return errors.New("not found") }}
		a := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			table:    "tbl",
			logger:   zap.NewNop(),
			db:       db,
			instance: fakeInstanceRepo{},
			dlq:      fakeDLQRepo{messages: nil},
			cost:     fakeCostRepo{monthlyErr: errors.New("unused")},
		}}

		status, err := a.GetInfrastructureHealth(context.Background())
		require.NoError(t, err)
		require.NotNil(t, status)
	})
}
