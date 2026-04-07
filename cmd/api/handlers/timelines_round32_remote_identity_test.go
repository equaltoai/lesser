package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound32HandleGetHomeTimelineLift_PreservesRemoteAuthorParity(t *testing.T) {
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

	statusURL := "https://remote.example/users/bob/statuses/home-123"
	canonicalID := storagemodels.CanonicalStatusID(statusURL)
	remoteStatus := &storagemodels.Status{
		StatusID:       canonicalID,
		AuthorID:       remoteActor.ID,
		AuthorUsername: "bob@remote.example",
		Content:        "followed remote timeline note",
		Visibility:     storagemodels.VisibilityPublic,
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		URLs:           []string{statusURL},
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType, Published: &now},
			AttributedTo: remoteActor.ID,
			Content:      "followed remote timeline note",
		},
	}

	notesSvc := &NotesServiceStub{
		ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{Notes: []*storagemodels.Status{remoteStatus}}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
	}, makeRegistry(notesSvc, nil))

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", map[string]string{
		"Authorization": "Bearer " + readToken,
	}, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetHomeTimelineLift(ctx))

	var body []apimodels.Status
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Len(t, body, 1)
	require.Equal(t, statusURL, body[0].URL)
	require.Equal(t, remoteActor.URL, body[0].Account.URL)
	require.Equal(t, "bob@remote.example", body[0].Account.Acct)
}
