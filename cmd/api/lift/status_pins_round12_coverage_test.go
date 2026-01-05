package lift

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestStatusPinsRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("pin/unpin fallback id extraction + map object ownership", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/s1"
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {
					Username: "alice",
					Actor:    &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"}},
				},
			},
			objectsByID: map[string]storagemodels.Object{
				// Non-note type forces map[string]any path for attributedTo extraction.
				objectID: {ID: objectID, Type: "Article", Content: "hello", AttributedTo: cfg.ActorURL("alice")},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxPin, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
		require.NoError(t, err)
		// Do not set ctx.Param("id") to exercise test-mode path extraction.
		handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxPin)
		require.Equal(t, http.StatusOK, ctxPin.Response.StatusCode)

		ctxUnpin, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
		require.NoError(t, err)
		handleWithAPIMiddleware(t, handler.HandleUnpinStatusLift, ctxUnpin)
		require.Equal(t, http.StatusOK, ctxUnpin.Response.StatusCode)
	})

	t.Run("pin: actor lookup error", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/s1"
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"ACTOR#alice#PROFILE": true,
			},
			objectsByID: map[string]storagemodels.Object{
				objectID: {ID: objectID, Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("alice"), Content: "hi"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctx)
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("unpin: delete error + object not found", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/s1"

		delErrState := &round10QueryState{
			deleteErrorOnce: errors.New("delete failed"),
			objectsByID: map[string]storagemodels.Object{
				objectID: {ID: objectID, Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("alice"), Content: "hi"},
			},
		}
		handlerDel, _, _ := round11NewHandler(t, cfg, delErrState)
		ctxDel, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
		require.NoError(t, err)
		ctxDel.SetParam("id", "s1")
		handleWithAPIMiddleware(t, handlerDel.HandleUnpinStatusLift, ctxDel)
		require.Equal(t, http.StatusInternalServerError, ctxDel.Response.StatusCode)

		notFoundState := &round10QueryState{
			notFoundPKs: map[string]bool{
				"object#" + objectID: true,
			},
		}
		handlerNF, _, _ := round11NewHandler(t, cfg, notFoundState)
		ctxNF, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
		require.NoError(t, err)
		ctxNF.SetParam("id", "s1")
		handleWithAPIMiddleware(t, handlerNF.HandleUnpinStatusLift, ctxNF)
		require.Equal(t, http.StatusNotFound, ctxNF.Response.StatusCode)
	})

	t.Run("mute helpers + retry branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		// extractStatusIDFromPath edge cases
		ctxNilReq, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		ctxNilReq.Request = nil
		require.Equal(t, "", handler.extractStatusIDFromPath(ctxNilReq, "mute"))

		ctxBadPath, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		ctxBadPath.Request.Path = ""
		require.Equal(t, "", handler.extractStatusIDFromPath(ctxBadPath, "mute"))

		ctxWrongAction, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/mute", nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "", handler.extractStatusIDFromPath(ctxWrongAction, "unmute"))

		// normalizeMuteObjectID already-scheme branch
		require.Equal(t, "https://example.com/objects/abc", handler.normalizeMuteObjectID("https://example.com/objects/abc"))

		// parseMuteDuration parse failure branches (both ParseRequest and json.Unmarshal)
		ctxBadJSON := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/mute", nil, nil, []byte(`{invalid}`))
		require.Zero(t, handler.parseMuteDuration(ctxBadJSON))

		// storeMuteWithRetry non-already-exists error branch
		errState := &round10QueryState{
			createErrorOnce: apperrors.Internal("boom"),
		}
		errHandler, _, _ := round11NewHandler(t, cfg, errState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)
		require.Error(t, errHandler.storeMuteWithRetry(ctx, "alice", "conv", &storage.ConversationMute{Username: "alice", ConversationID: "conv", CreatedAt: time.Now()}))

		// replaceMute delete warning branch (already-exists -> replace, delete fails, create succeeds)
		retryState := &round10QueryState{
			createErrorOnce: apperrors.AlreadyExists("conversation mute"),
			deleteErrorOnce: errors.New("delete failed"),
		}
		retryHandler, _, _ := round11NewHandler(t, cfg, retryState)
		require.NoError(t, retryHandler.storeMuteWithRetry(ctx, "alice", "conv", &storage.ConversationMute{Username: "alice", ConversationID: "conv", CreatedAt: time.Now()}))

		// replaceMute create error branch (direct)
		createFailState := &round10QueryState{
			createErrorOnce: errors.New("create failed"),
		}
		createFailHandler, _, _ := round11NewHandler(t, cfg, createFailState)
		require.Error(t, createFailHandler.replaceMute(ctx, "alice", "conv", &storage.ConversationMute{Username: "alice", ConversationID: "conv", CreatedAt: time.Now()}))
	})

	t.Run("buildMutedStatusResponse not found", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/missing"
		state := &round10QueryState{
			notFoundPKs: map[string]bool{
				"object#" + objectID: true,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/x", nil, nil, nil)
		require.NoError(t, err)

		handleWithAPIMiddleware(t, func(c *lift.Context) error {
			return handler.buildMutedStatusResponse(c, objectID, "alice")
		}, ctx)
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("unmute conversation delete error + fallback path extraction", func(t *testing.T) {
		objectID := cfg.BaseURL() + "/objects/s1"
		state := &round10QueryState{
			deleteErrorOnce: errors.New("delete failed"),
			objectsByID: map[string]storagemodels.Object{
				objectID: {ID: objectID, Type: activitypub.NoteType, Content: "hello", AttributedTo: cfg.ActorURL("alice")},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unmute", headers, nil, nil)
		require.NoError(t, err)
		// No ctx.Param("id") to force fallback extraction.
		ctx.Request.Path = strings.ReplaceAll(ctx.Request.Path, "//", "/")
		handleWithAPIMiddleware(t, handler.HandleUnmuteConversationLift, ctx)
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("extractMuteStatusID error path", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses//mute", nil, nil, nil)
		require.NoError(t, err)
		_, err = handler.extractMuteStatusID(ctx)
		require.Error(t, err)
	})
}
