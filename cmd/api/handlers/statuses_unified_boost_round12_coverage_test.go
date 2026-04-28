package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestUnifiedBoostRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("small helpers", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		require.Equal(t, cfg.BaseURL()+"/objects/123", h.normalizeBoostObjectID("123"))
		require.Equal(t, "https://example.com/objects/123", h.normalizeBoostObjectID("https://example.com/objects/123"))

		require.Equal(t, "alice", h.extractActorIDFromObject(&activitypub.Note{AttributedTo: "alice"}))
		require.Equal(t, "alice", h.extractActorIDFromObject(map[string]any{"attributedTo": "alice"}))
		require.Equal(t, "", h.extractActorIDFromObject(map[string]any{"attributedTo": 123}))

		require.Equal(t, "hello", h.extractContentFromObject(&activitypub.Note{Content: "hello"}))
		require.Equal(t, "mapped", h.extractContentFromObject(map[string]any{"content": "mapped"}))
		require.Equal(t, "", h.extractContentFromObject(map[string]any{"content": 123}))
	})

	t.Run("handle unified boost: missing id + unauthorized", func(t *testing.T) {
		state := &round10QueryState{}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctxMissingID, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses//reblog", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleUnifiedBoostLift(ctxMissingID))

		ctxUnauthed, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/reblog", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthed.Params["id"] = "123"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUnifiedBoostLift(ctxUnauthed))
	})

	t.Run("handle unified boost: invalid json is rejected", func(t *testing.T) {
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"}}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/123/reblog", headers, nil, []byte(`{invalid}`))
		ctx.Params["id"] = "123"

		requireStatus(t, http.StatusBadRequest)(h.HandleUnifiedBoostLift(ctx))
	})

	t.Run("handle unified boost: rejects oversized and overlong quote comments", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		tooLarge := []byte(`{"comment":"` + strings.Repeat("a", maxUnifiedBoostRequestBodyBytes) + `"}`)
		ctxTooLarge := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/123/reblog", headers, nil, tooLarge)
		ctxTooLarge.Params["id"] = "123"
		requireStatus(t, http.StatusBadRequest)(h.HandleUnifiedBoostLift(ctxTooLarge))

		tooLong := strings.Repeat("a", common.MaxStatusLength+1)
		ctxTooLong, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/reblog", headers, nil, models.ReblogRequest{Comment: &tooLong})
		require.NoError(t, err)
		ctxTooLong.Params["id"] = "123"
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleUnifiedBoostLift(ctxTooLong))
	})

	t.Run("pure boost: create announce error", func(t *testing.T) {
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"}}},
			},
			createErrorOnce: errors.New("create failed"),
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/reblog", headers, nil, models.ReblogRequest{})
		require.NoError(t, err)
		ctx.Params["id"] = "123"

		requireStatus(t, http.StatusInternalServerError)(h.HandleUnifiedBoostLift(ctx))
	})

	t.Run("quote boost: unlisted/private/direct + quoted actor lookup fallback", func(t *testing.T) {
		boostActor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
			Followers:         cfg.BaseURL() + "/users/alice/followers",
			Icon:              &activitypub.Image{URL: "https://example.com/a.png"},
			Image:             &activitypub.Image{URL: "https://example.com/h.png"},
		}

		objectID := cfg.BaseURL() + "/objects/status-1"
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: boostActor},
			},
			objectsByID: map[string]storagemodels.Object{
				// Make quoted object attributed to "bob" so quoted actor lookup can be forced to fail.
				objectID: {ID: objectID, Type: activitypub.NoteType, Content: "original", AttributedTo: "bob", Published: time.Now().Add(-1 * time.Hour)},
			},
			notFoundPKSK: map[string]bool{
				"ACTOR#bob#PROFILE": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		comment := "quote"
		for _, vis := range []string{"unlisted", "private", "direct"} {
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &comment, Visibility: vis})
			require.NoError(t, err)
			ctx.Params["id"] = "status-1"
			requireStatus(t, http.StatusOK)(h.HandleUnifiedBoostLift(ctx))
		}

		empty := ""
		ctxEmpty, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &empty})
		require.NoError(t, err)
		ctxEmpty.Params["id"] = "status-1"
		requireStatus(t, http.StatusOK)(h.HandleUnifiedBoostLift(ctxEmpty))
	})

	t.Run("undo handler: missing id + auth errors", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctxMissingID, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses//unreblog", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleUndoUnifiedBoostLift(ctxMissingID))

		ctxMissingAuth, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/unreblog", nil, nil, nil)
		require.NoError(t, err)
		ctxMissingAuth.Params["id"] = "123"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUndoUnifiedBoostLift(ctxMissingAuth))

		ctxInvalidAuth, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/unreblog", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		ctxInvalidAuth.Params["id"] = "123"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUndoUnifiedBoostLift(ctxInvalidAuth))

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctxScope, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/unreblog", headers, nil, nil)
		require.NoError(t, err)
		ctxScope.Params["id"] = "123"
		requireStatus(t, http.StatusForbidden)(h.HandleUndoUnifiedBoostLift(ctxScope))
	})

	t.Run("undo handler: idempotent when nothing to undo", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/status-1"
		actorID := cfg.BaseURL() + "/users/alice"

		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"OBJECT#" + objectID + "#ANNOUNCES#ACTOR#" + actorID: true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/unreblog", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"

		requireStatus(t, http.StatusOK)(h.HandleUndoUnifiedBoostLift(ctx))
	})

	t.Run("undo: traditional delete + create activity errors", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.BaseURL() + "/users/alice",
				Type: "Person",
			},
			PreferredUsername: "alice",
		}
		objectID := cfg.BaseURL() + "/objects/status-1"
		announceKey := "OBJECT#" + objectID + "#ANNOUNCES|ACTOR#" + actor.ID

		deleteState := &round10QueryState{
			announcesByKey: map[string]storagemodels.Announce{
				announceKey: {PK: "OBJECT#" + objectID + "#ANNOUNCES", SK: "ACTOR#" + actor.ID, Actor: actor.ID, Object: objectID, ID: "announce-1"},
			},
			deleteErrorOnce: errors.New("delete failed"),
		}
		hDel, _, _ := round11NewHandler(t, cfg, deleteState, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		require.False(t, hDel.undoTraditionalBoost(ctx, actor, objectID))

		createState := &round10QueryState{
			announcesByKey: map[string]storagemodels.Announce{
				announceKey: {PK: "OBJECT#" + objectID + "#ANNOUNCES", SK: "ACTOR#" + actor.ID, Actor: actor.ID, Object: objectID, ID: "announce-1"},
			},
			createErrorOnce: errors.New("create failed"),
		}
		hCreate, _, _ := round11NewHandler(t, cfg, createState, &RegistryStub{})
		ctx2, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		require.False(t, hCreate.undoTraditionalBoost(ctx2, actor, objectID))
	})

	t.Run("undoQuoteBoost + processQuoteBoostUndo error branches", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.BaseURL() + "/users/alice",
				Type: "Person",
			},
			PreferredUsername: "alice",
		}
		objectID := cfg.BaseURL() + "/objects/status-1"

		allErrState := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]models.QuoteRelationship": errors.New("boom"),
			},
		}
		hAllErr, _, _ := round11NewHandler(t, cfg, allErrState, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		require.False(t, hAllErr.undoQuoteBoost(ctx, actor, objectID))

		noMatchState := &round10QueryState{
			quoteRelationships: []storagemodels.QuoteRelationship{
				{QuoterNoteID: "note-1", TargetNoteID: objectID, QuoterID: "someone-else"},
			},
		}
		hNoMatch, _, _ := round11NewHandler(t, cfg, noMatchState, &RegistryStub{})
		ctx2, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		require.False(t, hNoMatch.undoQuoteBoost(ctx2, actor, objectID))

		undoErrState := &round10QueryState{
			firstErrorOnce:  errors.New("withdraw lookup failed"),
			createErrorOnce: errors.New("create activity failed"),
			deleteErrorOnce: errors.New("delete object failed"),
		}
		hUndoErr, _, _ := round11NewHandler(t, cfg, undoErrState, &RegistryStub{})
		ctx3, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		hUndoErr.processQuoteBoostUndo(ctx3, actor, &storage.QuoteRelationship{
			ID:           "q1",
			QuoterNoteID: "note-1",
			TargetNoteID: objectID,
			QuoterID:     actor.ID,
			Timestamp:    time.Now().Add(-1 * time.Minute),
		})
	})
}
