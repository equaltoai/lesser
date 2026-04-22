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

func TestStatusesContextAncestors_Round12(t *testing.T) {
	cfg := round11TestConfig()

	notesByID := map[string]*storagemodels.Status{
		"child": {
			StatusID:       "child",
			AuthorUsername: "alice",
			AuthorID:       cfg.ActorURL("alice"),
			Content:        "child",
			InReplyToID:    "parent",
		},
		"parent": {
			StatusID:       "parent",
			AuthorUsername: "alice",
			AuthorID:       cfg.ActorURL("alice"),
			Content:        "parent",
		},
	}

	notesSvc := &NotesServiceStub{
		GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
			if note, ok := notesByID[statusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		GetNoteWithViewerFunc: func(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
			if query == nil {
				return nil, errors.New("missing query")
			}
			if note, ok := notesByID[query.StatusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{}, nil
		},
	}

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
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

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(notesSvc, accountsSvc))

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/child/context", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "child"

	requireStatus(t, http.StatusOK)(handler.HandleGetStatusContextLift(ctx))
}

func TestStatusesContextAncestors_Round34_HydratesRemoteAncestorFromCachedActor(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/steward",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "steward",
		Name:              "Steward Remote",
		URL:               "https://remote.example/@steward",
		Inbox:             "https://remote.example/users/steward/inbox",
		Outbox:            "https://remote.example/users/steward/outbox",
	}
	cachedRemote := storagemodels.RemoteActor{
		Handle:    "steward@remote.example",
		Actor:     remoteActor,
		CachedAt:  now,
		UpdatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	cachedRemote.UpdateKeys()

	notesByID := map[string]*storagemodels.Status{
		"child": {
			StatusID:       "child",
			AuthorUsername: "alice",
			AuthorID:       cfg.ActorURL("alice"),
			Content:        "child",
			Visibility:     storagemodels.VisibilityPublic,
			InReplyToID:    "remote-parent",
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        cfg.BaseURL() + "/users/alice/statuses/child",
					Type:      activitypub.NoteType,
					Published: &now,
				},
				AttributedTo: cfg.ActorURL("alice"),
				Content:      "child",
			},
		},
		"remote-parent": {
			StatusID:       "remote-parent",
			AuthorUsername: "steward@remote.example",
			AuthorID:       remoteActor.ID,
			Content:        "remote parent",
			Visibility:     storagemodels.VisibilityPublic,
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
			URLs:           []string{"https://remote.example/users/steward/statuses/remote-parent"},
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://remote.example/users/steward/statuses/remote-parent",
					Type:      activitypub.NoteType,
					Published: &now,
				},
				AttributedTo: remoteActor.ID,
				Content:      "remote parent",
			},
		},
	}

	notesSvc := &NotesServiceStub{
		GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
			if note, ok := notesByID[statusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
			if query == nil {
				return nil, errors.New("missing query")
			}
			if note, ok := notesByID[query.StatusID]; ok {
				return note, nil
			}
			return nil, errors.New("not found")
		},
		ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{}, nil
		},
	}

	state := &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, makeRegistry(notesSvc, nil))

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/child/context", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "child"

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetStatusContextLift(ctx))

	var body apimodels.StatusContext
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Len(t, body.Ancestors, 1)
	require.Equal(t, remoteActor.ID, body.Ancestors[0].Account.ID)
	require.Equal(t, "steward", body.Ancestors[0].Account.Username)
	require.Equal(t, "steward@remote.example", body.Ancestors[0].Account.Acct)
	require.Equal(t, "https://remote.example/users/steward/statuses/remote-parent", body.Ancestors[0].URI)
	require.NotEmpty(t, body.Ancestors[0].Content)
}
