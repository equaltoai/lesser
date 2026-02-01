package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	pkgtypes "github.com/pay-theory/dynamorm/pkg/types"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error {
	return nil
}

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
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/users/"},
			Params:  map[string]string{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 422, resp.Status)
	})

	t.Run("locked bootstrap actor forbidden", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/users/bootstrap"},
			Params:  map[string]string{"username": storageModels.DefaultBootstrapUsername},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
	})

	t.Run("actor not found returns 404", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:    &fakeActorRepo{err: common.ActorNotFoundError{Username: "alice"}},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/users/alice"},
			Params:  map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("authorized fetch conversion failure returns 400", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{
				Method: "bad method",
				Path:   "/users/alice",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})

	t.Run("authorized fetch invalid signature returns 403", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              &fakeActorRepo{actor: &activitypub.Actor{PreferredUsername: "alice"}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("bad signature")},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice",
				Headers: map[string][]string{
					"accept":    {"application/activity+json"},
					"host":      {"example.com"},
					"signature": {"sig"},
				},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
	})

	t.Run("json response sets activitypub content type", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice")},
			PreferredUsername: "alice",
		}
		repo := &fakeActorRepo{actor: actor}
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			actorRepo:              repo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
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
		resp, err := h.HandleActorProfile(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"username": "alice"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		body := string(resp.Body)
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
			Config:    cfg,
			Logger:    nil,
			Repos:     repos,
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
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.APIGatewayV2HTTPRequest{
			Version:  "2.0",
			RouteKey: "GET /users/alice",
			RawPath:  "/users/alice",
			Headers:  map[string]string{"accept": "application/activity+json"},
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/users/alice",
				},
			},
		}

		raw, err := json.Marshal(event)
		require.NoError(t, err)
		resp, err := fn(context.Background(), raw)
		require.NoError(t, err)
		lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, 200, lambdaResp.StatusCode)
		require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])
	}

	main()
	require.True(t, called)
}

func TestConvertAppTheoryRequest_Round12(t *testing.T) {
	h := &Handler{}
	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodGet,
			Path:   "/users/alice",
			Headers: map[string][]string{
				"host":   {"example.com"},
				"x-test": {"1"},
			},
			Query: map[string][]string{"q": {"1"}},
		},
	}

	req, err := h.convertAppTheoryRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/users/alice?q=1", req.URL.String())
	require.Equal(t, "1", req.Header.Get("X-Test"))
}
