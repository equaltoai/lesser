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
	streamPollLimit       = int32(50)
	streamIdlePollDelay   = 500 * time.Millisecond
	streamHeartbeatEvery  = 15 * time.Second
	streamMaxDuration     = 14*time.Minute + 30*time.Second
	streamEventTypeUpdate = "update"
	streamEventTypeStatusUpdate = "status.update"
	streamEventTypeDelete = "delete"
)

var (
	lambdaCtx   *common.LambdaContext
	cfg         *config.Config
	logger      *zap.Logger
	repos       core.RepositoryStorage
	authService *auth.AuthService
	eventLog    *streaming.StreamEventLog
)

func init() {
	if common.RunningUnitTests() {
		return
	}

	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName:    "sse",
		LambdaType:     common.LambdaTypeAPI,
		RequestTimeout: 15 * time.Minute,
	})
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	// Best-effort standardized init (currently uses placeholders; SSE relies on manual wiring below).
	if err := lambdaCtx.InitializeWithDefaults(); err != nil {
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
		manualDB, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
		if err != nil {
			logger.Fatal("failed to initialize DynamoDB client", zap.Error(err))
		}
		db = manualDB
	}

	var err error
	repos, err = factory.NewRepositoryFactory(db, tableName, logger)
	if err != nil {
		logger.Fatal("failed to create repository factory", zap.Error(err))
	}

	authService, err = auth.NewAuthService(cfg, repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	if cfg.StreamEventsTable == "" {
		logger.Fatal("STREAM_EVENTS_TABLE_NAME environment variable is required")
	}
	eventLog = streaming.NewStreamEventLog(db, 30*time.Minute)
}

func main() {
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

	lambda.Start(app.HandleRequest)
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
	go func() {
		defer close(eventCh)

		start := time.Now()
		heartbeat := time.NewTicker(streamHeartbeatEvery)
		defer heartbeat.Stop()

		var afterID = lastEventID

		for {
			if time.Since(start) > streamMaxDuration {
				return
			}

			items, err := eventLog.Query(ctx, streamName, afterID, streamPollLimit)
			if err != nil {
				eventCh <- lift.SSEEvent{Event: "error", Data: `{"error":"internal_error"}`}
				return
			}

			if len(items) > 0 {
				for _, item := range items {
					afterID = item.ID
					if onlyMedia && (item.Event == streamEventTypeUpdate || item.Event == streamEventTypeStatusUpdate) {
						if !payloadHasMedia(item.Data) {
							continue
						}
					}
					eventCh <- lift.SSEEvent{ID: item.ID, Event: item.Event, Data: normalizeDeletePayload(item.Event, item.Data)}
				}
				continue
			}

			select {
			case <-ctx.Context.Done():
				return
			case <-heartbeat.C:
				eventCh <- lift.SSEEvent{Event: "keepalive", Data: "thump"}
			case <-time.After(streamIdlePollDelay):
			}
		}
	}()

	return lift.SSEResponse(ctx, eventCh)
}

func requireClaims(ctx *lift.Context) (*auth.EnhancedClaims, error) {
	token := strings.TrimSpace(ctx.Header("Authorization"))
	if token == "" {
		return nil, ctx.Unauthorized("Authentication required", nil)
	}

	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ctx.Unauthorized("Authentication required", nil)
	}

	claims, err := authService.ValidateAccessToken(token)
	if err != nil {
		return nil, ctx.Unauthorized("Invalid token", err)
	}

	return claims, nil
}

func clientIP(ctx *lift.Context) string {
	if ctx == nil {
		return "unknown"
	}
	if forwarded := ctx.Header("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := ctx.Header("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	return "unknown"
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
