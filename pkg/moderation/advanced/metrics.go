package advanced

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ModerationMetrics tracks moderation system performance
type ModerationMetrics struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// In-memory counters for current period
	counters  sync.Map
	startTime time.Time

	// Atomic counters for real-time metrics
	totalAnalyzed    atomic.Int64
	totalAllowed     atomic.Int64
	totalFlagged     atomic.Int64
	totalRemoved     atomic.Int64
	totalQuarantined atomic.Int64

	// Response time tracking
	responseTimes  []time.Duration
	responseTimeMu sync.Mutex
}

// NewModerationMetrics creates a new metrics tracker
func NewModerationMetrics(db *dynamodb.Client, tableName string, logger *zap.Logger) *ModerationMetrics {
	mm := &ModerationMetrics{
		db:            db,
		tableName:     tableName,
		logger:        logger,
		startTime:     time.Now(),
		responseTimes: make([]time.Duration, 0, 1000),
	}

	// Start periodic flush
	go mm.flushPeriodically()

	return mm
}

// RecordAnalysis records an analysis event
func (mm *ModerationMetrics) RecordAnalysis(ctx context.Context, contentType string, processingTime time.Duration, decision *ModerationDecision) {
	// Update atomic counters
	mm.totalAnalyzed.Add(1)

	switch decision.Decision {
	case ActionAllow:
		mm.totalAllowed.Add(1)
	case ActionFlag:
		mm.totalFlagged.Add(1)
	case ActionRemove:
		mm.totalRemoved.Add(1)
	case ActionQuarantine:
		mm.totalQuarantined.Add(1)
	}

	// Record response time
	mm.responseTimeMu.Lock()
	mm.responseTimes = append(mm.responseTimes, processingTime)
	if len(mm.responseTimes) > 10000 {
		// Keep only last 10k measurements
		mm.responseTimes = mm.responseTimes[5000:]
	}
	mm.responseTimeMu.Unlock()

	// Update detailed counters
	mm.incrementCounter(fmt.Sprintf("content_type:%s", contentType), 1)
	mm.incrementCounter(fmt.Sprintf("decision:%s", decision.Decision), 1)
	mm.incrementCounter(fmt.Sprintf("confidence:%.1f", roundToNearest(decision.Confidence, 0.1)), 1)

	// Track severity distribution
	for _, reason := range decision.Reasons {
		mm.incrementCounter(fmt.Sprintf("severity:%s", reason.Severity), 1)
		mm.incrementCounter(fmt.Sprintf("reason_type:%s", reason.Type), 1)
	}

	// Track review requirements
	if decision.RequiresReview {
		mm.incrementCounter("requires_review", 1)
		mm.incrementCounter(fmt.Sprintf("review_priority:%d", decision.ReviewPriority), 1)
	}

	// Store decision for later analysis if significant
	if decision.Decision != ActionAllow || decision.Confidence < 0.5 {
		mm.storeDecisionSample(ctx, decision, processingTime)
	}
}

// RecordFalsePositive records a false positive
func (mm *ModerationMetrics) RecordFalsePositive(ctx context.Context, contentID string, originalDecision *ModerationDecision) {
	mm.incrementCounter("false_positives", 1)
	mm.incrementCounter(fmt.Sprintf("false_positive:%s", originalDecision.Decision), 1)

	// Store for analysis
	item := map[string]types.AttributeValue{
		"PK":               &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#%s", time.Now().Format("2006-01-02"))},
		"SK":               &types.AttributeValueMemberS{Value: fmt.Sprintf("FP#%s", contentID)},
		"ContentID":        &types.AttributeValueMemberS{Value: contentID},
		"OriginalDecision": &types.AttributeValueMemberS{Value: string(originalDecision.Decision)},
		"Confidence":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", originalDecision.Confidence)},
		"Timestamp":        &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":              &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(mm.tableName),
		Item:      item,
	}

	_, err := mm.db.PutItem(ctx, putInput)
	if err != nil {
		mm.logger.Warn("failed to store false positive", zap.Error(err))
	}
}

// RecordTruePositive records a true positive (confirmed violation)
func (mm *ModerationMetrics) RecordTruePositive(ctx context.Context, contentID string, decision *ModerationDecision) {
	mm.incrementCounter("true_positives", 1)
	mm.incrementCounter(fmt.Sprintf("true_positive:%s", decision.Decision), 1)
}

// GetStats retrieves moderation statistics for a time range
func (mm *ModerationMetrics) GetStats(ctx context.Context, timeRange TimeRange) (*ModerationStats, error) {
	stats := &ModerationStats{
		TimeRange:      timeRange,
		ActionCounts:   make(map[ModerationAction]int64),
		CategoryCounts: make(map[string]int64),
		SeverityCounts: make(map[Severity]int64),
	}

	// Query aggregated stats from DynamoDB
	startDate := timeRange.Start.Format("2006-01-02")
	endDate := timeRange.End.Format("2006-01-02")

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(mm.tableName),
		KeyConditionExpression: aws.String("PK BETWEEN :start AND :end AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":start":  &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#%s", startDate)},
			":end":    &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#%s", endDate)},
			":prefix": &types.AttributeValueMemberS{Value: "STATS#"},
		},
	}

	result, err := mm.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}

	// Aggregate results
	for _, item := range result.Items {
		mm.aggregateStats(item, stats)
	}

	// Add current period stats if within range
	if mm.startTime.After(timeRange.Start) && mm.startTime.Before(timeRange.End) {
		mm.addCurrentPeriodStats(stats)
	}

	// Calculate average confidence
	if stats.TotalAnalyzed > 0 {
		stats.AverageConfidence = stats.AverageConfidence / float64(stats.TotalAnalyzed)
	}

	// Calculate average response time
	mm.responseTimeMu.Lock()
	if len(mm.responseTimes) > 0 {
		var total time.Duration
		for _, rt := range mm.responseTimes {
			total += rt
		}
		stats.ResponseTime = total / time.Duration(len(mm.responseTimes))
	}
	mm.responseTimeMu.Unlock()

	return stats, nil
}

// GetRealtimeStats returns current real-time statistics
func (mm *ModerationMetrics) GetRealtimeStats() *RealtimeStats {
	uptime := time.Since(mm.startTime)

	total := mm.totalAnalyzed.Load()
	if total == 0 {
		total = 1 // Prevent division by zero
	}

	stats := &RealtimeStats{
		Uptime:         uptime,
		TotalAnalyzed:  total,
		AnalysisRate:   float64(total) / uptime.Seconds(),
		AllowRate:      float64(mm.totalAllowed.Load()) / float64(total),
		FlagRate:       float64(mm.totalFlagged.Load()) / float64(total),
		RemoveRate:     float64(mm.totalRemoved.Load()) / float64(total),
		QuarantineRate: float64(mm.totalQuarantined.Load()) / float64(total),
	}

	// Calculate current response time
	mm.responseTimeMu.Lock()
	if len(mm.responseTimes) > 0 {
		// Get last 100 response times
		start := len(mm.responseTimes) - 100
		if start < 0 {
			start = 0
		}
		recent := mm.responseTimes[start:]

		var total time.Duration
		for _, rt := range recent {
			total += rt
		}
		stats.AvgResponseTime = total / time.Duration(len(recent))

		// Find p95
		if len(recent) >= 20 {
			sorted := make([]time.Duration, len(recent))
			copy(sorted, recent)
			// Simple bubble sort for small dataset
			for i := 0; i < len(sorted)-1; i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[i] > sorted[j] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			p95Index := int(float64(len(sorted)) * 0.95)
			stats.P95ResponseTime = sorted[p95Index]
		}
	}
	mm.responseTimeMu.Unlock()

	return stats
}

// GetTopPatterns returns the most frequently matched patterns
func (mm *ModerationMetrics) GetTopPatterns(ctx context.Context, limit int) ([]PatternStats, error) {
	// This would query pattern hit counts from the pattern matcher
	// For now, return empty
	return []PatternStats{}, nil
}

// Helper methods

func (mm *ModerationMetrics) incrementCounter(key string, delta int64) {
	val, _ := mm.counters.LoadOrStore(key, &atomic.Int64{})
	counter := val.(*atomic.Int64)
	counter.Add(delta)
}

func (mm *ModerationMetrics) getCounter(key string) int64 {
	val, ok := mm.counters.Load(key)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}

func (mm *ModerationMetrics) flushPeriodically() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := mm.flush(ctx); err != nil {
			mm.logger.Error("failed to flush metrics", zap.Error(err))
		}
	}
}

func (mm *ModerationMetrics) flush(ctx context.Context) error {
	// Get current date for partition key
	date := time.Now().Format("2006-01-02")
	hour := time.Now().Format("15")

	// Prepare batch write
	writeRequests := []types.WriteRequest{}

	// Flush all counters
	mm.counters.Range(func(key, value interface{}) bool {
		counter := value.(*atomic.Int64)
		count := counter.Swap(0) // Reset counter

		if count == 0 {
			return true
		}

		keyStr := key.(string)
		item := map[string]types.AttributeValue{
			"PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("METRICS#%s", date)},
			"SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("STATS#%s#%s", hour, keyStr)},
			"Count": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", count)},
			"Type":  &types.AttributeValueMemberS{Value: keyStr},
			"Hour":  &types.AttributeValueMemberS{Value: hour},
			"TTL":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})

		return true
	})

	// Write in batches of 25 (DynamoDB limit)
	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				mm.tableName: writeRequests[i:end],
			},
		}

		_, err := mm.db.BatchWriteItem(ctx, batchInput)
		if err != nil {
			mm.logger.Warn("failed to write metrics batch", zap.Error(err))
		}
	}

	mm.logger.Info("flushed metrics",
		zap.Int("counters", len(writeRequests)),
		zap.String("date", date),
		zap.String("hour", hour))

	return nil
}

func (mm *ModerationMetrics) storeDecisionSample(ctx context.Context, decision *ModerationDecision, processingTime time.Duration) {
	// Store a sample of decisions for analysis
	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("SAMPLES#%s", time.Now().Format("2006-01-02"))},
		"SK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", time.Now().UnixNano(), decision.ContentID)},
		"ContentID":      &types.AttributeValueMemberS{Value: decision.ContentID},
		"Decision":       &types.AttributeValueMemberS{Value: string(decision.Decision)},
		"Confidence":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", decision.Confidence)},
		"ProcessingTime": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", processingTime.Milliseconds())},
		"ReasonCount":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", len(decision.Reasons))},
		"RequiresReview": &types.AttributeValueMemberBOOL{Value: decision.RequiresReview},
		"Timestamp":      &types.AttributeValueMemberS{Value: decision.DecidedAt.Format(time.RFC3339)},
		"TTL":            &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(mm.tableName),
		Item:      item,
	}

	go func() {
		_, err := mm.db.PutItem(ctx, putInput)
		if err != nil {
			mm.logger.Warn("failed to store decision sample", zap.Error(err))
		}
	}()
}

func (mm *ModerationMetrics) aggregateStats(item map[string]types.AttributeValue, stats *ModerationStats) {
	// Parse counter type and value
	if typeVal, ok := item["Type"].(*types.AttributeValueMemberS); ok {
		if countVal, ok := item["Count"].(*types.AttributeValueMemberN); ok {
			var count int64
			if _, err := fmt.Sscanf(countVal.Value, "%d", &count); err != nil {
				mm.logger.Warn("failed to parse metric count",
					zap.String("type", typeVal.Value),
					zap.String("value", countVal.Value),
					zap.Error(err))
				return
			}

			// Aggregate by type
			switch {
			case typeVal.Value == "content_type:text":
				stats.TotalAnalyzed += count

			case typeVal.Value == "false_positives":
				stats.FalsePositives += count

			case typeVal.Value == "true_positives":
				stats.TruePositives += count

			case len(typeVal.Value) > 9 && typeVal.Value[:9] == "decision:":
				action := ModerationAction(typeVal.Value[9:])
				stats.ActionCounts[action] += count

			case len(typeVal.Value) > 9 && typeVal.Value[:9] == "severity:":
				severity := Severity(typeVal.Value[9:])
				stats.SeverityCounts[severity] += count

			case len(typeVal.Value) > 12 && typeVal.Value[:12] == "reason_type:":
				category := typeVal.Value[12:]
				stats.CategoryCounts[category] += count
			}
		}
	}
}

func (mm *ModerationMetrics) addCurrentPeriodStats(stats *ModerationStats) {
	// Add current in-memory stats
	stats.TotalAnalyzed += mm.totalAnalyzed.Load()
	stats.ActionCounts[ActionAllow] += mm.totalAllowed.Load()
	stats.ActionCounts[ActionFlag] += mm.totalFlagged.Load()
	stats.ActionCounts[ActionRemove] += mm.totalRemoved.Load()
	stats.ActionCounts[ActionQuarantine] += mm.totalQuarantined.Load()

	// Add other counters
	stats.FalsePositives += mm.getCounter("false_positives")
	stats.TruePositives += mm.getCounter("true_positives")
}

// RealtimeStats represents current real-time statistics
type RealtimeStats struct {
	Uptime          time.Duration
	TotalAnalyzed   int64
	AnalysisRate    float64 // per second
	AllowRate       float64
	FlagRate        float64
	RemoveRate      float64
	QuarantineRate  float64
	AvgResponseTime time.Duration
	P95ResponseTime time.Duration
}

// PatternStats represents pattern matching statistics
type PatternStats struct {
	PatternID   string
	PatternName string
	HitCount    int64
	LastHit     time.Time
}

func roundToNearest(value, nearest float64) float64 {
	return float64(int(value/nearest+0.5)) * nearest
}
