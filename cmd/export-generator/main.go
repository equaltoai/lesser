package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	dynamodbsdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger        *zap.Logger
	s3Client      *s3.Client
	dynamoClient  *dynamodbsdk.Client
	storageClient storage.Storage
	tableName     string
	bucketName    string
	baseURL       string
)

// ExportGeneratorEvent represents the event triggered for export generation
type ExportGeneratorEvent struct {
	ExportID     string                 `json:"export_id"`
	Username     string                 `json:"username"`
	Type         string                 `json:"type"`   // archive, followers, following, etc.
	Format       string                 `json:"format"` // activitypub, mastodon, csv
	Options      map[string]interface{} `json:"options"`
	IncludeMedia bool                   `json:"include_media"`
	DateRange    *DateRange             `json:"date_range"`
}

// DateRange for filtering exports
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func init() {
	// Initialize logger
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	logger, err = cfg.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	// Get configuration from environment
	bucketName = os.Getenv("S3_BUCKET_NAME")
	if bucketName == "" {
		logger.Fatal("S3_BUCKET_NAME environment variable not set")
	}

	tableName = os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE_NAME environment variable not set")
	}

	baseURL = os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://example.com" // Default
	}
}

func main() {
	lambda.Start(handleExportGeneration)
}

func handleExportGeneration(ctx context.Context, sqsEvent events.SQSEvent) error {
	// Initialize AWS clients
	if err := initializeAWSClients(ctx); err != nil {
		logger.Error("failed to initialize AWS clients", zap.Error(err))
		return err
	}

	// Process each message
	for _, message := range sqsEvent.Records {
		var event ExportGeneratorEvent
		if err := common.ParseRequestBody([]byte(message.Body), &event); err != nil {
			logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.Error(err))
			continue
		}

		if err := processExportJob(ctx, event); err != nil {
			logger.Error("failed to process export job",
				zap.String("export_id", event.ExportID),
				zap.String("username", event.Username),
				zap.Error(err))
			// Update job status as failed
			updateExportStatus(ctx, event.ExportID, "failed", nil, err.Error())
		}
	}

	return nil
}

func initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize S3 client
	s3Client = s3.NewFromConfig(cfg)

	// Initialize DynamoDB client
	dynamoClient = dynamodbsdk.NewFromConfig(cfg)

	// Initialize storage client
	storageClient = dynamodb.NewWithClient(dynamoClient, tableName)

	return nil
}

func processExportJob(ctx context.Context, event ExportGeneratorEvent) error {
	logger.Info("processing export job",
		zap.String("export_id", event.ExportID),
		zap.String("username", event.Username),
		zap.String("type", event.Type),
		zap.String("format", event.Format))

	// Update job status to processing
	if err := updateExportStatus(ctx, event.ExportID, "processing", nil, ""); err != nil {
		logger.Warn("failed to update export status", zap.Error(err))
	}

	// Generate export based on type and format
	var exportData []byte
	var filename string
	var contentType string
	var recordCount int
	var err error

	switch event.Format {
	case "csv":
		exportData, recordCount, err = generateCSVExport(ctx, event)
		filename = fmt.Sprintf("%s_%s.csv", event.Username, event.Type)
		contentType = "text/csv"

	case "activitypub":
		exportData, recordCount, err = generateActivityPubExport(ctx, event)
		filename = fmt.Sprintf("%s_activitypub_archive.zip", event.Username)
		contentType = "application/zip"

	case "mastodon":
		exportData, recordCount, err = generateMastodonExport(ctx, event)
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
	if err := uploadToS3(ctx, s3Key, exportData, contentType); err != nil {
		return fmt.Errorf("failed to upload export: %w", err)
	}

	// Generate pre-signed URL (24 hour expiry)
	presignClient := s3.NewPresignClient(s3Client)
	presignReq, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3Key),
	}, s3.WithPresignExpires(24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to generate pre-signed URL: %w", err)
	}

	// Update export job as completed
	completionData := map[string]interface{}{
		"download_url": presignReq.URL,
		"expires_at":   time.Now().Add(24 * time.Hour),
		"file_size":    len(exportData),
		"record_count": recordCount,
		"s3_key":       s3Key,
	}

	if err := updateExportStatus(ctx, event.ExportID, "completed", completionData, ""); err != nil {
		return fmt.Errorf("failed to update export status: %w", err)
	}

	logger.Info("export completed",
		zap.String("export_id", event.ExportID),
		zap.String("s3_key", s3Key),
		zap.Int("file_size", len(exportData)),
		zap.Int("record_count", recordCount))

	return nil
}

func generateCSVExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	recordCount := 0

	switch event.Type {
	case "followers":
		// Write header
		writer.Write([]string{"Account address", "Show boosts", "Notify on new posts", "Languages"})

		// Get followers
		followers, err := getFollowers(ctx, event.Username)
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
		following, err := getFollowing(ctx, event.Username)
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
		blocks, err := getBlocks(ctx, event.Username)
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
		mutes, err := getMutes(ctx, event.Username)
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
		lists, err := getListsWithMembers(ctx, event.Username)
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
		bookmarks, err := getBookmarks(ctx, event.Username)
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
		domainBlocks, err := getDomainBlocks(ctx, event.Username)
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

func generateActivityPubExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
	// Create ZIP archive
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	recordCount := 0

	// Get actor data
	actor, err := getActor(ctx, event.Username)
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
		outbox, count, err := getOutbox(ctx, event.Username, event.DateRange)
		if err != nil {
			return nil, 0, err
		}
		recordCount += count

		outboxJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/outbox", baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   count,
			"orderedItems": outbox,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "outbox.json", outboxJSON); err != nil {
			return nil, 0, err
		}

		// Following collection
		following, err := getFollowingActors(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		followingJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/following", baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(following),
			"orderedItems": following,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "following.json", followingJSON); err != nil {
			return nil, 0, err
		}

		// Followers collection
		followers, err := getFollowersActors(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		followersJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/followers", baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(followers),
			"orderedItems": followers,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "followers.json", followersJSON); err != nil {
			return nil, 0, err
		}

		// Likes collection
		likes, err := getLikes(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		likesJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/likes", baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   len(likes),
			"orderedItems": likes,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "likes.json", likesJSON); err != nil {
			return nil, 0, err
		}

		// TODO: Add media files if IncludeMedia is true
	}

	zipWriter.Close()
	return buf.Bytes(), recordCount, nil
}

func generateMastodonExport(ctx context.Context, event ExportGeneratorEvent) ([]byte, int, error) {
	// Create ZIP archive
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	recordCount := 0

	// Get actor data and convert to Mastodon format
	actor, err := getActor(ctx, event.Username)
	if err != nil {
		return nil, 0, err
	}

	// Create Mastodon-compatible actor.json
	mastodonActor := map[string]interface{}{
		"@context": []interface{}{
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
		outbox, count, err := getOutbox(ctx, event.Username, event.DateRange)
		if err != nil {
			return nil, 0, err
		}
		recordCount += count

		outboxJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"id":           fmt.Sprintf("%s/users/%s/outbox", baseURL, event.Username),
			"type":         "OrderedCollection",
			"totalItems":   count,
			"orderedItems": outbox,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "outbox.json", outboxJSON); err != nil {
			return nil, 0, err
		}

		// Create likes.json
		likes, err := getLikes(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		likesJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"type":         "OrderedCollection",
			"orderedItems": likes,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "likes.json", likesJSON); err != nil {
			return nil, 0, err
		}

		// Create bookmarks.json
		bookmarks, err := getBookmarksForExport(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		bookmarksJSON, _ := json.MarshalIndent(map[string]interface{}{
			"@context":     activitypub.Context,
			"type":         "OrderedCollection",
			"orderedItems": bookmarks,
		}, "", "  ")
		if err := addFileToZip(zipWriter, "bookmarks.json", bookmarksJSON); err != nil {
			return nil, 0, err
		}

		// Create lists.json
		lists, err := getListsForExport(ctx, event.Username)
		if err != nil {
			return nil, 0, err
		}

		listsJSON, _ := json.MarshalIndent(lists, "", "  ")
		if err := addFileToZip(zipWriter, "lists.json", listsJSON); err != nil {
			return nil, 0, err
		}

		// TODO: Add media_attachments directory if IncludeMedia is true
	}

	zipWriter.Close()
	return buf.Bytes(), recordCount, nil
}

// Helper functions for data retrieval
func getActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Get actor from DynamoDB
	result, err := dynamoClient.GetItem(ctx, &dynamodbsdk.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", username)},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("actor not found: %s", username)
	}

	// Convert to Actor struct
	// This is a simplified version - in production would use proper unmarshaling
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    "Person",
			ID:      fmt.Sprintf("%s/users/%s", baseURL, username),
		},
		PreferredUsername: username,
		Inbox:             fmt.Sprintf("%s/users/%s/inbox", baseURL, username),
		Outbox:            fmt.Sprintf("%s/users/%s/outbox", baseURL, username),
		Followers:         fmt.Sprintf("%s/users/%s/followers", baseURL, username),
		Following:         fmt.Sprintf("%s/users/%s/following", baseURL, username),
	}

	// TODO: Populate other fields from DynamoDB result using result.Item

	return actor, nil
}

func getFollowers(ctx context.Context, username string) ([]string, error) {
	// Query followers from DynamoDB using storage client
	var allFollowers []string
	cursor := ""

	for {
		followers, nextCursor, err := storageClient.GetFollowers(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get followers", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get followers: %w", err)
		}

		// Convert actor IDs to Mastodon handles for CSV export
		for _, follower := range followers {
			handle := convertActorIDToHandle(follower)
			allFollowers = append(allFollowers, handle)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowers, nil
}

func getFollowing(ctx context.Context, username string) ([]string, error) {
	// Query following from DynamoDB using storage client
	var allFollowing []string
	cursor := ""

	for {
		following, nextCursor, err := storageClient.GetFollowing(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get following", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get following: %w", err)
		}

		// Convert actor IDs to Mastodon handles for CSV export
		for _, follow := range following {
			handle := convertActorIDToHandle(follow)
			allFollowing = append(allFollowing, handle)
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allFollowing, nil
}

func getBlocks(ctx context.Context, username string) ([]string, error) {
	// Query blocks from DynamoDB using storage client
	var allBlocks []string
	cursor := ""

	for {
		blocks, nextCursor, err := storageClient.GetBlockedActors(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get blocked actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get blocked actors: %w", err)
		}

		for _, block := range blocks {
			handle := convertActorIDToHandle(block.Object)
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

func getMutes(ctx context.Context, username string) ([]MuteInfo, error) {
	// Query mutes from DynamoDB using storage client
	var allMutes []MuteInfo
	cursor := ""

	for {
		mutes, nextCursor, err := storageClient.GetMutedActors(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get muted actors", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get muted actors: %w", err)
		}

		for _, mute := range mutes {
			allMutes = append(allMutes, MuteInfo{
				AccountID:         convertActorIDToHandle(mute.Object),
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

func getListsWithMembers(ctx context.Context, username string) (map[string][]string, error) {
	// Query lists and their members from DynamoDB using storage client
	lists, err := storageClient.GetListsForUser(ctx, username)
	if err != nil {
		logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	result := make(map[string][]string)

	for _, list := range lists {
		members, err := storageClient.GetListAccounts(ctx, list.ID)
		if err != nil {
			logger.Error("failed to get list members",
				zap.String("list_id", list.ID),
				zap.String("list_title", list.Title),
				zap.Error(err))
			// Continue with other lists even if one fails
			continue
		}

		// Convert member IDs to Mastodon handles
		var handleMembers []string
		for _, member := range members {
			handleMembers = append(handleMembers, convertActorIDToHandle(member))
		}

		result[list.Title] = handleMembers
	}

	return result, nil
}

type BookmarkInfo struct {
	StatusURL string
	CreatedAt time.Time
}

func getBookmarks(ctx context.Context, username string) ([]BookmarkInfo, error) {
	// Query bookmarks from DynamoDB using storage client
	var allBookmarks []BookmarkInfo
	cursor := ""

	for {
		bookmarkIDs, nextCursor, err := storageClient.GetBookmarks(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get bookmarks", zap.String("username", username), zap.Error(err))
			return nil, fmt.Errorf("get bookmarks: %w", err)
		}

		// Convert bookmark IDs to BookmarkInfo
		// Note: We need to get the actual status objects to get their URLs
		for _, bookmarkID := range bookmarkIDs {
			obj, err := storageClient.GetObject(ctx, bookmarkID)
			if err != nil {
				logger.Warn("failed to get bookmarked object",
					zap.String("bookmark_id", bookmarkID),
					zap.Error(err))
				continue
			}

			// Extract URL from the object
			var statusURL string
			var createdAt time.Time

			// Handle different object types
			switch v := obj.(type) {
			case map[string]interface{}:
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

func getOutbox(ctx context.Context, username string, dateRange *DateRange) ([]interface{}, int, error) {
	// Query user's posts from DynamoDB using storage client
	var allActivities []interface{}
	cursor := ""

	for {
		activities, nextCursor, err := storageClient.GetOutboxActivities(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get outbox activities", zap.String("username", username), zap.Error(err))
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

func getFollowingActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for following (for ActivityPub export)
	// This returns the raw actor IDs without conversion to handles
	var allFollowing []string
	cursor := ""

	for {
		following, nextCursor, err := storageClient.GetFollowing(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get following actors", zap.String("username", username), zap.Error(err))
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

func getFollowersActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for followers (for ActivityPub export)
	// This returns the raw actor IDs without conversion to handles
	var allFollowers []string
	cursor := ""

	for {
		followers, nextCursor, err := storageClient.GetFollowers(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get follower actors", zap.String("username", username), zap.Error(err))
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

func getLikes(ctx context.Context, username string) ([]interface{}, error) {
	// Query user's likes from DynamoDB using storage client
	var allLikes []interface{}
	cursor := ""

	// First get the actor ID for the username
	actor, err := storageClient.GetActor(ctx, username)
	if err != nil {
		logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get actor: %w", err)
	}

	for {
		likes, nextCursor, err := storageClient.GetActorLikes(ctx, actor.ID, 1000, cursor)
		if err != nil {
			logger.Error("failed to get actor likes",
				zap.String("username", username),
				zap.String("actor_id", actor.ID),
				zap.Error(err))
			return nil, fmt.Errorf("get actor likes: %w", err)
		}

		// Convert likes to Like activities
		for _, like := range likes {
			likeActivity := map[string]interface{}{
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

func getBookmarksForExport(ctx context.Context, username string) ([]interface{}, error) {
	// Query bookmarks and convert to ActivityPub format
	bookmarks, err := getBookmarks(ctx, username)
	if err != nil {
		return nil, err
	}

	// Convert to ActivityPub bookmark activities
	var result []interface{}
	for _, bookmark := range bookmarks {
		bookmarkActivity := map[string]interface{}{
			"@context":  activitypub.Context,
			"type":      "Add",
			"actor":     fmt.Sprintf("%s/users/%s", baseURL, username),
			"object":    bookmark.StatusURL,
			"target":    fmt.Sprintf("%s/users/%s/bookmarks", baseURL, username),
			"published": bookmark.CreatedAt.Format(time.RFC3339),
		}
		result = append(result, bookmarkActivity)
	}

	return result, nil
}

func getListsForExport(ctx context.Context, username string) ([]interface{}, error) {
	// Query lists and convert to export format
	lists, err := storageClient.GetListsForUser(ctx, username)
	if err != nil {
		logger.Error("failed to get lists", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("get lists: %w", err)
	}

	// Convert to export format
	var result []interface{}
	for _, list := range lists {
		// Get members for each list
		members, err := storageClient.GetListAccounts(ctx, list.ID)
		if err != nil {
			logger.Warn("failed to get list members",
				zap.String("list_id", list.ID),
				zap.String("list_title", list.Title),
				zap.Error(err))
			members = []string{} // Empty array if error
		}

		listExport := map[string]interface{}{
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

func getActorPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	// Get user preferences from storage
	prefs, err := storageClient.GetUserPreferences(ctx, username)
	if err != nil {
		logger.Error("failed to get user preferences", zap.String("username", username), zap.Error(err))
		// Return default preferences if not found
		return map[string]interface{}{
			"posting:default:visibility": "public",
			"posting:default:sensitive":  false,
			"posting:default:language":   "en",
			"reading:expand:media":       "default",
			"reading:expand:spoilers":    false,
			"reading:autoplay:gifs":      true,
		}, nil
	}

	// Convert to Mastodon API format
	return map[string]interface{}{
		"posting:default:visibility": prefs.DefaultPostingVisibility,
		"posting:default:sensitive":  prefs.DefaultMediaSensitive,
		"posting:default:language":   prefs.Language,
		"reading:expand:media":       prefs.ExpandMedia,
		"reading:expand:spoilers":    prefs.ExpandSpoilers,
		"reading:autoplay:gifs":      prefs.AutoplayGifs,
	}, nil
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
func convertActorIDToHandle(actorID string) string {
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

func uploadToS3(ctx context.Context, key string, data []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := s3Client.PutObject(ctx, input)
	return err
}

func updateExportStatus(ctx context.Context, exportID, status string, completionData map[string]interface{}, errorMsg string) error {
	updateExpr := "SET #status = :status, UpdatedAt = :updated"
	exprAttrNames := map[string]string{
		"#status": "Status",
	}
	exprAttrValues := map[string]types.AttributeValue{
		":status":  &types.AttributeValueMemberS{Value: status},
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if completionData != nil {
		if url, ok := completionData["download_url"].(string); ok {
			updateExpr += ", DownloadURL = :url"
			exprAttrValues[":url"] = &types.AttributeValueMemberS{Value: url}
		}
		if expiresAt, ok := completionData["expires_at"].(time.Time); ok {
			updateExpr += ", ExpiresAt = :expires"
			exprAttrValues[":expires"] = &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)}
		}
		if size, ok := completionData["file_size"].(int); ok {
			updateExpr += ", FileSize = :size"
			exprAttrValues[":size"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", size)}
		}
		if count, ok := completionData["record_count"].(int); ok {
			updateExpr += ", RecordCount = :count"
			exprAttrValues[":count"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", count)}
		}
		if s3Key, ok := completionData["s3_key"].(string); ok {
			updateExpr += ", S3Key = :s3key"
			exprAttrValues[":s3key"] = &types.AttributeValueMemberS{Value: s3Key}
		}
		updateExpr += ", CompletedAt = :completed"
		exprAttrValues[":completed"] = &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)}
	}

	if errorMsg != "" {
		updateExpr += ", Error = :error"
		exprAttrValues[":error"] = &types.AttributeValueMemberS{Value: errorMsg}
	}

	_, err := dynamoClient.UpdateItem(ctx, &dynamodbsdk.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EXPORT#%s", exportID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EXPORT#%s", exportID)},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrValues,
	})

	return err
}

func getDomainBlocks(ctx context.Context, username string) ([]string, error) {
	// Query domain blocks from DynamoDB using storage client
	var allDomainBlocks []string
	cursor := ""

	for {
		domains, nextCursor, err := storageClient.GetUserDomainBlocks(ctx, username, 1000, cursor)
		if err != nil {
			logger.Error("failed to get domain blocks", zap.String("username", username), zap.Error(err))
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
