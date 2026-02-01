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
	apptheory "github.com/theory-cloud/apptheory/runtime"
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
	mustInitializeLambdaFn     = common.MustInitializeLambda
	initializeWithDefaultsFn   = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	newLambdaOptimizedClientFn = dynamorm.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = factory.NewRepositoryFactory
	newAuthServiceFn           = func(cfg *config.Config, repos core.RepositoryStorage) (accessTokenValidator, error) {
		return auth.NewAuthService(cfg, repos)
	}
	newStreamEventLogFn = func(db dynamormCore.DB, ttl time.Duration) streamEventLog {
		return streaming.NewStreamEventLog(db, ttl)
	}
	lambdaStartFn = lambda.Start
	timeAfterFn   = time.After
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
	app := apptheory.New(
		apptheory.WithCORS(apptheory.CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: false,
			AllowHeaders: []string{
				"Accept",
				"Authorization",
				"Content-Type",
				"Last-Event-ID",
				"User-Agent",
				"X-Forwarded-For",
				"X-Forwarded-Proto",
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  64 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	app.Use(ssePanicRecovery(logger))
	app.Use(sseLoggingMiddleware(logger))

	app.Get("/api/v1/streaming", handleStreamingRoot)
	app.Get("/api/v1/streaming/health", handleHealth)

	app.Get("/api/v1/streaming/user", handleUserStream)
	app.Get("/api/v1/streaming/user/notification", handleUserNotificationStream)
	app.Get("/api/v1/streaming/public", handlePublicStream(streaming.PublicStream))
	app.Get("/api/v1/streaming/public/local", handlePublicStream(streaming.PublicLocalStream))
	app.Get("/api/v1/streaming/public/remote", handlePublicStream(streaming.PublicRemoteStream))
	app.Get("/api/v1/streaming/hashtag", handleHashtagStream(false))
	app.Get("/api/v1/streaming/hashtag/local", handleHashtagStream(true))
	app.Get("/api/v1/streaming/list", handleListStream)
	app.Get("/api/v1/streaming/direct", handleDirectStream)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleStreamingRoot(*apptheory.Context) (*apptheory.Response, error) {
	return apptheory.Text(http.StatusNotFound, "Not Found"), nil
}

func handleHealth(*apptheory.Context) (*apptheory.Response, error) {
	return apptheory.Text(http.StatusOK, "OK"), nil
}

func handleUserStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp := requireClaims(ctx)
	if resp != nil {
		return resp, nil
	}
	return streamSSE(ctx, streaming.UserStreamName(claims.Username), false)
}

func handleUserNotificationStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp := requireClaims(ctx)
	if resp != nil {
		return resp, nil
	}
	return streamSSE(ctx, streaming.UserNotificationStreamName(claims.Username), false)
}

func handlePublicStream(streamName string) apptheory.Handler {
	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if _, resp := requireClaims(ctx); resp != nil {
			return resp, nil
		}
		onlyMedia := false
		if ctx != nil && sseQueryParam(ctx, "only_media") == "true" {
			onlyMedia = true
		}
		return streamSSE(ctx, streamName, onlyMedia)
	}
}

func handleHashtagStream(localOnly bool) apptheory.Handler {
	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if _, resp := requireClaims(ctx); resp != nil {
			return resp, nil
		}

		tag := ""
		if ctx != nil {
			tag = strings.TrimSpace(sseQueryParam(ctx, "tag"))
		}
		if tag == "" {
			return apptheory.MustJSON(http.StatusBadRequest, map[string]string{"error": "tag is required"}), nil
		}

		streamName := streaming.HashtagStreamName(tag)
		if localOnly {
			streamName = fmt.Sprintf("hashtag:local:%s", tag)
		}

		onlyMedia := false
		if ctx != nil && sseQueryParam(ctx, "only_media") == "true" {
			onlyMedia = true
		}
		return streamSSE(ctx, streamName, onlyMedia)
	}
}

func handleListStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp := requireClaims(ctx)
	if resp != nil {
		return resp, nil
	}

	listID := ""
	if ctx != nil {
		listID = strings.TrimSpace(sseQueryParam(ctx, "list"))
	}
	if listID == "" {
		return apptheory.MustJSON(http.StatusBadRequest, map[string]string{"error": "list is required"}), nil
	}

	_ = claims // placeholder for future list membership validation
	return streamSSE(ctx, streaming.ListStreamName(listID), false)
}

func handleDirectStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp := requireClaims(ctx)
	if resp != nil {
		return resp, nil
	}
	return streamSSE(ctx, streaming.DirectStreamName(claims.Username), false)
}

func streamSSE(ctx *apptheory.Context, streamName string, onlyMedia bool) (*apptheory.Response, error) {
	if eventLog == nil || !eventLog.Enabled() {
		return apptheory.MustJSON(http.StatusServiceUnavailable, map[string]string{"error": "streaming unavailable"}), nil
	}

	lastEventID := strings.TrimSpace(sseHeaderValue(ctx, "last-event-id"))

	eventCh := make(chan apptheory.SSEEvent, 8)

	streamCtx := context.Background()
	if ctx != nil {
		streamCtx = ctx.Context()
	}
	go produceSSEEvents(streamCtx, eventCh, streamName, onlyMedia, lastEventID)

	return apptheory.SSEStreamResponse(streamCtx, http.StatusOK, eventCh)
}

type sseStreamState struct {
	start   time.Time
	afterID string
}

func (s sseStreamState) expired() bool {
	return time.Since(s.start) > streamMaxDuration
}

func produceSSEEvents(ctx context.Context, eventCh chan<- apptheory.SSEEvent, streamName string, onlyMedia bool, lastEventID string) {
	defer close(eventCh)

	state := sseStreamState{start: time.Now(), afterID: lastEventID}
	heartbeat := time.NewTicker(streamHeartbeatEvery)
	defer heartbeat.Stop()

	for !state.expired() {
		items, err := eventLog.Query(ctx, streamName, state.afterID, streamPollLimit)
		if err != nil {
			eventCh <- apptheory.SSEEvent{Event: "error", Data: `{"error":"internal_error"}`}
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

func emitSSEItems(eventCh chan<- apptheory.SSEEvent, items []streaming.StreamEventLogItem, onlyMedia bool, afterID string) string {
	for _, item := range items {
		afterID = item.ID
		if shouldSkipSSEItem(onlyMedia, item) {
			continue
		}

		eventCh <- apptheory.SSEEvent{ID: item.ID, Event: item.Event, Data: normalizeDeletePayload(item.Event, item.Data)}
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

func waitForSSEPoll(ctx context.Context, eventCh chan<- apptheory.SSEEvent, heartbeat *time.Ticker) bool {
	select {
	case <-ctx.Done():
		return true
	case <-heartbeat.C:
		eventCh <- apptheory.SSEEvent{Event: "keepalive", Data: "thump"}
		return false
	case <-timeAfterFn(streamIdlePollDelay):
		return false
	}
}

func requireClaims(ctx *apptheory.Context) (*auth.EnhancedClaims, *apptheory.Response) {
	token := strings.TrimSpace(sseHeaderValue(ctx, "authorization"))
	if token == "" {
		return nil, apptheory.MustJSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apptheory.MustJSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	claims, err := authService.ValidateAccessToken(token)
	if err != nil {
		return nil, apptheory.MustJSON(http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
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

func ssePanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered", zap.Any("panic", r))
					resp = apptheory.MustJSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func sseLoggingMiddleware(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			method := ""
			path := ""
			status := 0
			if ctx != nil {
				method = ctx.Request.Method
				path = ctx.Request.Path
			}
			if resp != nil {
				status = resp.Status
			}

			logger.Info("sse request completed",
				zap.String("method", method),
				zap.String("path", path),
				zap.Int("status", status),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return resp, err
		}
	}
}

func sseHeaderValue(ctx *apptheory.Context, key string) string {
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

func sseQueryParam(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
