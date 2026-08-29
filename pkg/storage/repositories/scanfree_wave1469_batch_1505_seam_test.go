package repositories

// M3 (rework of attack finding on #1505) — the batch-1505 compile pins in
// scanfree_wave1469_batch_1505_compile_test.go compiled MIRROR chains retyped
// in the test file, so they could not catch a production-side regression.
//
// These tests drive the REAL production repository functions through a
// capturing core.DB seam: the seam wraps the production `core.DB`, and every
// terminal read (All/AllPaginated/First/Count/Scan) compiles the query the
// production code actually built and records the CompiledQuery at the
// core.DB/core.Query boundary. The pins then assert the DynamoDB contract on
// the REAL chain: Operation == Query, exactly one sort-key condition in the
// KeyConditionExpression, no `>=`+`<=` pair, the expected operator, and
// FilterExpression demotion where applicable.
//
// Sites the seam cannot drive keep their mirror pins in
// scanfree_wave1469_batch_1505_compile_test.go and are listed by name in that
// file's header (see the "Seam-unsupported sites" section): QueryUtils
// TimeRangeQuery (slice-of-maps model) and QueryCache PrefixQuery (no
// production function builds the chain).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	ttquery "github.com/theory-cloud/tabletheory/v3/pkg/query"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// ============================================================================
// Seam infrastructure — capture the compiled query at the core.DB/core.Query
// boundary without executing against DynamoDB.
// ============================================================================

type queryCapture struct {
	queries []*core.CompiledQuery
}

// captureDB wraps a real tabletheory DB; Model() returns a captureQuery that
// compiles and records the query the production chain built when a terminal
// read is invoked. All clones created by WithContext share one capture.
type captureDB struct {
	core.DB
	capture *queryCapture
}

func newCaptureDB(inner core.DB) *captureDB {
	return &captureDB{DB: inner, capture: &queryCapture{}}
}

func (d *captureDB) WithContext(ctx context.Context) core.DB {
	return &captureDB{DB: d.DB.WithContext(ctx), capture: d.capture}
}

func (d *captureDB) Model(model any) core.Query {
	return &captureQuery{Query: d.DB.Model(model), capture: d.capture}
}

// captureQuery wraps the real *ttquery.Query; terminal reads compile and
// record the query, then return an empty result so production control flow
// (has-more detection, next-cursor derivation) proceeds as on an empty table.
// Every chainable method re-wraps the inner result so the capture survives the
// full production chain (Where → OrderBy → Limit → All).
type captureQuery struct {
	core.Query
	capture *queryCapture
}

func (q *captureQuery) record() {
	tq, ok := q.Query.(*ttquery.Query)
	if !ok {
		return
	}
	compiled, err := tq.Compile()
	if err != nil {
		return
	}
	q.capture.queries = append(q.capture.queries, compiled)
}

func (q *captureQuery) rewrap(next core.Query) core.Query {
	return &captureQuery{Query: next, capture: q.capture}
}

func (q *captureQuery) Where(field string, op string, value any) core.Query {
	return q.rewrap(q.Query.Where(field, op, value))
}

func (q *captureQuery) Index(indexName string) core.Query {
	return q.rewrap(q.Query.Index(indexName))
}

func (q *captureQuery) Filter(field string, op string, value any) core.Query {
	return q.rewrap(q.Query.Filter(field, op, value))
}

func (q *captureQuery) OrFilter(field string, op string, value any) core.Query {
	return q.rewrap(q.Query.OrFilter(field, op, value))
}

func (q *captureQuery) FilterGroup(fn func(core.Query)) core.Query {
	return q.rewrap(q.Query.FilterGroup(fn))
}

func (q *captureQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	return q.rewrap(q.Query.OrFilterGroup(fn))
}

func (q *captureQuery) IfNotExists() core.Query {
	return q.rewrap(q.Query.IfNotExists())
}

func (q *captureQuery) IfExists() core.Query {
	return q.rewrap(q.Query.IfExists())
}

func (q *captureQuery) WithCondition(field, operator string, value any) core.Query {
	return q.rewrap(q.Query.WithCondition(field, operator, value))
}

func (q *captureQuery) WithConditionExpression(expr string, values map[string]any) core.Query {
	return q.rewrap(q.Query.WithConditionExpression(expr, values))
}

func (q *captureQuery) OrderBy(field string, order string) core.Query {
	return q.rewrap(q.Query.OrderBy(field, order))
}

func (q *captureQuery) Limit(limit int) core.Query {
	return q.rewrap(q.Query.Limit(limit))
}

func (q *captureQuery) Offset(offset int) core.Query {
	return q.rewrap(q.Query.Offset(offset))
}

func (q *captureQuery) Select(fields ...string) core.Query {
	return q.rewrap(q.Query.Select(fields...))
}

func (q *captureQuery) ConsistentRead() core.Query {
	return q.rewrap(q.Query.ConsistentRead())
}

func (q *captureQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	return q.rewrap(q.Query.WithRetry(maxRetries, initialDelay))
}

func (q *captureQuery) Cursor(cursor string) core.Query {
	return q.rewrap(q.Query.Cursor(cursor))
}

func (q *captureQuery) SetCursor(cursor string) error {
	return q.Query.SetCursor(cursor)
}

func (q *captureQuery) WithContext(ctx context.Context) core.Query {
	return q.rewrap(q.Query.WithContext(ctx))
}

func (q *captureQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	return q.rewrap(q.Query.ParallelScan(segment, totalSegments))
}

func (q *captureQuery) All(dest any) error {
	q.record()
	return nil
}

func (q *captureQuery) First(dest any) error {
	q.record()
	return nil
}

func (q *captureQuery) Count() (int64, error) {
	q.record()
	return 0, nil
}

func (q *captureQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	q.record()
	return &core.PaginatedResult{HasMore: false}, nil
}

func (q *captureQuery) Scan(dest any) error {
	q.record()
	return nil
}

// newSeamDB builds a capture-wrapped real tabletheory DB over an empty fakedb.
func newSeamDB(t *testing.T) *captureDB {
	t.Helper()
	inner, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	return newCaptureDB(inner)
}

// ============================================================================
// Seam assertion helpers. Reuse the contract helpers from the mirror compile
// file (countKCEAttr / assertSingleKeyCondition) — they parse the compiled
// KeyConditionExpression exactly the same way.
// ============================================================================

// assertSeamContract asserts EVERY captured query satisfies the single-key-
// condition contract on the given sort-key attribute.
func assertSeamContract(t *testing.T, cap *queryCapture, skAttr string) {
	t.Helper()
	require.NotEmpty(t, cap.queries, "seam captured no compiled queries")
	for i, compiled := range cap.queries {
		require.Equal(t, "Query", compiled.Operation, "query %d: production chain must compile to a Query, not a Scan", i)
		require.Equal(t, 1, countKCEAttr(compiled, skAttr),
			"query %d: KCE %q must carry exactly one %s condition", i, compiled.KeyConditionExpression, skAttr)
		require.NotContains(t, compiled.KeyConditionExpression, " >= ", "query %d: no >= + <= pair", i)
		require.NotContains(t, compiled.KeyConditionExpression, " <= ", "query %d: no >= + <= pair", i)
	}
}

// assertSeamLast asserts the LAST captured query matches the expected full
// shape (operator, index, bound values, filter demotion).
func assertSeamLast(t *testing.T, cap *queryCapture, skAttr, index, op string, values []string, filter string) {
	t.Helper()
	require.NotEmpty(t, cap.queries)
	assertSingleKeyCondition(t, cap.queries[len(cap.queries)-1], skAttr, index, op, values, filter)
}

// assertSeamLastLE is the allowLE variant of assertSeamLast — a single `<=`
// upper bound (e.g. the draft due-cutoff first page) is a valid one-condition
// KCE; only the `>=`+`<=` PAIR is the DynamoDB-rejected shape.
func assertSeamLastLE(t *testing.T, cap *queryCapture, skAttr, index, op string, values []string, filter string, allowLE bool) {
	t.Helper()
	require.NotEmpty(t, cap.queries)
	assertSingleKeyConditionLE(t, cap.queries[len(cap.queries)-1], skAttr, index, op, values, filter, allowLE)
}

func newSeamAccountRepo(t *testing.T) (*AccountRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewAccountRepository(db, "test-table", "example.com", zap.NewNop()), db
}

func newSeamAuditRepo(t *testing.T) (*AuditRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewAuditRepository(db, "test-table", zap.NewNop(), nil), db
}

// ============================================================================
// M1 — GetLoginHistory: the begins_with bound must survive on every cursor
// path (decode failure, empty SK, out-of-block cursor), never degrading to a
// PK-only walk of the USER# partition.
// ============================================================================

func TestBatch1505Seam_GetLoginHistory_RealChain(t *testing.T) {
	ctx := context.Background()
	enc := Utils.Pagination.EncodeCursor

	t.Run("first page keys begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"LOGIN#"}, "")
	})

	t.Run("valid cursor page closes the range at the block bottom", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER#alice", "LOGIN#123")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"LOGIN#", "LOGIN#123"}, "begins_with")
	})

	t.Run("decode-failure cursor falls back to a keyed begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: "!!!not-a-cursor!!!"})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"LOGIN#"}, "")
	})

	t.Run("empty-SK cursor falls back to a keyed begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER#alice", "")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"LOGIN#"}, "")
	})

	t.Run("out-of-block cursor falls back to a keyed begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER#alice", "OTHER#1")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"LOGIN#"}, "")
	})
}

// ============================================================================
// M2 — SearchAccounts: BEGINS_WITH must be re-applied on every cursor path
// (decode failure, empty SK, prefix mismatch), never a PK-only unfiltered read
// of the 2-char handle-prefix partition; the valid cursor page closes the
// range at the block top with the `~` sentinel.
// ============================================================================

func TestBatch1505Seam_SearchAccounts_RealChain(t *testing.T) {
	ctx := context.Background()
	enc := Utils.Pagination.EncodeCursor

	t.Run("first page keys begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})

	t.Run("valid cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER_HANDLE_PREFIX#al", "alice#1")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "BETWEEN", []string{"alice#1", "alice~"}, "begins_with")
	})

	t.Run("decode-failure cursor restarts keyed on begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: "!!!not-a-cursor!!!"})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})

	t.Run("empty-SK cursor restarts keyed on begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER_HANDLE_PREFIX#al", "")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})

	t.Run("prefix-mismatch cursor (query changed between pages) restarts keyed on begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		// Cursor under a different 2-char prefix: normal client behavior when
		// the query text changes between pages.
		_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: enc("USER_HANDLE_PREFIX#zz", "alice#1")})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})
}

func TestBatch1505Seam_QueryShortHandlePrefixPartition_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.queryShortHandlePrefixPartition(ctx, "USER_HANDLE_PREFIX#al", "alice", 20, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})

	t.Run("cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, err := repo.queryShortHandlePrefixPartition(ctx, "USER_HANDLE_PREFIX#al", "alice", 20, "alice#1")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi5SK", "gsi5", "BETWEEN", []string{"alice#1", "alice~"}, "begins_with")
	})
}

// ============================================================================
// L1 — Shape B window + cursor clamps: a stale/foreign cursor outside the
// window must not widen the BETWEEN range.
// ============================================================================

func TestBatch1505Seam_GetSecurityEvents_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(100, 0)
	end := time.Unix(200, 0)

	t.Run("first page window is one BETWEEN", func(t *testing.T) {
		repo, db := newSeamAuditRepo(t)
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", start, end, 20, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi4SK", "gsi4", "BETWEEN", []string{"AUDIT#100", "AUDIT#200"}, "")
	})

	t.Run("cursor inside the window clamps the lower bound to the cursor", func(t *testing.T) {
		repo, db := newSeamAuditRepo(t)
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", start, end, 20, "AUDIT#150")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi4SK", "gsi4", "BETWEEN", []string{"AUDIT#150", "AUDIT#200"}, "")
	})

	t.Run("stale cursor below the window start is clamped to the window start", func(t *testing.T) {
		repo, db := newSeamAuditRepo(t)
		// "AUDIT#0" sorts lexically below the window start "AUDIT#100"
		// ("AUDIT#50" would sort ABOVE it: '1' < '5' at the digit boundary).
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", start, end, 20, "AUDIT#0")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi4SK", "gsi4", "BETWEEN", []string{"AUDIT#100", "AUDIT#200"}, "")
	})

	t.Run("no-window cursor page keys the bare bound", func(t *testing.T) {
		repo, db := newSeamAuditRepo(t)
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", time.Time{}, time.Time{}, 20, "AUDIT#150")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi4SK", "gsi4", ">", []string{"AUDIT#150"}, "")
	})
}

func TestBatch1505Seam_QueryBetweenPaginated_RealChain(t *testing.T) {
	ctx := context.Background()
	base := NewBaseRepository[*models.Draft](newSeamDB(t), "test-table", zap.NewNop())
	// Window bounds: startSK sorts BELOW endSK lexically.
	const startSK, endSK = "SK#a", "SK#z"

	t.Run("first page window is one BETWEEN", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryBetweenPaginated(ctx, "PK#1", startSK, endSK, BasePaginationOptions{Limit: 10})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{startSK, endSK}, "")
	})

	t.Run("desc cursor page clamps the upper bound to the cursor", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryBetweenPaginated(ctx, "PK#1", startSK, endSK, BasePaginationOptions{Limit: 10, Cursor: "SK#m", Order: SortOrderDesc})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{startSK, "SK#m"}, "")
	})

	t.Run("desc stale cursor above the window end is clamped to the window end", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryBetweenPaginated(ctx, "PK#1", startSK, endSK, BasePaginationOptions{Limit: 10, Cursor: "SK#zzz", Order: SortOrderDesc})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{startSK, endSK}, "")
	})

	t.Run("asc cursor page clamps the lower bound to the cursor", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryBetweenPaginated(ctx, "PK#1", startSK, endSK, BasePaginationOptions{Limit: 10, Cursor: "SK#m", Order: SortOrderAsc})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"SK#m", endSK}, "")
	})

	t.Run("asc stale cursor below the window start is clamped to the window start", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryBetweenPaginated(ctx, "PK#1", startSK, endSK, BasePaginationOptions{Limit: 10, Cursor: "SK#0", Order: SortOrderAsc})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{startSK, endSK}, "")
	})

	_ = base
}

func TestBatch1505Seam_ListAggregatedByPeriod_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1756000000, 0)
	end := time.Unix(1756000100, 0)
	// Production formats the window bounds with time.RFC3339 in the given
	// time's location; derive the expected values the same way.
	wantStart := "window#" + start.Format(time.RFC3339)
	wantEnd := "window#" + end.Format(time.RFC3339)
	cfg := AggregatedQueryConfig{PKPrefix: "cost_agg", LogContext: "cost", ErrorPrefix: "failed to list aggregated cost tracking"}

	t.Run("first page window is one BETWEEN", func(t *testing.T) {
		db := newSeamDB(t)
		_, _, err := ListAggregatedByPeriod[*models.DynamoDBCostRecord](ctx, db, cfg, "day", "entity", start, end, 10, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
	})

	t.Run("cursor page clamps the upper bound to the cursor", func(t *testing.T) {
		db := newSeamDB(t)
		cursor := "window#" + start.Add(30*time.Second).Format(time.RFC3339) // strictly inside (start, end)
		_, _, err := ListAggregatedByPeriod[*models.DynamoDBCostRecord](ctx, db, cfg, "day", "entity", start, end, 10, cursor)
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, cursor}, "")
	})

	t.Run("stale cursor above the window end is clamped to the window end", func(t *testing.T) {
		db := newSeamDB(t)
		_, _, err := ListAggregatedByPeriod[*models.DynamoDBCostRecord](ctx, db, cfg, "day", "entity", start, end, 10, "window#zzz")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
	})
}

// ============================================================================
// L2 — Shape C prefix + cursor: the cursor page must close the key range with
// the `~` sentinel (or the block-bottom prefix for DESC sites) instead of
// leaving it open above/below the cursor, and demote BEGINS_WITH to a
// post-read FilterExpression.
// ============================================================================

func TestBatch1505Seam_ListByPKSKPrefixPaginated_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		_, _, err := listByPKSKPrefixPaginated[*models.Draft](ctx, db, &models.Draft{}, "USER#alice", "ID#", 25, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"ID#"}, "")
	})

	t.Run("cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		db := newSeamDB(t)
		_, _, err := listByPKSKPrefixPaginated[*models.Draft](ctx, db, &models.Draft{}, "USER#alice", "ID#", 25, "ID#d1")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"ID#d1", "ID#~"}, "begins_with")
	})
}

func TestBatch1505Seam_GetFollowing_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, _, err := repo.GetFollowing(ctx, "alice", 25, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"following#"}, "")
	})

	t.Run("cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		repo, db := newSeamAccountRepo(t)
		_, _, err := repo.GetFollowing(ctx, "alice", 25, "following#bob")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"following#bob", "following#~"}, "begins_with")
	})
}

func TestBatch1505Seam_QueryWalletCredentials_RealChain(t *testing.T) {
	ctx := context.Background()
	auth := NewAuthRepository(newSeamDB(t), "test-table", zap.NewNop())

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		auth := NewAuthRepository(db, "test-table", zap.NewNop())
		_, _, err := auth.queryWalletCredentials(ctx, "WALLET#w1", "CRED#", 25, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"CRED#"}, "")
	})

	t.Run("cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		db := newSeamDB(t)
		auth := NewAuthRepository(db, "test-table", zap.NewNop())
		_, _, err := auth.queryWalletCredentials(ctx, "WALLET#w1", "CRED#", 25, "CRED#c1")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"CRED#c1", "CRED#~"}, "begins_with")
	})

	_ = auth
}

func TestBatch1505Seam_QueryWithSKPrefixPaginated_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryWithSKPrefixPaginated(ctx, "PK#1", "SK#", BasePaginationOptions{Limit: 25})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"SK#"}, "")
	})

	t.Run("desc cursor page closes the range at the block bottom", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryWithSKPrefixPaginated(ctx, "PK#1", "SK#", BasePaginationOptions{Limit: 25, Cursor: "SK#zzz"})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"SK#", "SK#zzz"}, "begins_with")
	})

	t.Run("asc cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Draft](db, "test-table", zap.NewNop())
		_, err := base.QueryWithSKPrefixPaginated(ctx, "PK#1", "SK#", BasePaginationOptions{Limit: 25, Cursor: "SK#zzz", Order: SortOrderAsc})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"SK#zzz", "SK#~"}, "begins_with")
	})
}

func TestBatch1505Seam_QueryCollectionWithConversion_RealChain(t *testing.T) {
	ctx := context.Background()
	cfg := CollectionQueryConfig{
		PKKey:       "OBJ",
		SKKey:       "LIKES",
		LogName:     "likes",
		ErrorPrefix: "get likes",
	}
	converter := func(items []*models.Like) ([]*models.Like, error) { return items, nil }

	t.Run("cursor page closes the range at the block top with the sentinel", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Like](db, "test-table", zap.NewNop())
		_, _, err := QueryCollectionWithConversion(ctx, base, cfg, "obj1", 10, "LIKES#c1", converter)
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"LIKES#c1", "LIKES~"}, "begins_with")
	})

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		base := NewBaseRepository[*models.Like](db, "test-table", zap.NewNop())
		_, _, err := QueryCollectionWithConversion(ctx, base, cfg, "obj1", 10, "", converter)
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"LIKES"}, "")
	})
}

func TestBatch1505Seam_ListRevisionsPaginated_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewRevisionRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.ListRevisionsPaginated(ctx, "object-1", 25, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{"VERSION#"}, "")
	})

	t.Run("cursor page closes the range at the block bottom", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewRevisionRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.ListRevisionsPaginated(ctx, "object-1", 25, "VERSION#v10")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"VERSION#", "VERSION#v10"}, "begins_with")
	})

	t.Run("bare cursor is normalized to the block prefix", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewRevisionRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.ListRevisionsPaginated(ctx, "object-1", 25, "v10")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"VERSION#", "VERSION#v10"}, "begins_with")
	})
}

func TestBatch1505Seam_ListArticlesByCMSIndexPaginated_RealChain(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewArticleRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.listArticlesByCMSIndexPaginated(ctx, "CMS_CATEGORY#c1", 25, "")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "begins_with(", []string{models.CMSArticleIndexSKPrefix}, "")
	})

	t.Run("cursor page closes the range at the block bottom", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewArticleRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.listArticlesByCMSIndexPaginated(ctx, "CMS_CATEGORY#c1", 25, "ART#x")
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{models.CMSArticleIndexSKPrefix, "ART#x"}, "begins_with")
	})
}

// SearchHashtagsAdvancedPaginated — the documented NO-sentinel site: the gsi3SK
// block is the raw lowercased hashtag name whose writer alphabet allows Unicode
// letters/numbers (bytes > 0x7E), so no static sentinel can close the range
// without excluding valid block members. The seam still pins the REAL chain:
// first page keys BEGINS_WITH; the cursor page keys the exclusive `>` bound
// and demotes BEGINS_WITH to a post-read FilterExpression.
func TestBatch1505Seam_SearchHashtagsAdvancedPaginated_RealChain(t *testing.T) {
	ctx := context.Background()
	search := NewSearchRepository(newSeamDB(t), "test-table", zap.NewNop(), nil)

	t.Run("first page keys begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		search := NewSearchRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := search.SearchHashtagsAdvancedPaginated(ctx, "abc", &PaginationOptions{Limit: 20})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi3SK", "gsi3", "begins_with(", []string{"abc"}, "")
	})

	t.Run("cursor page keys the bare exclusive bound and filters begins_with", func(t *testing.T) {
		db := newSeamDB(t)
		search := NewSearchRepository(db, "test-table", zap.NewNop(), nil)
		cursor := EncodeCursor(&CursorData{LastID: "abc#1"})
		_, _, err := search.SearchHashtagsAdvancedPaginated(ctx, "abc", &PaginationOptions{Limit: 20, Cursor: cursor})
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi3SK", "gsi3", ">", []string{"abc#1"}, "begins_with")
	})

	_ = search
}

// ============================================================================
// P1 — GetDailySpending: the REAL chain must key the gsi1SK window with the
// writer's CompactTimeFormat bounds (issue #1506), not RFC3339.
// ============================================================================

func TestBatch1505Seam_GetDailySpending_RealChain(t *testing.T) {
	ctx := context.Background()
	db := newSeamDB(t)
	repo := NewNotificationCostRepository(db, "test-table", zap.NewNop(), nil)

	total, err := repo.GetDailySpending(ctx, "alice")
	require.NoError(t, err)
	require.Zero(t, total)
	assertSeamContract(t, db.capture, "gsi1SK")

	compiled := db.capture.queries[len(db.capture.queries)-1]
	require.Contains(t, compiled.KeyConditionExpression, "BETWEEN")
	got := compiledStringValues(compiled)
	var costBounds []string
	for _, v := range got {
		if strings.HasPrefix(v, "COST#") {
			costBounds = append(costBounds, v)
		}
	}
	require.Len(t, costBounds, 2, "BETWEEN needs two COST# bound values, got %v", got)
	for _, v := range costBounds {
		require.Equal(t, "COST#"+v[len("COST#"):], v)
		compact := v[len("COST#"):]
		require.Len(t, compact, len("20060102150405"), "bound %q must be COST# + CompactTimeFormat (issue #1506)", v)
		if _, err := time.Parse("20060102150405", compact); err != nil {
			t.Fatalf("bound %q carries a non-compact timestamp: %v", v, err)
		}
	}
}

// ============================================================================
// Shape A — `>=` + `<=` (or `>=` + `<`) collapsed to one BETWEEN key condition.
// Each pin drives the REAL production repository function through the seam and
// asserts the compiled query the production chain actually builds.
// ============================================================================

func newSeamFederationCostRepo(t *testing.T) (*FederationCostRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	enhanced := NewEnhancedBaseRepository[*models.FederationCostTracking](db, "test-table", zap.NewNop(), nil, "FederationCostRepository", "federation_cost")
	budget := NewEnhancedBaseRepository[*models.FederationBudget](db, "test-table", zap.NewNop(), nil, "FederationCostRepository.Budget", "federation_budget")
	return NewFederationCostRepository(enhanced, budget), db
}

func newSeamFederationRepo(t *testing.T) (*FederationRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewFederationRepository(db, "test-table", zap.NewNop(), nil, nil), db
}

func newSeamMetricsRepo(t *testing.T) (*MetricsRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewMetricsRepository(db, "test-table", zap.NewNop(), nil), db
}

func newSeamScheduledJobCostRepo(t *testing.T) (*ScheduledJobCostRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewScheduledJobCostRepository(db, "test-table", zap.NewNop(), nil), db
}

func newSeamTrackingRepo(t *testing.T) (*TrackingRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewTrackingRepository(db, "test-table", zap.NewNop(), nil), db
}

func newSeamAICostRepo(t *testing.T) (*AICostRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewAICostRepository(db, "test-table", zap.NewNop(), nil), db
}

func newSeamFilterRepo(t *testing.T) (*FilterRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewFilterRepository(db, "test-table", zap.NewNop(), nil), db
}

func newSeamModerationRepo(t *testing.T) (*ModerationRepository, *captureDB) {
	t.Helper()
	db := newSeamDB(t)
	return NewModerationRepository(db, "test-table", zap.NewNop()), db
}

func TestBatch1505Seam_FederationActivity_ListByDomain_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 17, 9, 19, 0, time.UTC)
	end := time.Date(2026, 8, 28, 17, 9, 19, 0, time.UTC)
	// Production formats the SK window with "20060102150405" in the given
	// time's location; derive the expected values the same way.
	wantStart := "activity#" + start.Format("20060102150405")
	wantEnd := "activity#" + end.Format("20060102150405")

	db := newSeamDB(t)
	repo := NewFederationActivityRepository(db, "test-table", zap.NewNop(), nil)
	_, err := repo.ListByDomain(ctx, "example.com", start, end, 100)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_FederationCost_DomainWindow_RealChain(t *testing.T) {
	ctx := context.Background()
	// Both bounds sit inside the single 1970-01 bucket, so exactly one keyed
	// gsi1 query is produced; production emits TS# + 13-digit UnixMilli bounds.
	start := time.UnixMilli(1)
	end := time.UnixMilli(2)

	repo, db := newSeamFederationCostRepo(t)
	_, err := repo.GetFederationCosts(ctx, "example.com", start, end, 100)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{"TS#0000000000001", "TS#0000000000002~"}, "")
}

func TestBatch1505Seam_FederationCost_ActivityTypeWindow_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 17, 9, 19, 0, time.UTC)
	end := time.Date(2026, 8, 28, 17, 9, 19, 0, time.UTC)
	// Production keys the gsi2SK window with DOMAIN# + CompactTimeFormat.
	wantStart := "DOMAIN#" + start.Format(common.CompactTimeFormat)
	wantEnd := "DOMAIN#" + end.Format(common.CompactTimeFormat)

	repo, db := newSeamFederationCostRepo(t)
	_, err := repo.GetFederationCostsByActivityType(ctx, "announce", start, end, 500)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi2SK", "gsi2", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Federation_UserCosts_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamFederationRepo(t)
	_, err := repo.GetFederationCostsByUser(ctx, "u1", start, end, 100, 0)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Federation_Statistics_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamFederationRepo(t)
	_, err := repo.GetFederationStatistics(ctx, start, end)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Federation_TimeSeriesByDomain_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamFederationRepo(t)
	_, err := repo.GetDetailedFederationMetrics(ctx, "example.com", "2026-08", start, end)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Federation_TimeSeriesByPeriod_RealChain(t *testing.T) {
	ctx := context.Background()
	// Production closes the gsi2SK range at the top of the day block with the
	// `#zzzz` sentinel (`>= start AND < end#zzzz` becomes one BETWEEN).
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339) + "#zzzz"

	repo, db := newSeamFederationRepo(t)
	_, err := repo.GetDetailedMetricsByPeriod(ctx, "2026-08", start, end, 500)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi2SK", "gsi2", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_ImportExport_UserCosts_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production keys the gsi1SK window with COST# + RFC3339.
	wantStart := "COST#" + start.Format(time.RFC3339)
	wantEnd := "COST#" + end.Format(time.RFC3339)

	db := newSeamDB(t)
	_, err := getUserCosts[*models.ExportCostTracking](ctx, db, zap.NewNop(), "u1", start, end, 100, "export", &models.ExportCostTracking{})
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Metrics_ListByType_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 17, 9, 19, 0, time.UTC)
	end := time.Date(2026, 8, 28, 17, 9, 19, 0, time.UTC)
	// Production keys the SK window with ts# + "20060102150405".
	wantStart := "ts#" + start.Format("20060102150405")
	wantEnd := "ts#" + end.Format("20060102150405")

	repo, db := newSeamMetricsRepo(t)
	_, err := repo.ListByType(ctx, "request", start, end, 20)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Metrics_ListByService_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamMetricsRepo(t)
	_, err := repo.ListByService(ctx, "api", start, end, 20)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_NotificationCost_AggregationsByPeriod_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production keys the SK window with WINDOW# + RFC3339.
	wantStart := "WINDOW#" + start.Format(time.RFC3339)
	wantEnd := "WINDOW#" + end.Format(time.RFC3339)

	db := newSeamDB(t)
	repo := NewNotificationCostRepository(db, "test-table", zap.NewNop(), nil)
	_, err := repo.ListAggregationsByPeriod(ctx, "day", "email", start, end, 500)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_ScheduledJobCost_ListByJob_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 17, 9, 19, 0, time.UTC)
	end := time.Date(2026, 8, 28, 17, 9, 19, 0, time.UTC)
	// Production keys the SK window with RUN# + "20060102150405".
	wantStart := "RUN#" + start.Format("20060102150405")
	wantEnd := "RUN#" + end.Format("20060102150405")

	repo, db := newSeamScheduledJobCostRepo(t)
	_, err := repo.ListByJob(ctx, "cleanup", "daily", start, end, 100)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_ScheduledJobCost_ListByStatus_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamScheduledJobCostRepo(t)
	_, err := repo.ListByStatus(ctx, "success", start, end, 100)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_CostTracking_GetCostProjections_RealChain(t *testing.T) {
	ctx := context.Background()
	// The REAL chain keys the COST#PROJECTION partition with the period-prefix
	// window (`day#` .. `day#~`) — NOT the retired COST_PROJECTIONS#day /
	// DAILY# keys the mirror pin retyped.

	db := newSeamDB(t)
	repo := NewTrackingRepository(db, "test-table", zap.NewNop(), nil)
	_, err := repo.GetCostProjections(ctx, "day")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"day#", "day#~"}, "")
}

func TestBatch1505Seam_CostTracking_ListByTable_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := start.Format(time.RFC3339)
	wantEnd := end.Format(time.RFC3339)

	repo, db := newSeamTrackingRepo(t)
	_, _, err := repo.ListByTable(ctx, "users", start, end, 10, "")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_CostTracking_GetRelayCostsByURL_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production keys the gsi1SK window with TS# + CompactTimeFormat and
	// filters the camelCase `operationType` attribute when an operation is
	// given (the mirror's uppercase OperationType was a retyped artifact).
	wantStart := "TS#" + start.Format(common.CompactTimeFormat)
	wantEnd := "TS#" + end.Format(common.CompactTimeFormat)

	repo, db := newSeamTrackingRepo(t)
	_, _, err := repo.GetRelayCostsByURL(ctx, "relay.example", start, end, 10, "", "deliver")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "attr:operationType")
}

func TestBatch1505Seam_CostTracking_GetRelayMetricsHistory_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production keys the gsi1SK window with daily# + CompactTimeFormat.
	wantStart := "daily#" + start.Format(common.CompactTimeFormat)
	wantEnd := "daily#" + end.Format(common.CompactTimeFormat)

	repo, db := newSeamTrackingRepo(t)
	_, _, err := repo.GetRelayMetricsHistory(ctx, "relay.example", start, end, 10, "")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_AICost_ByTimeRange_RealChain(t *testing.T) {
	ctx := context.Background()
	// Both bounds sit inside the single 1970-01 bucket, so exactly one keyed
	// gsi1 walk is produced; production emits TS# + 13-digit UnixMilli bounds.
	start := time.UnixMilli(1)
	end := time.UnixMilli(2)

	repo, db := newSeamAICostRepo(t)
	_, err := repo.GetAICostsByTimeRange(ctx, start, end, "chat", 100)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{"TS#0000000000001", "TS#0000000000002~"}, "")
}

func TestBatch1505Seam_AICost_GetAggregatedCosts_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production formats the gsi1SK window with CompactDateFormat for non-hour
	// periods.
	wantStart := start.Format(common.CompactDateFormat)
	wantEnd := end.Format(common.CompactDateFormat)

	repo, db := newSeamAICostRepo(t)
	_, err := repo.GetAggregatedCosts(ctx, "day", start, end)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_RouteOptimizer_RouteMetrics_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(0, 100)
	end := time.Unix(0, 200)
	// Production keys the SK window with RESULT# + UnixNano.
	wantStart := fmt.Sprintf("RESULT#%d", start.UnixNano())
	wantEnd := fmt.Sprintf("RESULT#%d", end.UnixNano())

	db := newSeamDB(t)
	repo := NewRouteOptimizerRepository(db, "test-table", zap.NewNop(), nil)
	_, err := repo.GetMetricsInRange(ctx, "r1", start, end, 500)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_RouteOptimizer_AllRouteMetrics_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1756000000, 0)
	end := time.Unix(1756000001, 0)
	// Production keys the gsi1SK window with the Unix second bounds.
	wantStart := fmt.Sprintf("%d", start.Unix())
	wantEnd := fmt.Sprintf("%d", end.Unix())

	db := newSeamDB(t)
	repo := NewRouteOptimizerRepository(db, "test-table", zap.NewNop(), nil)
	_, err := repo.GetMetricsInRange(ctx, "", start, end, 500)
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Instance_GetInstanceHistory_RealChain(t *testing.T) {
	ctx := context.Background()
	// Production derives the DATE# window from time.Now() with "2006-01-02" in
	// the local zone; derive the expected bounds the same way.
	now := time.Now()
	wantStart := "DATE#" + now.AddDate(0, 0, -7).Format("2006-01-02")
	wantEnd := "DATE#" + now.Format("2006-01-02")

	db := newSeamDB(t)
	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.getMetricHistory(ctx, 7, "memory", "memory_history", func(_ models.InstanceHistory) map[string]interface{} { return nil })
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Instance_SummarizeInstanceMetrics_RealChain(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	wantStart := "DATE#" + now.AddDate(0, 0, -7).Format("2006-01-02")
	wantEnd := "DATE#" + now.Format("2006-01-02")

	db := newSeamDB(t)
	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetMetricsSummary(ctx, "week")
	require.NoError(t, err)
	// The summary walks four keyed METRIC# partitions; every walk must keep the
	// single-BETWEEN window, and the last (federation_count) pins the shape.
	assertSeamContract(t, db.capture, "gsi1SK")
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Filter_GetUserFilters_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamFilterRepo(t)
	_, err := repo.GetUserFilters(ctx, "alice")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"FILTER#", "FILTER~"}, "")
}

func TestBatch1505Seam_Filter_GetFilterKeywords_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamFilterRepo(t)
	_, err := repo.GetFilterKeywords(ctx, "f1")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"KEYWORD#", "KEYWORD~"}, "")
}

func TestBatch1505Seam_Filter_GetFilterStatuses_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamFilterRepo(t)
	_, err := repo.GetFilterStatuses(ctx, "f1")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"STATUS#", "STATUS~"}, "")
}

func TestBatch1505Seam_Moderation_GetFiltersForUser_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamModerationRepo(t)
	_, err := repo.GetFiltersForUser(ctx, "alice")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"FILTER#", "FILTER~"}, "")
}

func TestBatch1505Seam_Moderation_GetFilterKeywords_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamModerationRepo(t)
	_, err := repo.GetFilterKeywords(ctx, "f1")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"KEYWORD#", "KEYWORD~"}, "")
}

func TestBatch1505Seam_Moderation_GetFilterStatuses_RealChain(t *testing.T) {
	ctx := context.Background()
	repo, db := newSeamModerationRepo(t)
	_, err := repo.GetFilterStatuses(ctx, "f1")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "SK", "", "BETWEEN", []string{"STATUS#", "STATUS~"}, "")
}

func TestBatch1505Seam_ModerationMetrics_GetFalsePositives_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	// Production keys the gsi1SK window with DATE#<day> .. DATE#<day>#Z.
	wantStart := "DATE#" + start.Format(common.DateFormat)
	wantEnd := "DATE#" + end.Format(common.DateFormat) + "#Z"

	db := newSeamDB(t)
	repo := NewModerationMetricsRepository(db, zap.NewNop())
	_, err := repo.GetFalsePositives(ctx, models.ModerationMetricsTimeRange{Start: start, End: end})
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_ModerationMetrics_GetDecisionSamples_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := "DATE#" + start.Format(common.DateFormat)
	wantEnd := "DATE#" + end.Format(common.DateFormat) + "#Z"

	db := newSeamDB(t)
	repo := NewModerationMetricsRepository(db, zap.NewNop())
	_, err := repo.GetDecisionSamples(ctx, models.ModerationMetricsTimeRange{Start: start, End: end}, "approve")
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_ModerationMetrics_GetMetricsEntries_RealChain(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	wantStart := "DATE#" + start.Format(common.DateFormat)
	wantEnd := "DATE#" + end.Format(common.DateFormat) + "#Z"

	db := newSeamDB(t)
	repo := NewModerationMetricsRepository(db, zap.NewNop())
	_, err := repo.GetMetricsEntries(ctx, models.ModerationMetricsTimeRange{Start: start, End: end}, []string{"spam"})
	require.NoError(t, err)
	assertSeamLast(t, db.capture, "gsi1SK", "gsi1", "BETWEEN", []string{wantStart, wantEnd}, "")
}

func TestBatch1505Seam_Draft_ListScheduledDraftsDuePaginated_RealChain(t *testing.T) {
	ctx := context.Background()
	dueBefore := time.Date(2026, 8, 24, 7, 6, 40, 0, time.UTC)
	// Production formats the cutoff with RFC3339Nano in UTC and closes it with
	// the `~` sentinel; derive the expected values the same way.
	cutoff := "TIME#" + dueBefore.UTC().Format(time.RFC3339Nano) + "~"
	cursor := "TIME#" + dueBefore.UTC().Add(-1*time.Second).Format(time.RFC3339Nano) + "~"

	t.Run("first page keys the bare upper bound", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.ListScheduledDraftsDuePaginated(ctx, dueBefore, 25, "")
		require.NoError(t, err)
		assertSeamLastLE(t, db.capture, "gsi4SK", "gsi4", "<=", []string{cutoff}, "", true)
	})

	t.Run("cursor page between cursor and cutoff", func(t *testing.T) {
		db := newSeamDB(t)
		repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
		_, _, err := repo.ListScheduledDraftsDuePaginated(ctx, dueBefore, 25, cursor)
		require.NoError(t, err)
		assertSeamLast(t, db.capture, "gsi4SK", "gsi4", "BETWEEN", []string{cursor, cutoff}, "")
	})
}
