package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesGetAndTimelines_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	state := &round10QueryState{
		tombstonesByObjectID: map[string]storagemodels.Tombstone{
			cfg.BaseURL() + "/objects/gone": {
				PK:         "OBJECT#" + cfg.BaseURL() + "/objects/gone",
				SK:         "TOMBSTONE",
				ID:         cfg.BaseURL() + "/objects/gone",
				Type:       "Tombstone",
				FormerType: activitypub.NoteType,
				Deleted:    now.Add(-1 * time.Hour),
				DeletedBy:  "alice",
				Summary:    "test",
				CreatedAt:  now.Add(-2 * time.Hour),
				TTL:        now.Add(24 * time.Hour).Unix(),
			},
		},
	}

	notesByID := map[string]*storagemodels.Status{
		"status-1": {StatusID: "status-1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hi"},
	}

	listNotesResult := &notes.Result{
		Notes: []*storagemodels.Status{
			{StatusID: "status-1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hi", PublishedAt: now},
		},
	}

	notesSvc := &NotesServiceStub{
		GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
			if statusID == cfg.BaseURL()+"/objects/gone" {
				return nil, errors.New("not found")
			}
			if statusID == "err" {
				return nil, errors.New("boom")
			}
			if note, ok := notesByID[statusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		ListNotesFunc: func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
			if query != nil && query.TimelineType == "error" {
				return nil, errors.New("boom")
			}
			return listNotesResult, nil
		},
	}

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			if username == "err" {
				return nil, errors.New("boom")
			}
			return &storage.Account{
				User: &storage.User{Username: username, DisplayName: username},
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
					PreferredUsername: username,
					Name:              username,
				},
			}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, makeRegistry(notesSvc, accountsSvc))

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("get_status_invalid_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/%", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "%"
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetStatusLift(ctx))
	})

	t.Run("get_status_not_found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/missing", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetStatusLift(ctx))
	})

	t.Run("get_status_notes_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/err", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "err"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetStatusLift(ctx))
	})

	t.Run("get_status_account_error", func(t *testing.T) {
		notesByID["status-2"] = &storagemodels.Status{StatusID: "status-2", AuthorUsername: "err", AuthorID: cfg.ActorURL("err"), Content: "hi"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/status-2", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status-2"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetStatusLift(ctx))
	})

	t.Run("get_status_success_with_viewer", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/status-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"
		requireStatus(t, http.StatusOK)(handler.HandleGetStatusLift(ctx))
	})

	t.Run("home_timeline_auth_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetHomeTimelineLift(ctx))
	})

	t.Run("home_timeline_list_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", readHeaders, map[string]string{"max_id": "c1"}, nil)
		require.NoError(t, err)
		// Force error via special timeline type.
		orig := notesSvc.ListNotesFunc
		notesSvc.ListNotesFunc = func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
			if query != nil {
				query.TimelineType = "error"
			}
			return orig(ctx, query)
		}
		t.Cleanup(func() { notesSvc.ListNotesFunc = orig })

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetHomeTimelineLift(ctx))
	})

	t.Run("public_timeline_local_and_invalid_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", map[string]string{"Authorization": "Bearer bad"}, map[string]string{"local": "true"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleGetPublicTimelineLift(ctx))
	})

	t.Run("status_context_tombstoned", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/gone/context", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "gone"
		requireStatus(t, http.StatusGone)(handler.HandleGetStatusContextLift(ctx))
	})
}
