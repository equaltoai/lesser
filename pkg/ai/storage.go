package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	dateStr := analysis.AnalyzedAt.Format("2006-01-02")
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
	// Calculate time range based on period
	now := time.Now()
	var startTime time.Time

	switch period {
	case "hour":
		startTime = now.Add(-1 * time.Hour)
	case "day":
		startTime = now.Add(-24 * time.Hour)
	case "week":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "month":
		startTime = now.Add(-30 * 24 * time.Hour)
	default:
		startTime = now.Add(-24 * time.Hour) // Default to day
	}

	// Initialize stats
	stats := &AIStats{
		Period:            period,
		TotalAnalyses:     0,
		ToxicContent:      0,
		SpamDetected:      0,
		AIGenerated:       0,
		NSFWContent:       0,
		PIIDetected:       0,
		ModerationActions: make(map[string]int),
	}

	// Query each day in the period using GSI4
	currentDate := startTime
	for currentDate.Before(now) {
		dateStr := currentDate.Format("2006-01-02")

		// Query using GSI4 for this specific day
		resp, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("GSI4"),
			KeyConditionExpression: aws.String("GSI4PK = :pk AND GSI4SK >= :start"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("AI#ANALYSIS#%s", dateStr)},
				":start": &types.AttributeValueMemberS{Value: startTime.Format(time.RFC3339)},
			},
		})

		if err != nil {
			// Continue with next day if query fails
			currentDate = currentDate.Add(24 * time.Hour)
			continue
		}

		// Process items for this day
		for _, item := range resp.Items {
			var analysis AIAnalysis
			err := attributevalue.UnmarshalMap(item, &analysis)
			if err != nil {
				continue
			}

			// Only count if within our time range
			if analysis.AnalyzedAt.Before(startTime) || analysis.AnalyzedAt.After(now) {
				continue
			}

			stats.TotalAnalyses++

			// Count various detections
			if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.5 {
				stats.ToxicContent++
			}

			if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.5 {
				stats.SpamDetected++
			}

			if analysis.AIDetection != nil && analysis.AIDetection.AIGeneratedProbability > 0.5 {
				stats.AIGenerated++
			}

			if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
				stats.NSFWContent++
			}

			if analysis.TextAnalysis != nil && analysis.TextAnalysis.ContainsPII {
				stats.PIIDetected++
			}

			// Count moderation actions
			stats.ModerationActions[analysis.ModerationAction]++
		}

		// Move to next day
		currentDate = currentDate.Add(24 * time.Hour)
	}

	// Calculate rates
	if stats.TotalAnalyses > 0 {
		stats.ToxicityRate = float64(stats.ToxicContent) / float64(stats.TotalAnalyses)
		stats.SpamRate = float64(stats.SpamDetected) / float64(stats.TotalAnalyses)
		stats.AIContentRate = float64(stats.AIGenerated) / float64(stats.TotalAnalyses)
		stats.NSFWRate = float64(stats.NSFWContent) / float64(stats.TotalAnalyses)
	}

	return stats, nil
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
