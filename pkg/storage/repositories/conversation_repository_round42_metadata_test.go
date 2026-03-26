package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound42_ConversationRepository_UpdateConversationParticipantRecord_UpdatesCanonicalState(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)

	recordedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	record := &models.ConversationParticipantRecord{
		PK:             "USER_CONVERSATIONS#arch",
		SK:             "2026-03-25T12:00:00Z#conv-1",
		ViewerID:       "arch",
		ConversationID: "conv-1",
		CounterpartID:  "medic",
		Folder:         models.UserConversationFolderInbox,
		RequestState:   models.DmRequestStateAccepted,
		AcceptedAt:     &recordedAt,
		Unread:         true,
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER_CONVERSATION_STATE#arch").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", "CONVERSATION#conv-1").Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.UserConversationState")).Run(func(args mock.Arguments) {
		state := args.Get(0).(*models.UserConversationState)
		*state = models.UserConversationState{
			ViewerID:       "arch",
			ConversationID: "conv-1",
			CounterpartID:  "medic",
			Folder:         models.UserConversationFolderHidden,
			SortAt:         recordedAt,
			CreatedAt:      recordedAt,
			UpdatedAt:      recordedAt,
		}
	}).Return(nil).Once()

	mockDB.On("Model", mockDBMatchedUserConversationState("arch", "conv-1")).Return(updateQuery).Once()
	updateQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.UpdateConversationParticipantRecord(ctx, record)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	loadQuery.AssertExpectations(t)
	updateQuery.AssertExpectations(t)
}

func mockDBMatchedUserConversationState(viewerID, conversationID string) interface{} {
	return mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.ViewerID == viewerID &&
			state.ConversationID == conversationID
	})
}
