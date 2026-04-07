package routing

import (
	"context"
	stdliberrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_Round11_ProcessAcceptActivity_EmbeddedFollowAcceptsCanonicalRelationship(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.RelationshipRecord)
			record.PK = "FOLLOW#alice"
			record.SK = "FOLLOWING#bob@remote.example"
			record.GSI1SK = "FOLLOWER#alice"
			record.State = models.RelationshipPending
		}).
		Return(nil).
		Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	update := env.mockQuery.On("Update", mock.Anything).Return(nil).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{update}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-embedded-follow",
		},
		Actor: env.remoteActorID,
		Object: map[string]any{
			"type":   activitypub.FollowType,
			"id":     env.cfg.BaseURL() + "/activities/follow-embedded",
			"actor":  env.cfg.ActorURL("alice"),
			"object": env.remoteActorID,
		},
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
}

func TestInboxHandler_Round11_ProcessAcceptActivity_FallsBackToPendingRelationshipWhenOriginalMissing(t *testing.T) {
	env := newInboxTestEnv(t)

	lookup := env.mockQuery.On("All", mock.AnythingOfType("*[]*models.Activity")).
		Return(stdliberrors.New("boom")).
		Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{lookup}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-follow-string-fallback",
		},
		Actor:  env.remoteActorID,
		Object: env.cfg.BaseURL() + "/activities/follow-missing",
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
	env.mockQuery.AssertNotCalled(t, "Update", mock.Anything)
}

func TestInboxHandler_Round11_ProcessAcceptActivity_EmbeddedFollowRequiresMatchingTarget(t *testing.T) {
	env := newInboxTestEnv(t)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-embedded-follow-mismatch",
		},
		Actor: env.remoteActorID,
		Object: map[string]any{
			"type":   activitypub.FollowType,
			"id":     env.cfg.BaseURL() + "/activities/follow-embedded-mismatch",
			"actor":  env.cfg.ActorURL("alice"),
			"object": "https://remote.example/users/carol",
		},
	}

	require.NoError(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
	env.mockQuery.AssertNotCalled(t, "Update", mock.Anything)
}

func TestInboxHandler_Round11_ProcessAcceptActivity_EmbeddedFollowUpdateErrorDoesNotFallback(t *testing.T) {
	env := newInboxTestEnv(t)

	call := env.mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.RelationshipRecord)
			record.PK = "FOLLOW#alice"
			record.SK = "FOLLOWING#bob@remote.example"
			record.GSI1SK = "FOLLOWER#alice"
			record.State = models.RelationshipPending
		}).
		Return(nil).
		Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	update := env.mockQuery.On("Update", mock.Anything).Return(stdliberrors.New("boom")).Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{update}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      env.cfg.BaseURL() + "/activities/accept-embedded-follow-error",
		},
		Actor: env.remoteActorID,
		Object: map[string]any{
			"type":   activitypub.FollowType,
			"id":     env.cfg.BaseURL() + "/activities/follow-embedded-error",
			"actor":  env.cfg.ActorURL("alice"),
			"object": env.remoteActorID,
		},
	}

	require.Error(t, env.handler.processAcceptActivity(context.Background(), accept, env.local))
}
