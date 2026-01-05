// Package main implements the sse Lambda function providing Mastodon-compatible
// Server-Sent Events (SSE) endpoints via API Gateway REST API (v1) response streaming.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

const (
	streamPollLimit             = int32(50)
	streamIdlePollDelay         = 500 * time.Millisecond
	streamHeartbeatEvery        = 15 * time.Second
	streamMaxDuration           = 14*time.Minute + 30*time.Second
	streamEventTypeUpdate       = "update"
	streamEventTypeStatusUpdate = "status.update"
	streamEventTypeDelete       = "delete"
)

var (
	lambdaCtx   *common.LambdaContext
	cfg         *config.Config
	logger      *zap.Logger
	repos       core.RepositoryStorage
	authService accessTokenValidator
	eventLog    streamEventLog
)

type accessTokenValidator interface {
	ValidateAccessToken(tokenString string) (*auth.EnhancedClaims, error)
}

type streamEventLog interface {
	Enabled() bool
	Query(ctx context.Context, streamName, afterID string, limit int32) ([]streaming.StreamEventLogItem, error)
}

var (
	mustInitializeLambdaFn      = common.MustInitializeLambda
	initializeWithDefaultsFn    = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	newLambdaOptimizedClientFn  = dynamorm.NewLambdaOptimizedClient
	newRepositoryFactoryFn      = factory.NewRepositoryFactory
	newAuthServiceFn            = func(cfg *config.Config, repos core.RepositoryStorage) (accessTokenValidator, error) {
		return auth.NewAuthService(cfg, repos)
	}
	newStreamEventLogFn         = func(db dynamormCore.DB, ttl time.Duration) streamEventLog {
		return streaming.NewStreamEventLog(db, ttl)
	}
	lambdaStartFn               = lambda.Start
	timeAfterFn                 = time.After
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeSSE()
}

func main() {
	runSSE()
}

func initializeSSE() {
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:    "sse",
		LambdaType:     common.LambdaTypeAPI,
		RequestTimeout: 15 * time.Minute,
	})
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Best-effort standardized init (currently uses placeholders; SSE relies on manual wiring below).
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Debug("standardized initialization unavailable; using manual wiring", zap.Error(err))
	}

	// Manual repo + auth initialization (keeps this lambda standalone).
	tableName := cfg.DynamoTableName
	if err := common.ValidateRequiredParam("DYNAMODB_TABLE", tableName); err != nil {
		logger.Fatal("DYNAMODB_TABLE environment variable is required", zap.Error(err))
	}

	var db dynamormCore.DB
	if lambdaCtx.DynamoDB != nil {
		if existing, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok && existing != nil {
			db = existing
		}
	}
	if db == nil {
		manualDB, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
		if err != nil {
			logger.Fatal("failed to initialize DynamoDB client", zap.Error(err))
		}
		db = manualDB
	}

	var err error
	repos, err = newRepositoryFactoryFn(db, tableName, logger)
	if err != nil {
		logger.Fatal("failed to create repository factory", zap.Error(err))
	}

	authService, err = newAuthServiceFn(cfg, repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	if cfg.StreamEventsTable == "" {
		logger.Fatal("STREAM_EVENTS_TABLE_NAME environment variable is required")
	}
	eventLog = newStreamEventLogFn(db, 30*time.Minute)
}

func runSSE() {
	app := lift.New()
	if cfg.DebugMode {
		app = lift.New(lift.WithDebug())
	}

	app.Use(liftMiddleware.RequestID())
	app.Use(liftMiddleware.Logger())
	app.Use(liftMiddleware.Recover())

	_ = app.GET("/api/v1/streaming", handleStreamingRoot)
	_ = app.GET("/api/v1/streaming/health", handleHealth)

	_ = app.GET("/api/v1/streaming/user", handleUserStream)
	_ = app.GET("/api/v1/streaming/user/notification", handleUserNotificationStream)
	_ = app.GET("/api/v1/streaming/public", handlePublicStream(streaming.PublicStream))
	_ = app.GET("/api/v1/streaming/public/local", handlePublicStream(streaming.PublicLocalStream))
	_ = app.GET("/api/v1/streaming/public/remote", handlePublicStream(streaming.PublicRemoteStream))
	_ = app.GET("/api/v1/streaming/hashtag", handleHashtagStream(false))
	_ = app.GET("/api/v1/streaming/hashtag/local", handleHashtagStream(true))
	_ = app.GET("/api/v1/streaming/list", handleListStream)
	_ = app.GET("/api/v1/streaming/direct", handleDirectStream)

	lambdaStartFn(app.HandleRequest)
}

func handleStreamingRoot(ctx *lift.Context) error {
	return ctx.Status(http.StatusNotFound).Text("Not Found")
}

func handleHealth(ctx *lift.Context) error {
	return ctx.Text("OK")
}

func handleUserStream(ctx *lift.Context) error {
	claims, err := requireClaims(ctx)
	if err != nil {
		return err
	}
	return streamSSE(ctx, streaming.UserStreamName(claims.Username), false)
}

func handleUserNotificationStream(ctx *lift.Context) error {
	claims, err := requireClaims(ctx)
	if err != nil {
		return err
	}
	return streamSSE(ctx, streaming.UserNotificationStreamName(claims.Username), false)
}

func handlePublicStream(streamName string) lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		if _, err := requireClaims(ctx); err != nil {
			return err
		}
		return streamSSE(ctx, streamName, ctx.QueryParam("only_media") == "true")
	})
}

func handleHashtagStream(localOnly bool) lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		if _, err := requireClaims(ctx); err != nil {
			return err
		}

		tag := strings.TrimSpace(ctx.QueryParam("tag"))
		if tag == "" {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "tag is required"})
		}

		streamName := streaming.HashtagStreamName(tag)
		if localOnly {
			streamName = fmt.Sprintf("hashtag:local:%s", tag)
		}
		return streamSSE(ctx, streamName, ctx.QueryParam("only_media") == "true")
	})
}

func handleListStream(ctx *lift.Context) error {
	claims, err := requireClaims(ctx)
	if err != nil {
		return err
	}

	listID := strings.TrimSpace(ctx.QueryParam("list"))
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "list is required"})
	}

	_ = claims // placeholder for future list membership validation
	return streamSSE(ctx, streaming.ListStreamName(listID), false)
}

func handleDirectStream(ctx *lift.Context) error {
	claims, err := requireClaims(ctx)
	if err != nil {
		return err
	}
	return streamSSE(ctx, streaming.DirectStreamName(claims.Username), false)
}

func streamSSE(ctx *lift.Context, streamName string, onlyMedia bool) error {
	if eventLog == nil || !eventLog.Enabled() {
		return ctx.Status(http.StatusServiceUnavailable).JSON(map[string]string{"error": "streaming unavailable"})
	}

	lastEventID := strings.TrimSpace(ctx.Header("Last-Event-ID"))

	eventCh := make(chan lift.SSEEvent, 8)
	go produceSSEEvents(ctx, eventCh, streamName, onlyMedia, lastEventID)

	return lift.SSEResponse(ctx, eventCh)
}

type sseStreamState struct {
	start   time.Time
	afterID string
}

func (s sseStreamState) expired() bool {
	return time.Since(s.start) > streamMaxDuration
}

func produceSSEEvents(ctx context.Context, eventCh chan<- lift.SSEEvent, streamName string, onlyMedia bool, lastEventID string) {
	defer close(eventCh)

	state := sseStreamState{start: time.Now(), afterID: lastEventID}
	heartbeat := time.NewTicker(streamHeartbeatEvery)
	defer heartbeat.Stop()

	for !state.expired() {
		items, err := eventLog.Query(ctx, streamName, state.afterID, streamPollLimit)
		if err != nil {
			eventCh <- lift.SSEEvent{Event: "error", Data: `{"error":"internal_error"}`}
			return
		}

		if len(items) > 0 {
			state.afterID = emitSSEItems(eventCh, items, onlyMedia, state.afterID)
			continue
		}

		if waitForSSEPoll(ctx, eventCh, heartbeat) {
			return
		}
	}
}

func emitSSEItems(eventCh chan<- lift.SSEEvent, items []streaming.StreamEventLogItem, onlyMedia bool, afterID string) string {
	for _, item := range items {
		afterID = item.ID
		if shouldSkipSSEItem(onlyMedia, item) {
			continue
		}

		eventCh <- lift.SSEEvent{ID: item.ID, Event: item.Event, Data: normalizeDeletePayload(item.Event, item.Data)}
	}
	return afterID
}

func shouldSkipSSEItem(onlyMedia bool, item streaming.StreamEventLogItem) bool {
	if !onlyMedia {
		return false
	}
	if item.Event != streamEventTypeUpdate && item.Event != streamEventTypeStatusUpdate {
		return false
	}
	return !payloadHasMedia(item.Data)
}

func waitForSSEPoll(ctx context.Context, eventCh chan<- lift.SSEEvent, heartbeat *time.Ticker) bool {
	select {
	case <-ctx.Done():
		return true
	case <-heartbeat.C:
		eventCh <- lift.SSEEvent{Event: "keepalive", Data: "thump"}
		return false
	case <-timeAfterFn(streamIdlePollDelay):
		return false
	}
}

func requireClaims(ctx *lift.Context) (*auth.EnhancedClaims, error) {
	token := strings.TrimSpace(ctx.Header("Authorization"))
	if token == "" {
		return nil, lift.Unauthorized("Authentication required")
	}

	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, lift.Unauthorized("Authentication required")
	}

	claims, err := authService.ValidateAccessToken(token)
	if err != nil {
		return nil, lift.Unauthorized("Invalid token").WithCause(err)
	}

	return claims, nil
}

func payloadHasMedia(data string) bool {
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return false
	}
	attachments, ok := raw["media_attachments"].([]any)
	return ok && len(attachments) > 0
}

func normalizeDeletePayload(eventType, data string) string {
	if eventType != streamEventTypeDelete {
		return data
	}

	// The stream-router may encode delete payloads as JSON objects. SSE expects the raw ID string.
	var obj map[string]any
	if err := json.Unmarshal([]byte(data), &obj); err == nil {
		if idVal, ok := obj["id"]; ok {
			if idStr, ok := idVal.(string); ok {
				return idStr
			}
		}
	}

	// If the payload is already a JSON string (e.g. "\"123\""), unwrap it.
	var idStr string
	if err := json.Unmarshal([]byte(data), &idStr); err == nil && idStr != "" {
		return idStr
	}

	return data
}
