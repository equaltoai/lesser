package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleReblogLift_UsesUnifiedBoostParserOnLiveRoute(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("rejects invalid request body before pure boost", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ReblogNoteFunc: func(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
					t.Fatal("reblog service should not be called for invalid JSON")
					return nil, nil
				},
			},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, []byte(`{invalid}`))
		ctx.Params["id"] = "status-1"

		requireStatus(t, http.StatusBadRequest)(h.HandleReblogLift(ctx))
	})

	t.Run("preserves pure boost service path for empty bodies", func(t *testing.T) {
		called := false
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ReblogNoteFunc: func(_ context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
					called = true
					require.Equal(t, "status-1", cmd.StatusID)
					require.Equal(t, "alice", cmd.RebloggerID)
					return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID, Content: "original"}}, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"

		resp := requireStatus(t, http.StatusOK)(h.HandleReblogLift(ctx))
		var body models.Status
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.True(t, body.Reblogged)
		require.True(t, called)
	})

	t.Run("routes quote comments through quote boost path", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/status-1"
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
					PreferredUsername: "alice",
					Followers:         cfg.ActorURL("alice") + "/followers",
				}},
			},
			objectsByID: map[string]storagemodels.Object{
				objectID: {ID: objectID, Type: activitypub.NoteType, Content: "original", AttributedTo: "bob", Published: time.Now().Add(-time.Hour)},
			},
			notFoundPKSK: map[string]bool{
				"ACTOR#bob#PROFILE": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
					require.Equal(t, "alice", viewerID)
					require.Equal(t, "status-1", rawQuoteTarget)
					return quoteBoostTarget("status-1", objectID, storagemodels.VisibilityPublic), nil
				},
				ReblogNoteFunc: func(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
					t.Fatal("pure reblog service should not be called for quote comments")
					return nil, nil
				},
			},
		})

		comment := "quoted safely"
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &comment})
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"

		resp := requireStatus(t, http.StatusOK)(h.HandleReblogLift(ctx))
		var body models.Status
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.True(t, body.IsQuoteBoost)
		require.Equal(t, comment, body.Content)
		require.NotNil(t, body.QuotedStatusID)
		require.Equal(t, objectID, *body.QuotedStatusID)
	})
}

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
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: &NotesServiceStub{
			ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
				require.Equal(t, "alice", viewerID)
				require.Equal(t, "status-1", rawQuoteTarget)
				return quoteBoostTarget("status-1", objectID, storagemodels.VisibilityPublic), nil
			},
			ReblogNoteFunc: func(_ context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{Status: &storagemodels.Status{StatusID: cmd.StatusID}}, nil
			},
		}})

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

func quoteBoostTarget(statusID, objectID, visibility string) *storagemodels.Status {
	now := time.Now().UTC()
	return &storagemodels.Status{
		StatusID:       statusID,
		AuthorID:       "https://remote.example/users/bob",
		AuthorUsername: "bob@remote.example",
		Visibility:     visibility,
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: objectID, Type: activitypub.NoteType},
			AttributedTo: "https://remote.example/users/bob",
			Content:      "original",
			Visibility:   visibility,
		},
	}
}

func TestHandleReblogLift_EnforcesQuoteReachAndViewerAccess(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	headers := map[string]string{"Authorization": "Bearer " + token}
	comment := "bounded quote"

	newQuoteHandler := func(t *testing.T, target *storagemodels.Status, resolveErr error) *Handler {
		t.Helper()
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
					PreferredUsername: "alice",
					Followers:         cfg.ActorURL("alice") + "/followers",
				}},
			},
		}
		if target != nil && target.Note != nil {
			state.objectsByID = map[string]storagemodels.Object{
				target.Note.ID: {
					ID:           target.Note.ID,
					Type:         activitypub.NoteType,
					Content:      target.Content,
					AttributedTo: target.AuthorID,
					Published:    target.PublishedAt,
				},
			}
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: &NotesServiceStub{
			ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
				require.Equal(t, "alice", viewerID)
				require.Equal(t, "status-1", rawQuoteTarget)
				return target, resolveErr
			},
		}})
		return h
	}

	for _, targetVisibility := range []string{storagemodels.VisibilityPrivate, storagemodels.VisibilityDirect} {
		t.Run(targetVisibility+" target cannot be quoted at equal reach", func(t *testing.T) {
			target := quoteBoostTarget("status-1", cfg.BaseURL()+"/objects/status-1", targetVisibility)
			h := newQuoteHandler(t, target, nil)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{
				Comment:    &comment,
				Visibility: targetVisibility,
			})
			require.NoError(t, err)
			ctx.Params["id"] = "status-1"

			resp := requireStatus(t, http.StatusUnprocessableEntity)(h.HandleReblogLift(ctx))
			var body common.StandardErrorResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, string(commonerrors.CodeUnprocessableEntity), body.Code)
			require.Equal(t, restTargetNotQuotable, body.Error)
		})
	}

	t.Run("unviewable target remains indistinguishable from not found", func(t *testing.T) {
		h := newQuoteHandler(t, nil, notes.ErrStatusNotFound)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{Comment: &comment})
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"
		requireStatus(t, http.StatusNotFound)(h.HandleReblogLift(ctx))
	})

	tests := []struct {
		name             string
		targetVisibility string
		quoteVisibility  string
	}{
		{name: "equal public", targetVisibility: storagemodels.VisibilityPublic, quoteVisibility: storagemodels.VisibilityPublic},
		{name: "narrower private", targetVisibility: storagemodels.VisibilityPublic, quoteVisibility: storagemodels.VisibilityPrivate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := quoteBoostTarget("status-1", cfg.BaseURL()+"/objects/status-1", tt.targetVisibility)
			h := newQuoteHandler(t, target, nil)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog", headers, nil, models.ReblogRequest{
				Comment:    &comment,
				Visibility: tt.quoteVisibility,
			})
			require.NoError(t, err)
			ctx.Params["id"] = "status-1"
			requireStatus(t, http.StatusOK)(h.HandleReblogLift(ctx))
		})
	}
}
