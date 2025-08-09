// Package main implements the export-generator Lambda function for generating user data exports.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// ExportProcessor handles data export generation from SQS messages
type ExportProcessor struct {
	db               core.DB
	repos            storageCore.RepositoryStorage
	exportRepo       *repositories.ExportRepository
	costTrackingRepo *repositories.CostTrackingRepository
	s3Client         *s3.Client
	logger           *zap.Logger
	tableName        string
	bucketName       string
	baseURL          string
}

var (
	processor *ExportProcessor
	cfg       *config.Config
)

// ExportGeneratorEvent represents the event triggered for export generation
type ExportGeneratorEvent struct {
	ExportID     string         `json:"export_id"`
	Username     string         `json:"username"`
	Type         string         `json:"type"`   // archive, followers, following, etc.
	Format       string         `json:"format"` // activitypub, mastodon, csv
	Options      map[string]any `json:"options"`
	IncludeMedia bool           `json:"include_media"`
	DateRange    *DateRange     `json:"date_range"`
}

// DateRange for filtering exports
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func init() {
	logger := common.Logger()
	cfg = config.Get()

	// Load AWS config
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repository factory
	repos, err := factory.NewRepositoryFactory(db, cfg.DynamoTableName, awsConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize export repository
	exportRepo := repositories.NewExportRepository(db, cfg.DynamoTableName, logger)

	// Initialize cost tracking repository
	costTrackingRepo := repositories.NewCostTrackingRepository(db, cfg.DynamoTableName, logger)

	// Create processor instance
	processor = &ExportProcessor{
		db:               db,
		repos:            repos,
		exportRepo:       exportRepo,
		costTrackingRepo: costTrackingRepo,
		logger:           logger,
		tableName:        cfg.DynamoTableName,
		bucketName:       cfg.S3BucketName,
		baseURL:          cfg.BaseURL(),
	}

	if processor.bucketName == "" {
		logger.Fatal("S3_BUCKET_NAME configuration not set")
	}
	if processor.baseURL == "" {
		processor.baseURL = "https://example.com" // Default
	}
}

func main() {
	// Use the SQS pattern from our Lift patterns
	patterns.StartSQSLambda("export-generator", processor, processor.logger)
}

// HandleSQS implements the SQS handler interface for Lift
func (ep *ExportProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Initialize AWS clients
	if err := ep.initializeAWSClients(ctx.Request.Context()); err != nil {
		ep.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	ep.logger.Info("processing export generation batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)))

	// Process each message
	for _, message := range event.Records {
		var exportEvent ExportGeneratorEvent
		if err := common.ParseRequestBody([]byte(message.Body), &exportEvent); err != nil {
			ep.logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.String("request_id", ctx.GetRequestID()),
				zap.Error(err))
			continue
		}

		if err := ep.processExportJob(ctx.Request.Context(), exportEvent); err != nil {
			ep.logger.Error("failed to process export job",
				zap.String("export_id", exportEvent.ExportID),
				zap.String("username", exportEvent.Username),
				zap.String("request_id", ctx.GetRequestID()),
				zap.Error(err))
			// Update job status as failed
			if updateErr := ep.exportRepo.UpdateExportStatus(ctx.Request.Context(), exportEvent.ExportID, "failed", nil, err.Error()); updateErr != nil {
				ep.logger.Error("failed to update export status to failed",
					zap.String("export_id", exportEvent.ExportID),
					zap.Error(updateErr))
			}
		}
	}

	return nil
}

func (ep *ExportProcessor) initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize S3 client
	ep.s3Client = s3.NewFromConfig(awsCfg)

	return nil
}

func (ep *ExportProcessor) processExportJob(ctx context.Context, event ExportGeneratorEvent) error {
	ep.logger.Info("processing export job",
		zap.String("export_id", event.ExportID),
		zap.String("username", event.Username),
		zap.String("type", event.Type),
		zap.String("format", event.Format))

	// Initialize cost tracking
	startTime := time.Now()
	exportCostTracking := &models.ExportCostTracking{
		ExportID:     event.ExportID,
		Username:     event.Username,
		Type:         event.Type,
		Format:       event.Format,
		IncludeMedia: event.IncludeMedia,
		Status:       "processing",
		StartedAt:    startTime,
	}

	// Note: Budget enforcement should be implemented at the API level before jobs are queued
	// This ensures users cannot exceed limits even if multiple exports are processed simultaneously

	// Track the export job completion
	defer func() {
		if exportCostTracking.CompletedAt == nil {
			completedAt := time.Now()
			exportCostTracking.CompletedAt = &completedAt
		}

		// Calculate Lambda execution cost
		exportCostTracking.LambdaDurationMs = time.Since(startTime).Milliseconds()
		exportCostTracking.LambdaExecutionCost = ep.calculateLambdaCost(exportCostTracking.LambdaDurationMs)

		// Calculate total cost
		exportCostTracking.CalculateTotalCost()

		// Save cost tracking record
		if err := ep.costTrackingRepo.Create(ctx, &models.DynamoDBCostRecord{
			Table:                ep.tableName,
			OperationType:        "ExportGeneration",
			EstimatedCostDollars: exportCostTracking.GetTotalCostDollars(),
			TotalCostMicroCents:  exportCostTracking.TotalCostMicroCents,
			ServiceName:          "export-generator",
			RequestDuration:      time.Since(startTime).Milliseconds(),
			Properties: map[string]interface{}{
				"username":  event.Username,
				"export_id": event.ExportID,
			},
		}); err != nil {
			ep.logger.Error("failed to save export cost tracking",
				zap.String("export_id", event.ExportID),
				zap.Error(err))
		}

		// Create a dedicated ImportRepository instance to update budget usage
		// This tracks actual costs against user budgets
		importRepo := repositories.NewImportRepository(ep.db, ep.tableName, ep.logger)
		if err := importRepo.UpdateBudgetUsage(ctx, event.Username, "daily", 0, exportCostTracking.TotalCostMicroCents); err != nil {
			ep.logger.Warn("failed to update budget usage",
				zap.String("export_id", event.ExportID),
				zap.String("username", event.Username),
				zap.Error(err))
		}
	}()

	// Update job status to processing
	if err := ep.exportRepo.UpdateExportStatus(ctx, event.ExportID, "processing", nil, ""); err != nil {
		ep.logger.Warn("failed to update export status", zap.Error(err))
	}

	// Generate export based on type and format
	var exportData []byte
	var filename string
	var contentType string
	var recordCount int
	var err error

	switch event.Format {
	case "csv":
		exportData, recordCount, err = ep.generateCSVExport(ctx, event, exportCostTracking)
		filename = fmt.Sprintf("%s_%s.csv", event.Username, event.Type)
		contentType = "text/csv"

	case "activitypub":
		exportData, recordCount, err = ep.generateActivityPubExport(ctx, event, exportCostTracking)
		filename = fmt.Sprintf("%s_activitypub_archive.zip", event.Username)
		contentType = "application/zip"

	case "mastodon":
		exportData, recordCount, err = ep.generateMastodonExport(ctx, event, exportCostTracking)
		filename = fmt.Sprintf("%s_mastodon_archive.zip", event.Username)
		contentType = "application/zip"

	default:
		return fmt.Errorf("unsupported export format: %s", event.Format)
	}

	if err != nil {
		return fmt.Errorf("failed to generate export: %w", err)
	}

	// Upload to S3 and track costs
	s3Key := fmt.Sprintf("exports/%s/%s/%s", event.Username, event.ExportID, filename)
	if err := ep.uploadToS3(ctx, s3Key, exportData, contentType, exportCostTracking); err != nil {
		return fmt.Errorf("failed to upload export: %w", err)
	}

	// Update export cost tracking with file metrics
	exportCostTracking.FileSize = int64(len(exportData))
	exportCostTracking.RecordCount = int64(recordCount)
	exportCostTracking.Status = "completed"

	// Generate pre-signed URL (24 hour expiry)
	presignClient := s3.NewPresignClient(ep.s3Client)
	presignReq, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ep.bucketName),
		Key:    aws.String(s3Key),
	}, s3.WithPresignExpires(24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to generate pre-signed URL: %w", err)
	}

	// Update export job as completed
	completionData := map[string]any{
		"download_url": presignReq.URL,
		"expires_at":   time.Now().Add(24 * time.Hour),
		"file_size":    len(exportData),
		"record_count": recordCount,
		"s3_key":       s3Key,
	}

	if err := ep.exportRepo.UpdateExportStatus(ctx, event.ExportID, "completed", completionData, ""); err != nil {
		return fmt.Errorf("failed to update export status: %w", err)
	}

	ep.logger.Info("export completed",
		zap.String("export_id", event.ExportID),
		zap.String("s3_key", s3Key),
		zap.Int("file_size", len(exportData)),
		zap.Int("record_count", recordCount))

	return nil
}

func (ep *ExportProcessor) generateCSVExport(ctx context.Context, event ExportGeneratorEvent, costTracking *models.ExportCostTracking) ([]byte, int, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	
	recordCount, err := ep.writeCSVData(ctx, writer, event, costTracking)
	if err != nil {
		return nil, 0, err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, 0, fmt.Errorf("CSV writer error: %w", err)
	}
	return buf.Bytes(), recordCount, nil
}

// writeCSVData writes the appropriate CSV data based on export type
func (ep *ExportProcessor) writeCSVData(ctx context.Context, writer *csv.Writer, event ExportGeneratorEvent, costTracking *models.ExportCostTracking) (int, error) {
	switch event.Type {
	case "followers":
		return ep.writeFollowersCSV(ctx, writer, event.Username, costTracking)
	case "following":
		return ep.writeFollowingCSV(ctx, writer, event.Username, costTracking)
	case "blocks":
		return ep.writeBlocksCSV(ctx, writer, event.Username)
	case "mutes":
		return ep.writeMutesCSV(ctx, writer, event.Username)
	case "lists":
		return ep.writeListsCSV(ctx, writer, event.Username)
	case "bookmarks":
		return ep.writeBookmarksCSV(ctx, writer, event.Username)
	case "domain_blocks":
		return ep.writeDomainBlocksCSV(ctx, writer, event.Username)
	default:
		return 0, fmt.Errorf("CSV export not supported for type: %s", event.Type)
	}
}

// writeFollowersCSV writes followers data to CSV
func (ep *ExportProcessor) writeFollowersCSV(ctx context.Context, writer *csv.Writer, username string, costTracking *models.ExportCostTracking) (int, error) {
	// Write header
	_ = writer.Write([]string{"Account address", "Show boosts", "Notify on new posts", "Languages"})

	// Get followers
	followers, err := ep.getFollowers(ctx, username)
	if err != nil {
		return 0, err
	}

	// Track costs
	ep.trackReadCosts(costTracking, len(followers))

	// Write data
	return ep.writeAccountRows(writer, followers)
}

// writeFollowingCSV writes following data to CSV
func (ep *ExportProcessor) writeFollowingCSV(ctx context.Context, writer *csv.Writer, username string, costTracking *models.ExportCostTracking) (int, error) {
	// Write header
	_ = writer.Write([]string{"Account address", "Show boosts", "Notify on new posts", "Languages"})

	// Get following
	following, err := ep.getFollowing(ctx, username)
	if err != nil {
		return 0, err
	}

	// Track costs
	ep.trackReadCosts(costTracking, len(following))

	// Write data
	return ep.writeAccountRows(writer, following)
}

// writeAccountRows writes account data rows with default values
func (ep *ExportProcessor) writeAccountRows(writer *csv.Writer, accounts []string) (int, error) {
	recordCount := 0
	for _, account := range accounts {
		_ = writer.Write([]string{
			account,
			"true",  // Show boosts default
			"false", // Notify default
			"",      // Languages default
		})
		recordCount++
	}
	return recordCount, nil
}

// writeBlocksCSV writes blocks data to CSV
func (ep *ExportProcessor) writeBlocksCSV(ctx context.Context, writer *csv.Writer, username string) (int, error) {
	// Write header
	_ = writer.Write([]string{"Account address"})

	// Get blocks
	blocks, err := ep.getBlocks(ctx, username)
	if err != nil {
		return 0, err
	}

	// Write data
	recordCount := 0
	for _, block := range blocks {
		_ = writer.Write([]string{block})
		recordCount++
	}
	return recordCount, nil
}

// writeMutesCSV writes mutes data to CSV
func (ep *ExportProcessor) writeMutesCSV(ctx context.Context, writer *csv.Writer, username string) (int, error) {
	// Write header
	_ = writer.Write([]string{"Account address", "Hide notifications"})

	// Get mutes
	mutes, err := ep.getMutes(ctx, username)
	if err != nil {
		return 0, err
	}

	// Write data
	recordCount := 0
	for _, mute := range mutes {
		_ = writer.Write([]string{
			mute.AccountID,
			fmt.Sprintf("%t", mute.HideNotifications),
		})
		recordCount++
	}
	return recordCount, nil
}

// writeListsCSV writes lists data to CSV
func (ep *ExportProcessor) writeListsCSV(ctx context.Context, writer *csv.Writer, username string) (int, error) {
	// Write header
	_ = writer.Write([]string{"List name", "Account address"})

	// Get lists with members
	lists, err := ep.getListsWithMembers(ctx, username)
	if err != nil {
		return 0, err
	}

	// Write data
	recordCount := 0
	for listName, members := range lists {
		for _, member := range members {
			_ = writer.Write([]string{listName, member})
			recordCount++
		}
	}
	return recordCount, nil
}

// writeBookmarksCSV writes bookmarks data to CSV
func (ep *ExportProcessor) writeBookmarksCSV(ctx context.Context, writer *csv.Writer, username string) (int, error) {
	// Write header
	_ = writer.Write([]string{"Status URL", "Bookmarked at"})

	// Get bookmarks
	bookmarks, err := ep.getBookmarks(ctx, username)
	if err != nil {
		return 0, err
	}

	// Write data
	recordCount := 0
	for _, bookmark := range bookmarks {
		_ = writer.Write([]string{
			bookmark.StatusURL,
			bookmark.CreatedAt.Format(time.RFC3339),
		})
		recordCount++
	}
	return recordCount, nil
}

// writeDomainBlocksCSV writes domain blocks data to CSV
func (ep *ExportProcessor) writeDomainBlocksCSV(ctx context.Context, writer *csv.Writer, username string) (int, error) {
	// Write header
	_ = writer.Write([]string{"Domain"})

	// Get domain blocks
	domainBlocks, err := ep.getDomainBlocks(ctx, username)
	if err != nil {
		return 0, err
	}

	// Write data
	recordCount := 0
	for _, domain := range domainBlocks {
		_ = writer.Write([]string{domain})
		recordCount++
	}
	return recordCount, nil
}

// trackReadCosts tracks DynamoDB read costs for export operations
func (ep *ExportProcessor) trackReadCosts(costTracking *models.ExportCostTracking, itemCount int) {
	if costTracking == nil {
		return
	}
	
	// Estimate read capacity units (1 RCU per item, pagination queries)
	estimatedRCU := float64(itemCount) / 1000 * 5 // 5 RCU per 1000 items (pagination overhead)
	if estimatedRCU < 1 {
		estimatedRCU = 1
	}
	
	costTracking.DynamoDBReadUnits += estimatedRCU
	costTracking.DynamoDBOperations++
	costTracking.DynamoDBReadCost += ep.calculateDynamoDBReadCost(estimatedRCU)
}

func (ep *ExportProcessor) generateActivityPubExport(ctx context.Context, event ExportGeneratorEvent, costTracking *models.ExportCostTracking) ([]byte, int, error) {
	// Create ZIP archive
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	recordCount := 0

	// Get actor data
	actor, err := ep.getActor(ctx, event.Username)
	if err != nil {
		return nil, 0, err
	}

	// Write actor.json
	actorJSON, _ := json.MarshalIndent(actor, "", "  ")
	if err := addFileToZip(zipWriter, "actor.json", actorJSON); err != nil {
		return nil, 0, err
	}

	// Write collections based on export type
	if event.Type == "archive" {
		// Export everything for archive type

		// Outbox (posts)
		outbox, count, err := ep.getOutbox(ctx, event.Username, event.DateRange)
		if err != nil {
			return nil, 0, err
		}
		recordCount += count

		outboxJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/outbox", ep.baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   count,
			"orderedItems": outbox,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "outbox.json", outboxJSON); err != nil {
			return nil, 0, err
		}

		// Following collection
		following, err := ep.getFollowingActors(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		followingJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/following", ep.baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(following),
			"orderedItems": following,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "following.json", followingJSON); err != nil {
			return nil, 0, err
		}

		// Followers collection
		followers, err := ep.getFollowersActors(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		followersJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/followers", ep.baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(followers),
			"orderedItems": followers,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "followers.json", followersJSON); err != nil {
			return nil, 0, err
		}

		// Likes collection
		likes, err := ep.getLikes(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		likesJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/likes", ep.baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(likes),
			"orderedItems": likes,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "likes.json", likesJSON); err != nil {
			return nil, 0, err
		}

		// Add media files if IncludeMedia is true
		if event.IncludeMedia {
			mediaCount, err := ep.includeMediaFiles(ctx, zipWriter, event.Username, event.DateRange)
			if err != nil {
				ep.logger.Error("failed to include media files", zap.Error(err))
				// Don't fail the export, just log the error
			} else {
				ep.logger.Info("included media files in export", zap.Int("count", mediaCount))

				// Track media file costs (estimate based on media count)
				if costTracking != nil {
					costTracking.MediaFilesIncluded = int64(mediaCount)
					costTracking.S3GetRequests += int64(mediaCount)
					costTracking.S3GetRequestCost += ep.calculateS3GetCost(int64(mediaCount))

					// Estimate media size (approximate 2MB per file)
					estimatedMediaSize := int64(mediaCount) * 2 * 1024 * 1024 // 2MB per file
					costTracking.MediaSizeBytes = estimatedMediaSize
					costTracking.S3DataTransferCost += ep.calculateS3DataTransferCost(estimatedMediaSize)
				}
			}
		}
	}

	if err := zipWriter.Close(); err != nil {
		ep.logger.Error("failed to close zip writer", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf.Bytes(), recordCount, nil
}

func (ep *ExportProcessor) generateMastodonExport(ctx context.Context, event ExportGeneratorEvent, costTracking *models.ExportCostTracking) ([]byte, int, error) {
	// Create ZIP archive
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	recordCount := 0

	// Get actor data and convert to Mastodon format
	actor, err := ep.getActor(ctx, event.Username)
	if err != nil {
		return nil, 0, err
	}

	// Create Mastodon-compatible actor.json
	mastodonActor := map[string]any{
		"@context": []any{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
		},
		"id":                        actor.ID,
		"type":                      actor.Type,
		"inbox":                     actor.Inbox,
		"outbox":                    actor.Outbox,
		"preferredUsername":         actor.PreferredUsername,
		"name":                      actor.Name,
		"summary":                   actor.Summary,
		"url":                       actor.URL,
		"manuallyApprovesFollowers": actor.ManuallyApprovesFollowers,
		"discoverable":              actor.Discoverable,
		"publicKey":                 actor.PublicKey,
		"endpoints":                 actor.Endpoints,
	}

	actorJSON, _ := json.MarshalIndent(mastodonActor, "", "  ")
	if err := addFileToZip(zipWriter, "actor.json", actorJSON); err != nil {
		return nil, 0, err
	}

	if event.Type == "archive" {
		// Create outbox.json with all posts
		outbox, count, err := ep.getOutbox(ctx, event.Username, event.DateRange)
		if err != nil {
			return nil, 0, err
		}
		recordCount += count

		outboxJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/outbox", ep.baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   count,
			"orderedItems": outbox,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "outbox.json", outboxJSON); err != nil {
			return nil, 0, err
		}

		// Create likes.json
		likes, err := ep.getLikes(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		likesJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"type":         "OrderedCollection",
			"orderedItems": likes,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "likes.json", likesJSON); err != nil {
			return nil, 0, err
		}

		// Create bookmarks.json
		bookmarks, err := ep.getBookmarksForExport(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		bookmarksJSON, _ := json.MarshalIndent(map[string]any{
			"@context":     activitypub.Context,
			"type":         "OrderedCollection",
			"orderedItems": bookmarks,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "bookmarks.json", bookmarksJSON); err != nil {
			return nil, 0, err
		}

		// Create lists.json
		lists, err := ep.getListsForExport(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		listsJSON, _ := json.MarshalIndent(lists, "", "  ")
		if err := addFileToZip(zipWriter, "lists.json", listsJSON); err != nil {
			return nil, 0, err
		}

		// Add media_attachments directory if IncludeMedia is true
		if event.IncludeMedia {
			mediaCount, err := ep.includeMediaFiles(ctx, zipWriter, event.Username, event.DateRange)
			if err != nil {
				ep.logger.Error("failed to include media files", zap.Error(err))
				// Don't fail the export, just log the error
			} else {
				ep.logger.Info("included media files in export", zap.Int("count", mediaCount))

				// Track media file costs (estimate based on media count)
				if costTracking != nil {
					costTracking.MediaFilesIncluded = int64(mediaCount)
					costTracking.S3GetRequests += int64(mediaCount)
					costTracking.S3GetRequestCost += ep.calculateS3GetCost(int64(mediaCount))

					// Estimate media size (approximate 2MB per file)
					estimatedMediaSize := int64(mediaCount) * 2 * 1024 * 1024 // 2MB per file
					costTracking.MediaSizeBytes = estimatedMediaSize
					costTracking.S3DataTransferCost += ep.calculateS3DataTransferCost(estimatedMediaSize)
				}
			}
		}
	}

	if err := zipWriter.Close(); err != nil {
		ep.logger.Error("failed to close zip writer", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf.Bytes(), recordCount, nil
}

// Helper functions for data retrieval
func (ep *ExportProcessor) getActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Get actor using storage client (already migrated to DynamORM)
	actor, err := ep.repos.Account().GetActor(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return actor, nil
}

func (ep *ExportProcessor) getFollowers(ctx context.Context, username string) ([]string, error) {
	// Query followers from DynamoDB using storage client
	var allFollowers []string
	cursor := ""

	for {
		followers, nextCursor, err := ep.repos.Relationship().GetFollowers(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get followers", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get followers: %w", err)
		}

		// Convert actor IDs to Mastodon handles for CSV export
		for _, follower := range followers {
			handle := ep.convertActorIDToHandle(follower)
			allFollowers = append(allFollowers, handle)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowers, nil
}

func (ep *ExportProcessor) getFollowing(ctx context.Context, username string) ([]string, error) {
	// Query following from DynamoDB using storage client
	var allFollowing []string
	cursor := ""

	for {
		following, nextCursor, err := ep.repos.Relationship().GetFollowing(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get following", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get following: %w", err)
		}

		// Convert actor IDs to Mastodon handles for CSV export
		for _, follow := range following {
			handle := ep.convertActorIDToHandle(follow)
			allFollowing = append(allFollowing, handle)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowing, nil
}

func (ep *ExportProcessor) getBlocks(ctx context.Context, username string) ([]string, error) {
	// Query blocks from DynamoDB using storage client
	var allBlocks []string
	cursor := ""

	for {
		blocks, nextCursor, err := ep.repos.Social().GetBlockedUsers(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get blocked actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get blocked actors: %w", err)
		}

		for _, block := range blocks {
			handle := ep.convertActorIDToHandle(block.Object)
			allBlocks = append(allBlocks, handle)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allBlocks, nil
}

// MuteInfo contains information about a muted account
type MuteInfo struct {
	AccountID         string
	HideNotifications bool
}

func (ep *ExportProcessor) getMutes(ctx context.Context, username string) ([]MuteInfo, error) {
	// Query mutes from DynamoDB using storage client
	var allMutes []MuteInfo
	cursor := ""

	for {
		mutes, nextCursor, err := ep.repos.Social().GetMutedUsers(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get muted actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get muted actors: %w", err)
		}

		for _, mute := range mutes {
			allMutes = append(allMutes, MuteInfo{
				AccountID:         ep.convertActorIDToHandle(mute.Object),
				HideNotifications: mute.HideNotifications,
			})
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allMutes, nil
}

func (ep *ExportProcessor) getListsWithMembers(ctx context.Context, username string) (map[string][]string, error) {
	// Query lists and their members from DynamoDB using storage client
	lists, err := ep.repos.List().GetListsForUser(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	result := make(map[string][]string)

	for _, list := range lists {
		members, err := ep.repos.List().GetListAccounts(ctx, list.ID)
		if err != nil {
			ep.logger.Error("failed to get list members",
				zap.String("list_id", list.ID),
				zap.String("list_title", list.Title),
				zap.Error(err))
			// Continue with other lists even if one fails
			continue
		}

		// Convert member IDs to Mastodon handles
		var handleMembers []string
		for _, member := range members {
			handleMembers = append(handleMembers, ep.convertActorIDToHandle(member))
		}

		result[list.Title] = handleMembers
	}

	return result, nil
}

// BookmarkInfo contains information about a bookmarked status
type BookmarkInfo struct {
	StatusURL string
	CreatedAt time.Time
}

func (ep *ExportProcessor) getBookmarks(ctx context.Context, username string) ([]BookmarkInfo, error) {
	var allBookmarks []BookmarkInfo
	cursor := ""

	for {
		bookmarkIDs, nextCursor, err := ep.fetchBookmarkBatch(ctx, username, cursor)
		if err != nil {
			return nil, err
		}

		// Convert bookmark IDs to BookmarkInfo
		bookmarks := ep.convertBookmarkIDsToInfo(ctx, bookmarkIDs)
		allBookmarks = append(allBookmarks, bookmarks...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allBookmarks, nil
}

// fetchBookmarkBatch fetches a batch of bookmark IDs from the repository
func (ep *ExportProcessor) fetchBookmarkBatch(ctx context.Context, username, cursor string) ([]string, string, error) {
	bookmarkIDs, nextCursor, err := ep.repos.User().GetBookmarks(ctx, username, 1000, cursor)
	if err != nil {
		ep.logger.Error("failed to get bookmarks", 
			zap.String("username", username), 
			zap.Error(err))
		return nil, "", fmt.Errorf("get bookmarks: %w", err)
	}
	return bookmarkIDs, nextCursor, nil
}

// convertBookmarkIDsToInfo converts bookmark IDs to BookmarkInfo objects
func (ep *ExportProcessor) convertBookmarkIDsToInfo(ctx context.Context, bookmarkIDs []string) []BookmarkInfo {
	var bookmarks []BookmarkInfo
	
	for _, bookmarkID := range bookmarkIDs {
		info := ep.convertSingleBookmark(ctx, bookmarkID)
		if info != nil {
			bookmarks = append(bookmarks, *info)
		}
	}
	
	return bookmarks
}

// convertSingleBookmark converts a single bookmark ID to BookmarkInfo
func (ep *ExportProcessor) convertSingleBookmark(ctx context.Context, bookmarkID string) *BookmarkInfo {
	obj, err := ep.repos.Object().GetObject(ctx, bookmarkID)
	if err != nil {
		ep.logger.Warn("failed to get bookmarked object",
			zap.String("bookmark_id", bookmarkID),
			zap.Error(err))
		return nil
	}

	statusURL, createdAt := ep.extractBookmarkData(obj)
	if statusURL == "" {
		return nil
	}

	return &BookmarkInfo{
		StatusURL: statusURL,
		CreatedAt: createdAt,
	}
}

// extractBookmarkData extracts URL and creation time from an object
func (ep *ExportProcessor) extractBookmarkData(obj any) (string, time.Time) {
	switch v := obj.(type) {
	case map[string]any:
		return ep.extractBookmarkFromMap(v)
	case *activitypub.Note:
		return ep.extractBookmarkFromNote(v)
	case *activitypub.Article:
		return ep.extractBookmarkFromArticle(v)
	default:
		return ep.extractBookmarkFromBaseObject(v)
	}
}

// extractBookmarkFromMap extracts bookmark data from a map
func (ep *ExportProcessor) extractBookmarkFromMap(v map[string]any) (string, time.Time) {
	var statusURL string
	var createdAt time.Time
	
	if url, ok := v["url"].(string); ok {
		statusURL = url
	} else if id, ok := v["id"].(string); ok {
		statusURL = id // Fallback to ID if no URL
	}
	
	if published, ok := v["published"].(string); ok {
		createdAt, _ = time.Parse(time.RFC3339, published)
	}
	
	return statusURL, createdAt
}

// extractBookmarkFromNote extracts bookmark data from a Note
func (ep *ExportProcessor) extractBookmarkFromNote(v *activitypub.Note) (string, time.Time) {
	statusURL := v.ID
	createdAt := time.Now()
	
	if v.Published != nil {
		createdAt = *v.Published
	}
	
	return statusURL, createdAt
}

// extractBookmarkFromArticle extracts bookmark data from an Article
func (ep *ExportProcessor) extractBookmarkFromArticle(v *activitypub.Article) (string, time.Time) {
	statusURL := v.ID
	createdAt := time.Now()
	
	if v.Published != nil {
		createdAt = *v.Published
	}
	
	return statusURL, createdAt
}

// extractBookmarkFromBaseObject tries to extract bookmark data from a BaseObject
func (ep *ExportProcessor) extractBookmarkFromBaseObject(v any) (string, time.Time) {
	baseObj, ok := v.(*activitypub.BaseObject)
	if !ok {
		return "", time.Time{}
	}
	
	statusURL := baseObj.ID
	createdAt := time.Now()
	
	if baseObj.Published != nil {
		createdAt = *baseObj.Published
	}
	
	return statusURL, createdAt
}

func (ep *ExportProcessor) getOutbox(ctx context.Context, username string, dateRange *DateRange) ([]any, int, error) {
	// Query user's posts from DynamoDB using storage client
	var allActivities []any
	cursor := ""

	for {
		activities, nextCursor, err := ep.repos.Activity().GetOutboxActivities(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get outbox activities", zap.String("username", username), zap.Error(err))
			return nil, 0, fmt.Errorf("get outbox activities: %w", err)
		}

		// Filter by date range if specified
		for _, activity := range activities {
			if dateRange != nil {
				// Check if activity is within date range
				activityTime := activity.Published
				if activityTime.Before(dateRange.Start) || activityTime.After(dateRange.End) {
					continue
				}
			}

			// Add the activity to results
			allActivities = append(allActivities, activity)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allActivities, len(allActivities), nil
}

func (ep *ExportProcessor) getFollowingActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for following (for ActivityPub export)
	// This returns the raw actor IDs without conversion to handles
	var allFollowing []string
	cursor := ""

	for {
		following, nextCursor, err := ep.repos.Relationship().GetFollowing(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get following actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get following actors: %w", err)
		}

		// Keep raw actor IDs for ActivityPub format
		allFollowing = append(allFollowing, following...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowing, nil
}

func (ep *ExportProcessor) getFollowersActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for followers (for ActivityPub export)
	// This returns the raw actor IDs without conversion to handles
	var allFollowers []string
	cursor := ""

	for {
		followers, nextCursor, err := ep.repos.Relationship().GetFollowers(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get follower actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get follower actors: %w", err)
		}

		// Keep raw actor IDs for ActivityPub format
		allFollowers = append(allFollowers, followers...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowers, nil
}

func (ep *ExportProcessor) getLikes(ctx context.Context, username string) ([]any, error) {
	// Query user's likes from DynamoDB using storage client
	var allLikes []any
	cursor := ""

	// First get the actor ID for the username
	actor, err := ep.repos.Account().GetActor(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get actor: %w", err)
	}

	for {
		likes, nextCursor, err := ep.repos.Like().GetActorLikes(ctx, actor.ID, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get actor likes",
				zap.String("username", username),
				zap.String("actor_id", actor.ID),
				zap.Error(err))
			return nil, fmt.Errorf("get actor likes: %w", err)
		}

		// Convert likes to Like activities
		for _, like := range likes {
			likeActivity := map[string]any{
				"@context":  activitypub.Context,
				"type":      "Like",
				"id":        like.ID,
				"actor":     like.Actor,
				"object":    like.Object,
				"published": like.Published.Format(time.RFC3339),
			}
			allLikes = append(allLikes, likeActivity)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allLikes, nil
}

func (ep *ExportProcessor) getBookmarksForExport(ctx context.Context, username string) ([]any, error) {
	// Query bookmarks and convert to ActivityPub format
	bookmarks, err := ep.getBookmarks(ctx, username)
	if err != nil {
		return nil, err
	}

	// Convert to ActivityPub bookmark activities
	result := make([]any, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		bookmarkActivity := map[string]any{
			"@context":  activitypub.Context,
			"type":      "Add",
			"actor":     fmt.Sprintf("%s/users/%s", ep.baseURL, username),
			"object":    bookmark.StatusURL,
			"target":    fmt.Sprintf("%s/users/%s/bookmarks", ep.baseURL, username),
			"published": bookmark.CreatedAt.Format(time.RFC3339),
		}
		result = append(result, bookmarkActivity)
	}

	return result, nil
}

func (ep *ExportProcessor) getListsForExport(ctx context.Context, username string) ([]any, error) {
	// Query lists and convert to export format
	lists, err := ep.repos.List().GetListsForUser(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	// Convert to export format
	result := make([]any, 0, len(lists))
	for _, list := range lists {
		// Get members for each list
		members, err := ep.repos.List().GetListAccounts(ctx, list.ID)
		if err != nil {
			ep.logger.Warn("failed to get list members",
				zap.String("list_id", list.ID),
				zap.String("list_title", list.Title),
				zap.Error(err))
			members = []string{} // Empty array if error
		}

		listExport := map[string]any{
			"id":             list.ID,
			"title":          list.Title,
			"replies_policy": list.RepliesPolicy,
			"exclusive":      false, // Default value
			"members":        members,
			"created_at":     list.CreatedAt.Format(time.RFC3339),
			"updated_at":     list.UpdatedAt.Format(time.RFC3339),
		}
		result = append(result, listExport)
	}

	return result, nil
}

// Helper functions
func addFileToZip(w *zip.Writer, filename string, data []byte) error {
	f, err := w.Create(filename)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// convertActorIDToHandle converts an actor ID like "https://example.com/users/alice"
// to a Mastodon handle like "@alice@example.com"
func (ep *ExportProcessor) convertActorIDToHandle(actorID string) string {
	// Handle local actors (simple usernames)
	if !strings.Contains(actorID, "://") {
		return actorID // Already in simple format
	}

	// Parse the URL
	parts := strings.Split(actorID, "/")
	if len(parts) < 3 {
		return actorID // Can't parse, return as-is
	}

	// Extract domain from URL (parts[2] is the domain)
	domain := parts[2]

	// Find username - usually the last part after /users/
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "users" && i+1 < len(parts) {
			username := parts[i+1]
			return fmt.Sprintf("@%s@%s", username, domain)
		}
	}

	// If we can't find /users/, try the last part
	if len(parts) > 0 {
		username := parts[len(parts)-1]
		if username != "" {
			return fmt.Sprintf("@%s@%s", username, domain)
		}
	}

	return actorID // Fallback to original
}

func (ep *ExportProcessor) uploadToS3(ctx context.Context, key string, data []byte, contentType string, costTracking *models.ExportCostTracking) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(ep.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := ep.s3Client.PutObject(ctx, input)

	// Track S3 costs
	if costTracking != nil {
		costTracking.S3PutRequests = 1
		costTracking.S3PutRequestCost = ep.calculateS3PutCost(1)
		costTracking.S3StorageCost = ep.calculateS3StorageCost(int64(len(data)))
		costTracking.DataTransferBytes = int64(len(data))
		costTracking.S3DataTransferCost = ep.calculateS3DataTransferCost(int64(len(data)))
	}

	return err
}

// updateExportStatus is deprecated - use ep.exportRepo.UpdateExportStatus instead

func (ep *ExportProcessor) getDomainBlocks(ctx context.Context, username string) ([]string, error) {
	// Query domain blocks from DynamoDB using storage client
	var allDomainBlocks []string
	cursor := ""

	for {
		domains, nextCursor, err := ep.repos.DomainBlock().GetUserDomainBlocks(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get domain blocks", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get domain blocks: %w", err)
		}

		allDomainBlocks = append(allDomainBlocks, domains...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allDomainBlocks, nil
}

// includeMediaFiles downloads and includes user's media files in the export ZIP
func (ep *ExportProcessor) includeMediaFiles(ctx context.Context, zipWriter *zip.Writer, username string, dateRange *DateRange) (int, error) {
	// Note: This function now tracks S3 GET costs for media file downloads
	// The costs will be accumulated in the calling function's cost tracking
	ep.logger.Info("including media files in export",
		zap.String("username", username),
		zap.Bool("has_date_range", dateRange != nil))

	// Get user's media files
	userMedia, err := ep.fetchUserMedia(ctx, username)
	if err != nil {
		return 0, err
	}

	ep.logger.Info("found user media files",
		zap.String("username", username),
		zap.Int("total_count", len(userMedia)))

	// Collect all media keys to download
	allMediaKeys := ep.collectMediaKeys(userMedia, dateRange)

	ep.logger.Info("downloading media files",
		zap.String("username", username),
		zap.Int("files_to_download", len(allMediaKeys)))

	// Download and add media files to ZIP
	totalDownloaded := ep.downloadMediaFilesToZip(ctx, zipWriter, allMediaKeys)

	ep.logger.Info("completed media files export",
		zap.String("username", username),
		zap.Int("files_downloaded", totalDownloaded),
		zap.Int("files_requested", len(allMediaKeys)))

	return totalDownloaded, nil
}

// fetchUserMedia retrieves and converts user media from the repository
func (ep *ExportProcessor) fetchUserMedia(ctx context.Context, username string) ([]map[string]any, error) {
	userMediaAny, err := ep.repos.Media().GetUserMedia(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get user media",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("get user media: %w", err)
	}

	// Convert to proper media type
	userMedia := make([]map[string]any, 0, len(userMediaAny))
	for _, mediaAny := range userMediaAny {
		if mediaMap, ok := mediaAny.(map[string]any); ok {
			userMedia = append(userMedia, mediaMap)
		}
	}

	return userMedia, nil
}

// collectMediaKeys collects all S3 keys from media items within the date range
func (ep *ExportProcessor) collectMediaKeys(userMedia []map[string]any, dateRange *DateRange) []string {
	var allMediaKeys []string

	for _, mediaItem := range userMedia {
		// Check if media is within date range
		if !ep.isMediaInDateRange(mediaItem, dateRange) {
			continue
		}

		// Extract main S3 key
		if s3Key := ep.extractS3Key(mediaItem); s3Key != "" {
			allMediaKeys = append(allMediaKeys, s3Key)
		}

		// Extract variant S3 keys
		variantKeys := ep.extractVariantKeys(mediaItem)
		allMediaKeys = append(allMediaKeys, variantKeys...)
	}

	return allMediaKeys
}

// isMediaInDateRange checks if a media item falls within the specified date range
func (ep *ExportProcessor) isMediaInDateRange(mediaItem map[string]any, dateRange *DateRange) bool {
	if dateRange == nil {
		return true
	}

	createdAtAny, exists := mediaItem["CreatedAt"]
	if !exists {
		return true
	}

	createdAtStr, ok := createdAtAny.(string)
	if !ok {
		return true
	}

	mediaDate, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return true
	}

	return !mediaDate.Before(dateRange.Start) && !mediaDate.After(dateRange.End)
}

// extractS3Key extracts the main S3 key from a media item
func (ep *ExportProcessor) extractS3Key(mediaItem map[string]any) string {
	s3KeyAny, exists := mediaItem["S3Key"]
	if !exists {
		return ""
	}

	s3Key, ok := s3KeyAny.(string)
	if !ok {
		return ""
	}

	return s3Key
}

// extractVariantKeys extracts all variant S3 keys from a media item
func (ep *ExportProcessor) extractVariantKeys(mediaItem map[string]any) []string {
	var keys []string

	variantsAny, exists := mediaItem["Variants"]
	if !exists {
		return keys
	}

	variants, ok := variantsAny.(map[string]any)
	if !ok {
		return keys
	}

	for variantName, variantAny := range variants {
		variant, ok := variantAny.(map[string]any)
		if !ok {
			continue
		}

		variantS3Key, exists := variant["S3Key"]
		if !exists {
			continue
		}

		s3Key, ok := variantS3Key.(string)
		if !ok || s3Key == "" {
			continue
		}

		keys = append(keys, s3Key)
		ep.logger.Debug("added variant to export",
			zap.String("variant_name", variantName),
			zap.String("s3_key", s3Key))
	}

	return keys
}

// downloadMediaFilesToZip downloads media files from S3 and adds them to the ZIP
func (ep *ExportProcessor) downloadMediaFilesToZip(ctx context.Context, zipWriter *zip.Writer, allMediaKeys []string) int {
	var totalDownloaded int

	for _, s3Key := range allMediaKeys {
		if ep.downloadSingleMediaFile(ctx, zipWriter, s3Key) {
			totalDownloaded++
		}
	}

	return totalDownloaded
}

// downloadSingleMediaFile downloads a single media file and adds it to the ZIP
func (ep *ExportProcessor) downloadSingleMediaFile(ctx context.Context, zipWriter *zip.Writer, s3Key string) bool {
	// Download file from S3
	result, err := ep.downloadFromS3(ctx, s3Key)
	if err != nil {
		ep.logger.Warn("failed to download media file",
			zap.String("s3_key", s3Key),
			zap.Error(err))
		return false
	}
	defer func() { _ = result.Body.Close() }()

	// Add to ZIP
	fileName := fmt.Sprintf("media/%s", strings.TrimPrefix(s3Key, "media/"))
	if err := ep.addFileToZip(zipWriter, fileName, result.Body); err != nil {
		ep.logger.Warn("failed to add media to ZIP",
			zap.String("s3_key", s3Key),
			zap.String("file_name", fileName),
			zap.Error(err))
		return false
	}

	ep.logger.Debug("added media file to export",
		zap.String("s3_key", s3Key),
		zap.String("zip_path", fileName))

	return true
}

// downloadFromS3 downloads a file from S3
func (ep *ExportProcessor) downloadFromS3(ctx context.Context, s3Key string) (*s3.GetObjectOutput, error) {
	getObjectInput := &s3.GetObjectInput{
		Bucket: &ep.bucketName,
		Key:    &s3Key,
	}

	return ep.s3Client.GetObject(ctx, getObjectInput)
}

// addFileToZip adds a file to the ZIP archive
func (ep *ExportProcessor) addFileToZip(zipWriter *zip.Writer, fileName string, content io.Reader) error {
	zipFile, err := zipWriter.Create(fileName)
	if err != nil {
		return fmt.Errorf("create ZIP entry: %w", err)
	}

	if _, err := io.Copy(zipFile, content); err != nil {
		return fmt.Errorf("copy to ZIP: %w", err)
	}

	return nil
}

// Cost calculation helper functions

// calculateLambdaCost calculates the cost of Lambda execution
// Lambda pricing: $0.0000166667 per GB-second (assumes 512MB memory)
func (ep *ExportProcessor) calculateLambdaCost(durationMs int64) int64 {
	const memoryGB = 0.5                 // 512MB = 0.5GB
	const costPerGBSecond = 0.0000166667 // USD per GB-second

	durationSeconds := float64(durationMs) / 1000.0
	costDollars := memoryGB * durationSeconds * costPerGBSecond

	// Convert to microcents (1 dollar = 1,000,000 microcents)
	return int64(costDollars * 1_000_000)
}

// calculateS3PutCost calculates the cost of S3 PUT requests
// S3 PUT pricing: $0.005 per 1,000 requests
func (ep *ExportProcessor) calculateS3PutCost(requestCount int64) int64 {
	const costPer1000Requests = 0.005 // USD per 1,000 requests

	costDollars := float64(requestCount) * costPer1000Requests / 1000.0

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateS3GetCost calculates the cost of S3 GET requests
// S3 GET pricing: $0.0004 per 1,000 requests
func (ep *ExportProcessor) calculateS3GetCost(requestCount int64) int64 {
	const costPer1000Requests = 0.0004 // USD per 1,000 requests

	costDollars := float64(requestCount) * costPer1000Requests / 1000.0

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateS3StorageCost calculates the cost of S3 storage
// S3 storage pricing: $0.023 per GB per month (prorated)
func (ep *ExportProcessor) calculateS3StorageCost(sizeBytes int64) int64 {
	const costPerGBMonth = 0.023 // USD per GB per month

	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024) // Convert bytes to GB

	// Assume storage for 30 days (1 month)
	costDollars := sizeGB * costPerGBMonth

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateS3DataTransferCost calculates the cost of S3 data transfer
// S3 data transfer pricing: $0.09 per GB (outbound)
func (ep *ExportProcessor) calculateS3DataTransferCost(transferBytes int64) int64 {
	const costPerGB = 0.09 // USD per GB

	transferGB := float64(transferBytes) / (1024 * 1024 * 1024) // Convert bytes to GB
	costDollars := transferGB * costPerGB

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}

// calculateDynamoDBReadCost calculates the cost of DynamoDB read operations
// DynamoDB pricing: $0.25 per million read requests
func (ep *ExportProcessor) calculateDynamoDBReadCost(readUnits float64) int64 {
	const costPerMillionReads = 0.25 // USD per million read requests

	costDollars := (readUnits / 1_000_000) * costPerMillionReads

	// Convert to microcents
	return int64(costDollars * 1_000_000)
}
