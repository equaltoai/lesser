package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
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

	handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: &NotesServiceStub{
		ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
			require.Equal(t, "alice", viewerID)
			require.Equal(t, "status-1", rawQuoteTarget)
			return quoteBoostTarget("status-1", objectID, storagemodels.VisibilityPublic), nil
		},
		ReblogNoteFunc: func(_ context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
			return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID}}, nil
		},
	}})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}

	comment := "quote boost"
	ctxQuote, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &comment, Visibility: "public"})
	require.NoError(t, err)
	ctxQuote.Params["id"] = "status-1"
	requireStatus(t, http.StatusOK)(handler.HandleUnifiedBoostLift(ctxQuote))

	ctxBoost, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{})
	require.NoError(t, err)
	ctxBoost.Params["id"] = "status-1"
	requireStatus(t, http.StatusOK)(handler.HandleUnifiedBoostLift(ctxBoost))

	ctxUndo, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/unreblog", headers, nil, nil)
	require.NoError(t, err)
	ctxUndo.Params["id"] = "status-1"
	requireStatus(t, http.StatusOK)(handler.HandleUndoUnifiedBoostLift(ctxUndo))

	require.NotEmpty(t, generateRandomStringForBoost())
}
