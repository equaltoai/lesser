// Package main implements the GraphQL WebSocket Lambda entrypoint.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	gqllimits "github.com/equaltoai/lesser/pkg/graphql/limits"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/golang-jwt/jwt/v5"
	appstreamer "github.com/theory-cloud/apptheory/v2/pkg/streamer"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

const graphqlWSName = "graphql-ws"
const graphqlTransportWSSubprotocol = "graphql-transport-ws"
const wsCodeUnauthenticated = "UNAUTHENTICATED"
const wsCodeCredentialExpired = "TOKEN_EXPIRED"
const wsCredentialExpiresAtHeader = "token_expires_at"
const protocolErrorID = "protocol-error"

const (
	wsCloseInvalidMessage                = 4400
	wsCloseForbidden                     = 4403
	wsCloseTooManyInitialisationRequests = 4429
)

type connectionState struct {
	username      string
	claims        *auth.Claims
	initialized   bool
	subscriptions map[string]*subscriptionState
}

type subscriptionState struct {
	cancel context.CancelFunc
}

type tokenValidator interface {
	ValidateAccessToken(tokenString string) (*auth.Claims, error)
}

type gqlExecutor interface {
	CreateOperationContext(ctx context.Context, params *graphql.RawParams) (*graphql.OperationContext, gqlerror.List)
	DispatchOperation(ctx context.Context, opCtx *graphql.OperationContext) (graphql.ResponseHandler, context.Context)
}

type graphqlConnectionRepo interface {
	WriteConnection(ctx context.Context, connectionID string, userID string, username string, streams []string) (*models.WebSocketConnection, error)
	UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error
	DeleteAllSubscriptions(ctx context.Context, connectionID string) error
	DeleteConnection(ctx context.Context, connectionID string) error
	GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error)
	DeleteSubscription(ctx context.Context, connectionID string, stream string) error
}

type subscriptionManager interface {
	IsRunning() bool
	Start(ctx context.Context) error
}

type instanceStateRepo interface {
	GetInstanceState(ctx context.Context) (*models.InstanceState, error)
}

type wsServer struct {
	oauthService        tokenValidator
	logger              *zap.Logger
	subscriptionManager subscriptionManager
	exec                gqlExecutor
	startOnce           sync.Once
	connRepo            graphqlConnectionRepo
	gqlSubRepo          *repositories.GraphQLStreamSubscriptionRepository
	instanceRepo        instanceStateRepo

	mu          sync.RWMutex
	connections map[string]*connectionState
	wsContexts  map[string]*apptheory.WebSocketContext // Store WebSocket contexts for message sending

	sendJSONMessage func(wsCtx *apptheory.WebSocketContext, payload any) error
	closeConnection func(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, code int, reason string) error
}

type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func accessTokenFromInitPayload(raw json.RawMessage) (token string, presented bool, err error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", false, err
	}
	if len(payload) == 0 {
		return "", false, nil
	}

	// Common shapes seen in GraphQL WS clients:
	// - { "authorization": "Bearer <token>" }
	// - { "Authorization": "Bearer <token>" }
	// - { "access_token": "<token>" } / { "token": "<token>" }
	// - { "headers": { "Authorization": "Bearer <token>" } }
	readString := func(container map[string]any, key string) (string, bool, error) {
		value, ok := container[key]
		if !ok {
			return "", false, nil
		}
		valueString, ok := value.(string)
		if !ok {
			return "", true, fmt.Errorf("connection_init credential %q must be a string", key)
		}
		return strings.TrimSpace(valueString), true, nil
	}

	for _, key := range []string{"access_token", "accessToken", "token", "authToken"} {
		if value, found, readErr := readString(payload, key); found || readErr != nil {
			return cleanToken(value), found, readErr
		}
	}

	readHeader := func(container map[string]any, key string) (string, bool, error) {
		var matchedKey string
		for candidate := range container {
			if !strings.EqualFold(candidate, key) {
				continue
			}
			if matchedKey != "" {
				return "", true, fmt.Errorf("connection_init credential %q is ambiguous", key)
			}
			matchedKey = candidate
		}
		if matchedKey == "" {
			return "", false, nil
		}
		return readString(container, matchedKey)
	}

	if value, found, readErr := readHeader(payload, "authorization"); found || readErr != nil {
		return normalizeAuthToken(value), found, readErr
	}

	headersValue, headersFound, headersErr := func() (any, bool, error) {
		var matchedKey string
		for candidate := range payload {
			if !strings.EqualFold(candidate, "headers") {
				continue
			}
			if matchedKey != "" {
				return nil, true, errors.New("connection_init headers field is ambiguous")
			}
			matchedKey = candidate
		}
		if matchedKey == "" {
			return nil, false, nil
		}
		return payload[matchedKey], true, nil
	}()
	if headersErr != nil {
		return "", true, headersErr
	}
	if headersFound {
		headers, ok := headersValue.(map[string]any)
		if !ok {
			return "", true, errors.New("connection_init headers must be an object")
		}
		if value, found, readErr := readHeader(headers, "authorization"); found || readErr != nil {
			return normalizeAuthToken(value), found, readErr
		}
		return "", true, errors.New("connection_init headers must contain authorization")
	}

	if containsCredentialKey(payload) {
		return "", true, errors.New("connection_init credential must use a supported top-level shape")
	}

	return "", false, nil
}

func containsCredentialKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
			switch normalized {
			case "authorization", "accesstoken", "token", "authtoken":
				return true
			}
			if containsCredentialKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsCredentialKey(nested) {
				return true
			}
		}
	}
	return false
}

type subscribePayload struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
	Extensions    map[string]interface{} `json:"extensions,omitempty"`
}

type responseEnvelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

var (
	lambdaCtx      *common.LambdaContext
	cfg            *appconfig.Config
	logger         *zap.Logger
	repos          core.RepositoryStorage
	oauth          *auth.OAuthService
	connectionRepo *repositories.StreamingConnectionRepository
	gqlSubRepo     *repositories.GraphQLStreamSubscriptionRepository
	server         *wsServer
)

func newServer(oauthService tokenValidator, resolver *graph.Resolver, exec gqlExecutor, log *zap.Logger, connRepo graphqlConnectionRepo, gqlSubRepo *repositories.GraphQLStreamSubscriptionRepository, instanceRepo instanceStateRepo) *wsServer {
	if log == nil {
		log = zap.NewNop()
	}

	var subManager subscriptionManager
	if resolver != nil {
		subManager = resolver.SubscriptionManager
	}

	return &wsServer{
		oauthService:        oauthService,
		logger:              log,
		subscriptionManager: subManager,
		exec:                exec,
		connRepo:            connRepo,
		gqlSubRepo:          gqlSubRepo,
		instanceRepo:        instanceRepo,
		connections:         make(map[string]*connectionState),
		wsContexts:          make(map[string]*apptheory.WebSocketContext),
		sendJSONMessage: func(wsCtx *apptheory.WebSocketContext, payload any) error {
			return wsCtx.SendJSONMessage(payload)
		},
		closeConnection: func(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, _ int, _ string) error {
			if wsCtx == nil {
				return errors.New("websocket context is nil")
			}
			client, err := appstreamer.NewClient(ctx, wsCtx.ManagementEndpoint)
			if err != nil {
				return err
			}
			return client.DeleteConnection(ctx, connectionID)
		},
	}
}

func (s *wsServer) sendJSON(wsCtx *apptheory.WebSocketContext, payload any) error {
	if wsCtx == nil {
		return fmt.Errorf("websocket context is nil")
	}
	if s != nil && s.sendJSONMessage != nil {
		return s.sendJSONMessage(wsCtx, payload)
	}
	return wsCtx.SendJSONMessage(payload)
}

func (s *wsServer) registerConnection(ctx context.Context, connectionID, username string, claims *auth.Claims) error {
	var persistErr error

	var connectionRecord *models.WebSocketConnection
	if s.connRepo != nil {
		streams := []string{"graphql"}
		conn, err := s.connRepo.WriteConnection(ctx, connectionID, username, username, streams)
		if err != nil {
			s.logger.Warn("failed to write graphql connection record",
				zap.String("connection_id", connectionID),
				zap.String("username", username),
				zap.Error(err),
				zap.String("error_details", err.Error()))
			if persistErr == nil {
				persistErr = err
			}
		} else {
			connectionRecord = conn
		}
		if connectionRecord != nil {
			connectionRecord.Streams = streams
			connectionRecord.Username = username
			connectionRecord.UserID = username
			connectionRecord.Info.Protocol = graphqlWSName
			connectionRecord.Info.AuthMethod = "oauth"
			if connectionRecord.Info.CustomHeaders == nil {
				connectionRecord.Info.CustomHeaders = make(map[string]string)
			}
			if claims != nil && len(claims.Scopes) > 0 {
				connectionRecord.Info.CustomHeaders["scopes"] = strings.Join(claims.Scopes, " ")
			}
			if claims != nil && claims.ExpiresAt != nil {
				connectionRecord.Info.CustomHeaders[wsCredentialExpiresAtHeader] = strconv.FormatInt(claims.ExpiresAt.Unix(), 10)
			}
			connectionRecord.LastActivity = time.Now()
			connectionRecord.Established = time.Now()
			connectionRecord.UpdateState(models.ConnectionStateConnected)

			if err := s.connRepo.UpdateConnection(ctx, connectionRecord); err != nil {
				s.logger.Warn("failed to update graphql connection metadata",
					zap.String("connection_id", connectionID),
					zap.Error(err))
				if persistErr == nil {
					persistErr = err
				}
			}
		}
	} else {
		persistErr = fmt.Errorf("connection repository not configured")
	}

	s.mu.Lock()
	subscriptions := make(map[string]*subscriptionState)
	if existing := s.connections[connectionID]; existing != nil && existing.subscriptions != nil {
		subscriptions = existing.subscriptions
	}
	s.connections[connectionID] = &connectionState{
		username:      username,
		claims:        claims,
		initialized:   true,
		subscriptions: subscriptions,
	}
	s.mu.Unlock()

	return persistErr
}

func (s *wsServer) registerAnonymousConnection(ctx context.Context, connectionID string) error {
	var persistErr error
	if s.connRepo != nil {
		streams := []string{"graphql"}
		connection, err := s.connRepo.WriteConnection(ctx, connectionID, "", "", streams)
		if err != nil {
			persistErr = err
		} else if connection != nil {
			connection.Streams = streams
			connection.Username = ""
			connection.UserID = ""
			connection.Info.Protocol = graphqlWSName
			connection.Info.AuthMethod = "anonymous"
			connection.LastActivity = time.Now()
			connection.Established = time.Now()
			connection.UpdateState(models.ConnectionStateConnected)
			if err := s.connRepo.UpdateConnection(ctx, connection); err != nil {
				persistErr = err
			}
		}
	} else {
		persistErr = fmt.Errorf("connection repository not configured")
	}

	s.mu.Lock()
	subscriptions := make(map[string]*subscriptionState)
	if existing := s.connections[connectionID]; existing != nil && existing.subscriptions != nil {
		subscriptions = existing.subscriptions
	}
	s.connections[connectionID] = &connectionState{
		initialized:   true,
		subscriptions: subscriptions,
	}
	s.mu.Unlock()

	return persistErr
}

func (s *wsServer) removeConnection(ctx context.Context, connectionID string) {
	var state *connectionState
	s.mu.Lock()
	if existing, ok := s.connections[connectionID]; ok {
		state = existing
	}
	delete(s.connections, connectionID)
	delete(s.wsContexts, connectionID)
	s.mu.Unlock()

	if state != nil {
		for id, sub := range state.subscriptions {
			if sub != nil && sub.cancel != nil {
				sub.cancel()
			}
			delete(state.subscriptions, id)
		}
	}

	if s.connRepo != nil {
		if err := s.connRepo.DeleteAllSubscriptions(ctx, connectionID); err != nil {
			s.logger.Warn("failed to purge graphql subscriptions for connection",
				zap.String("connection_id", connectionID),
				zap.Error(err))
		}

		if s.gqlSubRepo != nil {
			if err := s.gqlSubRepo.DeleteAllForConnection(ctx, connectionID); err != nil {
				s.logger.Warn("failed to purge graphql stream subscriptions for connection",
					zap.String("connection_id", connectionID),
					zap.Error(err))
			}
		}

		if err := s.connRepo.DeleteConnection(ctx, connectionID); err != nil {
			s.logger.Warn("failed to delete graphql connection record",
				zap.String("connection_id", connectionID),
				zap.Error(err))
		}
	}
}

func (s *wsServer) getConnection(ctx context.Context, connectionID string) (*connectionState, error) {
	s.mu.RLock()
	state, ok := s.connections[connectionID]
	s.mu.RUnlock()
	if ok && state != nil {
		return state, nil
	}

	if s.connRepo == nil {
		return nil, fmt.Errorf("connection repository not configured")
	}

	connection, err := s.connRepo.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	var scopes []string
	if connection.Info.CustomHeaders != nil {
		if scopeValue, ok := connection.Info.CustomHeaders["scopes"]; ok && scopeValue != "" {
			scopes = strings.Fields(scopeValue)
		}
	}

	var claims *auth.Claims
	if connection.Username != "" {
		claims = &auth.Claims{
			Username: connection.Username,
			Scopes:   scopes,
		}
		if connection.Info.CustomHeaders != nil {
			rawExpiresAt := strings.TrimSpace(connection.Info.CustomHeaders[wsCredentialExpiresAtHeader])
			if rawExpiresAt != "" {
				expiresAt, parseErr := strconv.ParseInt(rawExpiresAt, 10, 64)
				if parseErr != nil {
					return nil, fmt.Errorf("invalid persisted graphql credential expiry: %w", parseErr)
				}
				claims.ExpiresAt = jwt.NewNumericDate(time.Unix(expiresAt, 0).UTC())
			}
		}
	}

	state = &connectionState{
		username:      connection.Username,
		claims:        claims,
		initialized:   connection.Username != "" || connection.Info.AuthMethod == "anonymous",
		subscriptions: make(map[string]*subscriptionState),
	}

	s.mu.Lock()
	s.connections[connectionID] = state
	s.mu.Unlock()

	return state, nil
}

func subscriptionStreamName(subscriptionID string) string {
	return fmt.Sprintf("graphql:subscription:%s", subscriptionID)
}

func (s *wsServer) removeSubscriptionRecord(ctx context.Context, connectionID, subscriptionID string) {
	if s.connRepo == nil || subscriptionID == "" {
		return
	}

	stream := subscriptionStreamName(subscriptionID)
	if err := s.connRepo.DeleteSubscription(ctx, connectionID, stream); err != nil {
		s.logger.Warn("failed to delete graphql subscription record",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", subscriptionID),
			zap.String("stream", stream),
			zap.Error(err))
	}
}

func (s *wsServer) addSubscription(connectionID, subscriptionID string, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.connections[connectionID]
	if !ok {
		return false
	}

	if state.subscriptions == nil {
		state.subscriptions = make(map[string]*subscriptionState)
	}

	if existing, exists := state.subscriptions[subscriptionID]; exists && existing != nil && existing.cancel != nil {
		existing.cancel()
	}

	state.subscriptions[subscriptionID] = &subscriptionState{
		cancel: cancel,
	}
	return true
}

func (s *wsServer) cancelSubscription(ctx context.Context, connectionID, subscriptionID string) bool {
	return s.clearSubscription(ctx, connectionID, subscriptionID, true)
}

func (s *wsServer) clearSubscription(ctx context.Context, connectionID, subscriptionID string, cancel bool) bool {
	var cancelFunc context.CancelFunc
	var found bool
	var username string

	s.mu.Lock()
	if state, ok := s.connections[connectionID]; ok && state.subscriptions != nil {
		if sub, ok := state.subscriptions[subscriptionID]; ok {
			cancelFunc = sub.cancel
			delete(state.subscriptions, subscriptionID)
			found = true
			username = state.username
		}
	}
	s.mu.Unlock()

	if cancel && cancelFunc != nil {
		cancelFunc()
	}

	if found {
		s.removeSubscriptionRecord(ctx, connectionID, subscriptionID)
		if username != "" {
			s.logger.Debug("removed graphql subscription",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.String("username", username))
		}
	}

	return found
}

func (s *wsServer) ensureSubscriptionManagerStarted() {
	if s.subscriptionManager == nil {
		return
	}

	s.startOnce.Do(func() {
		if s.subscriptionManager.IsRunning() {
			return
		}
		if err := s.subscriptionManager.Start(context.Background()); err != nil {
			s.logger.Warn("failed to start subscription manager", zap.Error(err))
		}
	})
}

func (s *wsServer) buildRequestContext(ctx context.Context, state *connectionState, connectionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if state != nil && state.claims != nil {
		ctx = context.WithValue(ctx, common.ContextKeyClaims, state.claims)
	}

	if connectionID != "" {
		ctx = graph.WithConnectionID(ctx, connectionID)
	}

	return ctx
}

func convertErrors(list gqlerror.List) []error {
	errs := make([]error, 0, len(list))
	for _, e := range list {
		if e == nil {
			continue
		}
		errs = append(errs, e)
	}
	return errs
}

func queryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func appError(code, message string) error {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "app.internal"
	}
	if message == "" {
		message = "internal error"
	}
	return apptheory.NewAppTheoryError(code, message)
}

func okWebSocketResponse() *apptheory.Response {
	return &apptheory.Response{Status: 200}
}

func parseWebSocketSubprotocols(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func selectGraphQLSubprotocol(requested []string) (string, bool) {
	for _, protocol := range requested {
		if strings.EqualFold(protocol, graphqlTransportWSSubprotocol) {
			return graphqlTransportWSSubprotocol, true
		}
	}
	return "", false
}

func (s *wsServer) handleConnect(ctx *apptheory.Context) (*apptheory.Response, error) {
	if ctx == nil {
		return nil, appError("app.bad_request", "Invalid WebSocket event")
	}
	wsCtx := ctx.AsWebSocket()
	if wsCtx == nil {
		return nil, appError("app.bad_request", "Invalid WebSocket event")
	}

	connectionID := wsCtx.ConnectionID
	log := s.logger.With(
		zap.String("connection_id", connectionID),
		zap.String("route", "$connect"),
	)

	// Protocol negotiation for browser GraphQL clients:
	// Echo "graphql-transport-ws" back in the handshake when the client requests it.
	rawProtocols := headerValue(ctx, "sec-websocket-protocol")
	requestedProtocols := parseWebSocketSubprotocols(rawProtocols)
	selectedProtocol, ok := selectGraphQLSubprotocol(requestedProtocols)
	if len(requestedProtocols) > 0 && !ok {
		log.Warn("unsupported websocket subprotocol",
			zap.String("requested", rawProtocols))
		return nil, appError("app.bad_request", fmt.Sprintf("unsupported Sec-WebSocket-Protocol (expected %q)", graphqlTransportWSSubprotocol))
	}

	resp := okWebSocketResponse()
	if selectedProtocol != "" {
		resp.Headers = map[string][]string{
			"sec-websocket-protocol": {selectedProtocol},
		}
	}

	// Auth contract: authenticate via `connection_init` payload (not handshake query/header).
	rawAccessToken := strings.TrimSpace(queryValue(ctx, "access_token"))
	rawToken := strings.TrimSpace(queryValue(ctx, "token"))
	authHeader := strings.TrimSpace(headerValue(ctx, "authorization"))
	if rawAccessToken != "" || rawToken != "" || authHeader != "" {
		log.Info("websocket connect includes handshake auth token; ignored (use connection_init payload)",
			zap.Bool("has_access_token_param", rawAccessToken != ""),
			zap.Bool("has_token_param", rawToken != ""),
			zap.Bool("has_authorization_header", authHeader != ""))
	} else {
		log.Info("websocket connect pending authentication; expecting connection_init")
	}

	// Store WebSocket context for later message sending.
	s.rememberWebSocketContext(connectionID, wsCtx)

	// Persist a placeholder connection record so $default invocations can load state across Lambda containers.
	if s.connRepo != nil {
		streams := []string{"graphql"}
		conn, err := s.connRepo.WriteConnection(ctx.Context(), connectionID, "", "", streams)
		if err != nil {
			log.Warn("failed to write pending graphql connection record", zap.Error(err))
		} else if conn != nil {
			conn.Streams = streams
			conn.Info.Protocol = graphqlWSName
			conn.Info.AuthMethod = "pending"
			conn.LastActivity = time.Now()
			conn.Established = time.Now()
			conn.UpdateState(models.ConnectionStateConnected)
			_ = s.connRepo.UpdateConnection(ctx.Context(), conn)
		}
	}

	s.mu.Lock()
	s.connections[connectionID] = &connectionState{
		username:      "",
		claims:        nil,
		initialized:   false,
		subscriptions: make(map[string]*subscriptionState),
	}
	s.mu.Unlock()

	return resp, nil
}

func (s *wsServer) handleDisconnect(ctx *apptheory.Context) (*apptheory.Response, error) {
	if ctx == nil {
		return okWebSocketResponse(), nil
	}

	wsCtx := ctx.AsWebSocket()
	if wsCtx == nil {
		return okWebSocketResponse(), nil
	}

	connectionID := wsCtx.ConnectionID

	// Clean up stored WebSocket context
	s.mu.Lock()
	delete(s.wsContexts, connectionID)
	s.mu.Unlock()

	s.removeConnection(ctx.Context(), connectionID)
	s.logger.Info("websocket connection closed",
		zap.String("connection_id", connectionID))
	return okWebSocketResponse(), nil
}

func (s *wsServer) handleDefault(ctx *apptheory.Context) (*apptheory.Response, error) {
	wsCtx, connectionID, err := s.webSocketContextFromEvent(ctx)
	if err != nil {
		return nil, err
	}

	s.rememberWebSocketContext(connectionID, wsCtx)

	var msg wsMessage
	if err := json.Unmarshal(ctx.Request.Body, &msg); err != nil {
		s.logger.Warn("failed to parse websocket message",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		return nil, appError("app.bad_request", "Failed to parse message body")
	}

	switch strings.ToLower(msg.Type) {
	case "connection_init":
		return s.handleConnectionInit(ctx.Context(), wsCtx, connectionID, msg)
	case "ping":
		if err := s.sendJSON(wsCtx, responseEnvelope{Type: "pong"}); err != nil {
			return nil, err
		}
		return okWebSocketResponse(), nil
	case "subscribe":
		s.handleSubscribe(ctx.Context(), msg, wsCtx)
		return okWebSocketResponse(), nil
	case "complete":
		return s.handleComplete(ctx.Context(), wsCtx, connectionID, msg)
	default:
		s.logger.Warn("received unsupported websocket message type",
			zap.String("connection_id", connectionID),
			zap.String("type", msg.Type))
		return nil, appError("app.bad_request", fmt.Sprintf("message type %q is not supported", msg.Type))
	}
}

func (s *wsServer) webSocketContextFromEvent(ctx *apptheory.Context) (*apptheory.WebSocketContext, string, error) {
	if ctx == nil {
		return nil, "", appError("app.bad_request", "Invalid WebSocket event")
	}

	wsCtx := ctx.AsWebSocket()
	if wsCtx == nil {
		return nil, "", appError("app.bad_request", "Invalid WebSocket event")
	}

	return wsCtx, wsCtx.ConnectionID, nil
}

func (s *wsServer) rememberWebSocketContext(connectionID string, wsCtx *apptheory.WebSocketContext) {
	if wsCtx == nil || connectionID == "" {
		return
	}

	s.mu.Lock()
	s.wsContexts[connectionID] = wsCtx
	s.mu.Unlock()
}

func (s *wsServer) handleConnectionInit(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, msg wsMessage) (*apptheory.Response, error) {
	log := s.logger.With(zap.String("connection_id", connectionID))

	if s.isInitializedConnection(ctx, connectionID) {
		log.Warn("received duplicate connection_init")
		s.closeConnectionWithDesiredCode(
			ctx,
			wsCtx,
			connectionID,
			wsCloseTooManyInitialisationRequests,
			"Too many initialisation requests",
		)
		return okWebSocketResponse(), nil
	}

	tokenValue, credentialPresented, payloadErr := accessTokenFromInitPayload(msg.Payload)
	if payloadErr != nil {
		log.Warn("connection_init contains an invalid payload", zap.Error(payloadErr))
		s.rejectConnectionInit(ctx, wsCtx, connectionID, wsCloseInvalidMessage, "Invalid connection_init payload")
		return okWebSocketResponse(), nil
	}
	if !credentialPresented {
		if err := s.registerAnonymousConnection(ctx, connectionID); err != nil {
			log.Warn("failed to persist anonymous graphql connection identity", zap.Error(err))
		}
		log.Info("connection_init accepted for anonymous principal")
		if err := s.sendConnectionACK(wsCtx, false); err != nil {
			return nil, err
		}
		return okWebSocketResponse(), nil
	}
	if tokenValue == "" {
		log.Warn("connection_init contains an empty access token")
		s.rejectConnectionInit(ctx, wsCtx, connectionID, wsCloseForbidden, "Forbidden")
		return okWebSocketResponse(), nil
	}

	if s.oauthService == nil {
		log.Error("oauth service not configured for graphql websocket server")
		return nil, appError("app.internal", "OAuth service unavailable")
	}

	claims, err := s.oauthService.ValidateAccessToken(tokenValue)
	if err != nil {
		log.Warn("connection_init failed token validation", zap.Error(err))
		s.rejectConnectionInit(ctx, wsCtx, connectionID, wsCloseForbidden, "Forbidden")
		return okWebSocketResponse(), nil
	}

	username := claims.GetUsername()
	if username == "" {
		username = claims.Username
	}
	username = strings.TrimSpace(username)
	if username == "" {
		log.Warn("connection_init missing username in claims")
		s.rejectConnectionInit(ctx, wsCtx, connectionID, wsCloseForbidden, "Forbidden")
		return okWebSocketResponse(), nil
	}

	if err := s.registerConnection(ctx, connectionID, username, claims); err != nil {
		log.Warn("failed to persist graphql connection identity", zap.Error(err))
	}

	log.Info("connection_init authenticated", zap.String("username", username))
	if err := s.sendConnectionACK(wsCtx, true); err != nil {
		return nil, err
	}
	return okWebSocketResponse(), nil
}

func (s *wsServer) rejectConnectionInit(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, code int, reason string) {
	// graphql-transport-ws defines failed initialization as a socket close, not
	// an operation-scoped Error message.
	s.removeConnection(ctx, connectionID)
	s.closeConnectionWithDesiredCode(ctx, wsCtx, connectionID, code, reason)
}

func (s *wsServer) closeConnectionWithDesiredCode(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, code int, reason string) {
	// API Gateway's DeleteConnection API does not accept a close code or reason,
	// but keeping the protocol values in this seam lets tests pin the intended
	// classification and a future transport expose the exact CloseEvent without
	// changing the authorization decision. Callers decide whether state removal
	// is appropriate before reaching this side-effect-bounded transport seam.
	if s.closeConnection == nil {
		s.logger.Error("graphql websocket connection closer is not configured",
			zap.String("connection_id", connectionID),
			zap.Int("desired_close_code", code))
		return
	}
	if err := s.closeConnection(ctx, wsCtx, connectionID, code, reason); err != nil {
		s.logger.Warn("failed to disconnect rejected graphql websocket connection",
			zap.String("connection_id", connectionID),
			zap.Int("desired_close_code", code),
			zap.Error(err))
	}
}

func (s *wsServer) isInitializedConnection(ctx context.Context, connectionID string) bool {
	state, err := s.getConnection(ctx, connectionID)
	return err == nil && state != nil && state.initialized
}

func (s *wsServer) handleComplete(ctx context.Context, wsCtx *apptheory.WebSocketContext, connectionID string, msg wsMessage) (*apptheory.Response, error) {
	s.logger.Info("received completion request",
		zap.String("connection_id", connectionID),
		zap.String("subscription_id", msg.ID))

	if s.gqlSubRepo != nil && msg.ID != "" {
		if err := s.gqlSubRepo.DeleteSubscription(ctx, connectionID, msg.ID); err != nil {
			s.logger.Warn("failed to delete graphql stream subscription records",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", msg.ID),
				zap.Error(err))
		}
	}

	if !s.cancelSubscription(ctx, connectionID, msg.ID) {
		_ = s.sendJSON(wsCtx, responseEnvelope{
			ID:   msg.ID,
			Type: "complete",
		})
	}
	return okWebSocketResponse(), nil
}

func normalizeAuthToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "bearer ") {
		trimmed = strings.TrimSpace(trimmed[7:])
	}

	return cleanToken(trimmed)
}

func cleanToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" {
		return ""
	}

	if decoded, err := url.QueryUnescape(token); err == nil {
		token = decoded
	}

	token = strings.ReplaceAll(token, " ", "+")
	return token
}

func (s *wsServer) handleSubscribe(ctx context.Context, msg wsMessage, wsCtx *apptheory.WebSocketContext) {
	if msg.ID == "" {
		s.sendError(wsCtx, "", "invalid_request", "subscription messages must include an id")
		return
	}

	if s.instanceRepo == nil {
		s.logger.Error("instance repository unavailable for graphql subscriptions")
		s.sendError(wsCtx, msg.ID, "internal_error", "instance repository unavailable")
		return
	}

	instanceState, instanceErr := s.instanceRepo.GetInstanceState(ctx)
	if instanceErr != nil || instanceState.Locked {
		s.sendError(wsCtx, msg.ID, "instance_locked", "instance is locked")
		return
	}

	connectionID := wsCtx.ConnectionID
	state, err := s.getConnection(ctx, connectionID)
	if err != nil {
		s.logger.Warn("failed to load connection context",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		s.sendError(wsCtx, msg.ID, "unauthorized", "connection context not found")
		return
	}
	if !connectionInitialized(state) {
		s.sendError(wsCtx, msg.ID, "unauthorized", "connection_init has not been acknowledged")
		return
	}
	if connectionCredentialExpired(state, time.Now()) {
		s.sendOperationCredentialExpiredError(wsCtx, msg.ID)
		return
	}

	if s.exec == nil {
		s.logger.Error("graphql executor not initialized")
		s.sendError(wsCtx, msg.ID, "internal_error", "GraphQL executor unavailable")
		return
	}

	var payload subscribePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			s.logger.Warn("failed to parse subscription payload",
				zap.String("connection_id", connectionID),
				zap.Error(err))
			s.sendError(wsCtx, msg.ID, "invalid_payload", "subscription payload could not be parsed")
			return
		}
	}

	if strings.TrimSpace(payload.Query) == "" {
		s.sendError(wsCtx, msg.ID, "invalid_request", "subscription payload must include a query")
		return
	}

	s.ensureSubscriptionManagerStarted()

	baseCtx := s.buildRequestContext(ctx, state, connectionID)
	baseCtx = graphql.StartOperationTrace(baseCtx)
	baseCtx = graph.WithConnectionID(baseCtx, connectionID)

	start := graphql.Now()
	params := &graphql.RawParams{
		Query:         payload.Query,
		OperationName: payload.OperationName,
		Variables:     payload.Variables,
		Extensions:    payload.Extensions,
	}
	params.ReadTime = graphql.TraceTiming{Start: start, End: graphql.Now()}

	opCtx, gqlErrs := s.exec.CreateOperationContext(baseCtx, params)
	if len(gqlErrs) > 0 {
		s.logger.Warn("failed to create operation context",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", msg.ID),
			zap.Errors("errors", convertErrors(gqlErrs)))
		s.sendGraphQLErrors(wsCtx, msg.ID, gqlErrs)
		return
	}

	if opCtx.Operation == nil || opCtx.Operation.Operation != ast.Subscription {
		s.logger.Warn("operation is not a subscription",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", msg.ID))
		s.sendError(wsCtx, msg.ID, "invalid_operation", "operation must be a subscription")
		return
	}

	// Serverless subscription handling: for `conversationUpdates`, persist the client-provided
	// subscribe.id to DynamoDB so out-of-band processors can post `next` frames to the connection.
	//
	// Note: gqlgen's in-process channel-based subscriptions are not reliable in API Gateway WebSocket Lambdas.
	rootField := ""
	for _, sel := range opCtx.Operation.SelectionSet {
		if field, ok := sel.(*ast.Field); ok {
			rootField = field.Name
			break
		}
	}

	if rootField == "conversationUpdates" && s.gqlSubRepo != nil {
		if !connectionAuthenticated(state) {
			s.sendOperationAuthenticationError(wsCtx, msg.ID, "authentication required for conversation updates")
			return
		}
		// Replace any prior records for this subscription id (defensive in case of retries).
		_ = s.gqlSubRepo.DeleteSubscription(ctx, connectionID, msg.ID)

		streams := []string{
			streaming.DMInboxStreamName(state.username),
			streaming.DMRequestsStreamName(state.username),
		}

		for _, streamName := range streams {
			record := &models.GraphQLStreamSubscription{
				ConnectionID:   connectionID,
				SubscriptionID: msg.ID,
				Stream:         streamName,
				Field:          rootField,
				UserID:         state.username,
			}

			if err := s.gqlSubRepo.Put(ctx, record); err != nil {
				s.logger.Warn("failed to persist graphql stream subscription",
					zap.String("connection_id", connectionID),
					zap.String("subscription_id", msg.ID),
					zap.String("stream", streamName),
					zap.Error(err))
				s.sendError(wsCtx, msg.ID, "internal_error", "failed to register subscription")
				return
			}
		}

		s.logger.Info("registered serverless subscription",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", msg.ID),
			zap.String("field", rootField),
			zap.String("user", state.username))
		return
	}

	baseCtx = graphql.WithOperationContext(baseCtx, opCtx)
	baseCtx = graph.WithConnectionID(baseCtx, connectionID)
	subscriptionCtx, cancel := context.WithCancel(baseCtx)
	if !s.addSubscription(connectionID, msg.ID, cancel) {
		cancel()
		s.sendError(wsCtx, msg.ID, "connection_closed", "connection no longer active")
		return
	}

	s.logger.Info("starting subscription", zap.String("connection_id", connectionID), zap.String("subscription_id", msg.ID))

	go s.executeSubscription(subscriptionCtx, connectionID, msg.ID, opCtx, cancel, wsCtx)
}

func (s *wsServer) sendError(wsCtx *apptheory.WebSocketContext, id, code, message string) {
	if strings.TrimSpace(id) == "" {
		id = protocolErrorID
	}
	s.sendGraphQLErrors(wsCtx, id, gqlerror.List{&gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code":        structuredErrorCode(code),
			"legacy_code": code,
		},
	}})
}

func (s *wsServer) sendConnectionACK(wsCtx *apptheory.WebSocketContext, authenticated bool) error {
	return s.sendJSON(wsCtx, responseEnvelope{
		Type: "connection_ack",
		Payload: map[string]any{
			"extensions": map[string]any{"authenticated": authenticated},
		},
	})
}

func structuredErrorCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "unauthorized":
		return wsCodeUnauthenticated
	case "forbidden":
		return "FORBIDDEN"
	case "internal_error":
		return "INTERNAL_ERROR"
	case "instance_locked":
		return "INSTANCE_LOCKED"
	case "connection_closed":
		return "CONNECTION_CLOSED"
	case "invalid_request", "invalid_payload", "invalid_operation":
		return "BAD_REQUEST"
	default:
		return strings.ToUpper(strings.TrimSpace(code))
	}
}

func (s *wsServer) sendOperationAuthenticationError(wsCtx *apptheory.WebSocketContext, id, message string) {
	s.sendGraphQLErrors(wsCtx, id, gqlerror.List{&gqlerror.Error{
		Message:    message,
		Extensions: map[string]any{"code": wsCodeUnauthenticated},
	}})
}

func (s *wsServer) sendOperationCredentialExpiredError(wsCtx *apptheory.WebSocketContext, id string) {
	s.sendGraphQLErrors(wsCtx, id, gqlerror.List{&gqlerror.Error{
		Message:    "credential expired; re-authentication required",
		Extensions: map[string]any{"code": wsCodeCredentialExpired},
	}})
}

func connectionCredentialExpired(state *connectionState, now time.Time) bool {
	if state == nil || state.claims == nil || state.claims.ExpiresAt == nil {
		return false
	}
	return !now.Before(state.claims.ExpiresAt.Time)
}

func (s *wsServer) sendGraphQLErrors(wsCtx *apptheory.WebSocketContext, id string, errs gqlerror.List) {
	payload := make([]*gqlerror.Error, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			payload = append(payload, e)
		}
	}
	if len(payload) == 0 {
		payload = append(payload, &gqlerror.Error{Message: "unknown error"})
	}
	env := responseEnvelope{
		ID:      id,
		Type:    "error",
		Payload: payload,
	}
	_ = s.sendJSON(wsCtx, env)
}

func (s *wsServer) sendGraphQLResponse(wsCtx *apptheory.WebSocketContext, id string, resp *graphql.Response) error {
	payload := make(map[string]interface{})

	if resp != nil {
		if len(resp.Data) > 0 {
			var data interface{}
			if err := json.Unmarshal(resp.Data, &data); err != nil {
				return fmt.Errorf("failed to decode graphql data: %w", err)
			}
			payload["data"] = data
		} else {
			payload["data"] = nil
		}

		if len(resp.Errors) > 0 {
			payload["errors"] = resp.Errors
		}
		if resp.Extensions != nil {
			payload["extensions"] = resp.Extensions
		}
		if resp.HasNext != nil {
			payload["hasNext"] = resp.HasNext
		}
		if len(resp.Path) > 0 {
			payload["path"] = resp.Path
		}
		if resp.Label != "" {
			payload["label"] = resp.Label
		}
	} else {
		payload["data"] = nil
	}

	return s.sendJSON(wsCtx, responseEnvelope{ID: id, Type: "next", Payload: payload})
}

func (s *wsServer) executeSubscription(ctx context.Context, connectionID, subscriptionID string, opCtx *graphql.OperationContext, cancel context.CancelFunc, wsCtx *apptheory.WebSocketContext) {
	terminalError := false
	cleanup := sync.OnceFunc(func() {
		_ = s.clearSubscription(ctx, connectionID, subscriptionID, false)
		cancel()
	})
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("subscription panic: %v", r)
			s.logger.Error("graphql subscription panicked",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			cleanup()
			s.sendGraphQLErrors(wsCtx, subscriptionID, gqlerror.List{gqlerror.Errorf("%v", err)})
			terminalError = true
		}

		cleanup()

		if terminalError {
			return
		}
		if err := s.sendJSON(wsCtx, responseEnvelope{ID: subscriptionID, Type: "complete"}); err != nil {
			s.logger.Warn("failed to send subscription completion",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
		}
	}()

	responses, respCtx := s.exec.DispatchOperation(ctx, opCtx)
	for {
		if ctx.Err() != nil {
			return
		}

		response := responses(respCtx)
		if response == nil {
			return
		}
		if hasTerminalAuthorizationError(response.Errors) {
			// An Error frame terminates the operation. Remove the operation before
			// publishing that frame so a client can never observe a terminal error
			// while the subscription is still registered.
			cleanup()
			s.sendGraphQLErrors(wsCtx, subscriptionID, response.Errors)
			terminalError = true
			return
		}

		if err := s.sendGraphQLResponse(wsCtx, subscriptionID, response); err != nil {
			s.logger.Warn("failed to send subscription response",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			return
		}
	}
}

func hasTerminalAuthorizationError(errs gqlerror.List) bool {
	for _, gqlErr := range errs {
		if gqlErr == nil || gqlErr.Extensions == nil {
			continue
		}
		code, ok := gqlErr.Extensions["code"].(string)
		if !ok {
			continue
		}
		if code == wsCodeUnauthenticated {
			return true
		}
		status := apperrors.ErrorCode(code).GetHTTPStatusCode()
		if status == 401 || status == 403 {
			return true
		}
	}
	return false
}

func connectionAuthenticated(state *connectionState) bool {
	return state != nil && state.username != "" && state.claims != nil && state.claims.GetUsername() != ""
}

func connectionInitialized(state *connectionState) bool {
	return state != nil && (state.initialized || connectionAuthenticated(state))
}

// initializeManualServices is retained for legacy unit coverage of manual
// bootstrap behavior; production startup now uses pkg/lambdastorage.
//
//nolint:unused
func initializeManualServices() {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = appconfig.Get()
	}

	configRegion := strings.TrimSpace(cfg.Region)
	envRegion := strings.TrimSpace(os.Getenv("AWS_REGION"))
	envDefaultRegion := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	tableName := strings.TrimSpace(cfg.DynamoTableName)

	logger.Info("falling back to manual service initialization for graphql-ws",
		zap.String("config_region", configRegion),
		zap.String("env_region", envRegion),
		zap.String("env_default_region", envDefaultRegion),
		zap.String("table_name", tableName))

	if configRegion == "" {
		switch {
		case envRegion != "":
			configRegion = envRegion
		case envDefaultRegion != "":
			configRegion = envDefaultRegion
		default:
			configRegion = "us-east-1"
		}
		cfg.Region = configRegion
	}

	if envRegion == "" && configRegion != "" {
		_ = os.Setenv("AWS_REGION", configRegion)
	}
	if envDefaultRegion == "" && configRegion != "" {
		_ = os.Setenv("AWS_DEFAULT_REGION", configRegion)
	}

	if cfg.DynamoTableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required for graphql-ws lambda")
	}

	logger.Info("initializing DynamORM client for graphql-ws",
		zap.String("resolved_region", cfg.Region),
		zap.String("resolved_table", cfg.DynamoTableName),
		zap.String("AWS_REGION_env", os.Getenv("AWS_REGION")),
		zap.String("AWS_DEFAULT_REGION_env", os.Getenv("AWS_DEFAULT_REGION")))

	client, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM client", zap.Error(err))
	}

	repoFactory, err := newRepositoryFactoryFn(client, cfg.DynamoTableName, logger)
	if err != nil {
		logger.Fatal("failed to create repository factory", zap.Error(err))
	}

	lambdaCtx.DynamoDB = client
	repos = repoFactory
	lambdaCtx.Repos = repoFactory

	logger.Info("manual repository initialization complete",
		zap.String("table_name", cfg.DynamoTableName),
		zap.String("region", cfg.Region))
}

func extractServices() {
	if lambdaCtx == nil {
		return
	}

	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	if lambdaCtx.Repos != nil {
		if storage, ok := lambdaCtx.Repos.(core.RepositoryStorage); ok {
			repos = storage
		}
	}
}

func initializeOAuth() {
	if cfg == nil || repos == nil {
		logger.Fatal("graphql-ws initialization missing required configuration")
	}

	jwtSecret := resolveJWTSecretForGraphQLWS()
	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	oauth = auth.NewOAuthService(jwtSecret, cfg, repos, auditLogger)
}

func resolveJWTSecretForGraphQLWS() string {
	if cfg == nil {
		return ""
	}
	jwtSecret, err := cfg.ResolveJWTSecret()
	if err != nil {
		logger.Fatal("failed to resolve graphql-ws JWT secret", zap.Error(err))
	}
	if jwtSecret == "" {
		logger.Fatal("JWT secret is not configured for graphql-ws")
	}
	return jwtSecret
}

func initializeResolver() (*graph.Resolver, *executor.Executor) {
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.BaseURL(),
		JWTSecret: resolveJWTSecretForGraphQLWS(),
		Config:    cfg,
	}

	streamQueue := resolveStreamQueue()
	if streamQueue == nil {
		logger.Fatal("stream queue not available for graphql-ws")
	}
	publisher := streaming.NewQueuePublisher(streamQueue, logger)

	registry, err := services.NewRegistry(
		services.WithStorage(repos),
		services.WithPublisher(publisher),
		services.WithLogger(logger),
		services.WithConfig(serviceConfig),
	)
	if err != nil {
		logger.Fatal("failed to initialize service registry", zap.Error(err))
	}

	resolver := &graph.Resolver{
		Registry:  registry,
		Storage:   repos,
		Config:    cfg,
		Logger:    logger,
		TableName: cfg.DynamoTableName,
	}

	// Create WebSocket subscription manager with DynamoDB-backed persistence (reuse existing publisher)
	resolver.SubscriptionManager = graph.NewSubscriptionManager(
		repos.StreamingConnection(),
		publisher, // Reuse publisher created above
		logger,
	)

	schema := graph.NewExecutableSchema(graph.Config{Resolvers: resolver})
	exec := executor.New(schema)
	configureGraphQLExecutor(exec, cfg)

	return resolver, exec
}

func configureGraphQLExecutor(exec *executor.Executor, cfg *appconfig.Config) {
	if exec == nil {
		return
	}
	exec.SetErrorPresenter(graphQLWSErrorPresenter)
	if cfg == nil {
		return
	}

	if cfg.GraphQLParserTokenLimit > 0 {
		exec.SetParserTokenLimit(cfg.GraphQLParserTokenLimit)
	}

	// Depth enforcement: agents and CLI automation tokens are restricted to shallow queries (max depth 3),
	// humans use configured depth.
	defaultDepth := 0
	if cfg.GraphQLMaxDepth > 0 {
		defaultDepth = cfg.GraphQLMaxDepth
	}
	exec.Use(&gqllimits.DepthLimit{
		Func: func(ctx context.Context, _ *graphql.OperationContext) int {
			if claimsVal := ctx.Value(common.ContextKeyClaims); claimsVal != nil {
				if claims, ok := claimsVal.(*auth.Claims); ok && (claims.IsAgent || strings.EqualFold(claims.ClientClass, auth.ClientClassCLI)) {
					return 3
				}
			}
			return defaultDepth
		},
	})

	if cfg.GraphQLMaxComplexity > 0 {
		exec.Use(extension.FixedComplexityLimit(cfg.GraphQLMaxComplexity))
	}

	// Introspection is disabled by default; enable it explicitly for debug/playground workflows.
	if cfg.DebugMode || cfg.EnablePlayground || cfg.GraphQLAllowIntrospection {
		exec.Use(extension.Introspection{})
	}
}

func graphQLWSErrorPresenter(ctx context.Context, err error) *gqlerror.Error {
	gqlErr := graphql.DefaultErrorPresenter(ctx, err)
	if appErr, ok := apperrors.AsAppError(err); ok {
		if gqlErr.Extensions == nil {
			gqlErr.Extensions = map[string]any{}
		}
		code := string(appErr.Code)
		if appErr.Code == apperrors.CodeUnauthorized {
			code = wsCodeUnauthenticated
		}
		gqlErr.Extensions["code"] = code
		gqlErr.Extensions["http_status"] = appErr.HTTPStatusCode
		gqlErr.Message = appErr.Message
	}
	return gqlErr
}

func initializeConnectionRepository() {
	if cfg == nil || logger == nil {
		return
	}

	var dynamo dynamormCore.DB
	if lambdaCtx != nil {
		if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok && db != nil {
			dynamo = db
		}
	}

	if dynamo == nil && repos != nil {
		dynamo = repos.GetDB()
	}

	if dynamo == nil {
		logger.Warn("graphql-ws lambda missing dynamo client for connection repository")
		return
	}

	connectionsTable := cfg.DynamoTableName
	if tableOverride := strings.TrimSpace(cfg.ConnectionsTable); tableOverride != "" {
		connectionsTable = tableOverride
	}

	subscriptionsTable := cfg.DynamoTableName
	if tableOverride := strings.TrimSpace(cfg.SubscriptionsTable); tableOverride != "" {
		subscriptionsTable = tableOverride
	}

	connectionRepo = repositories.NewStreamingConnectionRepository(dynamo, connectionsTable, dynamo, subscriptionsTable, logger, nil)
	gqlSubRepo = repositories.NewGraphQLStreamSubscriptionRepository(dynamo, subscriptionsTable, logger)
}

func resolveStreamQueue() streaming.StreamQueueService {
	if lambdaCtx != nil && lambdaCtx.StreamQueue != nil {
		if sq, ok := lambdaCtx.StreamQueue.(streaming.StreamQueueService); ok {
			return sq
		}
	}

	var coreDB dynamormCore.DB
	if lambdaCtx != nil && lambdaCtx.DynamoDB != nil {
		if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok {
			coreDB = db
		}
	}

	if coreDB == nil && repos != nil {
		coreDB = repos.GetDB()
	}

	if coreDB == nil {
		client, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
		if err != nil {
			logger.Error("failed to initialize dynamo client for stream queue",
				zap.String("region", cfg.Region),
				zap.Error(err))
			return nil
		}
		coreDB = client
	}

	return streaming.NewDynamoStreamQueue(coreDB, cfg.DynamoTableName, logger)
}

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
	lambdaStartFn = lambda.Start
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeGraphQLWS()
}

func initializeGraphQLWS() {
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: graphqlWSName,
		LambdaType:  common.LambdaTypeBasic,
	})

	extractServices()

	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:          graphqlWSName,
		RequireRepositories:  true,
		NewDB:                newLambdaOptimizedClientFn,
		NewRepositoryStorage: newRepositoryFactoryFn,
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
	repos = deps.Repos
	extractServices()

	if cfg != nil && os.Getenv("JWT_SECRET") == "" {
		if jwtSecret := resolveJWTSecretForGraphQLWS(); jwtSecret != "" {
			if err := os.Setenv("JWT_SECRET", jwtSecret); err != nil {
				logger.Warn("failed to propagate JWT secret to environment", zap.Error(err))
			}
		}
	}

	initializeOAuth()
	resolver, exec := initializeResolver()
	initializeConnectionRepository()

	var instanceRepo instanceStateRepo
	if repos != nil {
		instanceRepo = repos.Instance()
	}
	server = newServer(oauth, resolver, exec, logger, connectionRepo, gqlSubRepo, instanceRepo)

	logger.Info("graphql-ws lambda initialized")
}

func main() {
	app := apptheory.New()
	app.WebSocket("$connect", server.handleConnect)
	app.WebSocket("$disconnect", server.handleDisconnect)
	app.WebSocket("$default", server.handleDefault)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}
