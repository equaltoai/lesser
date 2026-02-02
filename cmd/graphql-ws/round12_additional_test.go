package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

type fakeTokenValidator struct {
	claims *auth.Claims
	err    error
	calls  int32
	token  string
}

func (f *fakeTokenValidator) ValidateAccessToken(tokenString string) (*auth.Claims, error) {
	atomic.AddInt32(&f.calls, 1)
	f.token = tokenString
	return f.claims, f.err
}

type fakeGraphQLExecutor struct {
	createCalls   int32
	dispatchCalls int32
	create        func(ctx context.Context, params *graphql.RawParams) (*graphql.OperationContext, gqlerror.List)
	dispatch      func(ctx context.Context, opCtx *graphql.OperationContext) (graphql.ResponseHandler, context.Context)
}

func (f *fakeGraphQLExecutor) CreateOperationContext(ctx context.Context, params *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
	atomic.AddInt32(&f.createCalls, 1)
	if f.create != nil {
		return f.create(ctx, params)
	}
	return &graphql.OperationContext{}, nil
}

func (f *fakeGraphQLExecutor) DispatchOperation(ctx context.Context, opCtx *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
	atomic.AddInt32(&f.dispatchCalls, 1)
	if f.dispatch != nil {
		return f.dispatch(ctx, opCtx)
	}
	return func(context.Context) *graphql.Response { return nil }, ctx
}

type fakeConnRepo struct {
	writeErr            error
	updateErr           error
	deleteSubsErr       error
	deleteConnErr       error
	getConnErr          error
	deleteSubErr        error
	lastUpdated         *models.WebSocketConnection
	writeCalls          int32
	updateCalls         int32
	deleteSubsCalls     int32
	deleteConnCalls     int32
	getConnCalls        int32
	deleteSubCalls      int32
	lastDeleteSubConn   string
	lastDeleteSubStream string
}

func (f *fakeConnRepo) WriteConnection(_ context.Context, connectionID string, userID string, username string, streams []string) (*models.WebSocketConnection, error) {
	atomic.AddInt32(&f.writeCalls, 1)
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	return &models.WebSocketConnection{
		ConnectionID: connectionID,
		UserID:       userID,
		Username:     username,
		Streams:      streams,
	}, nil
}

func (f *fakeConnRepo) UpdateConnection(_ context.Context, connection *models.WebSocketConnection) error {
	atomic.AddInt32(&f.updateCalls, 1)
	f.lastUpdated = connection
	return f.updateErr
}

func (f *fakeConnRepo) DeleteAllSubscriptions(_ context.Context, _ string) error {
	atomic.AddInt32(&f.deleteSubsCalls, 1)
	return f.deleteSubsErr
}

func (f *fakeConnRepo) DeleteConnection(_ context.Context, _ string) error {
	atomic.AddInt32(&f.deleteConnCalls, 1)
	return f.deleteConnErr
}

func (f *fakeConnRepo) GetConnection(_ context.Context, _ string) (*models.WebSocketConnection, error) {
	atomic.AddInt32(&f.getConnCalls, 1)
	if f.getConnErr != nil {
		return nil, f.getConnErr
	}
	return &models.WebSocketConnection{
		ConnectionID: "c1",
		UserID:       "user",
		Username:     "user",
		Info: models.ConnectionInfo{
			CustomHeaders: map[string]string{
				"scopes": "read write",
			},
		},
	}, nil
}

func (f *fakeConnRepo) DeleteSubscription(_ context.Context, connectionID string, stream string) error {
	atomic.AddInt32(&f.deleteSubCalls, 1)
	f.lastDeleteSubConn = connectionID
	f.lastDeleteSubStream = stream
	return f.deleteSubErr
}

type fakeSubManager struct {
	running    bool
	startErr   error
	startCalls int32
}

func (f *fakeSubManager) IsRunning() bool { return f.running }

func (f *fakeSubManager) Start(_ context.Context) error {
	atomic.AddInt32(&f.startCalls, 1)
	f.running = true
	return f.startErr
}

type fakeInstanceRepo struct {
	state *models.InstanceState
	err   error
}

func (f *fakeInstanceRepo) GetInstanceState(_ context.Context) (*models.InstanceState, error) {
	return f.state, f.err
}

type fakeDynamoDB struct{}

func (fakeDynamoDB) Model(any) dynamormCore.Query                      { return nil }
func (fakeDynamoDB) Transaction(func(tx *dynamormCore.Tx) error) error { return nil }
func (fakeDynamoDB) Migrate() error                                    { return nil }
func (fakeDynamoDB) AutoMigrate(...any) error                          { return nil }
func (fakeDynamoDB) Close() error                                      { return nil }
func (fakeDynamoDB) WithContext(context.Context) dynamormCore.DB       { return fakeDynamoDB{} }

func setDummyAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_SESSION_TOKEN", "dummy")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestRegisterConnection_PersistsAndStoresState(t *testing.T) {
	repo := &fakeConnRepo{updateErr: errors.New("update failed")}
	s := newServer(nil, nil, nil, zap.NewNop(), repo, nil)

	err := s.registerConnection(context.Background(), "c1", "user", &auth.Claims{Username: "user", Scopes: []string{"read"}})
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.writeCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.updateCalls))
	require.NotNil(t, repo.lastUpdated)
	require.Equal(t, "graphql-ws", repo.lastUpdated.Info.Protocol)
	require.Equal(t, "oauth", repo.lastUpdated.Info.AuthMethod)
	require.Equal(t, "read", repo.lastUpdated.Info.CustomHeaders["scopes"])

	state, err2 := s.getConnection(context.Background(), "c1")
	require.NoError(t, err2)
	require.Equal(t, "user", state.username)
}

func TestRemoveConnection_CancelsSubscriptionsAndCleansRepo(t *testing.T) {
	repo := &fakeConnRepo{deleteSubsErr: errors.New("nope")}
	s := newServer(nil, nil, nil, zap.NewNop(), repo, nil)

	calls := 0
	s.connections["c1"] = &connectionState{
		username: "user",
		subscriptions: map[string]*subscriptionState{
			"sub1": {cancel: func() { calls++ }},
			"sub2": {cancel: func() { calls++ }},
		},
	}

	s.removeConnection(context.Background(), "c1")
	require.Equal(t, 2, calls)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubsCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteConnCalls))
	_, ok := s.connections["c1"]
	require.False(t, ok)
}

func TestGetConnection_FallsBackToRepoAndCaches(t *testing.T) {
	repo := &fakeConnRepo{}
	s := newServer(nil, nil, nil, zap.NewNop(), repo, nil)

	state, err := s.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Equal(t, "user", state.username)
	require.NotNil(t, state.claims)
	require.Equal(t, []string{"read", "write"}, state.claims.Scopes)

	state2, err := s.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Same(t, state, state2)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.getConnCalls))
}

func TestSubscriptionStreamNameAndRemoveSubscriptionRecord(t *testing.T) {
	repo := &fakeConnRepo{}
	s := newServer(nil, nil, nil, zap.NewNop(), repo, nil)

	require.Equal(t, "graphql:subscription:s1", subscriptionStreamName("s1"))

	s.removeSubscriptionRecord(context.Background(), "c1", "s1")
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubCalls))
	require.Equal(t, "c1", repo.lastDeleteSubConn)
	require.Equal(t, "graphql:subscription:s1", repo.lastDeleteSubStream)

	s.removeSubscriptionRecord(context.Background(), "c1", "")
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubCalls))
}

func TestEnsureSubscriptionManagerStarted_StartOnce(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	s.ensureSubscriptionManagerStarted()

	manager := &fakeSubManager{}
	s.subscriptionManager = manager

	s.ensureSubscriptionManagerStarted()
	s.ensureSubscriptionManagerStarted()
	require.Equal(t, int32(1), atomic.LoadInt32(&manager.startCalls))

	alreadyRunning := &fakeSubManager{running: true}
	s2 := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	s2.subscriptionManager = alreadyRunning
	s2.ensureSubscriptionManagerStarted()
	require.Equal(t, int32(0), atomic.LoadInt32(&alreadyRunning.startCalls))
}

func TestHandleConnect_TokenValidationAndState(t *testing.T) {
	setDummyAWSEnv(t)

	// Missing token => unauthorized.
	server := newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil)
	app := newWebSocketApp(server)
	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{}, map[string]string{}))
	require.Equal(t, 401, resp.StatusCode)

	// Token present but oauth service missing => internal error.
	server = newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	app = newWebSocketApp(server)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 500, resp.StatusCode)

	// Invalid token => unauthorized.
	badValidator := &fakeTokenValidator{err: errors.New("bad")}
	server = newServer(badValidator, nil, nil, zap.NewNop(), nil, nil)
	app = newWebSocketApp(server)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 401, resp.StatusCode)

	// Missing username => forbidden.
	noUserValidator := &fakeTokenValidator{claims: &auth.Claims{}}
	server = newServer(noUserValidator, nil, nil, zap.NewNop(), nil, nil)
	app = newWebSocketApp(server)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 403, resp.StatusCode)

	// Success.
	connRepo := &fakeConnRepo{}
	okValidator := &fakeTokenValidator{claims: &auth.Claims{Username: "user"}}
	server = newServer(okValidator, nil, nil, zap.NewNop(), connRepo, nil)
	app = newWebSocketApp(server)
	resp = app.ServeWebSocket(context.Background(), newWebSocketEvent("$connect", "c1", "", map[string]string{"access_token": "t"}, map[string]string{}))
	require.Equal(t, 200, resp.StatusCode)

	state, err := server.getConnection(context.Background(), "c1")
	require.NoError(t, err)
	require.Equal(t, "user", state.username)
	require.NotNil(t, server.wsContexts["c1"])
}

func TestHandleDisconnect_CleansUpAndPersists(t *testing.T) {
	setDummyAWSEnv(t)

	repo := &fakeConnRepo{}
	server := newServer(nil, nil, nil, zap.NewNop(), repo, nil)
	server.connections["c1"] = &connectionState{username: "user", subscriptions: map[string]*subscriptionState{}}
	server.wsContexts["c1"] = &apptheory.WebSocketContext{ConnectionID: "c1"}

	app := newWebSocketApp(server)
	resp := app.ServeWebSocket(context.Background(), newWebSocketEvent("$disconnect", "c1", "", nil, nil))
	require.Equal(t, 200, resp.StatusCode)
	require.Empty(t, server.wsContexts)
	require.Empty(t, server.connections)
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubsCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteConnCalls))
}

func TestHandleSubscribe_ErrorBranches(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	server := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	server.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}

	// Missing id.
	server.handleSubscribe(context.Background(), wsMessage{Type: "subscribe"}, wsCtx)
	require.Greater(t, len(msgs), 0)
	msgs = make(chan []byte, 10)

	// Instance repo missing.
	server.instanceRepo = nil
	server.handleSubscribe(context.Background(), wsMessage{ID: "s1", Type: "subscribe"}, wsCtx)
	require.Equal(t, 2, len(msgs)) // error + complete
	msgs = make(chan []byte, 10)

	// Instance locked.
	server.instanceRepo = &fakeInstanceRepo{state: &models.InstanceState{Locked: true}}
	server.handleSubscribe(context.Background(), wsMessage{ID: "s1", Type: "subscribe"}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// Connection context missing.
	server.instanceRepo = &fakeInstanceRepo{state: &models.InstanceState{Locked: false}}
	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "s1",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { costUpdates { operationCost dailyTotal monthlyProjection } }"}`),
	}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// Executor missing.
	server.connections["c1"] = &connectionState{username: "user", claims: &auth.Claims{Username: "user"}, subscriptions: map[string]*subscriptionState{}}
	server.exec = nil
	server.handleSubscribe(context.Background(), wsMessage{ID: "s1", Type: "subscribe", Payload: json.RawMessage(`{}`)}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// Payload parse error.
	server.exec = &fakeGraphQLExecutor{}
	server.handleSubscribe(context.Background(), wsMessage{ID: "s1", Type: "subscribe", Payload: json.RawMessage(`{`)}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// Missing query.
	server.handleSubscribe(context.Background(), wsMessage{ID: "s1", Type: "subscribe", Payload: json.RawMessage(`{}`)}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// CreateOperationContext errors.
	exec := &fakeGraphQLExecutor{
		create: func(_ context.Context, _ *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
			return nil, gqlerror.List{gqlerror.Errorf("boom")}
		},
	}
	server.exec = exec
	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "s1",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { costUpdates { operationCost dailyTotal monthlyProjection } }"}`),
	}, wsCtx)
	require.Equal(t, 2, len(msgs))
	msgs = make(chan []byte, 10)

	// Not a subscription.
	exec = &fakeGraphQLExecutor{
		create: func(_ context.Context, _ *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
			return &graphql.OperationContext{Operation: &ast.OperationDefinition{Operation: ast.Query}}, nil
		},
	}
	server.exec = exec
	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "s1",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"query { viewer { id } }"}`),
	}, wsCtx)
	require.Equal(t, 2, len(msgs))
}

func TestHandleSubscribe_SuccessPath(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	exec := &fakeGraphQLExecutor{
		create: func(_ context.Context, _ *graphql.RawParams) (*graphql.OperationContext, gqlerror.List) {
			return &graphql.OperationContext{Operation: &ast.OperationDefinition{Operation: ast.Subscription}}, nil
		},
		dispatch: func(ctx context.Context, _ *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
			calls := 0
			return func(context.Context) *graphql.Response {
				calls++
				if calls == 1 {
					return &graphql.Response{Data: []byte(`{"ok":true}`)}
				}
				return nil
			}, ctx
		},
	}

	repo := &fakeConnRepo{}
	server := newServer(nil, nil, exec, zap.NewNop(), repo, &fakeInstanceRepo{state: &models.InstanceState{Locked: false}})
	server.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}
	server.connections["c1"] = &connectionState{
		username:      "user",
		claims:        &auth.Claims{Username: "user"},
		subscriptions: make(map[string]*subscriptionState),
	}

	server.handleSubscribe(context.Background(), wsMessage{
		ID:      "sub1",
		Type:    "subscribe",
		Payload: json.RawMessage(`{"query":"subscription { costUpdates { operationCost dailyTotal monthlyProjection } }"}`),
	}, wsCtx)

	require.Eventually(t, func() bool { return len(msgs) >= 2 }, 200*time.Millisecond, 5*time.Millisecond)
}

func TestSendGraphQLResponse_FormatsPayload(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}
	require.NoError(t, s.sendGraphQLResponse(wsCtx, "id1", nil))
	require.Equal(t, 1, len(msgs))

	err := s.sendGraphQLResponse(wsCtx, "id2", &graphql.Response{Data: []byte("{")})
	require.Error(t, err)

	hasNext := true
	resp := &graphql.Response{
		Data:       []byte(`{"ok":true}`),
		Errors:     gqlerror.List{gqlerror.Errorf("boom")},
		Extensions: map[string]any{"x": "y"},
		HasNext:    &hasNext,
		Label:      "lbl",
	}
	require.NoError(t, s.sendGraphQLResponse(wsCtx, "id3", resp))
	require.Equal(t, 2, len(msgs))
}

func TestExecuteSubscription_SendsNextAndComplete(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	exec := &fakeGraphQLExecutor{
		dispatch: func(_ context.Context, _ *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
			calls := 0
			return func(context.Context) *graphql.Response {
				calls++
				if calls == 1 {
					return &graphql.Response{Data: []byte(`{"ok":true}`)}
				}
				return nil
			}, context.Background()
		},
	}

	repo := &fakeConnRepo{}
	s := newServer(nil, nil, exec, zap.NewNop(), repo, nil)
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}
	s.connections["c1"] = &connectionState{username: "user", subscriptions: map[string]*subscriptionState{"sub1": {cancel: func() {}}}}

	cancelled := 0
	cancel := func() { cancelled++ }
	s.executeSubscription(context.Background(), "c1", "sub1", &graphql.OperationContext{}, cancel, wsCtx)

	require.GreaterOrEqual(t, cancelled, 1)
	require.Equal(t, 2, len(msgs)) // next + complete
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubCalls))
}

func TestExecuteSubscription_RecoversPanic(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	exec := &fakeGraphQLExecutor{
		dispatch: func(_ context.Context, _ *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
			return func(context.Context) *graphql.Response {
				panic("boom")
			}, context.Background()
		},
	}

	s := newServer(nil, nil, exec, zap.NewNop(), nil, nil)
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}
	s.connections["c1"] = &connectionState{username: "user", subscriptions: map[string]*subscriptionState{"sub1": {cancel: func() {}}}}

	cancel := func() {}
	s.executeSubscription(context.Background(), "c1", "sub1", &graphql.OperationContext{}, cancel, wsCtx)

	require.Equal(t, 2, len(msgs)) // error + complete
}

func TestInitializeHelpersAndFallbacks(t *testing.T) {
	origMustInit := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInit
		initializeWithDefaultsFn = origDefaults
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewFactory
		lambdaCtx = nil
		cfg = nil
		logger = nil
		repos = nil
		oauth = nil
		server = nil
	})

	setDummyAWSEnv(t)

	mockStorage := pkgtesting.NewMockRepositoryStorage()
	fakeQueue := &struct{ streaming.StreamQueueService }{}

	cfg = &appconfig.Config{
		Domain:          "example.com",
		JWTSecret:       "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		DynamoTableName: "tbl",
		Region:          "us-east-1",
	}
	logger = zap.NewNop()

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:      cfg,
			Logger:      logger,
			Repos:       mockStorage,
			StreamQueue: fakeQueue,
		}
	}

	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("force fallback") }
	newLambdaOptimizedClientFn = func(_ context.Context, _ string) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (core.RepositoryStorage, error) {
		return mockStorage, nil
	}

	initializeGraphQLWS()
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, server)

	// Exercise helper functions directly.
	extractServices()
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
	require.NotNil(t, repos)

	require.NotNil(t, resolveStreamQueue())

	initializeOAuth()
	require.NotNil(t, oauth)

	resolver, exec := initializeResolver()
	require.NotNil(t, resolver)
	require.NotNil(t, exec)
	require.NotNil(t, resolver.SubscriptionManager)

	// Connection repository is optional in tests; should not panic.
	initializeConnectionRepository()
}

func TestInitializeGraphQLWS_SuccessPath(t *testing.T) {
	origMustInit := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInit
		initializeWithDefaultsFn = origDefaults
		lambdaCtx = nil
		cfg = nil
		logger = nil
		repos = nil
		oauth = nil
		server = nil
	})

	setDummyAWSEnv(t)

	storage := pkgtesting.NewMockRepositoryStorage()
	queue := &struct{ streaming.StreamQueueService }{}

	cfg = &appconfig.Config{
		Domain:          "example.com",
		JWTSecret:       "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		DynamoTableName: "tbl",
		Region:          "us-east-1",
	}
	logger = zap.NewNop()

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:      cfg,
			Logger:      logger,
			Repos:       storage,
			StreamQueue: queue,
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }

	initializeGraphQLWS()
	require.NotNil(t, server)
	require.NotNil(t, oauth)
}

func TestResolveStreamQueue_FallbacksAndErrors(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origNewClient
		lambdaCtx = nil
		cfg = nil
		logger = nil
		repos = nil
	})

	logger = zap.NewNop()
	cfg = &appconfig.Config{
		DynamoTableName: "tbl",
		Region:          "us-east-1",
	}

	// Error path when no stream queue, no db, and dynamo client initialization fails.
	lambdaCtx = &common.LambdaContext{}
	repos = nil
	newLambdaOptimizedClientFn = func(_ context.Context, _ string) (dynamormCore.DB, error) {
		return nil, errors.New("boom")
	}
	require.Nil(t, resolveStreamQueue())

	// Fallback path uses new client when repos.GetDB is nil.
	repos = pkgtesting.NewMockRepositoryStorage()
	newLambdaOptimizedClientFn = func(_ context.Context, _ string) (dynamormCore.DB, error) {
		return nil, nil
	}
	require.NotNil(t, resolveStreamQueue())
}

func TestInitializeConnectionRepository_CreatesRepoWithOverrides(t *testing.T) {
	origCtx := lambdaCtx
	origCfg := cfg
	origLogger := logger
	origConnRepo := connectionRepo
	t.Cleanup(func() {
		lambdaCtx = origCtx
		cfg = origCfg
		logger = origLogger
		connectionRepo = origConnRepo
	})

	cfg = &appconfig.Config{
		DynamoTableName:    "main",
		ConnectionsTable:   "connections",
		SubscriptionsTable: "subscriptions",
	}
	logger = zap.NewNop()
	lambdaCtx = &common.LambdaContext{
		DynamoDB: dynamormCore.DB(fakeDynamoDB{}),
	}
	repos = nil

	initializeConnectionRepository()
	require.NotNil(t, connectionRepo)
}

func TestInitializeManualServices_RegionSelection(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	origNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewFactory
		lambdaCtx = nil
		cfg = nil
		logger = nil
		repos = nil
	})

	logger = zap.NewNop()
	cfg = &appconfig.Config{
		DynamoTableName: "tbl",
	}
	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: logger,
	}

	newLambdaOptimizedClientFn = func(_ context.Context, _ string) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (core.RepositoryStorage, error) {
		return pkgtesting.NewMockRepositoryStorage(), nil
	}

	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	initializeManualServices()
	require.Equal(t, "eu-west-1", cfg.Region)
	require.Equal(t, "eu-west-1", os.Getenv("AWS_REGION"))

	// Default region when neither config nor env is set.
	cfg.Region = ""
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	initializeManualServices()
	require.Equal(t, "us-east-1", cfg.Region)
}

func TestMain_InvokesLambdaStart(t *testing.T) {
	origStart := lambdaStartFn
	origServer := server
	origLogger := logger
	t.Cleanup(func() {
		lambdaStartFn = origStart
		server = origServer
		logger = origLogger
	})

	calls := 0
	lambdaStartFn = func(handler any) {
		calls++
		require.NotNil(t, handler)
	}

	logger = zap.NewNop()
	server = newServer(&fakeTokenValidator{}, nil, nil, zap.NewNop(), nil, nil)

	main()
	require.Equal(t, 1, calls)
}

func TestExecuteSubscription_ReturnsOnSendError(t *testing.T) {
	setDummyAWSEnv(t)

	msgs := make(chan []byte, 10)
	wsCtx := &apptheory.WebSocketContext{ConnectionID: "c1"}

	exec := &fakeGraphQLExecutor{
		dispatch: func(_ context.Context, _ *graphql.OperationContext) (graphql.ResponseHandler, context.Context) {
			calls := 0
			return func(context.Context) *graphql.Response {
				calls++
				if calls == 1 {
					return &graphql.Response{Data: []byte("{")}
				}
				return nil
			}, context.Background()
		},
	}

	s := newServer(nil, nil, exec, zap.NewNop(), nil, nil)
	s.sendJSONMessage = func(_ *apptheory.WebSocketContext, payload any) error {
		b, mErr := json.Marshal(payload)
		require.NoError(t, mErr)
		msgs <- b
		return nil
	}
	s.connections["c1"] = &connectionState{username: "user", subscriptions: map[string]*subscriptionState{"sub1": {cancel: func() {}}}}

	s.executeSubscription(context.Background(), "c1", "sub1", &graphql.OperationContext{}, func() {}, wsCtx)
	require.Equal(t, 1, len(msgs)) // complete only
}

func TestAddSubscription_UnknownConnection(t *testing.T) {
	s := newServer(nil, nil, nil, zap.NewNop(), nil, nil)
	require.False(t, s.addSubscription("missing", "sub", func() {}))
}

func TestRemoveSubscriptionRecord_DeleteError(t *testing.T) {
	repo := &fakeConnRepo{deleteSubErr: errors.New("nope")}
	s := newServer(nil, nil, nil, zap.NewNop(), repo, nil)
	s.removeSubscriptionRecord(context.Background(), "c1", "s1")
	require.Equal(t, int32(1), atomic.LoadInt32(&repo.deleteSubCalls))
}

func TestNewServer_DefaultsLogger(t *testing.T) {
	s := newServer(nil, nil, nil, nil, nil, nil)
	require.NotNil(t, s)
	require.NotNil(t, s.logger)
}

func TestInitializeManualServices_PrefersAWSRegionEnv(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	origNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewFactory
		lambdaCtx = nil
		cfg = nil
		logger = nil
		repos = nil
	})

	logger = zap.NewNop()
	cfg = &appconfig.Config{
		DynamoTableName: "tbl",
	}
	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: logger,
	}

	newLambdaOptimizedClientFn = func(_ context.Context, _ string) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (core.RepositoryStorage, error) {
		return pkgtesting.NewMockRepositoryStorage(), nil
	}

	t.Setenv("AWS_REGION", "ap-south-1")
	t.Setenv("AWS_DEFAULT_REGION", "")
	initializeManualServices()
	require.Equal(t, "ap-south-1", cfg.Region)
}
