package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func handleWithAPIMiddleware(t *testing.T, handlerFunc apptheory.Handler, ctx *apptheory.Context) *apptheory.Response {
	t.Helper()
	mw := common.CreateAPIErrorMiddleware(zap.NewNop())
	resp, err := mw(handlerFunc)(ctx)
	require.NoError(t, err)
	return resp
}

func decodeStandardErrorResponse(t *testing.T, resp *apptheory.Response) common.StandardErrorResponse {
	t.Helper()

	require.NotNil(t, resp)

	var decoded common.StandardErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &decoded))
	return decoded
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
	respMissing := handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxMissing)
	require.Equal(t, http.StatusUnauthorized, respMissing.Status)
	var missingBody common.BearerAuthErrorResponse
	require.NoError(t, json.Unmarshal(respMissing.Body, &missingBody))
	require.Equal(t, common.BearerErrorInvalidToken, missingBody.Error)

	ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/bad id/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxInvalid.Params["id"] = "bad id"
	respInvalid := handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxInvalid)
	require.Equal(t, http.StatusBadRequest, respInvalid.Status)
	require.Equal(t, "VALIDATION_FAILED", decodeStandardErrorResponse(t, respInvalid).Code)

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
	ctxNotFound.Params["id"] = "missing"
	respNotFound := handleWithAPIMiddleware(t, notFoundHandler.HandlePinStatusLift, ctxNotFound)
	require.Equal(t, http.StatusNotFound, respNotFound.Status)
	require.Equal(t, "NOT_FOUND", decodeStandardErrorResponse(t, respNotFound).Code)

	state.objectsByID[objectID] = storagemodels.Object{ID: objectID, Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("bob")}
	ctxForbidden, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxForbidden.Params["id"] = "s1"
	respForbidden := handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxForbidden)
	require.Equal(t, http.StatusForbidden, respForbidden.Status)
	require.Equal(t, "FORBIDDEN", decodeStandardErrorResponse(t, respForbidden).Code)

	state.objectsByID[objectID] = ownedObject
	errorState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: apperrors.AlreadyExists("status pin"),
	}
	errorHandler, _, _ := round11NewHandler(t, cfg, errorState)
	ctxPinned, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxPinned.Params["id"] = "s1"
	respPinned := handleWithAPIMiddleware(t, errorHandler.HandlePinStatusLift, ctxPinned)
	require.Equal(t, http.StatusConflict, respPinned.Status)
	require.Equal(t, "ALREADY_EXISTS", decodeStandardErrorResponse(t, respPinned).Code)

	ctxOK, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxOK.Params["id"] = "s1"
	respOK := handleWithAPIMiddleware(t, handler.HandlePinStatusLift, ctxOK)
	require.Equal(t, http.StatusOK, respOK.Status)

	ctxUnpin, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
	require.NoError(t, err)
	ctxUnpin.Params["id"] = "s1"
	respUnpin := handleWithAPIMiddleware(t, handler.HandleUnpinStatusLift, ctxUnpin)
	require.Equal(t, http.StatusOK, respUnpin.Status)
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
	ctxMute.Params["id"] = "s1"
	respMute := handleWithAPIMiddleware(t, handler.HandleMuteConversationLift, ctxMute)
	require.Equal(t, http.StatusOK, respMute.Status)

	retryState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: apperrors.AlreadyExists("conversation mute"),
	}
	retryHandler, _, _ := round11NewHandler(t, cfg, retryState)
	ctxRetry := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/mute", headers, nil, []byte(`{"duration": 0}`))
	ctxRetry.Params["id"] = "s1"
	respRetry := handleWithAPIMiddleware(t, retryHandler.HandleMuteConversationLift, ctxRetry)
	require.Equal(t, http.StatusOK, respRetry.Status)

	ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctxUnmute.Params["id"] = "s1"
	respUnmute := handleWithAPIMiddleware(t, handler.HandleUnmuteConversationLift, ctxUnmute)
	require.Equal(t, http.StatusOK, respUnmute.Status)

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
