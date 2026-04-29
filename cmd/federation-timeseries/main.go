// Package main implements the federation-timeseries Lambda function for processing federation time-series data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/stream"
)

// TimeseriesProcessor handles time series data for federation metrics
type TimeseriesProcessor struct {
	db        dynamormCore.DB
	tableName string
	logger    *zap.Logger
}

// NewTimeseriesProcessor creates a new timeseries processor
func NewTimeseriesProcessor(db dynamormCore.DB, tableName string, logger *zap.Logger) *TimeseriesProcessor {
	return &TimeseriesProcessor{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

func (tp *TimeseriesProcessor) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if tp == nil {
		return fmt.Errorf("timeseries processor is nil")
	}
	if tp.logger == nil {
		tp.logger = zap.NewNop()
	}

	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if !tp.isFederationRecord(record) {
		return nil
	}

	timestamp := tp.extractTimestamp(record)
	if timestamp.IsZero() {
		return nil
	}

	windowSize := 5 * time.Minute
	window := timestamp.Truncate(windowSize)

	if err := tp.processWindow(runCtx, requestID, window, []events.DynamoDBEventRecord{record}); err != nil {
		tp.logger.Error("failed to process time window",
			zap.String("request_id", requestID),
			zap.Time("window", window),
			zap.Int("record_count", 1),
			zap.Error(err),
		)
		// Match previous Lift behavior: log and continue; do not fail the batch.
		return nil
	}

	return nil
}

func (tp *TimeseriesProcessor) isFederationRecord(record events.DynamoDBEventRecord) bool {
	if record.Change.NewImage == nil {
		return false
	}

	// Check if this is a federation-related record
	var item struct {
		PK   string `theorydb:"pk"`
		Type string `json:"type"`
	}

	if err := stream.UnmarshalItem(record, &item); err != nil {
		return false
	}

	// Check for federation activity patterns
	return strings.HasPrefix(item.PK, "ACTOR#") ||
		strings.HasPrefix(item.PK, "ACTIVITY#") ||
		strings.HasPrefix(item.PK, "FEDERATION#") ||
		item.Type == "Follow" ||
		item.Type == "Like" ||
		item.Type == "Announce"
}

func (tp *TimeseriesProcessor) extractTimestamp(record events.DynamoDBEventRecord) time.Time {
	// Try to get timestamp from the record
	var item struct {
		CreatedAt string `json:"created_at"`
		Timestamp string `json:"timestamp"`
		UpdatedAt string `json:"updated_at"`
	}

	if err := stream.UnmarshalItem(record, &item); err != nil {
		return time.Time{}
	}

	// Try different timestamp fields
	for _, timeStr := range []string{item.CreatedAt, item.Timestamp, item.UpdatedAt} {
		if timeStr != "" {
			if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
				return t
			}
		}
	}

	// Fallback to current time if no timestamp found
	// Note: DynamoDB streams don't always have precise timestamps

	return time.Time{}
}

func (tp *TimeseriesProcessor) processWindow(ctx context.Context, requestID string, window time.Time, records []events.DynamoDBEventRecord) error {
	// Aggregate metrics for this time window
	metrics := tp.aggregateMetrics(records)

	// Store aggregated metrics using DynamORM batch operations
	return tp.storeMetrics(ctx, requestID, window, metrics)
}

// FederationMetrics contains aggregated federation metrics
type FederationMetrics struct {
	FollowCount     int
	LikeCount       int
	AnnounceCount   int
	ActivityCount   int
	UniqueActors    map[string]bool
	UniqueInstances map[string]bool
}

func (tp *TimeseriesProcessor) aggregateMetrics(records []events.DynamoDBEventRecord) *FederationMetrics {
	metrics := &FederationMetrics{
		UniqueActors:    make(map[string]bool),
		UniqueInstances: make(map[string]bool),
	}

	for _, record := range records {
		var item struct {
			Type     string `json:"type"`
			ActorID  string `json:"actor_id"`
			Actor    string `json:"actor"`
			Activity string `json:"activity"`
		}

		if err := stream.UnmarshalItem(record, &item); err != nil {
			continue
		}

		// Count by activity type
		switch item.Type {
		case "Follow":
			metrics.FollowCount++
		case "Like":
			metrics.LikeCount++
		case "Announce":
			metrics.AnnounceCount++
		default:
			metrics.ActivityCount++
		}

		// Track unique actors
		if item.ActorID != "" {
			metrics.UniqueActors[item.ActorID] = true
		}
		if item.Actor != "" {
			metrics.UniqueActors[item.Actor] = true
		}

		// Extract instance from actor ID for unique instances
		if actorID := item.ActorID; actorID != "" {
			if instance := tp.extractInstance(actorID); instance != "" {
				metrics.UniqueInstances[instance] = true
			}
		}
	}

	return metrics
}

func (tp *TimeseriesProcessor) extractInstance(actorID string) string {
	// Extract instance domain from actor ID
	// e.g., "https://mastodon.social/users/alice" -> "mastodon.social"
	if strings.HasPrefix(actorID, "https://") {
		parts := strings.Split(actorID[8:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

func (tp *TimeseriesProcessor) storeMetrics(ctx context.Context, requestID string, window time.Time, metrics *FederationMetrics) error {
	now := time.Now().UTC()
	windowStr := window.Format(time.RFC3339)

	// Atomically add the current stream batch into the existing window instead
	// of overwriting previous records from the same 5-minute bucket.
	timeseriesRecord := models.NewFederationTimeseriesWindow(window, now)
	if err := tp.db.WithContext(ctx).Model(timeseriesRecord).UpdateBuilder().
		SetIfNotExists("Type", nil, timeseriesRecord.Type).
		SetIfNotExists("Window", nil, timeseriesRecord.Window).
		SetIfNotExists("CreatedAt", nil, timeseriesRecord.CreatedAt).
		Add("FollowCount", metrics.FollowCount).
		Add("LikeCount", metrics.LikeCount).
		Add("AnnounceCount", metrics.AnnounceCount).
		Add("ActivityCount", metrics.ActivityCount).
		Add("UniqueActorCount", len(metrics.UniqueActors)).
		Add("UniqueInstanceCount", len(metrics.UniqueInstances)).
		Set("UpdatedAt", timeseriesRecord.UpdatedAt).
		Set("TTL", timeseriesRecord.TTL).
		Execute(); err != nil {
		return fmt.Errorf("store timeseries metrics: %w", err)
	}

	// Also store per-instance metrics for detailed analytics
	for instance := range metrics.UniqueInstances {
		instanceRecord := models.NewInstanceTimeseriesWindow(instance, window, now)
		if err := tp.db.WithContext(ctx).Model(instanceRecord).UpdateBuilder().
			SetIfNotExists("Type", nil, instanceRecord.Type).
			SetIfNotExists("Instance", nil, instanceRecord.Instance).
			SetIfNotExists("Window", nil, instanceRecord.Window).
			SetIfNotExists("CreatedAt", nil, instanceRecord.CreatedAt).
			Set("UpdatedAt", instanceRecord.UpdatedAt).
			Set("TTL", instanceRecord.TTL).
			Execute(); err != nil {
			tp.logger.Error("failed to store instance metrics",
				zap.String("request_id", requestID),
				zap.String("instance", instance),
				zap.Error(err),
			)
			// Continue with other instances
		}
	}

	tp.logger.Info("stored federation timeseries metrics",
		zap.String("request_id", requestID),
		zap.String("window", windowStr),
		zap.Int("follow_count", metrics.FollowCount),
		zap.Int("like_count", metrics.LikeCount),
		zap.Int("announce_count", metrics.AnnounceCount),
		zap.Int("unique_actors", len(metrics.UniqueActors)),
		zap.Int("unique_instances", len(metrics.UniqueInstances)),
	)

	return nil
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	processor *TimeseriesProcessor
)

var (
	mustInitializeLambdaFn   = common.MustInitializeLambda
	initializeWithDefaultsFn = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	dynamormGetClientFn      = theorydb.GetClient
	lambdaStartFn            = lambda.Start
)

func initializeFederationTimeseries() {
	// Standardized Lambda initialization for federation-timeseries function
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "federation-timeseries",    // federation-timeseries
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	// Initialize with processor-specific defaults
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Function-specific initialization only
	// Initialize storage independently to avoid import cycles
	db, err := dynamormGetClientFn(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize processor
	processor = NewTimeseriesProcessor(db, cfg.DynamoTableName, logger)
}

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeFederationTimeseries()
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, handleFederationTimeseriesStreamRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleFederationTimeseriesStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if processor == nil {
		return fmt.Errorf("federation timeseries processor not initialized")
	}
	return processor.HandleDynamoDBRecord(ctx, record)
}
