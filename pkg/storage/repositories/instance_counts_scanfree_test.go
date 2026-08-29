package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
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

// scanFreeWalletIndex mirrors the reverse-index projection GetWalletByAddress
// queries (a local struct without TableName, so tabletheory resolves it to the
// pluralized default table name "IndexRecords"). Attribute names mirror the
// production projection's json tags.
type scanFreeWalletIndex struct {
	PK       string `theorydb:"pk,attr:PK"`
	SK       string `theorydb:"sk,attr:SK"`
	Type     string `json:"Type"`
	Username string `json:"Username"`
}

func (scanFreeWalletIndex) TableName() string { return "IndexRecords" }

// seedScanFreeRow updates keys explicitly (fakedb.Create does not run model
// hooks) and writes the row through the scan-forbidding DB.
func seedScanFreeRow(t *testing.T, ctx context.Context, db core.DB, row any) {
	t.Helper()
	require.NoError(t, db.WithContext(ctx).Model(row).Create())
}

// TestScanFree_ClearLoginAttempts pins that clearing login attempts is a
// keyed query: it must never issue a DynamoDB Scan.
func TestScanFree_ClearLoginAttempts(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.LoginAttempt{}, &models.RateLimitLockout{})

	baseTime := time.Now().UTC()
	for i := 0; i < 3; i++ {
		attempt := models.NewLoginAttempt("user-1", true)
		attempt.Timestamp = baseTime.Add(time.Duration(i) * time.Minute)
		attempt.SK = attempt.Timestamp.Format(time.RFC3339Nano)
		seedScanFreeRow(t, ctx, db, attempt)
	}
	lockout := models.NewRateLimitLockout("user-1", baseTime.Add(time.Minute))
	seedScanFreeRow(t, ctx, db, lockout)

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	require.NoError(t, repo.ClearLoginAttempts(ctx, "user-1"))

	require.Zero(t, s.scanCalls, "ClearLoginAttempts must not scan")
}

// TestScanFree_GetLoginAttemptCount pins that counting login attempts since a
// cutoff is a keyed query (PK equality + SK range), never a Scan.
func TestScanFree_GetLoginAttemptCount(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.LoginAttempt{})

	baseTime := time.Now().UTC()
	since := baseTime.Add(-time.Hour)
	attempts := []*models.LoginAttempt{
		models.NewLoginAttempt("user-1", true),  // recent (counted)
		models.NewLoginAttempt("user-1", false), // recent (counted)
		models.NewLoginAttempt("user-1", true),  // old (not counted)
	}
	attempts[0].Timestamp = baseTime.Add(-5 * time.Minute)
	attempts[1].Timestamp = baseTime.Add(-10 * time.Minute)
	attempts[2].Timestamp = baseTime.Add(-2 * time.Hour)
	for _, a := range attempts {
		a.SK = a.Timestamp.Format(time.RFC3339Nano)
		seedScanFreeRow(t, ctx, db, a)
	}

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	count, err := repo.GetLoginAttemptCount(ctx, "user-1", since)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.Zero(t, s.scanCalls, "GetLoginAttemptCount must not scan")
}

// TestScanFree_GetListMembers pins that listing list members is a keyed query.
func TestScanFree_GetListMembers(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.ListMember{}, &models.User{}, &models.Actor{})

	member := &models.ListMember{ListID: "list1", AccountID: "bob", ListUsername: "alice"}
	require.NoError(t, member.BeforeCreate())
	seedScanFreeRow(t, ctx, db, member)

	user := &models.User{Username: "bob", Email: "bob@example.com", DisplayName: "Bob", Role: "user"}
	require.NoError(t, user.UpdateKeys())
	seedScanFreeRow(t, ctx, db, user)

	actor := &models.Actor{Username: "bob", NumericID: "num-1"}
	require.NoError(t, actor.UpdateKeys())
	seedScanFreeRow(t, ctx, db, actor)

	repo := NewListRepository(db, "test-table", zap.NewNop(), nil)
	result, err := repo.GetListMembers(ctx, "list1", interfaces.PaginationOptions{Limit: 20})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.NotNil(t, result.Items[0].User)
	require.Equal(t, "bob", result.Items[0].User.Username)

	require.Zero(t, s.scanCalls, "GetListMembers must not scan")
}

// TestScanFree_RemoveAccountFromAllLists pins that removing an account from
// every list uses the GSI1 reverse index (keyed), never a Scan.
func TestScanFree_RemoveAccountFromAllLists(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.ListMember{})

	for _, listID := range []string{"list1", "list2"} {
		member := &models.ListMember{ListID: listID, AccountID: "bob", ListUsername: "alice"}
		require.NoError(t, member.BeforeCreate())
		seedScanFreeRow(t, ctx, db, member)
	}

	repo := NewListRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.RemoveAccountFromAllLists(ctx, "bob"))

	// Memberships are gone: a fresh member query returns nothing.
	result, err := repo.GetListMembers(ctx, "list1", interfaces.PaginationOptions{Limit: 20})
	require.NoError(t, err)
	require.Empty(t, result.Items)

	require.Zero(t, s.scanCalls, "RemoveAccountFromAllLists must not scan")
}

// TestScanFree_GetAccountPinsPaginated pins that pinned-account reads are
// keyed queries (PK equality), never Scans.
func TestScanFree_GetAccountPinsPaginated(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.AccountPin{})

	pin := &models.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
		CreatedAt:      time.Now().UTC(),
	}
	require.NoError(t, pin.UpdateKeys())
	seedScanFreeRow(t, ctx, db, pin)

	repo := NewSocialRepository(db, "test-table", zap.NewNop(), nil)
	pins, next, err := repo.GetAccountPinsPaginated(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	require.Equal(t, "bob", pins[0].PinnedUsername)
	require.Empty(t, next)

	require.Zero(t, s.scanCalls, "GetAccountPinsPaginated must not scan")
}

// TestScanFree_GetStatusByURL pins that URL lookups use the gsi7 URL index
// (keyed), never a Scan, and that the in-Go exact-match logic still resolves.
func TestScanFree_GetStatusByURL(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Status{})

	url := "https://example.com/status/1"
	status := &models.Status{
		StatusID:    "s1",
		AuthorID:    "https://example.com/users/alice",
		Visibility:  models.VisibilityPublic,
		URLs:        []string{url},
		PublishedAt: time.Now().UTC(),
	}
	require.NoError(t, status.UpdateKeys())
	seedScanFreeRow(t, ctx, db, status)

	repo := NewStatusRepository(db, "test-table", zap.NewNop(), nil)
	found, err := repo.GetStatusByURL(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "s1", found.StatusID)

	require.Zero(t, s.scanCalls, "GetStatusByURL must not scan")
}

// TestScanFree_GetThreadNodes pins that thread-node reads are keyed queries.
func TestScanFree_GetThreadNodes(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.ThreadNode{})

	for _, node := range []*models.ThreadNode{
		{RootStatusID: "root1", StatusID: "root1", Depth: 0, AuthorID: "alice"},
		{RootStatusID: "root1", StatusID: "s1", Depth: 1, AuthorID: "bob"},
	} {
		require.NoError(t, node.UpdateKeys())
		seedScanFreeRow(t, ctx, db, node)
	}

	repo := NewThreadRepository(db, zap.NewNop())
	nodes, err := repo.GetThreadNodes(ctx, "root1")
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	require.Zero(t, s.scanCalls, "GetThreadNodes must not scan")
}

// TestScanFree_GetMissingReplies pins that missing-reply reads are keyed queries.
func TestScanFree_GetMissingReplies(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.MissingReply{})

	missing := models.NewMissingReply("root1", "s1", "https://example.com/missing/1")
	require.NoError(t, missing.UpdateKeys())
	seedScanFreeRow(t, ctx, db, missing)

	repo := NewThreadRepository(db, zap.NewNop())
	missingReplies, err := repo.GetMissingReplies(ctx, "root1")
	require.NoError(t, err)
	require.Len(t, missingReplies, 1)

	require.Zero(t, s.scanCalls, "GetMissingReplies must not scan")
}

// TestScanFree_GetAccountPreferences pins that preference reads are keyed
// queries (PK equality + SK begins_with), never Scans.
func TestScanFree_GetAccountPreferences(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.UserPreference{})

	for _, pref := range []*models.UserPreference{
		{Username: "alice", Key: "language", Value: "en"},
		{Username: "alice", Key: "theme", Value: "dark"},
		{Username: "bob", Key: "language", Value: "fr"},
	} {
		pref.UpdateKeys()
		seedScanFreeRow(t, ctx, db, pref)
	}

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	prefs, err := repo.GetAccountPreferences(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, prefs, 2)
	require.Equal(t, "en", prefs["language"])
	require.Equal(t, "dark", prefs["theme"])

	require.Zero(t, s.scanCalls, "GetAccountPreferences must not scan")
}

// TestScanFree_GetPopularMediaByPeriod pins that trending-media reads are keyed
// GSI1 queries, never Scans.
func TestScanFree_GetPopularMediaByPeriod(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.MediaPopularity{})

	for _, media := range []*models.MediaPopularity{
		{MediaID: "media-1", Period: "DAY", ViewCount: 100},
		{MediaID: "media-2", Period: "DAY", ViewCount: 50},
		{MediaID: "media-3", Period: "WEEK", ViewCount: 200},
	} {
		media.SetForPeriod(media.MediaID, media.Period, media.ViewCount)
		seedScanFreeRow(t, ctx, db, media)
	}

	repo := NewMediaPopularityRepository(db, "test-table", zap.NewNop(), nil)
	records, err := repo.GetPopularMediaByPeriod(ctx, "DAY", 10, nil)
	require.NoError(t, err)
	require.Len(t, records, 2)

	require.Zero(t, s.scanCalls, "GetPopularMediaByPeriod must not scan")
}

// TestScanFree_GetExportsForUser pins that export listing uses the GSI1
// user index (keyed), never a Scan, and that the cursor round-trips.
func TestScanFree_GetExportsForUser(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Export{})

	baseTime := time.Now()
	for _, exp := range []*models.Export{
		{ID: "exp-1", Username: "alice", Type: "archive", Status: "completed", CreatedAt: baseTime.Add(-2 * time.Hour)},
		{ID: "exp-2", Username: "alice", Type: "followers", Status: "pending", CreatedAt: baseTime.Add(-time.Hour)},
		{ID: "exp-3", Username: "bob", Type: "archive", Status: "completed", CreatedAt: baseTime},
	} {
		exp.UpdateKeys()
		seedScanFreeRow(t, ctx, db, exp)
	}

	repo := NewExportRepository(db, "test-table", zap.NewNop())
	exports, next, err := repo.GetExportsForUser(ctx, "alice", 2, "")
	require.NoError(t, err)
	require.Len(t, exports, 2)
	require.NotEmpty(t, next)

	// Resume from the returned cursor: both alice exports precede the last
	// item, so the resumed page is empty.
	resumed, _, err := repo.GetExportsForUser(ctx, "alice", 2, next)
	require.NoError(t, err)
	require.Empty(t, resumed)

	require.Zero(t, s.scanCalls, "GetExportsForUser must not scan")
}

// TestScanFree_GetImportsForUser pins that import listing uses the GSI1
// user index (keyed), never a Scan.
func TestScanFree_GetImportsForUser(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Import{})

	baseTime := time.Now().UTC()
	imp := &models.Import{ID: "imp-1", Username: "alice", Type: "followers", Status: "processing", CreatedAt: baseTime}
	imp.UpdateKeys()
	seedScanFreeRow(t, ctx, db, imp)

	repo := NewImportRepository(db, "test-table", zap.NewNop())
	imports, _, err := repo.GetImportsForUser(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, imports, 1)
	require.Equal(t, "imp-1", imports[0].ID)

	require.Zero(t, s.scanCalls, "GetImportsForUser must not scan")
}

// TestScanFree_GetWalletByAddress pins that wallet-by-address lookups resolve
// through the reverse index (keyed) with no legacy fallback scan.
func TestScanFree_GetWalletByAddress(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.WalletCredential{}, &scanFreeWalletIndex{})

	wallet := &models.WalletCredential{Username: "user-1", Address: "0xAbC", ChainID: 1, Type: "ethereum"}
	require.NoError(t, wallet.UpdateKeys())
	seedScanFreeRow(t, ctx, db, wallet)

	index := &scanFreeWalletIndex{
		PK:       "WALLET#ethereum#0xabc",
		SK:       "USER#user-1",
		Type:     "ethereum",
		Username: "user-1",
	}
	seedScanFreeRow(t, ctx, db, index)

	repo := NewAuthRepository(db, "test-table", zap.NewNop())
	cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xAbC")
	require.NoError(t, err)
	require.NotNil(t, cred)
	require.Equal(t, "user-1", cred.Username)
	require.Equal(t, "0xAbC", cred.Address)

	// A legacy row without the reverse index is not found (no fallback scan).
	orphan := &models.WalletCredential{Username: "user-2", Address: "0xdef", ChainID: 1, Type: "ethereum"}
	require.NoError(t, orphan.UpdateKeys())
	seedScanFreeRow(t, ctx, db, orphan)

	missing, err := repo.GetWalletByAddress(ctx, "ethereum", "0xdef")
	require.NoError(t, err)
	require.Nil(t, missing)

	require.Zero(t, s.scanCalls, "GetWalletByAddress must not scan")
}
