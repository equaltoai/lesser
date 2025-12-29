package lift

import (
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, handler.HandlePinStatusLift(ctxMissing))
	require.Equal(t, http.StatusUnauthorized, ctxMissing.Response.StatusCode)

	ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/bad id/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxInvalid.SetParam("id", "bad id")
	require.NoError(t, handler.HandlePinStatusLift(ctxInvalid))
	require.Equal(t, http.StatusBadRequest, ctxInvalid.Response.StatusCode)

	state.objectsByID[objectID] = storagemodels.Object{ID: objectID, Type: activitypub.NoteType, AttributedTo: cfg.ActorURL("bob")}
	ctxForbidden, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxForbidden.SetParam("id", "s1")
	require.NoError(t, handler.HandlePinStatusLift(ctxForbidden))
	require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)

	state.objectsByID[objectID] = ownedObject
	errorState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: errors.New("already pinned"),
	}
	errorHandler, _, _ := round11NewHandler(t, cfg, errorState)
	ctxPinned, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxPinned.SetParam("id", "s1")
	require.NoError(t, errorHandler.HandlePinStatusLift(ctxPinned))
	require.Equal(t, http.StatusInternalServerError, ctxPinned.Response.StatusCode)

	ctxOK, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/pin", headers, nil, nil)
	require.NoError(t, err)
	ctxOK.SetParam("id", "s1")
	require.NoError(t, handler.HandlePinStatusLift(ctxOK))
	require.Equal(t, http.StatusOK, ctxOK.Response.StatusCode)

	ctxUnpin, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unpin", headers, nil, nil)
	require.NoError(t, err)
	ctxUnpin.SetParam("id", "s1")
	require.NoError(t, handler.HandleUnpinStatusLift(ctxUnpin))
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
	require.NoError(t, handler.HandleMuteConversationLift(ctxMute))
	require.Equal(t, http.StatusOK, ctxMute.Response.StatusCode)

	retryState := &round10QueryState{
		actorsByUser:    state.actorsByUser,
		objectsByID:     state.objectsByID,
		createErrorOnce: errors.New("already muted"),
	}
	retryHandler, _, _ := round11NewHandler(t, cfg, retryState)
	ctxRetry := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/mute", headers, nil, []byte(`{"duration": 0}`))
	ctxRetry.SetParam("id", "s1")
	_ = retryHandler.HandleMuteConversationLift(ctxRetry)
	require.NotZero(t, ctxRetry.Response.StatusCode)

	ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctxUnmute.SetParam("id", "s1")
	require.NoError(t, handler.HandleUnmuteConversationLift(ctxUnmute))
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
	require.NoError(t, handler.HandleMuteConversationLift(ctxAuth))
}
