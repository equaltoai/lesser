package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type nonTransactionalConversationRepo struct {
	mockConversationRepository
}

func (r *nonTransactionalConversationRepo) TransactionalDirectMessageSendEnabled() bool {
	return false
}

type transactionalConversationRepo struct {
	mockConversationRepository
}

func (r *transactionalConversationRepo) TransactionalDirectMessageSendEnabled() bool {
	return true
}

func TestService_enforceDirectMessageRequestRateLimit_AuditsAndFailsOnTotalLimit(t *testing.T) {
	ctx := context.Background()

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	rateLimitRepo.
		On("CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(storage.ErrRateLimited).
		Once()

	auditRepo := testmocks.NewMockAuditRepository()
	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.send",
			"HIGH",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			false,
			"rate_limited_request_total",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md != nil && md["conversation_id"] == "conv123" && md["sender_id"] == "alice" && md["recipient_id"] == "bob"
			}),
		).
		Return(nil).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, rateLimitRepo, auditRepo, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.enforceDirectMessageRequestRateLimit(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.ErrorIs(t, err, storage.ErrRateLimited)

	rateLimitRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestService_enforceDirectMessageRequestRateLimit_AuditsAndFailsOnPerRecipientLimit(t *testing.T) {
	ctx := context.Background()

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	rateLimitRepo.
		On("CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(nil).
		Once()
	rateLimitRepo.
		On("CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow).
		Return(storage.ErrRateLimited).
		Once()

	auditRepo := testmocks.NewMockAuditRepository()
	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.send",
			"HIGH",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			false,
			"rate_limited_request_to_recipient",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md != nil && md["conversation_id"] == "conv123" && md["sender_id"] == "alice" && md["recipient_id"] == "bob"
			}),
		).
		Return(nil).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, rateLimitRepo, auditRepo, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.enforceDirectMessageRequestRateLimit(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.ErrorIs(t, err, storage.ErrRateLimited)

	rateLimitRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestService_DirectMessageInboxPreferenceHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("directMessagesFromPreference_defaults_when_user_repo_nil", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.Equal(t, "FOLLOWING_ONLY", service.directMessagesFromPreference(ctx, "alice"))
	})

	t.Run("directMessagesFromPreference_uppercases_and_trims", func(t *testing.T) {
		userRepo := testmocks.NewMockUserRepositoryInterface()
		userRepo.
			On("GetUserPreferences", mock.Anything, "alice").
			Return(&storage.UserPreferences{DirectMessagesFrom: "  anyone "}, nil).
			Once()

		service := NewService(nil, nil, nil, nil, nil, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.Equal(t, "ANYONE", service.directMessagesFromPreference(ctx, "alice"))
		userRepo.AssertExpectations(t)
	})

	t.Run("shouldDeliverToInbox_returns_true_for_ANYONE_without_relationship_repo", func(t *testing.T) {
		userRepo := testmocks.NewMockUserRepositoryInterface()
		userRepo.
			On("GetUserPreferences", mock.Anything, "bob").
			Return(&storage.UserPreferences{DirectMessagesFrom: "ANYONE"}, nil).
			Once()

		service := NewService(nil, nil, nil, nil, nil, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.True(t, service.shouldDeliverToInbox(ctx, "bob", "alice"))
		userRepo.AssertExpectations(t)
	})

	t.Run("shouldDeliverToInbox_defaults_to_following_only", func(t *testing.T) {
		userRepo := testmocks.NewMockUserRepositoryInterface()
		userRepo.
			On("GetUserPreferences", mock.Anything, "bob").
			Return(&storage.UserPreferences{DirectMessagesFrom: ""}, nil).
			Once()

		service := NewService(nil, nil, nil, nil, nil, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.False(t, service.shouldDeliverToInbox(ctx, "bob", "alice"))
		userRepo.AssertExpectations(t)
	})

	t.Run("shouldDeliverToInbox_honors_following_relationship", func(t *testing.T) {
		userRepo := testmocks.NewMockUserRepositoryInterface()
		userRepo.
			On("GetUserPreferences", mock.Anything, "bob").
			Return(&storage.UserPreferences{DirectMessagesFrom: ""}, nil).
			Once()

		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.
			On("IsFollowing", mock.Anything, "bob", "alice").
			Return(true, nil).
			Once()

		service := NewService(nil, nil, nil, nil, relationshipRepo, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.True(t, service.shouldDeliverToInbox(ctx, "bob", "alice"))
		userRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("shouldDeliverToInbox_returns_false_on_relationship_error", func(t *testing.T) {
		userRepo := testmocks.NewMockUserRepositoryInterface()
		userRepo.
			On("GetUserPreferences", mock.Anything, "bob").
			Return(&storage.UserPreferences{DirectMessagesFrom: ""}, nil).
			Once()

		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.
			On("IsFollowing", mock.Anything, "bob", "alice").
			Return(false, errors.New("boom")).
			Once()

		service := NewService(nil, nil, nil, nil, relationshipRepo, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.False(t, service.shouldDeliverToInbox(ctx, "bob", "alice"))
		userRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})
}

func TestService_buildDirectMessageParticipantStatesForSend_CoversRecipientRequestStateCases(t *testing.T) {
	tests := []struct {
		name             string
		recipientState   models.DmRequestState
		deliversToInbox  bool
		recipientHasTime bool
		wantState        models.DmRequestState
		wantFolder       models.UserConversationFolder
		wantAcceptedAt   bool
		wantRequestedAt  bool
	}{
		{
			name:            "accepted_stays_accepted",
			recipientState:  models.DmRequestStateAccepted,
			deliversToInbox: false,
			wantState:       models.DmRequestStateAccepted,
			wantFolder:      models.UserConversationFolderInbox,
			wantAcceptedAt:  false,
			wantRequestedAt: false,
		},
		{
			name:            "declined_becomes_pending",
			recipientState:  models.DmRequestStateDeclined,
			deliversToInbox: false,
			wantState:       models.DmRequestStatePending,
			wantFolder:      models.UserConversationFolderRequests,
			wantAcceptedAt:  false,
			wantRequestedAt: true,
		},
		{
			name:            "unset_delivers_to_inbox_accepts",
			recipientState:  "",
			deliversToInbox: true,
			wantState:       models.DmRequestStateAccepted,
			wantFolder:      models.UserConversationFolderInbox,
			wantAcceptedAt:  true,
			wantRequestedAt: false,
		},
		{
			name:            "unset_respects_requests_sets_requested_at",
			recipientState:  "",
			deliversToInbox: false,
			wantState:       models.DmRequestStatePending,
			wantFolder:      models.UserConversationFolderRequests,
			wantAcceptedAt:  false,
			wantRequestedAt: true,
		},
		{
			name:             "pending_does_not_overwrite_requested_at",
			recipientState:   models.DmRequestStatePending,
			deliversToInbox:  false,
			recipientHasTime: true,
			wantState:        models.DmRequestStatePending,
			wantFolder:       models.UserConversationFolderRequests,
			wantAcceptedAt:   false,
			wantRequestedAt:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conversation := createTestConversation("conv123", []string{"alice", "bob"})
			publishedAt := time.Date(2026, 3, 26, 13, 15, 0, 0, time.UTC)

			senderRecord := &models.ConversationParticipantRecord{
				RequestState: models.DmRequestStatePending,
				DeletedAt:    ptrTime(time.Now()),
			}

			recipientRecord := &models.ConversationParticipantRecord{
				RequestState: tc.recipientState,
				DeletedAt:    ptrTime(time.Now()),
			}
			if tc.recipientHasTime {
				tm := time.Now().UTC().Add(-time.Minute)
				recipientRecord.RequestedAt = &tm
			}

			states := buildDirectMessageParticipantStatesForSend(
				conversation,
				&models.Status{StatusID: "status-1", PublishedAt: publishedAt},
				"alice",
				"bob",
				senderRecord,
				recipientRecord,
				tc.deliversToInbox,
			)

			require.Len(t, states, 2)

			senderState := states[0]
			require.Equal(t, models.DmRequestStateAccepted, senderState.RequestState)
			require.Equal(t, models.UserConversationFolderInbox, senderState.Folder)
			require.Nil(t, senderState.DeletedAt)
			require.NotNil(t, senderState.AcceptedAt)
			require.Nil(t, senderState.DeclinedAt)
			require.False(t, senderState.Unread)
			require.NotNil(t, senderState.LastReadAt)
			require.Equal(t, "status-1", senderState.PreviewStatusID)
			require.Equal(t, publishedAt, senderState.SortAt)

			recipientState := states[1]
			require.Equal(t, tc.wantState, recipientState.RequestState)
			require.Equal(t, tc.wantFolder, recipientState.Folder)
			require.Nil(t, recipientState.DeletedAt)
			require.True(t, recipientState.Unread)
			require.Nil(t, recipientState.LastReadAt)
			require.Equal(t, "status-1", recipientState.PreviewStatusID)
			require.Equal(t, publishedAt, recipientState.SortAt)
			if tc.wantAcceptedAt {
				require.NotNil(t, recipientState.AcceptedAt)
			}
			if tc.wantRequestedAt {
				require.NotNil(t, recipientState.RequestedAt)
			}
		})
	}
}

func TestService_applyDirectMessageSendTransition_MirrorsStatusForNonTransactionalRepositories(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &nonTransactionalConversationRepo{}
	noteRepo := &mockNoteRepository{}
	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	status := &models.Status{
		StatusID:    "status-1",
		PublishedAt: time.Date(2026, 3, 26, 13, 20, 0, 0, time.UTC),
	}

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
			if transition == nil || transition.Status == nil || transition.Conversation == nil {
				return false
			}
			return transition.Status.StatusID == "status-1" && transition.Conversation.ID == "conv123" && !transition.CreateConversation
		})).
		Return(nil).
		Once()
	noteRepo.
		On("CreateStatus", ctx, mock.MatchedBy(func(stored *models.Status) bool {
			return stored != nil && stored.StatusID == "status-1"
		})).
		Return(nil).
		Once()

	err := service.applyDirectMessageSendTransition(
		ctx,
		conversation,
		false,
		"alice",
		"bob",
		nil,
		nil,
		status,
		true,
	)
	require.NoError(t, err)
	require.Equal(t, "status-1", conversation.LastStatusID)
	require.EqualValues(t, 1, conversation.TotalMessageCount)
	require.Equal(t, status.PublishedAt.UTC(), conversation.LastMessageTime)
	require.False(t, conversation.Unread)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_applyDirectMessageSendTransition_FinalizesStatusForTransactionalRepositories(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &transactionalConversationRepo{}
	noteRepo := &mockNoteRepository{}
	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	status := &models.Status{
		StatusID:    "status-1",
		PublishedAt: time.Date(2026, 3, 26, 13, 20, 0, 0, time.UTC),
	}

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
			if transition == nil || transition.Status == nil || transition.Conversation == nil {
				return false
			}
			return transition.Status.StatusID == "status-1" &&
				transition.Conversation.ID == "conv123" &&
				!transition.CreateConversation &&
				len(transition.ExpectedParticipantStates) == 2
		})).
		Return(nil).
		Once()
	noteRepo.
		On("FinalizeCreatedStatus", ctx, mock.MatchedBy(func(stored *models.Status) bool {
			return stored != nil && stored.StatusID == "status-1"
		})).
		Return(nil).
		Once()

	err := service.applyDirectMessageSendTransition(
		ctx,
		conversation,
		false,
		"alice",
		"bob",
		nil,
		nil,
		status,
		true,
	)
	require.NoError(t, err)
	require.Equal(t, "status-1", conversation.LastStatusID)
	require.EqualValues(t, 1, conversation.TotalMessageCount)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	noteRepo.AssertNotCalled(t, "CreateStatus", mock.Anything, mock.Anything)
}

func TestService_applyDirectMessageSendTransition_UsesParticipantRecordVersionsForExpectedStates(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &transactionalConversationRepo{}
	noteRepo := &mockNoteRepository{}
	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	conversation.UpdatedAt = time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)
	status := &models.Status{
		StatusID:    "status-expected",
		PublishedAt: time.Date(2026, 3, 26, 13, 20, 0, 0, time.UTC),
	}

	senderUpdatedAt := time.Date(2026, 3, 26, 13, 5, 0, 0, time.UTC)
	recipientUpdatedAt := time.Date(2026, 3, 26, 13, 10, 0, 0, time.UTC)
	senderRecord := &models.ConversationParticipantRecord{
		RequestState: models.DmRequestStateAccepted,
		UpdatedAt:    senderUpdatedAt,
		Conversation: &models.Conversation{
			ID:        conversation.ID,
			CreatedAt: conversation.CreatedAt,
			UpdatedAt: conversation.UpdatedAt,
		},
	}
	recipientRecord := &models.ConversationParticipantRecord{
		RequestState: models.DmRequestStateAccepted,
		UpdatedAt:    recipientUpdatedAt,
		Conversation: &models.Conversation{
			ID:        conversation.ID,
			CreatedAt: conversation.CreatedAt,
			UpdatedAt: conversation.UpdatedAt,
		},
	}

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
			if transition == nil || transition.Conversation == nil || transition.Status == nil {
				return false
			}
			if transition.Conversation.ID != "conv123" || transition.Status.StatusID != "status-expected" {
				return false
			}
			if len(transition.ExpectedParticipantStates) != 2 {
				return false
			}

			expectedByViewer := make(map[string]*models.UserConversationState, len(transition.ExpectedParticipantStates))
			for _, state := range transition.ExpectedParticipantStates {
				expectedByViewer[state.ViewerID] = state
			}

			return expectedByViewer["alice"] != nil &&
				expectedByViewer["bob"] != nil &&
				expectedByViewer["alice"].UpdatedAt.Equal(senderUpdatedAt) &&
				expectedByViewer["bob"].UpdatedAt.Equal(recipientUpdatedAt)
		})).
		Return(nil).
		Once()
	noteRepo.
		On("FinalizeCreatedStatus", ctx, mock.MatchedBy(func(stored *models.Status) bool {
			return stored != nil && stored.StatusID == "status-expected"
		})).
		Return(nil).
		Once()

	err := service.applyDirectMessageSendTransition(
		ctx,
		conversation,
		false,
		"alice",
		"bob",
		senderRecord,
		recipientRecord,
		status,
		true,
	)
	require.NoError(t, err)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_applyDirectMessageSendTransition_ReturnsTransactionalFinalizerErrors(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &transactionalConversationRepo{}
	noteRepo := &mockNoteRepository{}
	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	status := &models.Status{
		StatusID:    "status-1",
		PublishedAt: time.Date(2026, 3, 26, 13, 20, 0, 0, time.UTC),
	}

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition")).
		Return(nil).
		Once()
	noteRepo.
		On("FinalizeCreatedStatus", ctx, mock.AnythingOfType("*models.Status")).
		Return(errors.New("boom")).
		Once()

	err := service.applyDirectMessageSendTransition(
		ctx,
		conversation,
		false,
		"alice",
		"bob",
		nil,
		nil,
		status,
		true,
	)
	require.ErrorIs(t, err, ErrCreateDirectMessage)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_finalizeDirectMessageStatusWrite_NoOpsWithoutCapabilitySignal(t *testing.T) {
	ctx := context.Background()
	noteRepo := &mockNoteRepository{}
	service := NewService(&mockConversationRepository{}, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.finalizeDirectMessageStatusWrite(ctx, &models.Status{StatusID: "status-1"})
	require.NoError(t, err)
	noteRepo.AssertNotCalled(t, "CreateStatus", mock.Anything, mock.Anything)
	noteRepo.AssertNotCalled(t, "FinalizeCreatedStatus", mock.Anything, mock.Anything)
}

func TestService_resolveDirectMessageConversationForSend_ReturnsLookupErrors(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), errors.New("boom")).
		Once()

	_, _, err := service.resolveDirectMessageConversationForSend(ctx, "alice", "bob")
	require.ErrorIs(t, err, ErrLookupExistingConversation)
	conversationRepo.AssertExpectations(t)
}

func TestService_resolveDirectMessageConversationForSend_CreatesCanonicalConversationWhenMissing(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), errors.New("not found")).
		Once()

	conversation, createConversation, err := service.resolveDirectMessageConversationForSend(ctx, "bob", "alice")
	require.NoError(t, err)
	require.True(t, createConversation)
	require.NotNil(t, conversation)
	require.NotEmpty(t, conversation.ID)
	require.Equal(t, []string{"alice", "bob"}, conversation.Participants)
	require.False(t, conversation.CreatedAt.IsZero())
	require.False(t, conversation.UpdatedAt.IsZero())
	conversationRepo.AssertExpectations(t)
}

func TestService_loadConversationAndRecipientForSendMessage_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_get_conversation_error", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.On("GetConversation", ctx, "conv123").Return((*models.Conversation)(nil), errors.New("boom")).Once()

		_, _, err := service.loadConversationAndRecipientForSendMessage(ctx, &SendMessageCommand{
			SenderID:       "alice",
			ConversationID: "conv123",
			Content:        "hi",
		})
		require.ErrorIs(t, err, ErrGetConversation)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("rejects_non_one_to_one", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.On("GetConversation", ctx, "conv123").Return(createTestConversation("conv123", []string{"alice"}), nil).Once()

		_, _, err := service.loadConversationAndRecipientForSendMessage(ctx, &SendMessageCommand{
			SenderID:       "alice",
			ConversationID: "conv123",
			Content:        "hi",
		})
		require.ErrorIs(t, err, ErrConversationMustBeOneToOne)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("rejects_non_participant", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.On("GetConversation", ctx, "conv123").Return(createTestConversation("conv123", []string{"bob", "carol"}), nil).Once()

		_, _, err := service.loadConversationAndRecipientForSendMessage(ctx, &SendMessageCommand{
			SenderID:       "alice",
			ConversationID: "conv123",
			Content:        "hi",
		})
		require.ErrorIs(t, err, ErrNotConversationParticipant)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("rejects_missing_recipient", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.On("GetConversation", ctx, "conv123").Return(createTestConversation("conv123", []string{"alice", "alice"}), nil).Once()

		_, _, err := service.loadConversationAndRecipientForSendMessage(ctx, &SendMessageCommand{
			SenderID:       "alice",
			ConversationID: "conv123",
			Content:        "hi",
		})
		require.ErrorIs(t, err, ErrInvalidRecipient)
		conversationRepo.AssertExpectations(t)
	})
}
