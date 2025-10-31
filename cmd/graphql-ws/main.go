// Package main implements the GraphQL WebSocket Lambda entrypoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
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
	awsCfg         aws.Config
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
	}
}

func (s *wsServer) registerConnection(ctx context.Context, connectionID, username string, claims *auth.Claims) error {
	var persistErr error

	if s.connRepo != nil {
		streams := []string{"graphql"}
		if err := s.connRepo.WriteConnection(ctx, connectionID, username, username, streams); err != nil {
			s.logger.Warn("failed to write graphql connection record",
				zap.String("connection_id", connectionID),
				zap.String("username", username),
				zap.Error(err),
				zap.String("error_details", err.Error()))
		}

		connection, err := s.connRepo.GetConnection(ctx, connectionID)
		if err != nil {
			s.logger.Warn("failed to fetch graphql connection",
				zap.String("connection_id", connectionID),
				zap.Error(err),
				zap.String("error_details", err.Error()))
			if persistErr == nil {
				persistErr = err
			}
		} else {
			connection.Streams = streams
			connection.Username = username
			connection.UserID = username
			connection.Info.Protocol = "graphql-ws"
			connection.Info.AuthMethod = "oauth"
			if connection.Info.CustomHeaders == nil {
				connection.Info.CustomHeaders = make(map[string]string)
			}
			if claims != nil && len(claims.Scopes) > 0 {
				connection.Info.CustomHeaders["scopes"] = strings.Join(claims.Scopes, " ")
			}
			connection.LastActivity = time.Now()
			connection.Established = time.Now()

			if err := s.connRepo.UpdateConnection(ctx, connection); err != nil {
				s.logger.Warn("failed to update graphql connection metadata",
					zap.String("connection_id", connectionID),
					zap.Error(err))
				if persistErr == nil {
					persistErr = err
				}
			}

			if err := s.connRepo.UpdateConnectionState(ctx, connectionID, models.ConnectionStateConnected, ""); err != nil {
				s.logger.Warn("failed to update graphql connection state",
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

func (s *wsServer) sendGraphQLErrors(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, id string, errs gqlerror.List) {
	payload := make([]*gqlerror.Error, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			payload = append(payload, e)
		}
	}
	if len(payload) == 0 {
		payload = append(payload, &gqlerror.Error{Message: "unknown error"})
	}

	if err := s.sendMessage(ctx, event, responseEnvelope{ID: id, Type: "error", Payload: payload}); err != nil {
		s.logger.Warn("failed to send graphql subscription error",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("subscription_id", id),
			zap.Error(err))
	}
}

func (s *wsServer) sendGraphQLResponse(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, id string, resp *graphql.Response) error {
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

	return s.sendMessage(ctx, event, responseEnvelope{ID: id, Type: "next", Payload: payload})
}

func (s *wsServer) executeSubscription(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, connectionID, subscriptionID string, opCtx *graphql.OperationContext, cancel context.CancelFunc) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("subscription panic: %v", r)
			s.logger.Error("graphql subscription panicked",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			s.sendGraphQLErrors(ctx, event, subscriptionID, gqlerror.List{gqlerror.Errorf("%v", err)})
		}

		_ = s.clearSubscription(ctx, connectionID, subscriptionID, false)
		cancel()

		if err := s.sendComplete(ctx, event, subscriptionID); err != nil {
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

		if err := s.sendGraphQLResponse(ctx, event, subscriptionID, response); err != nil {
			s.logger.Warn("failed to send subscription response",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err))
			return
		}
	}
}

func (s *wsServer) handleConnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	authHeader := ""
	for key, value := range event.Headers {
		if strings.EqualFold(key, "Authorization") {
			authHeader = value
			break
		}
	}

	if authHeader == "" {
		s.logger.Warn("websocket connect missing authorization header",
			zap.String("connection_id", event.RequestContext.ConnectionID))
		return events.APIGatewayProxyResponse{StatusCode: 401}, nil
	}

	token := strings.TrimSpace(authHeader)
	token = strings.TrimPrefix(token, "Bearer ")

	claims, err := s.oauthService.ValidateAccessToken(token)
	if err != nil {
		s.logger.Warn("websocket connect failed token validation",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 401}, nil
	}

	username := claims.GetUsername()
	if username == "" {
		username = claims.Username
	}

	if username == "" {
		s.logger.Warn("websocket connect missing username in claims",
			zap.String("connection_id", event.RequestContext.ConnectionID))
		return events.APIGatewayProxyResponse{StatusCode: 403}, nil
	}

	if err := s.registerConnection(ctx, event.RequestContext.ConnectionID, username, claims); err != nil {
		s.logger.Error("failed to persist websocket connection",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("username", username),
			zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}

	s.logger.Info("websocket connection established",
		zap.String("connection_id", event.RequestContext.ConnectionID),
		zap.String("username", username))

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func (s *wsServer) handleDisconnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	s.removeConnection(ctx, event.RequestContext.ConnectionID)
	s.logger.Info("websocket connection closed",
		zap.String("connection_id", event.RequestContext.ConnectionID))
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func (s *wsServer) handleDefault(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	var msg wsMessage
	if err := json.Unmarshal([]byte(event.Body), &msg); err != nil {
		s.logger.Warn("failed to parse websocket message",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.Error(err))
		_ = s.sendError(ctx, event, "", "invalid_message", "Failed to parse message body")
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	}

	switch strings.ToLower(msg.Type) {
	case "connection_init":
		s.logger.Info("received connection_init",
			zap.String("connection_id", event.RequestContext.ConnectionID))
		_ = s.sendAck(ctx, event)
	case "ping":
		_ = s.sendPong(ctx, event)
	case "subscribe":
		s.handleSubscribe(ctx, event, msg)
	case "complete":
		s.logger.Info("received completion request",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("subscription_id", msg.ID))
		if !s.cancelSubscription(ctx, event.RequestContext.ConnectionID, msg.ID) {
			_ = s.sendComplete(ctx, event, msg.ID)
		}
	default:
		s.logger.Warn("received unsupported websocket message type",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("type", msg.Type))
		_ = s.sendError(ctx, event, msg.ID, "unsupported_operation", fmt.Sprintf("message type %q is not supported", msg.Type))
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func (s *wsServer) handleSubscribe(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, msg wsMessage) {
	if msg.ID == "" {
		_ = s.sendError(ctx, event, "", "invalid_request", "subscription messages must include an id")
		return
	}

	state, err := s.getConnection(ctx, event.RequestContext.ConnectionID)
	if err != nil {
		s.logger.Warn("failed to load connection context",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.Error(err))
		_ = s.sendError(ctx, event, msg.ID, "unauthorized", "connection context not found")
		_ = s.sendComplete(ctx, event, msg.ID)
		return
	}

	if s.exec == nil {
		s.logger.Error("graphql executor not initialized")
		_ = s.sendError(ctx, event, msg.ID, "internal_error", "GraphQL executor unavailable")
		_ = s.sendComplete(ctx, event, msg.ID)
		return
	}

	var payload subscribePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			s.logger.Warn("failed to parse subscription payload",
				zap.String("connection_id", event.RequestContext.ConnectionID),
				zap.Error(err))
			_ = s.sendError(ctx, event, msg.ID, "invalid_payload", "subscription payload could not be parsed")
			_ = s.sendComplete(ctx, event, msg.ID)
			return
		}
	}

	if strings.TrimSpace(payload.Query) == "" {
		_ = s.sendError(ctx, event, msg.ID, "invalid_request", "subscription payload must include a query")
		_ = s.sendComplete(ctx, event, msg.ID)
		return
	}

	s.ensureSubscriptionManagerStarted()

	baseCtx := s.buildRequestContext(ctx, state, event.RequestContext.ConnectionID)
	baseCtx = graphql.StartOperationTrace(baseCtx)
	baseCtx = graph.WithConnectionID(baseCtx, event.RequestContext.ConnectionID)

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
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("subscription_id", msg.ID),
			zap.Errors("errors", convertErrors(gqlErrs)))
		s.sendGraphQLErrors(baseCtx, event, msg.ID, gqlErrs)
		_ = s.sendComplete(baseCtx, event, msg.ID)
		return
	}

	if opCtx.Operation == nil || opCtx.Operation.Operation != ast.Subscription {
		s.logger.Warn("operation is not a subscription",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("subscription_id", msg.ID))
		_ = s.sendError(baseCtx, event, msg.ID, "invalid_operation", "operation must be a subscription")
		_ = s.sendComplete(baseCtx, event, msg.ID)
		return
	}

	baseCtx = graphql.WithOperationContext(baseCtx, opCtx)
	baseCtx = graph.WithConnectionID(baseCtx, event.RequestContext.ConnectionID)
	subscriptionCtx, cancel := context.WithCancel(baseCtx)
	if !s.addSubscription(event.RequestContext.ConnectionID, msg.ID, cancel) {
		cancel()
		_ = s.sendError(baseCtx, event, msg.ID, "connection_closed", "connection no longer active")
		_ = s.sendComplete(baseCtx, event, msg.ID)
		return
	}

	s.logger.Info("starting subscription", zap.String("connection_id", event.RequestContext.ConnectionID), zap.String("subscription_id", msg.ID))

	go s.executeSubscription(subscriptionCtx, event, event.RequestContext.ConnectionID, msg.ID, opCtx, cancel)
}

func (s *wsServer) sendAck(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	msg := responseEnvelope{Type: "connection_ack"}
	return s.sendMessage(ctx, event, msg)
}

func (s *wsServer) sendPong(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	msg := responseEnvelope{Type: "pong"}
	return s.sendMessage(ctx, event, msg)
}

func (s *wsServer) sendComplete(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, id string) error {
	if id == "" {
		return nil
	}
	msg := responseEnvelope{
		ID:   id,
		Type: "complete",
	}
	return s.sendMessage(ctx, event, msg)
}

func (s *wsServer) sendError(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, id, code, message string) error {
	payload := errorPayload{
		Message: message,
		Code:    code,
	}

	env := responseEnvelope{
		ID:      id,
		Type:    "error",
		Payload: payload,
	}

	return s.sendMessage(ctx, event, env)
}

func (s *wsServer) sendMessage(ctx context.Context, event events.APIGatewayWebsocketProxyRequest, message interface{}) error {
	region := "us-east-1"
	if cfg != nil && cfg.Region != "" {
		region = cfg.Region
	} else if lambdaCtx != nil && lambdaCtx.Config != nil && lambdaCtx.Config.Region != "" {
		region = lambdaCtx.Config.Region
	}

	data, err := json.Marshal(message)
	if err != nil {
		s.logger.Error("failed to marshal websocket response",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.Error(err))
		return err
	}

	var lastErr error
	for _, endpoint := range managementAPIEndpoints(event, region) {
		cfgCopy := awsCfg.Copy()
		cfgCopy.Region = region
		cfgCopy.BaseEndpoint = aws.String(endpoint)

		client := apigatewaymanagementapi.NewFromConfig(cfgCopy)

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = client.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(event.RequestContext.ConnectionID),
			Data:         data,
		})
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		s.logger.Warn("failed to send websocket message via management endpoint",
			zap.String("connection_id", event.RequestContext.ConnectionID),
			zap.String("endpoint", endpoint),
			zap.Error(err))
	}

	return lastErr
}

func managementAPIEndpoints(event events.APIGatewayWebsocketProxyRequest, region string) []string {
	stage := strings.Trim(event.RequestContext.Stage, "/")
	stageSegment := func(base string, useStageFirst bool) []string {
		base = strings.TrimRight(base, "/")
		withStage := base
		if stage != "" {
			withStage = fmt.Sprintf("%s/%s", base, stage)
		}

		if stage == "" {
			return []string{base}
		}

		if useStageFirst {
			return []string{withStage, base}
		}

		return []string{base, withStage}
	}

	var endpoints []string
	seen := make(map[string]struct{})

	addEndpoint := func(candidate string) {
		candidate = strings.TrimRight(candidate, "/")
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		endpoints = append(endpoints, candidate)
	}

	domain := strings.TrimSpace(event.RequestContext.DomainName)
	if domain != "" {
		base := fmt.Sprintf("https://%s", domain)
		useStageFirst := strings.Contains(domain, ".execute-api.")
		for _, ep := range stageSegment(base, useStageFirst) {
			addEndpoint(ep)
		}
	}

	if event.RequestContext.APIID != "" {
		defaultDomain := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", event.RequestContext.APIID, region)
		if domain == "" || !strings.EqualFold(domain, strings.TrimPrefix(defaultDomain, "https://")) {
			for _, ep := range stageSegment(defaultDomain, true) {
				addEndpoint(ep)
			}
		}
	}

	return endpoints
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

	var err error
	awsCfg, err = awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS configuration", zap.Error(err))
	}

	logger.Info("graphql-ws lambda initialized")
}

func handler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch event.RequestContext.RouteKey {
	case "$connect":
		return server.handleConnect(ctx, event)
	case "$disconnect":
		return server.handleDisconnect(ctx, event)
	default:
		return server.handleDefault(ctx, event)
	}
}

func main() {
	lambda.Start(handler)
}
