package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

type fakeInstanceRepo struct {
	state *storageModels.InstanceState
	err   error
}

func (f *fakeInstanceRepo) GetInstanceState(context.Context) (*storageModels.InstanceState, error) {
	return f.state, f.err
}

func TestParseWebFingerResource(t *testing.T) {
	username, domain, err := parseWebFingerResource("acct:alice@example.com")
	require.NoError(t, err)
	require.Equal(t, "alice", username)
	require.Equal(t, "example.com", domain)

	_, _, err = parseWebFingerResource("not-a-webfinger")
	require.Error(t, err)
}

func TestWebFingerHandler_handleWebFinger_Branches(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actorRepo := testingmocks.NewMockActorRepository()

	handler := &WebFingerHandler{
		actorRepo:    actorRepo,
		instanceRepo: &fakeInstanceRepo{err: errors.New("boom")},
		logger:       zap.NewNop(),
		cfg:          cfg,
	}

	t.Run("missing resource", func(t *testing.T) {
		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
		})
		require.Equal(t, 422, resp.Status)
	})

	t.Run("invalid resource format", func(t *testing.T) {
		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {"acct:bad"}},
		})
		require.Equal(t, 422, resp.Status)
	})

	t.Run("domain mismatch", func(t *testing.T) {
		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {"acct:alice@other.example"}},
		})
		require.Equal(t, 404, resp.Status)
	})

	t.Run("locked instance hides non-bootstrap", func(t *testing.T) {
		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {"acct:alice@example.com"}},
		})
		require.Equal(t, 404, resp.Status)
	})

	t.Run("unlocked instance requires actor", func(t *testing.T) {
		handler.instanceRepo = &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}}

		actorRepo.On("GetActor", mock.Anything, "alice").Return(nil, common.ActorNotFoundError{Username: "alice"}).Once()

		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {"acct:alice@example.com"}},
		})
		require.Equal(t, 404, resp.Status)
	})

	t.Run("unlocked instance actor lookup error", func(t *testing.T) {
		handler.instanceRepo = &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}}

		actorRepo.On("GetActor", mock.Anything, "alice").Return(nil, errors.New("db down")).Once()

		app := buildApp(handler, zap.NewNop())
		resp := app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {"acct:alice@example.com"}},
		})
		require.Equal(t, 500, resp.Status)
	})
}

func TestWebFingerHandler_handleWebFinger_SuccessAndBootstrap(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actorRepo := testingmocks.NewMockActorRepository()

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{}, nil).Maybe()
	actorRepo.On("GetActor", mock.Anything, "bootstrap").Return(&activitypub.Actor{}, nil).Maybe()

	handler := &WebFingerHandler{
		actorRepo:    actorRepo,
		instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		logger:       zap.NewNop(),
		cfg:          cfg,
	}

	call := func(resource string) apptheory.Response {
		app := buildApp(handler, zap.NewNop())
		return app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {resource}},
		})
	}

	resp := call("acct:alice@example.com")
	require.Equal(t, 200, resp.Status)
	require.Equal(t, []string{"application/jrd+json"}, resp.Headers["content-type"])
	require.Equal(t, []string{CacheControlMaxAge}, resp.Headers["cache-control"])

	// Locked instance: only bootstrap is discoverable, and must be ensured.
	handler.instanceRepo = &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true, BootstrapUsername: "bootstrap"}}
	resp = call("acct:bootstrap@example.com")
	require.Equal(t, 200, resp.Status)
}

func TestWebFingerHandler_ensureBootstrapActor_Branches(t *testing.T) {
	origGenerate := rsaGenerateKeyFn
	origNow := timeNowFn
	origCfg := cfg
	t.Cleanup(func() {
		rsaGenerateKeyFn = origGenerate
		timeNowFn = origNow
		cfg = origCfg
	})

	cfg = &config.Config{Domain: "example.com"}
	timeNowFn = func() time.Time { return time.Unix(1, 0) }
	rsaGenerateKeyFn = func(_ io.Reader, _ int) (*rsa.PrivateKey, error) {
		return rsa.GenerateKey(rand.Reader, 2048)
	}

	actorRepo := testingmocks.NewMockActorRepository()
	handler := &WebFingerHandler{
		actorRepo: actorRepo,
		logger:    zap.NewNop(),
	}

	// Actor exists -> no-op.
	actorRepo.On("GetActor", mock.Anything, "bootstrap").Return(&activitypub.Actor{}, nil).Once()
	require.NoError(t, handler.ensureBootstrapActor(context.Background(), "bootstrap"))

	// Not found -> CreateActor conflict -> treated as success.
	actorRepo.On("GetActor", mock.Anything, "bootstrap2").Return(nil, common.ActorNotFoundError{Username: "bootstrap2"}).Once()
	actorRepo.On("CreateActor", mock.Anything, mock.Anything, mock.Anything).Return(common.ConflictError{Resource: "actor", Message: "exists"}).Once()
	require.NoError(t, handler.ensureBootstrapActor(context.Background(), "bootstrap2"))

	// Non-not-found GetActor error propagates.
	actorRepo.On("GetActor", mock.Anything, "bootstrap3").Return(nil, errors.New("boom")).Once()
	require.Error(t, handler.ensureBootstrapActor(context.Background(), "bootstrap3"))

	// CreateActor non-conflict error propagates.
	actorRepo.On("GetActor", mock.Anything, "bootstrap4").Return(nil, common.ActorNotFoundError{Username: "bootstrap4"}).Once()
	actorRepo.On("CreateActor", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("boom")).Once()
	require.Error(t, handler.ensureBootstrapActor(context.Background(), "bootstrap4"))
}

func TestWebFingerHandler_EnsureBootstrapActor_CreatesManifestDrivenActor(t *testing.T) {
	origGenerate := rsaGenerateKeyFn
	origNow := timeNowFn
	origCfg := cfg
	t.Cleanup(func() {
		rsaGenerateKeyFn = origGenerate
		timeNowFn = origNow
		cfg = origCfg
	})

	cfg = &config.Config{Domain: "example.com"}
	timeNowFn = func() time.Time { return time.Unix(2, 0) }
	rsaGenerateKeyFn = func(_ io.Reader, _ int) (*rsa.PrivateKey, error) {
		return rsa.GenerateKey(rand.Reader, 2048)
	}

	actorRepo := testingmocks.NewMockActorRepository()
	handler := &WebFingerHandler{
		actorRepo: actorRepo,
		logger:    zap.NewNop(),
	}

	var created *activitypub.Actor
	actorRepo.On("GetActor", mock.Anything, "bootstrap").Return(nil, common.ActorNotFoundError{Username: "bootstrap"}).Once()
	actorRepo.On("CreateActor", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		created = args.Get(1).(*activitypub.Actor)
	}).Return(nil).Once()

	require.NoError(t, handler.ensureBootstrapActor(context.Background(), "bootstrap"))
	require.NotNil(t, created)
	require.Equal(t, "https://example.com/inbox", created.Endpoints.SharedInbox)
	require.Equal(t, "https://example.com/users/bootstrap/inbox", created.Inbox)
	require.Equal(t, "https://example.com/users/bootstrap/outbox", created.Outbox)
}

func TestBuildApp_WebFingerRoute(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	actorRepo := testingmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{}, nil).Maybe()

	handler := &WebFingerHandler{
		actorRepo:    actorRepo,
		instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		logger:       zap.NewNop(),
		cfg:          cfg,
	}

	app := buildApp(handler, zap.NewNop())

	call := func(resource string) apptheory.Response {
		return app.Serve(context.Background(), apptheory.Request{
			Method: "GET",
			Path:   "/.well-known/webfinger",
			Query:  map[string][]string{"resource": {resource}},
		})
	}

	require.Equal(t, 200, call("acct:alice@example.com").Status)
	require.Equal(t, 422, call("bad").Status)
}

func TestRunWebFinger_UsesLambdaStartFn(t *testing.T) {
	origStart := lambdaStartFn
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() { lambdaStartFn = origStart })

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	actorRepo := testingmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{}, nil).Maybe()

	handler := &WebFingerHandler{
		actorRepo:    actorRepo,
		instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		logger:       zap.NewNop(),
		cfg:          cfg,
	}

	event := events.APIGatewayV2HTTPRequest{
		Version:               "2.0",
		RouteKey:              "GET /.well-known/webfinger",
		RawPath:               "/.well-known/webfinger",
		QueryStringParameters: map[string]string{"resource": "acct:alice@example.com"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-id",
			Stage:     "$default",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/.well-known/webfinger",
			},
		},
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		h, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)
		raw, err := json.Marshal(event)
		require.NoError(t, err)
		result, err := h(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := result.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, 200, resp.StatusCode)
	}

	runWebFinger(handler, &common.LambdaContext{Logger: zap.NewNop()})
	require.True(t, called)
}

func TestNewWebFingerHandler_WiresInstanceRepo(t *testing.T) {
	origLambdaCtx := lambdaCtx
	origRepos := repos
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		lambdaCtx = origLambdaCtx
		repos = origRepos
		cfg = origCfg
		logger = origLogger
	})

	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}
	repos = testingmocks.NewMockRepositoryStorage()
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	handler := NewWebFingerHandler()
	require.NotNil(t, handler.actorRepo)
	require.NotNil(t, handler.instanceRepo)
}

func TestInitializeWebFinger_SetsGlobals(t *testing.T) {
	origMustInitialize := mustInitializeLambdaFn
	origInitializeDefaults := initializeWithDefaultsFn
	origLambdaCtx := lambdaCtx
	origRepos := repos
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInitialize
		initializeWithDefaultsFn = origInitializeDefaults
		lambdaCtx = origLambdaCtx
		repos = origRepos
		cfg = origCfg
		logger = origLogger
	})

	fakeRepos := testingmocks.NewMockRepositoryStorage()
	fakeLambdaCtx := &common.LambdaContext{
		Config: &config.Config{Domain: "example.com"},
		Logger: zap.NewNop(),
		Repos:  fakeRepos,
	}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext { return fakeLambdaCtx }
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }

	initializeWebFinger()
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, repos)
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
}
