package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch N4 (umbrella #1469, 2026-08-28) — the keyed whole-partition remainder
// (108 sites / 52 files).
//
// Every conversion is a bounded page walk via walkKeyedPages (Limit(500)/page,
// 100-page cap, fail-closed errBoundedPageCapExceeded) or a floor/clamp that
// always issues Limit(n>0). Where a pre-existing error swallow would route cap
// exhaustion into a silent empty/partial result, a sentinel split routes
// errBoundedPageCapExceeded back to the caller FIRST.
//
// Every assertion pins a LITERAL (Limit(500), 100-page cap, exact cursor
// handoff) so that removing a bound or breaking the fail-closed routing kills
// the test. Each test fills a full page of 500 rows with HasMore=true for 100
// pages — the walk then hits the cap and MUST fail closed with
// errBoundedPageCapExceeded instead of returning a truncated/empty result.

// fullPage helper fills a page of 500 rows for the element type used by the
// walk under test; combined with HasMore=true + cursor handoff this exhausts
// the 100-page cap so the walk must fail closed.

// walkCapExhausted asserts that fn fails closed with errBoundedPageCapExceeded.
func walkCapExhausted(t *testing.T, mockQuery *dynamormmocks.MockQuery, elementType string, fn func() error) {
	t.Helper()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		switch v := args.Get(0).(type) {
		case *[]models.CommunityNoteVote:
			*v = make([]models.CommunityNoteVote, 500)
		case *[]models.UserConversationState:
			*v = make([]models.UserConversationState, 500)
		case *[]models.GraphQLStreamSubscription:
			*v = make([]models.GraphQLStreamSubscription, 500)
		case *[]models.WebSocketSubscription:
			*v = make([]models.WebSocketSubscription, 500)
		case *[]models.WebSocketEventSubscription:
			*v = make([]models.WebSocketEventSubscription, 500)
		case *[]models.WebSocketEventConnection:
			*v = make([]models.WebSocketEventConnection, 500)
		case *[]models.WebSocketConnection:
			*v = make([]models.WebSocketConnection, 500)
		case *[]models.InstanceHistory:
			*v = make([]models.InstanceHistory, 500)
		case *[]models.HashtagUsage:
			*v = make([]models.HashtagUsage, 500)
		}
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType(elementType)).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)
	err := fn()
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetCommunityNoteVotes_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CommunityNoteVote")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()

	repo := NewCommunityNoteRepository(mockDB, "test-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.CommunityNoteVote", func() error {
		_, err := repo.GetCommunityNoteVotes(ctx, "n1")
		return err
	})
}

func TestBatchN4_ListConversationParticipantStates_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "CONVERSATION#conv-1").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi3SK", "ASC").Return(mockQuery).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.UserConversationState", func() error {
		_, err := repo.ListConversationParticipantStates(ctx, "conv-1")
		return err
	})
}

func TestBatchN4_GraphQLListByStream_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.GraphQLStreamSubscription")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "GQLSUB#stream-1").Return(mockQuery).Once()

	repo := NewGraphQLStreamSubscriptionRepository(mockDB, "test-table", zap.NewNop())
	walkCapExhausted(t, mockQuery, "*[]models.GraphQLStreamSubscription", func() error {
		_, err := repo.ListByStream(ctx, "stream-1")
		return err
	})
}

func TestBatchN4_GraphQLDeleteAllForConnection_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.GraphQLStreamSubscription")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "CONN#conn-1").Return(mockQuery).Once()

	repo := NewGraphQLStreamSubscriptionRepository(mockDB, "test-table", zap.NewNop())
	walkCapExhausted(t, mockQuery, "*[]models.GraphQLStreamSubscription", func() error {
		return repo.DeleteAllForConnection(ctx, "conn-1")
	})
}

func TestBatchN4_StreamingDeleteAllSubscriptions_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	subDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	subDB.On("WithContext", ctx).Return(subDB)
	subDB.On("Model", mock.AnythingOfType("*models.WebSocketSubscription")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "CONN#conn-1").Return(mockQuery).Once()

	repo := NewStreamingConnectionRepository(mockDB, "test-table", subDB, "sub-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketSubscription", func() error {
		return repo.DeleteAllSubscriptions(ctx, "conn-1")
	})
}

func TestBatchN4_StreamingGetConnectionCountByState_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketConnection")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "STATE#connected").Return(mockQuery).Once()

	repo := NewStreamingConnectionRepository(mockDB, "test-table", mockDB, "sub-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketConnection", func() error {
		_, err := repo.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
		return err
	})
}

func TestBatchN4_StreamingGetUserConnectionCount_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketConnection")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER#user-1").Return(mockQuery).Once()

	repo := NewStreamingConnectionRepository(mockDB, "test-table", mockDB, "sub-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketConnection", func() error {
		_, err := repo.GetUserConnectionCount(ctx, "user-1")
		return err
	})
}

func TestBatchN4_WebSocketGetSubscriptionsForType_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketEventSubscription")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "SUBSCRIPTION#notifications").Return(mockQuery).Once()

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketEventSubscription", func() error {
		_, err := repo.GetSubscriptionsForType(ctx, "notifications")
		return err
	})
}

func TestBatchN4_WebSocketGetAllConnections_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketConnection")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "STATE#connected").Return(mockQuery).Once()

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketConnection", func() error {
		_, err := repo.GetAllConnections(ctx)
		return err
	})
}

func TestBatchN4_WebSocketGetUserConnections_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketEventConnection")).Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER#user-1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", "begins_with", "CONNECTION#").Return(mockQuery).Once()

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	walkCapExhausted(t, mockQuery, "*[]models.WebSocketEventConnection", func() error {
		_, err := repo.GetUserConnections(ctx, "user-1")
		return err
	})
}

func TestBatchN4_GetMetricsSummary_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceHistory")).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// The first metric type's walk caps out: GetMetricsSummary must fail the
	// whole summary closed instead of skipping the metric (the pre-existing
	// log+continue swallow would return a partial summary — the test dies on
	// require.ErrorIs).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		out := args.Get(0).(*[]models.InstanceHistory)
		*out = make([]models.InstanceHistory, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.InstanceHistory")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewInstanceRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetMetricsSummary(ctx, "week")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetHashtagStats_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	// GetHashtagInfo resolves through the BaseRepository Get (First terminal).
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.Hashtag")).Run(func(args mock.Arguments) {
		dst := args.Get(0).(*models.Hashtag)
		*dst = models.Hashtag{Name: "golang", UsageCount: 42, FirstSeen: time.Now(), LastUsed: time.Now()}
	}).Return(nil).Once()

	// The 7 per-day usage-history counts (BaseRepository.Count walk on the
	// embedded Hashtag base — *[]*models.Hashtag) run first and succeed with
	// empty partitions; the 30-day unique-user walk ([]models.HashtagUsage)
	// then caps out — GetHashtagStats must fail closed instead of the
	// pre-existing warn-and-swallow returning zero unique users.
	mockQuery.On("Limit", 500).Return(mockQuery).Maybe()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Hashtag")).Return(&core.PaginatedResult{HasMore: false}, nil).Times(7)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.HashtagUsage")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.HashtagUsage)
		*out = make([]models.HashtagUsage, 500)
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Maybe()
	mockQuery.On("Cursor", "c").Return(mockQuery).Maybe()

	repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagStats(ctx, "#golang")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetHashtagStats_CapExhaustionStillFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.Hashtag")).Run(func(args mock.Arguments) {
		dst := args.Get(0).(*models.Hashtag)
		*dst = models.Hashtag{Name: "golang", UsageCount: 1, FirstSeen: time.Now(), LastUsed: time.Now()}
	}).Return(nil).Once()

	// The 7 per-day usage-history counts (BaseRepository.Count walk on the
	// embedded Hashtag base — *[]*models.Hashtag) succeed with empty partitions;
	// the 30-day unique-user walk's FIRST page returns cap exhaustion:
	// GetHashtagStats must fail closed (the pre-existing swallow would degrade
	// to zero unique users and return stats — the test dies on require.ErrorIs).
	mockQuery.On("Limit", 500).Return(mockQuery).Maybe()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.Hashtag")).Return(&core.PaginatedResult{HasMore: false}, nil).Times(7)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.HashtagUsage")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.HashtagUsage)
		*out = []models.HashtagUsage{{AuthorID: "u1"}}
	}).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
	_, err := repo.GetHashtagStats(ctx, "#golang")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetCommunityNoteVotes_CapExhaustionFirstPageFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CommunityNoteVote")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CommunityNoteVote")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewCommunityNoteRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetCommunityNoteVotes(ctx, "n1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetCommunityNoteVotes_IsNotFoundSwallowsToEmpty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CommunityNoteVote")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// An ordinary not-found walk error keeps the pre-existing swallow
	// (community_note_repository.go:390-391): no votes for this note, success.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CommunityNoteVote")).Return(nil, dynamormErrors.ErrItemNotFound).Once()

	repo := NewCommunityNoteRepository(mockDB, "test-table", zap.NewNop(), nil)
	votes, err := repo.GetCommunityNoteVotes(ctx, "n1")
	require.NoError(t, err)
	require.Empty(t, votes)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_ConversationGetMutedConversations_WalkPreservesExpiryFilter(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("Model", mock.AnythingOfType("*models.ConversationMute")).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	// The expired mute is cleaned up via DeleteConversationMute (best effort).
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Maybe()
	mockQuery.On("Where", "SK", "=", "CONVERSATION_MUTE#c1").Return(mockQuery).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ConversationMute")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ConversationMute)
		*out = []models.ConversationMute{
			{PK: "USER#alice", SK: "CONV_MUTE#c1", ConversationID: "c1", ExpiresAt: time.Now().Add(-time.Hour)}, // expired
			{PK: "USER#alice", SK: "CONV_MUTE#c2", ConversationID: "c2", ExpiresAt: time.Now().Add(time.Hour)},  // live
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	ids, err := repo.GetMutedConversations(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, []string{"c2"}, ids)
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_GetAccountPreferences_WalkPreservesBooleanParsing(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreference")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "PREFERENCE#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.UserPreference")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.UserPreference)
		*out = []models.UserPreference{
			{Key: "a", Value: "true"},
			{Key: "b", Value: "false"},
			{Key: "c", Value: "hello"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	prefs, err := repo.GetAccountPreferences(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, true, prefs["a"])
	require.Equal(t, false, prefs["b"])
	require.Equal(t, "hello", prefs["c"])
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_ListConversationParticipantStates_WalkConversion(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "CONVERSATION#conv-1").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi3SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.UserConversationState")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.UserConversationState)
		*out = []models.UserConversationState{{ViewerID: "u1", ConversationID: "conv-1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	items, err := repo.ListConversationParticipantStates(ctx, "conv-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, interfaces.UserConversationStateContract{ViewerID: "u1", ConversationID: "conv-1"}, *items[0])
	mockQuery.AssertExpectations(t)
}

func TestBatchN4_CreateFeaturedTag_StatisticsErrorPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi3").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// CreateFeaturedTag first checks for existing tags (GetFeaturedTags, the
	// bounded Limit(101) QueryWithSKPrefixPaginated path — All terminal).
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FeaturedTag")).Return(nil).Once()

	// calculateTagStatistics walk caps out; the error must propagate out of
	// CreateFeaturedTag (the helper's signature gained an error return in N4 —
	// previously the swallow returned 0, nil).
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewFeaturedTagRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.CreateFeaturedTag(ctx, &storage.FeaturedTag{Username: "alice", Name: "golang"})
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// Multi-page accumulation pin: the walk must APPEND every page into the
// collected slice — a `collected = page` overwrite mutation truncates to the
// last page and this test dies on require.Len(votes, 1000).
func TestBatchN4_GetCommunityNoteVotes_MultiPageAccumulationAppends(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CommunityNoteVote")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTE#n1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "VOTE#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	fullPage := func(args mock.Arguments) {
		out := args.Get(0).(*[]models.CommunityNoteVote)
		*out = make([]models.CommunityNoteVote, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CommunityNoteVote")).Run(fullPage).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()
	mockQuery.On("Cursor", "c").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CommunityNoteVote")).Run(fullPage).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewCommunityNoteRepository(mockDB, "test-table", zap.NewNop(), nil)
	votes, err := repo.GetCommunityNoteVotes(ctx, "n1")
	require.NoError(t, err)
	require.Len(t, votes, 1000)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework (PR #1493 attack, review 5047884667): GetWalletByAddress's
// walk error previously hit the not-found demote which passes nil to
// HandleGetError and returns (nil, nil) — "no wallet, success". Cap exhaustion
// must propagate as a real error.
func TestBatchN4_GetWalletByAddress_CapExhaustionPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "WALLET#ethereum#0xabc").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "USER#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewAuthRepository(mockDB, "test-table", zap.NewNop())
	cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xAbC")
	require.Nil(t, cred)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework: GetAlertStats's three count-family loops previously logged
// and continued on ANY error, returning zeroed counters as a valid stats
// object. Cap exhaustion must fail the whole call closed.
func TestBatchN4_GetAlertStats_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Alert")).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi3").Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", 500).Return(mockQuery).Maybe()

	// The first status count (firing) caps out: the statuses loop's sentinel
	// split must fail GetAlertStats closed instead of returning a partial stats
	// object with zeroed counters.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Alert")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)
	stats, err := repo.GetAlertStats(ctx, time.Now().Add(-time.Hour))
	require.Nil(t, stats)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework: the existing GetAlertStats cap test drives the STATUS loop,
// which errors first — leaving the TYPE loop's sentinel split unpinned. Here the
// status (4 gsi3 walks) and severity (4 × getAllAlertsSince's 6 gsi1 All reads)
// count loops succeed with empty partitions and the FIRST type walk then caps
// out: the types loop's sentinel split must fail the whole call closed. A
// mutation that removes the split lets the log-and-continue swallow return a
// valid stats object with zeroed ByType (and then hit the un-mocked second type
// walk) — this test dies.
func TestBatchN4_GetAlertStats_TypeLoopCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Alert")).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	// 4 status counts (gsi3 STATUS# walks) succeed with empty partitions, then 4
	// severity counts (getAllAlertsSince: 6 gsi1 ALERT_TYPE# reads each via
	// Limit(1000/6)) succeed with empty results — the cap-exhaustion error can
	// only come from the type walk that follows.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Alert")).Return(&core.PaginatedResult{HasMore: false}, nil).Times(4)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Alert")).Return(nil).Times(24)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Alert")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewAlertRepository(mockDB, "test-table", zap.NewNop(), nil)
	stats, err := repo.GetAlertStats(ctx, time.Now().Add(-time.Hour))
	require.Nil(t, stats)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework: GetOAuthSessionByState replaced the walk error with a
// synthesized "OAuth session not found for state". The sentinel must propagate
// distinguishably.
func TestBatchN4_GetOAuthSessionByState_CapExhaustionPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthAuthSession")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "OAUTH_STATE#state-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.OAuthAuthSession")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewOAuthSessionRepository(mockDB, "test-table", zap.NewNop(), nil)
	session, err := repo.GetOAuthSessionByState(ctx, "state-1")
	require.Nil(t, session)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-2 rework: GetUserOAuthSessions at limit==0 walks the whole keyed gsi1
// partition (page-capped, fail-closed) instead of flooring at a single 500-row
// page — CountUserOAuthSessions above 500 was silently under-reported.
func TestBatchN4_GetUserOAuthSessions_WalkAtZero_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthAuthSession")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_OAUTH#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.OAuthAuthSession")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewOAuthSessionRepository(mockDB, "test-table", zap.NewNop(), nil)
	sessions, err := repo.GetUserOAuthSessions(ctx, "user-1", 0)
	require.Nil(t, sessions)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-2 rework: the limit==0 walk accumulates across pages — a floor that
// issues a single Limit(500) read would return 500 rows here and die on
// require.Len(sessions, 1000); a `sessions = page` overwrite dies too.
func TestBatchN4_GetUserOAuthSessions_WalkAtZero_MultiPageAccumulation(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthAuthSession")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_OAUTH#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	validPage := func(args mock.Arguments) {
		out := args.Get(0).(*[]models.OAuthAuthSession)
		*out = make([]models.OAuthAuthSession, 500)
		for i := range *out {
			(*out)[i].ExpiresAt = time.Now().Add(time.Hour).Unix()
		}
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.OAuthAuthSession")).Run(validPage).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()
	mockQuery.On("Cursor", "c").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.OAuthAuthSession")).Run(validPage).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewOAuthSessionRepository(mockDB, "test-table", zap.NewNop(), nil)
	sessions, err := repo.GetUserOAuthSessions(ctx, "user-1", 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1000)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework: searchFollowedActors gained an error return; cap exhaustion
// must propagate through SearchActors instead of returning a partial/empty
// follow graph as success.
func TestBatchN4_SearchActors_CapExhaustionPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Follow")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "FOLLOWER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "FOLLOWS#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Follow")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	actors, err := repo.SearchActors(ctx, "bo", 10, 0, true, "alice")
	require.Nil(t, actors)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}

// MAJOR-1 rework: getFollowingUsernames gained an error return; cap exhaustion
// must propagate through GetAccountSuggestions instead of returning suggestions
// computed over an empty/partial following set.
func TestBatchN4_GetAccountSuggestions_CapExhaustionPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Follow")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "FOLLOWER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "FOLLOWS#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Follow")).Return(nil, errBoundedPageCapExceeded).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	suggestions, err := repo.GetAccountSuggestions(ctx, "alice", 5)
	require.Nil(t, suggestions)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockQuery.AssertExpectations(t)
}
