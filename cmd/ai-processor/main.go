package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/pay-theory/dynamorm"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/ai"
	"github.com/aron23/lesser/pkg/common"
)

type AIProcessor struct {
	db        *dynamorm.LambdaDB
	tableName string
	aiService *ai.AIService
	logger    *zap.Logger
}

func NewAIProcessor() (*AIProcessor, error) {
	// Get table name from environment
	tableName := os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		tableName = "lesser-main"
	}

	// Initialize DynamORM with Lambda optimization
	db, err := dynamorm.NewLambdaOptimized()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Set timeout buffer to prevent Lambda timeouts
	if lambdaDB, ok := db.WithLambdaTimeoutBuffer(500 * time.Millisecond).(*dynamorm.LambdaDB); ok {
		db = lambdaDB
	}

	// Initialize AI service
	aiConfig := &ai.AIConfig{
		NSFWThreshold:       0.8,
		ToxicityThreshold:   0.7,
		SpamThreshold:       0.75,
		AIContentThreshold:  0.85,
		EnablePIIDetection:  true,
		EnableAIDetection:   true,
		EnableImageAnalysis: true,
		BedrockModelID:      "anthropic.claude-v2",
		S3Bucket:            os.Getenv("S3_BUCKET_NAME"),
	}

	// Load AWS config for AI service
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	aiService := ai.NewAIService(cfg, aiConfig)

	return &AIProcessor{
		db:        db,
		tableName: tableName,
		aiService: aiService,
		logger:    common.Logger(),
	}, nil
}

func (ap *AIProcessor) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	ap.logger.Info("processing DynamoDB stream event",
		zap.Int("record_count", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := ap.processRecord(ctx, record); err != nil {
			ap.logger.Error("error processing record",
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			// Continue processing other records
		}
	}
	return nil
}

func (ap *AIProcessor) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Only process objects with analyzable content
	if !ap.isAnalyzableRecord(record) {
		return nil
	}

	// Extract content from the stream record
	content, err := ap.extractContent(record)
	if err != nil {
		return fmt.Errorf("failed to extract content: %w", err)
	}

	// Perform AI analysis
	analysis, err := ap.aiService.AnalyzeContent(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to analyze content: %w", err)
	}

	// Store analysis result using DynamORM
	if err := ap.storeAnalysis(ctx, analysis); err != nil {
		return fmt.Errorf("failed to store analysis: %w", err)
	}

	// Handle moderation action if needed
	if analysis.ModerationAction != ai.ActionNone {
		if err := ap.handleModerationAction(ctx, analysis); err != nil {
			ap.logger.Error("failed to handle moderation action",
				zap.String("analysis_id", analysis.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (ap *AIProcessor) isAnalyzableRecord(record events.DynamoDBEventRecord) bool {
	// Check if this is an object we should analyze
	if record.Change.NewImage == nil {
		return false
	}

	// Try to unmarshal into a basic model to check PK
	var item struct {
		PK   string `dynamorm:"pk"`
		Type string `json:"type"`
	}

	if err := dynamorm.UnmarshalStreamImage(record.Change.NewImage, &item); err != nil {
		return false
	}

	// Check if it's an object and analyzable type
	if len(item.PK) > 7 && item.PK[:7] == "OBJECT#" {
		return ap.isAnalyzableType(item.Type)
	}

	return false
}

func (ap *AIProcessor) extractContent(record events.DynamoDBEventRecord) (*ai.Content, error) {
	// Unmarshal the stream record into a content model
	var item struct {
		PK      string `dynamorm:"pk"`
		Type    string `json:"type"`
		Content string `json:"content"`
		ActorID string `json:"actor_id"`
	}

	if err := dynamorm.UnmarshalStreamImage(record.Change.NewImage, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stream image: %w", err)
	}

	// Extract object ID from PK
	var objectID string
	if len(item.PK) > 7 && item.PK[:7] == "OBJECT#" {
		objectID = item.PK[7:]
	} else {
		return nil, fmt.Errorf("invalid object PK: %s", item.PK)
	}

	// Skip if not an analyzable type
	if !ap.isAnalyzableType(item.Type) {
		return nil, fmt.Errorf("not an analyzable type: %s", item.Type)
	}

	return &ai.Content{
		ID:        objectID,
		Type:      item.Type,
		Text:      item.Content,
		MediaURLs: []string{}, // TODO: Extract media URLs if present
		AuthorID:  item.ActorID,
		CreatedAt: time.Now(),
	}, nil
}

func (ap *AIProcessor) isAnalyzableType(objectType string) bool {
	switch objectType {
	case "Note", "Article", "Question", "Image", "Video":
		return true
	default:
		return false
	}
}

func (ap *AIProcessor) storeAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error {
	// Create analysis model for DynamORM
	analysisRecord := struct {
		PK               string `dynamorm:"pk"`
		SK               string `dynamorm:"sk"`
		Type             string `json:"type"`
		AnalysisID       string `json:"analysis_id"`
		ObjectID         string `json:"object_id"`
		ObjectType       string `json:"object_type"`
		OverallRisk      float64 `json:"overall_risk"`
		ModerationAction string `json:"moderation_action"`
		CreatedAt        string `json:"created_at"`
		TTL              int64  `dynamorm:"ttl"`
	}{
		PK:               fmt.Sprintf("ANALYSIS#%s", analysis.ObjectID),
		SK:               fmt.Sprintf("AI#%s", analysis.ID),
		Type:             "AIAnalysis",
		AnalysisID:       analysis.ID,
		ObjectID:         analysis.ObjectID,
		ObjectType:       analysis.ObjectType,
		OverallRisk:      analysis.OverallRisk,
		ModerationAction: string(analysis.ModerationAction),
		CreatedAt:        time.Now().Format(time.RFC3339),
		TTL:              time.Now().Add(30 * 24 * time.Hour).Unix(),
	}

	// Store using DynamORM Model Create
	return ap.db.Model(&analysisRecord).Create()
}

func (ap *AIProcessor) handleModerationAction(ctx context.Context, analysis *ai.AIAnalysis) error {
	// Create moderation event model for DynamORM
	moderationEvent := struct {
		PK              string  `dynamorm:"pk"`
		SK              string  `dynamorm:"sk"`
		Type            string  `json:"type"`
		EventID         string  `json:"event_id"`
		EventType       string  `json:"event_type"`
		ObjectID        string  `json:"object_id"`
		ObjectType      string  `json:"object_type"`
		ActorID         string  `json:"actor_id"`
		Category        string  `json:"category"`
		Severity        string  `json:"severity"`
		ConfidenceScore float64 `json:"confidence_score"`
		CreatedAt       string  `json:"created_at"`
		TTL             int64   `dynamorm:"ttl"`
	}{
		PK:              fmt.Sprintf("MODERATION#%s", analysis.ObjectID),
		SK:              fmt.Sprintf("EVENT#%s", analysis.ID),
		Type:            "ModerationEvent",
		EventID:         analysis.ID,
		EventType:       "flagged",
		ObjectID:        analysis.ObjectID,
		ObjectType:      analysis.ObjectType,
		ActorID:         "ai-processor",
		Category:        ap.determineCategory(analysis),
		Severity:        ap.determineSeverity(analysis),
		ConfidenceScore: analysis.OverallRisk,
		CreatedAt:       time.Now().Format(time.RFC3339),
		TTL:             time.Now().Add(30 * 24 * time.Hour).Unix(),
	}

	// Store moderation event using DynamORM Model Create
	return ap.db.Model(&moderationEvent).Create()
}

func (ap *AIProcessor) determineCategory(analysis *ai.AIAnalysis) string {
	if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.7 {
		return "spam"
	}
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.7 {
		return "hate_speech"
	}
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
		return "nsfw"
	}
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.ViolenceScore > 0.7 {
		return "violence"
	}
	return "other"
}

func (ap *AIProcessor) determineSeverity(analysis *ai.AIAnalysis) string {
	if analysis.OverallRisk > 0.9 {
		return "critical"
	}
	if analysis.OverallRisk > 0.7 {
		return "high"
	}
	if analysis.OverallRisk > 0.5 {
		return "medium"
	}
	return "low"
}

func main() {
	processor, err := NewAIProcessor()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize AI processor: %v", err))
	}

	// Handle DynamoDB stream events - use direct Lambda handler
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		defer func() {
			duration := time.Since(start)
			processor.logger.Info("request completed",
				zap.Duration("duration", duration),
			)
		}()
		return processor.HandleStream(ctx, event)
	})
}