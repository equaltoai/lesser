package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storage "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestStatusesRound18_SaveUpdatedStatusBranches(t *testing.T) {
	cfg := round11TestConfig()

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.BaseURL() + "/objects/n1",
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress},
		},
		Content: "hi",
	}

	t.Run("missing auth header returns 500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/n1", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.saveUpdatedStatus(ctx, note)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("update history failure returns 500", func(t *testing.T) {
		state := &round10QueryState{firstErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/n1", headers, nil, nil)
		require.NoError(t, err)

		resp, err := h.saveUpdatedStatus(ctx, note)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("success returns nil response", func(t *testing.T) {
		objectID := note.ID
		pk := "object#" + objectID
		sk := "object#" + objectID
		state := &round10QueryState{notFoundPKSK: map[string]bool{pk + "#" + sk: true}}

		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/n1", headers, nil, nil)
		require.NoError(t, err)

		resp, err := h.saveUpdatedStatus(ctx, note)
		require.NoError(t, err)
		require.Nil(t, resp)
	})
}

func TestStatusesRound18_TimelineBranches(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AllowAgents = true
	now := time.Now()

	notesSvc := &NotesServiceStub{
		ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{
				Notes: []*storagemodels.Status{
					{StatusID: "bot-1", AuthorUsername: "bot", AuthorID: "https://remote.example/users/bot", Content: "hi", PublishedAt: now},
					{StatusID: "s-1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hello", PublishedAt: now},
				},
			}, nil
		},
	}

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"bot": {
				Username: "bot",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("bot"), Type: "Service"},
					PreferredUsername: "bot",
					Name:              "bot",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now,
			},
		},
		agentInstanceConfig: &storagemodels.AgentInstanceConfig{
			AllowAgents:       true,
			AllowRemoteAgents: false,
		},
	}

	reg := &RegistryStub{NotesSvc: notesSvc, AccountsSvc: &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{User: &storage.User{Username: username, DisplayName: username}}, nil
		},
	}}

	h, _, _ := round11NewHandler(t, cfg, state, reg)

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	t.Run("home timeline returns 403 for insufficient scope", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleGetHomeTimelineLift(ctx))
	})

	t.Run("home timeline exclude_agents filters bot accounts", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", headers, map[string]string{"exclude_agents": "true"}, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleGetHomeTimelineLift(ctx))

		var out []map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
	})

	t.Run("public timeline surfaces list error", func(t *testing.T) {
		regErr := &RegistryStub{NotesSvc: &NotesServiceStub{
			ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
				return nil, errors.New("boom")
			},
		}}
		hErr, _, _ := round11NewHandler(t, cfg, state, regErr)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(hErr.HandleGetPublicTimelineLift(ctx))
	})

	t.Run("public timeline hides remote agents when policy disallows", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", headers, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetPublicTimelineLift(ctx))
		var out []map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1) // only the local non-bot status remains
	})

	t.Run("public timeline exclude_agents filters bot accounts", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", headers, map[string]string{"exclude_agents": "true"}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetPublicTimelineLift(ctx))
		var out []map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
	})
}

func TestStatusesRound18_ValidateStatusIDForContext_InvalidID(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetNoteFunc: func(context.Context, string) (*storagemodels.Status, error) {
				return &storagemodels.Status{}, nil
			},
		},
	})

	ctx := &apptheory.Context{Params: map[string]string{"id": "%"}}
	_, resp, err := h.validateStatusIDForContext(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)
}

func TestStatusesRound18_DeliverUpdateActivity_NoRecipients(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
		PreferredUsername: "alice",
	}
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.BaseURL() + "/objects/n1",
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{activitypub.PublicAddress},
		},
	}
	updateActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.BaseURL() + "/activities/update-1",
			Type: activitypub.UpdateType,
		},
		Actor:  actor.ID,
		Object: note,
	}

	require.NoError(t, h.deliverUpdateActivity(context.Background(), updateActivity, actor, note))
}
