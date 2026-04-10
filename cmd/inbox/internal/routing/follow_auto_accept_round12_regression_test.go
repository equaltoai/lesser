package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

type stubInboxRemoteActorResolver struct {
	resolution *federation.ExactActorResolution
	err        error
	inputs     []string
}

func (s *stubInboxRemoteActorResolver) ResolveDeliverableActor(_ context.Context, input, _ string) (*federation.ExactActorResolution, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return nil, s.err
	}
	return s.resolution, nil
}

type recordingInboxDeliverer struct {
	calls       int
	targetInbox string
	activity    *activitypub.Activity
	signer      *activitypub.Actor
}

func (d *recordingInboxDeliverer) DeliverActivity(_ context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	d.calls++
	d.targetInbox = targetInbox
	d.activity = activity
	d.signer = signingActor
	return nil
}

func TestInboxHandler_ProcessFollowActivity_AutoAcceptResolvesRemoteActorOnCacheMiss(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.relationshipRepository = inmemory.NewRelationshipRepository()

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   env.remoteActorID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             env.remoteActorID + "/inbox",
		Outbox:            env.remoteActorID + "/outbox",
	}
	resolver := &stubInboxRemoteActorResolver{
		resolution: &federation.ExactActorResolution{
			Actor: remoteActor,
			ActorIdentity: federation.ActorIdentity{
				ActorID:  remoteActor.ID,
				Username: "bob",
				Acct:     "bob@remote.example",
				Domain:   "remote.example",
				IsRemote: true,
			},
		},
	}
	deliverer := &recordingInboxDeliverer{}
	env.handler.remoteActorResolver = resolver
	env.handler.activityDeliverer = deliverer

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      env.cfg.BaseURL() + "/activities/follow-auto-accept-cache-miss",
			To:      []string{env.local.ID},
		},
		Actor:  env.remoteActorID,
		Object: env.local.ID,
	}

	require.NoError(t, env.handler.processFollowActivity(context.Background(), follow, env.local))
	require.Equal(t, []string{env.remoteActorID}, resolver.inputs)
	require.Equal(t, 1, deliverer.calls)
	require.Equal(t, remoteActor.Inbox, deliverer.targetInbox)
	require.NotNil(t, deliverer.activity)
	require.Equal(t, activitypub.AcceptType, deliverer.activity.Type)
	require.Equal(t, follow.ID, deliverer.activity.Object)
	require.Equal(t, env.local.ID, deliverer.signer.ID)

	isFollowing, err := env.handler.relationshipRepository.IsFollowing(context.Background(), env.handler.extractHandleFromActorID(env.remoteActorID), env.local.PreferredUsername)
	require.NoError(t, err)
	require.True(t, isFollowing)
}

func TestInboxHandler_ProcessFollowActivity_AutoAcceptPreservesAcceptedStateWhenResolutionFails(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.relationshipRepository = inmemory.NewRelationshipRepository()

	deliverer := &recordingInboxDeliverer{}
	env.handler.remoteActorResolver = &stubInboxRemoteActorResolver{err: errors.New("resolver boom")}
	env.handler.activityDeliverer = deliverer

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      env.cfg.BaseURL() + "/activities/follow-auto-accept-resolution-fail",
			To:      []string{env.local.ID},
		},
		Actor:  env.remoteActorID,
		Object: env.local.ID,
	}

	require.NoError(t, env.handler.processFollowActivity(context.Background(), follow, env.local))
	require.Zero(t, deliverer.calls)

	isFollowing, err := env.handler.relationshipRepository.IsFollowing(context.Background(), env.handler.extractHandleFromActorID(env.remoteActorID), env.local.PreferredUsername)
	require.NoError(t, err)
	require.True(t, isFollowing)
}
