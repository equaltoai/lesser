package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/auth"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/v3/pkg/streamer"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

type fakeConnectionRepo struct {
	wroteConnections []struct {
		connectionID string
		userID       string
		username     string
		streams      []string
	}
	writeConnectionErr error
	getConnectionResp  *models.WebSocketConnection
	getConnectionErr   error
	getConnectionCalls int

	updatedConnections  []*models.WebSocketConnection
	updateConnectionErr error

	wroteSubscriptions []struct {
		connectionID string
		userID       string
		stream       string
	}
	writeSubscriptionErr error
	deletedSubscriptions []struct {
		connectionID string
		stream       string
	}
	deleteSubscriptionErr error

	deletedConnections  []string
	deleteConnectionErr error
	deleteAllSubsErr    error
}

func (f *fakeConnectionRepo) WriteConnection(_ context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error) {
	f.wroteConnections = append(f.wroteConnections, struct {
		connectionID string
		userID       string
		username     string
		streams      []string
	}{connectionID: connectionID, userID: userID, username: username, streams: streams})
	if f.writeConnectionErr != nil {
		return nil, f.writeConnectionErr
	}
	return &models.WebSocketConnection{
		ConnectionID: connectionID,
		UserID:       userID,
		Username:     username,
		Streams:      streams,
	}, nil
}

func (f *fakeConnectionRepo) DeleteConnection(_ context.Context, connectionID string) error {
	f.deletedConnections = append(f.deletedConnections, connectionID)
	return f.deleteConnectionErr
}

func (f *fakeConnectionRepo) DeleteAllSubscriptions(_ context.Context, _ string) error {
	return f.deleteAllSubsErr
}

func (f *fakeConnectionRepo) GetConnection(_ context.Context, _ string) (*models.WebSocketConnection, error) {
	f.getConnectionCalls++
	if f.getConnectionErr != nil {
		return nil, f.getConnectionErr
	}
	if f.getConnectionResp != nil {
		return f.getConnectionResp, nil
	}
	return &models.WebSocketConnection{ConnectionID: "c1"}, nil
}

func (f *fakeConnectionRepo) UpdateConnection(_ context.Context, connection *models.WebSocketConnection) error {
	f.updatedConnections = append(f.updatedConnections, connection)
	return f.updateConnectionErr
}

func (f *fakeConnectionRepo) WriteSubscription(_ context.Context, connectionID, userID, stream string) error {
	f.wroteSubscriptions = append(f.wroteSubscriptions, struct {
		connectionID string
		userID       string
		stream       string
	}{connectionID: connectionID, userID: userID, stream: stream})
	return f.writeSubscriptionErr
}

func (f *fakeConnectionRepo) DeleteSubscription(_ context.Context, connectionID, stream string) error {
	f.deletedSubscriptions = append(f.deletedSubscriptions, struct {
		connectionID string
		stream       string
	}{connectionID: connectionID, stream: stream})
	return f.deleteSubscriptionErr
}

type fakeCostTracker struct {
	calls []*repositories.WebSocketOperationContext
	err   error
}

func (f *fakeCostTracker) TrackWebSocketOperation(_ context.Context, opCtx *repositories.WebSocketOperationContext, _ *repositories.WebSocketOperationResult) error {
	f.calls = append(f.calls, opCtx)
	return f.err
}

type fakeWSClient struct {
	postErr error
	posts   []struct {
		connectionID string
		data         []byte
	}
}

func (f *fakeWSClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	f.posts = append(f.posts, struct {
		connectionID string
		data         []byte
	}{connectionID: connectionID, data: data})
	return f.postErr
}

func (f *fakeWSClient) DeleteConnection(_ context.Context, _ string) error { return nil }

func (f *fakeWSClient) GetConnection(_ context.Context, _ string) (streamer.Connection, error) {
	return streamer.Connection{}, nil
}

type fakeCommandRouter struct {
	response *streaming.CommandResponse
	err      error

	registered []streaming.CommandHandler
	handled    []*streaming.Command
}

func (f *fakeCommandRouter) RegisterHandler(handler streaming.CommandHandler) {
	f.registered = append(f.registered, handler)
}

func (f *fakeCommandRouter) HandleCommand(_ context.Context, _ *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	f.handled = append(f.handled, cmd)
	return f.response, f.err
}

type stubCommandHandler struct{}

func (stubCommandHandler) HandleCommand(context.Context, *streaming.ConnectionInfo, *streaming.Command) (*streaming.CommandResponse, error) {
	return nil, nil
}

func (stubCommandHandler) GetSupportedCommands() []string { return []string{"noop"} }

func TestDecodeQueryToken(t *testing.T) {
	require.Equal(t, "", decodeQueryToken(""))
	require.Equal(t, "abc+def", decodeQueryToken("abc def"))
	require.Equal(t, "abc+def", decodeQueryToken("abc%20def"))
}

func TestGetAuthMethodFromEvent(t *testing.T) {
	require.Equal(t, "anonymous", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{}, ""))
	require.Equal(t, "oauth", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{
		QueryStringParameters: map[string]string{"access_token": "t"},
	}, "t"))
	require.Equal(t, "bearer", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{
		Headers: map[string]string{"Authorization": "Bearer t"},
	}, "t"))
	require.Equal(t, "basic", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{
		Headers: map[string]string{"Authorization": "Basic abc"},
	}, "t"))
	require.Equal(t, "bearer", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{
		Headers: map[string]string{"authorization": "Bearer t"},
	}, "t"))
	require.Equal(t, "oauth", getAuthMethodFromEvent(events.APIGatewayWebsocketProxyRequest{
		Headers: map[string]string{"Authorization": "Token t"},
	}, "t"))
}

func TestParseCommand_ErrorsAndSuccess(t *testing.T) {
	sh := &StreamingHandler{}

	_, err := sh.parseCommand(StreamMessage{Payload: map[string]any{}})
	require.ErrorIs(t, err, streaming.ErrCommandIDRequired)

	_, err = sh.parseCommand(StreamMessage{Payload: map[string]any{"id": 123}})
	require.ErrorIs(t, err, streaming.ErrCommandIDMustBeString)

	_, err = sh.parseCommand(StreamMessage{Payload: map[string]any{"id": "1"}})
	require.ErrorIs(t, err, streaming.ErrCommandTypeRequired)

	_, err = sh.parseCommand(StreamMessage{Payload: map[string]any{"id": "1", "command": 123}})
	require.ErrorIs(t, err, streaming.ErrCommandTypeMustBeString)

	cmd, err := sh.parseCommand(StreamMessage{Payload: map[string]any{"id": "1", "command": "noop", "data": "x"}})
	require.NoError(t, err)
	require.Equal(t, "1", cmd.ID)
	require.Equal(t, "noop", cmd.Type)
	require.Equal(t, map[string]any{"data": "x"}, cmd.Payload)

	cmd, err = sh.parseCommand(StreamMessage{Payload: map[string]any{
		"id":      "2",
		"command": "noop",
		"data":    map[string]interface{}{"a": "b"},
	}})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"a": "b"}, cmd.Payload)

	require.Equal(t, "unknown", sh.getCommandType(StreamMessage{Payload: map[string]any{}}))
	require.Equal(t, "noop", sh.getCommandType(StreamMessage{Payload: map[string]any{"command": "noop"}}))
}

func TestSendMessageToConnection_NoClient(t *testing.T) {
	sh := &StreamingHandler{}
	require.ErrorIs(t, sh.sendMessageToConnection("c1", StreamMessage{Type: "ping"}), streaming.ErrAPIGatewayClientNotInit)
}

func TestSendMessageToConnection_MarshalError(t *testing.T) {
	ws := &fakeWSClient{}
	sh := &StreamingHandler{wsClient: ws}
	require.Error(t, sh.sendMessageToConnection("c1", StreamMessage{Type: "x", Payload: map[string]any{"ch": make(chan int)}}))
	require.Len(t, ws.posts, 0)
}

func TestSendError_AndFailure(t *testing.T) {
	ws := &fakeWSClient{}
	sh := &StreamingHandler{wsClient: ws}
	require.NoError(t, sh.sendError("c1", "nope"))

	ws.postErr = errors.New("send failed")
	require.Error(t, sh.sendError("c1", "nope"))
}

func TestSendCommandResponse_IncludesErrorInfo(t *testing.T) {
	ws := &fakeWSClient{}
	sh := &StreamingHandler{wsClient: ws}

	resp := &streaming.CommandResponse{
		ID:      "1",
		Type:    "command_error",
		Success: false,
		Error:   &streaming.CommandError{Code: "X", Message: "m", Details: "d"},
	}

	require.NoError(t, sh.sendCommandResponse("c1", resp))
	require.Len(t, ws.posts, 1)

	var msg StreamMessage
	require.NoError(t, json.Unmarshal(ws.posts[0].data, &msg))
	errObj, ok := msg.Payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "X", errObj["code"])
}

func TestContains(t *testing.T) {
	require.True(t, contains([]string{"a", "b"}, "a"))
	require.False(t, contains([]string{"a", "b"}, "c"))
}

func TestHandleSubscribe_AndUnsubscribe(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}

	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "bad"}))
	require.Len(t, ws.posts, 1)
	var errMsg StreamMessage
	require.NoError(t, json.Unmarshal(ws.posts[0].data, &errMsg))
	require.Equal(t, "error", errMsg.Type)

	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), &models.WebSocketConnection{ConnectionID: "c1"}, StreamMessage{Type: "subscribe", Stream: "user"}))
	require.Len(t, ws.posts, 1)

	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "public"}))
	require.Len(t, connRepo.wroteSubscriptions, 1)
	require.Len(t, ws.posts, 1)

	connRepo.updateConnectionErr = errors.New("update failed")
	ws.posts = nil
	require.NoError(t, sh.handleUnsubscribe(context.Background(), connection, StreamMessage{Type: "unsubscribe", Stream: "public"}))
	require.Len(t, ws.posts, 1)

	connRepo.updateConnectionErr = nil
	connRepo.deleteSubscriptionErr = errors.New("delete sub failed")
	ws.posts = nil
	require.NoError(t, sh.handleUnsubscribe(context.Background(), connection, StreamMessage{Type: "unsubscribe", Stream: "public"}))
	require.Len(t, connRepo.deletedSubscriptions, 1)
	require.Len(t, ws.posts, 1)
}

func TestHandleSubscribe_AndUnsubscribe_ErrorBranches(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}

	connRepo.updateConnectionErr = errors.New("update failed")
	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "public"}))
	require.Len(t, ws.posts, 1)

	connRepo.updateConnectionErr = nil
	connRepo.writeSubscriptionErr = errors.New("write sub failed")
	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "public"}))
	require.Len(t, ws.posts, 1)

	connRepo.writeSubscriptionErr = nil
	ws.postErr = errors.New("send failed")
	require.ErrorIs(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "public"}), streaming.ErrConfirmationSendFailed)

	ws.postErr = errors.New("send failed")
	require.ErrorIs(t, sh.handleUnsubscribe(context.Background(), connection, StreamMessage{Type: "unsubscribe", Stream: "public"}), streaming.ErrConfirmationSendFailed)
}

func TestHandleSubscribe_CanonicalizesAndRestrictsUserStreams_Round12(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}
	ws.posts = nil

	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "user"}))
	require.Len(t, connRepo.wroteSubscriptions, 1)
	require.Equal(t, "user:alice", connRepo.wroteSubscriptions[0].stream)
	require.Contains(t, connection.Streams, "user:alice")
	require.Len(t, ws.posts, 1)

	var confirm StreamMessage
	require.NoError(t, json.Unmarshal(ws.posts[0].data, &confirm))
	require.Equal(t, "subscribed", confirm.Type)
	require.Equal(t, "user", confirm.Stream)
	require.Equal(t, "user:alice", confirm.Payload["canonical_stream"])

	// Cannot subscribe to another user's stream.
	ws.posts = nil
	connRepo.wroteSubscriptions = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "user:bob"}))
	require.Len(t, connRepo.wroteSubscriptions, 0)
	require.Len(t, ws.posts, 1)
	require.NoError(t, json.Unmarshal(ws.posts[0].data, &confirm))
	require.Equal(t, "error", confirm.Type)
}

func TestHandleSubscribe_DirectStreamsRequireAuth_Round12(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	// Anonymous connections cannot subscribe to direct streams (including direct:<user>).
	anon := &models.WebSocketConnection{ConnectionID: "c1"}
	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), anon, StreamMessage{Type: "subscribe", Stream: "direct:alice"}))
	require.Len(t, connRepo.wroteSubscriptions, 0)
	require.Len(t, ws.posts, 1)
}

func TestHandleSubscribe_ListStreamsRequireOwnership_Round12(t *testing.T) {
	origAuthorize := authorizeListStreamSubscriptionFn
	t.Cleanup(func() { authorizeListStreamSubscriptionFn = origAuthorize })

	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
		storageFactory: nil,
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}

	authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
		return apperrors.NotFound("list")
	}
	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "list:1"}))
	require.Len(t, connRepo.wroteSubscriptions, 0)
	require.Len(t, ws.posts, 1)

	authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
		return nil
	}
	ws.posts = nil
	require.NoError(t, sh.handleSubscribe(context.Background(), connection, StreamMessage{Type: "subscribe", Stream: "list:1"}))
	require.Len(t, connRepo.wroteSubscriptions, 1)
	require.Equal(t, "list:1", connRepo.wroteSubscriptions[0].stream)
	require.Contains(t, connection.Streams, "list:1")
	require.Len(t, ws.posts, 1)
}

func TestHandleUnsubscribe_UsesCanonicalStreamAliases_Round12(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	ws := &fakeWSClient{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice", Streams: []string{"user:alice"}}
	ws.posts = nil
	require.NoError(t, sh.handleUnsubscribe(context.Background(), connection, StreamMessage{Type: "unsubscribe", Stream: "user"}))
	require.Empty(t, connection.Streams)

	// We should attempt to delete both the alias and canonical name, for backward-compatibility.
	require.Len(t, connRepo.deletedSubscriptions, 2)
}

func TestHandlePing(t *testing.T) {
	ws := &fakeWSClient{}
	sh := &StreamingHandler{wsClient: ws, logger: zap.NewNop()}
	require.NoError(t, sh.handlePing("c1"))

	ws.postErr = errors.New("send failed")
	require.Error(t, sh.handlePing("c1"))
}

func TestHandleDisconnect_SuccessAndErrors(t *testing.T) {
	connRepo := &fakeConnectionRepo{}
	sh := &StreamingHandler{connectionRepo: connRepo, logger: zap.NewNop()}

	evt := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1"},
	}

	connRepo.deleteAllSubsErr = errors.New("cleanup failed")
	require.NoError(t, sh.handleDisconnect(context.Background(), evt))
	require.Len(t, connRepo.deletedConnections, 1)

	connRepo.deleteConnectionErr = errors.New("delete failed")
	require.Error(t, sh.handleDisconnect(context.Background(), evt))
}

func TestHandleCommand_AndResponses(t *testing.T) {
	ws := &fakeWSClient{}
	router := &fakeCommandRouter{
		response: &streaming.CommandResponse{ID: "1", Type: "ok", Success: true, Data: map[string]any{"x": "y"}},
	}
	sh := &StreamingHandler{
		commandRouter: router,
		wsClient:      ws,
		logger:        zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice", Streams: []string{"public"}}

	ws.posts = nil
	require.NoError(t, sh.handleCommand(context.Background(), connection, StreamMessage{Type: "command", Payload: map[string]any{}}))
	require.Len(t, ws.posts, 1)

	ws.posts = nil
	require.NoError(t, sh.handleCommand(context.Background(), connection, StreamMessage{Type: "command", Payload: map[string]any{
		"id":      "1",
		"command": "noop",
		"data":    map[string]any{"a": 1},
	}}))
	require.Len(t, router.handled, 1)
	require.Len(t, ws.posts, 1)

	var posted StreamMessage
	require.NoError(t, json.Unmarshal(ws.posts[0].data, &posted))
	require.Equal(t, "command_response", posted.Type)
}

func TestHandleCommand_RouterErrorAndNilResponse(t *testing.T) {
	ws := &fakeWSClient{}
	router := &fakeCommandRouter{err: errors.New("boom")}
	sh := &StreamingHandler{
		commandRouter: router,
		wsClient:      ws,
		logger:        zap.NewNop(),
	}

	connection := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}

	ws.posts = nil
	require.NoError(t, sh.handleCommand(context.Background(), connection, StreamMessage{Type: "command", Payload: map[string]any{
		"id":      "1",
		"command": "noop",
	}}))
	require.Len(t, ws.posts, 1)

	ws.posts = nil
	router.err = nil
	router.response = nil
	require.NoError(t, sh.handleCommand(context.Background(), connection, StreamMessage{Type: "command", Payload: map[string]any{
		"id":      "1",
		"command": "noop",
	}}))
	require.Len(t, ws.posts, 0)
}

func TestHandleMessage_RoutesAndErrors(t *testing.T) {
	originalRunAsync := runAsyncFn
	t.Cleanup(func() { runAsyncFn = originalRunAsync })
	runAsyncFn = func(fn func()) { fn() }

	connRepo := &fakeConnectionRepo{
		getConnectionResp: &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice", Streams: []string{"public"}},
	}
	ws := &fakeWSClient{}
	costTracker := &fakeCostTracker{err: errors.New("track failed")}
	router := &fakeCommandRouter{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		costTracker:    costTracker,
		commandRouter:  router,
		wsClient:       ws,
		logger:         zap.NewNop(),
	}

	evt := events.APIGatewayWebsocketProxyRequest{
		Body: `{"type":"ping"}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}
	require.NoError(t, sh.handleMessage(context.Background(), evt))
	require.Len(t, costTracker.calls, 1)
	require.Len(t, ws.posts, 1)

	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `not-json`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}))
	require.Len(t, ws.posts, 1)

	connRepo.getConnectionErr = errors.New("not found")
	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), evt))
	require.Len(t, ws.posts, 1)

	connRepo.getConnectionErr = nil
	connRepo.updateConnectionErr = errors.New("update failed")
	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"type":"wat"}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}))
	require.Len(t, ws.posts, 1)

	connRepo.updateConnectionErr = nil
	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"type":"subscribe","stream":"public"}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}))
	require.NotEmpty(t, ws.posts)

	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"type":"unsubscribe","stream":"public"}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}))
	require.NotEmpty(t, ws.posts)

	ws.posts = nil
	require.NoError(t, sh.handleMessage(context.Background(), events.APIGatewayWebsocketProxyRequest{
		Body: `{"type":"command","payload":{"id":"1","command":"noop"}}`,
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
		},
	}))
}

func TestHandleConnect_AnonymousAndAuth(t *testing.T) {
	originalRunAsync := runAsyncFn
	t.Cleanup(func() { runAsyncFn = originalRunAsync })
	runAsyncFn = func(fn func()) { fn() }

	connRepo := &fakeConnectionRepo{}
	costTracker := &fakeCostTracker{}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		costTracker:    costTracker,
		logger:         zap.NewNop(),
		cfg:            &config.Config{JWTSecret: "secret"},
	}

	require.NoError(t, sh.handleConnect(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1"},
	}))
	require.Len(t, connRepo.wroteConnections, 1)
	require.Equal(t, "", connRepo.wroteConnections[0].userID)
	require.Equal(t, "anonymous", costTracker.calls[0].AuthMethod)

	// Invalid token -> anonymous connection allowed
	connRepo.wroteConnections = nil
	costTracker.calls = nil
	require.NoError(t, sh.handleConnect(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c2"},
		QueryStringParameters: map[string]string{
			"access_token": "not-a-jwt",
		},
	}))
	require.Equal(t, "", connRepo.wroteConnections[0].userID)

	// Valid token -> authenticated connection
	connRepo.wroteConnections = nil
	costTracker.calls = nil
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Username: "alice",
		Scopes:   []string{"read"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)

	require.NoError(t, sh.handleConnect(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c3"},
		QueryStringParameters: map[string]string{
			"access_token": tokenString,
		},
	}))
	require.Equal(t, "u123", connRepo.wroteConnections[0].userID)
	require.Equal(t, "alice", connRepo.wroteConnections[0].username)
}

func TestHandleConnect_HeaderTokenAndErrors(t *testing.T) {
	originalRunAsync := runAsyncFn
	t.Cleanup(func() { runAsyncFn = originalRunAsync })
	runAsyncFn = func(fn func()) { fn() }

	connRepo := &fakeConnectionRepo{writeConnectionErr: errors.New("write failed")}
	costTracker := &fakeCostTracker{err: errors.New("track failed")}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		costTracker:    costTracker,
		logger:         zap.NewNop(),
		cfg:            &config.Config{JWTSecret: "secret"},
	}

	err := sh.handleConnect(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c1"},
		Headers: map[string]string{
			"Authorization": "Token ignored",
			"authorization": "Bearer not-a-jwt",
		},
	})
	require.ErrorIs(t, err, streaming.ErrConnectionWriteFailed)

	connRepo.writeConnectionErr = nil
	require.NoError(t, sh.handleConnect(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{ConnectionID: "c2"},
		Headers:        map[string]string{"Authorization": "Bearer not-a-jwt"},
	}))
}

func TestWebSocketDefault_InitializesClientAndRespondsToPing(t *testing.T) {
	originalNewClient := newStreamerClientFn
	originalRunAsync := runAsyncFn
	t.Cleanup(func() {
		newStreamerClientFn = originalNewClient
		runAsyncFn = originalRunAsync
	})
	runAsyncFn = func(fn func()) { fn() }

	ws := &fakeWSClient{}
	var gotEndpoint string
	newStreamerClientFn = func(_ context.Context, endpoint string, _ ...streamer.Option) (streamer.Client, error) {
		gotEndpoint = endpoint
		return ws, nil
	}

	connRepo := &fakeConnectionRepo{getConnectionResp: &models.WebSocketConnection{ConnectionID: "c1", Username: "alice"}}
	sh := &StreamingHandler{
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		cfg:            &config.Config{JWTSecret: "secret"},
		awsConfig:      aws.Config{Region: "us-east-1"},
	}

	app := apptheory.NewSecure(apptheory.SecureOptions{
		Tier:              apptheory.TierP2,
		PrincipalResolver: sh.resolveWebSocketPrincipal,
		WebSocketSupport:  true,
	})
	app.WebSocket("$default", sh.HandleWebSocketDefault, apptheory.Optional())

	resp := app.ServeWebSocket(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$default",
			DomainName:   "api.execute-api.us-east-1.amazonaws.com",
			Stage:        "dev",
		},
		Path: "/",
		Body: `{"type":"ping"}`,
	})
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "https://api.execute-api.us-east-1.amazonaws.com/dev", gotEndpoint)
	require.NotEmpty(t, ws.posts)
	require.Equal(t, "c1", ws.posts[0].connectionID)
	require.Contains(t, string(ws.posts[0].data), `"type":"pong"`)
	require.Equal(t, 1, connRepo.getConnectionCalls, "the gate-loaded connection is reused by the frame handler")
}

func TestInitializeStreaming_SuccessAndErrors(t *testing.T) {
	originalMustInit := mustInitializeLambdaFn
	originalInitDefaults := initializeWithDefaultsFn
	originalEnsureRepos := ensureRepositoryFactoryFn
	originalResolveDB := resolveDynamoClientFn
	originalNewUserRepo := newUserRepositoryFn
	originalNewConnRepo := newStreamingConnectionRepoFn
	originalNewCostTracker := newWebSocketCostTrackerFn
	originalNewRegistry := newServiceRegistryFn
	originalNewRouter := newCommandRouterFn
	originalRegisterHandlers := registerCommandHandlersFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInit
		initializeWithDefaultsFn = originalInitDefaults
		ensureRepositoryFactoryFn = originalEnsureRepos
		resolveDynamoClientFn = originalResolveDB
		newUserRepositoryFn = originalNewUserRepo
		newStreamingConnectionRepoFn = originalNewConnRepo
		newWebSocketCostTrackerFn = originalNewCostTracker
		newServiceRegistryFn = originalNewRegistry
		newCommandRouterFn = originalNewRouter
		registerCommandHandlersFn = originalRegisterHandlers
	})

	fakeLambda := &common.LambdaContext{
		Config: &config.Config{
			DynamoTableName:    "tbl",
			ConnectionsTable:   "conn_tbl",
			SubscriptionsTable: "sub_tbl",
			Domain:             "example.com",
			JWTSecret:          "secret",
		},
		Logger:      zap.NewNop(),
		AWSServices: &awsinit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
	}

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return fakeLambda }
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("defaults failed") }
	ensureRepositoryFactoryFn = func() error { return nil }
	resolveDynamoClientFn = func() dynamormCore.DB { return &tabletheory.LambdaDB{} }
	newUserRepositoryFn = func(dynamormCore.DB, string, *zap.Logger) *repositories.UserRepository { return nil }
	newStreamingConnectionRepoFn = func(dynamormCore.DB, string, dynamormCore.DB, string, *zap.Logger) streamingConnectionRepository {
		return &fakeConnectionRepo{}
	}
	newWebSocketCostTrackerFn = func(dynamormCore.DB, string, *zap.Logger) websocketCostTracker { return &fakeCostTracker{} }
	newServiceRegistryFn = func(storagecore.RepositoryStorage, streaming.Publisher, *zap.Logger, *services.ServiceConfig) (*services.Registry, error) {
		return nil, nil
	}
	newCommandRouterFn = func(*zap.Logger) streamingCommandRouter { return &fakeCommandRouter{} }
	registerCommandHandlersFn = func(streamingCommandRouter, *services.Registry, *zap.Logger) {}

	require.NoError(t, initializeStreaming())
	require.NotNil(t, handler)
	require.Equal(t, "us-east-1", handler.awsConfig.Region)

	resolveDynamoClientFn = func() dynamormCore.DB { return nil }
	require.Error(t, initializeStreaming())

	resolveDynamoClientFn = func() dynamormCore.DB { return &tabletheory.LambdaDB{} }
	fakeLambda.Config.DynamoTableName = ""
	require.Error(t, initializeStreaming())

	fakeLambda.Config.DynamoTableName = "tbl"
	newServiceRegistryFn = func(storagecore.RepositoryStorage, streaming.Publisher, *zap.Logger, *services.ServiceConfig) (*services.Registry, error) {
		return nil, errors.New("registry failed")
	}
	require.Error(t, initializeStreaming())
}

func TestRegisterCommandHandlers_RegistersHandlers(t *testing.T) {
	originalStatus := newStatusCommandHandlerFn
	originalAccount := newAccountCommandHandlerFn
	originalRelationship := newRelationshipCommandHandlerFn
	originalSystem := newSystemCommandHandlerFn
	t.Cleanup(func() {
		newStatusCommandHandlerFn = originalStatus
		newAccountCommandHandlerFn = originalAccount
		newRelationshipCommandHandlerFn = originalRelationship
		newSystemCommandHandlerFn = originalSystem
	})

	newStatusCommandHandlerFn = func(*services.Registry, *zap.Logger) streaming.CommandHandler { return stubCommandHandler{} }
	newAccountCommandHandlerFn = func(*services.Registry, *zap.Logger) streaming.CommandHandler { return stubCommandHandler{} }
	newRelationshipCommandHandlerFn = func(*services.Registry, *zap.Logger) streaming.CommandHandler { return stubCommandHandler{} }
	newSystemCommandHandlerFn = func(*services.Registry, *zap.Logger) streaming.CommandHandler { return stubCommandHandler{} }

	registerCommandHandlers(nil, &services.Registry{}, zap.NewNop())
	registerCommandHandlers(&fakeCommandRouter{}, nil, zap.NewNop())

	router := &fakeCommandRouter{}
	registerCommandHandlers(router, &services.Registry{}, zap.NewNop())
	require.Len(t, router.registered, 4)
}

func TestInitializeManualRepositories_SuccessAndErrors(t *testing.T) {
	originalLogger := logger
	originalCfg := cfg
	originalLambdaCtx := lambdaCtx
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		logger = originalLogger
		cfg = originalCfg
		lambdaCtx = originalLambdaCtx
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	cfg = &config.Config{
		Region:          "us-west-2",
		DynamoTableName: "tbl",
	}
	lambdaCtx = &common.LambdaContext{}
	logger = nil
	repos = nil

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) {
		return &factory.RepositoryFactory{}, nil
	}

	require.NoError(t, initializeManualRepositories())
	require.NotNil(t, repos)
	require.NotNil(t, lambdaCtx.Repos)
	require.NotNil(t, lambdaCtx.DynamoDB)
	require.Equal(t, "us-west-2", cfg.Region)
	require.NotEmpty(t, os.Getenv("AWS_REGION"))
	require.NotEmpty(t, os.Getenv("AWS_DEFAULT_REGION"))

	cfg.DynamoTableName = ""
	require.Error(t, initializeManualRepositories())

	cfg.DynamoTableName = "tbl"
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("client failed") }
	require.Error(t, initializeManualRepositories())

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) {
		return nil, errors.New("factory failed")
	}
	require.Error(t, initializeManualRepositories())
}

func TestInitializeManualRepositories_RegionResolution(t *testing.T) {
	originalLogger := logger
	originalCfg := cfg
	originalLambdaCtx := lambdaCtx
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		logger = originalLogger
		cfg = originalCfg
		lambdaCtx = originalLambdaCtx
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) {
		return &factory.RepositoryFactory{}, nil
	}

	t.Run("aws_region", func(t *testing.T) {
		t.Setenv("AWS_REGION", "ap-south-1")
		t.Setenv("AWS_DEFAULT_REGION", "")

		cfg = &config.Config{Region: "", DynamoTableName: "tbl"}
		lambdaCtx = &common.LambdaContext{}
		repos = nil
		require.NoError(t, initializeManualRepositories())
		require.Equal(t, "ap-south-1", cfg.Region)
	})

	t.Run("aws_default_region", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "eu-central-1")

		cfg = &config.Config{Region: "", DynamoTableName: "tbl"}
		lambdaCtx = &common.LambdaContext{}
		repos = nil
		require.NoError(t, initializeManualRepositories())
		require.Equal(t, "eu-central-1", cfg.Region)
	})

	t.Run("default_fallback", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		cfg = &config.Config{Region: "", DynamoTableName: "tbl"}
		lambdaCtx = &common.LambdaContext{}
		repos = nil
		require.NoError(t, initializeManualRepositories())
		require.Equal(t, "us-east-1", cfg.Region)
	})
}

func TestEnsureRepositoryFactory_Branches(t *testing.T) {
	originalLogger := logger
	originalCfg := cfg
	originalLambdaCtx := lambdaCtx
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		logger = originalLogger
		cfg = originalCfg
		lambdaCtx = originalLambdaCtx
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	// lambdaCtx nil -> noop
	lambdaCtx = nil
	repos = nil
	require.NoError(t, ensureRepositoryFactory())

	// lambdaCtx.Repos implements core.RepositoryStorage -> uses it
	lambdaCtx = &common.LambdaContext{Repos: &factory.RepositoryFactory{}}
	repos = nil
	require.NoError(t, ensureRepositoryFactory())
	require.NotNil(t, repos)

	// wrong type -> warn + fallback manual init (stubbed)
	lambdaCtx = &common.LambdaContext{Repos: struct{}{}}
	repos = nil
	logger = nil
	cfg = &config.Config{Region: "us-east-2", DynamoTableName: "tbl"}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &tabletheory.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) {
		return &factory.RepositoryFactory{}, nil
	}
	require.NoError(t, ensureRepositoryFactory())
	require.NotNil(t, repos)
}

func TestResolveDynamoClient_UsesLambdaContextDynamo(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		repos = originalRepos
	})

	lambdaCtx = &common.LambdaContext{DynamoDB: &tabletheory.LambdaDB{}}
	repos = nil
	require.NotNil(t, resolveDynamoClient())

	lambdaCtx = &common.LambdaContext{DynamoDB: struct{}{}}
	require.Nil(t, resolveDynamoClient())
}

func TestResolveDynamoClient_UsesReposDB(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		repos = originalRepos
	})

	mockStorage := &testingmocks.MockRepositoryStorage{}
	db := dynamormCore.DB(&tabletheory.LambdaDB{})
	mockStorage.On("GetDB").Return(db)

	lambdaCtx = &common.LambdaContext{}
	repos = mockStorage

	require.Equal(t, db, resolveDynamoClient())
	require.Equal(t, db, lambdaCtx.DynamoDB)
	mockStorage.AssertExpectations(t)
}

func TestInitializeStreamingOnStart_InitializesWhenNotUnitTests(t *testing.T) {
	originalRunning := runningUnitTestsFn
	originalMustInit := mustInitializeLambdaFn
	originalInitDefaults := initializeWithDefaultsFn
	originalEnsureRepos := ensureRepositoryFactoryFn
	originalResolveDB := resolveDynamoClientFn
	originalNewUserRepo := newUserRepositoryFn
	originalNewConnRepo := newStreamingConnectionRepoFn
	originalNewCostTracker := newWebSocketCostTrackerFn
	originalNewRegistry := newServiceRegistryFn
	originalNewRouter := newCommandRouterFn
	originalRegisterHandlers := registerCommandHandlersFn
	t.Cleanup(func() {
		runningUnitTestsFn = originalRunning
		mustInitializeLambdaFn = originalMustInit
		initializeWithDefaultsFn = originalInitDefaults
		ensureRepositoryFactoryFn = originalEnsureRepos
		resolveDynamoClientFn = originalResolveDB
		newUserRepositoryFn = originalNewUserRepo
		newStreamingConnectionRepoFn = originalNewConnRepo
		newWebSocketCostTrackerFn = originalNewCostTracker
		newServiceRegistryFn = originalNewRegistry
		newCommandRouterFn = originalNewRouter
		registerCommandHandlersFn = originalRegisterHandlers
	})

	runningUnitTestsFn = func() bool { return false }

	fakeLambda := &common.LambdaContext{
		Config: &config.Config{
			DynamoTableName: "tbl",
			Domain:          "example.com",
			JWTSecret:       "secret",
		},
		Logger:      zap.NewNop(),
		AWSServices: &awsinit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
	}

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return fakeLambda }
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
	ensureRepositoryFactoryFn = func() error { return nil }
	resolveDynamoClientFn = func() dynamormCore.DB { return &tabletheory.LambdaDB{} }
	newUserRepositoryFn = func(dynamormCore.DB, string, *zap.Logger) *repositories.UserRepository { return nil }
	newStreamingConnectionRepoFn = func(dynamormCore.DB, string, dynamormCore.DB, string, *zap.Logger) streamingConnectionRepository {
		return &fakeConnectionRepo{}
	}
	newWebSocketCostTrackerFn = func(dynamormCore.DB, string, *zap.Logger) websocketCostTracker { return &fakeCostTracker{} }
	newServiceRegistryFn = func(storagecore.RepositoryStorage, streaming.Publisher, *zap.Logger, *services.ServiceConfig) (*services.Registry, error) {
		return &services.Registry{}, nil
	}
	newCommandRouterFn = func(*zap.Logger) streamingCommandRouter { return &fakeCommandRouter{} }
	registerCommandHandlersFn = func(streamingCommandRouter, *services.Registry, *zap.Logger) {}

	initializeStreamingOnStart()
	require.NotNil(t, handler)
}

func TestMain_RegistersRoutesAndStartsLambda(t *testing.T) {
	originalHandler := handler
	originalLambdaStart := lambdaStartFn
	originalNewClient := newStreamerClientFn
	originalRunAsync := runAsyncFn
	t.Cleanup(func() {
		handler = originalHandler
		lambdaStartFn = originalLambdaStart
		newStreamerClientFn = originalNewClient
		runAsyncFn = originalRunAsync
	})

	runAsyncFn = func(fn func()) { fn() }

	wsClient := &fakeWSClient{}
	newStreamerClientFn = func(_ context.Context, _ string, _ ...streamer.Option) (streamer.Client, error) {
		return wsClient, nil
	}

	connRepo := &fakeConnectionRepo{
		getConnectionResp: &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"},
	}
	handler = &StreamingHandler{
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		cfg:            &config.Config{JWTSecret: "secret"},
		awsConfig:      aws.Config{Region: "us-east-1"},
	}

	var startedWith any
	lambdaStartFn = func(h any) { startedWith = h }

	main()
	require.NotNil(t, startedWith)

	handlerFn, ok := startedWith.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	connectEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$connect",
			DomainName:   "api.execute-api.us-east-1.amazonaws.com",
			Stage:        "dev",
		},
		Path: "/",
	}
	raw, err := json.Marshal(connectEvent)
	require.NoError(t, err)
	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok := respAny.(events.APIGatewayProxyResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
	require.Len(t, connRepo.wroteConnections, 1)

	defaultEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$default",
			DomainName:   "api.execute-api.us-east-1.amazonaws.com",
			Stage:        "dev",
		},
		Path: "/",
		Body: `{"type":"ping"}`,
	}
	raw, err = json.Marshal(defaultEvent)
	require.NoError(t, err)
	respAny, err = handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = respAny.(events.APIGatewayProxyResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
	require.NotEmpty(t, wsClient.posts)

	disconnectEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$disconnect",
			DomainName:   "api.execute-api.us-east-1.amazonaws.com",
			Stage:        "dev",
		},
		Path: "/",
	}
	raw, err = json.Marshal(disconnectEvent)
	require.NoError(t, err)
	respAny, err = handlerFn(context.Background(), raw)
	require.NoError(t, err)
	resp, ok = respAny.(events.APIGatewayProxyResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, []string{"c1"}, connRepo.deletedConnections)
}

func TestWebSocketDefault_Returns500WhenClientInitFails(t *testing.T) {
	originalNewClient := newStreamerClientFn
	t.Cleanup(func() { newStreamerClientFn = originalNewClient })

	newStreamerClientFn = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
		return nil, errors.New("client failed")
	}

	sh := &StreamingHandler{
		connectionRepo: &fakeConnectionRepo{
			getConnectionResp: &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"},
		},
		costTracker: &fakeCostTracker{},
		logger:      zap.NewNop(),
		cfg:         &config.Config{JWTSecret: "secret"},
		awsConfig:   aws.Config{Region: "us-east-1"},
	}

	app := apptheory.NewSecure(apptheory.SecureOptions{
		Tier:              apptheory.TierP2,
		PrincipalResolver: sh.resolveWebSocketPrincipal,
		WebSocketSupport:  true,
	})
	app.WebSocket("$default", sh.HandleWebSocketDefault, apptheory.Optional())

	resp := app.ServeWebSocket(context.Background(), events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "c1",
			RouteKey:     "$default",
			DomainName:   "api.execute-api.us-east-1.amazonaws.com",
			Stage:        "dev",
		},
		Path: "/",
		Body: `{"type":"ping"}`,
	})
	require.Equal(t, 500, resp.StatusCode)
}
