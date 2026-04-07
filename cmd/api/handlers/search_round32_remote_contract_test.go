package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound32HandleSearchV2Lift_ExactRemoteHandleUsesRemoteAwareResolution(t *testing.T) {
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

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":       "bob@remote.example",
		"type":    "accounts",
		"limit":   "5",
		"resolve": "true",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctx))

	var body apimodels.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Len(t, body.Accounts, 1)
	require.Empty(t, body.Statuses)
	require.Equal(t, remoteActor.ID, body.Accounts[0].ID)
	require.Equal(t, "bob", body.Accounts[0].Username)
	require.Equal(t, "bob@remote.example", body.Accounts[0].Acct)
	require.Equal(t, remoteActor.URL, body.Accounts[0].URL)
}

func TestRound32SearchV2ExactActorResult_RemoteHandleRequiresResolve(t *testing.T) {
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

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
	})

	result, handled, err := handler.searchV2ExactActorResult(t.Context(), &SearchParams{
		Query: "bob@remote.example",
		Type:  "accounts",
		Limit: 5,
	})
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, result)
}

func TestRound32SearchV2ExactActorResult_LocalActorNotFoundReturnsEmptyResult(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		notFoundPKs: map[string]bool{
			"ACTOR#missing": true,
		},
	})

	result, handled, err := handler.searchV2ExactActorResult(t.Context(), &SearchParams{
		Query: "https://example.com/users/missing",
		Type:  "accounts",
		Limit: 5,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, result)
	require.Empty(t, result.Accounts)
	require.Empty(t, result.Statuses)
	require.Empty(t, result.Hashtags)
}

func TestRound32HandleSearchV2Lift_ExactActorResolutionErrorReturnsInternalServerError(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		firstErrorOnce: errors.New("boom"),
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":     "https://example.com/users/alice",
		"type":  "accounts",
		"limit": "5",
	}, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(handler.HandleSearchV2Lift(ctx))
}

func TestRound32HandleSearchV2Lift_RemoteStatusURLUsesMaterializedCanonicalStatus(t *testing.T) {
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
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cachedRemote.PK: cachedRemote,
		},
		statusByID: map[string]storagemodels.Status{
			canonicalID: {
				StatusID:       canonicalID,
				AuthorID:       remoteActor.ID,
				AuthorUsername: "bob@remote.example",
				Content:        "materialized remote status",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				URLs:           []string{statusURL},
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType, Published: &now},
					AttributedTo: remoteActor.ID,
					Content:      "materialized remote status",
				},
			},
		},
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":     statusURL,
		"type":  "statuses",
		"limit": "5",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctx))

	var body apimodels.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Empty(t, body.Accounts)
	require.Len(t, body.Statuses, 1)
	require.Equal(t, canonicalID, body.Statuses[0].ID)
	require.Equal(t, statusURL, body.Statuses[0].URL)
	require.Equal(t, remoteActor.URL, body.Statuses[0].Account.URL)
}
