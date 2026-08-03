package routing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	costpkg "github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func setRunAsyncSynchronous(t *testing.T) {
	t.Helper()

	previous := runAsync
	runAsync = func(fn func()) { fn() }
	t.Cleanup(func() { runAsync = previous })
}

func signAppTheoryRequest(t *testing.T, env *inboxTestEnv, ctx *apptheory.Context, body []byte) {
	t.Helper()

	httpReq, err := env.handler.convertRequest(ctx, body)
	require.NoError(t, err)

	require.NoError(t, federation.SignHTTPRequest(httpReq, env.remotePrivateKey, env.remoteKeyID))

	if date := httpReq.Header.Get("Date"); date != "" {
		ctx.Request.Headers["date"] = []string{date}
	}
	if digest := httpReq.Header.Get("Digest"); digest != "" {
		ctx.Request.Headers["digest"] = []string{digest}
	}
	if signature := httpReq.Header.Get("Signature"); signature != "" {
		ctx.Request.Headers["signature"] = []string{signature}
	}
}

func TestInboxHandler_Round10_PostPipeline_CreateActivity(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	targetActor, err := env.handler.actorRepository.GetActorByUsername(context.Background(), "alice")
	require.NoError(t, err)

	centralized := costpkg.NewTrackingService(nil, env.logger, costpkg.DefaultTrackingServiceConfig())
	t.Cleanup(func() { _ = centralized.Close(context.Background()) })
	env.handler.centralizedCostService = centralized

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.cfg.BaseURL() + "/activities/create-1",
		"actor":    env.remoteActorID,
		"to":       []string{targetActor.ID},
		"object": map[string]any{
			"@context":     "https://www.w3.org/ns/activitystreams",
			"id":           env.cfg.BaseURL() + "/objects/note-1",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "hello world",
			"to":           []string{targetActor.ID},
			"cc":           []string{},
			"attachment": []any{
				map[string]any{
					"type": activitypub.DocumentType,
					"url":  "https://remote.example/media/1.png",
				},
			},
			"tag": []any{
				map[string]any{
					"type": "Hashtag",
					"href": "https://remote.example/tags/test",
					"name": "#test",
				},
			},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	headers := map[string]string{
		"Host":            "localhost",
		"Content-Type":    "application/activity+json",
		"User-Agent":      "Mastodon/4.0.0",
		"X-Forwarded-For": "203.0.113.10",
	}
	ctx := newAppTheoryContext("POST", "/users/alice/inbox", headers, nil, body)
	ctx.Params["username"] = "alice"

	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostInbox(ctx)
	require.NoError(t, err)
	require.Equal(t, 202, resp.Status)
}

func TestInboxHandler_Round10_ProcessorSweep(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	ctx := context.Background()

	t.Run("follow manual approval and auto accept", func(t *testing.T) {
		manual := *env.local
		manual.ManuallyApprovesFollowers = true

		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.FollowType,
				ID:      env.cfg.BaseURL() + "/activities/follow-1",
				To:      []string{manual.ID},
			},
			Actor:  env.remoteActorID,
			Object: manual.ID,
		}
		require.NoError(t, env.handler.processFollowActivity(ctx, follow, &manual))

		auto := *env.local
		auto.ManuallyApprovesFollowers = false
		follow.ID = env.cfg.BaseURL() + "/activities/follow-2"
		require.NoError(t, env.handler.processFollowActivity(ctx, follow, &auto))
	})

	t.Run("accept follow", func(t *testing.T) {
		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.AcceptType,
				ID:      env.cfg.BaseURL() + "/activities/accept-1",
				To:      []string{env.local.ID},
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/follow-lookup",
		}
		require.NoError(t, env.handler.processAcceptActivity(ctx, accept, env.local))
	})

	t.Run("reject paths", func(t *testing.T) {
		rejectByID := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-1",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/follow-lookup",
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, rejectByID, env.local))

		rejectEmbedded := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-2",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.FollowType,
				"id":     env.cfg.BaseURL() + "/activities/follow-embedded",
				"actor":  env.remoteActorID,
				"object": env.local.ID,
			},
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, rejectEmbedded, env.local))

		targetObjectID := env.cfg.BaseURL() + "/objects/1"
		likeActivity := &activitypub.Activity{Actor: env.remoteActorID, Object: targetObjectID}
		rejectActivity := &activitypub.Activity{Actor: env.remoteActorID}
		require.NoError(t, env.handler.processRejectLike(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectAnnounce(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectCreate(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectUpdate(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectDelete(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectAccept(ctx, rejectActivity, env.local, likeActivity))
		likeActivity.Target = env.local.ID + "/featured"
		require.NoError(t, env.handler.processRejectAdd(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectRemove(ctx, rejectActivity, env.local, likeActivity))
		require.NoError(t, env.handler.processRejectFlag(ctx, rejectActivity, env.local, likeActivity))
		likeActivity.Target = env.local.ID
		likeActivity.Object = map[string]any{"id": targetObjectID}
		require.NoError(t, env.handler.processRejectMove(ctx, rejectActivity, env.local, likeActivity))
	})

	t.Run("remote update and delete", func(t *testing.T) {
		objectID := env.cfg.BaseURL() + "/objects/1"

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UpdateType,
				ID:      env.cfg.BaseURL() + "/activities/update-1",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"id":           objectID,
				"type":         activitypub.NoteType,
				"attributedTo": env.remoteActorID,
				"content":      "updated content",
			},
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.DeleteType,
				ID:      env.cfg.BaseURL() + "/activities/delete-1",
			},
			Actor:  env.remoteActorID,
			Object: objectID,
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("like and announce", func(t *testing.T) {
		objectID := env.cfg.BaseURL() + "/objects/1"
		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.LikeType,
				ID:      env.cfg.BaseURL() + "/activities/like-1",
			},
			Actor:  env.remoteActorID,
			Object: objectID,
		}
		require.NoError(t, env.handler.processLikeActivity(ctx, like, env.local))

		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.AnnounceType,
				ID:      env.cfg.BaseURL() + "/activities/announce-1",
			},
			Actor:  env.remoteActorID,
			Object: objectID,
		}
		require.NoError(t, env.handler.processAnnounceActivity(ctx, announce, env.local))
	})

	t.Run("undo and block", func(t *testing.T) {
		objectID := env.cfg.BaseURL() + "/objects/1"

		undoFollow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-follow-1",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.FollowType,
				"id":     env.cfg.BaseURL() + "/activities/follow-undo-target",
				"actor":  env.remoteActorID,
				"object": env.local.ID,
			},
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undoFollow, env.local))

		undoLike := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-like-1",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.LikeType,
				"id":     env.cfg.BaseURL() + "/activities/like-undo-target",
				"actor":  env.remoteActorID,
				"object": objectID,
			},
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undoLike, env.local))

		undoAnnounce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-announce-1",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.AnnounceType,
				"id":     env.cfg.BaseURL() + "/activities/announce-undo-target",
				"actor":  env.remoteActorID,
				"object": objectID,
			},
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undoAnnounce, env.local))

		blockedActorID := "https://remote.example/users/spammer"
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.BlockType,
				ID:      env.cfg.BaseURL() + "/activities/block-1",
			},
			Actor:  env.remoteActorID,
			Object: blockedActorID,
		}
		require.NoError(t, env.handler.processBlockActivity(ctx, block, env.local))

		unauthorizedUndo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-block-unauth",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.BlockType,
				"id":     env.cfg.BaseURL() + "/activities/block-target",
				"actor":  "https://remote.example/users/other",
				"object": blockedActorID,
			},
		}
		require.Error(t, env.handler.processUndoActivity(ctx, unauthorizedUndo, env.local))

		authorizedUndo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-block-ok",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":   activitypub.BlockType,
				"id":     env.cfg.BaseURL() + "/activities/block-target",
				"actor":  env.remoteActorID,
				"object": blockedActorID,
			},
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, authorizedUndo, env.local))
	})

	t.Run("add and remove collection items", func(t *testing.T) {
		target := env.local.ID + "/featured"
		objectID := env.cfg.BaseURL() + "/objects/1"

		add := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.AddType,
				ID:      env.cfg.BaseURL() + "/activities/add-1",
			},
			Actor:  env.local.ID,
			Object: objectID,
			Target: target,
		}
		require.NoError(t, env.handler.processAddActivity(ctx, add, env.local))

		remove := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RemoveType,
				ID:      env.cfg.BaseURL() + "/activities/remove-1",
			},
			Actor:  env.local.ID,
			Object: objectID,
			Target: target,
		}
		require.NoError(t, env.handler.processRemoveActivity(ctx, remove, env.local))
	})

	t.Run("cost tracking happy path", func(t *testing.T) {
		centralized := costpkg.NewTrackingService(nil, env.logger, costpkg.DefaultTrackingServiceConfig())
		t.Cleanup(func() { _ = centralized.Close(context.Background()) })

		env.handler.centralizedCostService = centralized
		now := time.Now()
		req := &InboxRequest{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   env.cfg.BaseURL() + "/activities/x",
					Type: activitypub.CreateType,
				},
				Actor: env.remoteActorID,
			},
			ActorDomain: "remote.example",
			StartTime:   now,
			CostParams:  &federation.CostCalculationParams{DynamoDBReadCount: 1, DynamoDBWriteCount: 1},
		}
		env.handler.trackCentralizedCost(req, "Federation")
	})
}
