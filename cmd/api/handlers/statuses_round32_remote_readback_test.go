package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound32HandleGetStatusLift_ResolvesRemoteStatusURLAndPreservesRemoteIdentity(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Name:              "Bob Remote",
		URL:               "https://remote.example/@bob",
		Inbox:             "https://remote.example/users/bob/inbox",
		Outbox:            "https://remote.example/users/bob/outbox",
	}
	cachedRemote := storagemodels.RemoteActor{
		Handle:    "bob@remote.example",
		Actor:     remoteActor,
		CachedAt:  now,
		UpdatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	cachedRemote.UpdateKeys()

	statusURL := "https://remote.example/users/bob/statuses/abc-123"
	canonicalID := storagemodels.CanonicalStatusID(statusURL)
	state := &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
		statusByID: map[string]storagemodels.Status{
			canonicalID: {
				StatusID:       canonicalID,
				AuthorID:       remoteActor.ID,
				AuthorUsername: "bob@remote.example",
				Content:        "hello from remote",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				URLs:           []string{statusURL},
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType, Published: &now},
					AttributedTo: remoteActor.ID,
					Content:      "hello from remote",
				},
			},
		},
	}

	notesSvc := &NotesServiceStub{
		GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
			require.Equal(t, statusURL, query.StatusID)
			status := state.statusByID[canonicalID]
			return &status, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, makeRegistry(notesSvc, nil))

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/"+canonicalID, nil, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = statusURL

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetStatusLift(ctx))

	var body apimodels.Status
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, canonicalID, body.ID)
	require.Equal(t, statusURL, body.URI)
	require.Equal(t, statusURL, body.URL)
	require.Equal(t, remoteActor.ID, body.Account.ID)
	require.Equal(t, "bob", body.Account.Username)
	require.Equal(t, "bob@remote.example", body.Account.Acct)
	require.Equal(t, remoteActor.URL, body.Account.URL)
}

func TestRound32HandleGetStatusLift_RemoteURLMissReturnsNotFound(t *testing.T) {
	notesSvc := &NotesServiceStub{
		GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
			return nil, notes.ErrStatusNotFound
		},
	}
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{}, makeRegistry(notesSvc, nil))

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/remote-missing", nil, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "https://remote.example/users/bob/statuses/missing"

	requireStatus(t, http.StatusNotFound)(handler.HandleGetStatusLift(ctx))
}
