package routing

import (
	"context"
	stdliberrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_Round10_ProcessMoveActivity_AuthorizationFailure(t *testing.T) {
	env := newInboxTestEnv(t)

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.MoveType,
			ID:      env.cfg.BaseURL() + "/activities/move-unauthorized",
		},
		Actor:  env.cfg.ActorURL("old"),
		Target: env.cfg.ActorURL("alice"),
	}

	require.Error(t, env.handler.processMoveActivity(context.Background(), activity, env.local))
}

func TestInboxHandler_Round10_ProcessFollowActivity_Blocked_ReturnsNil(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("First", mock.AnythingOfType("*models.Block")).Return(nil).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      env.cfg.BaseURL() + "/activities/follow-blocked",
			To:      []string{env.local.ID},
		},
		Actor:  env.remoteActorID,
		Object: env.local.ID,
	}

	require.NoError(t, env.handler.processFollowActivity(context.Background(), follow, env.local))
}

func TestInboxHandler_Round10_ProcessFollowActivity_AcceptFollowRequest_Error(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("Update", mock.Anything).Return(stdliberrors.New("boom")).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      env.cfg.BaseURL() + "/activities/follow-accept-error",
			To:      []string{env.local.ID},
		},
		Actor:  env.remoteActorID,
		Object: env.local.ID,
	}

	require.Error(t, env.handler.processFollowActivity(context.Background(), follow, env.local))
}

func TestInboxHandler_Round10_ProcessAcceptActivity_OriginalLookupError_Ignored(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("All", mock.AnythingOfType("*[]*models.Activity")).Return(stdliberrors.New("boom")).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-missing-original",
			To:      []string{env.local.ID},
		},
		Actor:  env.remoteActorID,
		Object: env.cfg.BaseURL() + "/activities/follow-lookup",
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
}
