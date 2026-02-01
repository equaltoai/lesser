package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

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
		cfg.BaseURL() + "/objects/child": {
			StatusID:       "child",
			AuthorUsername: "alice",
			AuthorID:       cfg.ActorURL("alice"),
			Content:        "child",
			InReplyToID:    "parent",
		},
		cfg.BaseURL() + "/objects/parent": {
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
