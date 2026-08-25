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
	ddbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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

	_, _ = repo.GetConversationParticipants(ctx, "conv-1")

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

func TestRound07_ConversationRepository_GetConversationByParticipantRefs_LoadsTypedLookup(t *testing.T) {
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)

	refs := []models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/bob", Acct: "bob@remote.example"},
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "Alice"},
	}
	normalized := models.NormalizeConversationParticipantRefs(refs)
	lookupPK := conversationParticipantLookupV2PK(normalized)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationParticipantKey")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

	lookupQuery.On("WithContext", mock.Anything).Return(lookupQuery).Once()
	lookupQuery.On("ConsistentRead").Return(lookupQuery).Once()
	lookupQuery.On("Where", "PK", "=", lookupPK).Return(lookupQuery).Once()
	lookupQuery.On("Where", "SK", "=", conversationParticipantLookupSK).Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.ConversationParticipantKey")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.ConversationParticipantKey)
		record.ConversationID = "conv-typed"
	}).Return(nil).Once()

	conversationQuery.On("WithContext", mock.Anything).Return(conversationQuery).Once()
	conversationQuery.On("ConsistentRead").Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-typed").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		conv := args.Get(0).(*models.Conversation)
		conv.ID = "conv-typed"
		conv.ParticipantRefs = normalized
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	conv, err := repo.GetConversationByParticipantRefs(context.Background(), refs)
	require.NoError(t, err)
	require.NotNil(t, conv)
	require.Equal(t, "conv-typed", conv.ID)
	require.Equal(t, normalized, conv.ParticipantRefs)

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

func TestRound07_ConversationRepository_MuteCRUD_ErrorBranchesAndCleanup(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	require.Error(t, repo.CreateConversationMute(context.Background(), &storage.ConversationMute{ConversationID: "conv-1"}))

	mockQuery.On("CreateOrUpdate").Return(stdErrors.New("create-failed")).Once()
	require.Error(t, repo.CreateConversationMute(context.Background(), &storage.ConversationMute{Username: "user-1", ConversationID: "conv-1"}))

	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()
	muted, err := repo.IsConversationMuted(context.Background(), "user-1", "conv-1")
	require.NoError(t, err)
	require.False(t, muted)

	mockQuery.On("First", mock.Anything).Return(stdErrors.New("get-failed")).Once()
	_, err = repo.IsConversationMuted(context.Background(), "user-1", "conv-1")
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
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

	mockQuery.On("All", mock.Anything).Return(stdErrors.New("query-failed")).Once()
	_, err = repo.GetMutedConversations(context.Background(), "user-1")
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

func TestRound07_ConversationRepository_GetConversationParticipants_ErrorBranch(t *testing.T) {
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
}
