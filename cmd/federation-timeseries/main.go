// Package main implements the federation-timeseries Lambda function for processing federation time-series data.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
)

// TimeseriesProcessor handles time series data for federation metrics
type TimeseriesProcessor struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewTimeseriesProcessor creates a new timeseries processor
func NewTimeseriesProcessor(db core.DB, tableName string) *TimeseriesProcessor {
	return &TimeseriesProcessor{
		db:        db,
		tableName: tableName,
		logger:    common.Logger(),
	}
}

// HandleStream implements the DynamoDBStreamHandler interface for Lift framework
func (tp *TimeseriesProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	requestID := ctx.GetRequestID()

	tp.logger.Info("processing federation timeseries stream event",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Group records by time window for aggregation
	windows := tp.groupByTimeWindow(event.Records)

	// Process each time window
	for window, records := range windows {
		if err := tp.processWindow(ctx, window, records); err != nil {
			tp.logger.Error("failed to process time window",
				zap.String("request_id", requestID),
				zap.Time("window", window),
				zap.Int("record_count", len(records)),
				zap.Error(err),
			)
			// Continue processing other windows
		}
	}

	return nil
}

func (tp *TimeseriesProcessor) groupByTimeWindow(records []events.DynamoDBEventRecord) map[time.Time][]events.DynamoDBEventRecord {
	windows := make(map[time.Time][]events.DynamoDBEventRecord)
	windowSize := 5 * time.Minute // 5-minute aggregation windows

	for _, record := range records {
		if !tp.isFederationRecord(record) {
			continue
		}

		// Extract timestamp from the record
		timestamp := tp.extractTimestamp(record)
		if timestamp.IsZero() {
			continue
		}

		// Round down to the nearest window boundary
		window := timestamp.Truncate(windowSize)
		windows[window] = append(windows[window], record)
	}

	return windows
}

func (tp *TimeseriesProcessor) isFederationRecord(record events.DynamoDBEventRecord) bool {
	if record.Change.NewImage == nil {
		return false
	}

	// Check if this is a federation-related record
	var item struct {
		PK   string `dynamorm:"pk"`
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

func (tp *TimeseriesProcessor) processWindow(ctx *lift.Context, window time.Time, records []events.DynamoDBEventRecord) error {
	// Aggregate metrics for this time window
	metrics := tp.aggregateMetrics(records)

	// Store aggregated metrics using DynamORM batch operations
	return tp.storeMetrics(ctx, window, metrics)
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

func (tp *TimeseriesProcessor) storeMetrics(ctx *lift.Context, window time.Time, metrics *FederationMetrics) error {
	windowStr := window.Format(time.RFC3339)

	// Create timeseries record for federation metrics
	timeseriesRecord := struct {
		PK                  string `dynamorm:"pk"`
		SK                  string `dynamorm:"sk"`
		Type                string `json:"type"`
		Window              string `json:"window"`
		FollowCount         int    `json:"follow_count"`
		LikeCount           int    `json:"like_count"`
		AnnounceCount       int    `json:"announce_count"`
		ActivityCount       int    `json:"activity_count"`
		UniqueActorCount    int    `json:"unique_actor_count"`
		UniqueInstanceCount int    `json:"unique_instance_count"`
		CreatedAt           string `json:"created_at"`
		TTL                 int64  `dynamorm:"ttl"`
	}{
		PK:                  "TIMESERIES#FEDERATION",
		SK:                  fmt.Sprintf("WINDOW#%s", windowStr),
		Type:                "FederationTimeseries",
		Window:              windowStr,
		FollowCount:         metrics.FollowCount,
		LikeCount:           metrics.LikeCount,
		AnnounceCount:       metrics.AnnounceCount,
		ActivityCount:       metrics.ActivityCount,
		UniqueActorCount:    len(metrics.UniqueActors),
		UniqueInstanceCount: len(metrics.UniqueInstances),
		CreatedAt:           time.Now().Format(time.RFC3339),
		TTL:                 time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}

	// Store the aggregated metrics with Lift context
	if err := tp.db.WithContext(ctx.Context).Model(&timeseriesRecord).Create(); err != nil {
		return lift.NewLiftError("TIMESERIES_STORE_FAILED", "failed to store timeseries metrics", 500).WithCause(err)
	}

	// Also store per-instance metrics for detailed analytics
	for instance := range metrics.UniqueInstances {
		instanceRecord := struct {
			PK        string `dynamorm:"pk"`
			SK        string `dynamorm:"sk"`
			Type      string `json:"type"`
			Instance  string `json:"instance"`
			Window    string `json:"window"`
			CreatedAt string `json:"created_at"`
			TTL       int64  `dynamorm:"ttl"`
		}{
			PK:        fmt.Sprintf("TIMESERIES#INSTANCE#%s", instance),
			SK:        fmt.Sprintf("WINDOW#%s", windowStr),
			Type:      "InstanceTimeseries",
			Instance:  instance,
			Window:    windowStr,
			CreatedAt: time.Now().Format(time.RFC3339),
			TTL:       time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days retention
		}

		if err := tp.db.WithContext(ctx.Context).Model(&instanceRecord).Create(); err != nil {
			tp.logger.Error("failed to store instance metrics",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("instance", instance),
				zap.Error(err),
			)
			// Continue with other instances
		}
	}

	tp.logger.Info("stored federation timeseries metrics",
		zap.String("request_id", ctx.GetRequestID()),
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
	logger    *zap.Logger
	cfg       *config.Config
	processor *TimeseriesProcessor
	db        core.DB
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize processor
	processor = NewTimeseriesProcessor(db, cfg.DynamoTableName)
}

func main() {
	// Use Lift's DynamoDB stream pattern with full middleware stack
	patterns.StartDynamoDBStreamLambda("federation-timeseries", processor, logger)
}
