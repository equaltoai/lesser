package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestInteractionsHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	handler.registry = &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
				return &relationships.FollowResult{Relationship: &relationships.RelationshipData{ID: cmd.FollowingID, Following: true}}, nil
			},
			UnfollowFunc: func(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.FollowingID}}, nil
			},
			BlockFunc: func(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.BlockedID, Blocking: true}}, nil
			},
			UnblockFunc: func(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.BlockedID}}, nil
			},
			GetBlockedUsersFunc: func(ctx context.Context, query *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error) {
				return &relationships.BlockedUsersResult{
					BlockedUsers: []*storage.Account{
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"}},
					},
					NextCursor: "next",
				}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			LikeNoteFunc: func(ctx context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID, Content: "hi"}}, nil
			},
			UnlikeNoteFunc: func(ctx context.Context, cmd *notes.UnlikeNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID, Content: "hi"}}, nil
			},
			ReblogNoteFunc: func(ctx context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID, Content: "hi"}}, nil
			},
			UnreblogNoteFunc: func(ctx context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID, Content: "hi"}}, nil
			},
			GetLikersFunc: func(ctx context.Context, query *notes.GetLikersQuery) (*notes.UsersResult, error) {
				return &notes.UsersResult{
					Users: []*storage.Account{
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"}},
					},
					Pagination: &interfaces.PaginatedResult[*storage.Account]{NextCursor: "like-next"},
				}, nil
			},
			GetRebloggersFunc: func(ctx context.Context, query *notes.GetRebloggersQuery) (*notes.UsersResult, error) {
				return &notes.UsersResult{
					Users: []*storage.Account{
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/carol"}, PreferredUsername: "carol"}},
					},
					Pagination: &interfaces.PaginatedResult[*storage.Account]{NextCursor: "reblog-next"},
				}, nil
			},
		},
	}

	ctxFollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxFollow.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(handler.HandleFollowLift(ctxFollow))

	ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unfollow", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnfollow.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(handler.HandleUnfollowLift(ctxUnfollow))

	ctxBlock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/block", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxBlock.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(handler.HandleBlockLift(ctxBlock))

	ctxUnblock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unblock", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnblock.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(handler.HandleUnblockLift(ctxUnblock))

	ctxBlocks, err := round10NewLiftContext(http.MethodGet, "/api/v1/blocks", readHeaders, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetBlocksLift(ctxBlocks))

	ctxFav, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/favourite", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxFav.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleFavoriteLift(ctxFav))

	ctxUnfav, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/unfavourite", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnfav.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleUnfavoriteLift(ctxUnfav))

	ctxReblog, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/reblog", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxReblog.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleReblogLift(ctxReblog))

	ctxUnreblog, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/unreblog", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnreblog.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleUnreblogLift(ctxUnreblog))

	ctxFavBy, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", readHeaders, map[string]string{"limit": "2"}, nil)
	require.NoError(t, err)
	ctxFavBy.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleGetStatusFavouritedByLift(ctxFavBy))

	ctxReblogBy, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/reblogged_by", readHeaders, map[string]string{"limit": "2"}, nil)
	require.NoError(t, err)
	ctxReblogBy.Params["id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleGetStatusRebloggedByLift(ctxReblogBy))

}
