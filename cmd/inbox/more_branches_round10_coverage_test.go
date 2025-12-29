package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInboxHandler_Round10_RejectByActivityID_SwitchCases(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	cases := []string{
		"follow",
		"like",
		"announce",
		"create",
		"update",
		"delete",
		"accept",
		"add",
		"remove",
		"flag",
		"move",
		"unsupported",
	}

	for _, marker := range cases {
		t.Run(marker, func(t *testing.T) {
			reject := &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					Type:    activitypub.RejectType,
					ID:      env.cfg.BaseURL() + "/activities/reject-" + marker,
				},
				Actor:  env.remoteActorID,
				Object: env.cfg.BaseURL() + "/activities/" + marker + "-target",
			}
			require.NoError(t, env.handler.processRejectActivity(ctx, reject, env.local))
		})
	}

	t.Run("missing object id returns success", func(t *testing.T) {
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-missing",
			},
			Actor:  env.remoteActorID,
			Object: "",
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, reject, env.local))
	})

	t.Run("unsupported reject object type returns success", func(t *testing.T) {
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-unsupported-object",
			},
			Actor:  env.remoteActorID,
			Object: 123,
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, reject, env.local))
	})

	t.Run("embedded object missing type returns success", func(t *testing.T) {
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-embedded-missing-type",
			},
			Actor:  env.remoteActorID,
			Object: map[string]any{"id": env.cfg.BaseURL() + "/activities/follow-embedded"},
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, reject, env.local))
	})

	t.Run("embedded follow marshal failure returns error", func(t *testing.T) {
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-embedded-marshal-fail",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type":  activitypub.FollowType,
				"id":    env.cfg.BaseURL() + "/activities/follow-embedded-bad",
				"actor": make(chan int),
			},
		}
		require.Error(t, env.handler.processRejectActivity(ctx, reject, env.local))
	})

	t.Run("embedded unsupported type returns success", func(t *testing.T) {
		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.RejectType,
				ID:      env.cfg.BaseURL() + "/activities/reject-embedded-unsupported",
			},
			Actor: env.remoteActorID,
			Object: map[string]any{
				"type": activitypub.LikeType,
				"id":   env.cfg.BaseURL() + "/activities/like-embedded",
			},
		}
		require.NoError(t, env.handler.processRejectActivity(ctx, reject, env.local))
	})
}

func TestInboxHandler_Round10_Undo_StringLookup_Branches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	t.Run("follow via string lookup", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-follow-string",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/follow-string-target",
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})

	t.Run("like via string lookup", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-like-string",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/like-string-target",
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})

	t.Run("announce via string lookup", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-announce-string",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/announce-string-target",
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})

	t.Run("block via string lookup", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-block-string",
			},
			Actor:  env.remoteActorID,
			Object: env.cfg.BaseURL() + "/activities/block-string-target",
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})

	t.Run("unauthorized block undo via string lookup", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-block-string-unauth",
			},
			Actor:  "https://remote.example/users/other",
			Object: env.cfg.BaseURL() + "/activities/block-string-target",
		}
		require.Error(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})

	t.Run("invalid object type returns success", func(t *testing.T) {
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      env.cfg.BaseURL() + "/activities/undo-invalid-object",
			},
			Actor:  env.remoteActorID,
			Object: 123,
		}
		require.NoError(t, env.handler.processUndoActivity(ctx, undo, env.local))
	})
}

func TestInboxHandler_Round10_HelperCoverageExpansion(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("storeEditHistory errors", func(t *testing.T) {
		require.Error(t, env.handler.storeEditHistory(context.Background(), "x", func() {}, "bob"))
		require.Error(t, env.handler.storeEditHistory(context.Background(), "x", "not-an-object", "bob"))
	})

	t.Run("verify update/delete authorization", func(t *testing.T) {
		owner := env.remoteActorID
		other := "https://remote.example/users/other"

		updateActivity := &activitypub.Activity{Actor: other}
		require.Error(t, env.handler.verifyUpdateAuthorization(context.Background(), updateActivity, map[string]any{}))

		note := &activitypub.Note{AttributedTo: owner}
		require.Error(t, env.handler.verifyUpdateAuthorization(context.Background(), updateActivity, note))

		updateActivity.Actor = owner
		require.NoError(t, env.handler.verifyUpdateAuthorization(context.Background(), updateActivity, note))

		deleteActivity := &activitypub.Activity{Actor: other}
		require.Error(t, env.handler.verifyDeleteAuthorization(context.Background(), deleteActivity, map[string]any{}))
		require.Error(t, env.handler.verifyDeleteAuthorization(context.Background(), deleteActivity, note))

		deleteActivity.Actor = owner
		require.NoError(t, env.handler.verifyDeleteAuthorization(context.Background(), deleteActivity, note))
	})

	t.Run("extractDeleteTarget branches", func(t *testing.T) {
		objectID, original, err := env.handler.extractDeleteTarget(&activitypub.Activity{Object: "https://example.com/objects/1"})
		require.NoError(t, err)
		require.Equal(t, "https://example.com/objects/1", objectID)
		require.Nil(t, original)

		embedded := map[string]any{"id": "https://example.com/objects/2", "type": activitypub.NoteType}
		objectID, original, err = env.handler.extractDeleteTarget(&activitypub.Activity{Object: embedded})
		require.NoError(t, err)
		require.Equal(t, "https://example.com/objects/2", objectID)
		require.NotNil(t, original)

		objectID, original, err = env.handler.extractDeleteTarget(&activitypub.Activity{Object: &activitypub.BaseObject{ID: "https://example.com/objects/3"}})
		require.NoError(t, err)
		require.Equal(t, "https://example.com/objects/3", objectID)
		require.Nil(t, original)

		_, _, err = env.handler.extractDeleteTarget(&activitypub.Activity{Object: 123})
		require.Error(t, err)
	})

	t.Run("extractCollectionType branches", func(t *testing.T) {
		_, err := env.handler.extractCollectionType("")
		require.Error(t, err)

		_, err = env.handler.extractCollectionType("bad")
		require.Error(t, err)

		collectionType, err := env.handler.extractCollectionType("https://example.com/users/alice/unknown")
		require.NoError(t, err)
		require.Equal(t, "unknown", collectionType)
	})

	t.Run("verifyCollectionAuthorization branches", func(t *testing.T) {
		target := env.local

		require.Error(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  env.remoteActorID,
			Target: "bad",
		}, "likes", target))

		require.Error(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  env.remoteActorID,
			Target: "https://example.com/alice/featured",
		}, "featured", target))

		require.NoError(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  target.ID,
			Target: target.ID + "/featured",
		}, "featured", target))

		require.NoError(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  "https://example.com/users/alice",
			Target: target.ID + "/likes",
		}, "likes", target))

		require.Error(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  env.remoteActorID,
			Target: target.ID + "/featured",
		}, "featured", target))

		require.Error(t, env.handler.verifyCollectionAuthorization(context.Background(), &activitypub.Activity{
			Actor:  env.remoteActorID,
			Target: target.ID + "/likes",
		}, "likes", target))
	})

	t.Run("enrichActivitiesWithObjects branches", func(t *testing.T) {
		liftCtx := newLiftContext("GET", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, nil)

		activities := []*activitypub.Activity{
			{BaseObject: activitypub.BaseObject{Type: activitypub.FollowType}},
			{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Object: nil},
			{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Object: map[string]any{"id": "x"}},
			{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-enrich"}, Object: env.cfg.BaseURL() + "/objects/1"},
		}

		env.handler.enrichActivitiesWithObjects(liftCtx, activities)

		_, ok := activities[3].Object.(string)
		require.False(t, ok)

		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.AnythingOfType("*models.Object")).Return(errors.New("boom")).Maybe()

		badRepo := repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		handler := *env.handler
		handler.objectRepository = badRepo

		failActivities := []*activitypub.Activity{
			{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-enrich-fail"}, Object: env.cfg.BaseURL() + "/objects/1"},
		}
		handler.enrichActivitiesWithObjects(liftCtx, failActivities)
		_, ok = failActivities[0].Object.(string)
		require.True(t, ok)
	})
}

func TestInboxHandler_Round10_CheckBlockStatus_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("blocked returns operation not allowed", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.AnythingOfType("*models.Block")).Return(nil).Maybe()

		handler := *env.handler
		handler.relationshipRepository = repositories.NewRelationshipRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		require.Error(t, handler.checkBlockStatus(context.Background(), env.remoteActorID, env.local.ID))
	})

	t.Run("repository error fails open", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.AnythingOfType("*models.Block")).Return(errors.New("boom")).Maybe()

		handler := *env.handler
		handler.relationshipRepository = repositories.NewRelationshipRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		require.NoError(t, handler.checkBlockStatus(context.Background(), env.remoteActorID, env.local.ID))
	})
}
