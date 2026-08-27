package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// This file pins the unbounded-scan elimination wave (#1469, batch F): every
// read path of pkg/storage/repositories/federation_repository.go must resolve
// through a keyed query (primary or GSI) on the pre-provisioned gsi1-gsi8
// slots. Any regression that reintroduces a request-adjacent DynamoDB Scan
// fails the test through newWave1469ScanForbiddingTestDB (the fake overrides
// the DynamoDB client Scan method itself).
//
// Batch F eliminated all 22 baselined federation `.Scan` sites: 21 keyed
// partition/GSI queries converted `.Scan` -> `.All`, and the one key-less
// site (GetAllFederationEdges pagination) rerouted to a new additive GSI3
// listing key on FederationEdge (FED_EDGES#ALL). Legacy-row consequences per
// site are documented in docs/architecture/dynamodb-scan-inventory.md.

func newScanFreeFederationRepo(t *testing.T, modelTypes ...any) (*FederationRepository, *wave1469ScanForbiddingDB) {
	t.Helper()
	db, s := newWave1469ScanForbiddingTestDB(t, modelTypes...)
	return NewFederationRepository(db, "test-table", zap.NewNop(), nil, nil), s
}

// 1) GetKnownInstances — keyed gsi1 (FEDERATION_ACTIVE).
func TestScanFreeWave_Federation_GetKnownInstances(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationInstance{})

	inst := &models.FederationInstance{Domain: "remote.example", Software: "mastodon", LastSeen: time.Now()}
	inst.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(inst).Create())

	got, next, err := repo.GetKnownInstances(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetKnownInstances must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
	require.Equal(t, "", next)
}

// 2) GetFederationStatistics — keyed gsi1 with gsi1SK time range.
func TestScanFreeWave_Federation_GetFederationStatistics(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationInstance{})

	now := time.Now()
	for _, d := range []string{"a.example", "b.example"} {
		inst := &models.FederationInstance{Domain: d, LastSeen: now, ActiveUsers: 7, TotalMessages: 100}
		inst.UpdateKeys()
		require.NoError(t, repo.db.WithContext(ctx).Model(inst).Create())
	}

	stats, err := repo.GetFederationStatistics(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationStatistics must not scan")
	require.Equal(t, int64(2), stats.ActiveInstances)
	require.Equal(t, int64(200), stats.TotalMessages)
	require.Equal(t, int64(14), stats.TotalUsers)
}

// 3) GetInstanceStats — keyed PK (FEDERATION#domain#month) + SK range.
// Two rows are seeded (now and now-24h) so the internally computed month
// partition matches regardless of a month-boundary crossing.
func TestScanFreeWave_Federation_GetInstanceStats(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationInstance{}, &models.FederationCostActivity{})

	inst := &models.FederationInstance{Domain: "remote.example", Software: "mastodon"}
	inst.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(inst).Create())

	now := time.Now()
	for i, ts := range []time.Time{now, now.Add(-24 * time.Hour)} {
		act := &models.FederationCostActivity{ID: fmt.Sprintf("act-%d", i), Domain: "remote.example", Timestamp: ts, Success: true, ResponseTime: 12}
		require.NoError(t, act.UpdateKeys())
		require.NoError(t, repo.db.WithContext(ctx).Model(act).Create())
	}

	stats, err := repo.GetInstanceStats(ctx, "remote.example")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetInstanceStats must not scan")
	require.GreaterOrEqual(t, stats.TotalRequests, int64(1))
	require.GreaterOrEqual(t, stats.TotalMessages, int64(0))
}

// 4) GetFederationCosts — keyed PK (FEDERATION_COSTS#month).
func TestScanFreeWave_Federation_GetFederationCosts(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationCost{})

	now := time.Now()
	cost := &models.FederationCost{Domain: "remote.example", EstimatedCostUSD: 12.5}
	cost.PK = fmt.Sprintf("FEDERATION_COSTS#%s", now.Format(common.MonthFormat))
	cost.SK = fmt.Sprintf("DOMAIN#%s", cost.Domain)
	require.NoError(t, repo.db.WithContext(ctx).Model(cost).Create())

	got, next, err := repo.GetFederationCosts(ctx, now, now, 50, "")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationCosts must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
	require.Equal(t, "", next)
}

// 5) GetInstanceHealthReport — keyed PK (FEDERATION#domain#month) + SK range.
func TestScanFreeWave_Federation_GetInstanceHealthReport(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationCostActivity{})

	now := time.Now()
	for i, ts := range []time.Time{now, now.Add(-24 * time.Hour)} {
		act := &models.FederationCostActivity{ID: fmt.Sprintf("act-%d", i), Domain: "remote.example", Timestamp: ts, Success: false, ResponseTime: 300}
		require.NoError(t, act.UpdateKeys())
		require.NoError(t, repo.db.WithContext(ctx).Model(act).Create())
	}

	report, err := repo.GetInstanceHealthReport(ctx, "remote.example", 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetInstanceHealthReport must not scan")
	require.Equal(t, "remote.example", report.Domain)
}

// 6) GetCostProjections — keyed PK (FEDERATION_COSTS#current month).
func TestScanFreeWave_Federation_GetCostProjections(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationCost{})

	now := time.Now()
	cost := &models.FederationCost{Domain: "remote.example", EstimatedCostUSD: 42.0}
	cost.PK = fmt.Sprintf("FEDERATION_COSTS#%s", now.Format(common.MonthFormat))
	cost.SK = fmt.Sprintf("DOMAIN#%s", cost.Domain)
	require.NoError(t, repo.db.WithContext(ctx).Model(cost).Create())

	proj, err := repo.GetCostProjections(ctx, "monthly")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetCostProjections must not scan")
	require.Equal(t, 42.0, proj.CurrentCost)
	require.Len(t, proj.TopDrivers, 1)
}

// 7) GetFederationNodes — keyed gsi1 (FEDERATION_ACTIVE).
func TestScanFreeWave_Federation_GetFederationNodes(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationNode{})

	node := &models.FederationNode{Domain: "remote.example", Software: "mastodon", LastSeen: time.Now()}
	node.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(node).Create())

	got, err := repo.GetFederationNodes(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationNodes must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
}

// 8) GetFederationNodesByHealth — keyed gsi1 (FEDERATION_ACTIVE) + in-memory health filter.
func TestScanFreeWave_Federation_GetFederationNodesByHealth(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationNode{})

	for _, h := range []string{"healthy", "unhealthy"} {
		node := &models.FederationNode{Domain: h + ".example", Health: h, LastSeen: time.Now()}
		node.UpdateKeys()
		require.NoError(t, repo.db.WithContext(ctx).Model(node).Create())
	}

	got, err := repo.GetFederationNodesByHealth(ctx, "healthy", 10)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationNodesByHealth must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "healthy.example", got[0].Domain)
}

// 9) CalculateFederationClusters — keyed PK (FEDERATION_CLUSTER#CLUSTERS);
// three stored clusters short-circuit the compute path.
func TestScanFreeWave_Federation_CalculateFederationClusters(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.InstanceCluster{})

	for i := 0; i < 3; i++ {
		cl := &models.InstanceCluster{ClusterID: fmt.Sprintf("c%d", i), Instances: []string{fmt.Sprintf("d%d.example", i)}}
		cl.UpdateKeys()
		require.NoError(t, repo.db.WithContext(ctx).Model(cl).Create())
	}

	got, err := repo.CalculateFederationClusters(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "CalculateFederationClusters must not scan")
	require.Len(t, got, 3)
}

// 10) GetInstanceConnections — keyed gsi2 (typed branch matches the writer
// shape; untyped branch queries a legacy never-written gsi2 partition and
// stays empty — documented in the inventory doc).
func TestScanFreeWave_Federation_GetInstanceConnections(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.InstanceConnection{})

	conn := &models.InstanceConnection{Domain: "remote.example", TargetDomain: "other.example", ConnectionType: "follows", LastActivity: time.Now()}
	conn.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(conn).Create())

	typed, err := repo.GetInstanceConnections(ctx, "remote.example", "follows")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetInstanceConnections must not scan")
	require.Len(t, typed, 1)
	require.Equal(t, "other.example", typed[0].TargetDomain)

	untyped, err := repo.GetInstanceConnections(ctx, "remote.example", "")
	require.NoError(t, err)
	require.Len(t, untyped, 0, "untyped branch queries a legacy never-written gsi2 partition")
}

// 11) GetAffectedRelationships — keyed PK (follow#userID) + SK filter.
func TestScanFreeWave_Federation_GetAffectedRelationships(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.Follow{})

	follow := &models.Follow{
		PK:               "follow#alice",
		SK:               "following#bob@remote.example",
		FollowerUsername: "alice",
		FollowedUsername: "bob@remote.example",
	}
	require.NoError(t, repo.db.WithContext(ctx).Model(follow).Create())

	got, err := repo.GetAffectedRelationships(ctx, "alice", "remote.example")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetAffectedRelationships must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "follow#alice", got[0].PK)
}

// 12) GetRecentInstanceConnections — keyed gsi2. The production writer
// maintains gsi2PK with a #type suffix; this query's legacy gsi2 partition
// (no suffix) is seeded directly to prove the read is keyed and scan-free.
func TestScanFreeWave_Federation_GetRecentInstanceConnections(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.InstanceConnection{})

	now := time.Now()
	conn := &models.InstanceConnection{Domain: "remote.example", TargetDomain: "other.example", LastActivity: now}
	conn.GSI2PK = fmt.Sprintf("INSTANCE#%s#CONNECTIONS", conn.Domain)
	conn.GSI2SK = fmt.Sprintf("%d#%s", now.Unix(), conn.TargetDomain)
	require.NoError(t, repo.db.WithContext(ctx).Model(conn).Create())

	got, err := repo.GetRecentInstanceConnections(ctx, "remote.example", time.Hour)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetRecentInstanceConnections must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "other.example", got[0].TargetDomain)
}

// 13) ListFailedDeliveries — keyed gsi1 (FAILED_DELIVERIES) with retry-time sort key.
func TestScanFreeWave_Federation_ListFailedDeliveries(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.DeliveryStatus{})

	now := time.Now()
	delivery := &models.DeliveryStatus{
		ActivityID:   "act-1",
		TargetDomain: "remote.example",
		Status:       StatusFailed,
		Attempts:     1,
		CreatedAt:    now.Add(-time.Hour),
		NextRetry:    now.Add(-30 * time.Minute), // past retry so gsi1SK <= now
	}
	delivery.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(delivery).Create())

	got, err := repo.ListFailedDeliveries(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "ListFailedDeliveries must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "act-1", got[0].ActivityID)
}

// 14) GetInboxItems — keyed gsi1 (INBOX#actor).
func TestScanFreeWave_Federation_GetInboxItems(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.InboxItem{})

	item := &models.InboxItem{
		ActorID:    "https://remote.example/users/alice",
		ActivityID: "act-1",
		Activity:   &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://remote.example/act-1", Type: "Create"}},
		Timestamp:  time.Now(),
	}
	item.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(item).Create())

	got, next, err := repo.GetInboxItems(ctx, item.ActorID, 10, "")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetInboxItems must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "https://remote.example/act-1", got[0].ID)
	require.Equal(t, "", next)
}

// 15) GetPublicOutbox — keyed gsi1 (PUBLIC_OUTBOX#actor).
func TestScanFreeWave_Federation_GetPublicOutbox(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.OutboxItem{})

	item := &models.OutboxItem{
		ActorID:    "https://remote.example/users/alice",
		ActivityID: "act-1",
		Activity:   &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://remote.example/act-1", Type: "Create"}},
		Timestamp:  time.Now(),
		Public:     true,
	}
	item.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(item).Create())

	got, next, err := repo.GetPublicOutbox(ctx, item.ActorID, 10, "")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetPublicOutbox must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "", next)
}

// 16) GetOutboxItems — keyed PK (ACTOR#actor) + SK filter.
func TestScanFreeWave_Federation_GetOutboxItems(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.OutboxItem{})

	item := &models.OutboxItem{
		ActorID:    "https://remote.example/users/alice",
		ActivityID: "act-1",
		Activity:   &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://remote.example/act-1", Type: "Create"}},
		Timestamp:  time.Now(),
	}
	item.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(item).Create())

	got, next, err := repo.GetOutboxItems(ctx, item.ActorID, 10, "")
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetOutboxItems must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "https://remote.example/act-1", got[0].ID)
	require.Equal(t, "", next)
}

// 17) GetDetailedFederationMetrics — keyed PK (FEDERATION_TIMESERIES#domain#period).
func TestScanFreeWave_Federation_GetDetailedFederationMetrics(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationAnalyticsTimeSeries{})

	now := time.Now()
	m := &models.FederationAnalyticsTimeSeries{Domain: "remote.example", Period: "5min", Timestamp: now}
	m.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(m).Create())

	got, err := repo.GetDetailedFederationMetrics(ctx, "remote.example", "5min", now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetDetailedFederationMetrics must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
}

// 18) GetDetailedMetricsByPeriod — keyed gsi2 (PERIOD#period).
func TestScanFreeWave_Federation_GetDetailedMetricsByPeriod(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationAnalyticsTimeSeries{})

	now := time.Now()
	m := &models.FederationAnalyticsTimeSeries{Domain: "remote.example", Period: "5min", Timestamp: now}
	m.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(m).Create())

	got, err := repo.GetDetailedMetricsByPeriod(ctx, "5min", now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetDetailedMetricsByPeriod must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
}

// 19) GetFederationCostsByUser — keyed PK (USER_FEDERATION_COSTS#userID).
// The FederationCost writer maintains FEDERATION_COSTS#<month> PKs only, so
// production rows are not found by this listing (legacy data-model gap,
// preserved behavior — documented in the inventory doc). The keyed read path
// itself is proven here by seeding the queried shape directly.
func TestScanFreeWave_Federation_GetFederationCostsByUser(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationCost{})

	now := time.Now()
	cost := &models.FederationCost{Domain: "remote.example", EstimatedCostUSD: 5.0}
	cost.PK = fmt.Sprintf("USER_FEDERATION_COSTS#%s", "alice")
	cost.SK = now.Format(time.RFC3339)
	require.NoError(t, repo.db.WithContext(ctx).Model(cost).Create())

	got, err := repo.GetFederationCostsByUser(ctx, "alice", now.Add(-30*24*time.Hour), now, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationCostsByUser must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
}

// 20) GetAllFederationEdges — keyed gsi3 global listing (FED_EDGES#ALL),
// the additive key shape maintained by FederationEdge.UpdateKeys.
func TestScanFreeWave_Federation_GetAllFederationEdges(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationEdge{})

	for i, pair := range [][2]string{{"a.example", "b.example"}, {"b.example", "a.example"}} {
		edge := &models.FederationEdge{
			SourceDomain:   pair[0],
			TargetDomain:   pair[1],
			ConnectionType: "follows",
			Strength:       0.8,
			LastActivity:   time.Now(),
			SharedUsers:    int64(i + 1),
		}
		edge.UpdateKeys()
		require.NoError(t, repo.db.WithContext(ctx).Model(edge).Create())
	}

	got, err := repo.GetAllFederationEdges(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetAllFederationEdges must not scan")
	require.Len(t, got, 2)
	require.Equal(t, "a.example", got[0].SourceDomain)
	require.Equal(t, "b.example", got[1].SourceDomain)
}

// 21) GetFederationClusters — keyed PK (FEDERATION_CLUSTER#CLUSTERS).
func TestScanFreeWave_Federation_GetFederationClusters(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.InstanceCluster{})

	cl := &models.InstanceCluster{ClusterID: "c1", Instances: []string{"d1.example"}}
	cl.UpdateKeys()
	require.NoError(t, repo.db.WithContext(ctx).Model(cl).Create())

	got, err := repo.GetFederationClusters(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationClusters must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "c1", got[0].ClusterID)
}

// 22) GetFederationActivitiesByTimeRange — day-bucketed keyed gsi1 pages
// (FEDERATION_DAILY#date) via fetchActivityPage.
func TestScanFreeWave_Federation_GetFederationActivitiesByTimeRange(t *testing.T) {
	ctx := context.Background()
	repo, s := newScanFreeFederationRepo(t, &models.FederationCostActivity{})

	now := time.Now()
	act := &models.FederationCostActivity{ID: "act-1", Domain: "remote.example", Type: "ingress", Timestamp: now, Success: true}
	require.NoError(t, act.UpdateKeys())
	require.NoError(t, repo.db.WithContext(ctx).Model(act).Create())

	got, err := repo.GetFederationActivitiesByTimeRange(ctx, now.Add(-time.Hour), now.Add(time.Hour), 100)
	require.NoError(t, err)
	require.Equal(t, 0, s.scanCalls, "GetFederationActivitiesByTimeRange must not scan")
	require.Len(t, got, 1)
	require.Equal(t, "act-1", got[0].ID)
}
