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

func TestRound42_ConversationRepository_UpdateConversationParticipantRecord_UsesMetadataOnlyUpdate(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	recordedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	record := &models.ConversationParticipantRecord{
		PK:           "USER_CONVERSATIONS#arch",
		SK:           "2026-03-25T12:00:00Z#conv-1",
		RequestState: models.DmRequestStateAccepted,
		AcceptedAt:   &recordedAt,
		Unread:       true,
		ConversationData: &models.ConversationSnapshot{
			ID:           "",
			Participants: nil,
		},
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", record).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", record.PK).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", record.SK).Return(mockQuery).Once()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()

	mockUpdate.On("Set", "Unread", true).Return(mockUpdate).Once()
	mockUpdate.On("Set", "RequestState", models.DmRequestStateAccepted).Return(mockUpdate).Once()
	mockUpdate.On("Remove", "RequestedAt").Return(mockUpdate).Once()
	mockUpdate.On("Set", "AcceptedAt", recordedAt.UTC()).Return(mockUpdate).Once()
	mockUpdate.On("Remove", "DeclinedAt").Return(mockUpdate).Once()
	mockUpdate.On("Remove", "DeletedAt").Return(mockUpdate).Once()
	mockUpdate.On("Remove", "LastReadAt").Return(mockUpdate).Once()
	mockUpdate.On("Execute").Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.UpdateConversationParticipantRecord(ctx, record)
	require.NoError(t, err)

	mockUpdate.AssertNotCalled(t, "Set", "ConversationData", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "Conversation", mock.Anything)
}
