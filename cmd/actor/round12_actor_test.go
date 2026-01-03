package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	pkgtypes "github.com/pay-theory/dynamorm/pkg/types"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeActorRepo struct {
	actor       *activitypub.Actor
	err         error
	gotUsername string
}

func (f *fakeActorRepo) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	f.gotUsername = username
	if f.err != nil {
		return nil, f.err
	}
	return f.actor, nil
}

type fakeAuthorizedFetch struct {
	enabled   bool
	verifyErr error
}

func (f *fakeAuthorizedFetch) IsAuthorizedFetchEnabled(context.Context) bool { return f.enabled }

func (f *fakeAuthorizedFetch) VerifyAuthorizedFetch(context.Context, *http.Request) (*activitypub.Actor, error) {
	return nil, f.verifyErr
}

type fakeInstanceRepo struct {
	state *storageModels.InstanceState
	err   error
}

func (f *fakeInstanceRepo) GetInstanceState(context.Context) (*storageModels.InstanceState, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.state, nil
}

type extendedMockDB struct {
	inner *dynamormMocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query { return db.inner.Model(model) }

func (db *extendedMockDB) Transaction(fn func(tx *dynamormCore.Tx) error) error { return fn(nil) }

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error { return nil }

func (db *extendedMockDB) CreateTable(_ any, _ ...any) error { return nil }

func (db *extendedMockDB) EnsureTable(_ any) error { return nil }

func (db *extendedMockDB) DeleteTable(_ any) error { return nil }

func (db *extendedMockDB) DescribeTable(_ any) (any, error) { return nil, nil }

func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) dynamormCore.DB { return db }

func (db *extendedMockDB) TransactionFunc(fn func(tx any) error) error { return fn(nil) }

func (db *extendedMockDB) Transact() dynamormCore.TransactionBuilder { return nil }

func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	return fn(nil)
}

func TestActorHandler_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	t.Run("missing username", func(t *testing.T) {
		h := &Handler{}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/users/"}))
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
	})

	t.Run("locked bootstrap actor forbidden", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/users/bootstrap"}))
		ctx.SetParam("username", storageModels.DefaultBootstrapUsername)
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 403, liftErr.StatusCode)
	})

	t.Run("actor not found returns 404", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:    &fakeActorRepo{err: common.ActorNotFoundError{Username: "alice"}},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{Method: http.MethodGet, Path: "/users/alice"}))
		ctx.SetParam("username", "alice")
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 404, liftErr.StatusCode)
	})

	t.Run("authorized fetch conversion failure returns 400", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:  "bad method",
			Path:    "/users/alice",
			Headers: map[string]string{"Accept": "application/activity+json", "Host": "example.com"},
		}))
		ctx.SetParam("username", "alice")
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 400, liftErr.StatusCode)
		require.Equal(t, "REQUEST_CONVERSION_ERROR", liftErr.Code)
	})

	t.Run("authorized fetch missing signature returns 401", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:  http.MethodGet,
			Path:    "/users/alice",
			Headers: map[string]string{"Accept": "application/activity+json", "Host": "example.com"},
		}))
		ctx.SetParam("username", "alice")
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 401, liftErr.StatusCode)
	})

	t.Run("authorized fetch invalid signature returns 403", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("bad signature")},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:  http.MethodGet,
			Path:    "/users/alice",
			Headers: map[string]string{"Accept": "application/activity+json", "Host": "example.com", "Signature": "sig"},
		}))
		ctx.SetParam("username", "alice")
		err := h.HandleActorProfile(ctx)
		require.Error(t, err)
		var liftErr *lift.LiftError
		require.ErrorAs(t, err, &liftErr)
		require.Equal(t, 403, liftErr.StatusCode)
	})

	t.Run("json response sets activitypub content type", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice")},
			PreferredUsername: "alice",
		}
		repo := &fakeActorRepo{actor: actor}
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              repo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:  http.MethodGet,
			Path:    "/users/alice",
			Headers: map[string]string{"accept": "application/activity+json"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, h.HandleActorProfile(ctx))
		require.Equal(t, "application/activity+json", ctx.Response.Headers["Content-Type"])
		require.Equal(t, "alice", repo.gotUsername)
	})

	t.Run("html response renders profile", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:      cfg.ActorURL("alice"),
				Type:    activitypub.PersonType,
				Summary: "bio",
			},
			PreferredUsername: "alice",
			Followers:         cfg.ActorURL("alice") + "/followers",
			Following:         cfg.ActorURL("alice") + "/following",
			Icon:              &activitypub.Image{URL: "https://example.com/avatar.png"},
		}
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: actor},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
			Method:  http.MethodGet,
			Path:    "/users/alice",
			Headers: map[string]string{"Accept": "text/html"},
		}))
		ctx.SetParam("username", "alice")
		require.NoError(t, h.HandleActorProfile(ctx))
		require.Equal(t, "text/html; charset=utf-8", ctx.Response.Headers["Content-Type"])
		body, ok := ctx.Response.Body.(string)
		require.True(t, ok)
		require.Contains(t, body, "<!DOCTYPE html>")
		require.Contains(t, body, "@alice@example.com")
	})
}

func TestActorEntrypoint_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origRepos := repos
	origLambdaCtx := lambdaCtx
	origMust := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	origStart := lambdaStartFn
	origNewHandler := newHandlerFn
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		repos = origRepos
		lambdaCtx = origLambdaCtx
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origDefaults
		lambdaStartFn = origStart
		newHandlerFn = origNewHandler
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	innerDB := new(dynamormMocks.MockDB)
	db := &extendedMockDB{inner: innerDB}
	repoFactory, err := factory.NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	repos = repoFactory

	h := NewHandler()
	require.NotNil(t, h.actorRepo)

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:   cfg,
			Logger:   nil,
			Repos:    repos,
			StartTime: time.Now(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	initializeActor()

	newHandlerFn = func() *Handler {
		return &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice")}, PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, interface{}) (interface{}, error))
		require.True(t, ok)

		event := map[string]any{
			"version":  "2.0",
			"routeKey": "GET /users/alice",
			"rawPath":  "/users/alice",
			"headers": map[string]any{
				"accept": "application/activity+json",
			},
			"requestContext": map[string]any{
				"requestId": "req",
				"http": map[string]any{
					"method": "GET",
					"path":   "/users/alice",
				},
			},
		}

		resp, err := fn(context.Background(), event)
		require.NoError(t, err)
		liftResp, ok := resp.(*lift.Response)
		require.True(t, ok)
		require.Equal(t, 200, liftResp.StatusCode)
		require.Equal(t, "application/activity+json", liftResp.Headers["Content-Type"])
	}

	main()
	require.True(t, called)
}

func TestConvertLiftRequest_Round12(t *testing.T) {
	h := &Handler{}
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      http.MethodGet,
		Path:        "/users/alice",
		Headers:     map[string]string{"Host": "example.com", "X-Test": "1"},
		QueryParams: map[string]string{"q": "1"},
	}))

	req, err := h.convertLiftRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/users/alice?q=1", req.URL.String())
	require.Equal(t, "1", req.Header.Get("X-Test"))
}
