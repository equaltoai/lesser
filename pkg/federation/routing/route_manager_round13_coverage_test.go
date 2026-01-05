package routing

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeRouteOptimizer struct {
	optimizeFn func(ctx context.Context, routes []*fedTypes.Route, messageSize int64) ([]*fedTypes.Route, error)
	metricsFn  func(ctx context.Context, routeID string) (*fedTypes.RouteMetrics, error)
	recordFn   func(ctx context.Context, result *fedTypes.DeliveryResult) error
}

func (f fakeRouteOptimizer) OptimizeRoutes(ctx context.Context, routes []*fedTypes.Route, messageSize int64) ([]*fedTypes.Route, error) {
	if f.optimizeFn != nil {
		return f.optimizeFn(ctx, routes, messageSize)
	}
	return routes, nil
}

func (f fakeRouteOptimizer) GetRouteMetrics(ctx context.Context, routeID string) (*fedTypes.RouteMetrics, error) {
	if f.metricsFn != nil {
		return f.metricsFn(ctx, routeID)
	}
	return nil, errors.New("no metrics configured")
}

func (f fakeRouteOptimizer) RecordDeliveryResult(ctx context.Context, result *fedTypes.DeliveryResult) error {
	if f.recordFn != nil {
		return f.recordFn(ctx, result)
	}
	return nil
}

type fakeCircuitBreaker struct {
	mu sync.Mutex

	canAttemptFn      func(instanceID string) bool
	getStatusFn       func(instanceID string) fedTypes.CircuitStatus
	openFn            func(instanceID string, reason string) error
	closeFn           func(instanceID string) error
	recordSuccessFn   func(instanceID string) error
	recordFailureFn   func(instanceID string, err error) error
	emergencyModeFn   func(healthyRoutes, totalRoutes int) bool
	backpressureRules map[MessagePriority]BackpressureRule
	assessFn          func(ctx context.Context, routeID string, metrics *fedTypes.RouteMetrics) error

	successCalls []string
	failureCalls []string
}

func (f *fakeCircuitBreaker) AssessRouteHealthAndAdjustCircuit(ctx context.Context, routeID string, metrics *fedTypes.RouteMetrics) error {
	if f.assessFn != nil {
		return f.assessFn(ctx, routeID, metrics)
	}
	return nil
}

func (f *fakeCircuitBreaker) CanAttempt(instanceID string) bool {
	if f.canAttemptFn != nil {
		return f.canAttemptFn(instanceID)
	}
	return true
}

func (f *fakeCircuitBreaker) Close(instanceID string) error {
	if f.closeFn != nil {
		return f.closeFn(instanceID)
	}
	return nil
}

func (f *fakeCircuitBreaker) GetBackpressureRules() map[MessagePriority]BackpressureRule {
	if f.backpressureRules == nil {
		return make(map[MessagePriority]BackpressureRule)
	}
	return f.backpressureRules
}

func (f *fakeCircuitBreaker) GetStatus(instanceID string) fedTypes.CircuitStatus {
	if f.getStatusFn != nil {
		return f.getStatusFn(instanceID)
	}
	return fedTypes.CircuitClosed
}

func (f *fakeCircuitBreaker) Open(instanceID string, reason string) error {
	if f.openFn != nil {
		return f.openFn(instanceID, reason)
	}
	return nil
}

func (f *fakeCircuitBreaker) RecordFailure(instanceID string, err error) error {
	f.mu.Lock()
	f.failureCalls = append(f.failureCalls, instanceID)
	f.mu.Unlock()
	if f.recordFailureFn != nil {
		return f.recordFailureFn(instanceID, err)
	}
	return nil
}

func (f *fakeCircuitBreaker) RecordSuccess(instanceID string) error {
	f.mu.Lock()
	f.successCalls = append(f.successCalls, instanceID)
	f.mu.Unlock()
	if f.recordSuccessFn != nil {
		return f.recordSuccessFn(instanceID)
	}
	return nil
}

func (f *fakeCircuitBreaker) ShouldEnterEmergencyMode(healthyRoutes, totalRoutes int) bool {
	if f.emergencyModeFn != nil {
		return f.emergencyModeFn(healthyRoutes, totalRoutes)
	}
	return false
}

type fakeHTTPDoer struct {
	doFn func(req *http.Request) (*http.Response, error)
}

func (f fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	if f.doFn != nil {
		return f.doFn(req)
	}
	return nil, errors.New("no doFn configured")
}

type fakeFederationStore struct {
	actorFn          func(ctx context.Context, username string) (*activitypub.Actor, error)
	privateKeyFn     func(ctx context.Context, username string) (string, error)
	followersFn      func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	getCachedActorFn func(ctx context.Context, actorID string) (*activitypub.Actor, error)
	cacheActorFn     func(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
	recordFn         func(ctx context.Context, activity *storage.FederationActivity) error
}

func (f fakeFederationStore) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	if f.privateKeyFn != nil {
		return f.privateKeyFn(ctx, username)
	}
	return "", errors.New("no privateKeyFn configured")
}

func (f fakeFederationStore) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	if f.actorFn != nil {
		return f.actorFn(ctx, username)
	}
	return nil, errors.New("no actorFn configured")
}

func (f fakeFederationStore) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if f.followersFn != nil {
		return f.followersFn(ctx, username, limit, cursor)
	}
	return nil, "", nil
}

func (f fakeFederationStore) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	if f.getCachedActorFn != nil {
		return f.getCachedActorFn(ctx, actorID)
	}
	return nil, nil
}

func (f fakeFederationStore) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	if f.cacheActorFn != nil {
		return f.cacheActorFn(ctx, handle, actor, ttl)
	}
	return nil
}

func (f fakeFederationStore) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	if f.recordFn != nil {
		return f.recordFn(ctx, activity)
	}
	return nil
}

type fakeHealthRepo struct {
	getUnhealthyFn func(ctx context.Context, threshold float64) ([]string, error)
	getLatestFn    func(ctx context.Context, domain string) (*models.InstanceHealth, error)
}

func (f fakeHealthRepo) GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error) {
	if f.getLatestFn != nil {
		return f.getLatestFn(ctx, domain)
	}
	return nil, errors.New("no getLatestFn configured")
}

func (f fakeHealthRepo) GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error) {
	if f.getUnhealthyFn != nil {
		return f.getUnhealthyFn(ctx, threshold)
	}
	return nil, errors.New("no getUnhealthyFn configured")
}

type fakeCostRecorder struct {
	mu      sync.Mutex
	records []*models.FederationCostTracking
	err     error
}

func (f *fakeCostRecorder) RecordFederationCost(_ context.Context, tracker *models.FederationCostTracking) error {
	f.mu.Lock()
	f.records = append(f.records, tracker)
	f.mu.Unlock()
	return f.err
}

type fakeHealthChecker struct {
	checkFn func(instance *fedTypes.Instance) (*fedTypes.HealthStatus, error)
	startFn func(instance *fedTypes.Instance) error
}

func (f fakeHealthChecker) CheckHealth(instance *fedTypes.Instance) (*fedTypes.HealthStatus, error) {
	if f.checkFn != nil {
		return f.checkFn(instance)
	}
	return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, StatusCode: http.StatusOK}, nil
}

func (f fakeHealthChecker) StartMonitoring(instance *fedTypes.Instance) error {
	if f.startFn != nil {
		return f.startFn(instance)
	}
	return nil
}

func (f fakeHealthChecker) StopMonitoring(string) error { return nil }
func (f fakeHealthChecker) GetHealthHistory(string, time.Duration) ([]*fedTypes.HealthStatus, error) {
	return []*fedTypes.HealthStatus{}, nil
}

func rsaPrivateKeyPEM(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pemBytes, err := federation.EncodePrivateKeyPEM(key)
	require.NoError(t, err)

	return string(pemBytes), key
}

func newTestManager(t *testing.T) (*Manager, *FakeFederationInstanceRepository, *fakeCircuitBreaker) {
	t.Helper()

	logger := zaptest.NewLogger(t)
	repo := NewFakeFederationInstanceRepository()
	registry := NewInstanceRegistry(repo, logger)

	cb := &fakeCircuitBreaker{}

	m := &Manager{
		logger:           logger,
		config:           defaultRoutingConfig(),
		registry:         registry,
		optimizer:        noopRouteOptimizer{},
		circuitBreaker:   cb,
		healthChecker:    noopHealthChecker{},
		loadBalancer:     NewAdaptiveLoadBalancer(logger),
		thresholdManager: NewRouteThresholdManager(logger, DefaultThresholdConfig()),
		httpClient:       fakeHTTPDoer{},
		metrics:          NewRoutingMetrics(nil, "", logger),
		cacheTTL:         1 * time.Minute,
	}

	return m, repo, cb
}

func TestManager_NewManager_ConfigDefaults(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewFakeFederationInstanceRepository()

	m := NewManager(repo, nil, nil, nil, nil, nil, logger, nil)
	require.NotNil(t, m)
	require.NotNil(t, m.config)
	require.NotZero(t, m.cacheTTL)
	require.NotNil(t, m.registry)
	require.NotNil(t, m.optimizer)
	require.NotNil(t, m.circuitBreaker)
	require.NotNil(t, m.healthChecker)
	require.NotNil(t, m.thresholdManager)
	require.NotNil(t, m.httpClient)
	require.NotNil(t, m.metrics)
}

func TestManager_GetRoutes_CachesAndExpires(t *testing.T) {
	ctx := context.Background()
	m, repo, _ := newTestManager(t)

	inst := createTestInstance("example.com", "example.com")
	inst.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeCreate}
	inst.MonthlyQuota = 1000
	inst.CurrentUsage = 0
	repo.AddInstance(inst)

	// First call populates cache.
	routes1, err := m.GetRoutes("example.com")
	require.NoError(t, err)
	require.Len(t, routes1, 1)
	require.NotNil(t, routes1[0].Endpoint)

	// Second call should hit cache.
	routes2, err := m.GetRoutes("example.com")
	require.NoError(t, err)
	require.Len(t, routes2, 1)

	// Force cache expiration and ensure it rebuilds.
	cacheKey := fmt.Sprintf("routes:%s", "example.com")
	raw, ok := m.routeCache.Load(cacheKey)
	require.True(t, ok)
	cr := raw.(*cachedRoutes)
	cr.cachedAt = time.Now().Add(-10 * time.Minute)
	m.routeCache.Store(cacheKey, cr)

	_, err = m.GetRoutes("example.com")
	require.NoError(t, err)

	raw2, ok := m.routeCache.Load(cacheKey)
	require.True(t, ok)
	cr2 := raw2.(*cachedRoutes)
	assert.Less(t, time.Since(cr2.cachedAt), 2*time.Minute)

	_ = ctx
}

func TestManager_SelectRoute_Branches(t *testing.T) {
	ctx := context.Background()
	m, repo, cb := newTestManager(t)

	// Two instances backing cached routes.
	inst1 := createTestInstance("inst1", "one.example")
	inst1.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeCreate}
	inst1.MonthlyQuota = 1000
	inst1.CurrentUsage = 0
	repo.AddInstance(inst1)

	inst2 := createTestInstance("inst2", "two.example")
	inst2.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeCreate}
	inst2.MonthlyQuota = 1000
	inst2.CurrentUsage = 0
	repo.AddInstance(inst2)

	route1 := &fedTypes.Route{ID: "r1", InstanceID: inst1.ID, Domain: inst1.Domain, Priority: 1}
	route2 := &fedTypes.Route{ID: "r2", InstanceID: inst2.ID, Domain: inst2.Domain, Priority: 2}
	m.cacheRoutes("dest.example", []*fedTypes.Route{route1, route2})

	t.Run("no message type support", func(t *testing.T) {
		inst1.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeFollow}
		inst2.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeFollow}
		_, err := m.SelectRoute("dest.example", fedTypes.MessageTypeCreate)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoMessageTypeSupport)
	})

	t.Run("no healthy routes but half-open available", func(t *testing.T) {
		inst1.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeCreate}
		inst2.SupportedTypes = []fedTypes.MessageType{fedTypes.MessageTypeCreate}

		cb.canAttemptFn = func(string) bool { return false }
		cb.getStatusFn = func(id string) fedTypes.CircuitStatus {
			if id == inst2.ID {
				return fedTypes.CircuitHalfOpen
			}
			return fedTypes.CircuitOpen
		}

		got, err := m.SelectRoute("dest.example", fedTypes.MessageTypeCreate)
		require.NoError(t, err)
		assert.Equal(t, "r2", got.ID)
	})

	t.Run("optimizer error falls back", func(t *testing.T) {
		cb.canAttemptFn = func(string) bool { return true }
		cb.getStatusFn = nil

		m.optimizer = fakeRouteOptimizer{
			optimizeFn: func(context.Context, []*fedTypes.Route, int64) ([]*fedTypes.Route, error) {
				return nil, errors.New("optimize failed")
			},
		}

		got, err := m.SelectRoute("dest.example", fedTypes.MessageTypeCreate)
		require.NoError(t, err)
		assert.Equal(t, "r1", got.ID)
	})

	t.Run("optimizer reorders routes", func(t *testing.T) {
		m.optimizer = fakeRouteOptimizer{
			optimizeFn: func(_ context.Context, routes []*fedTypes.Route, _ int64) ([]*fedTypes.Route, error) {
				return []*fedTypes.Route{routes[1], routes[0]}, nil
			},
		}

		got, err := m.SelectRoute("dest.example", fedTypes.MessageTypeCreate)
		require.NoError(t, err)
		assert.Equal(t, "r2", got.ID)
	})

	t.Run("emergency mode backpressure drop", func(t *testing.T) {
		inst1.SupportedTypes = nil
		inst2.SupportedTypes = nil

		m.optimizer = noopRouteOptimizer{}
		cb.canAttemptFn = func(string) bool { return true }
		cb.emergencyModeFn = func(_, _ int) bool { return true }
		cb.backpressureRules = map[MessagePriority]BackpressureRule{
			PriorityLow: {Action: "drop", Threshold: 0.0},
		}
		cb.getStatusFn = func(string) fedTypes.CircuitStatus { return fedTypes.CircuitClosed }

		_, err := m.SelectRoute("dest.example", fedTypes.MessageTypeDelete)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMessageDroppedEmergency)
	})

	t.Run("emergency mode selects degraded route", func(t *testing.T) {
		m.emergencyMu.Lock()
		m.emergencyMode = true
		m.emergencyMu.Unlock()

		cb.backpressureRules = map[MessagePriority]BackpressureRule{}
		cb.canAttemptFn = func(string) bool { return true }

		got, err := m.SelectRoute("dest.example", fedTypes.MessageTypeCreate)
		require.NoError(t, err)
		assert.Equal(t, "r1", got.ID)

		m.emergencyMu.Lock()
		m.emergencyMode = false
		m.emergencyMu.Unlock()
	})

	_ = ctx
}

func TestManager_prepareActivityForDelivery_and_getSigningActor(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newTestManager(t)

	privateKeyPEM, _ := rsaPrivateKeyPEM(t)
	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/inbox",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://example.com/users/alice#main-key",
			Owner:        "https://example.com/users/alice",
			PublicKeyPem: "pem",
		},
	}

	m.federationStore = fakeFederationStore{
		actorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
			if username != "alice" {
				return nil, errors.New("unexpected username")
			}
			return signingActor, nil
		},
		privateKeyFn: func(_ context.Context, username string) (string, error) {
			if username != "alice" {
				return "", errors.New("unexpected username")
			}
			return privateKeyPEM, nil
		},
	}

	t.Run("payload parses into activity", func(t *testing.T) {
		payload, err := jsonMarshal(t, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: string(fedTypes.MessageTypeCreate)},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{"type": "Note"},
		})
		require.NoError(t, err)

		msg := &fedTypes.FederationMessage{
			ID:        "m1",
			Type:      fedTypes.MessageTypeCreate,
			Actor:     "https://example.com/users/alice",
			Payload:   payload,
			CreatedAt: time.Now(),
		}

		act, actor, err := m.prepareActivityForDelivery(ctx, msg, []string{"https://remote.example/inbox"})
		require.NoError(t, err)
		require.NotNil(t, act)
		require.NotNil(t, actor)
		assert.Equal(t, "alice", actor.PreferredUsername)
	})

	t.Run("builds activity when payload missing", func(t *testing.T) {
		msg := &fedTypes.FederationMessage{
			ID:        "m2",
			Type:      fedTypes.MessageTypeCreate,
			Actor:     "https://example.com/users/alice",
			Payload:   nil,
			Object:    map[string]any{"type": "Note"},
			CreatedAt: time.Now(),
		}

		act, actor, err := m.prepareActivityForDelivery(ctx, msg, []string{"https://remote.example/inbox"})
		require.NoError(t, err)
		require.NotNil(t, act)
		require.NotNil(t, actor)
		assert.Equal(t, msg.ID, act.ID)
		assert.Equal(t, msg.Actor, act.Actor)
		assert.NotEmpty(t, act.To)
	})

	t.Run("getSigningActor errors when store missing", func(t *testing.T) {
		m2, _, _ := newTestManager(t)
		_, err := m2.getSigningActor(ctx, "https://example.com/users/alice")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationStoreNotConfigured)
	})

	t.Run("getSigningActor errors on malformed actorID", func(t *testing.T) {
		_, err := m.getSigningActor(ctx, "not-a-url")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrExtractUsernameFromActorID)
	})
}

func TestManager_performHTTPDelivery_ResultBranches(t *testing.T) {
	ctx := context.Background()
	m, _, _ := newTestManager(t)

	privateKeyPEM, _ := rsaPrivateKeyPEM(t)
	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/inbox",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://example.com/users/alice#main-key",
			Owner:        "https://example.com/users/alice",
			PublicKeyPem: "pem",
		},
	}

	m.federationStore = fakeFederationStore{
		privateKeyFn: func(_ context.Context, username string) (string, error) {
			if username == "alice" {
				return privateKeyPEM, nil
			}
			return "", errors.New("unknown user")
		},
	}

	t.Run("marshal error", func(t *testing.T) {
		result := &fedTypes.DeliveryResult{}
		err := m.performHTTPDelivery(ctx, &activitypub.Activity{Object: make(chan int)}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMarshalActivityFailed)
	})

	t.Run("request creation error", func(t *testing.T) {
		result := &fedTypes.DeliveryResult{}
		err := m.performHTTPDelivery(ctx, &activitypub.Activity{}, "://bad-url", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateRequestFailed)
	})

	t.Run("missing federation store for signing", func(t *testing.T) {
		m2, _, _ := newTestManager(t)
		m2.httpClient = fakeHTTPDoer{doFn: func(*http.Request) (*http.Response, error) { return nil, nil }}
		result := &fedTypes.DeliveryResult{}
		err := m2.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationStoreNotConfiguredForSigning)
	})

	t.Run("private key retrieval error", func(t *testing.T) {
		m3, _, _ := newTestManager(t)
		m3.federationStore = fakeFederationStore{
			privateKeyFn: func(context.Context, string) (string, error) {
				return "", errors.New("no key")
			},
		}
		m3.httpClient = fakeHTTPDoer{doFn: func(*http.Request) (*http.Response, error) { return nil, nil }}

		result := &fedTypes.DeliveryResult{}
		err := m3.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGetPrivateKeyFailed)
	})

	t.Run("private key parse error", func(t *testing.T) {
		m4, _, _ := newTestManager(t)
		m4.federationStore = fakeFederationStore{
			privateKeyFn: func(context.Context, string) (string, error) {
				return "not a pem", nil
			},
		}
		m4.httpClient = fakeHTTPDoer{doFn: func(*http.Request) (*http.Response, error) { return nil, nil }}

		result := &fedTypes.DeliveryResult{}
		err := m4.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrParsePrivateKeyFailed)
	})

	t.Run("send request error", func(t *testing.T) {
		m5, _, _ := newTestManager(t)
		m5.federationStore = m.federationStore
		m5.httpClient = fakeHTTPDoer{doFn: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}}

		result := &fedTypes.DeliveryResult{}
		err := m5.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSendRequestFailed)
	})

	t.Run("2xx success", func(t *testing.T) {
		m6, _, _ := newTestManager(t)
		m6.federationStore = m.federationStore
		m6.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}}

		result := &fedTypes.DeliveryResult{}
		err := m6.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, result.StatusCode)
	})

	t.Run("non-2xx failure", func(t *testing.T) {
		m7, _, _ := newTestManager(t)
		m7.federationStore = m.federationStore
		m7.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad")),
				Request:    req,
			}, nil
		}}

		result := &fedTypes.DeliveryResult{}
		err := m7.performHTTPDelivery(ctx, &activitypub.Activity{}, "https://remote.example/inbox", signingActor, result)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrHTTPDeliveryFailed)
		assert.Equal(t, http.StatusBadRequest, result.StatusCode)
	})
}

func TestManager_deliverToRoute_RetriesAndCancellation(t *testing.T) {
	baseCtx := context.Background()
	m, repo, _ := newTestManager(t)

	privateKeyPEM, _ := rsaPrivateKeyPEM(t)
	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/inbox",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://example.com/users/alice#main-key",
			Owner:        "https://example.com/users/alice",
			PublicKeyPem: "pem",
		},
	}
	m.federationStore = fakeFederationStore{
		actorFn: func(context.Context, string) (*activitypub.Actor, error) { return signingActor, nil },
		privateKeyFn: func(context.Context, string) (string, error) {
			return privateKeyPEM, nil
		},
	}

	inst := createTestInstance("inst1", "one.example")
	inst.SharedInboxURL = "https://remote.example/inbox"
	inst.InboxURL = "https://remote.example/inbox"
	inst.Status = fedTypes.InstanceStatusActive
	inst.TierLevel = fedTypes.TierStandard
	inst.MonthlyQuota = 1000
	inst.CurrentUsage = 0
	repo.AddInstance(inst)

	route := &fedTypes.Route{
		ID:          "inst1-primary",
		InstanceID:  inst.ID,
		Domain:      inst.Domain,
		Priority:    1,
		CostPerByte: 0.5,
	}

	payload, err := jsonMarshal(t, &activitypub.Activity{Actor: "https://example.com/users/alice", Object: map[string]any{"type": "Note"}})
	require.NoError(t, err)
	msg := &fedTypes.FederationMessage{
		ID:        "m1",
		Type:      fedTypes.MessageTypeCreate,
		Actor:     "https://example.com/users/alice",
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	t.Run("retry then succeed", func(t *testing.T) {
		call := 0
		m.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("boom")),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}}

		res := m.deliverToRoute(baseCtx, route, msg, []string{"one"}, fedTypes.DeliveryOptions{
			MaxRetries:   2,
			RetryBackoff: 1 * time.Nanosecond,
		})
		require.True(t, res.Success)
		assert.Equal(t, 2, res.Attempts)
		assert.Equal(t, int64(len(payload)), res.BytesSent)
		assert.Greater(t, res.Cost, float64(0))
	})

	t.Run("retryable failure cancels before retry backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(baseCtx)
		defer cancel()

		m.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
			cancel()
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Request:    req,
			}, nil
		}}

		res := m.deliverToRoute(ctx, route, msg, []string{"one"}, fedTypes.DeliveryOptions{
			MaxRetries:   2,
			RetryBackoff: 50 * time.Millisecond,
		})
		require.False(t, res.Success)
		assert.Equal(t, "delivery cancelled", res.ErrorMessage)
	})

	t.Run("non-retryable failure stops immediately", func(t *testing.T) {
		m.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad")),
				Request:    req,
			}, nil
		}}

		res := m.deliverToRoute(baseCtx, route, msg, []string{"one"}, fedTypes.DeliveryOptions{
			MaxRetries: 3,
		})
		require.False(t, res.Success)
		assert.Equal(t, 1, res.Attempts)
		assert.Contains(t, res.ErrorMessage, ErrHTTPDeliveryFailed.Error())
	})
}

func TestManager_DeliverMessage_AggregatesAndTracks(t *testing.T) {
	ctx := context.Background()
	m, repo, cb := newTestManager(t)

	privateKeyPEM, _ := rsaPrivateKeyPEM(t)
	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/inbox",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://example.com/users/alice#main-key",
			Owner:        "https://example.com/users/alice",
			PublicKeyPem: "pem",
		},
	}
	m.federationStore = fakeFederationStore{
		actorFn: func(context.Context, string) (*activitypub.Actor, error) { return signingActor, nil },
		privateKeyFn: func(context.Context, string) (string, error) {
			return privateKeyPEM, nil
		},
	}

	// Instances keyed by their domain (getInstancesForDomain uses registry.GetInstance(domain)).
	for _, domain := range []string{"a.example", "b.example"} {
		inst := createTestInstance(domain, domain)
		inst.SupportedTypes = nil // empty means all
		inst.MonthlyQuota = 1000
		inst.CurrentUsage = 0
		inst.SharedInboxURL = "https://" + domain + "/inbox"
		inst.InboxURL = "https://" + domain + "/inbox"
		repo.AddInstance(inst)
	}

	m.httpClient = fakeHTTPDoer{doFn: func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "a.example"):
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad")),
				Request:    req,
			}, nil
		}
	}}

	recorder := &fakeCostRecorder{}
	m.costTrackingRepo = recorder

	payload, err := jsonMarshal(t, &activitypub.Activity{Actor: signingActor.ID, Object: map[string]any{"type": "Note"}})
	require.NoError(t, err)

	msg := &fedTypes.FederationMessage{
		ID:        "m1",
		Type:      fedTypes.MessageTypeCreate,
		Actor:     signingActor.ID,
		Target:    []string{"a.example", "b.example"},
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	res, err := m.DeliverMessage(ctx, msg, fedTypes.DeliveryOptions{MaxRetries: 1})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success) // one failure
	assert.GreaterOrEqual(t, res.Attempts, 2)
	assert.GreaterOrEqual(t, len(recorder.records), 1)

	// Circuit breaker should see both a success and a failure.
	cb.mu.Lock()
	defer cb.mu.Unlock()
	assert.NotEmpty(t, cb.successCalls)
	assert.NotEmpty(t, cb.failureCalls)
}

func TestManager_recordDetailedCostTracking_and_HealthSummaries(t *testing.T) {
	ctx := context.Background()
	m, _, cb := newTestManager(t)

	recorder := &fakeCostRecorder{}
	m.costTrackingRepo = recorder

	msg := &fedTypes.FederationMessage{
		ID:        "m1",
		Type:      fedTypes.MessageTypeCreate,
		CreatedAt: time.Now(),
		Payload:   []byte("payload"),
	}

	route := &fedTypes.Route{
		ID:          "r1",
		InstanceID:  "i1",
		Domain:      "", // force endpoint fallback
		Endpoint:    mustParseURL(t, "https://example.com/inbox"),
		CostPerByte: 0.0000001,
	}

	result := &fedTypes.DeliveryResult{
		MessageID:    "m1",
		InstanceID:   "i1",
		RouteID:      "r1",
		Attempts:     3,
		BytesSent:    1024,
		Duration:     250 * time.Millisecond,
		Success:      false,
		ErrorMessage: "failed",
		Timestamp:    time.Now(),
	}

	err := m.recordDetailedCostTracking(ctx, msg, route, result)
	require.NoError(t, err)
	require.NotEmpty(t, recorder.records)

	got := recorder.records[len(recorder.records)-1]
	assert.Equal(t, msg.ID, got.ActivityID)
	assert.Equal(t, "example.com", got.Domain)
	assert.Equal(t, route.ID, got.RouteID)
	assert.Equal(t, int64(result.Attempts), got.HTTPRequestCount)
	assert.NotEmpty(t, got.RetryDelaySeconds)
	assert.NotEmpty(t, got.RetryErrorMessages)

	// Health summary with a fake health repo and circuit breaker statuses.
	m.healthRepo = fakeHealthRepo{
		getLatestFn: func(_ context.Context, domain string) (*models.InstanceHealth, error) {
			return &models.InstanceHealth{
				Domain:       domain,
				Timestamp:    time.Now(),
				Reachable:    true,
				StatusCode:   200,
				ResponseTime: 123 * time.Millisecond,
				ErrorRate:    0.0,
			}, nil
		},
		getUnhealthyFn: func(context.Context, float64) ([]string, error) {
			return []string{"i1", "i2"}, nil
		},
	}

	cb.getStatusFn = func(id string) fedTypes.CircuitStatus {
		if id == "i2" {
			return fedTypes.CircuitOpen
		}
		return fedTypes.CircuitClosed
	}

	// Seed the registry with instances and list them as healthy.
	repo := NewFakeFederationInstanceRepository()
	repo.AddInstance(createTestInstance("i1", "one.example"))
	repo.AddInstance(createTestInstance("i2", "two.example"))
	m.registry = NewInstanceRegistry(repo, m.logger)
	repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) {
		return []*fedTypes.Instance{
			createTestInstance("i1", "one.example"),
			createTestInstance("i2", "two.example"),
		}, nil
	}

	unhealthy, err := m.DetectUnhealthyInstances()
	require.NoError(t, err)
	assert.Equal(t, []string{"i2"}, unhealthy)

	summary, err := m.GetHealthSummary()
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, 2, summary.TotalInstances)
	assert.NotEmpty(t, summary.InstanceDetails)
}

func TestManager_InstanceLifecycle_HealthAndCircuitMethods(t *testing.T) {
	ctx := context.Background()
	m, repo, cb := newTestManager(t)

	instance := createTestInstance("inst1", "example.com")
	instance.MonthlyQuota = 1000
	instance.CurrentUsage = 0

	// Pre-seed route cache so we can validate invalidation.
	m.cacheRoutes(instance.Domain, []*fedTypes.Route{{ID: "r1", InstanceID: instance.ID, Domain: instance.Domain, Priority: 1}})

	t.Run("RegisterInstance clears cache and tolerates monitoring/circuit errors", func(t *testing.T) {
		m.healthChecker = fakeHealthChecker{
			startFn: func(*fedTypes.Instance) error { return errors.New("monitoring unavailable") },
		}
		cb.closeFn = func(string) error { return errors.New("close failed") }

		err := m.RegisterInstance(instance)
		require.NoError(t, err)

		_, ok := m.routeCache.Load(fmt.Sprintf("routes:%s", instance.Domain))
		assert.False(t, ok)
	})

	t.Run("RegisterInstance returns error when registry fails", func(t *testing.T) {
		repo.CreateInstanceFunc = func(context.Context, *fedTypes.Instance) error {
			return errors.New("create failed")
		}
		err := m.RegisterInstance(createTestInstance("", "bad.example"))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRegisterInstanceFailed)
		repo.CreateInstanceFunc = nil
	})

	t.Run("UpdateInstanceHealth records failure and clears cache", func(t *testing.T) {
		repo.AddInstance(instance)

		// Re-seed cache.
		m.cacheRoutes(instance.Domain, []*fedTypes.Route{{ID: "r1", InstanceID: instance.ID, Domain: instance.Domain, Priority: 1}})

		cb.recordFailureFn = func(string, error) error { return errors.New("record failure failed") }
		err := m.UpdateInstanceHealth(instance.ID, &fedTypes.HealthStatus{Reachable: false, ErrorRate: 1.0})
		require.NoError(t, err)

		_, ok := m.routeCache.Load(fmt.Sprintf("routes:%s", instance.Domain))
		assert.False(t, ok)
	})

	t.Run("UpdateInstanceHealth records success", func(t *testing.T) {
		cb.recordSuccessFn = func(string) error { return errors.New("record success failed") }
		err := m.UpdateInstanceHealth(instance.ID, &fedTypes.HealthStatus{Reachable: true, ErrorRate: 0.0, StatusCode: 200})
		require.NoError(t, err)
	})

	t.Run("UpdateInstanceHealth returns error when registry fails", func(t *testing.T) {
		repo.UpdateInstanceHealthFunc = func(context.Context, string, *fedTypes.HealthStatus) error {
			return errors.New("update failed")
		}
		err := m.UpdateInstanceHealth(instance.ID, &fedTypes.HealthStatus{Reachable: true})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUpdateHealthFailed)
		repo.UpdateInstanceHealthFunc = nil
	})

	t.Run("OpenCircuit / CloseCircuit clear cache and report status", func(t *testing.T) {
		repo.AddInstance(instance)
		m.cacheRoutes(instance.Domain, []*fedTypes.Route{{ID: "r1", InstanceID: instance.ID, Domain: instance.Domain, Priority: 1}})

		cb.openFn = func(string, string) error { return nil }
		cb.closeFn = func(string) error { return nil }
		cb.getStatusFn = func(string) fedTypes.CircuitStatus { return fedTypes.CircuitOpen }

		require.NoError(t, m.OpenCircuit(instance.ID, "test"))
		require.NoError(t, m.CloseCircuit(instance.ID))
		assert.Equal(t, fedTypes.CircuitOpen, m.GetCircuitStatus(instance.ID))
	})

	_ = ctx
}

func TestManager_OptimizationAndMetricsMethods(t *testing.T) {
	ctx := context.Background()
	m, repo, _ := newTestManager(t)

	// Create two cached routes for a domain, so OptimizeRoutes mutates priorities.
	inst := createTestInstance("opt.example", "opt.example")
	repo.AddInstance(inst)

	r1 := &fedTypes.Route{ID: "r1", InstanceID: inst.ID, Domain: inst.Domain, Priority: 10}
	r2 := &fedTypes.Route{ID: "r2", InstanceID: inst.ID, Domain: inst.Domain, Priority: 20}
	m.cacheRoutes(inst.Domain, []*fedTypes.Route{r1, r2})

	repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) {
		return []*fedTypes.Instance{inst}, nil
	}

	m.optimizer = fakeRouteOptimizer{
		optimizeFn: func(_ context.Context, routes []*fedTypes.Route, _ int64) ([]*fedTypes.Route, error) {
			return []*fedTypes.Route{routes[1], routes[0]}, nil
		},
	}

	require.NoError(t, m.OptimizeRoutes())
	assert.Equal(t, 1, r2.Priority)
	assert.Equal(t, 2, r1.Priority)

	// GetRouteMetrics aggregation.
	m.cacheRoutes("metrics.example", []*fedTypes.Route{
		{ID: "m1", InstanceID: inst.ID, Domain: inst.Domain, Priority: 1},
		{ID: "m2", InstanceID: inst.ID, Domain: inst.Domain, Priority: 2},
	})

	m.optimizer = fakeRouteOptimizer{
		metricsFn: func(_ context.Context, routeID string) (*fedTypes.RouteMetrics, error) {
			switch routeID {
			case "m1":
				return &fedTypes.RouteMetrics{
					TotalMessages:   10,
					SuccessfulCount: 10,
					FailedCount:     0,
					TotalBytes:      100,
					TotalCost:       1.0,
					P95Latency:      2 * time.Second,
					P99Latency:      4 * time.Second,
					LastUpdated:     time.Now(),
				}, nil
			case "m2":
				return &fedTypes.RouteMetrics{
					TotalMessages:   20,
					SuccessfulCount: 18,
					FailedCount:     2,
					TotalBytes:      200,
					TotalCost:       2.0,
					P95Latency:      3 * time.Second,
					P99Latency:      5 * time.Second,
					LastUpdated:     time.Now(),
				}, nil
			default:
				return nil, errors.New("unknown route")
			}
		},
	}

	metrics, err := m.GetRouteMetrics("metrics.example")
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.Equal(t, int64(30), metrics.TotalMessages)
	assert.Equal(t, int64(28), metrics.SuccessfulCount)
	assert.Equal(t, 3*time.Second, metrics.P95Latency)
	assert.Equal(t, 5*time.Second, metrics.P99Latency)
	assert.Equal(t, 1500*time.Millisecond, metrics.AvgLatency)

	// No routes case.
	_, err = m.GetRouteMetrics("missing.example")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoRoutesForDestination)

	// ListHealthyInstances error path for OptimizeRoutes.
	repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) {
		return nil, errors.New("list failed")
	}
	err = m.OptimizeRoutes()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListInstancesFailed)

	_ = ctx
}

func TestManager_HealthMonitoring_Methods(t *testing.T) {
	ctx := context.Background()
	m, repo, cb := newTestManager(t)

	inst := createTestInstance("h1", "health.example")
	repo.AddInstance(inst)

	t.Run("PerformHealthCheck instance missing", func(t *testing.T) {
		_, err := m.PerformHealthCheck("missing")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGetInstanceFailed)
	})

	t.Run("PerformHealthCheck short-circuits when circuit open", func(t *testing.T) {
		cb.canAttemptFn = func(string) bool { return false }
		health, err := m.PerformHealthCheck(inst.ID)
		require.NoError(t, err)
		require.NotNil(t, health)
		assert.False(t, health.Reachable)
	})

	t.Run("PerformHealthCheck propagates checker error and records failure", func(t *testing.T) {
		cb.canAttemptFn = func(string) bool { return true }
		m.healthChecker = fakeHealthChecker{
			checkFn: func(*fedTypes.Instance) (*fedTypes.HealthStatus, error) {
				return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: false, StatusCode: 0}, errors.New("check failed")
			},
		}
		_, err := m.PerformHealthCheck(inst.ID)
		require.Error(t, err)
	})

	t.Run("PerformHealthCheck records success and clears cache", func(t *testing.T) {
		m.healthChecker = fakeHealthChecker{
			checkFn: func(*fedTypes.Instance) (*fedTypes.HealthStatus, error) {
				return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, StatusCode: 200}, nil
			},
		}
		cb.recordSuccessFn = func(string) error { return errors.New("record success failed") }
		m.cacheRoutes(inst.Domain, []*fedTypes.Route{{ID: "r1", InstanceID: inst.ID, Domain: inst.Domain, Priority: 1}})

		health, err := m.PerformHealthCheck(inst.ID)
		require.NoError(t, err)
		require.NotNil(t, health)

		_, ok := m.routeCache.Load(fmt.Sprintf("routes:%s", inst.Domain))
		assert.False(t, ok)
	})

	t.Run("PerformHealthCheck records failure when unhealthy", func(t *testing.T) {
		m.healthChecker = fakeHealthChecker{
			checkFn: func(*fedTypes.Instance) (*fedTypes.HealthStatus, error) {
				return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, StatusCode: 503}, nil
			},
		}
		cb.recordFailureFn = func(string, error) error { return errors.New("record failure failed") }
		health, err := m.PerformHealthCheck(inst.ID)
		require.NoError(t, err)
		require.NotNil(t, health)
		assert.False(t, health.StatusCode < 500)
	})

	t.Run("MonitorInstanceHealth handles empty list", func(t *testing.T) {
		repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) { return []*fedTypes.Instance{}, nil }
		require.NoError(t, m.MonitorInstanceHealth())
	})

	t.Run("MonitorInstanceHealth runs checks and updates", func(t *testing.T) {
		repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) { return []*fedTypes.Instance{inst}, nil }
		repo.UpdateInstanceHealthFunc = func(context.Context, string, *fedTypes.HealthStatus) error { return nil }
		m.healthChecker = fakeHealthChecker{
			checkFn: func(*fedTypes.Instance) (*fedTypes.HealthStatus, error) {
				return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, StatusCode: 200}, nil
			},
		}
		require.NoError(t, m.MonitorInstanceHealth())
	})

	t.Run("MonitorInstanceHealth list error", func(t *testing.T) {
		repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) { return nil, errors.New("list failed") }
		err := m.MonitorInstanceHealth()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrListInstancesFailed)
	})

	t.Run("RecoverInstances probes half-open circuits", func(t *testing.T) {
		repo.ListHealthyInstancesFunc = func(context.Context) ([]*fedTypes.Instance, error) { return []*fedTypes.Instance{inst}, nil }
		cb.getStatusFn = func(string) fedTypes.CircuitStatus { return fedTypes.CircuitHalfOpen }
		m.healthChecker = fakeHealthChecker{
			checkFn: func(*fedTypes.Instance) (*fedTypes.HealthStatus, error) {
				return &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, StatusCode: 200}, nil
			},
		}
		require.NoError(t, m.RecoverInstances())
	})

	_ = ctx
}

func TestManager_RouteConstructionAndCachingHelpers(t *testing.T) {
	ctx := context.Background()
	m, _, cb := newTestManager(t)

	// createRouteFromInstance error path.
	_, err := m.createRouteFromInstance(&fedTypes.Instance{
		ID:             "i",
		Domain:         "d",
		SharedInboxURL: "://bad",
		InboxURL:       "://bad",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidInboxURLs)

	// enhanceRouteWithMetrics populates route metrics and surfaces circuit assessment failures.
	m.optimizer = fakeRouteOptimizer{
		metricsFn: func(_ context.Context, _ string) (*fedTypes.RouteMetrics, error) {
			return &fedTypes.RouteMetrics{
				TotalMessages:   10,
				SuccessfulCount: 9,
				FailedCount:     1,
				TotalBytes:      100,
				TotalCost:       0.5,
				AvgLatency:      100 * time.Millisecond,
				P95Latency:      200 * time.Millisecond,
				LastUpdated:     time.Now(),
			}, nil
		},
	}
	cb.assessFn = func(context.Context, string, *fedTypes.RouteMetrics) error { return errors.New("assessment failed") }

	route := &fedTypes.Route{ID: "route-1", InstanceID: "i", Domain: "d"}
	m.enhanceRouteWithMetrics(route)
	assert.Equal(t, 100*time.Millisecond, route.Latency)
	assert.Greater(t, route.SuccessRate, 0.0)

	// populateHealthStatus fills status entries when metrics are available.
	m.thresholdManager.config.HealthyRouteTTL = 1 * time.Second
	m.thresholdManager.config.DegradedRouteTTL = 2 * time.Second
	m.thresholdManager.config.UnknownRouteTTL = 3 * time.Second
	m.cacheRoutes("cache.example", []*fedTypes.Route{{ID: "route-1", InstanceID: "i", Domain: "d"}})

	raw, ok := m.routeCache.Load("routes:cache.example")
	require.True(t, ok)
	cr := raw.(*cachedRoutes)
	assert.Contains(t, cr.healthStatus, "route-1")

	// getAdaptiveCacheTTL returns different TTLs based on assessed status.
	m.optimizer = fakeRouteOptimizer{
		metricsFn: func(_ context.Context, routeID string) (*fedTypes.RouteMetrics, error) {
			switch routeID {
			case "p":
				return &fedTypes.RouteMetrics{TotalMessages: 100, SuccessfulCount: 97, FailedCount: 3, P95Latency: 1 * time.Second, LastUpdated: time.Now()}, nil
			case "d":
				return &fedTypes.RouteMetrics{TotalMessages: 100, SuccessfulCount: 65, FailedCount: 35, P95Latency: 4 * time.Second, LastUpdated: time.Now()}, nil
			default:
				return &fedTypes.RouteMetrics{TotalMessages: 100, SuccessfulCount: 85, FailedCount: 15, P95Latency: 3 * time.Second, LastUpdated: time.Now()}, nil
			}
		},
	}
	m.thresholdManager.config.HealthyRouteTTL = 11 * time.Second
	m.thresholdManager.config.DegradedRouteTTL = 22 * time.Second
	m.thresholdManager.config.UnknownRouteTTL = 33 * time.Second

	assert.Equal(t, 11*time.Second, m.getAdaptiveCacheTTL([]*fedTypes.Route{{ID: "p"}}))
	assert.Equal(t, 22*time.Second, m.getAdaptiveCacheTTL([]*fedTypes.Route{{ID: "d"}}))
	assert.Equal(t, 33*time.Second, m.getAdaptiveCacheTTL([]*fedTypes.Route{{ID: "m"}}))

	_ = ctx
}

func TestManager_NoopsAndRemainingBranches(t *testing.T) {
	ctx := context.Background()

	// Cover no-op implementations defined in route_manager.go.
	hc := noopHealthChecker{}
	health, err := hc.CheckHealth(nil)
	require.NoError(t, err)
	require.NotNil(t, health)
	assert.False(t, health.Reachable)
	assert.NotEmpty(t, health.ErrorMessage)
	require.NoError(t, hc.StartMonitoring(nil))
	require.NoError(t, hc.StopMonitoring("x"))
	history, err := hc.GetHealthHistory("x", 5*time.Minute)
	require.NoError(t, err)
	assert.Empty(t, history)

	ncb := noopCircuitBreaker{}
	assert.True(t, ncb.CanAttempt("x"))
	assert.Equal(t, fedTypes.CircuitClosed, ncb.GetStatus("x"))
	assert.NoError(t, ncb.Open("x", "reason"))
	assert.NoError(t, ncb.Close("x"))
	assert.NoError(t, ncb.RecordSuccess("x"))
	assert.NoError(t, ncb.RecordFailure("x", errors.New("fail")))
	assert.False(t, ncb.ShouldEnterEmergencyMode(0, 10))
	assert.Empty(t, ncb.GetBackpressureRules())
	assert.NoError(t, ncb.AssessRouteHealthAndAdjustCircuit(ctx, "route", &fedTypes.RouteMetrics{}))

	// Cover shouldEnterEmergencyMode nil-circuit branch explicitly.
	m, _, _ := newTestManager(t)
	m.circuitBreaker = nil
	assert.False(t, m.shouldEnterEmergencyMode(0, 0))

	// Cover getSigningActor error from storage.
	m.federationStore = fakeFederationStore{
		actorFn: func(context.Context, string) (*activitypub.Actor, error) {
			return nil, errors.New("boom")
		},
	}
	_, err = m.getSigningActor(ctx, "https://example.com/users/alice")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetActorFailed)
}

func TestManager_deliverToRoute_EarlyExitBranches(t *testing.T) {
	ctx := context.Background()
	m, repo, _ := newTestManager(t)

	route := &fedTypes.Route{ID: "r1", InstanceID: "missing", Domain: "d", CostPerByte: 0.01}
	msg := &fedTypes.FederationMessage{
		ID:        "m1",
		Type:      fedTypes.MessageTypeCreate,
		Actor:     "https://example.com/users/alice",
		Payload:   []byte("not json"),
		CreatedAt: time.Now(),
	}

	t.Run("instance lookup failure", func(t *testing.T) {
		repo.GetInstanceFunc = func(context.Context, string) (*fedTypes.Instance, error) {
			return nil, errors.New("not found")
		}
		res := m.deliverToRoute(ctx, route, msg, []string{"t"}, fedTypes.DeliveryOptions{MaxRetries: 1})
		require.False(t, res.Success)
		assert.Contains(t, res.ErrorMessage, "failed to get instance")
		repo.GetInstanceFunc = nil
	})

	t.Run("missing inbox URLs", func(t *testing.T) {
		inst := &fedTypes.Instance{ID: "inst1", Domain: "d", Status: fedTypes.InstanceStatusActive}
		repo.AddInstance(inst)
		route.InstanceID = inst.ID

		res := m.deliverToRoute(ctx, route, msg, []string{"t"}, fedTypes.DeliveryOptions{MaxRetries: 1})
		require.False(t, res.Success)
		assert.Equal(t, "no inbox URL available for instance", res.ErrorMessage)
	})

	t.Run("prepare activity error", func(t *testing.T) {
		inst := &fedTypes.Instance{
			ID:             "inst2",
			Domain:         "d2",
			Status:         fedTypes.InstanceStatusActive,
			SharedInboxURL: "https://d2/inbox",
			InboxURL:       "https://d2/inbox",
		}
		repo.AddInstance(inst)
		route.InstanceID = inst.ID

		m.federationStore = nil // forces getSigningActor failure
		res := m.deliverToRoute(ctx, route, msg, []string{"t"}, fedTypes.DeliveryOptions{MaxRetries: 1})
		require.False(t, res.Success)
		assert.Contains(t, res.ErrorMessage, "failed to prepare activity")
	})
}

func TestManager_selectRouteInEmergencyMode_BackpressureActions(t *testing.T) {
	ctx := context.Background()
	m, _, cb := newTestManager(t)

	routes := []*fedTypes.Route{
		{ID: "r1", InstanceID: "i1"},
		{ID: "r2", InstanceID: "i2"},
		{ID: "r3", InstanceID: "i3"},
		{ID: "r4", InstanceID: "i4"},
	}
	m.cacheRoutes("emergency.example", routes)

	// One healthy circuit out of four => 0.25.
	cb.getStatusFn = func(id string) fedTypes.CircuitStatus {
		if id == "i1" {
			return fedTypes.CircuitClosed
		}
		return fedTypes.CircuitOpen
	}

	t.Run("queue_if_below_threshold", func(t *testing.T) {
		cb.backpressureRules = map[MessagePriority]BackpressureRule{
			PriorityNormal: {Action: "queue_if_below_threshold", Threshold: 0.5},
		}
		_, err := m.selectRouteInEmergencyMode("emergency.example", fedTypes.MessageTypeCreate)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMessageQueuedBackpressure)
	})

	t.Run("queue", func(t *testing.T) {
		cb.backpressureRules = map[MessagePriority]BackpressureRule{
			PriorityNormal: {Action: "queue", Threshold: 0.0},
		}
		_, err := m.selectRouteInEmergencyMode("emergency.example", fedTypes.MessageTypeCreate)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMessageQueuedEmergency)
	})

	t.Run("no routes returns ErrNoHealthyRoutes", func(t *testing.T) {
		_, err := m.selectRouteInEmergencyMode("missing.example", fedTypes.MessageTypeCreate)
		require.Error(t, err)
		assert.ErrorIs(t, err, fedTypes.ErrNoHealthyRoutes)
	})

	_ = ctx
}

func jsonMarshal(t *testing.T, v any) ([]byte, error) {
	t.Helper()
	return json.Marshal(v)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
