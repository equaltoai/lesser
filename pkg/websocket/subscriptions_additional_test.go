package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/v3/pkg/streamer"
	"go.uber.org/zap"
)

type stubSubscriptionRepo struct {
	handleConnectErr    error
	handleDisconnectErr error
	getConnectionErr    error
	createErr           error
	deleteErr           error
	getErr              error
	connectRole         string

	created []struct {
		connectionID     string
		subscriptionType string
		filter           map[string]any
	}
	deleted []struct {
		connectionID     string
		subscriptionType string
	}

	subsByType      map[string][]storageModels.WebSocketEventSubscription
	connectionsByID map[string]storageModels.WebSocketEventConnection

	disconnected chan string
}

func (s *stubSubscriptionRepo) HandleConnect(_ context.Context, _, _ string) error {
	return s.handleConnectErr
}

func (s *stubSubscriptionRepo) HandleConnectWithPrincipal(_ context.Context, _, _ string, role string) error {
	s.connectRole = role
	return s.handleConnectErr
}

func (s *stubSubscriptionRepo) HandleDisconnect(_ context.Context, connectionID string) error {
	if s.disconnected != nil {
		s.disconnected <- connectionID
	}
	return s.handleDisconnectErr
}

func (s *stubSubscriptionRepo) GetConnection(_ context.Context, connectionID string) (*storageModels.WebSocketEventConnection, error) {
	if s.getConnectionErr != nil {
		return nil, s.getConnectionErr
	}
	if s.connectionsByID == nil {
		return nil, nil
	}
	connection, ok := s.connectionsByID[connectionID]
	if !ok {
		return nil, nil
	}
	return &connection, nil
}

func (s *stubSubscriptionRepo) CreateSubscription(_ context.Context, connectionID, subscriptionType string, filter map[string]any) error {
	s.created = append(s.created, struct {
		connectionID     string
		subscriptionType string
		filter           map[string]any
	}{connectionID: connectionID, subscriptionType: subscriptionType, filter: filter})
	return s.createErr
}

func (s *stubSubscriptionRepo) DeleteSubscription(_ context.Context, connectionID, subscriptionType string) error {
	s.deleted = append(s.deleted, struct {
		connectionID     string
		subscriptionType string
	}{connectionID: connectionID, subscriptionType: subscriptionType})
	return s.deleteErr
}

func (s *stubSubscriptionRepo) GetSubscriptionsForType(_ context.Context, subscriptionType string) ([]storageModels.WebSocketEventSubscription, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.subsByType[subscriptionType], nil
}

type postCall struct {
	connectionID string
	data         []byte
}

type stubStreamerClient struct {
	postErrByConnection map[string]error
	posts               []postCall
}

func (s *stubStreamerClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	s.posts = append(s.posts, postCall{connectionID: connectionID, data: data})
	if err := s.postErrByConnection[connectionID]; err != nil {
		return err
	}
	return nil
}

func (s *stubStreamerClient) DeleteConnection(_ context.Context, _ string) error { return nil }

func (s *stubStreamerClient) GetConnection(_ context.Context, _ string) (streamer.Connection, error) {
	return streamer.Connection{}, nil
}

func TestSubscriptionManager_SubscribeAndUnsubscribe(t *testing.T) {
	repo := &stubSubscriptionRepo{}
	client := &stubStreamerClient{}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         client,
		connections:   map[string]*Connection{"c1": {ConnectionID: "c1"}},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.SubscribeModerationQueue("c1", ModerationFilter{Severity: []string{"high"}, Types: []string{"spam"}})
	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	assert.Equal(t, "moderation", repo.created[0].subscriptionType)
	assert.NotNil(t, repo.created[0].filter)

	err = sm.Unsubscribe("c1", "moderation")
	require.NoError(t, err)
	require.Len(t, repo.deleted, 1)
	assert.Equal(t, "moderation", repo.deleted[0].subscriptionType)
}

func TestSubscriptionManager_SubscribeRejectsUnknownConnection(t *testing.T) {
	sm := &subscriptionManager{
		repo:          &stubSubscriptionRepo{},
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.SubscribeThreatIntel("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not found")
}

func TestSubscriptionManager_SubscribeHydratesConnectionFromRepository(t *testing.T) {
	now := time.Now()
	repo := &stubSubscriptionRepo{
		connectionsByID: map[string]storageModels.WebSocketEventConnection{
			"c1": {
				ConnectionID: "c1",
				UserID:       "admin",
				Role:         "admin",
				ConnectedAt:  now,
				LastSeen:     now,
				TTL:          now.Add(time.Hour).Unix(),
			},
		},
	}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.SubscribeThreatIntel("c1")
	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	assert.Equal(t, "threat_intel", repo.created[0].subscriptionType)
	require.Contains(t, sm.connections, "c1")
	assert.Equal(t, "admin", sm.connections["c1"].Role)
}

func TestSubscriptionManager_ConnectionPrincipalLoadsDurableRole(t *testing.T) {
	repo := &stubSubscriptionRepo{
		connectionsByID: map[string]storageModels.WebSocketEventConnection{
			"c1": {ConnectionID: "c1", UserID: "mod", Role: "moderator"},
		},
	}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	principal, err := sm.ConnectionPrincipal("c1")
	require.NoError(t, err)
	assert.True(t, principal.Authenticated)
	assert.Equal(t, "mod", principal.UserID)
	assert.Equal(t, "moderator", principal.Role)
}

func TestSubscriptionManager_SubscribeConvenienceMethodsAndPublishAlerts(t *testing.T) {
	repo := &stubSubscriptionRepo{
		subsByType: map[string][]storageModels.WebSocketEventSubscription{
			"performance": {
				{
					ConnectionID:     "c1",
					SubscriptionType: "performance",
					Filter:           map[string]any{"severity": "critical"},
				},
			},
			"infrastructure": {
				{ConnectionID: "c2", SubscriptionType: "infrastructure"},
			},
		},
	}
	client := &stubStreamerClient{}
	sm := &subscriptionManager{
		repo:  repo,
		apiGW: client,
		connections: map[string]*Connection{
			"c1": {ConnectionID: "c1"},
		},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	require.NoError(t, sm.SubscribePerformanceAlerts("c1", "critical"))
	require.NoError(t, sm.SubscribeInfrastructureEvents("c1"))
	require.NoError(t, sm.SubscribeCommunityNotes("c1"))
	require.NoError(t, sm.SubscribeTimeline("c1"))
	require.NoError(t, sm.SubscribeNotifications("c1"))
	require.Len(t, repo.created, 5)

	require.NoError(t, sm.PublishPerformanceAlert(&PerformanceAlert{
		ID:        "p1",
		Severity:  "critical",
		Timestamp: time.Now(),
	}))
	require.NoError(t, sm.PublishInfrastructureEvent(&InfrastructureEvent{
		ID:        "i1",
		Timestamp: time.Now(),
	}))
	require.Len(t, client.posts, 2)
	assert.Equal(t, "c1", client.posts[0].connectionID)
	assert.Equal(t, "c2", client.posts[1].connectionID)
}

func TestSubscriptionManager_PublishToSubscribers_MarshalError(t *testing.T) {
	sm := &subscriptionManager{
		repo:          &stubSubscriptionRepo{},
		apiGW:         &stubStreamerClient{},
		connections:   map[string]*Connection{},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	err := sm.publishToSubscribers("any", WebSocketMessage{Type: "x", Data: func() {}}, nil)
	require.Error(t, err)
}

func TestSubscriptionManager_PublishModerationEvent_FilterAndDeadConnection(t *testing.T) {
	disconnected := make(chan string, 1)
	repo := &stubSubscriptionRepo{
		subsByType: map[string][]storageModels.WebSocketEventSubscription{
			"moderation": {
				{ConnectionID: "c1", SubscriptionType: "moderation"},
				{ConnectionID: "c2", SubscriptionType: "moderation", Filter: map[string]any{"severity": []any{"low"}}},
				{ConnectionID: "c3", SubscriptionType: "moderation"},
			},
		},
		disconnected: disconnected,
	}
	client := &stubStreamerClient{postErrByConnection: map[string]error{"c3": errors.New("gone")}}
	sm := &subscriptionManager{
		repo:          repo,
		apiGW:         client,
		connections:   map[string]*Connection{"c1": {ConnectionID: "c1"}, "c2": {ConnectionID: "c2"}, "c3": {ConnectionID: "c3"}},
		subscriptions: map[string]map[string]*Subscription{},
		logger:        zap.NewNop(),
	}

	event := &ModerationEvent{ID: "e1", Type: "spam", Severity: "high", ContentID: "c", UserID: "u", Timestamp: time.Now()}
	err := sm.PublishModerationEvent(event)
	require.Error(t, err)

	// c2 is filtered out (severity mismatch), c3 fails and triggers disconnect cleanup, c1 succeeds.
	require.Len(t, client.posts, 2)

	var msg WebSocketMessage
	require.NoError(t, json.Unmarshal(client.posts[0].data, &msg))
	assert.Equal(t, "moderation_event", msg.Type)

	select {
	case connID := <-disconnected:
		assert.Equal(t, "c3", connID)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected dead connection cleanup")
	}
}

type stubSubscriptionManager struct {
	connectUserID string
	connectErr    error

	subscribeModerationFilter ModerationFilter
	performanceSeverity       string
	lastUnsubscribeType       string
	unsubscribeErr            error
}

func (s *stubSubscriptionManager) SubscribeModerationQueue(_ string, filter ModerationFilter) error {
	s.subscribeModerationFilter = filter
	return nil
}
func (s *stubSubscriptionManager) SubscribeThreatIntel(_ string) error { return nil }
func (s *stubSubscriptionManager) SubscribePerformanceAlerts(_ string, severity string) error {
	s.performanceSeverity = severity
	return nil
}
func (s *stubSubscriptionManager) SubscribeInfrastructureEvents(_ string) error { return nil }
func (s *stubSubscriptionManager) SubscribeCommunityNotes(_ string) error       { return nil }
func (s *stubSubscriptionManager) SubscribeTimeline(_ string) error             { return nil }
func (s *stubSubscriptionManager) SubscribeNotifications(_ string) error        { return nil }
func (s *stubSubscriptionManager) Unsubscribe(_ string, subscriptionType string) error {
	s.lastUnsubscribeType = subscriptionType
	return s.unsubscribeErr
}
func (s *stubSubscriptionManager) PublishModerationEvent(_ *ModerationEvent) error   { return nil }
func (s *stubSubscriptionManager) PublishThreatAlert(_ *ThreatAlert) error           { return nil }
func (s *stubSubscriptionManager) PublishPerformanceAlert(_ *PerformanceAlert) error { return nil }
func (s *stubSubscriptionManager) PublishInfrastructureEvent(_ *InfrastructureEvent) error {
	return nil
}
func (s *stubSubscriptionManager) HandleConnect(_ string, userID string) error {
	s.connectUserID = userID
	return s.connectErr
}
func (s *stubSubscriptionManager) HandleDisconnect(_ string) error { return nil }

type principalAwareStubSubscriptionManager struct {
	stubSubscriptionManager
	connectPrincipal webSocketPrincipal
	principal        webSocketPrincipal
	principalErr     error
}

func (s *principalAwareStubSubscriptionManager) HandleConnectWithPrincipal(_ string, principal webSocketPrincipal) error {
	s.connectPrincipal = principal
	s.connectUserID = principal.UserID
	return s.connectErr
}

func (s *principalAwareStubSubscriptionManager) ConnectionPrincipal(_ string) (webSocketPrincipal, error) {
	return s.principal, s.principalErr
}

func TestWebSocketHandler_Routes(t *testing.T) {
	sm := &stubSubscriptionManager{}
	h := NewWebSocketHandler(sm, zap.NewNop())

	resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$connect",
			Authorizer: map[string]any{
				"username": "u1",
				"role":     "admin",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "u1", sm.connectUserID)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext:        events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:                  `{"type":"performance","filter":{"severity":"critical"}}`,
		QueryStringParameters: map[string]string{},
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "critical", sm.performanceSeverity)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"moderation","filter":{"severity":["high"],"types":["spam"],"user_id":"u1"}}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []string{"high"}, sm.subscribeModerationFilter.Severity)
	assert.Equal(t, []string{"spam"}, sm.subscribeModerationFilter.Types)
	assert.Equal(t, "u1", sm.subscribeModerationFilter.UserID)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "unsubscribe"},
		Body:           `{"type":"moderation"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "moderation", sm.lastUnsubscribeType)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `not-json`,
	})
	require.Error(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "nope"},
	})
	require.Error(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestWebSocketHandler_AdminAlertSubscriptionsRequireAuthenticatedRoles(t *testing.T) {
	sm := &stubSubscriptionManager{}
	h := NewWebSocketHandler(sm, zap.NewNop())

	resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"moderation"}`,
	})
	require.Error(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	h.rememberConnectionPrincipal("c1", webSocketPrincipal{
		UserID:        "alice",
		Role:          "user",
		Authenticated: true,
	})
	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"threat_intel"}`,
	})
	require.Error(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	h.rememberConnectionPrincipal("c1", webSocketPrincipal{
		UserID:        "mod",
		Role:          "moderator",
		Authenticated: true,
	})
	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"infrastructure"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"timeline"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestWebSocketHandler_AdminAlertSubscriptionUsesRouteAuthorizer(t *testing.T) {
	sm := &stubSubscriptionManager{}
	h := NewWebSocketHandler(sm, zap.NewNop())

	resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "subscribe",
			Authorizer: map[string]any{
				"username": "mod",
				"role":     "moderator",
			},
		},
		Body: `{"type":"threat_intel"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	stored, ok := h.connectionPrincipal("c1")
	require.True(t, ok)
	assert.Equal(t, "mod", stored.UserID)
	assert.Equal(t, "moderator", stored.Role)
}

func TestWebSocketHandler_AdminAlertSubscriptionLoadsDurablePrincipal(t *testing.T) {
	sm := &principalAwareStubSubscriptionManager{
		principal: webSocketPrincipal{
			UserID:        "admin",
			Role:          "admin",
			Authenticated: true,
		},
	}
	h := NewWebSocketHandler(sm, zap.NewNop())

	resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1", RouteKey: "subscribe"},
		Body:           `{"type":"infrastructure"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	stored, ok := h.connectionPrincipal("c1")
	require.True(t, ok)
	assert.Equal(t, "admin", stored.UserID)
	assert.Equal(t, "admin", stored.Role)
}

func TestWebSocketHandler_ConnectPersistsDurablePrincipalRole(t *testing.T) {
	sm := &principalAwareStubSubscriptionManager{}
	h := NewWebSocketHandler(sm, zap.NewNop())

	resp, err := h.HandleAPIGatewayWebSocketEvent(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$connect",
			Authorizer: map[string]any{
				"username": "mod",
				"role":     "moderator",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "mod", sm.connectPrincipal.UserID)
	assert.Equal(t, "moderator", sm.connectPrincipal.Role)
}
