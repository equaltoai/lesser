package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestInteractionsRound13_AgentFollowRailsAndStatusInteractions(t *testing.T) {
	cfg := round10TestConfig()

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				Version:   1,
				IsAgent:   true,
				Suspended: false,
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKAgentGovernance,
				Username:  "agent",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-time.Hour),
				Version:   1,
			},
		},
	}

	reg := &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(context.Context, *relationships.FollowCommand) (*relationships.FollowResult, error) {
				return &relationships.FollowResult{Relationship: &relationships.RelationshipData{}}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			LikeNoteFunc: func(context.Context, *notes.LikeNoteCommand) (*notes.LikeResult, error) {
				return nil, assertErr("not found")
			},
			UnlikeNoteFunc: func(context.Context, *notes.UnlikeNoteCommand) (*notes.LikeResult, error) {
				return nil, assertErr("not found")
			},
			ReblogNoteFunc: func(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
				return nil, assertErr("not found")
			},
			UnreblogNoteFunc: func(context.Context, *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
				return nil, assertErr("not found")
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, reg)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}

	t.Run("agent follow invokes rails and succeeds", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusOK)(h.HandleFollowLift(ctx))
	})

	t.Run("favorite/unfavorite/reblog/unreblog not-found paths", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/favourite", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNotFound)(h.HandleFavoriteLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unfavourite", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNotFound)(h.HandleUnfavoriteLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/reblog", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNotFound)(h.HandleReblogLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unreblog", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNotFound)(h.HandleUnreblogLift(ctx))

		requireStatus(t, http.StatusBadRequest)(h.statusInteraction(ctx, "nope"))
	})

	t.Run("relationship operations reject without relationships service", func(t *testing.T) {
		hNoSvc, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusServiceUnavailable)(hNoSvc.HandleFollowLift(ctx))
	})
}

func TestInteractionsRound13_GetBlocksLinkHeader(t *testing.T) {
	cfg := round10TestConfig()

	reg := &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			GetBlockedUsersFunc: func(context.Context, *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error) {
				return &relationships.BlockedUsersResult{
					BlockedUsers: []*storage.Account{
						{Actor: activitypub.NewActor(activitypub.PersonType, "https://example.com/users/bob", "bob")},
					},
					NextCursor: "next",
				}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)
	userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + userToken}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/blocks", headers, map[string]string{"max_id": "cursor"}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetBlocksLift(ctx))
	require.NotEmpty(t, resp.Headers["link"])
}
