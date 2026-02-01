package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestConversationHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"bob": {
				Username: "bob",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bob", Type: "Person"},
					PreferredUsername: "bob",
					Name:              "Bob",
				},
			},
		},
		statusByID: map[string]storagemodels.Status{
			"status-1": {
				StatusID:       "status-1",
				AuthorUsername: "bob",
				AuthorID:       "https://example.com/users/bob",
				Content:        "hello",
				CreatedAt:      now.Add(-1 * time.Hour),
				UpdatedAt:      now,
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		ConversationsSvc: &ConversationsServiceStub{
			ListConversationsFunc: func(ctx context.Context, query *conversations.ListConversationsQuery) (*conversations.Result, error) {
				return &conversations.Result{
					Conversations: &interfaces.PaginatedResult[*storagemodels.Conversation]{
						Items: []*storagemodels.Conversation{
							{
								ID:           "conv-1",
								Participants: []string{"alice", "bob"},
								LastStatusID: "status-1",
								Unread:       true,
							},
						},
						HasMore:    true,
						NextCursor: "next",
					},
				}, nil
			},
			DeleteConversationFunc: func(ctx context.Context, cmd *conversations.DeleteConversationCommand) (*conversations.ConversationResult, error) {
				return &conversations.ConversationResult{Conversation: &storagemodels.Conversation{ID: cmd.ConversationID}}, nil
			},
			MarkConversationReadFunc: func(ctx context.Context, cmd *conversations.MarkConversationReadCommand) (*conversations.ConversationResult, error) {
				return &conversations.ConversationResult{
					Conversation: &storagemodels.Conversation{
						ID:           cmd.ConversationID,
						Participants: []string{"alice", "bob"},
						Unread:       false,
					},
				}, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations", readHeaders, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetConversationsLift(ctxList))

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/conversations/conv-1", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxDelete.Params["id"] = "conv-1"
	requireStatus(t, http.StatusOK)(handler.HandleDeleteConversationLift(ctxDelete))

	ctxMark, err := round10NewLiftContext(http.MethodPost, "/api/v1/conversations/conv-1/read", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxMark.Params["id"] = "conv-1"
	requireStatus(t, http.StatusOK)(handler.HandleMarkConversationReadLift(ctxMark))
}
