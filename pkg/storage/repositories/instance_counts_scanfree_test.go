package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// scanForbiddingDB wraps fakedb.Fake with a Scan method that fails the test:
// any request-adjacent read that issues a DynamoDB Scan (the regression this
// delegation removes) surfaces as a test failure instead of silently passing.
// Reads on unseeded tables must be point reads only.
type scanForbiddingDB struct {
	*fakedb.Fake
	scanCalls int
}

func (s *scanForbiddingDB) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	return nil, errors.New("full-table scan forbidden on request path")
}

// newScanForbiddingTestDB builds a tabletheory DB whose DynamoDBAPI seam fails
// on any Scan call, proving the reads below never scan.
func newScanForbiddingTestDB(t *testing.T, modelTypes ...any) (core.DB, *scanForbiddingDB) {
	t.Helper()
	s := &scanForbiddingDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, s)
	require.NoError(t, err)
	for _, m := range modelTypes {
		require.NoError(t, db.CreateTable(m))
	}
	return db, s
}

// TestInstanceCounts_ReadsOnUnseededTable_NeverScan pins the release-blocking
// invariant (#1476): a stats read on an unseeded table responds with the
// documented default (0) without issuing a single full-table scan. Any
// regression that re-introduces a request-adjacent All()/Scan (marker-gated or
// not) fails this test.
func TestInstanceCounts_ReadsOnUnseededTable_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t,
		&models.User{}, &models.Actor{}, &models.Activity{},
		&models.ActivityDayCounter{}, &models.InstanceMetrics{},
	)

	// Federated-scale rows that must never be counted by scanning.
	seedUsers(t, ctx, db, 500)
	seedActors(t, ctx, db, 500, []string{"a.example.com", "b.example.com"})
	seedActivities(t, ctx, db, 500, 30)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)

	users, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Zero(t, users)

	domains, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Zero(t, domains)

	active, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Zero(t, active)

	require.Zero(t, s.scanCalls, "no scan may run on a request-adjacent read")
}

// TestInstanceCounts_ReadsOnSeededTable_NeverScan pins that reads serving
// maintained counters (the write-path / offline-recount state) are point reads
// that never scan.
func TestInstanceCounts_ReadsOnSeededTable_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t,
		&models.User{}, &models.Actor{}, &models.Activity{},
		&models.ActivityDayCounter{}, &models.InstanceMetrics{},
	)

	seedTotalUsersMetric(t, ctx, db, 42)
	seedTotalDomainsMetric(t, ctx, db, 7)
	seedActiveMonthCounters(t, ctx, db, 9)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)

	users, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 42, users)

	domains, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, domains)

	active, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 9, active)

	require.Zero(t, s.scanCalls, "no scan may run on a request-adjacent read")
}
