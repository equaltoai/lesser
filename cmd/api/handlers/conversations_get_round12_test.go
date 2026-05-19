package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleGetConversationLift_ReturnsExpandedConversation(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
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
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		ConversationsSvc: &ConversationsServiceStub{
			GetConversationFunc: func(_ context.Context, query *conversations.GetConversationQuery) (*conversations.ConversationWithMessages, error) {
				require.Equal(t, "alice", query.ViewerID)
				require.Equal(t, "conv-1", query.ConversationID)
				require.Equal(t, 5, query.Pagination.Limit)
				return &conversations.ConversationWithMessages{
					Conversation: &storagemodels.Conversation{
						ID:           "conv-1",
						Participants: []string{"alice", "bob"},
						LastStatusID: "status-2",
						Unread:       true,
					},
					Messages: &interfaces.PaginatedResult[*storagemodels.Status]{
						Items: []*storagemodels.Status{
							{
								StatusID:       "status-1",
								AuthorID:       "https://example.com/users/bob",
								AuthorUsername: "bob",
								Content:        "hello alice",
								Visibility:     conversations.VisibilityDirect,
								ConversationID: "conv-1",
								PublishedAt:    now.Add(-time.Minute),
								CreatedAt:      now.Add(-time.Minute),
								UpdatedAt:      now.Add(-time.Minute),
							},
						},
						HasMore:    true,
						NextCursor: "cursor-2",
					},
				}, nil
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations/conv-1", map[string]string{
		"Authorization": "Bearer " + token,
	}, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "conv-1"

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetConversationLift(ctx))
	require.Contains(t, resp.Headers["link"][0], "/api/v1/conversations/conv-1?max_id=cursor-2&limit=5")

	var body apimodels.ConversationDetail
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "conv-1", body.ID)
	require.True(t, body.Unread)
	require.Len(t, body.Accounts, 1)
	require.Equal(t, "bob", body.Accounts[0].Username)
	require.Len(t, body.Messages, 1)
	require.Equal(t, "status-1", body.Messages[0].ID)
}

func TestHandleGetConversationLift_DeniesUnavailableConversation(t *testing.T) {
	cfg := round10TestConfig()
	tests := []struct {
		name string
		err  error
	}{
		{name: "not found", err: conversations.ErrConversationNotFound},
		{name: "not participant", err: conversations.ErrNotConversationParticipant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg)
			handler.registry = &RegistryStub{
				ConversationsSvc: &ConversationsServiceStub{
					GetConversationFunc: func(context.Context, *conversations.GetConversationQuery) (*conversations.ConversationWithMessages, error) {
						return nil, tt.err
					},
				},
			}

			token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations/conv-hidden", map[string]string{
				"Authorization": "Bearer " + token,
			}, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "conv-hidden"

			requireStatus(t, http.StatusNotFound)(handler.HandleGetConversationLift(ctx))
		})
	}
}

func TestHandleLookupConversationByCounterpartLift_ReturnsExpandedConversation(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"ops": {
				Username: "ops",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/ops", Type: "Person"},
					PreferredUsername: "ops",
					Name:              "Ops",
				},
			},
		},
	})

	handler.registry = &RegistryStub{
		ConversationsSvc: &ConversationsServiceStub{
			LookupConversationByCounterpartFunc: func(_ context.Context, query *conversations.LookupConversationByCounterpartQuery) (*conversations.ConversationWithMessages, error) {
				require.Equal(t, "alice", query.ViewerID)
				require.Equal(t, "ops", query.Counterpart)
				require.Equal(t, 2, query.Pagination.Limit)
				return &conversations.ConversationWithMessages{
					Conversation: &storagemodels.Conversation{
						ID:           "conv-ops",
						Participants: []string{"alice", "ops"},
					},
					Messages: &interfaces.PaginatedResult[*storagemodels.Status]{
						Items: []*storagemodels.Status{
							{
								StatusID:       "status-ops",
								AuthorID:       "https://example.com/users/ops",
								AuthorUsername: "ops",
								Content:        "ack",
								Visibility:     conversations.VisibilityDirect,
								ConversationID: "conv-ops",
								PublishedAt:    now,
								CreatedAt:      now,
								UpdatedAt:      now,
							},
						},
						HasMore:    true,
						NextCursor: "cursor-ops",
					},
				}, nil
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations/lookup", map[string]string{
		"Authorization": "Bearer " + token,
	}, map[string]string{"counterpart": "ops", "limit": "2"}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleLookupConversationByCounterpartLift(ctx))
	require.Contains(t, resp.Headers["link"][0], "/api/v1/conversations/lookup?")
	require.Contains(t, resp.Headers["link"][0], "counterpart=ops")
	require.Contains(t, resp.Headers["link"][0], "max_id=cursor-ops")

	var body apimodels.ConversationDetail
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "conv-ops", body.ID)
	require.Len(t, body.Messages, 1)
	require.Equal(t, "status-ops", body.Messages[0].ID)
}

func TestHandleLookupConversationByCounterpartLift_RequiresCounterpart(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations/lookup", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(handler.HandleLookupConversationByCounterpartLift(ctx))
}

func TestHandleLookupConversationByCounterpartLift_NotFound(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg)
	handler.registry = &RegistryStub{
		ConversationsSvc: &ConversationsServiceStub{
			LookupConversationByCounterpartFunc: func(context.Context, *conversations.LookupConversationByCounterpartQuery) (*conversations.ConversationWithMessages, error) {
				return nil, conversations.ErrConversationNotFound
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/conversations/lookup", map[string]string{
		"Authorization": "Bearer " + token,
	}, map[string]string{"counterpart": "missing"}, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusNotFound)(handler.HandleLookupConversationByCounterpartLift(ctx))
}
