// Package main implements the GraphQL WebSocket Lambda entrypoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

type connectionState struct {
	username      string
	claims        *auth.Claims
	subscriptions map[string]*subscriptionState
}

type subscriptionState struct {
	cancel context.CancelFunc
}

type wsServer struct {
	oauthService *auth.OAuthService
	logger       *zap.Logger
	resolver     *graph.Resolver
	exec         *executor.Executor
	startOnce    sync.Once
	connRepo     *repositories.StreamingConnectionRepository

	mu          sync.RWMutex
	connections map[string]*connectionState
	wsContexts  map[string]*lift.WebSocketContext // Store WebSocket contexts for message sending
}

type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type subscribePayload struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
	Extensions    map[string]interface{} `json:"extensions,omitempty"`
}

type errorPayload struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
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
	server         *wsServer
)

func newServer(oauthService *auth.OAuthService, resolver *graph.Resolver, exec *executor.Executor, log *zap.Logger, connRepo *repositories.StreamingConnectionRepository) *wsServer {
	if log == nil {
		log = zap.NewNop()
	}

	return &wsServer{
		oauthService: oauthService,
		logger:       log,
		resolver:     resolver,
		exec:         exec,
		connRepo:     connRepo,
		connections:  make(map[string]*connectionState),
		wsContexts:   make(map[string]*lift.WebSocketContext),
	}
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
			connectionRecord.Info.Protocol = "graphql-ws"
			connectionRecord.Info.AuthMethod = "oauth"
			if connectionRecord.Info.CustomHeaders == nil {
				connectionRecord.Info.CustomHeaders = make(map[string]string)
			}
			if claims != nil && len(claims.Scopes) > 0 {
				connectionRecord.Info.CustomHeaders["scopes"] = strings.Join(claims.Scopes, " ")
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
	s.connections[connectionID] = &connectionState{
		username:      username,
		claims:        claims,
		subscriptions: make(map[string]*subscriptionState),
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

	claims := &auth.Claims{
		Username: connection.Username,
		Scopes:   scopes,
	}

	state = &connectionState{
		username:      connection.Username,
		claims:        claims,
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
	if s.resolver == nil || s.resolver.SubscriptionManager == nil {
		return
	}

	s.startOnce.Do(func() {
		if s.resolver.SubscriptionManager.IsRunning() {
			return
		}
		if err := s.resolver.SubscriptionManager.Start(context.Background()); err != nil {
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

// Lift adapter handlers - use Lift framework's WebSocket support
func (s *wsServer) handleConnectLift(ctx *lift.Context) error {
	wsCtx, err := ctx.AsWebSocket()
	if err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid WebSocket event", 400)
	}

	connectionID := wsCtx.ConnectionID()
	log := s.logger.With(
		zap.String("connection_id", connectionID),
		zap.String("route", "$connect"),
	)

	// Use Lift's ctx.Query() method exactly like penny-lift does
	// Priority order: access_token (Mastodon standard) > token (alternative)
	rawAccessToken := strings.TrimSpace(ctx.Query("access_token"))
	sanitizedAccessToken := cleanToken(rawAccessToken)
	rawToken := strings.TrimSpace(ctx.Query("token"))
	sanitizedToken := cleanToken(rawToken)
	log.Info("graphql ws token probes",
		zap.Bool("has_access_token_param", rawAccessToken != ""),
		zap.Bool("has_token_param", rawToken != ""),
		zap.Int("query_param_count", len(ctx.Request.QueryParams)),
		zap.String("access_token_preview", previewToken(sanitizedAccessToken)))

	tokenValue := sanitizedAccessToken
	if tokenValue == "" {
		tokenValue = sanitizedToken
	}
	if tokenValue == "" {
		authHeader := ctx.Header("Authorization")
		tokenValue = normalizeAuthToken(authHeader)
	}
	if tokenValue == "" {
		authHeaderLower := ctx.Header("authorization")
		tokenValue = normalizeAuthToken(authHeaderLower)
	}

	if tokenValue == "" {
		log.Warn("websocket connect missing authentication token",
			zap.Any("query_params", ctx.Request.QueryParams),
			zap.Any("headers", ctx.Request.Headers))
		return lift.NewLiftError("UNAUTHORIZED", "Access token required", 401)
	}

	claims, err := s.oauthService.ValidateAccessToken(tokenValue)
	if err != nil {
		log.Warn("websocket connect failed token validation",
			zap.Error(err))
		return lift.NewLiftError("UNAUTHORIZED", "Invalid or expired token", 401)
	}

	username := claims.GetUsername()
	if username == "" {
		username = claims.Username
	}

	if username == "" {
		log.Warn("websocket connect missing username in claims")
		return lift.NewLiftError("FORBIDDEN", "Missing username in token claims", 403)
	}

	// Store WebSocket context for later message sending
	s.mu.Lock()
	s.wsContexts[connectionID] = wsCtx
	s.mu.Unlock()

	// Register connection
	if err := s.registerConnection(ctx.Request.Context(), connectionID, username, claims); err != nil {
		log.Error("failed to persist websocket connection",
			zap.String("username", username),
			zap.Error(err))

		// Clean up stored context on error
		s.mu.Lock()
		delete(s.wsContexts, connectionID)
		s.mu.Unlock()

		return lift.NewLiftError("INTERNAL_ERROR", "Failed to persist connection", 500)
	}

	log.Info("websocket connection established",
		zap.String("username", username))

	return nil
}

func (s *wsServer) handleDisconnectLift(ctx *lift.Context) error {
	wsCtx, err := ctx.AsWebSocket()
	if err != nil {
		return nil // Ignore errors on disconnect
	}

	connectionID := wsCtx.ConnectionID()

	// Clean up stored WebSocket context
	s.mu.Lock()
	delete(s.wsContexts, connectionID)
	s.mu.Unlock()

	s.removeConnection(ctx.Request.Context(), connectionID)
	s.logger.Info("websocket connection closed",
		zap.String("connection_id", connectionID))
	return nil
}

func (s *wsServer) handleDefaultLift(ctx *lift.Context) error {
	wsCtx, err := ctx.AsWebSocket()
	if err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid WebSocket event", 400)
	}

	connectionID := wsCtx.ConnectionID()

	// Store WebSocket context for async message sending (subscriptions)
	s.mu.Lock()
	s.wsContexts[connectionID] = wsCtx
	s.mu.Unlock()

	// Parse message body
	var msg wsMessage
	if err := ctx.ParseRequest(&msg); err != nil {
		s.logger.Warn("failed to parse websocket message",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		return lift.NewLiftError("BAD_REQUEST", "Failed to parse message body", 400)
	}

	switch strings.ToLower(msg.Type) {
	case "connection_init":
		s.logger.Info("received connection_init",
			zap.String("connection_id", connectionID))
		return wsCtx.SendJSONMessage(responseEnvelope{Type: "connection_ack"})
	case "ping":
		return wsCtx.SendJSONMessage(responseEnvelope{Type: "pong"})
	case "subscribe":
		s.handleSubscribeWithLift(ctx.Request.Context(), msg, wsCtx)
		return nil
	case "complete":
		s.logger.Info("received completion request",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", msg.ID))
		if !s.cancelSubscription(ctx.Request.Context(), connectionID, msg.ID) {
			_ = wsCtx.SendJSONMessage(responseEnvelope{
				ID:   msg.ID,
				Type: "complete",
			})
		}
		return nil
	default:
		s.logger.Warn("received unsupported websocket message type",
			zap.String("connection_id", connectionID),
			zap.String("type", msg.Type))
		return lift.NewLiftError("BAD_REQUEST", fmt.Sprintf("message type %q is not supported", msg.Type), 400)
	}
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

func previewToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 10 {
		return token
	}
	return token[:5] + "..." + token[len(token)-5:]
}

// handleSubscribeWithLift handles subscription requests using Lift's WebSocket context
func (s *wsServer) handleSubscribeWithLift(ctx context.Context, msg wsMessage, wsCtx *lift.WebSocketContext) {
	if msg.ID == "" {
		s.sendErrorViaLift(wsCtx, "", "invalid_request", "subscription messages must include an id")
		return
	}

	connectionID := wsCtx.ConnectionID()
	state, err := s.getConnection(ctx, connectionID)
	if err != nil {
		s.logger.Warn("failed to load connection context",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		s.sendErrorViaLift(wsCtx, msg.ID, "unauthorized", "connection context not found")
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
		return
	}

	if s.exec == nil {
		s.logger.Error("graphql executor not initialized")
		s.sendErrorViaLift(wsCtx, msg.ID, "internal_error", "GraphQL executor unavailable")
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
		return
	}

	var payload subscribePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			s.logger.Warn("failed to parse subscription payload",
				zap.String("connection_id", connectionID),
				zap.Error(err))
			s.sendErrorViaLift(wsCtx, msg.ID, "invalid_payload", "subscription payload could not be parsed")
			_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
			return
		}
	}

	if strings.TrimSpace(payload.Query) == "" {
		s.sendErrorViaLift(wsCtx, msg.ID, "invalid_request", "subscription payload must include a query")
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
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
		s.sendGraphQLErrorsViaLift(wsCtx, msg.ID, gqlErrs)
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
		return
	}

	if opCtx.Operation == nil || opCtx.Operation.Operation != ast.Subscription {
		s.logger.Warn("operation is not a subscription",
			zap.String("connection_id", connectionID),
			zap.String("subscription_id", msg.ID))
		s.sendErrorViaLift(wsCtx, msg.ID, "invalid_operation", "operation must be a subscription")
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
		return
	}

	baseCtx = graphql.WithOperationContext(baseCtx, opCtx)
	baseCtx = graph.WithConnectionID(baseCtx, connectionID)
	subscriptionCtx, cancel := context.WithCancel(baseCtx)
	if !s.addSubscription(connectionID, msg.ID, cancel) {
		cancel()
		s.sendErrorViaLift(wsCtx, msg.ID, "connection_closed", "connection no longer active")
		_ = wsCtx.SendJSONMessage(responseEnvelope{ID: msg.ID, Type: "complete"})
		return
	}

	s.logger.Info("starting subscription", zap.String("connection_id", connectionID), zap.String("subscription_id", msg.ID))

	go s.executeSubscriptionWithLift(subscriptionCtx, connectionID, msg.ID, opCtx, cancel, wsCtx)
}

// Helper methods for sending messages via Lift WebSocket context
func (s *wsServer) sendErrorViaLift(wsCtx *lift.WebSocketContext, id, code, message string) {
	payload := errorPayload{
		Message: message,
		Code:    code,
	}
	env := responseEnvelope{
		ID:      id,
		Type:    "error",
		Payload: payload,
	}
	_ = wsCtx.SendJSONMessage(env)
}

func (s *wsServer) sendGraphQLErrorsViaLift(wsCtx *lift.WebSocketContext, id string, errs gqlerror.List) {
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
	_ = wsCtx.SendJSONMessage(env)
}

func (s *wsServer) sendGraphQLResponseViaLift(wsCtx *lift.WebSocketContext, id string, resp *graphql.Response) error {
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

	return wsCtx.SendJSONMessage(responseEnvelope{ID: id, Type: "next", Payload: payload})
}

func (s *wsServer) executeSubscriptionWithLift(ctx context.Context, connectionID, subscriptionID string, opCtx *graphql.OperationContext, cancel context.CancelFunc, wsCtx *lift.WebSocketContext) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("subscription panic: %v", r)
			s.logger.Error("graphql subscription panicked",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			s.sendGraphQLErrorsViaLift(wsCtx, subscriptionID, gqlerror.List{gqlerror.Errorf("%v", err)})
		}

		_ = s.clearSubscription(ctx, connectionID, subscriptionID, false)
		cancel()

		if err := wsCtx.SendJSONMessage(responseEnvelope{ID: subscriptionID, Type: "complete"}); err != nil {
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

		if err := s.sendGraphQLResponseViaLift(wsCtx, subscriptionID, response); err != nil {
			s.logger.Warn("failed to send subscription response",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			return
		}
	}
}

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

	client, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM client", zap.Error(err))
	}

	repoFactory, err := factory.NewRepositoryFactory(client, cfg.DynamoTableName, logger)
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

	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	oauth = auth.NewOAuthService(cfg.JWTSecret, cfg, repos, auditLogger)
}

func initializeResolver() (*graph.Resolver, *executor.Executor) {
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.BaseURL(),
		JWTSecret: cfg.JWTSecret,
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

	return resolver, exec
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
		client, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
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

func init() {
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "graphql-ws",
		LambdaType:  common.LambdaTypeBasic,
	})

	extractServices()

	if err := lambdaCtx.InitializeWithDefaults(); err != nil {
		initializeManualServices()
	} else {
		extractServices()
	}

	if repos == nil {
		initializeManualServices()
		extractServices()
	}

	if cfg != nil && cfg.JWTSecret != "" && os.Getenv("JWT_SECRET") == "" {
		if err := os.Setenv("JWT_SECRET", cfg.JWTSecret); err != nil {
			logger.Warn("failed to propagate JWT secret to environment", zap.Error(err))
		}
	}

	initializeOAuth()
	resolver, exec := initializeResolver()
	initializeConnectionRepository()

	server = newServer(oauth, resolver, exec, logger, connectionRepo)

	logger.Info("graphql-ws lambda initialized")
}

// liftLoggerAdapter adapts zap.Logger to Lift's Logger interface
type liftLoggerAdapter struct {
	logger *zap.Logger
}

func (l *liftLoggerAdapter) Debug(msg string, fields ...map[string]any) {
	l.logger.Debug(msg, mergeFieldsToZapFields(fields)...)
}

func (l *liftLoggerAdapter) Info(msg string, fields ...map[string]any) {
	l.logger.Info(msg, mergeFieldsToZapFields(fields)...)
}

func (l *liftLoggerAdapter) Warn(msg string, fields ...map[string]any) {
	l.logger.Warn(msg, mergeFieldsToZapFields(fields)...)
}

func (l *liftLoggerAdapter) Error(msg string, fields ...map[string]any) {
	l.logger.Error(msg, mergeFieldsToZapFields(fields)...)
}

func (l *liftLoggerAdapter) WithFields(fields map[string]any) lift.Logger {
	return &liftLoggerAdapter{
		logger: l.logger.With(mapToZapFields(fields)...),
	}
}

func (l *liftLoggerAdapter) WithField(key string, value any) lift.Logger {
	return &liftLoggerAdapter{
		logger: l.logger.With(zap.Any(key, value)),
	}
}

func mergeFieldsToZapFields(fields []map[string]any) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	// Merge all field maps into one
	merged := make(map[string]any)
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			merged[k] = v
		}
	}
	return mapToZapFields(merged)
}

func mapToZapFields(fields map[string]any) []zap.Field {
	if fields == nil {
		return nil
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return zapFields
}

func main() {
	// Create Lift app with WebSocket support
	app := lift.New(lift.WithWebSocketSupport())

	// Set up logger if available
	if logger != nil {
		app.WithLogger(&liftLoggerAdapter{logger: logger})
	}

	// Register WebSocket routes using Lift's adapter
	app.WebSocket("$connect", server.handleConnectLift)
	app.WebSocket("$disconnect", server.handleDisconnectLift)
	app.WebSocket("$default", server.handleDefaultLift)

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}
