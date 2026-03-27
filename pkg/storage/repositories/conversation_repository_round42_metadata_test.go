package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound42_ConversationRepository_EnsureUserConversationStateModel_BootstrapsMissingState(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery1 := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	loadQuery2 := new(mocks.MockQuery)
	createQuery := new(mocks.MockQuery)

	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(4)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery1).Once()
	loadQuery1.On("Where", "PK", "=", "USER_CONVERSATION_STATE#arch").Return(loadQuery1).Once()
	loadQuery1.On("Where", "SK", "=", "CONVERSATION#conv-3").Return(loadQuery1).Once()
	loadQuery1.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-3").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		conversation := args.Get(0).(*models.Conversation)
		*conversation = models.Conversation{
			ID:              "conv-3",
			Participants:    []string{"arch", "medic"},
			LastStatusID:    "status-3",
			LastMessageTime: updatedAt,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery2).Once()
	loadQuery2.On("Where", "PK", "=", "USER_CONVERSATION_STATE#arch").Return(loadQuery2).Once()
	loadQuery2.On("Where", "SK", "=", "CONVERSATION#conv-3").Return(loadQuery2).Once()
	loadQuery2.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == "arch" &&
			state.ConversationID == "conv-3" &&
			state.CounterpartID == "medic" &&
			state.PreviewStatusID == "status-3" &&
			state.SortAt.Equal(updatedAt)
	})).Return(createQuery).Once()
	createQuery.On("Create").Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	state, err := repo.ensureUserConversationStateModel(ctx, "Arch", "conv-3")
	require.NoError(t, err)
	require.Equal(t, "arch", state.ViewerID)
	require.Equal(t, "conv-3", state.ConversationID)
	require.Equal(t, "medic", state.CounterpartID)
	require.Equal(t, models.UserConversationFolderHidden, state.Folder)
	require.Equal(t, "status-3", state.PreviewStatusID)
	require.Equal(t, updatedAt, state.SortAt)

	mockDB.AssertExpectations(t)
	loadQuery1.AssertExpectations(t)
	conversationQuery.AssertExpectations(t)
	loadQuery2.AssertExpectations(t)
	createQuery.AssertExpectations(t)
}

func TestRound42_ConversationRepository_EnsureUserConversationStateModel_PreservesNotFoundWhenConversationMissing(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#arch").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#missing").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#missing").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	state, err := repo.ensureUserConversationStateModel(ctx, "arch", "missing")
	require.Nil(t, state)
	require.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	loadQuery.AssertExpectations(t)
	conversationQuery.AssertExpectations(t)
}

func TestRound42_ConversationRepository_EnsureUserConversationStateModel_PropagatesLookupError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#arch").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-error").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(stdErrors.New("boom")).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	state, err := repo.ensureUserConversationStateModel(ctx, "arch", "conv-error")
	require.Nil(t, state)
	require.EqualError(t, err, "boom")
}

func TestRound42_ConversationRepository_CreateOrUpdateUserConversationState_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.ErrorIs(t, repo.createOrUpdateUserConversationState(ctx, nil), storage.ErrInvalidInput)

	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	state := &models.UserConversationState{
		ViewerID:       "alice",
		ConversationID: "conv-5",
		CounterpartID:  "bob",
		Folder:         models.UserConversationFolderInbox,
		SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-5").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(stdErrors.New("boom")).Once()

	repo = NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.createOrUpdateUserConversationState(ctx, state)
	require.EqualError(t, err, "boom")
}

func TestRound42_ConversationRepository_MarkConversationRead_UpdatesCanonicalStateOnly(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-6").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.UserConversationState)
		*dest = models.UserConversationState{
			ViewerID:       "alice",
			ConversationID: "conv-6",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			Unread:         true,
			SortAt:         sortAt,
			CreatedAt:      sortAt.Add(-time.Hour),
			UpdatedAt:      sortAt.Add(-time.Minute),
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == "alice" &&
			state.ConversationID == "conv-6" &&
			state.CounterpartID == "bob" &&
			!state.Unread &&
			state.LastReadAt != nil &&
			!state.LastReadAt.IsZero()
	})).Return(updateQuery).Once()
	updateQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.MarkConversationRead(ctx, "conv-6", "Alice"))
}

func TestRound42_ConversationRepository_MarkConversationRead_ErrorPaths(t *testing.T) {
	t.Run("rejects empty identifiers", func(t *testing.T) {
		repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		err := repo.MarkConversationRead(context.Background(), "", "alice")
		require.Error(t, err)
	})

	t.Run("returns an update error when the canonical state write fails", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		loadQuery := new(mocks.MockQuery)
		updateQuery := new(mocks.MockQuery)
		sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
		loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
		loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-6").Return(loadQuery).Once()
		loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.UserConversationState)
			*dest = models.UserConversationState{
				ViewerID:       "alice",
				ConversationID: "conv-6",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
				Unread:         true,
				SortAt:         sortAt,
				CreatedAt:      sortAt.Add(-time.Hour),
				UpdatedAt:      sortAt.Add(-time.Minute),
			}
		}).Return(nil).Once()

		mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
			return state != nil && state.ViewerID == "alice" && state.ConversationID == "conv-6"
		})).Return(updateQuery).Once()
		updateQuery.On("Update", mock.Anything).Return(stdErrors.New("boom")).Once()

		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
		err := repo.MarkConversationRead(ctx, "conv-6", "alice")
		require.Error(t, err)
	})
}

func TestRound42_ConversationRepository_MarkConversationUnread_UpdatesCanonicalStateOnly(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	sortAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-9").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.UserConversationState)
		*dest = models.UserConversationState{
			ViewerID:       "alice",
			ConversationID: "conv-9",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderInbox,
			Unread:         false,
			LastReadAt:     conversationTimePtr(sortAt.Add(-time.Minute)),
			SortAt:         sortAt,
			CreatedAt:      sortAt.Add(-time.Hour),
			UpdatedAt:      sortAt.Add(-time.Minute),
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == "alice" &&
			state.ConversationID == "conv-9" &&
			state.CounterpartID == "bob" &&
			state.Unread &&
			state.LastReadAt == nil
	})).Return(updateQuery).Once()
	updateQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.MarkConversationUnread(ctx, "conv-9", "Alice"))
}

func TestRound42_ConversationRepository_InitializeUserConversationStates_PreservesExistingStateAndBootstrapsMissingViewers(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadAliceExisting := new(mocks.MockQuery)
	loadAliceForUpdate := new(mocks.MockQuery)
	updateAlice := new(mocks.MockQuery)
	loadBobMissing := new(mocks.MockQuery)
	loadBobForCreate := new(mocks.MockQuery)
	createBob := new(mocks.MockQuery)
	existingCreatedAt := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	existingUpdatedAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	lastReadAt := time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC)
	requestedAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	conversation := &models.Conversation{
		ID:              "conv-7",
		Participants:    []string{"Alice", "Bob"},
		LastStatusID:    "status-7",
		LastMessageTime: updatedAt,
		CreatedAt:       existingCreatedAt,
		UpdatedAt:       updatedAt,
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Times(6)
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadAliceExisting).Once()
	loadAliceExisting.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadAliceExisting).Once()
	loadAliceExisting.On("Where", "SK", "=", "CONVERSATION#conv-7").Return(loadAliceExisting).Once()
	loadAliceExisting.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.UserConversationState)
		*dest = models.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-7",
			CounterpartID:   "bob",
			Folder:          models.UserConversationFolderRequests,
			RequestState:    models.DmRequestStatePending,
			Unread:          true,
			LastReadAt:      &lastReadAt,
			RequestedAt:     &requestedAt,
			PreviewStatusID: "legacy-status",
			SortAt:          existingUpdatedAt,
			CreatedAt:       existingCreatedAt,
			UpdatedAt:       existingUpdatedAt,
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadAliceForUpdate).Once()
	loadAliceForUpdate.On("Where", "PK", "=", "USER_CONVERSATION_STATE#alice").Return(loadAliceForUpdate).Once()
	loadAliceForUpdate.On("Where", "SK", "=", "CONVERSATION#conv-7").Return(loadAliceForUpdate).Once()
	loadAliceForUpdate.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.UserConversationState)
		*dest = models.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-7",
			CounterpartID:   "bob",
			Folder:          models.UserConversationFolderRequests,
			RequestState:    models.DmRequestStatePending,
			Unread:          true,
			LastReadAt:      &lastReadAt,
			RequestedAt:     &requestedAt,
			PreviewStatusID: "legacy-status",
			SortAt:          existingUpdatedAt,
			CreatedAt:       existingCreatedAt,
			UpdatedAt:       existingUpdatedAt,
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == "alice" &&
			state.ConversationID == "conv-7" &&
			state.CounterpartID == "bob" &&
			state.Folder == models.UserConversationFolderRequests &&
			state.RequestState == models.DmRequestStatePending &&
			state.Unread &&
			state.LastReadAt != nil &&
			state.RequestedAt != nil &&
			state.PreviewStatusID == "legacy-status" &&
			state.SortAt.Equal(existingUpdatedAt) &&
			state.CreatedAt.Equal(existingCreatedAt) &&
			state.UpdatedAt.Equal(updatedAt)
	})).Return(updateAlice).Once()
	updateAlice.On("Update", mock.Anything).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadBobMissing).Once()
	loadBobMissing.On("Where", "PK", "=", "USER_CONVERSATION_STATE#bob").Return(loadBobMissing).Once()
	loadBobMissing.On("Where", "SK", "=", "CONVERSATION#conv-7").Return(loadBobMissing).Once()
	loadBobMissing.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadBobForCreate).Once()
	loadBobForCreate.On("Where", "PK", "=", "USER_CONVERSATION_STATE#bob").Return(loadBobForCreate).Once()
	loadBobForCreate.On("Where", "SK", "=", "CONVERSATION#conv-7").Return(loadBobForCreate).Once()
	loadBobForCreate.On("First", mock.AnythingOfType("*models.UserConversationState")).Return(dynamormerrors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == "bob" &&
			state.ConversationID == "conv-7" &&
			state.CounterpartID == "alice" &&
			state.Folder == models.UserConversationFolderHidden &&
			state.PreviewStatusID == "status-7" &&
			state.SortAt.Equal(updatedAt) &&
			state.CreatedAt.Equal(existingCreatedAt) &&
			state.UpdatedAt.Equal(updatedAt)
	})).Return(createBob).Once()
	createBob.On("Create").Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.initializeUserConversationStates(ctx, conversation))
}

func mockDBMatchedUserConversationState(viewerID, conversationID string) interface{} {
	return mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == viewerID &&
			state.ConversationID == conversationID
	})
}
