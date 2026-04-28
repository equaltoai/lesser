package websocket

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSubscriptionManager_HandleConnectAndDisconnect_Errors(t *testing.T) {
	repo := &stubSubscriptionRepo{handleConnectErr: errors.New("boom"), handleDisconnectErr: errors.New("boom")}
	client := &stubStreamerClient{}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         client,
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.HandleConnect("c1", "u1")
	require.Error(t, err)

	// Even when the repository delete fails, HandleDisconnect should not fail.
	sm.connections["c1"] = &Connection{ConnectionID: "c1"}
	require.NoError(t, sm.HandleDisconnect("c1"))
	_, ok := sm.connections["c1"]
	assert.False(t, ok)
}

func TestSubscriptionManager_HandleConnectWithPrincipalPersistsRole(t *testing.T) {
	repo := &stubSubscriptionRepo{}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.HandleConnectWithPrincipal("c1", webSocketPrincipal{
		UserID:        "mod",
		Role:          "moderator",
		Authenticated: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "moderator", repo.connectRole)
	require.Contains(t, sm.connections, "c1")
	assert.Equal(t, "moderator", sm.connections["c1"].Role)
}

func TestSubscriptionManager_createSubscription_ErrorBranches(t *testing.T) {
	repo := &stubSubscriptionRepo{createErr: errors.New("boom")}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{"c1": {ConnectionID: "c1"}},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.createSubscription("c1", "moderation", func() {})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to convert filter")

	err = sm.createSubscription("c1", "moderation", map[string]any{"severity": "high"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to store subscription")
}

func TestSubscriptionManager_ConnectionAndFilterBranches(t *testing.T) {
	smNoRepo := &subscriptionManager{}
	_, err := smNoRepo.ConnectionPrincipal("c1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection repository is not configured")

	smEmptyConnection := &subscriptionManager{
		repo: &stubSubscriptionRepo{
			connectionsByID: map[string]storageModels.WebSocketEventConnection{
				"empty": {},
			},
		},
	}
	_, err = smEmptyConnection.ConnectionPrincipal("empty")
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection not found")

	now := time.Now()
	sm := &subscriptionManager{
		connections: map[string]*Connection{},
	}
	dbConn := &storageModels.WebSocketEventConnection{
		ConnectionID: "c1",
		UserID:       "anonymous",
		ConnectedAt:  now,
		LastSeen:     now,
		TTL:          now.Add(time.Hour).Unix(),
	}
	sm.cacheConnection(dbConn)
	sm.cacheConnection(dbConn)
	require.Contains(t, sm.connections, "c1")

	event := &ModerationEvent{
		Severity:  "high",
		Type:      "spam",
		UserID:    "u1",
		ContentID: "note1",
	}
	require.True(t, sm.matchesModerationFilter(event, nil))
	require.False(t, sm.matchesModerationFilter(event, map[string]any{"severity": []any{"low"}}))
	require.False(t, sm.matchesModerationFilter(event, map[string]any{"types": []any{"abuse"}}))
	require.False(t, sm.matchesModerationFilter(event, map[string]any{"user_id": "u2"}))
	require.False(t, sm.matchesModerationFilter(event, map[string]any{"content_id": "note2"}))

	alert := &PerformanceAlert{Severity: "critical"}
	require.True(t, sm.matchesPerformanceFilter(alert, nil))
	require.False(t, sm.matchesPerformanceFilter(alert, map[string]any{"severity": "warning"}))
}

func TestNewSubscriptionManagerRejectsEmptyEndpoint(t *testing.T) {
	manager, err := NewSubscriptionManager(nil, "", zap.NewNop())
	require.Error(t, err)
	require.Nil(t, manager)
}

func TestNewSubscriptionManagerInitializesState(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	manager, err := NewSubscriptionManager(nil, "wss://example.com/dev", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, manager)

	sm, ok := manager.(*subscriptionManager)
	require.True(t, ok)
	require.NotNil(t, sm.apiGW)
	require.NotNil(t, sm.connections)
	require.NotNil(t, sm.subscriptions)
	assert.Equal(t, "wss://example.com/dev", sm.endpoint)
}

func TestSubscriptionManager_Unsubscribe_RepoError(t *testing.T) {
	repo := &stubSubscriptionRepo{deleteErr: errors.New("boom")}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{"c1": {ConnectionID: "c1"}},
		subscriptions: map[string]map[string]*Subscription{"c1": {"moderation": {ConnectionID: "c1", SubscriptionType: "moderation"}}},
		logger:        zap.NewNop(),
	}

	err := sm.Unsubscribe("c1", "moderation")
	require.Error(t, err)
}

func TestSubscriptionManager_publishToSubscribers_GetSubscriptionsError(t *testing.T) {
	repo := &stubSubscriptionRepo{getErr: errors.New("boom")}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.PublishThreatAlert(&ThreatAlert{ID: "t1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get subscriptions")
}

type stubSubscriptionManagerWithDisconnect struct {
	stubSubscriptionManager
	disconnectErr error
}

func (s *stubSubscriptionManagerWithDisconnect) HandleDisconnect(_ string) error {
	return s.disconnectErr
}

func TestWebSocketHandler_MoreRoutesAndParsing(t *testing.T) {
	sm := &stubSubscriptionManagerWithDisconnect{
		disconnectErr: errors.New("boom"),
	}
	h := NewWebSocketHandler(sm, zap.NewNop())

	t.Run("unknown route", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "unknown"},
		})
		require.Error(t, err)
		require.Equal(t, 404, resp.StatusCode)
	})

	t.Run("connect uses anonymous when missing user_id", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext:        events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "$connect"},
			QueryStringParameters: map[string]string{},
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		require.Equal(t, "anonymous", sm.connectUserID)
	})

	t.Run("connect error returns 500", func(t *testing.T) {
		sm.connectErr = errors.New("boom")
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext:        events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "$connect"},
			QueryStringParameters: map[string]string{"user_id": "u1"},
		})
		require.Error(t, err)
		require.Equal(t, 500, resp.StatusCode)
	})

	t.Run("disconnect logs error but returns 200", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "$disconnect"},
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
	})

	t.Run("subscribe invalid json returns 400", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
			Body:           "{not-json",
		})
		require.Error(t, err)
		require.Equal(t, 400, resp.StatusCode)
	})

	t.Run("subscribe unknown type returns error", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
			Body:           `{"type":"unknown"}`,
		})
		require.Error(t, err)
		require.Equal(t, 500, resp.StatusCode)
	})

	t.Run("subscribe performance uses default severity when missing", func(t *testing.T) {
		h.rememberConnectionPrincipal("c1", webSocketPrincipal{UserID: "admin", Role: "admin", Authenticated: true})
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
			Body:           `{"type":"performance","filter":{}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		require.Equal(t, "medium", sm.performanceSeverity)
	})

	t.Run("moderation filter unmarshal failure falls back to empty filter", func(t *testing.T) {
		h.rememberConnectionPrincipal("c1", webSocketPrincipal{UserID: "admin", Role: "admin", Authenticated: true})
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
			Body:           `{"type":"moderation","filter":{"severity":123}}`,
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		require.Empty(t, sm.subscribeModerationFilter.Severity)
	})

	t.Run("unsubscribe invalid json returns 400", func(t *testing.T) {
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "unsubscribe"},
			Body:           "{not-json",
		})
		require.Error(t, err)
		require.Equal(t, 400, resp.StatusCode)
	})

	t.Run("unsubscribe error returns 500", func(t *testing.T) {
		sm.unsubscribeErr = errors.New("boom")
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "unsubscribe"},
			Body:           `{"type":"moderation"}`,
		})
		require.Error(t, err)
		require.Equal(t, 500, resp.StatusCode)
	})

	t.Run("unsubscribe success returns 200", func(t *testing.T) {
		sm.unsubscribeErr = nil
		resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "unsubscribe"},
			Body:           `{"type":"moderation"}`,
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
	})
}

func TestWebSocketHandler_PrincipalHelpers(t *testing.T) {
	h := NewWebSocketHandler(&stubSubscriptionManager{}, zap.NewNop())

	principal := h.extractPrincipal(events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			Authorizer: map[string]any{
				"claims": map[string]any{
					"cognito:username": "mod",
					"cognito:groups":   []any{"users", "moderator"},
				},
			},
		},
	})
	require.True(t, principal.Authenticated)
	require.Equal(t, "mod", principal.UserID)
	require.Equal(t, "users,moderator", principal.Role)
	require.True(t, webSocketRoleAllowsAdminAlerts(principal.Role))
	require.True(t, webSocketRoleAllowsAdminAlerts("admin"))
	require.True(t, webSocketRoleAllowsAdminAlerts("mod"))
	require.False(t, webSocketRoleAllowsAdminAlerts("user"))

	h.rememberConnectionPrincipal("c1", principal)
	stored, ok := h.connectionPrincipal("c1")
	require.True(t, ok)
	require.Equal(t, "mod", stored.UserID)

	h.forgetConnectionPrincipal("c1")
	_, ok = h.connectionPrincipal("c1")
	require.False(t, ok)

	anonymous := h.extractPrincipal(events.APIGatewayWebsocketProxyRequest{})
	require.False(t, anonymous.Authenticated)

	require.Nil(t, flattenAuthorizerFields(make(chan int)))
	require.Empty(t, flattenAuthorizerFields("plain"))
	require.Equal(t, "", firstAuthorizerField(nil, "role"))

	h.principals = nil
	h.rememberConnectionPrincipal("c2", principal)
	stored, ok = h.connectionPrincipal("c2")
	require.True(t, ok)
	require.Equal(t, "mod", stored.UserID)

	h.principals = nil
	_, ok = h.connectionPrincipal("missing")
	require.False(t, ok)

	require.Equal(t, http.StatusInternalServerError, webSocketErrorStatus(errors.New("boom")))
	require.Equal(t, "forbidden", (&webSocketStatusError{message: "forbidden"}).Error())
	require.Equal(t, http.StatusForbidden, webSocketErrorStatus(&webSocketStatusError{
		statusCode: http.StatusForbidden,
		message:    "forbidden",
	}))
}

func TestWebSocketHandler_PrincipalForEventDurableMisses(t *testing.T) {
	h := NewWebSocketHandler(&principalAwareStubSubscriptionManager{principalErr: errors.New("boom")}, zap.NewNop())
	_, ok := h.principalForEvent("c1", events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1"},
	})
	require.False(t, ok)

	h = NewWebSocketHandler(&principalAwareStubSubscriptionManager{}, zap.NewNop())
	_, ok = h.principalForEvent("c1", events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1"},
	})
	require.False(t, ok)

	assert.Empty(t, h.parseModerationFilter("not-a-map"))
}

func TestSubscriptionManager_handleDeadConnection_Async(t *testing.T) {
	ch := make(chan string, 1)
	repo := &stubSubscriptionRepo{disconnected: ch}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{"c1": {ConnectionID: "c1"}},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	sm.handleDeadConnection("c1")

	select {
	case connID := <-ch:
		require.Equal(t, "c1", connID)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected disconnect to be invoked")
	}
}
