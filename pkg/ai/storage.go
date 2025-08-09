package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
)

// Storage handles DynamoDB operations for AI analysis
type Storage struct {
	client    *dynamodb.Client
	tableName string
}

// NewStorage creates a new AI storage instance
func NewStorage(client *dynamodb.Client, tableName string) *Storage {
	return &Storage{
		client:    client,
		tableName: tableName,
	}
}

// SaveAnalysis stores AI analysis results in DynamoDB
func (s *Storage) SaveAnalysis(ctx context.Context, analysis *AIAnalysis) error {
	item, err := attributevalue.MarshalMap(analysis)
	if err != nil {
		return fmt.Errorf("failed to marshal analysis: %w", err)
	}

	// Add partition key and sort key
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#%s", analysis.ObjectID)}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("ANALYSIS#%s", analysis.ID)}
	item["Type"] = &types.AttributeValueMemberS{Value: "AIAnalysis"}
	item["CreatedAt"] = &types.AttributeValueMemberS{Value: analysis.AnalyzedAt.Format(time.RFC3339)}

	// Add GSI4 attributes for temporal queries (using the Cost Tracking GSI)
	// GSI4PK: AI analysis by date for cost/stats tracking
	// GSI4SK: Timestamp for ordering
	dateStr := analysis.AnalyzedAt.Format(common.DateFormat)
	item["GSI4PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#ANALYSIS#%s", dateStr)}
	item["GSI4SK"] = &types.AttributeValueMemberS{Value: analysis.AnalyzedAt.Format(time.RFC3339Nano)}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	return err
}

// GetAnalysis retrieves AI analysis for an object
func (s *Storage) GetAnalysis(ctx context.Context, objectID string) (*AIAnalysis, error) {
	// Query for the most recent analysis
	resp, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#%s", objectID)},
			":sk": &types.AttributeValueMemberS{Value: "ANALYSIS#"},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(1),
	})

	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("no analysis found for object %s", objectID)
	}

	var analysis AIAnalysis
	err = attributevalue.UnmarshalMap(resp.Items[0], &analysis)
	if err != nil {
		return nil, err
	}

	return &analysis, nil
}

// GetAnalysisByID retrieves a specific AI analysis by ID
func (s *Storage) GetAnalysisByID(ctx context.Context, objectID, analysisID string) (*AIAnalysis, error) {
	resp, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANALYSIS#%s", analysisID)},
		},
	})

	if err != nil {
		return nil, err
	}

	if resp.Item == nil {
		return nil, fmt.Errorf("analysis not found")
	}

	var analysis AIAnalysis
	err = attributevalue.UnmarshalMap(resp.Item, &analysis)
	if err != nil {
		return nil, err
	}

	return &analysis, nil
}

// GetStats retrieves AI analysis statistics for a given period
func (s *Storage) GetStats(ctx context.Context, period string) (*AIStats, error) {
	// Calculate time range
	now := time.Now()
	startTime := s.calculateStartTime(now, period)

	// Initialize stats
	stats := s.initializeStats(period)

	// Query and process data for each day
	s.processStatsForPeriod(ctx, startTime, now, stats)

	// Calculate rates
	s.calculateStatsRates(stats)

	return stats, nil
}

// calculateStartTime calculates the start time based on period
func (s *Storage) calculateStartTime(now time.Time, period string) time.Time {
	switch period {
	case "hour":
		return now.Add(-1 * time.Hour)
	case "day":
		return now.Add(-24 * time.Hour)
	case "week":
		return now.Add(-7 * 24 * time.Hour)
	case "month":
		return now.Add(-30 * 24 * time.Hour)
	default:
		return now.Add(-24 * time.Hour) // Default to day
	}
}

// initializeStats creates a new AIStats structure
func (s *Storage) initializeStats(period string) *AIStats {
	return &AIStats{
		Period:            period,
		TotalAnalyses:     0,
		ToxicContent:      0,
		SpamDetected:      0,
		AIGenerated:       0,
		NSFWContent:       0,
		PIIDetected:       0,
		ModerationActions: make(map[string]int),
	}
}

// processStatsForPeriod queries and processes stats for the time period
func (s *Storage) processStatsForPeriod(ctx context.Context, startTime, endTime time.Time, stats *AIStats) {
	currentDate := startTime
	for currentDate.Before(endTime) {
		s.processStatsForDay(ctx, currentDate, startTime, endTime, stats)
		currentDate = currentDate.Add(24 * time.Hour)
	}
}

// processStatsForDay processes stats for a single day
func (s *Storage) processStatsForDay(ctx context.Context, date, startTime, endTime time.Time, stats *AIStats) {
	dateStr := date.Format(common.DateFormat)
	resp, err := s.queryDayStats(ctx, dateStr, startTime)
	if err != nil {
		return
	}

	for _, item := range resp.Items {
		s.processAnalysisItem(item, startTime, endTime, stats)
	}
}

// queryDayStats queries statistics for a specific day
func (s *Storage) queryDayStats(ctx context.Context, dateStr string, startTime time.Time) (*dynamodb.QueryOutput, error) {
	return s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI4"),
		KeyConditionExpression: aws.String("GSI4PK = :pk AND GSI4SK >= :start"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#ANALYSIS#%s", dateStr)},
			":start": &types.AttributeValueMemberS{Value: startTime.Format(time.RFC3339)},
		},
	})
}

// processAnalysisItem processes a single analysis item
func (s *Storage) processAnalysisItem(item map[string]types.AttributeValue, startTime, endTime time.Time, stats *AIStats) {
	var analysis AIAnalysis
	if err := attributevalue.UnmarshalMap(item, &analysis); err != nil {
		return
	}

	// Skip if outside time range
	if !s.isWithinTimeRange(analysis.AnalyzedAt, startTime, endTime) {
		return
	}

	stats.TotalAnalyses++
	s.updateStatsCounters(&analysis, stats)
}

// isWithinTimeRange checks if a time is within the specified range
func (s *Storage) isWithinTimeRange(t, start, end time.Time) bool {
	return !t.Before(start) && !t.After(end)
}

// updateStatsCounters updates various detection counters
func (s *Storage) updateStatsCounters(analysis *AIAnalysis, stats *AIStats) {
	s.checkToxicContent(analysis, stats)
	s.checkSpamContent(analysis, stats)
	s.checkAIGenerated(analysis, stats)
	s.checkNSFWContent(analysis, stats)
	s.checkPIIContent(analysis, stats)
	stats.ModerationActions[analysis.ModerationAction]++
}

// checkToxicContent checks and counts toxic content
func (s *Storage) checkToxicContent(analysis *AIAnalysis, stats *AIStats) {
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.5 {
		stats.ToxicContent++
	}
}

// checkSpamContent checks and counts spam content
func (s *Storage) checkSpamContent(analysis *AIAnalysis, stats *AIStats) {
	if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.5 {
		stats.SpamDetected++
	}
}

// checkAIGenerated checks and counts AI-generated content
func (s *Storage) checkAIGenerated(analysis *AIAnalysis, stats *AIStats) {
	if analysis.AIDetection != nil && analysis.AIDetection.AIGeneratedProbability > 0.5 {
		stats.AIGenerated++
	}
}

// checkNSFWContent checks and counts NSFW content
func (s *Storage) checkNSFWContent(analysis *AIAnalysis, stats *AIStats) {
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
		stats.NSFWContent++
	}
}

// checkPIIContent checks and counts PII content
func (s *Storage) checkPIIContent(analysis *AIAnalysis, stats *AIStats) {
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ContainsPII {
		stats.PIIDetected++
	}
}

// calculateStatsRates calculates percentage rates for the stats
func (s *Storage) calculateStatsRates(stats *AIStats) {
	if stats.TotalAnalyses == 0 {
		return
	}

	total := float64(stats.TotalAnalyses)
	stats.ToxicityRate = float64(stats.ToxicContent) / total
	stats.SpamRate = float64(stats.SpamDetected) / total
	stats.AIContentRate = float64(stats.AIGenerated) / total
	stats.NSFWRate = float64(stats.NSFWContent) / total
}

// MarkAnalyzed updates the last analyzed timestamp for content
func (s *Storage) MarkAnalyzed(ctx context.Context, objectID string) error {
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
		},
		UpdateExpression: aws.String("SET LastAnalyzed = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

// AIStats represents aggregated statistics
//
//nolint:revive // AI prefix clarifies this is AI-related statistics
type AIStats struct {
	Period            string         `json:"period"`
	TotalAnalyses     int            `json:"total_analyses"`
	ToxicContent      int            `json:"toxic_content"`
	SpamDetected      int            `json:"spam_detected"`
	AIGenerated       int            `json:"ai_generated"`
	NSFWContent       int            `json:"nsfw_content"`
	PIIDetected       int            `json:"pii_detected"`
	ToxicityRate      float64        `json:"toxicity_rate"`
	SpamRate          float64        `json:"spam_rate"`
	AIContentRate     float64        `json:"ai_content_rate"`
	NSFWRate          float64        `json:"nsfw_rate"`
	ModerationActions map[string]int `json:"moderation_actions"`
}
