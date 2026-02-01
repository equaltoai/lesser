package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesCRUDAndContext_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"}, PreferredUsername: "alice"}},
			"bob":   {Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"}, PreferredUsername: "bob"}},
		},
		statusByID: map[string]storagemodels.Status{
			"s1": {StatusID: "s1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hello", PublishedAt: now.Add(-1 * time.Hour)},
		},
		objectsByID: map[string]storagemodels.Object{
			cfg.BaseURL() + "/objects/parent": {
				ID:           cfg.BaseURL() + "/objects/parent",
				Type:         activitypub.NoteType,
				Content:      "parent",
				Published:    now.Add(-2 * time.Hour),
				AttributedTo: cfg.ActorURL("alice"),
			},
		},
		objectList: []storagemodels.Object{
			{
				ID:           cfg.BaseURL() + "/objects/reply",
				Type:         activitypub.NoteType,
				Content:      "reply",
				Published:    now.Add(-30 * time.Minute),
				AttributedTo: cfg.ActorURL("bob"),
				InReplyTo:    ptrString(cfg.BaseURL() + "/objects/parent"),
			},
		},
	}

	notesByID := map[string]*storagemodels.Status{
		"s1": {StatusID: "s1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hello", PublishedAt: now.Add(-1 * time.Hour)},
	}

	notesSvc := &NotesServiceStub{
		CreateNoteFunc: func(ctx context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
			if cmd == nil || cmd.AuthorID == "" {
				return nil, errors.New("bad cmd")
			}
			return &notes.NoteResult{Note: &storagemodels.Status{
				StatusID:       "created-1",
				AuthorUsername: cmd.AuthorID,
				AuthorID:       cfg.ActorURL(cmd.AuthorID),
				Content:        cmd.Content,
				PublishedAt:    now,
			}}, nil
		},
		GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
			if note, ok := notesByID[statusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		DeleteNoteFunc: func(ctx context.Context, cmd *notes.DeleteNoteCommand) error {
			if cmd != nil && cmd.StatusID == "forbidden" {
				return errors.New("not authorized")
			}
			if cmd != nil && cmd.StatusID == "missing" {
				return errors.New("not found")
			}
			if cmd != nil && cmd.StatusID == "boom" {
				return errors.New("boom")
			}
			return nil
		},
	}

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			actor := &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
				PreferredUsername: username,
				Name:              username,
			}
			return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, makeRegistry(notesSvc, accountsSvc))

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("create_invalid_json", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, []byte("{"))
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateStatusLift(ctx))
	})

	t.Run("create_invalid_params", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, &models.CreateStatusRequest{Status: ""})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateStatusLift(ctx))
	})

	t.Run("create_missing_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, &models.CreateStatusRequest{Status: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateStatusLift(ctx))
	})

	t.Run("create_notes_error", func(t *testing.T) {
		orig := notesSvc.CreateNoteFunc
		notesSvc.CreateNoteFunc = func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
			return nil, errors.New("boom")
		}
		t.Cleanup(func() { notesSvc.CreateNoteFunc = orig })

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, &models.CreateStatusRequest{Status: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateStatusLift(ctx))
	})

	t.Run("create_success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, &models.CreateStatusRequest{Status: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusCreated)(handler.HandleCreateStatusLift(ctx))
	})

	t.Run("delete_invalid_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_missing_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_get_not_found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/missing", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_delete_not_found", func(t *testing.T) {
		notesByID["missing"] = &storagemodels.Status{StatusID: "missing", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hi"}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/missing", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_forbidden", func(t *testing.T) {
		notesByID["forbidden"] = &storagemodels.Status{StatusID: "forbidden", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hi"}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/forbidden", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "forbidden"
		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_internal_error", func(t *testing.T) {
		notesByID["boom"] = &storagemodels.Status{StatusID: "boom", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hi"}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/boom", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "boom"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("delete_success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusOK)(handler.HandleDeleteStatusLift(ctx))
	})

	t.Run("context_descendants_and_helpers", func(t *testing.T) {
		notesByID[cfg.BaseURL()+"/objects/parent"] = &storagemodels.Status{
			StatusID:       "parent",
			AuthorUsername: "alice",
			AuthorID:       cfg.ActorURL("alice"),
			Content:        "parent",
		}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/parent/context", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "parent"
		requireStatus(t, http.StatusOK)(handler.HandleGetStatusContextLift(ctx))

		require.NotNil(t, handler.loadStatusWithActor(context.Background(), "s1"))
	})
}

func ptrString(s string) *string { return &s }
