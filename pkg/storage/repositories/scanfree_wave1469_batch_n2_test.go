package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch N2 (umbrella #1469, 2026-08-27) — the BOUNDED-QUERY class in
// user_repository.go / object_repository.go / analytics_repository.go: keyed
// whole-partition `.All()` reads with no enforced bound. Every assertion is
// mutation-viable: it pins the LITERAL bound (reverting a clamp issues a
// Limit the mock does not expect — the test dies), the exact 100-page cap
// count at which a walk fails closed (Times(100) + Cursor Times(99)), and the
// cursor handoff sequence between pages. notification_repository.go
// enumerated to 0 sites (every `.All()` chain there already carries an
// explicit Limit).
//
// The `mockWalkExpectations` helper (scanfree_wave1469_batch_n1_test.go)
// registers the Limit(pageSize) + Cursor + AllPaginated sequence for a walk.

// ===== user_repository.go — page-capped walks =====

func TestBatchN2_GetActiveUserCount_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "ACTIVITY").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	// Two bounded pages with an explicit cursor handoff; reverting the walk to
	// a bare `.All()` leaves AllPaginated/Cursor unfulfilled and dies.
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{{Username: "user-1"}, {Username: "user-2"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{{Username: "user-3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetActiveUserCount_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "ACTIVITY").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = make([]models.User, 500)
	}
	// Exactly 100 full pages with more available: the walk must fail closed at
	// the 100-page cap. A `>` vs `>=` off-by-one changes the call count.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.User")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetActiveUserCount(ctx, 30)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_UnlinkProviderAccount_PageCappedWalkThenDelete(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery).Once()
	// Two pages: page 1 has github (not google, filtered out), page 2 has the
	// google row to delete. Provider filtering is partition-wide across the
	// collected rows — a per-page stop would miss the page-2 match.
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ProviderAccount")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = []models.ProviderAccount{{UserID: "testuser", Provider: "github", ProviderID: "456", IsActive: true}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ProviderAccount")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = []models.ProviderAccount{{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	// The google row is deleted with its own keyed Model chain.
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.UnlinkProviderAccount(ctx, "testuser", "google"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetLinkedProviders_PageCappedWalkExactPageSize(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// Exactly one full 500-item page with HasMore=false: the walk must stop
	// after it (an exact page-size multiple is NOT cap exhaustion).
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ProviderAccount")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = make([]models.ProviderAccount, 500)
		for i := range *dest {
			(*dest)[i] = models.ProviderAccount{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	providers, err := repo.GetLinkedProviders(ctx, "testuser")
	require.NoError(t, err)
	require.Len(t, providers, 1) // 500 identical google rows dedup to one provider
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetAccountPins_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACCOUNT_PIN#alice").Return(mockQuery).Once()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "PIN#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AccountPin")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.AccountPin)
		*dest = []models.AccountPin{{
			Username: "alice", PinnedActorID: "https://example.com/users/bob",
			PinnedUsername: "bob", CreatedAt: time.Now(),
		}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AccountPin")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	pins, err := repo.GetAccountPins(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	require.Equal(t, "bob", pins[0].PinnedUsername)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetVouchesByActor_PageCappedWalkWithActiveFilter(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "VOUCHER#alice").Return(mockQuery).Once()
	mockQuery.On("Filter", "Active", "=", true).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Vouch")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{{VouchData: `{"id":"vouch-1","from":"alice","to":"bob","active":true}`}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Vouch")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	vouches, err := repo.GetVouchesByActor(ctx, "alice", true)
	require.NoError(t, err)
	require.Len(t, vouches, 1)
	require.Equal(t, "vouch-1", vouches[0].ID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetMonthlyVouchCount_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "VOUCHER#alice").Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetMonthlyVouchCount(ctx, "alice", 2026, time.August)
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetMutedConversations_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationMute")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice#CONV_MUTES").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ConversationMute")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ConversationMute)
		*dest = []models.ConversationMute{
			{ConversationID: "active-1", ExpiresAt: time.Now().Add(time.Hour)},
			{ConversationID: "expired-1", ExpiresAt: time.Now().Add(-time.Hour)},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	ids, err := repo.GetMutedConversations(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, []string{"active-1"}, ids)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_ListUsersByRole_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "ROLE#admin").Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	users, err := repo.ListUsersByRole(ctx, "admin")
	require.NoError(t, err)
	require.Empty(t, users)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== user_repository.go — limit clamps =====

func TestBatchN2_GetReputationHistory_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -5, 20},
		{"over max clamps to 100", 1000, 100},
		{"in-range limit passes through", 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery)
			// A canonical actor resolves to two key candidates
			// (ACTOR#<canonical> and the legacy ACTOR#<username>), so the loop
			// queries both partitions.
			mockQuery.On("Where", "PK", "=", mock.MatchedBy(matchAliceReputationPK)).Return(mockQuery).Times(2)
			mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Times(2)
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Times(2)
			// The clamp must always issue Limit(<sanitized>) — reverting the
			// clamp (old `if remaining > 0` gate at limit<=0) issues no Limit,
			// which leaves this expectation unfulfilled and dies.
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Times(2)
			mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Return(nil).Times(2)

			repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
			history, err := repo.GetReputationHistory(ctx, "https://example.com/users/alice", tt.limit)
			require.NoError(t, err)
			require.Empty(t, history)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN2_GetTimelineEntries_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int // Limit(limit+1) after the clamp
	}{
		{"limit zero defaults to 20 -> Limit(21)", 0, 21},
		{"negative limit defaults to 20 -> Limit(21)", -3, 21},
		{"over max clamps to 100 -> Limit(101)", 500, 101},
		{"in-range limit passes through", 2, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.Timeline")).Return(mockQuery)
			mockQuery.On("Where", "PK", "=", "timeline#DIRECT#alice").Return(mockQuery).Once()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]*models.Timeline")).Return(nil).Once()

			repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
			_, _, err := repo.GetDirectTimeline(ctx, "alice", tt.limit, "")
			require.NoError(t, err)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

// ===== object_repository.go — page-capped walks =====

func TestBatchN2_CountObjectReplies_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "reply#note-1").Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.CountObjectReplies(ctx, "note-1")
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_CountObjectReplies_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "reply#note-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Object)
		*dest = make([]models.Object, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	_, err := repo.CountObjectReplies(ctx, "note-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetObjectsByActor_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedFirst int // Limit(limit) before the cursor
		expectedFinal int // Limit(limit+1) sentinel
	}{
		{"limit zero defaults to 20", 0, 20, 21},
		{"negative limit defaults to 20", -1, 20, 21},
		{"over max clamps to 100", 500, 100, 101},
		{"in-range limit passes through", 1, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1PK", "=", "actor#alice").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedFirst).Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedFinal).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.Object")).Return(nil).Once()

			repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
			_, _, err := repo.GetObjectsByActor(ctx, "alice", "", tt.limit)
			require.NoError(t, err)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN2_GetQuotesOfStatus_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -1, 20},
		{"over max clamps to 100", 500, 100},
		{"in-range limit passes through", 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1PK", "=", "QUOTED#status-1").Return(mockQuery).Once()
			mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.QuoteRelationship")).Return(nil).Once()

			repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
			_, err := repo.GetQuotesOfStatus(ctx, "status-1", tt.limit)
			require.NoError(t, err)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN2_GetDirectReplies_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi6PK", "=", "REPLIES#status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Object)
		*dest = []models.Object{{ID: "reply-1"}, {ID: "reply-2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	replies, err := repo.getDirectReplies(ctx, "status-1")
	require.NoError(t, err)
	require.Equal(t, []string{"reply-1", "reply-2"}, replies)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetThreadAncestors_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ThreadContext")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "THREAD#root-1").Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	ancestors, err := repo.getThreadAncestors(ctx, "root-1", "status-1", 5)
	require.NoError(t, err)
	require.Empty(t, ancestors)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetThreadDescendants_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ThreadContext")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "THREAD#root-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ThreadContext")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ThreadContext)
		*dest = []models.ThreadContext{
			{StatusID: "child-1", Path: "/root-1/status-1/child-1"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	descendants, err := repo.getThreadDescendants(ctx, "root-1", "status-1")
	require.NoError(t, err)
	require.Equal(t, []string{"child-1"}, descendants)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_WithdrawExistingQuotes_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "QUOTED#status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.QuoteRelationship")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.QuoteRelationship)
		*dest = []models.QuoteRelationship{{PK: "QUOTE#q1", SK: "TARGET#status-1"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.QuoteRelationship")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	// The withdrawn quote is persisted back with its own keyed chain.
	mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "QUOTE#q1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "TARGET#status-1").Return(mockQuery).Once()
	// MockQuery.Update(fields ...string) passes the (possibly empty) field list
	// as a single []string argument.
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	require.NoError(t, repo.withdrawExistingQuotes(ctx, "status-1"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_WithdrawExistingQuotes_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "QUOTED#status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.QuoteRelationship)
		*dest = make([]models.QuoteRelationship, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.QuoteRelationship")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	err := repo.withdrawExistingQuotes(ctx, "status-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== analytics_repository.go — page-capped walks =====

func TestBatchN2_RecordHashtagUsage_WalksUsagePartition(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "HASHTAG#golang").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.HashtagUsage")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.HashtagUsage)
		*dest = []models.HashtagUsage{
			{AuthorID: "user-1", UsedAt: time.Now()},
			{AuthorID: "user-2", UsedAt: time.Now()},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery).Once()
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	require.NoError(t, repo.RecordHashtagUsage(ctx, "golang", "status-1", "user-3"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_RecordStatusEngagement_WalksEngagementPartition(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "STATUS_ENGAGEMENT#status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.StatusEngagement)
		*dest = []models.StatusEngagement{{UserID: "user-1", EngagementType: "like"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery).Once()
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	require.NoError(t, repo.RecordStatusEngagement(ctx, "status-1", "like", "user-2"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_RecordLinkShare_WalksSharePartition(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "LINK_SHARE#https://example.com/a").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.LinkShare")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.LinkShare)
		*dest = []models.LinkShare{{AuthorID: "user-1", SharedAt: time.Now()}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery).Once()
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	require.NoError(t, repo.RecordLinkShare(ctx, "https://example.com/a", "status-1", "user-2"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_AggregateEngagementMetrics_PageCappedWalks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	date1 := "2026-08-26"
	date2 := "2026-08-27"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "METRICS#status#"+date1).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.EngagementMetrics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.EngagementMetrics)
		*dest = []models.EngagementMetrics{{Date: date1, MetricType: "status", TargetID: "t1", Views: 10}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("Where", "PK", "=", "METRICS#status#"+date2).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.EngagementMetrics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.EngagementMetrics)
		*dest = []models.EngagementMetrics{{Date: date2, MetricType: "status", TargetID: "t1", Views: 5}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	agg, err := repo.AggregateEngagementMetrics(ctx, "status", []string{date1, date2})
	require.NoError(t, err)
	require.Equal(t, int64(15), agg.TotalViews)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_AggregateEngagementMetrics_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	date := "2026-08-26"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "METRICS#status#"+date).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.EngagementMetrics)
		*dest = make([]models.EngagementMetrics, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.EngagementMetrics")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	_, err := repo.AggregateEngagementMetrics(ctx, "status", []string{date})
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetManifestGenerationStats_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MANIFEST#hls").Return(mockQuery).Once()
	mockQuery.On("Where", "Date", ">=", "2026-08-01").Return(mockQuery).Once()
	mockQuery.On("Where", "Date", "<=", "2026-08-27").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.MediaAnalytics)
		*dest = []models.MediaAnalytics{
			{Date: "2026-08-01"},
			{Date: "2026-08-02"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	stats, err := repo.GetManifestGenerationStats(ctx, "hls", "2026-08-01", "2026-08-27")
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"2026-08-01": 1, "2026-08-02": 1}, stats)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_QuerySessionEvents_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA_EVENT#session_start").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	events, err := repo.querySessionEvents(ctx, time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, events)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_QuerySessionEvents_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA_EVENT#session_start").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.MediaAnalytics)
		*dest = make([]models.MediaAnalytics, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.MediaAnalytics")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	_, err := repo.querySessionEvents(ctx, time.Now().Add(-7*24*time.Hour))
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_QueryBufferingEvents_PageCappedWalkDegradesToZero(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	// Contract: errors — including cap exhaustion — degrade to (0, nil); the
	// sole caller GetStreamingAnalytics discards the error by design
	// (`bufferingEvents, _ := r.queryBufferingEvents(...)`).
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA_EVENT#rebuffer_start").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.MediaAnalytics")).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Times(100)
	mockQuery.On("Cursor", "c1").Return(mockQuery).Times(99)

	count, err := repo.queryBufferingEvents(ctx, "media-1", time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_QueryQualityChangeEvents_PageCappedWalkDegradesToZero(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	// Contract: errors — including cap exhaustion — degrade to (0, nil); the
	// sole caller GetStreamingAnalytics discards the error by design
	// (`qualityChanges, _ := r.queryQualityChangeEvents(...)`).
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "MEDIA_QUALITY#media-1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.MediaAnalytics)
		*dest = make([]models.MediaAnalytics, 2)
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	count, err := repo.queryQualityChangeEvents(ctx, "media-1", time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 2, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetModeratorStats_PageCappedWalks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	now := time.Now()
	date1 := now.Format("2006-01-02")
	date2 := now.AddDate(0, 0, -1).Format("2006-01-02")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MOD_ANALYTICS#"+date1).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ModerationAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationAnalytics)
		*dest = []models.ModerationAnalytics{{
			Date: date1, ReportType: "spam",
			ModeratorActions: map[string]int64{"mod-1": 3},
		}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("Where", "PK", "=", "MOD_ANALYTICS#"+date2).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ModerationAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationAnalytics)
		*dest = []models.ModerationAnalytics{{
			Date: date2, ReportType: "abuse",
			ModeratorActions: map[string]int64{"mod-1": 2},
		}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	stats, err := repo.GetModeratorStats(ctx, "mod-1", 2)
	require.NoError(t, err)
	require.Equal(t, int64(5), stats.TotalActions)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetModeratorStats_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	date := time.Now().Format("2006-01-02")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MOD_ANALYTICS#"+date).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationAnalytics)
		*dest = make([]models.ModerationAnalytics, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ModerationAnalytics")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	_, err := repo.GetModeratorStats(ctx, "mod-1", 1)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== analytics_repository.go — limit clamps =====

func TestBatchN2_GetTrendingHashtags_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery).Once()
	// limit=0 must clamp to the 20 default: the old code compiled Limit(0) —
	// no limit — an unbounded gsi8 partition read. The generic reflection path
	// passes a *[]*models.HashtagTrend destination, so match Any.
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	_, err := repo.GetTrendingHashtags(ctx, time.Now(), 0)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetTrendingLinks_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkTrend")).Return(nil).Once()

	_, err := repo.GetTrendingLinks(ctx, time.Now(), 0)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetRecentStatusesWithEngagement_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	since := time.Now().Add(-24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "ENGAGEMENTS#ALL").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	// limit=0 -> default 20 -> Limit(20*10=200). The old code compiled
	// Limit(0) — no limit — an unbounded ENGAGEMENTS#ALL read.
	mockQuery.On("Limit", 200).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Return(nil).Once()

	statuses, err := repo.GetRecentStatusesWithEngagement(ctx, since, 0)
	require.NoError(t, err)
	require.Empty(t, statuses)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetRecentStatusesWithEngagement_EngineLimitPassesThrough(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	since := time.Now().Add(-24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "ENGAGEMENTS#ALL").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	// The trend aggregator's full-set request of 1000 statuses passes through
	// unchanged (a naive max clamp below 1000 would narrow its semantics):
	// Limit(1000*10=10000).
	mockQuery.On("Limit", 10000).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Return(nil).Once()

	_, err := repo.GetRecentStatusesWithEngagement(ctx, since, 1000)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetRecentLinks_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	since := time.Now().Add(-24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "LINK_SHARES#ALL").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	// limit=0 -> default 20 -> Limit(20*5=100).
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkShare")).Return(nil).Once()

	links, err := repo.GetRecentLinks(ctx, since, 0)
	require.NoError(t, err)
	require.Empty(t, links)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetTopEngagedContent_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	date := "2026-08-27"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "METRICS#status#"+date).Return(mockQuery).Once()
	// limit=0 -> default 20 -> Limit(20*2=40).
	mockQuery.On("Limit", 40).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.EngagementMetrics")).Return(nil).Once()

	rankings, err := repo.GetTopEngagedContent(ctx, "status", date, 0)
	require.NoError(t, err)
	require.Empty(t, rankings)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetEngagementByDateRange_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EngagementMetrics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	// limit=0 -> default 20 -> Limit(20) on each day of the two-day range.
	mockQuery.On("Limit", 20).Return(mockQuery).Twice()
	mockQuery.On("All", mock.AnythingOfType("*[]models.EngagementMetrics")).Return(nil).Twice()

	results, err := repo.GetEngagementByDateRange(ctx, "status", "2026-08-26", "2026-08-27", 0)
	require.NoError(t, err)
	require.Empty(t, results)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN2_GetTopQueries_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PopularQueryCounter")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery).Once()
	// limit=0 must clamp to the 20 default (ValidateQueryLimit accepts 0, which
	// previously compiled to Limit(0) — no limit).
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.PopularQueryCounter")).Return(nil).Once()

	_, err := repo.GetTopQueries(ctx, 0, 24*time.Hour)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== walkKeyedPages unit-level cross-page selection (N1 lesson applied) =====

// TestBatchN2_WalkCollectsThenSelects pins that a walk that must select one
// row across pages collects every page before choosing (the N1 GetStatusByURL
// lesson): the second page's row wins even though page 1 already held a
// candidate, because the selection loop runs partition-wide over the whole
// collection.
func TestBatchN2_WalkCollectsThenSelects(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "first", URLs: []string{"https://example.com/u"}}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "canonical", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/u"}}}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	var collected []models.Status
	err := walkKeyedPages(
		mockDB.WithContext(ctx).Model(&models.Status{}),
		500, 100,
		func(page []models.Status) (bool, error) {
			collected = append(collected, page...)
			return false, nil
		},
	)
	require.NoError(t, err)
	require.Len(t, collected, 2)

	// Partition-wide selection over the collected rows (loop 1 then loop 2).
	var selected *models.Status
	for i := range collected {
		if collected[i].Note != nil && collected[i].Note.ID == "https://example.com/u" {
			selected = &collected[i]
			break
		}
	}
	require.NotNil(t, selected)
	require.Equal(t, "canonical", selected.StatusID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestBatchN2_GetAccountPins_PageCapExhaustionFailsClosed pins the exact cap
// on a user-repository walk with a post-read Filter (SK prefix) preserved.
func TestBatchN2_GetAccountPins_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACCOUNT_PIN#alice").Return(mockQuery).Once()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "PIN#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.AccountPin)
		*dest = make([]models.AccountPin, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AccountPin")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetAccountPins(ctx, "alice")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestBatchN2_GetUpdateHistory_ZeroLimitClamped pins the degenerate-input fix
// on GetUpdateHistory (the object-repository history read): limit=0 previously
// compiled to Limit(0) — no limit — an unbounded OBJECT#<id>#HISTORY read.
func TestBatchN2_GetUpdateHistory_ZeroLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UpdateHistory")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OBJECT#note-1#HISTORY").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 10).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	_, err := repo.GetUpdateHistory(ctx, "note-1", 0)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestBatchN2_EmptyPartitionWalksReturnEmpty asserts the empty-partition shape
// for a representative walk: an AllPaginated page that reports no items and no
// cursor terminates the walk without error and returns an empty result.
func TestBatchN2_EmptyPartitionWalksReturnEmpty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "reply#note-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.CountObjectReplies(ctx, "note-1")
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
