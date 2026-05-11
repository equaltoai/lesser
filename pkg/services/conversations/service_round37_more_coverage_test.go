package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_buildDirectMessageParticipantStatesForSend_CoversExistingConversationRequestStateCases(t *testing.T) {
	tests := []struct {
		name             string
		recipientState   models.DmRequestState
		recipientHasTime bool
		deliversToInbox  bool
		wantState        models.DmRequestState
		wantFolder       models.UserConversationFolder
		wantAcceptedAt   bool
		wantRequestedAt  bool
	}{
		{
			name:            "accepted_stays_accepted",
			recipientState:  models.DmRequestStateAccepted,
			wantState:       models.DmRequestStateAccepted,
			wantFolder:      models.UserConversationFolderInbox,
			wantAcceptedAt:  false,
			wantRequestedAt: false,
		},
		{
			name:            "declined_becomes_pending",
			recipientState:  models.DmRequestStateDeclined,
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
			name:            "unset_defaults_to_pending",
			recipientState:  "",
			wantFolder:      models.UserConversationFolderRequests,
			wantState:       models.DmRequestStatePending,
			wantAcceptedAt:  false,
			wantRequestedAt: true,
		},
		{
			name:             "pending_does_not_overwrite_requested_at",
			recipientState:   models.DmRequestStatePending,
			recipientHasTime: true,
			wantFolder:       models.UserConversationFolderRequests,
			wantState:        models.DmRequestStatePending,
			wantAcceptedAt:   false,
			wantRequestedAt:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conversation := createTestConversation("conv123", []string{"alice", "bob"})
			publishedAt := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)

			senderState := &models.UserConversationState{
				RequestState: models.DmRequestStatePending,
				DeletedAt:    ptrTime(time.Now()),
			}

			recipientState := &models.UserConversationState{
				RequestState: tc.recipientState,
				DeletedAt:    ptrTime(time.Now()),
			}
			if tc.recipientHasTime {
				tm := time.Now().UTC().Add(-time.Minute)
				recipientState.RequestedAt = &tm
			}

			states := buildDirectMessageParticipantStatesForSend(
				conversation,
				&models.Status{StatusID: "status-1", PublishedAt: publishedAt},
				"alice",
				"bob",
				senderState,
				recipientState,
				tc.deliversToInbox,
			)

			require.Len(t, states, 2)

			computedSenderState := states[0]
			require.Equal(t, models.DmRequestStateAccepted, computedSenderState.RequestState)
			require.NotNil(t, computedSenderState.AcceptedAt)
			require.Nil(t, computedSenderState.DeclinedAt)
			require.False(t, computedSenderState.Unread)

			computedRecipientState := states[1]
			require.Equal(t, tc.wantState, computedRecipientState.RequestState)
			require.Equal(t, tc.wantFolder, computedRecipientState.Folder)
			require.True(t, computedRecipientState.Unread)
			if tc.wantAcceptedAt {
				require.NotNil(t, computedRecipientState.AcceptedAt)
			}
			if tc.wantRequestedAt {
				require.NotNil(t, computedRecipientState.RequestedAt)
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
			On("CreateConversationWithParticipantStates", ctx, mock.AnythingOfType("*models.Conversation"), []string{"alice", "bob"}, mock.AnythingOfType("[]*models.UserConversationState")).
			Return(errors.New("boom")).
			Once()

		_, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.ErrorIs(t, err, ErrCreateConversation)
		accountRepo.AssertExpectations(t)
		conversationRepo.AssertExpectations(t)
	})

	t.Run("create_conversation_race_reloads_existing_lookup", func(t *testing.T) {
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
			On("CreateConversationWithParticipantStates", ctx, mock.AnythingOfType("*models.Conversation"), []string{"alice", "bob"}, mock.AnythingOfType("[]*models.UserConversationState")).
			Return(storage.ErrAlreadyExists).
			Once()

		existing := createTestConversation("conv-race", []string{"alice", "bob"})
		conversationRepo.
			On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
			Return(existing, nil).
			Once()

		conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-race").Return(
			testConversationStateContract("alice", "conv-race", nil),
			nil,
		).Once()

		result, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "conv-race", result.Conversation.ID)

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

		conversationRepo.On("GetUserConversationState", ctx, "alice", "conv123").Return(
			testConversationStateContract("alice", "conv123", func(state *interfaces.UserConversationStateContract) {
				state.Unread = true
			}),
			nil,
		).Once()

		result, err := service.CreateConversation(ctx, &CreateConversationCommand{CreatorID: "alice", ParticipantID: "bob"})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "conv123", result.Conversation.ID)
		require.True(t, result.Conversation.Unread)
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

	for _, recipient := range []string{"alice", "Alice", "alice@example.com", "https://example.com/users/alice"} {
		_, err = service.validateSendDirectMessageCommand(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{recipient},
			Content:    "hi",
		})
		require.ErrorIs(t, err, ErrInvalidRecipient)
		appErr, ok := apperrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, "DIRECT_SELF_POST_NOT_ALLOWED", appErr.Code.String())
	}

	recipientID, err := service.validateSendDirectMessageCommand(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"alice@remote.example"},
		Content:    "hi",
	})
	require.NoError(t, err)
	require.Equal(t, "alice@remote.example", recipientID)
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

		_, _, _, err := service.getDirectMessageAccounts(ctx, &SendDirectMessageCommand{
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

		_, _, _, err := service.getDirectMessageAccounts(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    "hi",
		}, "bob")
		require.ErrorIs(t, err, ErrInvalidRecipient)
	})
}

func TestService_getUserConversationStateForSend_ReturnsStateWhenPresent(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("GetUserConversationState", ctx, "bob", "conv123").
		Return(&interfaces.UserConversationStateContract{RequestState: models.DmRequestStateAccepted}, nil).
		Once()

	record, err := service.getUserConversationStateForSend(ctx, "conv123", "bob")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, models.DmRequestStateAccepted, record.RequestState)
	conversationRepo.AssertExpectations(t)
}

func TestService_createDirectMessageStatus_BuildsStatusWithoutPersistence(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob": createTestAccount("bob", "bob"),
	}, "conv123", "bob")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "conv123", status.ConversationID)
	require.Equal(t, []string{"https://example.com/users/bob"}, status.ToRecipients)
	require.Equal(t, status.ToRecipients, status.Note.To)
	require.Equal(t, []string{"https://example.com/users/bob"}, status.Mentions)
	require.Len(t, status.Note.Tag, 1)
	require.Equal(t, activitypub.Tag{
		Type: "Mention",
		Href: "https://example.com/users/bob",
		Name: "@bob",
	}, status.Note.Tag[0])
}

func TestService_createDirectMessageStatus_SanitizesContentAndSummary(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	rawContent := `<p onclick="steal()">hello</p><img src=x onerror="steal()"><script>alert(1)</script>`
	rawSpoiler := `<span onclick="steal()">cw</span><script>alert(2)</script>`

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:    "alice",
		Recipients:  []string{"bob"},
		Content:     rawContent,
		SpoilerText: rawSpoiler,
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob": createTestAccount("bob", "bob"),
	}, "conv123", "bob")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, status.Content, status.Note.Content)
	require.Contains(t, status.Content, "hello")
	require.NotContains(t, status.Content, "<script")
	require.NotContains(t, status.Content, "onclick")
	require.NotContains(t, status.Content, "onerror")
	require.NotContains(t, status.Content, "<img")
	require.Contains(t, status.Note.Summary, "cw")
	require.NotContains(t, status.Note.Summary, "<script")
	require.NotContains(t, status.Note.Summary, "onclick")
}

func TestService_createDirectMessageStatus_SetsAllCreationTimestamps(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob": createTestAccount("bob", "bob"),
	}, "conv123", "bob")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.False(t, status.PublishedAt.IsZero())
	require.Equal(t, status.PublishedAt, status.CreatedAt)
	require.Equal(t, status.PublishedAt, status.ModifiedAt)
	require.Equal(t, status.PublishedAt, status.UpdatedAt)
}

func TestService_createDirectMessageStatus_KeepsRemoteAudienceAndMentionsConsistent(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	remoteActorID := "https://remote.example/users/bob"

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{remoteActorID},
		Content:    "hi remote",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		remoteActorID: {
			User: &storage.User{Username: "bob@remote.example"},
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: remoteActorID},
				PreferredUsername: "bob",
			},
		},
	}, "conv123", remoteActorID)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, []string{remoteActorID}, status.ToRecipients)
	require.Equal(t, []string{remoteActorID}, status.Mentions)
	require.Equal(t, status.ToRecipients, status.Note.To)
	require.Len(t, status.Note.Tag, 1)
	require.Equal(t, activitypub.Tag{
		Type: "Mention",
		Href: remoteActorID,
		Name: "@bob@remote.example",
	}, status.Note.Tag[0])
}

func TestService_createDirectMessageStatus_RejectsRemoteHandleWithoutActorID(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	status, _, err := service.createDirectMessageStatus(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob@remote.example"},
		Content:    "hi remote",
	}, createTestAccount("alice", "alice"), map[string]*storage.Account{
		"bob@remote.example": {
			User: &storage.User{Username: "bob"},
		},
	}, "conv123", "bob@remote.example")
	require.Nil(t, status)
	require.ErrorIs(t, err, ErrInvalidRecipient)
	require.ErrorIs(t, err, errDirectMessageRemoteRecipientActorRequired)
}

func TestService_applyDirectMessageSendTransition_PropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(errors.New("boom")).
		Once()

	err := service.applyDirectMessageSendTransition(ctx, createTestConversation("conv123", []string{"alice", "bob"}), false, "alice", "bob", nil, nil, &models.Status{
		StatusID:    "msg-1",
		PublishedAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}, true)
	require.ErrorIs(t, err, ErrCreateDirectMessage)
	conversationRepo.AssertExpectations(t)
}

func TestService_applyDirectMessageSendTransition_PropagatesStatusMirrorErrors(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &nonTransactionalConversationRepo{}
	noteRepo := &mockNoteRepository{}
	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})

	publishedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(nil).
		Once()
	noteRepo.
		On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).
		Return(errors.New("boom")).
		Once()

	err := service.applyDirectMessageSendTransition(ctx, conversation, false, "alice", "bob", nil, nil, &models.Status{
		StatusID:    "msg-1",
		PublishedAt: publishedAt,
	}, true)
	require.ErrorIs(t, err, ErrCreateDirectMessage)
	require.Empty(t, conversation.LastStatusID)
	require.Zero(t, conversation.TotalMessageCount)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
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

		_, _, err := service.getSendMessageAccountsForSend(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
		}, nil, "alice", "bob")
		require.ErrorIs(t, err, ErrGetSenderAccount)
	})

	t.Run("recipient_lookup_fails", func(t *testing.T) {
		accountRepo := &mockAccountRepository{}
		service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
		accountRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, _, err := service.getSendMessageAccountsForSend(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
		}, nil, "alice", "bob")
		require.ErrorIs(t, err, ErrInvalidRecipient)
	})
}

func TestService_createSendMessageStatus_SetsAllCreationTimestamps(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

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
	require.False(t, status.PublishedAt.IsZero())
	require.Equal(t, status.PublishedAt, status.CreatedAt)
	require.Equal(t, status.PublishedAt, status.ModifiedAt)
	require.Equal(t, status.PublishedAt, status.UpdatedAt)
	require.Equal(t, []string{"https://example.com/users/bob"}, status.ToRecipients)
	require.Equal(t, []string{"https://example.com/users/bob"}, status.Mentions)
	require.Equal(t, status.ToRecipients, status.Note.To)
	require.Len(t, status.Note.Tag, 1)
}

func TestService_createSendMessageStatus_SanitizesContent(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	rawContent := `<p onclick="steal()">hello again</p><img src=x onerror="steal()"><script>alert(1)</script>`

	status, _, err := service.createSendMessageStatus(
		ctx,
		&SendMessageCommand{
			SenderID: "alice",
			Content:  rawContent,
		},
		&SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    rawContent,
		},
		createTestAccount("alice", "alice"),
		createTestAccount("bob", "bob"),
		"conv123",
		"bob",
	)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, status.Content, status.Note.Content)
	require.Contains(t, status.Content, "hello again")
	require.NotContains(t, status.Content, "<script")
	require.NotContains(t, status.Content, "onclick")
	require.NotContains(t, status.Content, "onerror")
	require.NotContains(t, status.Content, "<img")
}

func TestService_createSendMessageStatus_RejectsRemoteHandleWithoutActorID(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	status, _, err := service.createSendMessageStatus(
		ctx,
		&SendMessageCommand{
			SenderID: "alice",
			Content:  "hi again",
		},
		&SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob@remote.example"},
			Content:    "hi again",
		},
		createTestAccount("alice", "alice"),
		&storage.Account{User: &storage.User{Username: "bob"}},
		"conv123",
		"bob@remote.example",
	)
	require.Nil(t, status)
	require.ErrorIs(t, err, ErrInvalidRecipient)
	require.ErrorIs(t, err, errDirectMessageRemoteRecipientActorRequired)
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
		On("GetUserConversationsByFolder", ctx, "alice", models.UserConversationFolderRequests, mock.Anything).
		Return(paginated, nil).
		Once()
	_, err := service.ListConversations(ctx, &ListConversationsQuery{
		UserID:     "alice",
		Folder:     ConversationFolderRequests,
		Pagination: interfaces.PaginationOptions{Limit: 10},
	})
	require.NoError(t, err)

	conversationRepo.
		On("GetUserConversationsByFolder", ctx, "alice", models.UserConversationFolderInbox, mock.Anything).
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

func TestService_updateUserConversationState_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_mutator_is_noop", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
		require.NoError(t, service.updateUserConversationState(ctx, "conv123", "alice", nil))
	})

	t.Run("returns_error_from_repo", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.
			On("GetUserConversationState", ctx, "alice", "conv123").
			Return((*interfaces.UserConversationStateContract)(nil), errors.New("boom")).
			Once()

		err := service.updateUserConversationState(ctx, "conv123", "alice", func(*models.UserConversationState) {})
		require.Error(t, err)
	})

	t.Run("returns_not_found_when_record_nil", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

		conversationRepo.
			On("GetUserConversationState", ctx, "alice", "conv123").
			Return((*interfaces.UserConversationStateContract)(nil), nil).
			Once()

		err := service.updateUserConversationState(ctx, "conv123", "alice", func(*models.UserConversationState) {})
		require.ErrorIs(t, err, storage.ErrNotFound)
	})
}
