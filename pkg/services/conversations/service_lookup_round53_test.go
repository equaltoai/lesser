package conversations

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_LookupConversationByCounterpart_LocalIdentifiers(t *testing.T) {
	inputs := []string{
		"ops",
		"ops@example.com",
		"@ops@example.com",
		"acct:ops@example.com",
		"https://example.com/users/ops",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			service, conversationRepo, noteRepo, _, _, _ := createTestService()
			ctx := context.Background()
			conversation := createTestConversation("conv-lookup", []string{"alice", "ops"})
			message := createTestMessage("msg-1", "ops", "conv-lookup", "ready")
			message.AuthorUsername = "ops"
			message.ToRecipients = []string{"https://example.com/users/alice"}
			pagination := interfaces.PaginationOptions{Limit: 7}

			conversationRepo.
				On("GetConversationByParticipants", ctx, mock.MatchedBy(func(participants []string) bool {
					return fmt.Sprint(participants) == "[alice ops]"
				})).
				Return(conversation, nil).
				Once()
			conversationRepo.On("GetConversation", ctx, "conv-lookup").Return(conversation, nil).Once()
			conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-lookup").Return(
				testConversationStateContract("alice", "conv-lookup", func(state *interfaces.UserConversationStateContract) {
					state.Folder = models.UserConversationFolderInbox
				}), nil,
			).Once()
			noteRepo.On("GetConversationThread", ctx, "conv-lookup", pagination).Return(&interfaces.PaginatedResult[*models.Status]{
				Items: []*models.Status{message},
				Total: 1,
			}, nil).Once()

			result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
				ViewerID:    "alice",
				Counterpart: input,
				Pagination:  pagination,
			})
			require.NoError(t, err)
			require.Equal(t, "conv-lookup", result.Conversation.ID)
			require.Len(t, result.Messages.Items, 1)
			require.Equal(t, "msg-1", result.Messages.Items[0].StatusID)
			conversationRepo.AssertExpectations(t)
			noteRepo.AssertExpectations(t)
		})
	}
}

func TestService_LookupConversationByCounterpart_RemoteActorURLUsesTypedLookup(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()
	remoteActorID := "https://remote.example/users/Bob"
	refs := models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
		{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: remoteActorID, Acct: "bob@remote.example", Domain: "remote.example"},
	})
	conversation := createTestConversation("conv-remote", models.ConversationParticipantIDsFromRefs(refs))
	conversation.ParticipantRefs = refs
	message := createTestMessage("msg-remote", remoteActorID, "conv-remote", "federated hello")
	message.AuthorUsername = "bob"
	message.ToRecipients = []string{"https://example.com/users/alice"}
	pagination := interfaces.PaginationOptions{Limit: 3}

	conversationRepo.
		On("GetConversationByParticipantRefs", ctx, mock.MatchedBy(func(got []models.ConversationParticipantRef) bool {
			got = models.NormalizeConversationParticipantRefs(got)
			return len(got) == 2 &&
				got[0].ParticipantID == "alice" &&
				got[1].ParticipantID == remoteActorID &&
				got[1].ParticipantType == models.ConversationParticipantTypeRemoteActor &&
				got[1].Acct == "bob@remote.example"
		})).
		Return(conversation, nil).
		Once()
	conversationRepo.On("GetConversation", ctx, "conv-remote").Return(conversation, nil).Once()
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-remote").Return(
		testConversationStateContract("alice", "conv-remote", func(state *interfaces.UserConversationStateContract) {
			state.Folder = models.UserConversationFolderInbox
		}), nil,
	).Once()
	noteRepo.On("GetConversationThread", ctx, "conv-remote", pagination).Return(&interfaces.PaginatedResult[*models.Status]{
		Items: []*models.Status{message},
		Total: 1,
	}, nil).Once()

	result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
		ViewerID:    "alice",
		Counterpart: remoteActorID,
		Pagination:  pagination,
	})
	require.NoError(t, err)
	require.Equal(t, "conv-remote", result.Conversation.ID)
	require.Len(t, result.Messages.Items, 1)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_LookupConversationByCounterpart_RemoteAcctResolvesActor(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, federation := createTestService()
	ctx := context.Background()
	remoteActorID := "https://remote.example/users/bob"
	federation.actors = map[string]*activitypub.Actor{
		"bob@remote.example": {
			BaseObject:        activitypub.BaseObject{ID: remoteActorID, Type: activitypub.PersonType},
			PreferredUsername: "bob",
		},
	}
	refs := models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
		{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: remoteActorID, Acct: "bob@remote.example", Domain: "remote.example"},
	})
	conversation := createTestConversation("conv-remote-acct", models.ConversationParticipantIDsFromRefs(refs))
	conversation.ParticipantRefs = refs
	message := createTestMessage("msg-remote-acct", remoteActorID, "conv-remote-acct", "resolved hello")
	message.AuthorUsername = "bob"
	message.ToRecipients = []string{"https://example.com/users/alice"}
	pagination := interfaces.PaginationOptions{Limit: 4}

	conversationRepo.
		On("GetConversationByParticipantRefs", ctx, mock.MatchedBy(func(got []models.ConversationParticipantRef) bool {
			got = models.NormalizeConversationParticipantRefs(got)
			return len(got) == 2 &&
				got[0].ParticipantID == "alice" &&
				got[1].ParticipantID == remoteActorID &&
				got[1].Acct == "bob@remote.example" &&
				got[1].Domain == "remote.example" &&
				got[1].ResolvedAt != nil
		})).
		Return(conversation, nil).
		Once()
	conversationRepo.On("GetConversation", ctx, "conv-remote-acct").Return(conversation, nil).Once()
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-remote-acct").Return(
		testConversationStateContract("alice", "conv-remote-acct", func(state *interfaces.UserConversationStateContract) {
			state.Folder = models.UserConversationFolderInbox
		}), nil,
	).Once()
	noteRepo.On("GetConversationThread", ctx, "conv-remote-acct", pagination).Return(&interfaces.PaginatedResult[*models.Status]{
		Items: []*models.Status{message},
		Total: 1,
	}, nil).Once()

	result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
		ViewerID:    "alice",
		Counterpart: "bob@remote.example",
		Pagination:  pagination,
	})
	require.NoError(t, err)
	require.Equal(t, "conv-remote-acct", result.Conversation.ID)
	require.Len(t, result.Messages.Items, 1)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_LookupConversationByCounterpart_RemoteAcctResolutionFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Service, *mockFederationService)
	}{
		{
			name: "without resolver",
			setup: func(service *Service, _ *mockFederationService) {
				service.federation = nil
			},
		},
		{
			name: "resolver error",
			setup: func(_ *Service, federation *mockFederationService) {
				federation.resolveErr = errors.New("resolve failed")
			},
		},
		{
			name: "empty actor id",
			setup: func(_ *Service, federation *mockFederationService) {
				federation.actors = map[string]*activitypub.Actor{
					"bob@remote.example": {PreferredUsername: "bob"},
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service, conversationRepo, noteRepo, _, _, federation := createTestService()
			ctx := context.Background()
			testCase.setup(service, federation)

			result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
				ViewerID:    "alice",
				Counterpart: "bob@remote.example",
			})
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrConversationNotFound)
			conversationRepo.AssertNotCalled(t, "GetConversationByParticipants")
			conversationRepo.AssertNotCalled(t, "GetConversationByParticipantRefs")
			noteRepo.AssertNotCalled(t, "GetConversationThread")
		})
	}
}

func TestService_LookupConversationByCounterpart_InvalidInputs(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	result, err := service.LookupConversationByCounterpart(ctx, nil)
	require.Nil(t, result)
	require.Error(t, err)

	result, err = service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{ViewerID: "alice"})
	require.Nil(t, result)
	require.Error(t, err)

	result, err = service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{ViewerID: "alice", Counterpart: "@alice@example.com"})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrConversationNotFound)

	conversationRepo.AssertNotCalled(t, "GetConversationByParticipants")
	conversationRepo.AssertNotCalled(t, "GetConversationByParticipantRefs")
	noteRepo.AssertNotCalled(t, "GetConversationThread")
}

func TestConversationCounterpartLookupHelpers(t *testing.T) {
	require.Equal(t, "ops@example.com", normalizeConversationCounterpartLookupInput(" acct:@ops@example.com "))
	require.Equal(t, "ops@example.com", formatConversationLookupAcct("@Ops", "Example.COM"))
	require.Equal(t, "ops@example.com", formatConversationLookupAcct("Ops@Example.COM", "ignored.example"))
	require.Empty(t, formatConversationLookupAcct("", "example.com"))

	participants, refs, err := buildConversationLookupParticipants("alice", models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeLocalUser,
		ParticipantID:   "ops",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "ops"}, participants)
	require.Len(t, refs, 2)

	participants, refs, err = buildConversationLookupParticipants("alice", models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeLocalUser,
		ParticipantID:   "alice",
	})
	require.Nil(t, participants)
	require.Nil(t, refs)
	require.ErrorIs(t, err, ErrConversationNotFound)

	require.False(t, conversationIsOneToOne(nil))
	require.True(t, conversationIsOneToOne(createTestConversation("conv-local", []string{"alice", "ops"})))
	require.False(t, conversationIsOneToOne(createTestConversation("conv-group", []string{"alice", "bob", "carol"})))

	refConversation := createTestConversation("conv-refs", nil)
	refConversation.ParticipantRefs = []models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
		{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/bob"},
	}
	require.True(t, conversationIsOneToOne(refConversation))
	refConversation.ParticipantRefs = append(refConversation.ParticipantRefs,
		models.ConversationParticipantRef{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "carol"})
	require.False(t, conversationIsOneToOne(refConversation))
}

func TestService_LookupConversationByCounterpart_NotFound(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "missing"}).Return(nil, storage.ErrNotFound).Once()

	result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
		ViewerID:    "alice",
		Counterpart: "missing",
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrConversationNotFound)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertNotCalled(t, "GetConversationThread")
}

func TestService_LookupConversationByCounterpart_RepositoryEdgeFailures(t *testing.T) {
	cases := []struct {
		name         string
		conversation *models.Conversation
		err          error
		wantErr      error
	}{
		{
			name:    "lookup error",
			err:     errors.New("dynamodb unavailable"),
			wantErr: ErrLookupExistingConversation,
		},
		{
			name:    "nil conversation",
			wantErr: ErrConversationNotFound,
		},
		{
			name:         "empty id",
			conversation: createTestConversation("", []string{"alice", "ops"}),
			wantErr:      ErrConversationNotFound,
		},
		{
			name:         "group conversation",
			conversation: createTestConversation("conv-group", []string{"alice", "ops", "carol"}),
			wantErr:      ErrConversationMustBeOneToOne,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service, conversationRepo, noteRepo, _, _, _ := createTestService()
			ctx := context.Background()
			conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "ops"}).
				Return(testCase.conversation, testCase.err).
				Once()

			result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
				ViewerID:    "alice",
				Counterpart: "ops",
			})
			require.Nil(t, result)
			require.ErrorIs(t, err, testCase.wantErr)
			conversationRepo.AssertExpectations(t)
			noteRepo.AssertNotCalled(t, "GetConversationThread")
		})
	}
}

func TestService_LookupConversationByCounterpart_ReloadDeniesNonParticipant(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()
	conversation := createTestConversation("conv-other", []string{"bob", "ops"})

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "ops"}).Return(conversation, nil).Once()
	conversationRepo.On("GetConversation", ctx, "conv-other").Return(conversation, nil).Once()

	result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
		ViewerID:    "alice",
		Counterpart: "ops",
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNotConversationParticipant)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertNotCalled(t, "GetConversationThread")
}

func TestService_LookupConversationByCounterpart_RespectsHiddenViewerState(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()
	conversation := createTestConversation("conv-hidden", []string{"alice", "ops"})

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "ops"}).Return(conversation, nil).Once()
	conversationRepo.On("GetConversation", ctx, "conv-hidden").Return(conversation, nil).Once()
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-hidden").Return(
		testConversationStateContract("alice", "conv-hidden", func(state *interfaces.UserConversationStateContract) {
			state.Folder = models.UserConversationFolderHidden
		}), nil,
	).Once()

	result, err := service.LookupConversationByCounterpart(ctx, &LookupConversationByCounterpartQuery{
		ViewerID:    "alice",
		Counterpart: "ops",
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrConversationNotFound)
	conversationRepo.AssertExpectations(t)
	noteRepo.AssertNotCalled(t, "GetConversationThread")
}
