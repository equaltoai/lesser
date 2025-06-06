package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger       *zap.Logger
	s3Client     *s3.Client
	dynamoClient *dynamodb.Client
	tableName    string
	bucketName   string
	baseURL      string
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
		if err := json.Unmarshal([]byte(message.Body), &event); err != nil {
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
	dynamoClient = dynamodb.NewFromConfig(cfg)

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
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
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
	// Query followers from DynamoDB
	// This would query GSI with pattern FOLLOWER#username
	// For now, return empty list
	return []string{}, nil
}

func getFollowing(ctx context.Context, username string) ([]string, error) {
	// Query following from DynamoDB
	// This would query with pattern USER#username and SK prefix FOLLOWING#
	// For now, return empty list
	return []string{}, nil
}

func getBlocks(ctx context.Context, username string) ([]string, error) {
	// Query blocks from DynamoDB
	// For now, return empty list
	return []string{}, nil
}

type MuteInfo struct {
	AccountID         string
	HideNotifications bool
}

func getMutes(ctx context.Context, username string) ([]MuteInfo, error) {
	// Query mutes from DynamoDB
	// For now, return empty list
	return []MuteInfo{}, nil
}

func getListsWithMembers(ctx context.Context, username string) (map[string][]string, error) {
	// Query lists and their members from DynamoDB
	// For now, return empty map
	return map[string][]string{}, nil
}

type BookmarkInfo struct {
	StatusURL string
	CreatedAt time.Time
}

func getBookmarks(ctx context.Context, username string) ([]BookmarkInfo, error) {
	// Query bookmarks from DynamoDB
	// For now, return empty list
	return []BookmarkInfo{}, nil
}

func getOutbox(ctx context.Context, username string, dateRange *DateRange) ([]interface{}, int, error) {
	// Query user's posts from DynamoDB
	// This would query objects where AttributedTo = user's actor ID
	// For now, return empty list
	return []interface{}{}, 0, nil
}

func getFollowingActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for following
	// For now, return empty list
	return []string{}, nil
}

func getFollowersActors(ctx context.Context, username string) ([]string, error) {
	// Get full actor IDs for followers
	// For now, return empty list
	return []string{}, nil
}

func getLikes(ctx context.Context, username string) ([]interface{}, error) {
	// Query user's likes from DynamoDB
	// For now, return empty list
	return []interface{}{}, nil
}

func getBookmarksForExport(ctx context.Context, username string) ([]interface{}, error) {
	// Query bookmarks and convert to ActivityPub format
	// For now, return empty list
	return []interface{}{}, nil
}

func getListsForExport(ctx context.Context, username string) ([]interface{}, error) {
	// Query lists and convert to export format
	// For now, return empty list
	return []interface{}{}, nil
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

	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
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
