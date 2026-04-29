package conversations

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_SendDirectMessage_BlockedBidirectional(t *testing.T) {
	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(true, nil)

	service := NewService(
		nil,
		nil,
		nil,
		nil,
		relationshipRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	_, err := service.SendDirectMessage(context.Background(), &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	})
	require.ErrorIs(t, err, ErrDirectMessageBlocked)
	relationshipRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_RateLimited_TotalThroughput(t *testing.T) {
	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(false, nil)

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(false, 0, time.Now().UTC().Add(dmSendTotalWindow), nil).
		Once()

	service := NewService(
		nil,
		nil,
		nil,
		nil,
		relationshipRepo,
		nil,
		rateLimitRepo,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	_, err := service.SendDirectMessage(context.Background(), &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	})
	require.ErrorIs(t, err, storage.ErrRateLimited)
	relationshipRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_PendingRequest_BlocksAdditionalMessages(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	accountRepo := &mockAccountRepository{}

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(false, nil)
	relationshipRepo.
		On("IsFollowing", mock.Anything, "bob", "alice").
		Return(false, nil)

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(true, dmSendTotalLimit-1, time.Now().UTC().Add(dmSendTotalWindow), nil).
		Once()

	service := NewService(
		conversationRepo,
		noteRepo,
		nil,
		accountRepo,
		relationshipRepo,
		nil,
		rateLimitRepo,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	accountRepo.On("GetAccount", mock.Anything, "alice").Return(createTestAccount("alice", "alice"), nil)
	accountRepo.On("GetAccount", mock.Anything, "bob").Return(createTestAccount("bob", "bob"), nil)

	conversationRepo.
		On("GetConversationByParticipants", mock.Anything, []string{"alice", "bob"}).
		Return(conversation, nil)
	conversationRepo.
		On("GetUserConversationState", mock.Anything, "bob", "conv123").
		Return(testConversationStateContract("bob", "conv123", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStatePending
			state.RequestedAt = ptrTime(time.Now().UTC().Add(-time.Minute))
		}), nil)

	_, err := service.SendDirectMessage(context.Background(), &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "another message",
	})
	require.ErrorIs(t, err, ErrMessageRequestPending)

	relationshipRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	conversationRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_RequestDisallowsMedia(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	accountRepo := &mockAccountRepository{}

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(false, nil)
	relationshipRepo.
		On("IsFollowing", mock.Anything, "bob", "alice").
		Return(false, nil)

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(true, dmSendTotalLimit-1, time.Now().UTC().Add(dmSendTotalWindow), nil).
		Once()

	service := NewService(
		conversationRepo,
		noteRepo,
		nil,
		accountRepo,
		relationshipRepo,
		nil,
		rateLimitRepo,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	accountRepo.On("GetAccount", mock.Anything, "alice").Return(createTestAccount("alice", "alice"), nil)
	accountRepo.On("GetAccount", mock.Anything, "bob").Return(createTestAccount("bob", "bob"), nil)

	conversationRepo.
		On("GetConversationByParticipants", mock.Anything, []string{"alice", "bob"}).
		Return(conversation, nil)
	conversationRepo.
		On("GetUserConversationState", mock.Anything, "bob", "conv123").
		Return(testConversationStateContract("bob", "conv123", nil), nil)

	_, err := service.SendDirectMessage(context.Background(), &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hello",
		MediaIDs:   []string{"media-1"},
	})
	require.ErrorIs(t, err, ErrMessageRequestMediaNotAllowed)

	relationshipRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	conversationRepo.AssertExpectations(t)
}

func TestService_AuditDMEvent_DoesNotIncludeContent(t *testing.T) {
	auditRepo := testmocks.NewMockAuditRepository()
	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.send",
			"LOW",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			true,
			"",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				if md == nil {
					return false
				}
				for k, v := range md {
					if k == "content" {
						return false
					}
					if s, ok := v.(string); ok && s == "super secret dm body" {
						return false
					}
				}
				return md["conversation_id"] == "conv123" && md["sender_id"] == "alice" && md["recipient_id"] == "bob"
			}),
		).
		Return(nil).
		Once()

	service := NewService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		auditRepo,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	service.auditDMEvent(context.Background(), &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "super secret dm body",
	}, "conv123", true, "", map[string]any{
		"recipient_id": "bob",
	})

	auditRepo.AssertExpectations(t)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
