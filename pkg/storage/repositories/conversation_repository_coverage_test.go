package repositories

import (
	"context"
	"testing"
	"time"

	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	ddbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound07_ConversationRepository_SweepSuccess(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	conv := &models.Conversation{ID: ""}
	require.NoError(t, repo.CreateConversation(ctx, conv, []string{"user-1", "user-2"}))
	require.Len(t, conv.Participants, 2)
	require.NotEmpty(t, conv.ID)

	_, _ = repo.GetConversation(ctx, "conv-1")
	_ = repo.UpdateConversation(ctx, conv)
	_ = repo.DeleteConversation(ctx, "conv-1")

	_, _ = repo.GetUserConversations(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetConversationByParticipants(ctx, []string{"user-2", "user-1"})
	_ = repo.MarkConversationRead(ctx, "conv-1", "user-1")

	_, _ = repo.GetUnreadConversationCount(ctx, "user-1")
	_ = repo.AddStatusToConversation(ctx, "conv-1", "status-1", "user-1")
	_, _, _ = repo.GetConversationStatuses(ctx, "conv-1", 1, "")
	_ = repo.RemoveStatusFromConversation(ctx, "conv-1", "status-1")
	_ = repo.MarkStatusRead(ctx, "conv-1", "status-ignored", "user-1")
	_, _ = repo.GetUnreadStatusCount(ctx, "conv-1", "user-1")

	_, _ = repo.GetConversationParticipants(ctx, "conv-1")
	_ = repo.UpdateConversationLastStatus(ctx, "conv-1", "status-2")

	mute := &storage.ConversationMute{
		Username:       "user-1",
		ConversationID: "conv-1",
		CreatedAt:      baseTime,
		ExpiresAt:      baseTime.Add(24 * time.Hour),
	}
	require.NoError(t, repo.CreateConversationMute(ctx, mute))
	_ = repo.DeleteConversationMute(ctx, "user-1", "conv-1")
	_, _ = repo.IsConversationMuted(ctx, "user-1", "conv-1")
	_, _ = repo.GetMutedConversations(ctx, "user-1")

	_ = repo.MarkConversationUnread(ctx, "conv-1", "user-1")
	_, _ = repo.GetUnreadConversations(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.SearchConversations(ctx, "user-1", "user", interfaces.PaginationOptions{Limit: 1})

	_, _ = repo.GetConversationMessageCount(ctx, "conv-1")
	_, _ = repo.GetUnreadMessageCount(ctx, "user-1")
	_, _ = repo.GetConversationMessagesByTimeRange(ctx, "conv-1", time.Time{}, time.Time{}, 1)
}

func TestRound07_ConversationRepository_GetConversation_NotFoundBranch(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetConversation(context.Background(), "missing")
	require.Error(t, err)
}

func TestRound07_ConversationRepository_IsConversationMuted_ExpiredDeletes(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		mute := args.Get(0).(*models.ConversationMute)
		mute.Username = "user-1"
		mute.ConversationID = "conv-1"
		mute.ExpiresAt = time.Now().Add(-time.Minute)
	}).Return(nil).Once()

	mockQuery.On("Delete").Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	muted, err := repo.IsConversationMuted(context.Background(), "user-1", "conv-1")
	require.NoError(t, err)
	require.False(t, muted)
}

func TestRound07_ConversationRepository_MessageCount_ScanPaths(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetConversation will fail (forces scan counting path).
	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Once()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.ConversationMessage)
		*ptr = []models.ConversationMessage{{}, {}}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetConversationMessageCount(context.Background(), "conv-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	// Scan error path.
	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Once()
	mockQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan-failed")).Once()
	_, err = repo.GetConversationMessageCount(context.Background(), "conv-1")
	require.Error(t, err)
}

func TestRound07_ConversationRepository_GetConversationStatuses_UsesStatusThreadQuery(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "CONVERSATION#conv-1").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi3SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", ">", "cursor-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{
			{
				StatusID:       "status-1",
				ConversationID: "conv-1",
				AuthorUsername: "alice",
				InReplyToID:    "status-0",
				PublishedAt:    time.Unix(10, 0).UTC(),
				CreatedAt:      time.Unix(10, 0).UTC(),
				GSI3SK:         "2026-03-26T10:00:00Z#status-1",
			},
			{
				StatusID:       "status-2",
				ConversationID: "conv-1",
				AuthorUsername: "bob",
				PublishedAt:    time.Unix(20, 0).UTC(),
				CreatedAt:      time.Unix(20, 0).UTC(),
				GSI3SK:         "2026-03-26T10:01:00Z#status-2",
			},
		}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	statuses, nextCursor, err := repo.GetConversationStatuses(ctx, "conv-1", 1, "cursor-1")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, "status-1", statuses[0].StatusID)
	require.Equal(t, "alice", statuses[0].UserID)
	require.Equal(t, "status-0", statuses[0].ReplyToID)
	require.Equal(t, "2026-03-26T10:00:00Z#status-1", nextCursor)
}

func TestRound07_ConversationRepository_UnreadStatusCount_Branches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	stateQuery := new(mocks.MockQuery)
	messageQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(stateQuery).Times(4)
	stateQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#user-1").Return(stateQuery).Times(4)
	stateQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(stateQuery).Times(4)

	// Missing canonical state -> no unread truth.
	stateQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetUnreadStatusCount(context.Background(), "conv-1", "user-1")
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// Canonical state found, unread=false -> returns 0 without counting.
	stateQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		state := args.Get(0).(*models.UserConversationState)
		state.Unread = false
		state.LastReadAt = conversationTimePtr(time.Unix(1, 0).UTC())
	}).Return(nil).Once()

	count, err = repo.GetUnreadStatusCount(context.Background(), "conv-1", "user-1")
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// Canonical state found, unread=true -> count messages after last read time.
	readAt := time.Unix(2, 0).UTC()
	stateQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		state := args.Get(0).(*models.UserConversationState)
		*state = models.UserConversationState{
			ViewerID:       "user-1",
			ConversationID: "conv-1",
			CounterpartID:  "user-2",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			LastReadAt:     &readAt,
			SortAt:         readAt,
			CreatedAt:      readAt.Add(-time.Hour),
			UpdatedAt:      readAt,
		}
	}).Return(nil).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationMessage")).Return(messageQuery).Once()
	messageQuery.On("WithContext", mock.Anything).Return(messageQuery).Once()
	messageQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(messageQuery).Once()
	messageQuery.On("Where", "SK", ">", mock.Anything).Return(messageQuery).Once()
	messageQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.ConversationMessage)
		*ptr = []models.ConversationMessage{{}, {}, {}}
	}).Return(nil).Once()

	count, err = repo.GetUnreadStatusCount(context.Background(), "conv-1", "user-1")
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// State query error.
	stateQuery.On("First", mock.Anything).Return(stdErrors.New("state-failed")).Once()
	_, err = repo.GetUnreadStatusCount(context.Background(), "conv-1", "user-1")
	require.Error(t, err)
}

func TestRound07_ConversationRepository_GetConversationStatuses_ScanErrorAndPagination(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(stdErrors.New("query-failed")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, _, err := repo.GetConversationStatuses(context.Background(), "conv-1", 1, "")
	require.Error(t, err)

	// Pagination + nextCursor branch.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Status)
		now := time.Unix(10, 0).UTC()
		*ptr = []models.Status{
			{StatusID: "status-1", AuthorUsername: "user-1", CreatedAt: now, GSI3SK: "STATUS#1"},
			{StatusID: "status-2", AuthorUsername: "user-1", CreatedAt: now.Add(time.Second), GSI3SK: "STATUS#2"},
		}
	}).Return(nil).Once()

	_, nextCursor, err := repo.GetConversationStatuses(context.Background(), "conv-1", 1, "")
	require.NoError(t, err)
	require.Equal(t, "STATUS#1", nextCursor)
}

func TestRound07_ConversationRepository_StatusCreateValidationErrors(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.Error(t, repo.MarkConversationRead(context.Background(), "", "user-1"))
	require.Error(t, repo.MarkConversationUnread(context.Background(), "", "user-1"))
}

func TestRound07_ConversationRepository_CreateConversation_EmptyParticipantsAndLookupKeyError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Once()                            // main conversation record
	mockQuery.On("IfNotExists").Return(mockQuery).Once()                 // conditional lookup key create
	mockQuery.On("Create").Return(stdErrors.New("lookup-failed")).Once() // lookup key

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	conv := &models.Conversation{}
	require.Error(t, repo.CreateConversation(context.Background(), conv, nil))
}

func TestRound07_ConversationRepository_GetConversationByParticipants_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Twice()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetConversationByParticipants(context.Background(), []string{"user-1", "user-2"})
	require.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(stdErrors.New("query-failed")).Once()
	_, err = repo.GetConversationByParticipants(context.Background(), []string{"user-1", "user-2"})
	require.Error(t, err)
}

func TestRound07_ConversationRepository_GetConversationByParticipants_CanonicalizesMixedCaseParticipants(t *testing.T) {
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationParticipantKey")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

	lookupQuery.On("WithContext", mock.Anything).Return(lookupQuery).Once()
	lookupQuery.On("ConsistentRead").Return(lookupQuery).Once()
	lookupQuery.On("Where", "PK", "=", "CONVERSATION_PARTICIPANTS#arch,medic").Return(lookupQuery).Once()
	lookupQuery.On("Where", "SK", "=", "LOOKUP").Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.ConversationParticipantKey")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.ConversationParticipantKey)
		record.ConversationID = "conv-1"
	}).Return(nil).Once()

	conversationQuery.On("WithContext", mock.Anything).Return(conversationQuery).Once()
	conversationQuery.On("ConsistentRead").Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		conv := args.Get(0).(*models.Conversation)
		conv.ID = "conv-1"
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	conv, err := repo.GetConversationByParticipants(context.Background(), []string{"Medic", "Arch"})
	require.NoError(t, err)
	require.NotNil(t, conv)
	require.Equal(t, "conv-1", conv.ID)

	lookupQuery.AssertExpectations(t)
	conversationQuery.AssertExpectations(t)
}

func TestRound07_ConversationRepository_GetConversationByParticipants_ConversationLoadErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("conversation metadata not found after lookup row returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.AnythingOfType("*models.ConversationParticipantKey")).Return(lookupQuery).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

		lookupQuery.On("WithContext", mock.Anything).Return(lookupQuery).Once()
		lookupQuery.On("ConsistentRead").Return(lookupQuery).Once()
		lookupQuery.On("Where", "PK", "=", "CONVERSATION_PARTICIPANTS#arch,medic").Return(lookupQuery).Once()
		lookupQuery.On("Where", "SK", "=", "LOOKUP").Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.ConversationParticipantKey")).Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.ConversationParticipantKey)
			record.ConversationID = "conv-missing"
		}).Return(nil).Once()

		conversationQuery.On("WithContext", mock.Anything).Return(conversationQuery).Once()
		conversationQuery.On("ConsistentRead").Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-missing").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Return(ddbErrors.ErrItemNotFound).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		conv, err := repo.GetConversationByParticipants(ctx, []string{"Medic", "Arch"})
		require.Nil(t, conv)
		require.Error(t, err)
	})

	t.Run("conversation metadata query failure returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.AnythingOfType("*models.ConversationParticipantKey")).Return(lookupQuery).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

		lookupQuery.On("WithContext", mock.Anything).Return(lookupQuery).Once()
		lookupQuery.On("ConsistentRead").Return(lookupQuery).Once()
		lookupQuery.On("Where", "PK", "=", "CONVERSATION_PARTICIPANTS#arch,medic").Return(lookupQuery).Once()
		lookupQuery.On("Where", "SK", "=", "LOOKUP").Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.ConversationParticipantKey")).Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.ConversationParticipantKey)
			record.ConversationID = "conv-error"
		}).Return(nil).Once()

		conversationQuery.On("WithContext", mock.Anything).Return(conversationQuery).Once()
		conversationQuery.On("ConsistentRead").Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-error").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Return(stdErrors.New("conversation-load-failed")).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		conv, err := repo.GetConversationByParticipants(ctx, []string{"Medic", "Arch"})
		require.Nil(t, conv)
		require.Error(t, err)
	})
}

func TestRound07_ConversationRepository_RemoveStatusFromConversation_DeleteAndConvGetErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.ConversationMessage)
		*ptr = []models.ConversationMessage{{StatusID: "status-1"}}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(stdErrors.New("delete-failed")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.RemoveStatusFromConversation(context.Background(), "conv-1", "status-1"))

	// GetConversation error after deleting message -> returns nil.
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.ConversationMessage)
		*ptr = []models.ConversationMessage{{StatusID: "status-1"}}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Once()
	require.NoError(t, repo.RemoveStatusFromConversation(context.Background(), "conv-1", "status-1"))
}

func TestRound07_ConversationRepository_MuteCRUD_ErrorBranchesAndCleanup(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	require.Error(t, repo.CreateConversationMute(context.Background(), &storage.ConversationMute{ConversationID: "conv-1"}))

	mockQuery.On("Create").Return(stdErrors.New("create-failed")).Once()
	require.Error(t, repo.CreateConversationMute(context.Background(), &storage.ConversationMute{Username: "user-1", ConversationID: "conv-1"}))

	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()
	muted, err := repo.IsConversationMuted(context.Background(), "user-1", "conv-1")
	require.NoError(t, err)
	require.False(t, muted)

	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Once()
	_, err = repo.IsConversationMuted(context.Background(), "user-1", "conv-1")
	require.Error(t, err)

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.ConversationMute)
		*ptr = []models.ConversationMute{
			{Username: "user-1", ConversationID: "expired", ExpiresAt: time.Now().Add(-time.Minute)},
			{Username: "user-1", ConversationID: "active"},
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Maybe()
	ids, err := repo.GetMutedConversations(context.Background(), "user-1")
	require.NoError(t, err)
	require.Equal(t, []string{"active"}, ids)

	mockQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan-failed")).Once()
	_, err = repo.GetMutedConversations(context.Background(), "user-1")
	require.Error(t, err)
}

func TestRound07_ConversationRepository_GetUnreadMessageCount_WarnsAndContinues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	inboxQuery := new(mocks.MockQuery)
	conversationQuery1 := new(mocks.MockQuery)
	conversationQuery2 := new(mocks.MockQuery)
	pointStateQuery1 := new(mocks.MockQuery)
	pointStateQuery2 := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(inboxQuery).Once()
	inboxQuery.On("Index", "gsi1").Return(inboxQuery).Once()
	inboxQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#user-1#INBOX").Return(inboxQuery).Once()
	inboxQuery.On("OrderBy", "gsi1SK", "DESC").Return(inboxQuery).Once()
	inboxQuery.On("Limit", 101).Return(inboxQuery).Once()
	inboxQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]*models.UserConversationState)
		*ptr = []*models.UserConversationState{
			{ViewerID: "user-1", ConversationID: "conv-1", Folder: models.UserConversationFolderInbox, SortAt: time.Unix(2, 0).UTC()},
			{ViewerID: "user-1", ConversationID: "conv-2", Folder: models.UserConversationFolderInbox, SortAt: time.Unix(1, 0).UTC()},
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery1).Once()
	conversationQuery1.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery1).Once()
	conversationQuery1.On("Where", "SK", "=", "METADATA").Return(conversationQuery1).Once()
	conversationQuery1.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*models.Conversation)
		*ptr = models.Conversation{ID: "conv-1"}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery2).Once()
	conversationQuery2.On("Where", "PK", "=", "CONVERSATION#conv-2").Return(conversationQuery2).Once()
	conversationQuery2.On("Where", "SK", "=", "METADATA").Return(conversationQuery2).Once()
	conversationQuery2.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*models.Conversation)
		*ptr = models.Conversation{ID: "conv-2"}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(pointStateQuery1).Once()
	pointStateQuery1.On("Where", "PK", "=", "USER_CONVERSATION_STATE#user-1").Return(pointStateQuery1).Once()
	pointStateQuery1.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(pointStateQuery1).Once()
	pointStateQuery1.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(stdErrors.New("state-failed")).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(pointStateQuery2).Once()
	pointStateQuery2.On("Where", "PK", "=", "USER_CONVERSATION_STATE#user-1").Return(pointStateQuery2).Once()
	pointStateQuery2.On("Where", "SK", "=", "CONVERSATION#conv-2").Return(pointStateQuery2).Once()
	pointStateQuery2.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*models.UserConversationState)
		*ptr = models.UserConversationState{
			ViewerID:       "user-1",
			ConversationID: "conv-2",
			CounterpartID:  "user-2",
			Folder:         models.UserConversationFolderInbox,
			Unread:         false,
			SortAt:         time.Unix(1, 0).UTC(),
			CreatedAt:      time.Unix(1, 0).UTC(),
			UpdatedAt:      time.Unix(1, 0).UTC(),
		}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetUnreadMessageCount(context.Background(), "user-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestRound07_ConversationRepository_MessageTimeQueries_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan-failed")).Once()
	_, err := repo.countMessagesAfterTime(context.Background(), "conv-1", time.Unix(1, 0).UTC())
	require.Error(t, err)

	mockQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan-failed")).Once()
	_, err = repo.GetConversationMessagesByTimeRange(context.Background(), "conv-1", time.Unix(1, 0).UTC(), time.Unix(2, 0).UTC(), 0)
	require.Error(t, err)
}

func TestRound07_ConversationRepository_DeleteConversation_CleanupWarnings(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		conv := args.Get(0).(*models.Conversation)
		conv.ID = "conv-1"
		conv.Participants = []string{"user-1"}
		conv.UpdatedAt = time.Unix(10, 0).UTC()
		_ = conv.UpdateKeys()
	}).Return(nil).Once()

	// Main delete succeeds; cleanup deletes can fail without failing the operation.
	mockQuery.On("Delete").Return(nil).Once()
	mockQuery.On("Delete").Return(stdErrors.New("cleanup-delete-failed")).Maybe()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.DeleteConversation(context.Background(), "conv-1"))
}

func TestRound07_ConversationRepository_MarkConversationUnread_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#user-1").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*models.UserConversationState)
		*ptr = models.UserConversationState{
			ViewerID:       "user-1",
			ConversationID: "conv-1",
			CounterpartID:  "user-2",
			Folder:         models.UserConversationFolderInbox,
			SortAt:         time.Now().UTC(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
	}).Return(nil).Once()
	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil && state.ViewerID == "user-1" && state.ConversationID == "conv-1"
	})).Return(updateQuery).Once()
	updateQuery.On("Update", mock.Anything).Return(stdErrors.New("update-failed")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.MarkConversationUnread(context.Background(), "conv-1", "user-1"))
}

func TestRound07_ConversationRepository_DeleteConversationMute_DeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Delete").Return(stdErrors.New("delete-failed")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.DeleteConversationMute(context.Background(), "user-1", "conv-1"))
}

func TestRound07_ConversationRepository_GetConversationParticipants_UpdateLastStatus_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Maybe()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetConversationParticipants(context.Background(), "conv-1")
	require.Error(t, err)
	require.Error(t, repo.UpdateConversationLastStatus(context.Background(), "conv-1", "status-1"))
}

func TestRound07_ConversationRepository_GetConversationMessageCount_Cached(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetConversationMessageCount(context.Background(), "conv-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestRound07_ConversationRepository_AddStatusToConversation_ErrorPaths(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.Error(t, repo.AddStatusToConversation(context.Background(), "", "status-1", "user-1"))

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Once() // message create
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-conversation-failed")).Once()

	repo = NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.AddStatusToConversation(context.Background(), "conv-1", "status-1", "user-1"))
}
