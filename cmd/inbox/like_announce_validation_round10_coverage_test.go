package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInboxHandler_Round10_LikeActivity_MoreBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	objectID := env.cfg.BaseURL() + "/objects/1"

	t.Run("invalid object id returns success", func(t *testing.T) {
		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-invalid"},
			Actor:      env.remoteActorID,
			Object:     123,
		}
		require.NoError(t, env.handler.processLikeActivity(ctx, like, env.local))
	})

	t.Run("map object id extraction", func(t *testing.T) {
		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-map"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"id": objectID},
		}
		require.NoError(t, env.handler.processLikeActivity(ctx, like, env.local))
	})

	t.Run("object fetch error skips notification", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.Anything).Return(errors.New("boom")).Maybe()
		query.On("Create").Return(nil).Maybe()
		query.On("IfNotExists").Return(query).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-no-object"},
			Actor:      env.remoteActorID,
			Object:     objectID,
		}
		require.NoError(t, handler.processLikeActivity(ctx, like, env.local))
	})

	t.Run("create like error returns createLikeError", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("IfNotExists").Return(query).Maybe()
		query.On("Create").Return(errors.New("boom")).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()

		handler := *env.handler
		handler.likeRepository = repositories.NewLikeRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.LikeType, ID: env.cfg.BaseURL() + "/activities/like-create-fail"},
			Actor:      env.remoteActorID,
			Object:     objectID,
		}
		require.Error(t, handler.processLikeActivity(ctx, like, env.local))
	})
}

func TestInboxHandler_Round10_AnnounceActivity_MoreBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()
	objectID := env.cfg.BaseURL() + "/objects/1"

	t.Run("invalid object id returns success", func(t *testing.T) {
		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-invalid"},
			Actor:      env.remoteActorID,
			Object:     123,
		}
		require.NoError(t, env.handler.processAnnounceActivity(ctx, announce, env.local))
	})

	t.Run("map object id extraction", func(t *testing.T) {
		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-map"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"id": objectID},
		}
		require.NoError(t, env.handler.processAnnounceActivity(ctx, announce, env.local))
	})

	t.Run("object fetch error skips notification", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.Anything).Return(errors.New("boom")).Maybe()
		query.On("Create").Return(nil).Maybe()
		query.On("IfNotExists").Return(query).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-no-object"},
			Actor:      env.remoteActorID,
			Object:     objectID,
		}
		require.NoError(t, handler.processAnnounceActivity(ctx, announce, env.local))
	})

	t.Run("create announce error returns createAnnounceError", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("IfNotExists").Return(query).Maybe()
		query.On("Create").Return(errors.New("boom")).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()

		handler := *env.handler
		handler.socialRepository = repositories.NewSocialRepository(db, env.cfg.DynamoTableName, zap.NewNop(), nil)

		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType, ID: env.cfg.BaseURL() + "/activities/announce-create-fail"},
			Actor:      env.remoteActorID,
			Object:     objectID,
		}
		require.Error(t, handler.processAnnounceActivity(ctx, announce, env.local))
	})
}

func TestInboxHandler_Round10_ValidationAndDigest_MoreBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("validateComprehensiveAddressing error", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: activitypub.CreateType,
				To:   []string{"ftp://example.com/users/alice"},
			},
		}
		require.Error(t, env.handler.validateComprehensiveAddressing(activity))
	})

	t.Run("validateBasicActor and public key errors", func(t *testing.T) {
		require.Error(t, env.handler.validateBasicActor(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "not-a-url", Type: activitypub.PersonType}}))
		require.Error(t, env.handler.validateActorPublicKey(&activitypub.Actor{
			PublicKey: &activitypub.PublicKey{
				ID:           "https://example.com/key",
				Owner:        "https://example.com/users/alice",
				PublicKeyPem: "not-a-pem",
			},
		}))
	})

	t.Run("validateActorUsername error", func(t *testing.T) {
		require.Error(t, env.handler.validateActorUsername("https://example.com/"))
	})

	t.Run("validateDirectMessage branches", func(t *testing.T) {
		require.Error(t, env.handler.validateDirectMessage(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{"not-a-url"},
			},
		}, env.local))

		require.Error(t, env.handler.validateDirectMessage(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{activitypub.PublicAddress, "https://example.com/users/bob"},
			},
		}, env.local))

		require.Error(t, env.handler.validateDirectMessage(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{"https://example.com/users/alice/followers"},
			},
		}, env.local))

		require.NoError(t, env.handler.validateDirectMessage(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{"https://example.com/users/bob"},
			},
		}, env.local))
	})

	t.Run("digest verification branches", func(t *testing.T) {
		body := []byte(`{"hello":"world"}`)

		liftCtx := newLiftContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
			"Digest":       "SHA-256=not-a-valid-digest",
		}, nil, body)
		liftCtx.SetParam("username", "alice")

		req := &InboxRequest{
			Activity: &activitypub.Activity{Actor: env.remoteActorID},
			Body:     body,
		}
		require.Error(t, env.handler.verifyDigest(liftCtx, req))
		require.Error(t, env.handler.verifyDigestEnhanced(liftCtx, req))

		convertFail := newLiftContext("BAD METHOD", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
			"Digest":       "SHA-256=not-a-valid-digest",
		}, nil, body)
		convertFail.SetParam("username", "alice")
		require.NoError(t, env.handler.verifyDigestEnhanced(convertFail, req))
	})

	t.Run("query utils not-found handling paths are stable", func(t *testing.T) {
		require.True(t, dynamormErrors.IsNotFound(dynamormErrors.ErrItemNotFound))
	})
}
