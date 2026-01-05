package lift

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func handleWithAPIMiddleware(t *testing.T, handlerFunc func(*lift.Context) error, ctx *lift.Context) {
	t.Helper()
	mw := common.CreateAPIErrorMiddleware(zap.NewNop())
	require.NoError(t, mw(lift.HandlerFunc(handlerFunc)).Handle(ctx))
}

func decodeStandardErrorResponse(t *testing.T, ctx *lift.Context) common.StandardErrorResponse {
	t.Helper()

	if resp, ok := ctx.Response.Body.(common.StandardErrorResponse); ok {
		return resp
	}

	var raw []byte
	switch v := ctx.Response.Body.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		raw = []byte(fmt.Sprintf("%v", ctx.Response.Body))
	}

	var resp common.StandardErrorResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	return resp
}

func TestStatusPins_PinsAndUnpins(t *testing.T) {
	cfg := round11TestConfig()
	objectID := cfg.BaseURL() + "/objects/s1"

	ownedObject := storagemodels.Object{
		ID:           objectID,
		Type:         activitypub.NoteType,
		Content:      "hello",
		AttributedTo: cfg.ActorURL("alice"),
		To:           []string{activitypub.PublicAddress},
	}
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor:    &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"}},
			},
		},
		objectsByID: map[string]storagemodels.Object{
			objectID: ownedObject,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxMissing, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", nil, nil, nil)
	require.NoError(t, err)
	handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxMissing)
	require.Equal(t, http.StatusUnauthorized, ctxMissing.Response.StatusCode)
	require.Equal(t, "UNAUTHORIZED", decodeStandardErrorResponse(t, ctxMissing).Code)

	ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/bad id/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxInvalid.SetParam("id", "bad id")
	handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxInvalid)
	require.Equal(t, http.StatusBadRequest, ctxInvalid.Response.StatusCode)
	require.Equal(t, "VALIDATION_FAILED", decodeStandardErrorResponse(t, ctxInvalid).Code)

	missingObjectID := cfg.BaseURL() + "/objects/missing"
	notFoundState := &round10QueryState{
		actorsByUser: state.actorsByUser,
		objectsByID:  state.objectsByID,
		notFoundPKs: map[string]bool{
			"object#" + missingObjectID: true,
		},
	}
	notFoundHandler, _, _ := round11NewHandler(t, cfg, notFoundState)

	ctxNotFound, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/missing/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxNotFound.SetParam("id", "missing")
	handleWithAPIMiddleware(t, notFoundHandler.HandlePinStatusLift, ctxNotFound)
	require.Equal(t, http.StatusNotFound, ctxNotFound.Response.StatusCode)
	require.Equal(t, "NOT_FOUND", decodeStandardErrorResponse(t, ctxNotFound).Code)

	state.objectsByID[objectID] = storagemodels.Object{ID: objectID, Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("bob")}
	ctxForbidden, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxForbidden.SetParam("id", "s1")
	handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxForbidden)
	require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)
	require.Equal(t, "FORBIDDEN", decodeStandardErrorResponse(t, ctxForbidden).Code)

	state.objectsByID[objectID] = ownedObject
	errorState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: apperrors.AlreadyExists("status pin"),
	}
	errorHandler, _, _ := round11NewHandler(t, cfg, errorState)
	ctxPinned, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxPinned.SetParam("id", "s1")
	handleWithAPIMiddleware(t, errorHandler.HandlePinStatusLift, ctxPinned)
	require.Equal(t, http.StatusConflict, ctxPinned.Response.StatusCode)
	require.Equal(t, "ALREADY_EXISTS", decodeStandardErrorResponse(t, ctxPinned).Code)

	ctxOK, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxOK.SetParam("id", "s1")
	handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxOK)
	require.Equal(t, http.StatusOK, ctxOK.Response.StatusCode)

	ctxUnpin, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
	require.NoError(t, err)
	ctxUnpin.SetParam("id", "s1")
	handleWithAPIMiddleware(t, handler.HandleUnpinStatusLift, ctxUnpin)
	require.Equal(t, http.StatusOK, ctxUnpin.Response.StatusCode)
}

func TestStatusPins_MuteConversation(t *testing.T) {
	cfg := round11TestConfig()
	objectID := cfg.BaseURL() + "/objects/s1"

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor:    &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"}},
			},
		},
		objectsByID: map[string]storagemodels.Object{
			objectID: {
				ID:           objectID,
				Type:         activitypub.NoteType,
				Content:      "hello",
				AttributedTo: cfg.ActorURL("alice"),
				To:           []string{activitypub.PublicAddress},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxMute := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/mute", headers, nil, []byte(`{"duration": 60}`))
	ctxMute.SetParam("id", "s1")
	handleWithAPIMiddleware(t, handler.HandleMuteConversationLift, ctxMute)
	require.Equal(t, http.StatusOK, ctxMute.Response.StatusCode)

	retryState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: apperrors.AlreadyExists("conversation mute"),
	}
	retryHandler, _, _ := round11NewHandler(t, cfg, retryState)
	ctxRetry := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/mute", headers, nil, []byte(`{"duration": 0}`))
	ctxRetry.SetParam("id", "s1")
	handleWithAPIMiddleware(t, retryHandler.HandleMuteConversationLift, ctxRetry)
	require.Equal(t, http.StatusOK, ctxRetry.Response.StatusCode)

	ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctxUnmute.SetParam("id", "s1")
	handleWithAPIMiddleware(t, handler.HandleUnmuteConversationLift, ctxUnmute)
	require.Equal(t, http.StatusOK, ctxUnmute.Response.StatusCode)

	require.Equal(t, objectID, handler.normalizeMuteObjectID("s1"))
	require.Equal(t, "s1", handler.extractStatusIDFromPath(ctxUnmute, "unmute"))

	ctxEmpty, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/mute", headers, nil, nil)
	require.NoError(t, err)
	require.Zero(t, handler.parseMuteDuration(ctxEmpty))
}

func TestStatusPins_MuteStatusIDFallback(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/test-id/mute", nil, nil, []byte(`{"duration": 5}`))
	ctx.Request.Path = "/api/v1/statuses/test-id/mute"
	statusID := handler.extractStatusIDFromPath(ctx, "mute")
	require.Equal(t, "test-id", statusID)

	state := &round10QueryState{actorsByUser: map[string]storagemodels.Actor{"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice")}}}}, objectsByID: map[string]storagemodels.Object{cfg.BaseURL() + "/objects/test-id": {ID: cfg.BaseURL() + "/objects/test-id", Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("alice")}}}
	handler, _, _ = round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	ctxAuth := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/test-id/mute", map[string]string{"Authorization": "Bearer " + token}, nil, []byte(`{"duration": 1}`))
	ctxAuth.Request.Path = "/api/v1/statuses/test-id/mute"
	handleWithAPIMiddleware(t, handler.HandleMuteConversationLift, ctxAuth)
}
