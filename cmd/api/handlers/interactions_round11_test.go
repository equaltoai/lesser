package lift

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
	ctxFollow.SetParam("id", "bob")
	require.NoError(t, handler.HandleFollowLift(ctxFollow))
	require.Equal(t, http.StatusOK, ctxFollow.Response.StatusCode)

	ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unfollow", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnfollow.SetParam("id", "bob")
	require.NoError(t, handler.HandleUnfollowLift(ctxUnfollow))
	require.Equal(t, http.StatusOK, ctxUnfollow.Response.StatusCode)

	ctxBlock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/block", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxBlock.SetParam("id", "bob")
	require.NoError(t, handler.HandleBlockLift(ctxBlock))
	require.Equal(t, http.StatusOK, ctxBlock.Response.StatusCode)

	ctxUnblock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unblock", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnblock.SetParam("id", "bob")
	require.NoError(t, handler.HandleUnblockLift(ctxUnblock))
	require.Equal(t, http.StatusOK, ctxUnblock.Response.StatusCode)

	ctxBlocks, err := round10NewLiftContext(http.MethodGet, "/api/v1/blocks", readHeaders, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetBlocksLift(ctxBlocks))
	require.Equal(t, http.StatusOK, ctxBlocks.Response.StatusCode)

	ctxFav, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/favourite", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxFav.SetParam("id", "1")
	require.NoError(t, handler.HandleFavoriteLift(ctxFav))
	require.Equal(t, http.StatusOK, ctxFav.Response.StatusCode)

	ctxUnfav, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/unfavourite", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnfav.SetParam("id", "1")
	require.NoError(t, handler.HandleUnfavoriteLift(ctxUnfav))
	require.Equal(t, http.StatusOK, ctxUnfav.Response.StatusCode)

	ctxReblog, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/reblog", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxReblog.SetParam("id", "1")
	require.NoError(t, handler.HandleReblogLift(ctxReblog))
	require.Equal(t, http.StatusOK, ctxReblog.Response.StatusCode)

	ctxUnreblog, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/unreblog", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxUnreblog.SetParam("id", "1")
	require.NoError(t, handler.HandleUnreblogLift(ctxUnreblog))
	require.Equal(t, http.StatusOK, ctxUnreblog.Response.StatusCode)

	ctxFavBy, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", nil, map[string]string{"limit": "2"}, nil)
	require.NoError(t, err)
	ctxFavBy.SetParam("id", "1")
	require.NoError(t, handler.HandleGetStatusFavouritedByLift(ctxFavBy))
	require.Equal(t, http.StatusOK, ctxFavBy.Response.StatusCode)

	ctxReblogBy, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/reblogged_by", nil, map[string]string{"limit": "2"}, nil)
	require.NoError(t, err)
	ctxReblogBy.SetParam("id", "1")
	require.NoError(t, handler.HandleGetStatusRebloggedByLift(ctxReblogBy))
	require.Equal(t, http.StatusOK, ctxReblogBy.Response.StatusCode)

}
