package conversations

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestService_MessageRequestDecision_ActedByAuditMetadata pins the share-grant
// act-as caller-attribution contract on message-request decisions: when the
// caller acts under a grant, the service-level audit event records the real
// caller as acted_by; the owner path never gains the field.
func TestService_MessageRequestDecision_ActedByAuditMetadata(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T, actedBy string) (*Service, *mockConversationRepository, *testmocks.MockAuditRepository) {
		t.Helper()

		conversationRepo := &mockConversationRepository{}
		auditRepo := testmocks.NewMockAuditRepository()

		service := NewService(
			conversationRepo,
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

		conversation := createTestConversation("conv123", []string{"agent-one", "bob"})
		deletedAt := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
		conversationRepo.On("GetUserConversationState", ctx, "agent-one", "conv123").Return(
			testConversationStateContract("agent-one", "conv123", func(state *interfaces.UserConversationStateContract) {
				state.RequestState = models.DmRequestStatePending
				state.Folder = models.UserConversationFolderRequests
				state.DeletedAt = &deletedAt
				state.RequestedAt = &deletedAt
				state.DeclinedAt = &deletedAt
			}),
			nil,
		).Twice()
		conversationRepo.On("PutUserConversationState", ctx, mock.MatchedBy(func(state *models.UserConversationState) bool {
			return state != nil && state.RequestState == models.DmRequestStateAccepted
		})).Return(nil).Once()

		auditRepo.
			On(
				"StoreAuditEvent",
				mock.Anything,
				"dm.request.accept",
				"LOW",
				"agent-one",
				"agent-one",
				"",
				"",
				"",
				"",
				"",
				true,
				"",
				mock.MatchedBy(func(md map[string]interface{}) bool {
					if md == nil || md["conversation_id"] != "conv123" {
						return false
					}
					if actedBy == "" {
						_, present := md["acted_by"]
						return !present
					}
					return md["acted_by"] == actedBy
				}),
			).
			Return(nil).
			Once()

		return service, conversationRepo, auditRepo
	}

	t.Run("act-as records acted_by", func(t *testing.T) {
		service, conversationRepo, auditRepo := setup(t, "alice")

		result, err := service.AcceptMessageRequest(ctx, &AcceptMessageRequestCommand{
			ConversationID: "conv123",
			UserID:         "agent-one",
			ActedBy:        "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		conversationRepo.AssertExpectations(t)
		auditRepo.AssertExpectations(t)
	})

	t.Run("owner path never gains acted_by", func(t *testing.T) {
		service, conversationRepo, auditRepo := setup(t, "")

		result, err := service.AcceptMessageRequest(ctx, &AcceptMessageRequestCommand{
			ConversationID: "conv123",
			UserID:         "agent-one",
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		conversationRepo.AssertExpectations(t)
		auditRepo.AssertExpectations(t)
	})
}
