// Package main implements an AI-powered content analysis processor that monitors
// DynamoDB streams for new content, performs toxicity detection, spam analysis,
// and automated moderation actions using AWS Bedrock. It processes INSERT and
// MODIFY events from the stream, analyzing text and media content to ensure
// community guidelines compliance and protect users from harmful content.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	lesserConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	aiService "github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
)

// AIProcessor handles AI-based content analysis for posts and media in the system.
// It integrates with AWS Bedrock to perform toxicity detection, spam filtering,
// and automated moderation decisions based on configurable thresholds.
type AIProcessor struct {
	db            core.DB
	tableName     string
	aiAnalyzer    *ai.AIService // For AI analysis (Comprehend, Rekognition, etc.)
	aiService     *aiService.Service // For storage and event publishing
	serviceRegistry *services.Registry
	logger        *zap.Logger
}

// NewAIProcessor creates a new AI processor instance configured with the specified
// database connection, DynamoDB table name, and AI service. The processor will
// analyze content from stream events and store results for moderation workflows.
func NewAIProcessor(db core.DB, tableName string, aiAnalyzer *ai.AIService, serviceRegistry *services.Registry) *AIProcessor {
	return &AIProcessor{
		db:            db,
		tableName:     tableName,
		aiAnalyzer:    aiAnalyzer,
		aiService:     serviceRegistry.AI(),
		serviceRegistry: serviceRegistry,
		logger:        common.Logger(),
	}
}

// HandleStream processes DynamoDB stream events with Lift-style patterns
func (ap *AIProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	requestID := ctx.GetRequestID()

	ap.logger.Info("processing AI analysis stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := ap.processRecord(ctx, record); err != nil {
			ap.logger.Error("error processing record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			// Continue processing other records
		}
	}
	return nil
}

func (ap *AIProcessor) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
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

	// Perform AI analysis using the analyzer (AWS services)
	analysis, err := ap.aiAnalyzer.AnalyzeContent(ctx.Request.Context(), content)
	if err != nil {
		return fmt.Errorf("failed to analyze content: %w", err)
	}

	// Store analysis and publish events using the service layer
	saveCmd := &aiService.SaveAnalysisCommand{
		Analysis: analysis,
		UserID:   content.AuthorID, // Use author as context for events
	}
	_, err = ap.aiService.SaveAnalysis(ctx.Request.Context(), saveCmd)
	if err != nil {
		return fmt.Errorf("failed to save analysis: %w", err)
	}

	// Handle moderation action if needed
	if analysis.ModerationAction != ai.ActionNone {
		if err := ap.handleModerationAction(ctx, analysis); err != nil {
			requestID := ctx.GetRequestID()
			ap.logger.Error("failed to handle moderation action",
				zap.String("request_id", requestID),
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

	if err := stream.UnmarshalItem(record, &item); err != nil {
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
		PK         string `dynamorm:"pk"`
		Type       string `json:"type"`
		Content    string `json:"content"`
		ActorID    string `json:"actor_id"`
		Attachment []struct {
			Type      string `json:"type"`
			MediaType string `json:"mediaType"`
			URL       string `json:"url"`
		} `json:"attachment"`
	}

	if err := stream.UnmarshalItem(record, &item); err != nil {
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

	// Extract media URLs from attachments
	var mediaURLs []string
	for _, att := range item.Attachment {
		if att.URL != "" && (att.Type == "Image" || att.Type == "Video" || att.Type == "Document") {
			mediaURLs = append(mediaURLs, att.URL)
		}
	}

	return &ai.Content{
		ID:        objectID,
		Type:      item.Type,
		Text:      item.Content,
		MediaURLs: mediaURLs,
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

// storeAnalysis is no longer needed - the service layer handles storage

func (ap *AIProcessor) handleModerationAction(ctx *lift.Context, analysis *ai.AIAnalysis) error {
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

	// Store moderation event using DynamORM Model Create (use underlying context)
	return ap.db.WithContext(ctx.Request.Context()).Model(&moderationEvent).Create()
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

var (
	logger    *zap.Logger
	cfg       *lesserConfig.Config
	processor *AIProcessor
	db        core.DB
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = lesserConfig.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
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
		S3Bucket:            cfg.S3BucketName,
	}

	// Load AWS config for AI service
	awsConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Create AI analyzer (for AWS AI services)
	aiAnalyzer := ai.NewAIService(awsConfig, aiConfig)

	// Create repository factory for storage
	repoFactory, err := factory.NewRepositoryFactory(db, cfg.DynamoTableName, awsConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Create service registry
	publisher := streaming.NewMockPublisher() // Or real publisher if configured
	serviceRegistry, err := services.NewRegistry(
		services.WithStorage(repoFactory),
		services.WithPublisher(publisher),
		services.WithLogger(logger),
		services.WithConfig(&services.ServiceConfig{
			BaseURL:   fmt.Sprintf("https://%s", cfg.Domain),
			JWTSecret: cfg.JWTSecret,
		}),
	)
	if err != nil {
		logger.Fatal("Failed to create service registry", zap.Error(err))
	}

	// Initialize processor
	processor = NewAIProcessor(db, cfg.DynamoTableName, aiAnalyzer, serviceRegistry)
}

func main() {
	// Use Lift DynamoDB stream pattern with proper middleware and error handling
	patterns.StartDynamoDBStreamLambda("ai-processor", processor, logger)
}
