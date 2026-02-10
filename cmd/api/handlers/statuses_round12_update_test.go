package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestUpdateStatusLift_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	makeActor := func(username string) *activitypub.Actor {
		return &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.ActorURL(username),
				Type: "Person",
			},
			PreferredUsername: username,
			Name:              username,
		}
	}

	makeStatus := func(author string) *storagemodels.Status {
		authorActor := makeActor(author)
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.ObjectURL("objects", "s1"),
				Type: activitypub.NoteType,
				To:   []string{activitypub.PublicAddress},
			},
			AttributedTo: authorActor.ID,
			Content:      "hello",
		}
		return &storagemodels.Status{
			StatusID:       "s1",
			AuthorUsername: author,
			AuthorID:       authorActor.ID,
			Content:        "hello",
			PublishedAt:    now.Add(-1 * time.Hour),
			CreatedAt:      now.Add(-1 * time.Hour),
			UpdatedAt:      now.Add(-30 * time.Minute),
			Note:           note,
		}
	}

	makeRegistry := func(notesSvc NotesService, accountsSvc AccountsService) ServiceRegistry {
		return &RegistryStub{NotesSvc: notesSvc, AccountsSvc: accountsSvc}
	}

	makeHandler := func(t *testing.T, state *round10QueryState, reg ServiceRegistry) *Handler {
		t.Helper()
		h, _, _ := round11NewHandler(t, cfg, state, reg)
		return h
	}

	t.Run("success updates and returns status", func(t *testing.T) {
		status := makeStatus("alice")
		actor := makeActor("alice")
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) { return status, nil },
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{User: &storage.User{Username: "alice"}, Actor: actor}, nil
				},
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{
			Status:      "updated",
			SpoilerText: "warn",
			Sensitive:   true,
			Visibility:  "unlisted",
			Language:    "en",
		})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		resp := requireStatus(t, http.StatusOK)(h.HandleUpdateStatusLift(ctx))

		var body apimodels.Status
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "unlisted", body.Visibility)
		require.Equal(t, "en", body.Language)
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h := makeHandler(t, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", nil, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		h := makeHandler(t, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", map[string]string{"Authorization": "Bearer invalid"}, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		reg := makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{})
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("account lookup error returns 500", func(t *testing.T) {
		reg := makeRegistry(
			&NotesServiceStub{},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) { return nil, errors.New("boom") },
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("status not found returns 404", func(t *testing.T) {
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) {
					return nil, errors.New("not found")
				},
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil
				},
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNotFound)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("status tombstoned returns 410 with details", func(t *testing.T) {
		objectID := cfg.ObjectURL("objects", "s1")
		tombstone := storagemodels.Tombstone{
			ID:         objectID,
			Type:       "Tombstone",
			FormerType: activitypub.NoteType,
			Deleted:    now.Add(-2 * time.Hour),
		}
		require.NoError(t, tombstone.UpdateKeys())

		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) {
					return nil, errors.New("gone")
				},
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil
				},
			},
		)
		h := makeHandler(t, &round10QueryState{tombstonesByObjectID: map[string]storagemodels.Tombstone{objectID: tombstone}}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusGone)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("forbidden when updating someone else's status", func(t *testing.T) {
		status := makeStatus("bob")
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) { return status, nil },
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil
				},
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleUpdateStatusLift(ctx))
	})

	t.Run("invalid request format returns 400", func(t *testing.T) {
		status := makeStatus("alice")
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) { return status, nil },
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil
				},
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/statuses/s1", headers, nil, []byte("{"))
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateStatusLift(ctx))
	})
}

func TestUpdateStatusHelpers_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))

	ctx, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
	require.NoError(t, err)

	t.Run("convertMapToNote errors", func(t *testing.T) {
		note, resp, err := handler.convertMapToNote(ctx, map[string]any{"x": make(chan int)})
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)

		note2, resp2, err := handler.convertMapToNote(ctx2, map[string]any{"to": 123})
		require.NoError(t, err)
		require.Nil(t, note2)
		require.NotNil(t, resp2)
		require.Equal(t, http.StatusInternalServerError, resp2.Status)
	})

	t.Run("convertUnknownObjectToNote branches", func(t *testing.T) {
		type withoutAttr struct{}

		ctx3, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		note3, resp3, err := handler.convertUnknownObjectToNote(ctx3, withoutAttr{}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Nil(t, note3)
		require.NotNil(t, resp3)
		require.Equal(t, http.StatusInternalServerError, resp3.Status)

		type withAttr struct {
			AttributedTo string `json:"attributedTo"`
		}

		ctx4, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		note4, resp4, err := handler.convertUnknownObjectToNote(ctx4, withAttr{AttributedTo: cfg.ActorURL("bob")}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Nil(t, note4)
		require.NotNil(t, resp4)
		require.Equal(t, http.StatusForbidden, resp4.Status)

		type badTo struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			AttributedTo string `json:"attributedTo"`
			To           any    `json:"to"`
		}

		ctx5, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		note5, resp5, err := handler.convertUnknownObjectToNote(ctx5, badTo{
			ID:           cfg.ObjectURL("objects", "s1"),
			Type:         activitypub.NoteType,
			AttributedTo: cfg.ActorURL("alice"),
			To:           123,
		}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Nil(t, note5)
		require.NotNil(t, resp5)
		require.Equal(t, http.StatusInternalServerError, resp5.Status)
	})

	t.Run("extractUsernameFromToken", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctxWithAuth, err := round10NewLiftContext(http.MethodGet, "/status", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "alice", handler.extractUsernameFromToken(ctxWithAuth))

		ctxNoAuth, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "", handler.extractUsernameFromToken(ctxNoAuth))
	})

	t.Run("deliverUpdateActivity to local recipient", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
			PreferredUsername: "alice",
		}
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.ObjectURL("objects", "s1"),
				Type: activitypub.NoteType,
				To:   []string{activitypub.PublicAddress, cfg.ActorURL("bob")},
			},
			AttributedTo: cfg.ActorURL("alice"),
			Content:      "hello",
		}
		updateActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.BaseURL() + "/activities/update-test",
				Type: activitypub.UpdateType,
				To:   []string{cfg.ActorURL("bob")},
			},
			Actor:  actor.ID,
			Object: note,
		}

		require.NoError(t, handler.deliverUpdateActivity(context.Background(), updateActivity, actor, note))
	})

	t.Run("getStringFromMap", func(t *testing.T) {
		m := map[string]any{"a": "b"}
		require.Equal(t, "b", getStringFromMap(m, "a", "x"))
		require.Equal(t, "x", getStringFromMap(m, "missing", "x"))
	})
}

func makeRegistry(notesSvc NotesService, accountsSvc AccountsService) ServiceRegistry {
	return &RegistryStub{NotesSvc: notesSvc, AccountsSvc: accountsSvc}
}
