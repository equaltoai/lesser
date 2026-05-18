// Package main implements an AI-powered content analysis processor that monitors
// DynamoDB streams for new content, performs toxicity detection, spam analysis,
// and automated moderation actions using AWS Bedrock. It processes INSERT and
// MODIFY events from the stream, analyzing text and media content to ensure
// community guidelines compliance and protect users from harmful content.
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
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	aiService "github.com/equaltoai/lesser/pkg/services/ai"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/stream"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// AIProcessor handles AI-based content analysis for posts and media in the system.
// It integrates with AWS Bedrock to perform toxicity detection, spam filtering,
// and automated moderation decisions based on configurable thresholds.
type AIProcessor struct {
	db         core.DB
	tableName  string
	aiAnalyzer contentAnalyzer // For AI analysis (Comprehend, Rekognition, etc.)
	aiService  analysisSaver   // For storage and event publishing
	logger     *zap.Logger
}

type contentAnalyzer interface {
	AnalyzeContent(ctx context.Context, content *ai.Content) (*ai.AIAnalysis, error)
}

type analysisSaver interface {
	SaveAnalysis(ctx context.Context, cmd *aiService.SaveAnalysisCommand) (*aiService.SaveAnalysisResult, error)
}

type analyzableStreamItem struct {
	PK   string `theorydb:"pk"`
	Type string `json:"type"`
}

type contentStreamAttachment struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
}

type contentStreamItem struct {
	PK         string                    `theorydb:"pk"`
	Type       string                    `json:"type"`
	Content    string                    `json:"content"`
	ActorID    string                    `json:"actor_id"`
	Attachment []contentStreamAttachment `json:"attachment"`
}

func (ap *AIProcessor) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) (err error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if ap.logger == nil {
		ap.logger = zap.NewNop()
	}

	defer func() {
		if r := recover(); r != nil {
			ap.logger.Error("panic processing AI stream record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Any("panic", r),
			)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	if err := ap.processRecord(runCtx, requestID, record); err != nil {
		ap.logger.Error("error processing record",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)
		// Preserve prior Lift behavior: log and continue without failing the batch.
		return nil
	}
	return nil
}

func (ap *AIProcessor) processRecord(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
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
		if err := ap.handleModerationAction(ctx, analysis); err != nil {
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
	var item analyzableStreamItem

	if err := unmarshalItemFn(record, &item); err != nil {
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
	var item contentStreamItem

	if err := unmarshalItemFn(record, &item); err != nil {
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

func (ap *AIProcessor) handleModerationAction(ctx context.Context, analysis *ai.AIAnalysis) error {
	// Create moderation event model for DynamORM
	moderationEvent := struct {
		PK              string  `theorydb:"pk"`
		SK              string  `theorydb:"sk"`
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
		TTL             int64   `theorydb:"ttl"`
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
	lambdaCtx                  *common.LambdaContext
	cfg                        *config.Config //nolint:unused // Reserved for dependency injection pattern
	logger                     *zap.Logger
	repos                      storageCore.RepositoryStorage //nolint:unused // Reserved for dependency injection pattern
	processor                  *AIProcessor
	unmarshalItemFn            = stream.UnmarshalItem
	runningUnitTestsFn         = common.RunningUnitTests
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db core.DB, tableName string, logger *zap.Logger) (storageCore.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
	newContentAnalyzerFn = func(lambdaCtx *common.LambdaContext) (contentAnalyzer, error) {
		if lambdaCtx == nil || lambdaCtx.AWSServices == nil {
			return nil, fmt.Errorf("AI processor AWS services are not initialized")
		}
		return ai.NewAIService(lambdaCtx.AWSServices.Config, ai.DefaultAIConfig()), nil
	}
	newAnalysisSaverFn = func(repos storageCore.RepositoryStorage, _ core.DB, logger *zap.Logger) analysisSaver {
		return aiService.NewService(repos, nil, logger)
	}
	lambdaStartFn = lambda.Start
)

func init() {
	if runningUnitTestsFn() {
		return
	}
	if err := initializeAIProcessor(); err != nil {
		logger.Fatal("failed to initialize AI processor", zap.Error(err))
	}
}

func initializeAIProcessor() error {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "ai-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	var err error
	repos, err = initializeAIStorage(lambdaCtx)
	if err != nil {
		return err
	}

	// Initialize processor with simplified configuration
	processor, err = NewSimplifiedAIProcessor(lambdaCtx)
	return err
}

func initializeAIStorage(lambdaCtx *common.LambdaContext) (storageCore.RepositoryStorage, error) {
	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:          "AI processor",
		RequireRepositories:  true,
		NewDB:                newLambdaOptimizedClientFn,
		NewRepositoryStorage: newRepositoryFactoryFn,
	})
	if err != nil {
		return nil, err
	}
	return deps.Repos, nil
}

// NewSimplifiedAIProcessor creates a new AI processor instance with simplified Lambda context
func NewSimplifiedAIProcessor(lambdaCtx *common.LambdaContext) (*AIProcessor, error) {
	if lambdaCtx == nil {
		return nil, fmt.Errorf("AI processor lambda context is nil")
	}
	if lambdaCtx.Config == nil {
		return nil, fmt.Errorf("AI processor config is nil")
	}

	db, ok := lambdaCtx.DynamoDB.(core.DB)
	if !ok || db == nil {
		return nil, fmt.Errorf("AI processor dynamodb client is not initialized")
	}

	repoStorage, ok := lambdaCtx.Repos.(storageCore.RepositoryStorage)
	if !ok || repoStorage == nil {
		return nil, fmt.Errorf("AI processor repository storage is not initialized")
	}

	analyzer, err := newContentAnalyzerFn(lambdaCtx)
	if err != nil {
		return nil, err
	}
	if analyzer == nil {
		return nil, fmt.Errorf("AI processor content analyzer is not initialized")
	}

	saver := newAnalysisSaverFn(repoStorage, db, lambdaCtx.Logger)
	if saver == nil {
		return nil, fmt.Errorf("AI processor analysis saver is not initialized")
	}

	// Initialize simplified processor with essential components
	return &AIProcessor{
		db:         db,
		tableName:  lambdaCtx.Config.DynamoTableName,
		aiAnalyzer: analyzer,
		aiService:  saver,
		logger:     lambdaCtx.Logger,
	}, nil
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, handleAIProcessorStreamRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleAIProcessorStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if processor == nil {
		return fmt.Errorf("AI processor not initialized")
	}
	return processor.HandleDynamoDBRecord(ctx, record)
}
