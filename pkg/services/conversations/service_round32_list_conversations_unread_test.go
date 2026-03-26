package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_ListConversations_OnlyUnread(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	query := &ListConversationsQuery{
		UserID: "user-1",
		Pagination: interfaces.PaginationOptions{
			Limit: 10,
		},
		OnlyUnread: true,
	}

	t.Run("success uses unread repository method", func(t *testing.T) {
		conversations := []*models.Conversation{
			createTestConversation("conv-1", []string{"user-1", "user-2"}),
		}
		conversationRepo.On("GetUnreadConversations", ctx, "user-1", query.Pagination).Return(&interfaces.PaginatedResult[*models.Conversation]{Items: conversations}, nil).Once()

		out, err := service.ListConversations(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Len(t, out.Conversations.Items, 1)
		conversationRepo.AssertNotCalled(t, "GetUserConversations", ctx, "user-1", query.Pagination)
		conversationRepo.AssertNotCalled(t, "GetUserConversationsByFolder", ctx, "user-1", mock.Anything, query.Pagination)
	})

	t.Run("repo error returns expected error", func(t *testing.T) {
		conversationRepo.On("GetUnreadConversations", ctx, "user-1", query.Pagination).Return((*interfaces.PaginatedResult[*models.Conversation])(nil), errors.New("boom")).Once()
		_, err := service.ListConversations(ctx, query)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGetUserConversations)
	})
}
