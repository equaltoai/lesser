package lift

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestUnifiedBoostHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		Followers:         "https://example.com/users/alice/followers",
	}
	objectID := cfg.BaseURL() + "/objects/status-1"

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {Username: "alice", Actor: actor},
		},
		objectsByID: map[string]storagemodels.Object{
			objectID: {
				ID:           objectID,
				Type:         activitypub.NoteType,
				Content:      "original",
				AttributedTo: actor.ID,
				Published:    time.Now().Add(-1 * time.Hour),
			},
		},
		quoteRelationships: []storagemodels.QuoteRelationship{
			{
				ID:           "note-1:status-1",
				QuoterNoteID: "note-1",
				TargetNoteID: objectID,
				QuoterID:     actor.ID,
				Timestamp:    time.Now().Add(-10 * time.Minute),
			},
		},
		announcesByKey: map[string]storagemodels.Announce{
			"OBJECT#" + objectID + "#ANNOUNCES|ACTOR#" + actor.ID: {
				PK:        "OBJECT#" + objectID + "#ANNOUNCES",
				SK:        "ACTOR#" + actor.ID,
				Actor:     actor.ID,
				Object:    objectID,
				ID:        "announce-1",
				Published: time.Now().Add(-5 * time.Minute),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}

	comment := "quote boost"
	ctxQuote, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &comment, Visibility: "public"})
	require.NoError(t, err)
	ctxQuote.SetParam("id", "status-1")
	require.NoError(t, handler.HandleUnifiedBoostLift(ctxQuote))
	require.Equal(t, http.StatusOK, ctxQuote.Response.StatusCode)

	ctxBoost, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{})
	require.NoError(t, err)
	ctxBoost.SetParam("id", "status-1")
	require.NoError(t, handler.HandleUnifiedBoostLift(ctxBoost))
	require.Equal(t, http.StatusOK, ctxBoost.Response.StatusCode)

	ctxUndo, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/unreblog", headers, nil, nil)
	require.NoError(t, err)
	ctxUndo.SetParam("id", "status-1")
	require.NoError(t, handler.HandleUndoUnifiedBoostLift(ctxUndo))
	require.Equal(t, http.StatusOK, ctxUndo.Response.StatusCode)

	require.NotEmpty(t, generateRandomStringForBoost())
}
