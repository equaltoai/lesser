package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
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
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// ExportProcessor handles data export generation from SQS messages
type ExportProcessor struct {
	db           core.DB
	storageAdapter *dynamorm.StorageAdapter
	exportRepo   *repositories.ExportRepository
	s3Client     *s3.Client
	logger       *zap.Logger
	tableName    string
	bucketName   string
	baseURL      string
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

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize storage adapter (for compatibility with existing storage interface)
	storageAdapter := dynamorm.NewStorageAdapter(db, cfg.DynamoTableName, logger, nil)

	// Initialize export repository
	exportRepo := repositories.NewExportRepository(db, cfg.DynamoTableName, logger)

	// Create processor instance
	processor = &ExportProcessor{
		db:           db,
		storageAdapter: storageAdapter,
		exportRepo:   exportRepo,
		logger:       logger,
		tableName:    cfg.DynamoTableName,
		bucketName:   cfg.S3BucketName,
		baseURL:      cfg.BaseURL(),
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
			ep.exportRepo.UpdateExportStatus(ctx.Request.Context(), exportEvent.ExportID, "failed", nil, err.Error())
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
		exportData, recordCount, err = ep.generateCSVExport(ctx, event)
		filename = fmt.Sprintf("%s_%s.csv", event.Username, event.Type)
		contentType = "text/csv"

	case "activitypub":
		exportData, recordCount, err = ep.generateActivityPubExport(ctx, event)
		filename = fmt.Sprintf("%s_activitypub_archive.zip", event.Username)
		contentType = "application/zip"

	case "mastodon":
		exportData, recordCount, err = ep.generateMastodonExport(ctx, event)
		filename = fmt.Sprintf("%s_mastodon_archive.zip", event.Username)
		contentType = "application/zip"

	default:
		return fmt.Errorf("unsupported export format: %s", event.Format)
	}

	if err != nil {
		return fmt.Errorf("failed to generate export: %w", err)
	}

	// Upload to S3
	s3Key := fmt.Sprintf("exports/%s/%s/%s", event.Username, event.ExportID, filename)
	if err := ep.uploadToS3(ctx, s3Key, exportData, contentType); err != nil {
		return fmt.Errorf("failed to upload export: %w", err)
	}

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

func (ep *ExportProcessor) generateCSVExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	recordCount := 0

	switch event.Type {
	case "followers":
		// Write header
		writer.Write([]string{"Account address", "Show boosts", "Notify on new posts", "Languages"})

		// Get followers
		followers, err := ep.getFollowers(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, follower := range followers {
			writer.Write([]string{
				follower,
				"true",  // Show boosts default
				"false", // Notify default
				"",      // Languages default
			})
			recordCount++
		}

	case "following":
		// Write header
		writer.Write([]string{"Account address", "Show boosts", "Notify on new posts", "Languages"})

		// Get following
		following, err := ep.getFollowing(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, follow := range following {
			writer.Write([]string{
				follow,
				"true",  // Show boosts default
				"false", // Notify default
				"",      // Languages default
			})
			recordCount++
		}

	case "blocks":
		// Write header
		writer.Write([]string{"Account address"})

		// Get blocks
		blocks, err := ep.getBlocks(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, block := range blocks {
			writer.Write([]string{block})
			recordCount++
		}

	case "mutes":
		// Write header
		writer.Write([]string{"Account address", "Hide notifications"})

		// Get mutes
		mutes, err := ep.getMutes(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, mute := range mutes {
			writer.Write([]string{
				mute.AccountID,
				fmt.Sprintf("%t", mute.HideNotifications),
			})
			recordCount++
		}

	case "lists":
		// Write header
		writer.Write([]string{"List name", "Account address"})

		// Get lists with members
		lists, err := ep.getListsWithMembers(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for listName, members := range lists {
			for _, member := range members {
				writer.Write([]string{listName, member})
				recordCount++
			}
		}

	case "bookmarks":
		// Write header
		writer.Write([]string{"Status URL", "Bookmarked at"})

		// Get bookmarks
		bookmarks, err := ep.getBookmarks(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, bookmark := range bookmarks {
			writer.Write([]string{
				bookmark.StatusURL,
				bookmark.CreatedAt.Format(time.RFC3339),
			})
			recordCount++
		}

	case "domain_blocks":
		// Write header
		writer.Write([]string{"Domain"})

		// Get domain blocks
		domainBlocks, err := ep.getDomainBlocks(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		for _, domain := range domainBlocks {
			writer.Write([]string{domain})
			recordCount++
		}

	default:
		return nil, 0, fmt.Errorf("CSV export not supported for type: %s", event.Type)
	}

	writer.Flush()
	return buf.Bytes(), recordCount, nil
}

func (ep *ExportProcessor) generateActivityPubExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
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
			}
		}
	}

	zipWriter.Close()
	return buf.Bytes(), recordCount, nil
}

func (ep *ExportProcessor) generateMastodonExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
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
			}
		}
	}

	zipWriter.Close()
	return buf.Bytes(), recordCount, nil
}

// Helper functions for data retrieval
func (ep *ExportProcessor) getActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Get actor using storage client (already migrated to DynamORM)
	actor, err := ep.storageAdapter.GetActor(ctx, username)
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
		followers, nextCursor, err := ep.storageAdapter.GetFollowers(ctx, username, 1000, cursor)
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
		following, nextCursor, err := ep.storageAdapter.GetFollowing(ctx, username, 1000, cursor)
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
		blocks, nextCursor, err := ep.storageAdapter.GetBlockedActors(ctx, username, 1000, cursor)
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

type MuteInfo struct {
	AccountID         string
	HideNotifications bool
}

func (ep *ExportProcessor) getMutes(ctx context.Context, username string) ([]MuteInfo, error) {
	// Query mutes from DynamoDB using storage client
	var allMutes []MuteInfo
	cursor := ""

	for {
		mutes, nextCursor, err := ep.storageAdapter.GetMutedActors(ctx, username, 1000, cursor)
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
	lists, err := ep.storageAdapter.GetListsForUser(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	result := make(map[string][]string)

	for _, list := range lists {
		members, err := ep.storageAdapter.GetListAccounts(ctx, list.ID)
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

type BookmarkInfo struct {
	StatusURL string
	CreatedAt time.Time
}

func (ep *ExportProcessor) getBookmarks(ctx context.Context, username string) ([]BookmarkInfo, error) {
	// Query bookmarks from DynamoDB using storage client
	var allBookmarks []BookmarkInfo
	cursor := ""

	for {
		bookmarkIDs, nextCursor, err := ep.storageAdapter.GetBookmarks(ctx, username, 1000, cursor)
		if err != nil {
			ep.logger.Error("failed to get bookmarks", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get bookmarks: %w", err)
		}

		// Convert bookmark IDs to BookmarkInfo
		// Note: We need to get the actual status objects to get their URLs
		for _, bookmarkID := range bookmarkIDs {
			obj, err := ep.storageAdapter.GetObject(ctx, bookmarkID)
			if err != nil {
				ep.logger.Warn("failed to get bookmarked object",
					zap.String("bookmark_id", bookmarkID),
					zap.Error(err))
				continue
			}

			// Extract URL from the object
			var statusURL string
			var createdAt time.Time

			// Handle different object types
			switch v := obj.(type) {
			case map[string]any:
				if url, ok := v["url"].(string); ok {
					statusURL = url
				} else if id, ok := v["id"].(string); ok {
					statusURL = id // Fallback to ID if no URL
				}
				if published, ok := v["published"].(string); ok {
					createdAt, _ = time.Parse(time.RFC3339, published)
				}
			case *activitypub.Note:
				// Notes have ID from BaseObject
				statusURL = v.ID
				if v.Published != nil {
					createdAt = *v.Published
				} else {
					createdAt = time.Now()
				}
			case *activitypub.Article:
				// Articles have ID from BaseObject
				statusURL = v.ID
				if v.Published != nil {
					createdAt = *v.Published
				} else {
					createdAt = time.Now()
				}
			default:
				// For any other type, try to extract ID
				if baseObj, ok := v.(*activitypub.BaseObject); ok {
					statusURL = baseObj.ID
					if baseObj.Published != nil {
						createdAt = *baseObj.Published
					} else {
						createdAt = time.Now()
					}
				}
			}

			if statusURL != "" {
				allBookmarks = append(allBookmarks, BookmarkInfo{
					StatusURL: statusURL,
					CreatedAt: createdAt,
				})
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allBookmarks, nil
}

func (ep *ExportProcessor) getOutbox(ctx context.Context, username string, dateRange *DateRange) ([]any, int, error) {
	// Query user's posts from DynamoDB using storage client
	var allActivities []any
	cursor := ""

	for {
		activities, nextCursor, err := ep.storageAdapter.GetOutboxActivities(ctx, username, 1000, cursor)
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
		following, nextCursor, err := ep.storageAdapter.GetFollowing(ctx, username, 1000, cursor)
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
		followers, nextCursor, err := ep.storageAdapter.GetFollowers(ctx, username, 1000, cursor)
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
	actor, err := ep.storageAdapter.GetActor(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get actor: %w", err)
	}

	for {
		likes, nextCursor, err := ep.storageAdapter.GetActorLikes(ctx, actor.ID, 1000, cursor)
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
	var result []any
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
	lists, err := ep.storageAdapter.GetListsForUser(ctx, username)
	if err != nil {
		ep.logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	// Convert to export format
	var result []any
	for _, list := range lists {
		// Get members for each list
		members, err := ep.storageAdapter.GetListAccounts(ctx, list.ID)
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

func (ep *ExportProcessor) uploadToS3(ctx context.Context, key string, data []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(ep.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := ep.s3Client.PutObject(ctx, input)
	return err
}

// updateExportStatus is deprecated - use ep.exportRepo.UpdateExportStatus instead

func (ep *ExportProcessor) getDomainBlocks(ctx context.Context, username string) ([]string, error) {
	// Query domain blocks from DynamoDB using storage client
	var allDomainBlocks []string
	cursor := ""

	for {
		domains, nextCursor, err := ep.storageAdapter.GetUserDomainBlocks(ctx, username, 1000, cursor)
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
	// Query user's media files from DynamoDB
	// Note: Media files export temporarily disabled during migration

	for {
		// Query media files for the user
		// Note: This would need to be migrated to use DynamORM patterns
		// For now, we'll skip media files in the export
		ep.logger.Warn("media files not included in export - migration needed")
		return 0, nil
		/*
		queryInput := &dynamodbsdk.QueryInput{
			TableName:              aws.String(ep.tableName),
			IndexName:              aws.String("GSI1"), // Assuming GSI1 is used for media queries
			KeyConditionExpression: aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("MEDIA_USER#%s", username)},
			},
			Limit: aws.Int32(100),
		}

		if cursor != "" {
			// Add cursor for pagination
			queryInput.ExclusiveStartKey = map[string]types.AttributeValue{
				"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MEDIA_USER#%s", username)},
				"GSI1SK": &types.AttributeValueMemberS{Value: cursor},
			}
		}

		result, err := dynamoClient.Query(ctx, queryInput)
		if err != nil {
			ep.logger.Error("failed to query media files", zap.String("username", username), zap.Error(err))
			return 0, fmt.Errorf("query media files: %w", err)
		}

		// Process each media item
		for _, item := range result.Items {
			// Extract S3 key from the media item
			if s3KeyAttr, exists := item["S3Key"]; exists {
				if s3Key, ok := s3KeyAttr.(*types.AttributeValueMemberS); ok {
					// Check date range if specified
					if dateRange != nil {
						if createdAtAttr, exists := item["CreatedAt"]; exists {
							if createdAt, ok := createdAtAttr.(*types.AttributeValueMemberS); ok {
								mediaDate, err := time.Parse(time.RFC3339, createdAt.Value)
								if err == nil {
									if mediaDate.Before(dateRange.Start) || mediaDate.After(dateRange.End) {
										continue // Skip media outside date range
									}
								}
							}
						}
					}

					allMediaKeys = append(allMediaKeys, s3Key.Value)
				}
			}
		}

		// Check for more results
		if result.LastEvaluatedKey == nil {
			break
		}

		// Extract cursor for next iteration
		if cursorAttr, exists := result.LastEvaluatedKey["GSI1SK"]; exists {
			if cursorVal, ok := cursorAttr.(*types.AttributeValueMemberS); ok {
				cursor = cursorVal.Value
			} else {
				break
			}
		} else {
			break
		}
		*/
	}

	// Media files export is not implemented in this migration
	// This would require implementing proper DynamORM queries for media records
	return 0, nil
}
