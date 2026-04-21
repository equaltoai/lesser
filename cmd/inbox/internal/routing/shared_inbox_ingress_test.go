package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_SharedInboxGetReturnsMethodNotAllowed(t *testing.T) {
	env := newInboxTestEnv(t)

	resp, err := env.handler.handleGetSharedInbox(newAppTheoryContext(http.MethodGet, "/inbox", map[string]string{"Host": "localhost"}, nil, nil))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusMethodNotAllowed, resp.Status)
}

func TestInboxHandler_SharedInboxPublicCreateResolvesFollowerTargets(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	require.NoError(t, relationshipRepo.CreateRelationship(context.Background(), "alice", "bob@remote.example", env.cfg.BaseURL()+"/activities/follow-remote"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(context.Background(), "alice", "bob@remote.example"))
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.cfg.BaseURL() + "/activities/shared-create-public",
		"actor":    env.remoteActorID,
		"to":       []string{activitypub.PublicAddress},
		"cc":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.cfg.BaseURL() + "/objects/shared-note-public",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "hello shared inbox",
			"to":           []string{activitypub.PublicAddress},
			"cc":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)

	stored, err := env.handler.activityRepository.GetActivity(context.Background(), env.cfg.BaseURL()+"/activities/shared-create-public")
	require.NoError(t, err)
	require.Equal(t, env.remoteActorID, stored.Actor)
}

func TestInboxHandler_SharedInboxFollowersOnlyCreateResolvesFollowerTargets(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	require.NoError(t, relationshipRepo.CreateRelationship(context.Background(), "alice", "bob@remote.example", env.cfg.BaseURL()+"/activities/follow-private"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(context.Background(), "alice", "bob@remote.example"))
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.cfg.BaseURL() + "/activities/shared-create-private",
		"actor":    env.remoteActorID,
		"to":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.cfg.BaseURL() + "/objects/shared-note-private",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "hello followers only",
			"to":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)
}

func TestInboxHandler_SharedInboxFollowResolvesTargetActorAndReusesProcessing(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.FollowType,
		"id":       env.cfg.BaseURL() + "/activities/shared-follow",
		"actor":    env.remoteActorID,
		"object":   env.local.ID,
		"to":       []string{env.local.ID},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)

	relationship, err := relationshipRepo.GetRelationship(context.Background(), "bob@remote.example", "alice")
	require.NoError(t, err)
	require.Equal(t, "accepted", relationship.State)
}
