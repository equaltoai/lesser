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
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	aiService "github.com/equaltoai/lesser/pkg/services/ai"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// AIProcessor handles AI-based content analysis for posts and media in the system.
// It integrates with AWS Bedrock to perform toxicity detection, spam filtering,
// and automated moderation decisions based on configurable thresholds.
type AIProcessor struct {
	db         core.DB
	tableName  string
	aiAnalyzer *ai.AIService      // For AI analysis (Comprehend, Rekognition, etc.)
	aiService  *aiService.Service // For storage and event publishing
	logger     *zap.Logger
}

// HandleStreamWithContext processes DynamoDB stream events with explicit context
func (ap *AIProcessor) HandleStreamWithContext(ctx context.Context, liftCtx *lift.Context, event events.DynamoDBEvent) error {
	requestID := liftCtx.GetRequestID()

	ap.logger.Info("processing AI analysis stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := ap.processRecord(ctx, liftCtx, record); err != nil {
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

func (ap *AIProcessor) processRecord(ctx context.Context, liftCtx *lift.Context, record events.DynamoDBEventRecord) error {
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
		return pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to extract content from stream record")
	}

	// Perform AI analysis using the analyzer (AWS services)
	analysis, err := ap.aiAnalyzer.AnalyzeContent(ctx, content)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "AI analysis failed")
	}

	// Store analysis and publish events using the service layer
	saveCmd := &aiService.SaveAnalysisCommand{
		Analysis: analysis,
		UserID:   content.AuthorID, // Use author as context for events
	}
	_, err = ap.aiService.SaveAnalysis(ctx, saveCmd)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to save AI analysis")
	}

	// Handle moderation action if needed
	if analysis.ModerationAction != ai.ActionNone {
		if err := ap.handleModerationAction(ctx, liftCtx, analysis); err != nil {
			requestID := liftCtx.GetRequestID()
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
		return nil, pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to unmarshal stream record")
	}

	// Extract object ID from PK
	var objectID string
	if len(item.PK) > 7 && item.PK[:7] == "OBJECT#" {
		objectID = item.PK[7:]
	} else {
		ap.logger.Error("invalid object primary key format",
			zap.String("pk", item.PK),
		)
		return nil, pkgErrors.AIProcessorInvalidObjectPK()
	}

	// Skip if not an analyzable type
	if !ap.isAnalyzableType(item.Type) {
		ap.logger.Debug("object type is not analyzable",
			zap.String("type", item.Type),
			zap.String("object_id", objectID),
		)
		return nil, pkgErrors.AIProcessorNotAnalyzableType()
	}

	// Extract media URLs from attachments
	var mediaURLs []string
	for _, att := range item.Attachment {
		if att.URL != "" && common.IsProcessableMediaType(att.Type) {
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

func (ap *AIProcessor) handleModerationAction(ctx context.Context, _ *lift.Context, analysis *ai.AIAnalysis) error {
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
	return ap.db.WithContext(ctx).Model(&moderationEvent).Create()
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
	lambdaCtx *common.LambdaContext
	cfg       *config.Config //nolint:unused // Reserved for dependency injection pattern
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage //nolint:unused // Reserved for dependency injection pattern
	processor *AIProcessor
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization for processor functions
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "ai-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if lambdaCtx.Repos != nil {
		repos = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	}

	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Initialize processor with simplified configuration
	processor = NewSimplifiedAIProcessor(lambdaCtx)
}

// NewSimplifiedAIProcessor creates a new AI processor instance with simplified Lambda context
func NewSimplifiedAIProcessor(lambdaCtx *common.LambdaContext) *AIProcessor {
	// Initialize simplified processor with essential components
	return &AIProcessor{
		db:        lambdaCtx.DynamoDB.(core.DB),
		tableName: lambdaCtx.Config.DynamoTableName,
		logger:    lambdaCtx.Logger,
	}
}

func main() {
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) (err error) {
		defer func() {
			if r := recover(); r != nil {
				requestID := fmt.Sprintf("ai-processor-%d", time.Now().UnixNano())
				logger.Error("panic in ai processor handler",
					zap.String("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"))
				err = fmt.Errorf("panic recovered in ai-processor: %v", r)
			}
		}()

		// Create a simple lift context with minimal setup
		liftCtx := &lift.Context{
			RequestID: fmt.Sprintf("ai-processor-%d", time.Now().UnixNano()),
		}
		// Add a method to get context
		liftCtx.Request = &lift.Request{}
		// Use direct context access in the handler methods
		return processor.HandleStreamWithContext(ctx, liftCtx, event)
	})
}
