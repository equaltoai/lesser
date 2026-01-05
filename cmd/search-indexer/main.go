// Package main implements the search-indexer Lambda function for indexing content for search functionality.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

type searchCostRecorder interface {
	RecordSearchCost(ctx context.Context, costData *models.SearchCostTracking) error
}

type unifiedCostTracker interface {
	TrackDynamoWrite(ctx context.Context, tableName string, units int64) error
}

// SearchIndexer handles search index operations for content
type SearchIndexer struct {
	db             core.DB
	tableName      string
	logger         *zap.Logger
	costRepo       searchCostRecorder
	unifiedTracker unifiedCostTracker
}

// NewSearchIndexer creates a new search indexer instance
func NewSearchIndexer(db core.DB, tableName string) *SearchIndexer {
	costRepo := repositories.NewSearchCostRepository(db, tableName, lambdaCtx.Logger, nil)

	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(nil, lambdaCtx.Logger, "SearchIndexer", "", "")

	return &SearchIndexer{
		db:             db,
		tableName:      tableName,
		logger:         lambdaCtx.Logger,
		costRepo:       costRepo,
		unifiedTracker: unifiedTracker,
	}
}

// HandleStream processes DynamoDB stream events for search indexing.
func (si *SearchIndexer) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	requestID := ctx.GetRequestID()

	si.logger.Info("processing search indexer stream event",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Filter and process indexable records
	indexableRecords := si.filterIndexable(event.Records)

	si.logger.Info("filtered indexable records",
		zap.String("request_id", requestID),
		zap.Int("total_records", len(event.Records)),
		zap.Int("indexable_records", len(indexableRecords)),
	)

	// Process each indexable record
	var errors []error
	for _, record := range indexableRecords {
		if err := si.processRecord(ctx, record); err != nil {
			si.logger.Error("failed to process indexable record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.String("event_name", record.EventName),
				zap.Error(err),
			)
			errors = append(errors, err)
			// Continue processing other records
		}
	}

	// Return error if there were any failures
	if len(errors) > 0 {
		si.logger.Error("partial batch failure during search indexing",
			zap.String("request_id", requestID),
			zap.Int("failed_records", len(errors)),
			zap.Int("total_indexable_records", len(indexableRecords)),
		)
		return pkgErrors.SearchIndexerPartialBatchFailure()
	}

	return nil
}

func (si *SearchIndexer) filterIndexable(records []events.DynamoDBEventRecord) []events.DynamoDBEventRecord {
	var indexable []events.DynamoDBEventRecord

	for _, record := range records {
		if si.isIndexableRecord(record) {
			indexable = append(indexable, record)
		}
	}

	return indexable
}

func (si *SearchIndexer) isIndexableRecord(record events.DynamoDBEventRecord) bool {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return false
	}

	if record.Change.NewImage == nil {
		return false
	}

	// Check if this is indexable content
	var item struct {
		PK      string `dynamorm:"pk"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}

	if err := unmarshalItemFn(record, &item); err != nil {
		return false
	}

	// Check for indexable content types
	switch item.Type {
	case "Note", "Article", "Question":
		// Text content that should be searchable
		return item.Content != ""
	case "Actor", "Person":
		// User profiles that should be searchable
		return true
	default:
		// Check if it's an object or status we should index
		return strings.HasPrefix(item.PK, "OBJECT#") || strings.HasPrefix(item.PK, "STATUS#")
	}
}

func (si *SearchIndexer) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	startTime := time.Now()

	// Extract indexable content from the record
	content, err := si.extractIndexableContent(record)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to extract indexable content")
	}

	// Initialize cost tracking for indexing operation
	costData := &models.SearchCostTracking{
		UserID:        content.ActorID,
		RequestID:     ctx.GetRequestID(),
		OperationType: "search_indexing",
		SearchType:    "indexing",
		Query:         content.Text,
		QueryLength:   len(content.Text),
		Timestamp:     startTime,
	}

	// Track database writes for indexing
	var writeCount int64

	// Create search index entry
	if err := si.createSearchIndex(ctx, content); err != nil {
		// Record failed indexing cost
		si.recordIndexingCost(ctx, costData, startTime, 0, writeCount, err)
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create search index")
	}

	// Estimate write operations (main index + additional indexes)
	writeCount = 1 // Main search index
	if content.ActorID != "" {
		writeCount++ // Actor-specific index
	}
	writeCount += int64(len(content.Tags)) // Tag indexes

	// Track successful indexing
	si.recordIndexingCost(ctx, costData, startTime, 1, writeCount, nil)

	si.logger.Debug("indexed content for search",
		zap.String("content_id", content.ID),
		zap.String("content_type", content.Type),
		zap.Int("content_length", len(content.Text)),
		zap.Int64("write_operations", writeCount),
	)

	return nil
}

// IndexableContent represents content that can be indexed for search
type IndexableContent struct {
	ID        string
	Type      string
	Text      string
	ActorID   string
	Tags      []string
	Language  string
	CreatedAt time.Time
}

func (si *SearchIndexer) extractIndexableContent(record events.DynamoDBEventRecord) (*IndexableContent, error) {
	var item struct {
		PK          string   `dynamorm:"pk"`
		SK          string   `dynamorm:"sk"`
		Type        string   `json:"type"`
		Content     string   `json:"content"`
		Summary     string   `json:"summary"`
		Name        string   `json:"name"`
		ActorID     string   `json:"actor_id"`
		Tags        []string `json:"tags"`
		Language    string   `json:"language"`
		CreatedAt   string   `json:"created_at"`
		PublishedAt string   `json:"published_at"`
	}

	if err := unmarshalItemFn(record, &item); err != nil {
		return nil, pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to unmarshal stream image")
	}

	// Extract ID from PK
	var contentID string
	if strings.HasPrefix(item.PK, "OBJECT#") {
		contentID = item.PK[7:] // Remove "OBJECT#" prefix
	} else if strings.HasPrefix(item.PK, "STATUS#") {
		contentID = item.PK[7:] // Remove "STATUS#" prefix
	} else if strings.HasPrefix(item.PK, "ACTOR#") {
		contentID = item.PK[6:] // Remove "ACTOR#" prefix
	} else {
		contentID = item.PK
	}

	// Combine available text content
	var textContent strings.Builder
	if item.Content != "" {
		textContent.WriteString(item.Content)
	}
	if item.Summary != "" {
		if textContent.Len() > 0 {
			textContent.WriteString(" ")
		}
		textContent.WriteString(item.Summary)
	}
	if item.Name != "" {
		if textContent.Len() > 0 {
			textContent.WriteString(" ")
		}
		textContent.WriteString(item.Name)
	}

	// Parse timestamp
	var createdAt time.Time
	for _, timeStr := range []string{item.CreatedAt, item.PublishedAt} {
		if timeStr != "" {
			if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
				createdAt = t
				break
			}
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return &IndexableContent{
		ID:        contentID,
		Type:      item.Type,
		Text:      textContent.String(),
		ActorID:   item.ActorID,
		Tags:      item.Tags,
		Language:  item.Language,
		CreatedAt: createdAt,
	}, nil
}

func (si *SearchIndexer) createSearchIndex(ctx *lift.Context, content *IndexableContent) error {
	// Create search index record with full-text search capabilities
	searchRecord := struct {
		PK          string   `dynamorm:"pk"`
		SK          string   `dynamorm:"sk"`
		Type        string   `json:"type"`
		ContentID   string   `json:"content_id"`
		ContentType string   `json:"content_type"`
		Text        string   `json:"text"`
		TextLower   string   `json:"text_lower"` // For case-insensitive search
		ActorID     string   `json:"actor_id"`
		Tags        []string `json:"tags"`
		Language    string   `json:"language"`
		WordCount   int      `json:"word_count"`
		CreatedAt   string   `json:"created_at"`
		IndexedAt   string   `json:"indexed_at"`
		TTL         int64    `dynamorm:"ttl"`
	}{
		PK:          fmt.Sprintf("SEARCH#%s", content.Type),
		SK:          fmt.Sprintf("CONTENT#%s#%s", content.CreatedAt.Format(common.DateFormat), content.ID),
		Type:        "SearchIndex",
		ContentID:   content.ID,
		ContentType: content.Type,
		Text:        content.Text,
		TextLower:   strings.ToLower(content.Text),
		ActorID:     content.ActorID,
		Tags:        content.Tags,
		Language:    content.Language,
		WordCount:   len(strings.Fields(content.Text)),
		CreatedAt:   content.CreatedAt.Format(time.RFC3339),
		IndexedAt:   time.Now().Format(time.RFC3339),
		TTL:         time.Now().Add(365 * 24 * time.Hour).Unix(), // 1 year retention
	}

	// Store the search index record using Lift context
	if err := si.db.WithContext(ctx).Model(&searchRecord).Create(); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to store search index")
	}

	// Create additional indexes for common search patterns
	if err := si.createAdditionalIndexes(ctx, content); err != nil {
		si.logger.Warn("failed to create additional search indexes",
			zap.String("content_id", content.ID),
			zap.Error(err),
		)
		// Don't fail the main indexing operation for additional indexes
	}

	return nil
}

func (si *SearchIndexer) createAdditionalIndexes(ctx *lift.Context, content *IndexableContent) error {
	// Create actor-specific index for searching user's content
	if content.ActorID != "" {
		actorIndex := struct {
			PK        string `dynamorm:"pk"`
			SK        string `dynamorm:"sk"`
			Type      string `json:"type"`
			ContentID string `json:"content_id"`
			Text      string `json:"text"`
			CreatedAt string `json:"created_at"`
			TTL       int64  `dynamorm:"ttl"`
		}{
			PK:        fmt.Sprintf("SEARCH#ACTOR#%s", content.ActorID),
			SK:        fmt.Sprintf("CONTENT#%s", content.ID),
			Type:      "ActorSearchIndex",
			ContentID: content.ID,
			Text:      content.Text,
			CreatedAt: content.CreatedAt.Format(time.RFC3339),
			TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
		}

		if err := si.db.WithContext(ctx).Model(&actorIndex).Create(); err != nil {
			return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create actor search index")
		}
	}

	// Create tag-based indexes
	for _, tag := range content.Tags {
		if tag != "" {
			tagIndex := struct {
				PK        string `dynamorm:"pk"`
				SK        string `dynamorm:"sk"`
				Type      string `json:"type"`
				Tag       string `json:"tag"`
				ContentID string `json:"content_id"`
				ActorID   string `json:"actor_id"`
				CreatedAt string `json:"created_at"`
				TTL       int64  `dynamorm:"ttl"`
			}{
				PK:        fmt.Sprintf("SEARCH#TAG#%s", strings.ToLower(tag)),
				SK:        fmt.Sprintf("CONTENT#%s", content.ID),
				Type:      "TagSearchIndex",
				Tag:       tag,
				ContentID: content.ID,
				ActorID:   content.ActorID,
				CreatedAt: content.CreatedAt.Format(time.RFC3339),
				TTL:       time.Now().Add(180 * 24 * time.Hour).Unix(), // 180 days retention
			}

			if err := si.db.WithContext(ctx).Model(&tagIndex).Create(); err != nil {
				si.logger.Warn("failed to create tag search index",
					zap.String("tag", tag),
					zap.Error(err),
				)
				// Continue with other tags
			}
		}
	}

	return nil
}

// recordIndexingCost records the cost of indexing operations
func (si *SearchIndexer) recordIndexingCost(ctx *lift.Context, costData *models.SearchCostTracking, startTime time.Time, resultCount int, writeCount int64, err error) {
	// Complete cost tracking
	responseTime := time.Since(startTime)
	costData.ResponseTimeMs = responseTime.Milliseconds()
	costData.ResultCount = resultCount
	costData.DynamoWrites = writeCount
	costData.DynamoQueries = 1 // Indexing typically involves create operations

	// Track costs using centralized tracker
	if err := si.unifiedTracker.TrackDynamoWrite(ctx.Request.Context(), si.tableName, writeCount); err != nil {
		si.logger.Warn("failed to track cost", zap.Error(err))
	}

	// Calculate costs
	costData.DynamoCostMicros = si.calculateIndexingCost(writeCount)
	costData.TotalCostMicros = costData.DynamoCostMicros

	if err := costData.UpdateKeys(); err != nil {
		si.logger.Warn("failed to update cost data keys", zap.Error(err))
		// Continue with recording cost even if key update fails
	}

	// Record cost asynchronously to not impact indexing performance
	runAsyncFn(func() {
		bgCtx := context.Background()
		if err := si.costRepo.RecordSearchCost(bgCtx, costData); err != nil {
			si.logger.Error("failed to record indexing cost",
				zap.String("user_id", costData.UserID),
				zap.String("content_id", costData.Query),
				zap.Error(err))
		}
	})

	si.logger.Debug("indexing_cost_tracked",
		zap.String("user_id", costData.UserID),
		zap.Int("result_count", resultCount),
		zap.Int64("write_count", writeCount),
		zap.Int64("cost_micros", costData.TotalCostMicros),
		zap.Int64("response_time_ms", costData.ResponseTimeMs),
		zap.Bool("success", err == nil))
}

// calculateIndexingCost calculates the cost of indexing operations in microcents
func (si *SearchIndexer) calculateIndexingCost(writeCount int64) int64 {
	// DynamoDB write cost: $1.25 per million write request units
	const writeCostPer1M = 125000 // 125000 microcents per million writes

	if writeCount == 0 {
		return 0
	}

	return (writeCount * writeCostPer1M) / 1000000
}

var (
	lambdaCtx *common.LambdaContext
	processor *SearchIndexer
	db        core.DB
)

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = dynamorm.NewLambdaOptimizedClient
	lambdaStartFn              = lambda.Start
	unmarshalItemFn            = stream.UnmarshalItem
	runAsyncFn                 = func(fn func()) { go fn() }
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeSearchIndexer()
}

func main() {
	runSearchIndexer()
}

func initializeSearchIndexer() {
	// Initialize Lambda with processor configuration for search indexing
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:        "search-indexer",
		LambdaType:         common.LambdaTypeProcessor,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableTracing:      true,
		EnableHealthCheck:  false,
		EnableCostTracking: true,
		RequestTimeout:     30 * time.Second,
		RetryMaxAttempts:   3,
	})

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = newLambdaOptimizedClientFn(context.Background(), lambdaCtx.Config.Region)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize processor
	processor = NewSearchIndexer(db, lambdaCtx.Config.DynamoTableName)
}

func runSearchIndexer() {
	app := lift.New()
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.RequestID())))
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.Recover())))

	_ = app.DynamoDB("*", func(ctx *lift.Context) error {
		records, err := ctx.DynamoDBRecords()
		if err != nil {
			return err
		}
		return processor.HandleStream(ctx, events.DynamoDBEvent{Records: records})
	})

	lambdaStartFn(app.HandleRequest)
}
