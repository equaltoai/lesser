package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound34_ConversationRepository_RequestStateHelpers(t *testing.T) {
	t.Run("clampListLimit", func(t *testing.T) {
		require.Equal(t, 20, clampListLimit(0, 20, 100))
		require.Equal(t, 20, clampListLimit(-1, 20, 100))
		require.Equal(t, 100, clampListLimit(101, 20, 100))
		require.Equal(t, 50, clampListLimit(50, 20, 100))
	})

	t.Run("folderFromRequestState", func(t *testing.T) {
		require.Equal(t, models.UserConversationFolderInbox, folderFromRequestState(models.DmRequestStateAccepted))
		require.Equal(t, models.UserConversationFolderRequests, folderFromRequestState(models.DmRequestStatePending))
		require.Equal(t, models.UserConversationFolderDeclined, folderFromRequestState(models.DmRequestStateDeclined))
		require.Equal(t, models.UserConversationFolderInbox, folderFromRequestState(""))
	})

	t.Run("projection helpers preserve canonical state and clone conversation payloads", func(t *testing.T) {
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
		conversationUpdatedAt := sortAt.Add(2 * time.Hour)
		lastReadAt := conversationTimePtr(sortAt.Add(-30 * time.Minute))
		state := &models.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-1",
			CounterpartID:   "bob",
			Folder:          models.UserConversationFolderInbox,
			RequestState:    models.DmRequestStateAccepted,
			PreviewStatusID: "status-1",
			Unread:          true,
			LastReadAt:      lastReadAt,
			SortAt:          sortAt,
			CreatedAt:       sortAt.Add(-time.Hour),
			UpdatedAt:       sortAt,
		}
		conversation := &models.Conversation{
			ID:           "conv-1",
			Participants: []string{"alice", "bob"},
			UpdatedAt:    conversationUpdatedAt,
		}

		require.Nil(t, stateContractFromModel(nil))
		contract := stateContractFromModel(state)
		require.Equal(t, "alice", contract.ViewerID)
		require.Equal(t, "conv-1", contract.ConversationID)
		require.True(t, contract.Unread)
		require.Equal(t, lastReadAt.UTC(), *contract.LastReadAt)

		require.Nil(t, cloneConversationForViewer(nil, nil))
		cloned := cloneConversationForViewer(conversation, state)
		require.NotNil(t, cloned)
		require.True(t, cloned.Unread)
		require.NotNil(t, cloned.ViewerState)
		require.Equal(t, "status-1", cloned.ViewerState.PreviewStatusID)
		cloned.Participants[0] = "mutated"
		require.Equal(t, "alice", conversation.Participants[0])
	})

	t.Run("defaultUserConversationState initializes hidden rows without conversation metadata", func(t *testing.T) {
		state := defaultUserConversationState(nil, "alice")
		require.NotNil(t, state)
		require.Equal(t, "alice", state.ViewerID)
		require.Equal(t, models.UserConversationFolderHidden, state.Folder)
		require.Empty(t, state.ConversationID)
		require.False(t, state.SortAt.IsZero())
		require.False(t, state.CreatedAt.IsZero())
		require.False(t, state.UpdatedAt.IsZero())
	})
}

func TestRound34_ConversationRepository_ListUserConversationStatesByFolderModels(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 3).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<", "cursor").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-1", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)},
			{ViewerID: "alice", ConversationID: "conv-2", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC)},
			{ViewerID: "alice", ConversationID: "conv-3", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)},
		}
	}).Return(nil).Once()

	states, nextCursor, hasMore, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderInbox, interfaces.PaginationOptions{
		Limit:  2,
		Cursor: "cursor",
	})
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.True(t, hasMore)
	require.Equal(t, states[1].LegacyListCursor(), nextCursor)
}

func TestRound34_ConversationRepository_ListUserConversationStatesByFolderModels_MultiPageWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	firstQuery := new(mocks.MockQuery)
	secondQuery := new(mocks.MockQuery)
	newest := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	older := newest.Add(-time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(firstQuery).Once()
	firstQuery.On("Index", "gsi1").Return(firstQuery).Once()
	firstQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(firstQuery).Once()
	firstQuery.On("OrderBy", "gsi1SK", "DESC").Return(firstQuery).Once()
	firstQuery.On("Limit", 2).Return(firstQuery).Once()
	firstQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-new", Folder: models.UserConversationFolderInbox, SortAt: newest},
			{ViewerID: "alice", ConversationID: "conv-old", Folder: models.UserConversationFolderInbox, SortAt: older},
		}
	}).Return(nil).Once()

	firstCursor := (&models.UserConversationState{SortAt: newest, ConversationID: "conv-new"}).LegacyListCursor()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(secondQuery).Once()
	secondQuery.On("Index", "gsi1").Return(secondQuery).Once()
	secondQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(secondQuery).Once()
	secondQuery.On("OrderBy", "gsi1SK", "DESC").Return(secondQuery).Once()
	secondQuery.On("Limit", 2).Return(secondQuery).Once()
	secondQuery.On("Where", "gsi1SK", "<", firstCursor).Return(secondQuery).Once()
	secondQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-old", Folder: models.UserConversationFolderInbox, SortAt: older},
		}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	firstPage, cursor, hasMore, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderInbox, interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, firstCursor, cursor)
	require.Equal(t, []string{"conv-new"}, []string{firstPage[0].ConversationID})

	secondPage, nextCursor, hasMore, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderInbox, interfaces.PaginationOptions{Limit: 1, Cursor: cursor})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Empty(t, nextCursor)
	require.Equal(t, []string{"conv-new", "conv-old"}, []string{firstPage[0].ConversationID, secondPage[0].ConversationID})

	mockDB.AssertExpectations(t)
	firstQuery.AssertExpectations(t)
	secondQuery.AssertExpectations(t)
}

func TestRound34_ConversationRepository_ListUserConversationStatesByFolderModels_MissingAndError(t *testing.T) {
	t.Run("missing folder rows returns an empty page", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 21).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

		states, nextCursor, hasMore, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderRequests, interfaces.PaginationOptions{})
		require.NoError(t, err)
		require.Empty(t, states)
		require.Empty(t, nextCursor)
		require.False(t, hasMore)
	})

	t.Run("unexpected query errors are wrapped", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 21).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Return(stdErrors.New("boom")).Once()

		_, _, _, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderInbox, interfaces.PaginationOptions{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "user conversation state by folder")
	})
}

func TestRound34_ConversationRepository_UserConversationStateWrappers(t *testing.T) {
	t.Run("GetUserConversationState returns a contract", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.UserConversationState)
			*dest = models.UserConversationState{
				ViewerID:       "alice",
				ConversationID: "conv-1",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
				CreatedAt:      time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		state, err := repo.GetUserConversationState(ctx, "Alice", "conv-1")
		require.NoError(t, err)
		require.NotNil(t, state)
		require.Equal(t, "alice", state.ViewerID)
		require.Equal(t, "conv-1", state.ConversationID)
		require.Equal(t, "bob", state.CounterpartID)
		require.Equal(t, models.UserConversationFolderInbox, state.Folder)
		require.True(t, state.Unread)
	})

	t.Run("ListUserConversationStatesByFolder returns projected contracts", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.UserConversationState)
			*dest = []*models.UserConversationState{{
				ViewerID:       "alice",
				ConversationID: "conv-1",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
				CreatedAt:      time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
			}}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.ListUserConversationStatesByFolder(ctx, "Alice", interfaces.UserConversationFolder(models.UserConversationFolderInbox), interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		require.Equal(t, "alice", result.Items[0].ViewerID)
		require.Equal(t, "conv-1", result.Items[0].ConversationID)
		require.True(t, result.Items[0].Unread)
	})

	t.Run("createOrUpdateUserConversationState covers create and update branches", func(t *testing.T) {
		t.Run("create branch", func(t *testing.T) {
			ctx := context.Background()
			mockDB := new(mocks.MockDB)
			loadQuery := new(mocks.MockQuery)
			createQuery := new(mocks.MockQuery)
			state := &models.UserConversationState{
				ViewerID:       "Alice",
				ConversationID: "conv-1",
				CounterpartID:  "Bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
			}

			mockDB.On("WithContext", ctx).Return(mockDB).Twice()
			mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
			loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
			loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(loadQuery).Once()
			loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

			mockDB.On("Model", mock.MatchedBy(func(candidate *models.UserConversationState) bool {
				return candidate != nil &&
					candidate.ViewerID == "alice" &&
					candidate.CounterpartID == "bob" &&
					candidate.PK == "USER_CONVERSATION_STATE#alice" &&
					candidate.SK == "CONVERSATION#conv-1"
			})).Return(createQuery).Once()
			createQuery.On("Create").Return(nil).Once()

			repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
			require.NoError(t, repo.createOrUpdateUserConversationState(ctx, state))
		})

		t.Run("update branch preserves CreatedAt", func(t *testing.T) {
			ctx := context.Background()
			mockDB := new(mocks.MockDB)
			loadQuery := new(mocks.MockQuery)
			updateQuery := new(mocks.MockQuery)
			existingCreatedAt := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
			state := &models.UserConversationState{
				ViewerID:       "alice",
				ConversationID: "conv-1",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
				CreatedAt:      time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC),
			}

			mockDB.On("WithContext", ctx).Return(mockDB).Twice()
			mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
			loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
			loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(loadQuery).Once()
			loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.UserConversationState)
				*dest = models.UserConversationState{
					ViewerID:       "alice",
					ConversationID: "conv-1",
					CounterpartID:  "bob",
					Folder:         models.UserConversationFolderInbox,
					CreatedAt:      existingCreatedAt,
					UpdatedAt:      existingCreatedAt,
					SortAt:         state.SortAt,
				}
			}).Return(nil).Once()

			mockDB.On("Model", mock.MatchedBy(func(candidate *models.UserConversationState) bool {
				return candidate != nil && candidate.CreatedAt.Equal(existingCreatedAt)
			})).Return(updateQuery).Once()
			updateQuery.On("Update", mock.Anything).Return(nil).Once()

			repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
			require.NoError(t, repo.createOrUpdateUserConversationState(ctx, state))
			require.Equal(t, existingCreatedAt, state.CreatedAt)
		})

		t.Run("GetUserConversationState returns storage not found sentinel", func(t *testing.T) {
			ctx := context.Background()
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB).Once()
			mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
			mockQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "=", "CONVERSATION#missing").Return(mockQuery).Once()
			mockQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
			state, err := repo.GetUserConversationState(ctx, "alice", "missing")
			require.Nil(t, state)
			require.ErrorIs(t, err, storage.ErrNotFound)
		})
	})
}

func TestRound34_ConversationRepository_GetUserConversationsByFolder_UsesFolderQuery(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	requestQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	conversation := &models.Conversation{
		ID:           "conv-1",
		Participants: []string{"alice", "bob"},
		UpdatedAt:    time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(requestQuery).Once()
	requestQuery.On("Index", "gsi1").Return(requestQuery).Once()
	requestQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(requestQuery).Once()
	requestQuery.On("OrderBy", "gsi1SK", "DESC").Return(requestQuery).Once()
	requestQuery.On("Limit", 2).Return(requestQuery).Once()
	requestQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{{
			ViewerID:       "alice",
			ConversationID: "conv-1",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderRequests,
			SortAt:         conversation.UpdatedAt,
			Unread:         true,
		}}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Conversation)
		*dest = *conversation
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUserConversationsByFolder(ctx, "alice", models.UserConversationFolderRequests, interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "conv-1", result.Items[0].ID)
	require.True(t, result.Items[0].Unread)
	require.NotNil(t, result.Items[0].ViewerState)
	require.Equal(t, models.UserConversationFolderRequests, result.Items[0].ViewerState.Folder)
	require.Equal(t, "bob", result.Items[0].ViewerState.CounterpartID)
}

func TestRound34_ConversationRepository_GetUserConversationsByFolder_PropagatesFolderQueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	requestQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(requestQuery).Once()
	requestQuery.On("Index", "gsi1").Return(requestQuery).Once()
	requestQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(requestQuery).Once()
	requestQuery.On("OrderBy", "gsi1SK", "DESC").Return(requestQuery).Once()
	requestQuery.On("Limit", 21).Return(requestQuery).Once()
	requestQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Return(stdErrors.New("boom")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUserConversationsByFolder(ctx, "alice", models.UserConversationFolderRequests, interfaces.PaginationOptions{})
	require.Nil(t, result)
	require.Error(t, err)
}

func TestRound34_ConversationRepository_GetUserConversationsByRequestState_MapsRequestStateToFolderQuery(t *testing.T) {
	t.Run("pending uses requests folder", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		requestQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Times(2)
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(requestQuery).Once()
		requestQuery.On("Index", "gsi1").Return(requestQuery).Once()
		requestQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(requestQuery).Once()
		requestQuery.On("OrderBy", "gsi1SK", "DESC").Return(requestQuery).Once()
		requestQuery.On("Limit", 2).Return(requestQuery).Once()
		requestQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.UserConversationState)
			*dest = []*models.UserConversationState{{
				ViewerID:       "alice",
				ConversationID: "conv-pending",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderRequests,
				RequestState:   models.DmRequestStatePending,
				SortAt:         sortAt,
			}}
		}).Return(nil).Once()

		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-pending").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Conversation)
			*dest = models.Conversation{
				ID:           "conv-pending",
				Participants: []string{"alice", "bob"},
				UpdatedAt:    sortAt,
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.GetUserConversationsByRequestState(ctx, "alice", models.DmRequestStatePending, interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		require.Equal(t, "conv-pending", result.Items[0].ID)
	})

	t.Run("accepted defaults to inbox folder", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		inboxQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Times(2)
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(inboxQuery).Once()
		inboxQuery.On("Index", "gsi1").Return(inboxQuery).Once()
		inboxQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(inboxQuery).Once()
		inboxQuery.On("OrderBy", "gsi1SK", "DESC").Return(inboxQuery).Once()
		inboxQuery.On("Limit", 2).Return(inboxQuery).Once()
		inboxQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.UserConversationState)
			*dest = []*models.UserConversationState{{
				ViewerID:       "alice",
				ConversationID: "conv-accepted",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				RequestState:   models.DmRequestStateAccepted,
				SortAt:         sortAt,
			}}
		}).Return(nil).Once()

		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-accepted").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Conversation)
			*dest = models.Conversation{
				ID:           "conv-accepted",
				Participants: []string{"alice", "bob"},
				UpdatedAt:    sortAt,
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.GetUserConversationsByRequestState(ctx, "alice", models.DmRequestStateAccepted, interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		require.Equal(t, "conv-accepted", result.Items[0].ID)
	})

	t.Run("declined uses declined folder", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		declinedQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Times(2)
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(declinedQuery).Once()
		declinedQuery.On("Index", "gsi1").Return(declinedQuery).Once()
		declinedQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#DECLINED").Return(declinedQuery).Once()
		declinedQuery.On("OrderBy", "gsi1SK", "DESC").Return(declinedQuery).Once()
		declinedQuery.On("Limit", 2).Return(declinedQuery).Once()
		declinedQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.UserConversationState)
			*dest = []*models.UserConversationState{{
				ViewerID:       "alice",
				ConversationID: "conv-declined",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderDeclined,
				RequestState:   models.DmRequestStateDeclined,
				SortAt:         sortAt,
			}}
		}).Return(nil).Once()

		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-declined").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Conversation)
			*dest = models.Conversation{
				ID:           "conv-declined",
				Participants: []string{"alice", "bob"},
				UpdatedAt:    sortAt,
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.GetUserConversationsByRequestState(ctx, "alice", models.DmRequestStateDeclined, interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		require.Equal(t, "conv-declined", result.Items[0].ID)
	})
}

func TestRound34_ConversationRepository_GetUserConversations_UsesInboxFolderQuery(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	inboxQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(2)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(inboxQuery).Once()
	inboxQuery.On("Index", "gsi1").Return(inboxQuery).Once()
	inboxQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(inboxQuery).Once()
	inboxQuery.On("OrderBy", "gsi1SK", "DESC").Return(inboxQuery).Once()
	inboxQuery.On("Limit", 2).Return(inboxQuery).Once()
	inboxQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-2", CounterpartID: "bob", Folder: models.UserConversationFolderInbox, SortAt: sortAt, Unread: true},
			{ViewerID: "alice", ConversationID: "conv-3", CounterpartID: "cara", Folder: models.UserConversationFolderInbox, SortAt: sortAt.Add(-time.Hour), Unread: true},
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-2").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Conversation)
		*dest = models.Conversation{
			ID:           "conv-2",
			Participants: []string{"alice", "bob"},
			UpdatedAt:    sortAt,
		}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUserConversations(ctx, "Alice", interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "conv-2", result.Items[0].ID)
	require.True(t, result.Items[0].Unread)
	require.True(t, result.HasMore)
	require.Equal(t, (&models.UserConversationState{SortAt: sortAt, ConversationID: "conv-2"}).LegacyListCursor(), result.NextCursor)
}

func TestRound34_ConversationRepository_LoadConversationsForStates_SkipsNilAndMissing(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	foundQuery := new(mocks.MockQuery)
	missingQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(foundQuery).Once()
	foundQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(foundQuery).Once()
	foundQuery.On("Where", "SK", "=", "METADATA").Return(foundQuery).Once()
	foundQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Conversation)
		*dest = models.Conversation{
			ID:           "conv-1",
			Participants: []string{"alice", "bob"},
			UpdatedAt:    sortAt,
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(missingQuery).Once()
	missingQuery.On("Where", "PK", "=", "CONVERSATION#missing").Return(missingQuery).Once()
	missingQuery.On("Where", "SK", "=", "METADATA").Return(missingQuery).Once()
	missingQuery.On("First", mock.AnythingOfType("*models.Conversation")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	items, err := repo.loadConversationsForStates(ctx, []*models.UserConversationState{
		nil,
		{ConversationID: "conv-1", Unread: true},
		{ConversationID: "missing", Unread: false},
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "conv-1", items[0].ID)
	require.True(t, items[0].Unread)
}

func TestRound34_ConversationRepository_ListUnreadUserConversationStates(t *testing.T) {
	t.Run("returns sparse unread rows with pagination", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2SK", "<", "cursor").Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.UserConversationState)
			*dest = []*models.UserConversationState{
				{ViewerID: "alice", ConversationID: "conv-1", CounterpartID: "bob", Folder: models.UserConversationFolderInbox, Unread: true, SortAt: sortAt},
				{ViewerID: "alice", ConversationID: "conv-2", CounterpartID: "cara", Folder: models.UserConversationFolderRequests, Unread: true, SortAt: sortAt.Add(-time.Hour)},
				{ViewerID: "alice", ConversationID: "conv-3", CounterpartID: "dave", Folder: models.UserConversationFolderInbox, Unread: true, SortAt: sortAt.Add(-2 * time.Hour)},
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.ListUnreadUserConversationStates(ctx, "Alice", interfaces.PaginationOptions{Limit: 2, Cursor: "cursor"})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.True(t, result.HasMore)
		require.Equal(t, (&models.UserConversationState{SortAt: sortAt.Add(-time.Hour), ConversationID: "conv-2"}).LegacyListCursor(), result.NextCursor)
		require.Equal(t, "alice", result.Items[0].ViewerID)
		require.True(t, result.Items[0].Unread)
	})

	t.Run("treats missing unread rows as empty result", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 21).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		result, err := repo.ListUnreadUserConversationStates(ctx, "alice", interfaces.PaginationOptions{})
		require.NoError(t, err)
		require.Empty(t, result.Items)
		require.EqualValues(t, 0, result.Total)
		require.False(t, result.HasMore)
	})
}

func TestRound34_ConversationRepository_GetUnreadConversations_ProjectsCanonicalUnreadState(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	unreadQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(unreadQuery).Once()
	unreadQuery.On("Index", "gsi2").Return(unreadQuery).Once()
	unreadQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(unreadQuery).Once()
	unreadQuery.On("OrderBy", "gsi2SK", "DESC").Return(unreadQuery).Once()
	unreadQuery.On("Limit", 2).Return(unreadQuery).Once()
	unreadQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{{
			ViewerID:        "alice",
			ConversationID:  "conv-8",
			CounterpartID:   "bob",
			Folder:          models.UserConversationFolderInbox,
			RequestState:    models.DmRequestStateAccepted,
			PreviewStatusID: "status-preview",
			Unread:          true,
			SortAt:          sortAt,
			CreatedAt:       sortAt.Add(-time.Hour),
			UpdatedAt:       sortAt,
		}}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-8").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Conversation)
		*dest = models.Conversation{
			ID:           "conv-8",
			Participants: []string{"alice", "bob"},
			UpdatedAt:    sortAt,
		}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUnreadConversations(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "conv-8", result.Items[0].ID)
	require.True(t, result.Items[0].Unread)
	require.NotNil(t, result.Items[0].ViewerState)
	require.Equal(t, "status-preview", result.Items[0].ViewerState.PreviewStatusID)
	require.Equal(t, "bob", result.Items[0].ViewerState.CounterpartID)
}

func TestRound34_ConversationRepository_GetUnreadConversations_WrapsConversationLoadError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	unreadQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(unreadQuery).Once()
	unreadQuery.On("Index", "gsi2").Return(unreadQuery).Once()
	unreadQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(unreadQuery).Once()
	unreadQuery.On("OrderBy", "gsi2SK", "DESC").Return(unreadQuery).Once()
	unreadQuery.On("Limit", 2).Return(unreadQuery).Once()
	unreadQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{{
			ViewerID:       "alice",
			ConversationID: "conv-broken",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			SortAt:         sortAt,
		}}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-broken").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Return(stdErrors.New("boom")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUnreadConversations(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
	require.Nil(t, result)
	require.Error(t, err)
}

func TestRound34_ConversationRepository_GetUnreadConversations_PropagatesUnreadStateQueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	unreadQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(unreadQuery).Once()
	unreadQuery.On("Index", "gsi2").Return(unreadQuery).Once()
	unreadQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(unreadQuery).Once()
	unreadQuery.On("OrderBy", "gsi2SK", "DESC").Return(unreadQuery).Once()
	unreadQuery.On("Limit", 2).Return(unreadQuery).Once()
	unreadQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Return(stdErrors.New("boom")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUnreadConversations(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
	require.Nil(t, result)
	require.Error(t, err)
}

func TestRound34_ConversationRepository_GetUnreadConversationCount_PaginatesUnreadStatePages(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	firstQuery := new(mocks.MockQuery)
	secondQuery := new(mocks.MockQuery)
	baseTime := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	page1 := make([]*models.UserConversationState, 0, 101)
	for i := 0; i < 101; i++ {
		page1 = append(page1, &models.UserConversationState{
			ViewerID:       "alice",
			ConversationID: "conv-page-1-" + time.Duration(i).String(),
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			SortAt:         baseTime.Add(-time.Duration(i) * time.Minute),
		})
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(firstQuery).Once()
	firstQuery.On("Index", "gsi2").Return(firstQuery).Once()
	firstQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(firstQuery).Once()
	firstQuery.On("OrderBy", "gsi2SK", "DESC").Return(firstQuery).Once()
	firstQuery.On("Limit", 101).Return(firstQuery).Once()
	firstQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = page1
	}).Return(nil).Once()

	expectedCursor := page1[99].LegacyListCursor()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(secondQuery).Once()
	secondQuery.On("Index", "gsi2").Return(secondQuery).Once()
	secondQuery.On("Where", "gsi2PK", "=", "USER_CONVERSATION_UNREAD#alice").Return(secondQuery).Once()
	secondQuery.On("OrderBy", "gsi2SK", "DESC").Return(secondQuery).Once()
	secondQuery.On("Limit", 101).Return(secondQuery).Once()
	secondQuery.On("Where", "gsi2SK", "<", expectedCursor).Return(secondQuery).Once()
	secondQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{{
			ViewerID:       "alice",
			ConversationID: "conv-page-2",
			CounterpartID:  "cara",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			SortAt:         baseTime.Add(-200 * time.Minute),
		}}
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetUnreadConversationCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 101, count)
}

func conversationTimePtr(t time.Time) *time.Time {
	return &t
}
