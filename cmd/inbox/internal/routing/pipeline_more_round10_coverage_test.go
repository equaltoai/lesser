package routing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/golang-jwt/jwt/v5"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

func TestInboxHandler_Round10_ProcessAddRemove_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	target := env.local.ID + "/featured"
	objectID := env.cfg.BaseURL() + "/objects/1"

	t.Run("add missing target", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-missing-target"},
			Actor:      env.local.ID,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add missing object", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-missing-object"},
			Actor:      env.local.ID,
			Target:     target,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add object missing id", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-object-no-id"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     map[string]any{"type": activitypub.NoteType},
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add invalid target", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-invalid-target"},
			Actor:      env.local.ID,
			Target:     "bad",
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add unauthorized", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-unauthorized"},
			Actor:      env.remoteActorID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add item persistence fails", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Create").Return(errors.New("boom")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-create-fail"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove missing target", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-missing-target"},
			Actor:      env.local.ID,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove missing object", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-missing-object"},
			Actor:      env.local.ID,
			Target:     target,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove object missing id", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-object-no-id"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     map[string]any{"type": activitypub.NoteType},
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove invalid target", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-invalid-target"},
			Actor:      env.local.ID,
			Target:     "bad",
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove unauthorized", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-unauthorized"},
			Actor:      env.remoteActorID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove idempotent not found", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Delete").Return(errors.New("not found")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-not-found"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.NoError(t, err)
	})

	t.Run("remove delete failure", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Delete").Return(errors.New("boom")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-boom"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})
}

func TestInboxHandler_Round10_RemoteCreateUpdateDelete_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	t.Run("create invalid object returns success", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-invalid-object"},
			Actor:      env.remoteActorID,
			Object:     "not-a-map",
		}
		require.NoError(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("create invalid note returns error", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-invalid-note"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"type":    activitypub.NoteType,
				"content": "missing id attributedTo etc",
			},
		}
		require.Error(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("create non-note object returns success", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-non-note"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": "Article"},
		}
		require.NoError(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("update invalid object returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-invalid-object"},
			Actor:      env.remoteActorID,
			Object:     "not-a-map",
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update missing id returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-missing-id"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": activitypub.NoteType},
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update unauthorized returns error", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-unauthorized"},
			Actor:      "https://remote.example/users/other",
			Object: map[string]any{
				"id":           env.cfg.BaseURL() + "/objects/1",
				"type":         activitypub.NoteType,
				"attributedTo": env.remoteActorID,
				"content":      "updated content",
			},
		}
		require.Error(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update non-note object returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-non-note"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":           env.cfg.BaseURL() + "/objects/1",
				"type":         "Article",
				"attributedTo": env.remoteActorID,
				"content":      "updated content",
			},
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("delete unsupported object returns success", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-unsupported"},
			Actor:      env.remoteActorID,
			Object:     123,
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete missing object id returns success", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-missing-id"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": "Tombstone"},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete unauthorized returns error", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-unauthorized"},
			Actor:      "https://remote.example/users/other",
			Object:     env.cfg.BaseURL() + "/objects/1",
		}
		require.Error(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete embedded object uses formerType", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-embedded"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":   env.cfg.BaseURL() + "/objects/1",
				"type": "Article",
			},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete typed object branch", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-typed"},
			Actor:      env.remoteActorID,
			Object:     &activitypub.BaseObject{ID: env.cfg.BaseURL() + "/objects/1"},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})
}

func TestInboxHandler_Round10_AuthenticateInboxRequest_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	makeToken := func(username string, issuedAt time.Time) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username": username,
			"scopes":   []string{"read"},
			"iat":      issuedAt.Unix(),
			"exp":      time.Now().Add(time.Hour).Unix(),
		})
		signed, err := token.SignedString([]byte(env.cfg.JWTSecret))
		require.NoError(t, err)
		return signed
	}

	t.Run("invalid header prefix", func(t *testing.T) {
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Basic abc",
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})

	t.Run("invalid token", func(t *testing.T) {
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer invalid",
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})

	t.Run("token too old triggers auth failure branch", func(t *testing.T) {
		oldToken := makeToken("alice", time.Now().Add(-48*time.Hour))
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer " + oldToken,
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})

	t.Run("username mismatch is forbidden", func(t *testing.T) {
		bobToken := makeToken("bob", time.Now())
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer " + bobToken,
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})
}

func TestInboxHandler_Round10_InboxPagination_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	headers := map[string]string{
		"Host": "localhost",
	}
	liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", headers, nil, nil)

	innerDB := new(dynamormMocks.MockDB)
	query := new(dynamormMocks.MockQuery)
	db := &extendedMockDB{inner: innerDB}

	innerDB.On("Model", mock.Anything).Return(query).Maybe()
	query.On("WithContext", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("All", mock.Anything).Return(errors.New("boom")).Maybe()

	badHandler := *env.handler
	badHandler.activityRepository = repositories.NewActivityRepository(db, env.cfg.DynamoTableName, zap.NewNop(), nil)

	_, err := badHandler.returnInboxCollection(liftCtx, env.local, "alice")
	require.Error(t, err)
	_, err = badHandler.returnInboxPage(liftCtx, env.local, "alice", 20, "")
	require.Error(t, err)

	page := env.handler.buildCollectionPage(env.local, []*activitypub.Activity{}, "cursor", "next", 20)
	require.NotEmpty(t, page.Next)
	require.NotEmpty(t, page.Prev)
}

func TestInboxHandler_Round10_HandlePostInbox_EarlyFailures(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("missing username param", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, []byte(`{}`))
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid content type", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "text/plain",
		}, nil, []byte(`{}`))
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("missing body", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, nil)
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid json body", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, []byte("{"))
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("activity not addressed to actor fails validation", func(t *testing.T) {
		raw := map[string]any{
			"@context": activitypub.Context,
			"type":     activitypub.CreateType,
			"id":       env.cfg.BaseURL() + "/activities/not-addressed",
			"actor":    env.remoteActorID,
			"to":       []string{"https://remote.example/users/other"},
			"object":   env.cfg.BaseURL() + "/objects/1",
		}
		body, err := json.Marshal(raw)
		require.NoError(t, err)

		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, body)
		ctx.Params["username"] = "alice"
		_, err = env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid attachments fail validation", func(t *testing.T) {
		raw := map[string]any{
			"@context": activitypub.Context,
			"type":     activitypub.CreateType,
			"id":       env.cfg.BaseURL() + "/activities/bad-attachments",
			"actor":    env.remoteActorID,
			"to":       []string{env.local.ID},
			"object": map[string]any{
				"type":       activitypub.NoteType,
				"id":         env.cfg.BaseURL() + "/objects/bad-attachments",
				"content":    "hi",
				"attachment": "not-an-array",
			},
		}
		body, err := json.Marshal(raw)
		require.NoError(t, err)

		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, body)
		ctx.Params["username"] = "alice"
		_, err = env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})
}
