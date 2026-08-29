package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch S1 (umbrella #1469, issue #1496) — the keyed-filter `.Scan` class and
// the searchAllActors zero-limit exposure.
//
// Every site converted in this batch carried key predicates on a chain whose
// terminal was `.Scan` (which tabletheory compiles to a DynamoDB Scan
// unconditionally — pkg/query/query_execution.go Query.Scan → compileScan),
// so the key predicates were only ever post-read filters. Each conversion
// switches the terminal to the wave's keyed patterns: `walkKeyedPages`
// (Limit(500)/page via AllPaginated, 100-page cap, fail-closed
// errBoundedPageCapExceeded) or a keyed `.All`.
//
// Every assertion pins a LITERAL so a mutation dies: the key-condition chain
// (partition-key equality + sort-key prefix/range), the Limit values, the
// AllPaginated/All terminals (a mutation restoring `.Scan` dies on the strict
// mock), and the fail-closed sentinel.

// QueryWithPKAndSKPrefix useFilter branch: the SK prefix must be a Where key
// condition walked in bounded pages — a mutation restoring the old
// Filter("SK","BEGINS_WITH",...) + Scan shape dies (unexpected Filter call,
// missing Where/AllPaginated expectations).
func TestBatchS1_QueryWithPKAndSKPrefix_UseFilterWalksKeyed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	type m struct{ PK, SK string }
	convert := func(in m) string { return in.PK + "#" + in.SK }

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]m)
		*dest = []m{{PK: "pk", SK: "pref#1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	out, err := QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "pref#", true, convert, "op", "param")
	require.NoError(t, err)
	require.Equal(t, []string{"pk#pref#1"}, out)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// QueryWithPKAndSKPrefix useFilter branch cap exhaustion: a partition that
// never exhausts must fail closed with errBoundedPageCapExceeded — a mutation
// returning a truncated result as success dies on require.ErrorIs.
func TestBatchS1_QueryWithPKAndSKPrefix_UseFilterCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	type m struct{ PK, SK string }

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]m)
		*dest = make([]m, 500)
	}
	mockQuery.On("AllPaginated", mock.Anything).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	_, err := QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "pref#", true, func(in m) string { return in.PK }, "op", "param")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetSeveredRelationship: the keyed chain (SEVERED#<local> + INSTANCE#<remote>#
// SK prefix) walks in bounded pages and stops at the first ID match. The first
// page carries the match AND HasMore=true, so a mutation that drops the early
// stop would try to read page 2 and die on the unregistered Cursor/AllPaginated
// call.
func TestBatchS1_GetSeveredRelationship_KeyedWalkStopsAtIDMatch(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SeveredRelationship")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "SEVERED#local").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "INSTANCE#remote#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.SeveredRelationship")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.SeveredRelationship)
		s := models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)
		s.ID = "local_remote_1"
		*dest = append(*dest, s)
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()

	repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
	got, err := repo.GetSeveredRelationship(ctx, "local_remote_1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "local_remote_1", got.ID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetSeveredRelationship cap exhaustion: a partition whose pages never contain
// the target ID must fail closed with errBoundedPageCapExceeded instead of
// degrading to NotFound — a mutation that swallows the cap into a NotFound
// result dies on require.ErrorIs.
func TestBatchS1_GetSeveredRelationship_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SeveredRelationship")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "SEVERED#local").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "INSTANCE#remote#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.SeveredRelationship)
		items := make([]*models.SeveredRelationship, 500)
		for i := range items {
			items[i] = &models.SeveredRelationship{ID: "other"}
		}
		*dest = items
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.SeveredRelationship")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
	got, err := repo.GetSeveredRelationship(ctx, "local_remote_missing")
	require.Nil(t, got)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetReconnectionAttempts: the keyed chain (SEVERED#<id> + RECONNECT# SK
// prefix + OrderBy SK DESC) walks in bounded pages, accumulating every page in
// cursor order. A mutation restoring `.Scan` dies on the strict mock.
func TestBatchS1_GetReconnectionAttempts_KeyedWalkAccumulatesPages(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SeveranceReconnectionAttempt")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "SEVERED#sev").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "RECONNECT#").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.SeveranceReconnectionAttempt")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.SeveranceReconnectionAttempt)
		*dest = []*models.SeveranceReconnectionAttempt{
			{SeveranceID: "sev", ID: "a1"},
			{SeveranceID: "sev", ID: "a2"},
		}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.SeveranceReconnectionAttempt")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.SeveranceReconnectionAttempt)
		*dest = []*models.SeveranceReconnectionAttempt{{SeveranceID: "sev", ID: "a3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
	attempts, err := repo.GetReconnectionAttempts(ctx, "sev")
	require.NoError(t, err)
	require.Len(t, attempts, 3)
	require.Equal(t, "a1", attempts[0].ID)
	require.Equal(t, "a2", attempts[1].ID)
	require.Equal(t, "a3", attempts[2].ID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetReconnectionAttempts cap exhaustion: the sentinel split must fail the
// read closed BEFORE the pre-existing IsNotFound→empty swallow — a mutation
// that maps cap exhaustion to an empty success result dies on require.Error.
func TestBatchS1_GetReconnectionAttempts_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SeveranceReconnectionAttempt")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "SEVERED#sev").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "RECONNECT#").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.SeveranceReconnectionAttempt")).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())
	attempts, err := repo.GetReconnectionAttempts(ctx, "sev")
	require.Nil(t, attempts)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// CascadeDeleteAnnounces cap exhaustion: a partition that never exhausts must
// fail the cascade closed with errBoundedPageCapExceeded and issue NO Delete —
// a mutation that swallows the cap and deletes the partial set dies on the
// strict mocks (no Delete expectation registered, Model called exactly once).
func TestBatchS1_CascadeDeleteAnnounces_CapExhaustionFailsClosedNoPartialDelete(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Announce")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "OBJECT#s1#ANNOUNCES").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Announce")).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
	disableSocialEnhancedServices(repo)
	err := repo.CascadeDeleteAnnounces(ctx, "s1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// CascadeDeleteAnnounces success path: the walked rows are deleted by exact
// PK+SK. The literal partition-key pin proves the read is keyed; a mutation
// that drops the walk collection (so deletes never run) dies on the missing
// Delete call.
func TestBatchS1_CascadeDeleteAnnounces_KeyedWalkDeletesAnnounces(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQueryRead := new(mocks.MockQuery)
	mockQueryDelete := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(2)

	mockDB.On("Model", mock.AnythingOfType("*models.Announce")).Return(mockQueryRead).Once()
	mockQueryRead.On("Where", "PK", "=", "OBJECT#s1#ANNOUNCES").Return(mockQueryRead).Once()
	mockQueryRead.On("Limit", 500).Return(mockQueryRead).Once()
	mockQueryRead.On("AllPaginated", mock.AnythingOfType("*[]models.Announce")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Announce)
		*dest = []models.Announce{{PK: "OBJECT#s1#ANNOUNCES", SK: "ACTOR#alice", Actor: "alice", Object: "s1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Announce")).Return(mockQueryDelete).Once()
	mockQueryDelete.On("Where", "PK", "=", "OBJECT#s1#ANNOUNCES").Return(mockQueryDelete).Once()
	mockQueryDelete.On("Where", "SK", "=", "ACTOR#alice").Return(mockQueryDelete).Once()
	mockQueryDelete.On("Delete").Return(nil).Once()

	repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
	disableSocialEnhancedServices(repo)
	require.NoError(t, repo.CascadeDeleteAnnounces(ctx, "s1"))
	mockDB.AssertExpectations(t)
	mockQueryRead.AssertExpectations(t)
	mockQueryDelete.AssertExpectations(t)
}

// getTombstonesByGSI: the GSI chain (gsi1PK equality + clamped Limit) must end
// in a keyed All — a mutation restoring `.Scan` dies on the strict mock. A zero
// limit is floored to the default 50, pinning Limit(50+1) so a zero call can
// never compile to an unbounded read.
func TestBatchS1_GetTombstonesByActor_KeyedAllWithZeroLimitFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "ACTOR#alice#TOMBSTONES").Return(mockQuery).Once()
	mockQuery.On("Limit", 51).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Tombstone")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Tombstone)
		*dest = []*models.Tombstone{{ID: "o1"}}
	}).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	tombstones, next, err := repo.GetTombstonesByActor(ctx, "alice", 0, "")
	require.NoError(t, err)
	require.Len(t, tombstones, 1)
	require.Empty(t, next)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// getTombstonesByGSI cursor path: the caller's limit (10) passes through the
// clamp and the sort-key range (> cursor) stays a key condition on the keyed
// All; the next cursor is derived from the trimmed page's last GSI2SK.
func TestBatchS1_GetTombstonesByType_CursorRangeOnKeyedAll(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "TOMBSTONE#Note").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", ">", "cursor-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 11).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Tombstone")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Tombstone)
		for i := 0; i < 12; i++ {
			*dest = append(*dest, &models.Tombstone{ID: "o", GSI2SK: "DELETED#x"})
		}
	}).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	tombstones, next, err := repo.GetTombstonesByType(ctx, activitypub.NoteType, 10, "cursor-1")
	require.NoError(t, err)
	require.Len(t, tombstones, 10)
	require.Equal(t, "DELETED#x", next)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// searchAllActors zero-limit floor: a 0/0 call must compile to Limit(20) — the
// clamped default — never to NO limit (the raw `Limit(limit+offset)` shape
// compiled Limit(0), which tabletheory drops, reading the whole gsi3 DOMAIN
// partition). A mutation restoring the raw shape dies on the strict Limit(20)
// expectation.
func TestBatchS1_SearchAllActors_ZeroLimitFloorsReadLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	// Exact-match leg: getActorModel point read on ACTOR#bo / PROFILE.
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#bo").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		a := args.Get(0).(*models.Actor)
		a.Username = "bo"
		a.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bo"},
			PreferredUsername: "bo",
			Name:              "Bo",
		}
	}).Return(nil).Once()

	// Search leg: keyed gsi3 DOMAIN#example.com read, floored Limit(20).
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "DOMAIN#example.com").Return(mockQuery).Once()
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Actor)
		*dest = []models.Actor{{Username: "bob", Actor: &activitypub.Actor{PreferredUsername: "bob", Name: "Bob"}}}
	}).Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	actors := repo.searchAllActors(ctx, "bo", 0, 0)
	require.Len(t, actors, 2) // exact match + gsi3 match
	require.Equal(t, "bo", actors[0].PreferredUsername)
	require.Equal(t, "bob", actors[1].PreferredUsername)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// searchAllActors limit+offset paging contract: the read window stays
// clamp(limit)+offset (0/5 → Limit(25)), so offset pagination is preserved
// under the floor.
func TestBatchS1_SearchAllActors_OffsetAddsToClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	// Exact match not found: getActorModel First returns not-found and the
	// fallback canonical-handle lookup is exhausted, so search proceeds to GSI3.
	mockQuery.On("Where", "PK", "=", "ACTOR#bo").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(ErrTestMockError).Once()

	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "DOMAIN#example.com").Return(mockQuery).Once()
	mockQuery.On("Limit", 25).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	actors := repo.searchAllActors(ctx, "bo", 0, 5)
	require.Empty(t, actors)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
