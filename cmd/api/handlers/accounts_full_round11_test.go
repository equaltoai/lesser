package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAccountsFullHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	now := time.Now()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: actor, User: &storage.User{Username: username, Role: "user"}}, nil
			},
			UpdateProfileFunc: func(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
				return &accounts.AccountResult{
					Account: &storage.Account{
						User:  &storage.User{Username: cmd.Username},
						Actor: &activitypub.Actor{PreferredUsername: cmd.Username},
					},
				}, nil
			},
			IsAccountPinnedFunc: func(ctx context.Context, userID, pinnedActorID string) (bool, error) {
				return true, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			ListNotesFunc: func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
				return &notes.Result{
					Notes: []*storagemodels.Status{
						{
							StatusID:       "status-1",
							AuthorUsername: "alice",
							AuthorID:       "https://example.com/users/alice",
							Content:        "hello",
							Visibility:     "public",
							CreatedAt:      now.Add(-1 * time.Hour),
							UpdatedAt:      now,
						},
					},
					Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{NextCursor: "next"},
				}, nil
			},
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) { return 3, nil },
			CountRepliesFunc:       func(ctx context.Context, statusID string) (int, error) { return 1, nil },
			GetBoostCountFunc:      func(ctx context.Context, statusID string) (int64, error) { return 2, nil },
			GetLikeCountFunc:       func(ctx context.Context, statusID string) (int64, error) { return 4, nil },
			HasLikedFunc:           func(ctx context.Context, userID, statusID string) (bool, error) { return true, nil },
			HasRebloggedFunc:       func(ctx context.Context, userID, statusID string) (bool, error) { return false, nil },
			IsBookmarkedFunc:       func(ctx context.Context, userID, statusID string) (bool, error) { return true, nil },
			GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
				return &storagemodels.Status{StatusID: statusID, AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"}, nil
			},
			GetUserTimelineFunc: func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
				return &notes.GetUserTimelineResult{Items: []*storagemodels.Status{}, NextCursor: ""}, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			GetFollowersFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{{Actor: actor}}, "next", nil
			},
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{{Actor: actor}}, "next", nil
			},
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
			IsMutedFunc:        func(ctx context.Context, userID, targetID string) (bool, error) { return false, nil },
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxGet.Params["id"] = "alice"
	requireStatus(t, http.StatusOK)(handler.HandleGetAccountFull(ctxGet))

	ctxVerify, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", readHeaders, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleVerifyCredentialsFull(ctxVerify))

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	updateReq := models.UpdateCredentialsRequest{DisplayName: "Alice", Note: "bio"}
	ctxUpdate, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", writeHeaders, nil, updateReq)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleUpdateCredentialsFull(ctxUpdate))

	ctxStatuses, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", readHeaders, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	ctxStatuses.Params["id"] = "alice"
	requireStatus(t, http.StatusOK)(handler.HandleGetAccountStatusesFull(ctxStatuses))

	ctxFollowers, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/followers", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxFollowers.Params["id"] = "alice"
	requireStatus(t, http.StatusOK)(handler.HandleGetAccountFollowersFull(ctxFollowers))

	ctxFollowing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/following", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxFollowing.Params["id"] = "alice"
	requireStatus(t, http.StatusOK)(handler.HandleGetAccountFollowingFull(ctxFollowing))

	actorResolved, err := handler.resolveAccountIDFull(context.Background(), "https://example.com/users/alice")
	require.NoError(t, err)
	require.NotNil(t, actorResolved)

	actorResolved, err = handler.resolveAccountIDFull(context.Background(), "@alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, actorResolved)

	account := handler.buildAccountResponseFull(context.Background(), actor)
	require.Equal(t, "alice", account["username"])

	status := &storagemodels.Status{
		StatusID:       "status-2",
		AuthorUsername: "alice",
		AuthorID:       "https://example.com/users/alice",
		Content:        "status",
		Visibility:     "public",
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now,
	}
	result := handler.convertStatusToMastodonAPI(status, "alice")
	require.Equal(t, "status-2", result["id"])
}
