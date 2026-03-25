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

func TestRound43_ConversationRepository_UpdateConversationLastStatus_UsesCanonicalStatusMetadata(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	statusQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	publishedAt := time.Date(2026, 3, 25, 10, 39, 9, 829133328, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(statusQuery).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()

	statusQuery.On("Where", "PK", "=", "status#status-1").Return(statusQuery).Once()
	statusQuery.On("Where", "SK", "=", "status#status-1").Return(statusQuery).Once()
	statusQuery.On("First", mock.AnythingOfType("*models.Status")).Run(func(args mock.Arguments) {
		status := args.Get(0).(*models.Status)
		status.StatusID = "status-1"
		status.ConversationID = "conv-1"
		status.PublishedAt = publishedAt
	}).Return(nil).Once()

	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("UpdateBuilder").Return(updateBuilder).Once()

	updateBuilder.On("SetIfNotExists", "TotalMessageCount", nil, int64(0)).Return(updateBuilder).Once()
	updateBuilder.On("Add", "TotalMessageCount", 1).Return(updateBuilder).Once()
	updateBuilder.On("Set", "LastStatusID", "status-1").Return(updateBuilder).Once()
	updateBuilder.On("Set", "LastMessageTime", publishedAt.UTC()).Return(updateBuilder).Once()
	updateBuilder.On("Set", "UpdatedAt", mock.AnythingOfType("time.Time")).Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateConversationLastStatus(ctx, "conv-1", "status-1"))
}
