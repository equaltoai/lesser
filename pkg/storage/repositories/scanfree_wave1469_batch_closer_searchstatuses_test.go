package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch closer (umbrella #1469, issue #1494, 2026-08-28) — the last baselined
// request-path scan, StatusRepository.SearchStatuses, rewritten as a bounded
// recency-window walk over the time-ordered GSI2 PUBLIC_TIMELINE partition.
//
// The walk mirrors the wave's walkKeyedPages class: Limit(500)/page via
// AllPaginated, 100-page cap, fail-closed errBoundedPageCapExceeded on
// exhaustion, with the per-page Content CONTAINS and Deleted filters on the
// chain. The consume callback stops as soon as the caller's (clamped) limit of
// matches is reached, preserving the pre-existing one-page contract
// (NextCursor "" hardcoded).
//
// Every assertion pins a LITERAL so a mutation dies: the gsi2 chain
// (Index("gsi2") / Where("gsi2PK","=","PUBLIC_TIMELINE") / OrderBy("gsi2SK",
// "DESC")), both per-page filters, Limit(500), the exact 100-page cap count
// with cursor handoff, the fail-closed sentinel, the stop-at-limit condition,
// and the floor for a zero/omitted limit.

// searchStatusesChain registers the keyed GSI2 walk chain expectations shared
// by every SearchStatuses test. It returns the mock query so callers can layer
// the per-test Limit/AllPaginated/Cursor expectations on top.
func searchStatusesChain(t *testing.T, mockQuery *dynamormmocks.MockQuery, query string) {
	t.Helper()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "PUBLIC_TIMELINE").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Filter", "Content", "CONTAINS", query).Return(mockQuery).Once()
	mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery).Once()
}

// A query matching nothing in the newest public timeline: every page comes back
// empty (the per-page CONTAINS filter yields no matches) with HasMore=true, so
// the walk exhausts the 100-page cap and MUST fail closed with
// errBoundedPageCapExceeded — a mutation that returns a truncated/empty result
// as success dies on require.ErrorIs, a mutation that drops the Limit(500) page
// size or raises/lowers the cap count dies on the strict mock expectations.
func TestBatchCloser_SearchStatuses_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "nothing-matches")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.SearchStatuses(ctx, "nothing-matches", interfaces.PaginationOptions{Limit: 20})
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Cap exhaustion on the very first page must propagate out of SearchStatuses as
// a real error (the sentinel survives ErrorHandler.HandleQueryError) — a
// mutation that swallows the walk error into an empty success result dies here.
func TestBatchCloser_SearchStatuses_CapExhaustionFirstPageFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 20})
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// The per-page Content CONTAINS and Deleted filters stay on the query chain
// (pinned by the strict Filter expectations — a mutation that moves filtering
// into the consume callback dies on AssertExpectations), and matching rows flow
// from the page into the result. limit=2 with a 2-match first page must stop
// the walk after that single page (a mutation that drops the stop-at-limit
// condition would keep paging and die on the Once() AllPaginated), returning
// the one-page contract: 2 items, NextCursor "", HasMore = page filled.
func TestBatchCloser_SearchStatuses_PerPageFilterRoutingStopsAtLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Status)
		*out = []models.Status{{StatusID: "s1", Content: "hello world"}, {StatusID: "s2", Content: "hello again"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	res, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, "s1", res.Items[0].StatusID)
	require.Equal(t, "s2", res.Items[1].StatusID)
	require.Equal(t, "", res.NextCursor)
	require.True(t, res.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Matches spanning multiple pages must accumulate (append, not overwrite) and
// the walk must stop exactly at the caller's limit: page 1 yields 3 matches,
// page 2 yields 2 more, limit=5 — a `statuses = page` overwrite mutation
// returns 2 and dies on require.Len, and removing the stop condition pages
// again and dies on the Once() AllPaginated for page 2.
func TestBatchCloser_SearchStatuses_MultiPageAccumulationStopsAtLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Status)
		*out = []models.Status{{StatusID: "s1"}, {StatusID: "s2"}, {StatusID: "s3"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Status)
		*out = []models.Status{{StatusID: "s4"}, {StatusID: "s5"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	res, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 5})
	require.NoError(t, err)
	require.Len(t, res.Items, 5)
	require.Equal(t, "s5", res.Items[4].StatusID)
	require.Equal(t, "", res.NextCursor)
	require.True(t, res.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// A walk that ends at the partition boundary with fewer than the requested
// limit of matches returns the partial page as success with HasMore=false —
// the one-page contract and the current HasMore computation (page not filled)
// are preserved.
func TestBatchCloser_SearchStatuses_WalkExhaustsPartialPage(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Status)
		*out = []models.Status{{StatusID: "s1"}, {StatusID: "s2"}, {StatusID: "s3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	res, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Items, 3)
	require.Equal(t, "", res.NextCursor)
	require.False(t, res.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// A zero/omitted limit floors to the default page size (wave #1469 floor/clamp
// class): the walk keeps paging past an empty first page and reports
// HasMore=false at the partition end. A mutation that removes the floor leaves
// limit=0, the consume stop condition (len >= 0) is then true after the first
// page, and the walk stops there claiming HasMore=true over an empty result —
// this test dies on require.False.
func TestBatchCloser_SearchStatuses_ZeroLimitFloorsAndKeepsWalking(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// Page 1: no matches yet (sparse), more data remains; page 2: partition end.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()
	mockQuery.On("Cursor", "c").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	res, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 0})
	require.NoError(t, err)
	require.Empty(t, res.Items)
	require.Equal(t, "", res.NextCursor)
	require.False(t, res.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// One page holding more matches than the caller's limit must be trimmed to
// exactly limit: page 1 yields 5 matches with limit=2, so the walk stops after
// that single page but the result is cut to the 2 newest matches. A mutation
// that drops the trim returns 5 items and dies on require.Len; the restored
// HasMore semantics (len(result) == limit → true, i.e. the walk collected >=
// limit matches so more may exist beyond the page) are pinned by require.True.
func TestBatchCloser_SearchStatuses_OvershootTrimmedToLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	searchStatusesChain(t, mockQuery, "hello")
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Status)
		*out = []models.Status{
			{StatusID: "s1", Content: "hello 1"},
			{StatusID: "s2", Content: "hello 2"},
			{StatusID: "s3", Content: "hello 3"},
			{StatusID: "s4", Content: "hello 4"},
			{StatusID: "s5", Content: "hello 5"},
		}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	res, err := repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, "s1", res.Items[0].StatusID)
	require.Equal(t, "s2", res.Items[1].StatusID)
	require.Equal(t, "", res.NextCursor)
	require.True(t, res.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
