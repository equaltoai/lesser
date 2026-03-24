package conversations

import (
	"context"
	"errors"
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

func TestService_updateSendMessageParticipantStateAfterSend_CoversRecipientRequestStateCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		recipientState   models.DmRequestState
		recipientHasTime bool
		preference       string
		wantState        models.DmRequestState
		wantAcceptedAt   bool
		wantRequestedAt  bool
	}{
		{
			name:            "accepted_stays_accepted",
			recipientState:  models.DmRequestStateAccepted,
			wantState:       models.DmRequestStateAccepted,
			wantAcceptedAt:  false,
			wantRequestedAt: false,
		},
		{
			name:            "declined_becomes_pending",
			recipientState:  models.DmRequestStateDeclined,
			wantState:       models.DmRequestStatePending,
			wantAcceptedAt:  false,
			wantRequestedAt: true,
		},
		{
			name:            "unset_delivers_to_inbox_accepts",
			recipientState:  "",
			preference:      "ANYONE",
			wantState:       models.DmRequestStateAccepted,
			wantAcceptedAt:  true,
			wantRequestedAt: false,
		},
		{
			name:            "unset_defaults_to_pending",
			recipientState:  "",
			wantState:       models.DmRequestStatePending,
			wantAcceptedAt:  false,
			wantRequestedAt: true,
		},
		{
			name:             "pending_does_not_overwrite_requested_at",
			recipientState:   models.DmRequestStatePending,
			recipientHasTime: true,
			wantState:        models.DmRequestStatePending,
			wantAcceptedAt:   false,
			wantRequestedAt:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conversationRepo := &mockConversationRepository{}

			var userRepo interfaces.UserRepository
			if tc.preference != "" {
				mockUserRepo := testmocks.NewMockUserRepositoryInterface()
				mockUserRepo.
					On("GetUserPreferences", mock.Anything, "bob").
					Return(&storage.UserPreferences{DirectMessagesFrom: tc.preference}, nil).
					Once()
				userRepo = mockUserRepo
			}

			service := NewService(conversationRepo, nil, nil, nil, nil, userRepo, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

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

			conversationRepo.
				On("GetConversationParticipantRecord", ctx, "conv123", "alice").
				Return(senderRecord, nil).
				Once()
			conversationRepo.
				On("GetConversationParticipantRecord", ctx, "conv123", "bob").
				Return(recipientRecord, nil).
				Once()
			conversationRepo.
				On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).
				Return(nil).
				Twice()

			service.updateSendMessageParticipantStateAfterSend(ctx, "conv123", "alice", "bob")

			require.Equal(t, models.DmRequestStateAccepted, senderRecord.RequestState)
			require.NotNil(t, senderRecord.AcceptedAt)
			require.Nil(t, senderRecord.DeclinedAt)

			require.Equal(t, tc.wantState, recipientRecord.RequestState)
			if tc.wantAcceptedAt {
				require.NotNil(t, recipientRecord.AcceptedAt)
			}
			if tc.wantRequestedAt {
				require.NotNil(t, recipientRecord.RequestedAt)
			}

			conversationRepo.AssertExpectations(t)
			if mockUserRepo, ok := userRepo.(*testmocks.MockUserRepositoryInterface); ok {
				mockUserRepo.AssertExpectations(t)
			}
		})
	}
}

func TestService_CreateConversation_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_command", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		_, err := service.CreateConversation(ctx, nil)
		require.ErrorIs(t, err, ErrConversationValidationFailed)
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("invalid_creator_or_participant", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: " ", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrConversationValidationFailed)
		require.ErrorIs(t, err, ErrInvalidRecipient)
	})

	t.Run("creator_account_lookup_failure", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		service := NewService(conversationRepo, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrGetSenderAccount)
		accountRepo.AssertExpectations(t)
	})

	t.Run("participant_account_lookup_failure", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		service := NewService(conversationRepo, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrInvalidRecipient)
		accountRepo.AssertExpectations(t)
	})

	t.Run("relationship_check_error", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		service := NewService(conversationRepo, nil, nil, accountRepo, relationshipRepo, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

		relationshipRepo.On("IsBlockedBidirectional", mock.Anything, "alice", "bob").Return(false, errors.New("boom")).Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrConversationValidationFailed)
		accountRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("blocked_users", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		service := NewService(conversationRepo, nil, nil, accountRepo, relationshipRepo, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

		relationshipRepo.On("IsBlockedBidirectional", mock.Anything, "alice", "bob").Return(true, nil).Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrDirectMessageBlocked)
		accountRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("lookup_existing_conversation_error", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		service := NewService(conversationRepo, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

		conversationRepo.
			On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
			Return((*models.Conversation)(nil), errors.New("boom")).
			Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrLookupExistingConversation)
		accountRepo.AssertExpectations(t)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("create_conversation_failure", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		service := NewService(conversationRepo, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

		conversationRepo.
			On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
			Return((*models.Conversation)(nil), errors.New("not found")).
			Once()
		conversationRepo.
			On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"alice", "bob"}).
			Return(errors.New("boom")).
			Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrCreateConversation)
		accountRepo.AssertExpectations(t)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("existing_conversation_does_not_reset_participant_state", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		accountRepo := &mockAccountRepository{}
		service := NewService(conversationRepo, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

		existing := createTestConversation("conv123", []string{"alice", "bob"})
		conversationRepo.
			On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
			Return(existing, nil).
			Once()

		creatorRecord := &models.ConversationParticipantRecord{Unread: true}
		participantRecord := &models.ConversationParticipantRecord{RequestState: models.DmRequestStatePending}

		conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "alice").Return(creatorRecord, nil).Twice()
		conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "bob").Return(participantRecord, nil).Once()
		conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil).Twice()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.NoError(t, err)
		require.Equal(t, models.DmRequestStateAccepted, creatorRecord.RequestState)
		require.Equal(t, models.DmRequestStatePending, participantRecord.RequestState)
	})
}

func TestService_FilterConversationMessagesForViewer_FiltersExpectedMessages(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	viewerUsername := "alice"
	viewerActorID := "https://example.com/users/alice"

	kept := service.filterConversationMessagesForViewer([]*models.Status{
		nil,
		{StatusID: "s1", Visibility: "public"},
		{StatusID: "s2", Visibility: VisibilityDirect, AuthorUsername: "bob"},
		{StatusID: "s3", Visibility: VisibilityDirect, AuthorUsername: "alice", BccRecipients: []string{"secret"}},
		{StatusID: "s4", Visibility: VisibilityDirect, AuthorUsername: "bob", ToRecipients: []string{viewerActorID}, BccRecipients: []string{"secret"}},
	}, viewerUsername, viewerActorID)

	require.Len(t, kept, 2)
	require.Equal(t, "s3", kept[0].StatusID)
	require.Nil(t, kept[0].BccRecipients)
	require.Equal(t, "s4", kept[1].StatusID)
	require.Nil(t, kept[1].BccRecipients)
}

func TestService_validateSendDirectMessageCommand_RejectsInvalidRecipientForms(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	_, err := service.validateSendDirectMessageCommand(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob", "carol"},
		Content:    "hi",
	})
	require.ErrorIs(t, err, ErrDirectMessageRequiresSingleRecipient)

	_, err = service.validateSendDirectMessageCommand(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{" "},
		Content:    "hi",
	})
	require.ErrorIs(t, err, ErrInvalidRecipient)

	_, err = service.validateSendDirectMessageCommand(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"alice"},
		Content:    "hi",
	})
	require.ErrorIs(t, err, ErrInvalidRecipient)
}

func TestService_enforceDirectMessageNotBlocked_ReturnsValidationErrorOnRepoFailure(t *testing.T) {
	ctx := context.Background()

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(false, errors.New("boom")).
		Once()

	service := NewService(nil, nil, nil, nil, relationshipRepo, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.enforceDirectMessageNotBlocked(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "bob")
	require.ErrorIs(t, err, ErrConversationValidationFailed)

	relationshipRepo.AssertExpectations(t)
}

func TestService_getDirectMessageAccounts_ReturnsErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("sender_lookup_fails", func(t *testing.T) {
		accountRepo := &mockAccountRepository{}
		service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, _, err := service.getDirectMessageAccounts(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    "hi",
		}, "bob")
		require.ErrorIs(t, err, ErrGetSenderAccount)
	})

	t.Run("recipient_lookup_fails", func(t *testing.T) {
		accountRepo := &mockAccountRepository{}
		service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, _, err := service.getDirectMessageAccounts(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    "hi",
		}, "bob")
		require.ErrorIs(t, err, ErrInvalidRecipient)
	})
}

func TestService_directMessageRecipientRequestState_ReturnsStateWhenPresent(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("GetConversationParticipantRecord", ctx, "conv123", "bob").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted}, nil).
		Once()

	require.Equal(t, models.DmRequestStateAccepted, service.directMessageRecipientRequestState(ctx, "conv123", "bob"))
	conversationRepo.AssertExpectations(t)
}

func TestService_createDirectMessageStatus_ReturnsErrorsWhenRepositoryFails(t *testing.T) {
	ctx := context.Background()

	noteRepo := &mockNoteRepository{}
	service := NewService(nil, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(errors.New("boom")).Once()

	_, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob": createTestAccount("bob", "bob"),
	}, "conv123", "bob")
	require.ErrorIs(t, err, ErrCreateDirectMessage)
}

func TestService_createDirectMessageStatus_SetsAllCreationTimestamps(t *testing.T) {
	ctx := context.Background()

	noteRepo := &mockNoteRepository{}
	service := NewService(nil, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	var created *models.Status
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).
		Run(func(args mock.Arguments) {
			created = args.Get(1).(*models.Status)
		}).
		Return(nil).
		Once()

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob": createTestAccount("bob", "bob"),
	}, "conv123", "bob")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.NotNil(t, created)
	require.False(t, created.PublishedAt.IsZero())
	require.Equal(t, created.PublishedAt, created.CreatedAt)
	require.Equal(t, created.PublishedAt, created.ModifiedAt)
	require.Equal(t, created.PublishedAt, created.UpdatedAt)
}

func TestService_updateConversationLastStatus_HandlesUpdateFailures(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).
		Return(errors.New("boom")).
		Once()

	service.updateConversationLastStatus(ctx, createTestConversation("conv123", []string{"alice", "bob"}), "msg-1")
	conversationRepo.AssertExpectations(t)
}

func TestService_validateSendMessageReplyTarget_ReturnsErrorWhenParentMissing(t *testing.T) {
	ctx := context.Background()

	noteRepo := &mockNoteRepository{}
	service := NewService(nil, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	noteRepo.On("GetStatus", ctx, "parent-1").Return((*models.Status)(nil), errors.New("boom")).Once()

	err := service.validateSendMessageReplyTarget(ctx, "parent-1", "conv123")
	require.ErrorIs(t, err, ErrInvalidInReplyToIDConversation)
}

func TestService_getSendMessageAccounts_ReturnsErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("sender_lookup_fails", func(t *testing.T) {
		accountRepo := &mockAccountRepository{}
		service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, _, err := service.getSendMessageAccounts(ctx, "alice", "bob")
		require.ErrorIs(t, err, ErrGetSenderAccount)
	})

	t.Run("recipient_lookup_fails", func(t *testing.T) {
		accountRepo := &mockAccountRepository{}
		service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, _, err := service.getSendMessageAccounts(ctx, "alice", "bob")
		require.ErrorIs(t, err, ErrInvalidRecipient)
	})
}

func TestService_createSendMessageStatus_SetsAllCreationTimestamps(t *testing.T) {
	ctx := context.Background()

	noteRepo := &mockNoteRepository{}
	service := NewService(nil, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	var created *models.Status
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).
		Run(func(args mock.Arguments) {
			created = args.Get(1).(*models.Status)
		}).
		Return(nil).
		Once()

	status, _, err := service.createSendMessageStatus(
		ctx,
		&SendMessageCommand{
			SenderID: "alice",
			Content:  "hi again",
		},
		&SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    "hi again",
		},
		createTestAccount("alice", "alice"),
		createTestAccount("bob", "bob"),
		"conv123",
		"bob",
	)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.NotNil(t, created)
	require.False(t, created.PublishedAt.IsZero())
	require.Equal(t, created.PublishedAt, created.CreatedAt)
	require.Equal(t, created.PublishedAt, created.ModifiedAt)
	require.Equal(t, created.PublishedAt, created.UpdatedAt)
}

func TestService_AcceptAndDeclineMessageRequest_NilCommand(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	_, err := service.AcceptMessageRequest(ctx, nil)
	require.ErrorIs(t, err, ErrConversationValidationFailed)
	require.ErrorIs(t, err, storage.ErrInvalidInput)

	_, err = service.DeclineMessageRequest(ctx, nil)
	require.ErrorIs(t, err, ErrConversationValidationFailed)
	require.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestService_ListConversations_FolderBranchesAndErrors(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	paginated := &interfaces.PaginatedResult[*models.Conversation]{Items: []*models.Conversation{}, HasMore: false}

	conversationRepo.
		On("GetUserConversationsByRequestState", ctx, "alice", models.DmRequestStatePending, mock.Anything).
		Return(paginated, nil).
		Once()
	_, err := service.ListConversations(ctx, &ListConversationsQuery{
		UserID:     "alice",
		Folder:     ConversationFolderRequests,
		Pagination: interfaces.PaginationOptions{Limit: 10},
	})
	require.NoError(t, err)

	conversationRepo.
		On("GetUserConversationsByRequestState", ctx, "alice", models.DmRequestStateAccepted, mock.Anything).
		Return(paginated, nil).
		Once()
	_, err = service.ListConversations(ctx, &ListConversationsQuery{
		UserID:     "alice",
		Folder:     ConversationFolderInbox,
		Pagination: interfaces.PaginationOptions{Limit: 10},
	})
	require.NoError(t, err)

	conversationRepo.
		On("GetUserConversations", ctx, "alice", mock.Anything).
		Return((*interfaces.PaginatedResult[*models.Conversation])(nil), errors.New("boom")).
		Once()
	_, err = service.ListConversations(ctx, &ListConversationsQuery{
		UserID:     "alice",
		Pagination: interfaces.PaginationOptions{Limit: 10},
	})
	require.ErrorIs(t, err, ErrGetUserConversations)
}

func TestService_auditEvent_AllowsNilMetadataAndSwallowsStoreErrors(t *testing.T) {
	ctx := context.Background()

	auditRepo := testmocks.NewMockAuditRepository()
	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"event.type",
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
			mock.MatchedBy(func(md map[string]interface{}) bool { return md != nil }),
		).
		Return(errors.New("boom")).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, auditRepo, nil, nil, zaptest.NewLogger(t), "example.com")
	service.auditEvent(ctx, "event.type", "LOW", "alice", "alice", true, "", nil)

	auditRepo.AssertExpectations(t)
}

func TestService_actorURLForUsername_HandlesBlankAndURLs(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	require.Equal(t, "", service.actorURLForUsername("  "))
	require.Equal(t, "https://remote.example/users/alice", service.actorURLForUsername("https://remote.example/users/alice/"))
	require.Equal(t, "https://example.com/users/alice", service.actorURLForUsername("alice"))
}

func TestService_updateParticipantRecord_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_mutator_is_noop", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.NoError(t, service.updateParticipantRecord(ctx, "conv123", "alice", nil))
	})

	t.Run("returns_error_from_repo", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.
			On("GetConversationParticipantRecord", ctx, "conv123", "alice").
			Return((*models.ConversationParticipantRecord)(nil), errors.New("boom")).
			Once()

		err := service.updateParticipantRecord(ctx, "conv123", "alice", func(*models.ConversationParticipantRecord) {})
		require.Error(t, err)
	})

	t.Run("returns_not_found_when_record_nil", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.
			On("GetConversationParticipantRecord", ctx, "conv123", "alice").
			Return((*models.ConversationParticipantRecord)(nil), nil).
			Once()

		err := service.updateParticipantRecord(ctx, "conv123", "alice", func(*models.ConversationParticipantRecord) {})
		require.ErrorIs(t, err, storage.ErrNotFound)
	})
}
