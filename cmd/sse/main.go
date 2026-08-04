// Package main implements the sse Lambda function providing Mastodon-compatible
// Server-Sent Events (SSE) endpoints via API Gateway REST API (v1) response streaming.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
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

	oauthDeviceStreamEventType = "oauth.device"
	oauthDeviceStreamPollDelay = 1 * time.Second
	oauthDeviceCodeHeaderKey   = "x-lesser-device-code"
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
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = factory.NewRepositoryFactory
	newAuthServiceFn           = func(cfg *config.Config, repos core.RepositoryStorage) (accessTokenValidator, error) {
		return auth.NewAuthService(cfg, repos)
	}
	newStreamEventLogFn = func(db dynamormCore.DB, ttl time.Duration) streamEventLog {
		return streaming.NewStreamEventLog(db, ttl)
	}
	lambdaStartFn           = lambda.Start
	timeAfterFn             = time.After
	authorizeListStreamFn   = authorizeListStream
	getOAuthDeviceSessionFn = func(ctx context.Context, deviceCodeHash string) (*storage.OAuthDeviceSession, error) {
		if repos == nil || repos.Account() == nil {
			return nil, apperrors.Internal("repositories not initialized")
		}
		return repos.Account().GetOAuthDeviceSession(ctx, deviceCodeHash)
	}
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

	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:         "sse",
		RequireRepositories: true,
		NewDB:               newLambdaOptimizedClientFn,
		NewRepositoryStorage: func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
			return newRepositoryFactoryFn(db, tableName, logger)
		},
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
	repos = deps.Repos

	authService, err = newAuthServiceFn(cfg, repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	if cfg.StreamEventsTable == "" {
		logger.Fatal("STREAM_EVENTS_TABLE_NAME environment variable is required")
	}
	eventLog = newStreamEventLogFn(deps.DB, 30*time.Minute)
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
				"X-Lesser-Device-Code",
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
	app.Get("/api/v1/streaming/oauth/device", handleOAuthDeviceStream)

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

func handleOAuthDeviceStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	if cfg == nil || !cfg.AllowDeviceFlow {
		return apptheory.MustJSON(http.StatusForbidden, map[string]string{
			"error":             "access_denied",
			"error_description": "device authorization is disabled",
		}), nil
	}

	deviceCode := strings.TrimSpace(sseHeaderValue(ctx, oauthDeviceCodeHeaderKey))
	if deviceCode == "" {
		return apptheory.MustJSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "x-lesser-device-code header is required",
		}), nil
	}

	eventCh := make(chan apptheory.SSEEvent, 8)
	streamCtx := context.Background()
	if ctx != nil {
		streamCtx = ctx.Context()
	}

	go produceOAuthDeviceStreamEvents(streamCtx, eventCh, oauthDeviceCodeHash(deviceCode))
	return apptheory.SSEStreamResponse(streamCtx, http.StatusOK, eventCh)
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
		onlyMedia := ctx != nil && sseQueryParam(ctx, "only_media") == "true"
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

		onlyMedia := ctx != nil && sseQueryParam(ctx, "only_media") == "true"
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

	if err := authorizeListStreamFn(ctx.Context(), listID, claims.Username); err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) || apperrors.HasCode(err, apperrors.CodeForbidden) {
			return apptheory.Text(http.StatusNotFound, "Not Found"), nil
		}
		logger.Warn("failed to authorize list stream",
			zap.String("list_id", listID),
			zap.String("username", claims.Username),
			zap.Error(err),
		)
		return apptheory.Text(http.StatusInternalServerError, "Internal Server Error"), nil
	}
	return streamSSE(ctx, streaming.ListStreamName(listID), false)
}

func handleDirectStream(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp := requireClaims(ctx)
	if resp != nil {
		return resp, nil
	}
	return streamSSE(ctx, streaming.DirectStreamName(claims.Username), false)
}

func authorizeListStream(ctx context.Context, listID, username string) error {
	if repos == nil {
		return apperrors.Internal("repositories not initialized")
	}

	listRepo := repos.List()
	if listRepo == nil {
		return apperrors.Internal("list repository not initialized")
	}

	list, err := listRepo.GetList(ctx, listID)
	if err != nil {
		return err
	}

	if list == nil || list.Username != username {
		return apperrors.NotFound("list")
	}

	return nil
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

func produceOAuthDeviceStreamEvents(ctx context.Context, eventCh chan<- apptheory.SSEEvent, deviceCodeHash string) {
	defer close(eventCh)

	if strings.TrimSpace(deviceCodeHash) == "" {
		eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"invalid"}`}
		return
	}

	heartbeat := time.NewTicker(streamHeartbeatEvery)
	defer heartbeat.Stop()

	start := time.Now()
	sentPending := false

	for time.Since(start) <= streamMaxDuration {
		session, err := getOAuthDeviceSessionFn(ctx, deviceCodeHash)
		if err != nil || session == nil {
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"invalid"}`}
			return
		}

		now := time.Now().UTC()
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"expired"}`}
			return
		}

		status := strings.ToLower(strings.TrimSpace(session.Status))
		switch status {
		case "approved":
			payload := map[string]any{"status": "approved"}
			if username := strings.TrimSpace(session.ApprovedUsername); username != "" {
				payload["username"] = username
			}
			data, _ := json.Marshal(payload)
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: string(data)}
			return
		case "denied":
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"denied"}`}
			return
		case "consumed":
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"consumed"}`}
			return
		case "", "pending":
			if !sentPending {
				payload := map[string]any{"status": "pending"}
				if !session.ExpiresAt.IsZero() {
					payload["expires_at"] = session.ExpiresAt.UTC().Format(time.RFC3339)
				}
				data, _ := json.Marshal(payload)
				eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: string(data)}
				sentPending = true
			}
		default:
			eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"invalid"}`}
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			eventCh <- apptheory.SSEEvent{Event: "keepalive", Data: "thump"}
		case <-timeAfterFn(oauthDeviceStreamPollDelay):
		}
	}

	eventCh <- apptheory.SSEEvent{Event: oauthDeviceStreamEventType, Data: `{"status":"expired"}`}
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

func oauthDeviceCodeHash(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return hex.EncodeToString(sum[:])
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
