// Package main implements the import-processor Lambda function for processing user data imports.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/services"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ImportProcessor handles data import processing from SQS messages
type ImportProcessor struct {
	db               core.DB
	importRepo       *repositories.ImportRepository
	costTrackingRepo *repositories.TrackingRepository
	s3Client         *s3.Client
	repos            storageCore.RepositoryStorage
	cfg              *config.Config
	logger           *zap.Logger
	bucketName       string
	baseURL          string
}

var processor *ImportProcessor

// ImportProcessorEvent represents the event triggered for import processing
type ImportProcessorEvent struct {
	ImportID string `json:"import_id"`
	Username string `json:"username"`
	Type     string `json:"type"` // followers, following, blocks, mutes, lists, bookmarks
	Mode     string `json:"mode"` // merge, overwrite
	S3Key    string `json:"s3_key"`
}

// ImportResult tracks the results of an import
type ImportResult struct {
	Success int      `json:"success"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// ImportTransaction represents a transaction for import operations
type ImportTransaction struct {
	importID   string
	operations []func() error
	rollbacks  []func() error
	logger     *zap.Logger
}

// NewImportTransaction creates a new import transaction
func NewImportTransaction(importID string, logger *zap.Logger) *ImportTransaction {
	return &ImportTransaction{
		importID:   importID,
		operations: make([]func() error, 0),
		rollbacks:  make([]func() error, 0),
		logger:     logger,
	}
}

// AddOperation adds an operation with its corresponding rollback
func (t *ImportTransaction) AddOperation(operation func() error, rollback func() error) {
	t.operations = append(t.operations, operation)
	t.rollbacks = append(t.rollbacks, rollback)
}

// Execute executes all operations and rolls back on failure
func (t *ImportTransaction) Execute(_ context.Context) error {
	// Execute operations
	for i, operation := range t.operations {
		if err := operation(); err != nil {
			// Rollback already executed operations in reverse order
			t.logger.Warn("operation failed, rolling back",
				zap.String("import_id", t.importID),
				zap.Int("failed_operation", i),
				zap.Error(err))

			if rollbackErr := t.rollback(i); rollbackErr != nil {
				t.logger.Error("rollback failed",
					zap.String("import_id", t.importID),
					zap.Error(rollbackErr))
			}

			return errors.Join(ErrOperationFailed, err)
		}
	}

	return nil
}

// rollback rolls back executed operations in reverse order
func (t *ImportTransaction) rollback(lastExecutedIndex int) error {
	var rollbackErrors []error

	// Rollback in reverse order
	for i := lastExecutedIndex - 1; i >= 0; i-- {
		if i < len(t.rollbacks) && t.rollbacks[i] != nil {
			if err := t.rollbacks[i](); err != nil {
				rollbackErrors = append(rollbackErrors, err)
				t.logger.Error("rollback operation failed",
					zap.String("import_id", t.importID),
					zap.Int("rollback_operation", i),
					zap.Error(err))
			}
		}
	}

	if len(rollbackErrors) > 0 {
		return ErrRollbackFailed
	}

	return nil
}

func init() {
	// Initialize Lambda with processor configuration for import processing
	lambdaCtx := common.MustInitializeLambda(common.LambdaConfig{
		ServiceName:        "import-processor",
		LambdaType:         common.LambdaTypeProcessor,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableTracing:      true,
		EnableHealthCheck:  false,
		EnableCostTracking: true,
		RequestTimeout:     2 * time.Minute, // Import processing can take longer
		RetryMaxAttempts:   3,
	})

	// AWS config no longer needed - DynamORM handles configuration internally

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), lambdaCtx.Config.Region)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repository factory
	repos, err := factory.NewRepositoryFactory(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize repositories
	importRepo := repositories.NewImportRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	costTrackingRepo := repositories.NewTrackingRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)

	// Get configuration from centralized config
	bucketName := lambdaCtx.Config.S3BucketName
	if err := common.ValidateRequiredParam("bucketName", bucketName); err != nil {
		lambdaCtx.Logger.Fatal("S3_BUCKET_NAME not configured")
	}

	baseURL := lambdaCtx.Config.BaseURL()

	// Create processor instance
	processor = &ImportProcessor{
		db:               db,
		importRepo:       importRepo,
		costTrackingRepo: costTrackingRepo,
		repos:            repos,
		cfg:              lambdaCtx.Config,
		logger:           lambdaCtx.Logger,
		bucketName:       bucketName,
		baseURL:          baseURL,
	}
}

func main() {
	// Use the standard Lift SQS pattern
	patterns.StartSQSLambda("import-processing", processor, processor.logger)
}

// HandleSQS implements the SQS handler interface for Lift
func (p *ImportProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Initialize AWS clients
	if err := p.initializeAWSClients(ctx.Request.Context()); err != nil {
		p.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	p.logger.Info("processing import batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)))

	// Process each message
	for _, message := range event.Records {
		// Try parsing as services.ImportJobMessage first (new format)
		var importMsg services.ImportJobMessage
		if err := common.ParseRequestBody([]byte(message.Body), &importMsg); err == nil {
			// Convert to legacy format for processing
			importEvent := ImportProcessorEvent{
				ImportID: importMsg.ImportID,
				Username: importMsg.Username,
				Type:     importMsg.Type,
				Mode:     importMsg.Mode,
				S3Key:    importMsg.S3Key,
			}
			if err := p.processImportJob(ctx.Request.Context(), importEvent); err != nil {
				p.logger.Error("failed to process import job",
					zap.String("import_id", importEvent.ImportID),
					zap.String("username", importEvent.Username),
					zap.Error(err))
				// Update job status as failed
				if updateErr := p.importRepo.UpdateImportStatus(ctx.Request.Context(), importEvent.ImportID, "failed", nil, err.Error()); updateErr != nil {
					p.logger.Error("failed to update import status to failed",
						zap.String("import_id", importEvent.ImportID),
						zap.Error(updateErr))
				}
			}
			continue
		}

		// Fallback to legacy format
		var importEvent ImportProcessorEvent
		if err := common.ParseRequestBody([]byte(message.Body), &importEvent); err != nil {
			p.logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.Error(err))
			continue
		}

		if err := p.processImportJob(ctx.Request.Context(), importEvent); err != nil {
			p.logger.Error("failed to process import job",
				zap.String("import_id", importEvent.ImportID),
				zap.String("username", importEvent.Username),
				zap.Error(err))
			// Update job status as failed
			if updateErr := p.importRepo.UpdateImportStatus(ctx.Request.Context(), importEvent.ImportID, "failed", nil, err.Error()); updateErr != nil {
				p.logger.Error("failed to update import status to failed",
					zap.String("import_id", importEvent.ImportID),
					zap.Error(updateErr))
			}
		}
	}

	return nil
}

func (p *ImportProcessor) initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return errors.Join(ErrAWSConfigLoad, err)
	}

	// Initialize S3 client
	p.s3Client = s3.NewFromConfig(awsCfg)

	return nil
}

func (p *ImportProcessor) processImportJob(ctx context.Context, event ImportProcessorEvent) error {
	p.logger.Info("processing import job",
		zap.String("import_id", event.ImportID),
		zap.String("username", event.Username),
		zap.String("type", event.Type))

	// Initialize cost tracking
	startTime := time.Now()
	importCostTracking := &models.ImportCostTracking{
		ImportID:  event.ImportID,
		Username:  event.Username,
		Type:      event.Type,
		Mode:      event.Mode,
		Status:    "processing",
		StartedAt: startTime,
	}

	// Track the import job completion
	defer func() {
		if importCostTracking.CompletedAt == nil {
			completedAt := time.Now()
			importCostTracking.CompletedAt = &completedAt
		}

		// Calculate Lambda execution cost
		importCostTracking.LambdaDurationMs = time.Since(startTime).Milliseconds()
		importCostTracking.LambdaExecutionCost = p.calculateLambdaCost(importCostTracking.LambdaDurationMs)

		// Calculate total cost
		importCostTracking.CalculateTotalCost()

		// Save cost tracking record
		if err := p.costTrackingRepo.Create(ctx, &models.DynamoDBCostRecord{
			Table:                p.cfg.DynamoTableName,
			OperationType:        "ImportProcessing",
			EstimatedCostDollars: importCostTracking.GetTotalCostDollars(),
			TotalCostMicroCents:  importCostTracking.TotalCostMicroCents,
			ServiceName:          "import-processor",
			RequestDuration:      time.Since(startTime).Milliseconds(),
			Properties: map[string]interface{}{
				"username":  event.Username,
				"import_id": event.ImportID,
			},
		}); err != nil {
			p.logger.Error("failed to save import cost tracking",
				zap.String("import_id", event.ImportID),
				zap.Error(err))
		}

		// Update budget usage with actual import costs
		if err := p.importRepo.UpdateBudgetUsage(ctx, event.Username, "daily", importCostTracking.TotalCostMicroCents, 0); err != nil {
			p.logger.Warn("failed to update budget usage",
				zap.String("import_id", event.ImportID),
				zap.String("username", event.Username),
				zap.Error(err))
		}
	}()

	// Update job status to processing
	if err := p.importRepo.UpdateImportStatus(ctx, event.ImportID, "processing", nil, ""); err != nil {
		p.logger.Warn("failed to update import status", zap.Error(err))
	}

	// Download file from S3
	fileData, err := p.downloadFromS3(ctx, event.S3Key)
	if err != nil {
		return errors.Join(ErrImportDownloadFailed, err)
	}

	// Track S3 download costs
	importCostTracking.FileSize = int64(len(fileData))
	importCostTracking.S3GetRequests = 1
	importCostTracking.S3GetRequestCost = p.calculateS3GetCost(1)
	importCostTracking.DataTransferBytes = int64(len(fileData))
	importCostTracking.S3DataTransferCost = p.calculateS3DataTransferCost(int64(len(fileData)))

	// Detect format
	format := detectFormat(fileData)
	p.logger.Info("detected import format", zap.String("format", format))

	// Process based on format and type
	var result ImportResult

	switch format {
	case "csv":
		result, err = p.processCSVImport(ctx, event, fileData, importCostTracking)
	case "json":
		result, err = p.processJSONImport(ctx, event, fileData, importCostTracking)
	case "activitypub":
		result, err = p.processActivityPubImport(ctx, event, fileData, importCostTracking)
	default:
		p.logger.Error("unsupported import format detected",
			zap.String("format", format),
			zap.String("import_id", event.ImportID))
		return ErrUnsupportedImportFormat
	}

	if err != nil {
		return errors.Join(ErrImportProcessFailed, err)
	}

	// Update cost tracking with final metrics
	importCostTracking.RecordCount = int64(result.Success + result.Skipped + result.Failed)
	importCostTracking.ProcessedCount = int64(result.Success + result.Failed)
	importCostTracking.SuccessCount = int64(result.Success)
	importCostTracking.SkipCount = int64(result.Skipped)
	importCostTracking.ErrorCount = int64(result.Failed)
	importCostTracking.Status = "completed"

	// Update import job as completed
	completionData := map[string]any{
		"total":   result.Success + result.Skipped + result.Failed,
		"success": result.Success,
		"skipped": result.Skipped,
		"failed":  result.Failed,
		"errors":  result.Errors,
	}

	if err := p.importRepo.UpdateImportStatus(ctx, event.ImportID, "completed", completionData, ""); err != nil {
		return errors.Join(ErrImportStatusUpdateFailed, err)
	}

	p.logger.Info("import completed",
		zap.String("import_id", event.ImportID),
		zap.Int("success", result.Success),
		zap.Int("skipped", result.Skipped),
		zap.Int("failed", result.Failed))

	return nil
}

func detectFormat(data []byte) string {
	// Try to parse as JSON first
	var jsonTest any
	if err := common.ParseActivityPubObject(data, &jsonTest); err == nil {
		// Check if it's an ActivityPub collection
		if jsonMap, ok := jsonTest.(map[string]any); ok {
			if _, hasContext := jsonMap["@context"]; hasContext {
				return "activitypub"
			}
		}
		return "json"
	}

	// Check if it's CSV by looking for common patterns
	str := string(data)
	if strings.Contains(str, "Account address") || strings.Contains(str, ",") {
		return "csv"
	}

	return "unknown"
}

func (p *ImportProcessor) processCSVImport(ctx context.Context, event ImportProcessorEvent, data []byte, costTracking *models.ImportCostTracking) (ImportResult, error) {
	reader := csv.NewReader(bytes.NewReader(data))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return ImportResult{Errors: make([]string, 0)}, errors.Join(ErrCSVHeaderRead, err)
	}

	// Process based on type
	switch event.Type {
	case "followers":
		return p.processFollowersCSV(reader)

	case "following":
		return p.processFollowingCSV(ctx, event, reader, costTracking)

	case "blocks":
		return p.processBlocksCSV(ctx, event, reader, costTracking)

	case "mutes":
		return p.processMutesCSV(ctx, event, reader, header)

	case "bookmarks":
		return p.processBookmarksCSV(ctx, event, reader)

	default:
		p.logger.Error("CSV import not supported for type",
			zap.String("type", event.Type),
			zap.String("import_id", event.ImportID))
		return ImportResult{Errors: make([]string, 0)}, ErrCSVImportNotSupportedForType
	}
}

func (p *ImportProcessor) processJSONImport(ctx context.Context, event ImportProcessorEvent, data []byte, _ *models.ImportCostTracking) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	// Parse JSON based on type
	switch event.Type {
	case "lists":
		// Import lists with members
		var lists map[string][]string
		if err := common.ParseActivityPubObject(data, &lists); err != nil {
			return result, errors.Join(ErrJSONParseFailed, err)
		}

		for listName, members := range lists {
			// Create or update list
			listID, err := p.createOrUpdateList(ctx, event.Username, listName)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to create list %s: %v", listName, err))
				continue
			}

			// Add members to list
			for _, member := range members {
				if err := p.importRepo.UpdateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1); err != nil {
					p.logger.Warn("failed to update import progress",
						zap.String("import_id", event.ImportID),
						zap.Error(err))
				}

				if err := p.addToList(ctx, event.Username, listID, member); err != nil {
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to add %s to list: %v", member, err))
				} else {
					result.Success++
				}
			}
		}

	default:
		p.logger.Error("JSON import not supported for type",
			zap.String("type", event.Type),
			zap.String("import_id", event.ImportID))
		return result, ErrJSONImportNotSupportedForType
	}

	return result, nil
}

func (p *ImportProcessor) processActivityPubImport(ctx context.Context, event ImportProcessorEvent, data []byte, costTracking *models.ImportCostTracking) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	// For ActivityPub archives, we need to handle them differently based on type
	if event.Type != "archive" {
		return result, ErrActivityPubImportOnlySupportsArchive
	}

	var collection map[string]any
	if err := common.ParseActivityPubObject(data, &collection); err != nil {
		return result, errors.Join(ErrActivityPubCollectionParseFailed, err)
	}

	// Process ActivityPub collection items
	if items, ok := collection["orderedItems"].([]any); ok {
		return p.processActivityPubItems(ctx, event, items, costTracking)
	}

	// If no orderedItems, check for items array
	if items, ok := collection["items"].([]any); ok {
		return p.processActivityPubItems(ctx, event, items, costTracking)
	}

	return result, ErrNoItemsFoundInActivityPubCollection
}

// processActivityPubItems processes individual items in an ActivityPub collection
func (p *ImportProcessor) processActivityPubItems(ctx context.Context, event ImportProcessorEvent, items []any, costTracking *models.ImportCostTracking) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	// Process items with transaction safety
	for i, item := range items {
		// Update progress
		p.updateImportProgress(ctx, event.ImportID, i+1)

		// Process item in a transaction-like manner
		if err := p.processActivityPubItem(ctx, event, item, costTracking); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to import item %d: %v", i, err))

			// Skip item but continue processing
			continue
		}

		result.Success++

		// Track costs for ActivityPub processing
		if costTracking != nil {
			costTracking.DynamoDBOperations++
			costTracking.DynamoDBWriteUnits += 1.0
			costTracking.DynamoDBWriteCost += p.calculateDynamoDBWriteCost(1.0)
		}
	}

	return result, nil
}

// processActivityPubItem processes a single ActivityPub item
func (p *ImportProcessor) processActivityPubItem(ctx context.Context, event ImportProcessorEvent, item any, _ *models.ImportCostTracking) error {
	itemMap, ok := item.(map[string]any)
	if !ok {
		return ErrItemNotValidActivityPubObject
	}

	itemType, ok := itemMap["type"].(string)
	if !ok {
		return ErrItemMissingTypeField
	}

	switch itemType {
	case "Create":
		return p.importCreateActivity(ctx, event, itemMap)
	case "Follow":
		return p.importFollowActivity(ctx, event, itemMap)
	case "Like":
		return p.importLikeActivity(ctx, event, itemMap)
	case "Announce":
		return p.importAnnounceActivity(ctx, event, itemMap)
	case "Note", "Article":
		return p.importObject(ctx, event, itemMap)
	default:
		// Log unsupported type but don't fail
		p.logger.Info("skipping unsupported ActivityPub type",
			zap.String("type", itemType),
			zap.String("import_id", event.ImportID))
		return nil
	}
}

// importCreateActivity imports a Create activity
func (p *ImportProcessor) importCreateActivity(ctx context.Context, event ImportProcessorEvent, activityMap map[string]any) error {
	// Extract the object from the Create activity
	object, ok := activityMap["object"]
	if !ok {
		return ErrCreateActivityMissingObject
	}

	// Process the embedded object
	if objMap, ok := object.(map[string]any); ok {
		return p.importObject(ctx, event, objMap)
	}

	return ErrCreateActivityObjectNotValid
}

// importFollowActivity imports a Follow activity
func (p *ImportProcessor) importFollowActivity(ctx context.Context, event ImportProcessorEvent, activityMap map[string]any) error {
	target, ok := activityMap["object"].(string)
	if !ok {
		return ErrFollowActivityMissingTargetObject
	}

	return p.followAccount(ctx, event.Username, target)
}

// importLikeActivity imports a Like activity
func (p *ImportProcessor) importLikeActivity(ctx context.Context, event ImportProcessorEvent, activityMap map[string]any) error {
	objectID, ok := activityMap["object"].(string)
	if !ok {
		return ErrLikeActivityMissingObjectID
	}

	// Create a like/favorite
	like := models.NewLike(
		fmt.Sprintf("%s/users/%s", p.baseURL, event.Username),
		objectID,
		event.Username, // statusAuthorID - using the username of the person doing the like
	)

	return p.repos.Object().CreateObject(ctx, like)
}

// importAnnounceActivity imports an Announce (boost/share) activity
func (p *ImportProcessor) importAnnounceActivity(ctx context.Context, event ImportProcessorEvent, activityMap map[string]any) error {
	objectID, ok := activityMap["object"].(string)
	if !ok {
		return ErrAnnounceActivityMissingObjectID
	}

	// Create an announce
	announce := &models.Announce{
		Actor:  fmt.Sprintf("%s/users/%s", p.baseURL, event.Username),
		Object: objectID,
	}
	if err := announce.BeforeCreate(); err != nil {
		return errors.Join(ErrAnnouncePrepFailed, err)
	}

	return p.repos.Object().CreateObject(ctx, announce)
}

// importObject imports a generic ActivityPub object (Note, Article, etc.)
func (p *ImportProcessor) importObject(ctx context.Context, _ ImportProcessorEvent, objMap map[string]any) error {
	id, ok := objMap["id"].(string)
	if !ok {
		return ErrObjectMissingID
	}

	content, _ := objMap["content"].(string)
	published, _ := objMap["published"].(string)

	var publishedTime time.Time
	if published != "" {
		if pt, err := time.Parse(time.RFC3339, published); err == nil {
			publishedTime = pt
		} else {
			publishedTime = time.Now()
		}
	} else {
		publishedTime = time.Now()
	}

	// Create object record
	obj := &models.Object{
		ID:           id,
		Type:         getStringFromMap(objMap, "type"),
		Content:      content,
		AttributedTo: getStringFromMap(objMap, "attributedTo"),
		Published:    publishedTime,
		Updated:      time.Now(),
		IsRemote:     false, // Imported objects are treated as local
		CreatedAt:    time.Now(),
	}

	return p.repos.Object().CreateObject(ctx, obj)
}

// getStringFromMap safely gets a string value from a map
func getStringFromMap(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// CSV processing helper functions

// processFollowersCSV processes a followers CSV file
func (p *ImportProcessor) processFollowersCSV(reader *csv.Reader) (ImportResult, error) {
	result := ImportResult{Errors: make([]string, 0)}

	// For followers, we don't actually import them (they need to follow us)
	// Just count the records
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("CSV parse error: %v", err))
			continue
		}

		// Currently skips processing - future enhancement could send follow invites
		result.Skipped++
	}

	return result, nil
}

// processFollowingCSV processes a following CSV file
func (p *ImportProcessor) processFollowingCSV(ctx context.Context, event ImportProcessorEvent, reader *csv.Reader, costTracking *models.ImportCostTracking) (ImportResult, error) {
	result := ImportResult{Errors: make([]string, 0)}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("CSV parse error: %v", err))
			continue
		}

		if len(record) < 1 {
			continue
		}

		accountAddress := record[0]
		if err := common.ValidateRequiredParam("accountAddress", accountAddress); err != nil {
			continue
		}

		// Update progress and process follow
		p.updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

		if err := p.followAccount(ctx, event.Username, accountAddress); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to follow %s: %v", accountAddress, err))
		} else {
			result.Success++
			p.trackFollowCosts(costTracking)
		}
	}

	return result, nil
}

// processBlocksCSV processes a blocks CSV file
func (p *ImportProcessor) processBlocksCSV(ctx context.Context, event ImportProcessorEvent, reader *csv.Reader, costTracking *models.ImportCostTracking) (ImportResult, error) {
	result := ImportResult{Errors: make([]string, 0)}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			continue
		}

		if len(record) < 1 {
			continue
		}

		accountAddress := record[0]
		if err := common.ValidateRequiredParam("accountAddress", accountAddress); err != nil {
			continue
		}

		// Update progress and process block
		p.updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

		if err := p.blockAccount(ctx, event.Username, accountAddress); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to block %s: %v", accountAddress, err))
		} else {
			result.Success++
			p.trackBlockCosts(costTracking)
		}
	}

	return result, nil
}

// processMutesCSV processes a mutes CSV file
func (p *ImportProcessor) processMutesCSV(ctx context.Context, event ImportProcessorEvent, reader *csv.Reader, header []string) (ImportResult, error) {
	result := ImportResult{Errors: make([]string, 0)}

	// Find hide notifications column
	hideNotificationsIndex := p.findHideNotificationsIndex(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			continue
		}

		if len(record) < 1 {
			continue
		}

		accountAddress := record[0]
		if err := common.ValidateRequiredParam("accountAddress", accountAddress); err != nil {
			continue
		}

		hideNotifications := p.shouldHideNotifications(record, hideNotificationsIndex)

		// Update progress and process mute
		p.updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

		if err := p.muteAccount(ctx, event.Username, accountAddress, hideNotifications); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to mute %s: %v", accountAddress, err))
		} else {
			result.Success++
		}
	}

	return result, nil
}

// processBookmarksCSV processes a bookmarks CSV file
func (p *ImportProcessor) processBookmarksCSV(ctx context.Context, event ImportProcessorEvent, reader *csv.Reader) (ImportResult, error) {
	result := ImportResult{Errors: make([]string, 0)}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Failed++
			continue
		}

		if len(record) < 1 {
			continue
		}

		statusURL := record[0]
		if err := common.ValidateRequiredParam("statusURL", statusURL); err != nil {
			continue
		}

		// Update progress and process bookmark
		p.updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

		if err := p.bookmarkStatus(ctx, event.Username, statusURL); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to bookmark %s: %v", statusURL, err))
		} else {
			result.Success++
		}
	}

	return result, nil
}

// Helper utility functions

// updateImportProgress updates the import progress
func (p *ImportProcessor) updateImportProgress(ctx context.Context, importID string, progress int) {
	if err := p.importRepo.UpdateImportProgress(ctx, importID, progress); err != nil {
		p.logger.Warn("failed to update import progress",
			zap.String("import_id", importID),
			zap.Error(err))
	}
}

// trackFollowCosts tracks costs for follow operations
func (p *ImportProcessor) trackFollowCosts(costTracking *models.ImportCostTracking) {
	if costTracking != nil {
		costTracking.DynamoDBOperations += 2   // Follow creation + activity storage
		costTracking.DynamoDBWriteUnits += 2.0 // Estimated write capacity
		costTracking.DynamoDBWriteCost += p.calculateDynamoDBWriteCost(2.0)
		costTracking.ExternalAPICalls++ // WebFinger lookup
		costTracking.ExternalAPICallCost += p.calculateExternalAPICallCost(1)
	}
}

// trackBlockCosts tracks costs for block operations
func (p *ImportProcessor) trackBlockCosts(costTracking *models.ImportCostTracking) {
	if costTracking != nil {
		costTracking.DynamoDBOperations++
		costTracking.DynamoDBWriteUnits += 1.0
		costTracking.DynamoDBWriteCost += p.calculateDynamoDBWriteCost(1.0)
		costTracking.ExternalAPICalls++ // WebFinger lookup
		costTracking.ExternalAPICallCost += p.calculateExternalAPICallCost(1)
	}
}

// findHideNotificationsIndex finds the index of the "Hide notifications" column
func (p *ImportProcessor) findHideNotificationsIndex(header []string) int {
	for i, col := range header {
		if col == "Hide notifications" {
			return i
		}
	}
	return -1
}

// shouldHideNotifications determines if notifications should be hidden for a mute
func (p *ImportProcessor) shouldHideNotifications(record []string, hideNotificationsIndex int) bool {
	if hideNotificationsIndex >= 0 && len(record) > hideNotificationsIndex {
		result, _ := common.ParseAndValidateBoolean(record[hideNotificationsIndex])
		return result
	}
	return false
}

// Helper functions for performing the actual import operations

func (p *ImportProcessor) followAccount(ctx context.Context, username, targetAccount string) error {
	// Resolve the account via WebFinger if needed
	actorID := p.resolveAccount(ctx, targetAccount)

	// Create follow relationship using the storage interface
	follow := models.NewFollow(username, actorID, fmt.Sprintf("%s/activities/follow-%d", actorID, time.Now().Unix()))
	follow.State = models.FollowStateAccepted // Import assumes accepted

	if err := p.repos.Object().CreateObject(ctx, follow); err != nil {
		return errors.Join(ErrFollowRelationshipStore, err)
	}

	// Get the follower actor to send the follow activity
	followerActor, err := p.repos.Account().GetActor(ctx, username)
	if err != nil {
		return errors.Join(ErrFollowerActorGet, err)
	}

	// Create and send Follow activity to the remote actor
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      fmt.Sprintf("%s/activities/follow-%d", followerActor.ID, time.Now().Unix()),
			To:      []string{actorID},
		},
		Actor:  followerActor.ID,
		Object: actorID,
	}
	now := time.Now()
	followActivity.Published = &now

	// Store the activity in the outbox (this will trigger delivery)
	err = p.repos.Activity().CreateActivity(ctx, followActivity)
	if err != nil {
		p.logger.Warn("failed to store follow activity in outbox",
			zap.String("follower", username),
			zap.String("target", actorID),
			zap.Error(err))
		// Don't fail the import if activity delivery fails
	} else {
		p.logger.Info("follow activity created for delivery",
			zap.String("follower", username),
			zap.String("target", actorID),
			zap.String("activity_id", followActivity.ID))
	}

	return nil
}

func (p *ImportProcessor) blockAccount(ctx context.Context, username, targetAccount string) error {
	// Resolve the account
	actorID := p.resolveAccount(ctx, targetAccount)

	// Create block using the storage interface
	block := &models.Block{
		Actor:  fmt.Sprintf("%s/users/%s", p.baseURL, username),
		Object: actorID,
	}
	if err := block.BeforeCreate(); err != nil {
		return errors.Join(ErrBlockPrepFailed, err)
	}

	return p.repos.Object().CreateObject(ctx, block)
}

func (p *ImportProcessor) muteAccount(ctx context.Context, username, targetAccount string, hideNotifications bool) error {
	// Resolve the account
	actorID := p.resolveAccount(ctx, targetAccount)

	// Create mute using the storage interface
	mute := &models.Mute{
		Actor:             fmt.Sprintf("%s/users/%s", p.baseURL, username),
		Object:            actorID,
		HideNotifications: hideNotifications,
	}
	if err := mute.BeforeCreate(); err != nil {
		return errors.Join(ErrMutePrepFailed, err)
	}

	return p.repos.Object().CreateObject(ctx, mute)
}

func (p *ImportProcessor) bookmarkStatus(ctx context.Context, username, statusURL string) error {
	// Extract status ID from URL
	// This is simplified - would need proper URL parsing
	statusID := strings.TrimPrefix(statusURL, p.baseURL+"/")

	// Create bookmark using the Bookmark model
	bookmark := &models.Bookmark{
		Username:  username,
		ObjectID:  statusID,
		CreatedAt: time.Now(),
	}

	// Prepare the bookmark for creation
	if err := bookmark.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update bookmark keys: %w", err)
	}

	// Create the bookmark in storage
	if err := p.repos.Object().CreateObject(ctx, bookmark); err != nil {
		return errors.Join(ErrBookmarkCreate, err)
	}

	p.logger.Info("created bookmark",
		zap.String("username", username),
		zap.String("status_id", statusID),
		zap.String("status_url", statusURL))

	return nil
}

func (p *ImportProcessor) createOrUpdateList(ctx context.Context, username, listName string) (string, error) {
	// Generate list ID
	listID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create list using the storage interface
	list := &models.List{
		ID:            listID,
		Title:         listName,
		RepliesPolicy: "list", // Default
		Username:      username,
	}
	if err := list.BeforeCreate(); err != nil {
		return "", errors.Join(ErrListPrepFailed, err)
	}

	if err := p.repos.Object().CreateObject(ctx, list); err != nil {
		return "", errors.Join(ErrListCreate, err)
	}

	return listID, nil
}

func (p *ImportProcessor) addToList(ctx context.Context, username, listID, accountAddress string) error {
	// Resolve the account
	actorID := p.resolveAccount(ctx, accountAddress)

	// Add member to list using the storage interface
	listMember := &models.ListMember{
		ListID:       listID,
		AccountID:    actorID,
		ListUsername: username,
	}
	if err := listMember.BeforeCreate(); err != nil {
		return errors.Join(ErrListMemberPrepFailed, err)
	}

	return p.repos.Object().CreateObject(ctx, listMember)
}

func (p *ImportProcessor) resolveAccount(_ context.Context, accountAddress string) string {
	// If it's already a full actor ID, return it
	if strings.HasPrefix(accountAddress, "https://") {
		return accountAddress
	}

	// Parse account address (user@domain)
	parts := strings.Split(accountAddress, "@")
	if len(parts) != 2 {
		// Assume local user if no domain
		return fmt.Sprintf("%s/users/%s", p.baseURL, accountAddress)
	}

	username := parts[0]
	domain := parts[1]

	// Check if it's a local user
	if domain == strings.TrimPrefix(p.baseURL, "https://") {
		return fmt.Sprintf("%s/users/%s", p.baseURL, username)
	}

	// For import processing, use a simple fallback without WebFinger resolution
	// to avoid storage interface compatibility issues during migration
	p.logger.Info("resolving remote account for import",
		zap.String("username", username),
		zap.String("domain", domain))

	// Construct likely actor ID as fallback
	return fmt.Sprintf("https://%s/users/%s", domain, username)
}

// Helper functions for status updates

func (p *ImportProcessor) downloadFromS3(ctx context.Context, key string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(p.bucketName),
		Key:    aws.String(key),
	}

	result, err := p.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, errors.Join(ErrS3ObjectGet, err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			p.logger.Warn("failed to close S3 object body", zap.Error(closeErr))
		}
	}()

	return io.ReadAll(result.Body)
}

// These functions have been removed and replaced by ImportRepository methods:
// - updateImportStatus is now handled by importRepo.UpdateImportStatus
// - updateImportProgress is now handled by importRepo.UpdateImportProgress
// This eliminates direct DynamoDB SDK usage in favor of DynamORM patterns.

// Cost calculation helper functions

// calculateLambdaCost calculates the cost of Lambda execution
// Lambda pricing: $0.0000166667 per GB-second (assumes 512MB memory)
func (p *ImportProcessor) calculateLambdaCost(durationMs int64) int64 {
	const memoryGB = 0.5                 // 512MB = 0.5GB
	const costPerGBSecond = 0.0000166667 // USD per GB-second

	durationSeconds := float64(durationMs) / 1000.0
	costDollars := memoryGB * durationSeconds * costPerGBSecond

	// Convert to microcents (1 dollar = 1,000,000 microcents)
	return int64(costDollars * 1_000_000)
}

// calculateS3GetCost calculates the cost of S3 GET requests
// S3 GET pricing: $0.0004 per 1,000 requests
func (p *ImportProcessor) calculateS3GetCost(requestCount int64) int64 {
	const costPer1000Requests = 0.0004 // USD per 1,000 requests

	costDollars := float64(requestCount) * costPer1000Requests / 1000.0

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateS3DataTransferCost calculates the cost of S3 data transfer
// S3 data transfer pricing: $0.09 per GB (outbound)
func (p *ImportProcessor) calculateS3DataTransferCost(transferBytes int64) int64 {
	const costPerGB = 0.09 // USD per GB

	transferGB := float64(transferBytes) / (1024 * 1024 * 1024) // Convert bytes to GB
	costDollars := transferGB * costPerGB

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateDynamoDBWriteCost calculates the cost of DynamoDB write operations
// DynamoDB pricing: $1.25 per million write requests
func (p *ImportProcessor) calculateDynamoDBWriteCost(writeUnits float64) int64 {
	const costPerMillionWrites = 1.25 // USD per million write requests

	costDollars := (writeUnits / 1_000_000) * costPerMillionWrites

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateExternalAPICallCost calculates the cost of external API calls
// Estimated cost: $0.001 per call (WebFinger, ActivityPub lookups)
func (p *ImportProcessor) calculateExternalAPICallCost(callCount int64) int64 {
	const costPerCall = 0.001 // USD per call

	costDollars := float64(callCount) * costPerCall

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}
