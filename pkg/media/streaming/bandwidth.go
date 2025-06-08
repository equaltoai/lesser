package streaming

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// CostTracker interface for tracking AWS costs
type CostTracker interface {
	TrackDynamoRead(units int)
	TrackDynamoWrite(units int)
}

// BandwidthTracker tracks bandwidth usage for users
type BandwidthTracker struct {
	db          *dynamodb.Client
	tableName   string
	logger      *zap.Logger
	costTracker CostTracker

	// In-memory cache for active sessions
	sessionCache sync.Map
	cacheTTL     time.Duration
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker(db *dynamodb.Client, tableName string, logger *zap.Logger, costTracker CostTracker) *BandwidthTracker {
	return &BandwidthTracker{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		costTracker: costTracker,
		cacheTTL:    5 * time.Minute,
	}
}

// TrackBandwidth records bandwidth usage for a user
func (bt *BandwidthTracker) TrackBandwidth(ctx context.Context, userID string, bytesTransferred int64) error {
	now := time.Now()

	// Update in-memory cache first for real-time performance
	bt.updateCache(userID, bytesTransferred, now)

	// Prepare DynamoDB update
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(bt.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: "BANDWIDTH#CURRENT"},
		},
		UpdateExpression: aws.String(`
			SET TotalBytes = if_not_exists(TotalBytes, :zero) + :bytes,
			    SessionBytes = if_not_exists(SessionBytes, :zero) + :bytes,
			    LastMeasurement = :timestamp,
			    UpdatedAt = :timestamp,
			    #ttl = :ttl
			ADD MeasurementCount :one
		`),
		ExpressionAttributeNames: map[string]string{
			"#ttl": "TTL",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":bytes":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", bytesTransferred)},
			":zero":      &types.AttributeValueMemberN{Value: "0"},
			":one":       &types.AttributeValueMemberN{Value: "1"},
			":timestamp": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":ttl":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Add(30*24*time.Hour).Unix())},
		},
		ReturnValues: types.ReturnValueAllNew,
	}

	result, err := bt.db.UpdateItem(ctx, updateInput)
	if err != nil {
		bt.logger.Error("failed to track bandwidth",
			zap.String("user", userID),
			zap.Int64("bytes", bytesTransferred),
			zap.Error(err))
		return fmt.Errorf("track bandwidth: %w", err)
	}

	// Track cost
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoWrite(1)
	}

	// Log significant bandwidth usage
	if bytesTransferred > 10*1024*1024 { // 10MB
		bt.logger.Info("significant bandwidth usage",
			zap.String("user", userID),
			zap.Int64("bytes", bytesTransferred),
			zap.String("size_mb", fmt.Sprintf("%.2f", float64(bytesTransferred)/(1024*1024))))
	}

	return bt.calculateBandwidthMetrics(ctx, userID, result.Attributes)
}

// GetBandwidthStats retrieves bandwidth statistics for a user
func (bt *BandwidthTracker) GetBandwidthStats(ctx context.Context, userID string) (*BandwidthStats, error) {
	// Check cache first
	if cached, ok := bt.sessionCache.Load(userID); ok {
		if stats, ok := cached.(*cachedBandwidthStats); ok && time.Since(stats.lastUpdate) < bt.cacheTTL {
			return &stats.BandwidthStats, nil
		}
	}

	// Query DynamoDB
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(bt.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: "BANDWIDTH#CURRENT"},
		},
	}

	result, err := bt.db.GetItem(ctx, getInput)
	if err != nil {
		bt.logger.Error("failed to get bandwidth stats",
			zap.String("user", userID),
			zap.Error(err))
		return nil, fmt.Errorf("get bandwidth stats: %w", err)
	}

	// Track cost
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoRead(1)
	}

	if result.Item == nil {
		// Return default stats for new user
		return &BandwidthStats{
			UserID:            userID,
			TotalBytes:        0,
			SessionBytes:      0,
			AverageBandwidth:  0,
			PeakBandwidth:     0,
			LastMeasurement:   time.Now(),
			MeasurementWindow: 5 * time.Minute,
		}, nil
	}

	return bt.parseBandwidthStats(result.Item)
}

// GetOptimalQuality determines the best quality based on user's bandwidth
func (bt *BandwidthTracker) GetOptimalQuality(ctx context.Context, userID string, availableBandwidth int) Quality {
	// If availableBandwidth is provided, use it directly
	if availableBandwidth > 0 {
		return bt.selectQualityByBandwidth(availableBandwidth)
	}

	// Otherwise, get historical stats
	stats, err := bt.GetBandwidthStats(ctx, userID)
	if err != nil {
		bt.logger.Warn("failed to get bandwidth stats, using default quality",
			zap.String("user", userID),
			zap.Error(err))
		return Quality480p // Safe default
	}

	// Use average bandwidth with safety margin
	safeBandwidth := int(float64(stats.AverageBandwidth) * 0.8) // 80% of average

	return bt.selectQualityByBandwidth(safeBandwidth)
}

// RecordBandwidthMeasurement records a bandwidth measurement sample
func (bt *BandwidthTracker) RecordBandwidthMeasurement(ctx context.Context, userID string, bandwidth int) error {
	now := time.Now()

	// Store measurement sample
	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(bt.tableName),
		Item: map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("BANDWIDTH#SAMPLE#%d", now.UnixNano())},
			"Bandwidth": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", bandwidth)},
			"Timestamp": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Add(24*time.Hour).Unix())},
		},
	}

	_, err := bt.db.PutItem(ctx, putInput)
	if err != nil {
		bt.logger.Error("failed to record bandwidth measurement",
			zap.String("user", userID),
			zap.Int("bandwidth", bandwidth),
			zap.Error(err))
		return fmt.Errorf("record bandwidth measurement: %w", err)
	}

	// Track cost
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoWrite(1)
	}

	// Update peak bandwidth if necessary
	return bt.updatePeakBandwidth(ctx, userID, bandwidth)
}

// GetBandwidthHistory retrieves bandwidth measurement history
func (bt *BandwidthTracker) GetBandwidthHistory(ctx context.Context, userID string, duration time.Duration) ([]BandwidthMeasurement, error) {
	startTime := time.Now().Add(-duration)

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(bt.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			":start": &types.AttributeValueMemberS{Value: fmt.Sprintf("BANDWIDTH#SAMPLE#%d", startTime.UnixNano())},
			":end":   &types.AttributeValueMemberS{Value: fmt.Sprintf("BANDWIDTH#SAMPLE#%d", time.Now().UnixNano())},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(100),  // Limit to last 100 samples
	}

	result, err := bt.db.Query(ctx, queryInput)
	if err != nil {
		bt.logger.Error("failed to query bandwidth history",
			zap.String("user", userID),
			zap.Error(err))
		return nil, fmt.Errorf("query bandwidth history: %w", err)
	}

	// Track cost
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoRead(len(result.Items))
	}

	measurements := make([]BandwidthMeasurement, 0, len(result.Items))
	for _, item := range result.Items {
		measurement, err := bt.parseBandwidthMeasurement(item)
		if err != nil {
			bt.logger.Warn("failed to parse bandwidth measurement",
				zap.Error(err))
			continue
		}
		measurements = append(measurements, *measurement)
	}

	return measurements, nil
}

// Helper types and methods

type cachedBandwidthStats struct {
	BandwidthStats
	lastUpdate time.Time
}

type BandwidthMeasurement struct {
	UserID    string
	Bandwidth int
	Timestamp time.Time
}

func (bt *BandwidthTracker) updateCache(userID string, bytesTransferred int64, now time.Time) {
	cached, _ := bt.sessionCache.LoadOrStore(userID, &cachedBandwidthStats{
		BandwidthStats: BandwidthStats{
			UserID:            userID,
			LastMeasurement:   now,
			MeasurementWindow: 5 * time.Minute,
		},
		lastUpdate: now,
	})

	stats := cached.(*cachedBandwidthStats)
	stats.SessionBytes += bytesTransferred
	stats.TotalBytes += bytesTransferred
	stats.lastUpdate = now

	// Calculate bandwidth based on time window
	if stats.LastMeasurement.IsZero() {
		stats.LastMeasurement = now
	} else {
		elapsed := now.Sub(stats.LastMeasurement).Seconds()
		if elapsed > 0 {
			// Calculate bandwidth in kbps
			bandwidth := int((float64(bytesTransferred) * 8) / (elapsed * 1000))

			// Update average bandwidth (exponential moving average)
			if stats.AverageBandwidth == 0 {
				stats.AverageBandwidth = bandwidth
			} else {
				stats.AverageBandwidth = (stats.AverageBandwidth*3 + bandwidth) / 4
			}

			// Update peak bandwidth
			if bandwidth > stats.PeakBandwidth {
				stats.PeakBandwidth = bandwidth
			}
		}
	}
}

func (bt *BandwidthTracker) calculateBandwidthMetrics(ctx context.Context, userID string, attributes map[string]types.AttributeValue) error {
	// Get recent measurements for more accurate calculation
	measurements, err := bt.GetBandwidthHistory(ctx, userID, 5*time.Minute)
	if err != nil {
		return err
	}

	if len(measurements) == 0 {
		return nil
	}

	// Calculate average bandwidth from recent samples
	var totalBandwidth int
	for _, m := range measurements {
		totalBandwidth += m.Bandwidth
	}
	avgBandwidth := totalBandwidth / len(measurements)

	// Update the main record with calculated metrics
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(bt.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: "BANDWIDTH#CURRENT"},
		},
		UpdateExpression: aws.String("SET AverageBandwidth = :avg"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":avg": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", avgBandwidth)},
		},
	}

	_, err = bt.db.UpdateItem(ctx, updateInput)
	return err
}

func (bt *BandwidthTracker) updatePeakBandwidth(ctx context.Context, userID string, bandwidth int) error {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(bt.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: "BANDWIDTH#CURRENT"},
		},
		UpdateExpression:    aws.String("SET PeakBandwidth = if_not_exists(PeakBandwidth, :zero)"),
		ConditionExpression: aws.String("attribute_not_exists(PeakBandwidth) OR PeakBandwidth < :bandwidth"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":bandwidth": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", bandwidth)},
			":zero":      &types.AttributeValueMemberN{Value: "0"},
		},
	}

	// Update peak if higher
	updateInput.UpdateExpression = aws.String("SET PeakBandwidth = :bandwidth")

	_, err := bt.db.UpdateItem(ctx, updateInput)
	if err != nil {
		// Ignore conditional check failures
		var ccf *types.ConditionalCheckFailedException
		if !errors.As(err, &ccf) {
			return err
		}
	}

	return nil
}

func (bt *BandwidthTracker) selectQualityByBandwidth(bandwidth int) Quality {
	// Quality selection with buffer for stability
	switch {
	case bandwidth >= 20000:
		return Quality4K
	case bandwidth >= 8000:
		return Quality1080p
	case bandwidth >= 4000:
		return Quality720p
	case bandwidth >= 2000:
		return Quality480p
	case bandwidth >= 1000:
		return Quality360p
	default:
		return Quality240p
	}
}

func (bt *BandwidthTracker) parseBandwidthStats(item map[string]types.AttributeValue) (*BandwidthStats, error) {
	stats := &BandwidthStats{
		MeasurementWindow: 5 * time.Minute,
	}

	// Parse UserID from PK
	if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
		stats.UserID = strings.TrimPrefix(pk.Value, "USER#")
	}

	// Parse numeric fields
	if v, ok := item["TotalBytes"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &stats.TotalBytes)
	}
	if v, ok := item["SessionBytes"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &stats.SessionBytes)
	}
	if v, ok := item["AverageBandwidth"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &stats.AverageBandwidth)
	}
	if v, ok := item["PeakBandwidth"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &stats.PeakBandwidth)
	}

	// Parse timestamp
	if v, ok := item["LastMeasurement"].(*types.AttributeValueMemberS); ok {
		stats.LastMeasurement, _ = time.Parse(time.RFC3339, v.Value)
	}

	return stats, nil
}

func (bt *BandwidthTracker) parseBandwidthMeasurement(item map[string]types.AttributeValue) (*BandwidthMeasurement, error) {
	measurement := &BandwidthMeasurement{}

	// Parse UserID from PK
	if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
		measurement.UserID = strings.TrimPrefix(pk.Value, "USER#")
	}

	// Parse bandwidth
	if v, ok := item["Bandwidth"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &measurement.Bandwidth)
	}

	// Parse timestamp
	if v, ok := item["Timestamp"].(*types.AttributeValueMemberS); ok {
		measurement.Timestamp, _ = time.Parse(time.RFC3339, v.Value)
	}

	return measurement, nil
}
