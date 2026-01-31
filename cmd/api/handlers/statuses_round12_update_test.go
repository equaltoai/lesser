package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
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
			Note:           &storagemodels.NoteField{Note: note},
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
				GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) { return status, nil },
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
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp := ctx.Response.Body.(apimodels.Status)
		require.Equal(t, "unlisted", resp.Visibility)
		require.Equal(t, "en", resp.Language)
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h := makeHandler(t, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", nil, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		h := makeHandler(t, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", map[string]string{"Authorization": "Bearer invalid"}, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		reg := makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{})
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
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
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("status not found returns 404", func(t *testing.T) {
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) { return nil, errors.New("not found") },
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) { return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil },
			},
		)
		h := makeHandler(t, &round10QueryState{}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
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
				GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) { return nil, errors.New("gone") },
			},
			&AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) { return &storage.Account{User: &storage.User{Username: "alice"}, Actor: makeActor("alice")}, nil },
			},
		)
		h := makeHandler(t, &round10QueryState{tombstonesByObjectID: map[string]storagemodels.Tombstone{objectID: tombstone}}, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "x"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusGone, ctx.Response.StatusCode)
	})

	t.Run("forbidden when updating someone else's status", func(t *testing.T) {
		status := makeStatus("bob")
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) { return status, nil },
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
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("invalid request format returns 400", func(t *testing.T) {
		status := makeStatus("alice")
		reg := makeRegistry(
			&NotesServiceStub{
				GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) { return status, nil },
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
		ctx.SetParam("id", "s1")
		require.NoError(t, h.HandleUpdateStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})
}

func TestUpdateStatusHelpers_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))

	ctx, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
	require.NoError(t, err)

	t.Run("convertMapToNote errors", func(t *testing.T) {
		_, err := handler.convertMapToNote(ctx, map[string]any{"x": make(chan int)})
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		_, err = handler.convertMapToNote(ctx2, map[string]any{"to": 123})
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, ctx2.Response.StatusCode)
	})

	t.Run("convertUnknownObjectToNote branches", func(t *testing.T) {
		type withoutAttr struct{}

		ctx3, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		_, err = handler.convertUnknownObjectToNote(ctx3, withoutAttr{}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, ctx3.Response.StatusCode)

		type withAttr struct {
			AttributedTo string `json:"attributedTo"`
		}

		ctx4, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		_, err = handler.convertUnknownObjectToNote(ctx4, withAttr{AttributedTo: cfg.ActorURL("bob")}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, ctx4.Response.StatusCode)

		type badTo struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			AttributedTo string `json:"attributedTo"`
			To           any    `json:"to"`
		}

		ctx5, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
		require.NoError(t, err)
		_, err = handler.convertUnknownObjectToNote(ctx5, badTo{
			ID:           cfg.ObjectURL("objects", "s1"),
			Type:         activitypub.NoteType,
			AttributedTo: cfg.ActorURL("alice"),
			To:           123,
		}, cfg.ActorURL("alice"))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, ctx5.Response.StatusCode)
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
			BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
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

