package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestHandleGetRelationshipsLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{User: &storage.User{Username: username}}, nil
		},
	}
	relSvc := &RelationshipsServiceStub{
		GetRelationshipFunc: func(ctx context.Context, requesterID, targetID string) (*relationships.RelationshipData, error) {
			return &relationships.RelationshipData{ID: targetID, Following: true, CreatedAt: time.Now()}, nil
		},
	}

	h.registry = &RegistryStub{AccountsSvc: accountsSvc, RelationshipsSvc: relSvc}

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"read:follows", auth.ScopeRead}, "sess-1")
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{"Authorization": "Bearer " + token}, map[string]string{"id": "bob,carol"}, nil)
	require.NoError(t, err)

	require.NoError(t, h.HandleGetRelationshipsLift(ctx))
}

func TestRelationshipsFullHandlers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	rel := &relationships.RelationshipData{ID: "bob", Following: true}
	relSvc := &RelationshipsServiceStub{
		FollowFunc: func(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
			return &relationships.FollowResult{Relationship: rel}, nil
		},
		UnfollowFunc: func(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		BlockFunc: func(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		UnblockFunc: func(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		MuteFunc: func(ctx context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		UnmuteFunc: func(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		GetRelationshipsFunc: func(ctx context.Context, query *relationships.GetRelationshipsQuery) (*relationships.Result, error) {
			return &relationships.Result{Relationships: []*relationships.RelationshipData{rel}}, nil
		},
	}

	h.registry = &RegistryStub{RelationshipsSvc: relSvc}

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeWrite, auth.ScopeRead}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxFollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", headers, nil, nil)
	require.NoError(t, err)
	ctxFollow.SetParam("id", "bob")
	require.NoError(t, h.HandleFollowAccountFull(ctxFollow))

	ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unfollow", headers, nil, nil)
	require.NoError(t, err)
	ctxUnfollow.SetParam("id", "bob")
	require.NoError(t, h.HandleUnfollowAccountFull(ctxUnfollow))

	ctxBlock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/block", headers, nil, nil)
	require.NoError(t, err)
	ctxBlock.SetParam("id", "bob")
	require.NoError(t, h.HandleBlockAccountFull(ctxBlock))

	ctxUnblock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unblock", headers, nil, nil)
	require.NoError(t, err)
	ctxUnblock.SetParam("id", "bob")
	require.NoError(t, h.HandleUnblockAccountFull(ctxUnblock))

	ctxMute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, nil)
	require.NoError(t, err)
	ctxMute.SetParam("id", "bob")
	require.NoError(t, h.HandleMuteAccountFull(ctxMute))

	ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctxUnmute.SetParam("id", "bob")
	require.NoError(t, h.HandleUnmuteAccountFull(ctxUnmute))

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", headers, map[string]string{"id": "bob"}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetRelationshipsFull(ctxGet))
}
