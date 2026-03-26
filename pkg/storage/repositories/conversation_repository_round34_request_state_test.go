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
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
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

	t.Run("participantRecordFolder prefers explicit folder", func(t *testing.T) {
		require.Equal(t, models.UserConversationFolderHidden, participantRecordFolder(nil))
		require.Equal(t, models.UserConversationFolderRequests, participantRecordFolder(&models.ConversationParticipantRecord{
			Folder: models.UserConversationFolderRequests,
		}))
		require.Equal(t, models.UserConversationFolderHidden, participantRecordFolder(&models.ConversationParticipantRecord{
			DeletedAt: conversationTimePtr(time.Unix(1, 0).UTC()),
		}))
		require.Equal(t, models.UserConversationFolderDeclined, participantRecordFolder(&models.ConversationParticipantRecord{
			RequestState: models.DmRequestStateDeclined,
		}))
	})

	t.Run("mergeVisibleConversationStatePages orders by sort time", func(t *testing.T) {
		inbox := []*models.UserConversationState{{
			ConversationID: "conv-1",
			SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		}}
		requests := []*models.UserConversationState{{
			ConversationID: "conv-2",
			SortAt:         time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		}}

		merged, nextCursor, hasMore := mergeVisibleConversationStatePages(inbox, requests, 1)
		require.Len(t, merged, 1)
		require.Equal(t, "conv-1", merged[0].ConversationID)
		require.True(t, hasMore)
		require.Equal(t, merged[0].LegacyListCursor(), nextCursor)
	})

	t.Run("projection helpers preserve canonical state and clone conversation payloads", func(t *testing.T) {
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
		conversationUpdatedAt := sortAt.Add(2 * time.Hour)
		lastReadAt := conversationTimePtr(sortAt.Add(-30 * time.Minute))
		state := &models.UserConversationState{
			ViewerID:       "alice",
			ConversationID: "conv-1",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			RequestState:   models.DmRequestStateAccepted,
			Unread:         true,
			LastReadAt:     lastReadAt,
			SortAt:         sortAt,
			CreatedAt:      sortAt.Add(-time.Hour),
			UpdatedAt:      sortAt,
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

		require.Nil(t, stateModelFromContract(nil))
		model := stateModelFromContract(contract)
		require.NotNil(t, model)
		require.Equal(t, contract.ViewerID, model.ViewerID)
		require.Equal(t, contract.ConversationID, model.ConversationID)
		require.Equal(t, contract.CounterpartID, model.CounterpartID)
		require.Equal(t, contract.RequestState, model.RequestState)
		require.Equal(t, contract.SortAt, model.SortAt)
		require.NotNil(t, model.LastReadAt)
		require.Equal(t, *contract.LastReadAt, *model.LastReadAt)

		modelsFromContracts := stateModelsFromContracts([]*interfaces.UserConversationStateContract{nil, contract})
		require.Len(t, modelsFromContracts, 1)
		require.Equal(t, contract.ConversationID, modelsFromContracts[0].ConversationID)

		require.Nil(t, cloneConversationForViewer(nil, false))
		cloned := cloneConversationForViewer(conversation, true)
		require.NotNil(t, cloned)
		require.True(t, cloned.Unread)
		cloned.Participants[0] = "mutated"
		require.Equal(t, "alice", conversation.Participants[0])

		require.Nil(t, stateRecordFromModel(nil, conversation))
		record := stateRecordFromModel(state, conversation)
		require.Equal(t, "USER_CONVERSATIONS#alice", record.PK)
		require.Equal(t, "PARTICIPANT#alice", record.GSI1SK)
		require.Equal(t, sortAt.Format(time.RFC3339Nano)+"#conv-1", record.SK)
		require.Equal(t, sortAt, record.UpdatedAt)
		require.NotNil(t, record.Conversation)
		require.True(t, record.Conversation.Unread)
		require.Equal(t, conversationUpdatedAt, record.Conversation.UpdatedAt)
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

	t.Run("GetConversationParticipantRecord projects canonical state into legacy record shape", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		stateQuery := new(mocks.MockQuery)
		conversationQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(stateQuery).Once()
		stateQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(stateQuery).Once()
		stateQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(stateQuery).Once()
		stateQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.UserConversationState)
			*dest = models.UserConversationState{
				ViewerID:       "alice",
				ConversationID: "conv-1",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         sortAt,
				CreatedAt:      time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
				UpdatedAt:      sortAt,
			}
		}).Return(nil).Once()

		mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
		conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
		conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
		conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Conversation)
			*dest = models.Conversation{
				ID:           "conv-1",
				Participants: []string{"alice", "bob"},
				UpdatedAt:    sortAt,
			}
		}).Return(nil).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		record, err := repo.GetConversationParticipantRecord(ctx, "conv-1", "Alice")
		require.NoError(t, err)
		require.Equal(t, "USER_CONVERSATIONS#alice", record.PK)
		require.Equal(t, sortAt.Format(time.RFC3339Nano)+"#conv-1", record.SK)
		require.Equal(t, "PARTICIPANT#alice", record.GSI1SK)
		require.Equal(t, "alice", record.ViewerID)
		require.Equal(t, "conv-1", record.ConversationID)
		require.Equal(t, "bob", record.CounterpartID)
		require.NotNil(t, record.Conversation)
		require.True(t, record.Conversation.Unread)
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

func TestRound34_ConversationRepository_GetUserConversations_UsesMergedFolderQueries(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	inboxQuery := new(mocks.MockQuery)
	requestQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(3)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(inboxQuery).Once()
	inboxQuery.On("Index", "gsi1").Return(inboxQuery).Once()
	inboxQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(inboxQuery).Once()
	inboxQuery.On("OrderBy", "gsi1SK", "DESC").Return(inboxQuery).Once()
	inboxQuery.On("Limit", 2).Return(inboxQuery).Once()
	inboxQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(requestQuery).Once()
	requestQuery.On("Index", "gsi1").Return(requestQuery).Once()
	requestQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(requestQuery).Once()
	requestQuery.On("OrderBy", "gsi1SK", "DESC").Return(requestQuery).Once()
	requestQuery.On("Limit", 2).Return(requestQuery).Once()
	requestQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-2", CounterpartID: "bob", Folder: models.UserConversationFolderRequests, SortAt: sortAt, Unread: true},
			{ViewerID: "alice", ConversationID: "conv-3", CounterpartID: "cara", Folder: models.UserConversationFolderRequests, SortAt: sortAt.Add(-time.Hour), Unread: true},
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
	require.Equal(t, sortAt.Format(time.RFC3339Nano)+"#conv-2", result.NextCursor)
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
		require.Equal(t, sortAt.Add(-time.Hour).Format(time.RFC3339Nano)+"#conv-2", result.NextCursor)
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
			ViewerID:       "alice",
			ConversationID: "conv-8",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			SortAt:         sortAt,
			CreatedAt:      sortAt.Add(-time.Hour),
			UpdatedAt:      sortAt,
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
