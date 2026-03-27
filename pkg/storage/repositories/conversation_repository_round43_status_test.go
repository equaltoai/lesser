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

func TestRound43_ConversationRepository_GetConversationParticipants_LoadsMetadataParticipants(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	conversationQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		conv := args.Get(0).(*models.Conversation)
		conv.ID = "conv-1"
		conv.Participants = []string{"alice", "bob"}
		conv.UpdatedAt = time.Date(2026, 3, 25, 10, 39, 9, 829133328, time.UTC)
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	participants, err := repo.GetConversationParticipants(ctx, "conv-1")
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "bob"}, participants)
}
