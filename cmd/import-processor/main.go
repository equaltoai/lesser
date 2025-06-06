package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
	lambda.Start(handleImportProcessing)
}

func handleImportProcessing(ctx context.Context, sqsEvent events.SQSEvent) error {
	// Initialize AWS clients
	if err := initializeAWSClients(ctx); err != nil {
		logger.Error("failed to initialize AWS clients", zap.Error(err))
		return err
	}

	// Process each message
	for _, message := range sqsEvent.Records {
		var event ImportProcessorEvent
		if err := json.Unmarshal([]byte(message.Body), &event); err != nil {
			logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.Error(err))
			continue
		}

		if err := processImportJob(ctx, event); err != nil {
			logger.Error("failed to process import job",
				zap.String("import_id", event.ImportID),
				zap.String("username", event.Username),
				zap.Error(err))
			// Update job status as failed
			updateImportStatus(ctx, event.ImportID, "failed", nil, err.Error())
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

func processImportJob(ctx context.Context, event ImportProcessorEvent) error {
	logger.Info("processing import job",
		zap.String("import_id", event.ImportID),
		zap.String("username", event.Username),
		zap.String("type", event.Type))

	// Update job status to processing
	if err := updateImportStatus(ctx, event.ImportID, "processing", nil, ""); err != nil {
		logger.Warn("failed to update import status", zap.Error(err))
	}

	// Download file from S3
	fileData, err := downloadFromS3(ctx, event.S3Key)
	if err != nil {
		return fmt.Errorf("failed to download import file: %w", err)
	}

	// Detect format
	format := detectFormat(fileData)
	logger.Info("detected import format", zap.String("format", format))

	// Process based on format and type
	var result ImportResult

	switch format {
	case "csv":
		result, err = processCSVImport(ctx, event, fileData)
	case "json":
		result, err = processJSONImport(ctx, event, fileData)
	case "activitypub":
		result, err = processActivityPubImport(ctx, event, fileData)
	default:
		return fmt.Errorf("unsupported import format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("failed to process import: %w", err)
	}

	// Update import job as completed
	completionData := map[string]interface{}{
		"total":   result.Success + result.Skipped + result.Failed,
		"success": result.Success,
		"skipped": result.Skipped,
		"failed":  result.Failed,
		"errors":  result.Errors,
	}

	if err := updateImportStatus(ctx, event.ImportID, "completed", completionData, ""); err != nil {
		return fmt.Errorf("failed to update import status: %w", err)
	}

	logger.Info("import completed",
		zap.String("import_id", event.ImportID),
		zap.Int("success", result.Success),
		zap.Int("skipped", result.Skipped),
		zap.Int("failed", result.Failed))

	return nil
}

func detectFormat(data []byte) string {
	// Try to parse as JSON first
	var jsonTest interface{}
	if err := json.Unmarshal(data, &jsonTest); err == nil {
		// Check if it's an ActivityPub collection
		if jsonMap, ok := jsonTest.(map[string]interface{}); ok {
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

func processCSVImport(ctx context.Context, event ImportProcessorEvent, data []byte) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	reader := csv.NewReader(bytes.NewReader(data))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return result, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Process based on type
	switch event.Type {
	case "followers":
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

			// In a real implementation, we might send follow invites or something
			result.Skipped++
		}

	case "following":
		// Process following list
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
			if accountAddress == "" {
				continue
			}

			// Update progress
			updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

			// Follow the account
			if err := followAccount(ctx, event.Username, accountAddress); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to follow %s: %v", accountAddress, err))
			} else {
				result.Success++
			}
		}

	case "blocks":
		// Process blocks
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
			if accountAddress == "" {
				continue
			}

			// Update progress
			updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

			// Block the account
			if err := blockAccount(ctx, event.Username, accountAddress); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to block %s: %v", accountAddress, err))
			} else {
				result.Success++
			}
		}

	case "mutes":
		// Process mutes
		hideNotificationsIndex := -1
		for i, col := range header {
			if col == "Hide notifications" {
				hideNotificationsIndex = i
				break
			}
		}

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
			if accountAddress == "" {
				continue
			}

			hideNotifications := false
			if hideNotificationsIndex >= 0 && len(record) > hideNotificationsIndex {
				hideNotifications = record[hideNotificationsIndex] == "true"
			}

			// Update progress
			updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

			// Mute the account
			if err := muteAccount(ctx, event.Username, accountAddress, hideNotifications); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to mute %s: %v", accountAddress, err))
			} else {
				result.Success++
			}
		}

	case "bookmarks":
		// Process bookmarks
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
			if statusURL == "" {
				continue
			}

			// Update progress
			updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

			// Bookmark the status
			if err := bookmarkStatus(ctx, event.Username, statusURL); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to bookmark %s: %v", statusURL, err))
			} else {
				result.Success++
			}
		}

	default:
		return result, fmt.Errorf("CSV import not supported for type: %s", event.Type)
	}

	return result, nil
}

func processJSONImport(ctx context.Context, event ImportProcessorEvent, data []byte) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	// Parse JSON based on type
	switch event.Type {
	case "lists":
		// Import lists with members
		var lists map[string][]string
		if err := json.Unmarshal(data, &lists); err != nil {
			return result, fmt.Errorf("failed to parse lists JSON: %w", err)
		}

		for listName, members := range lists {
			// Create or update list
			listID, err := createOrUpdateList(ctx, event.Username, listName)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to create list %s: %v", listName, err))
				continue
			}

			// Add members to list
			for _, member := range members {
				updateImportProgress(ctx, event.ImportID, result.Success+result.Skipped+result.Failed+1)

				if err := addToList(ctx, event.Username, listID, member); err != nil {
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to add %s to list: %v", member, err))
				} else {
					result.Success++
				}
			}
		}

	default:
		return result, fmt.Errorf("JSON import not supported for type: %s", event.Type)
	}

	return result, nil
}

func processActivityPubImport(ctx context.Context, event ImportProcessorEvent, data []byte) (ImportResult, error) {
	result := ImportResult{
		Errors: make([]string, 0),
	}

	// For ActivityPub archives, we need to handle them differently based on type
	if event.Type != "archive" {
		return result, fmt.Errorf("ActivityPub import only supported for archive type")
	}

	// This would be a complex operation involving:
	// 1. Parsing the ActivityPub collection
	// 2. Importing posts (if applicable)
	// 3. Recreating follows/blocks/etc
	// For now, we'll just count the items

	var collection map[string]interface{}
	if err := json.Unmarshal(data, &collection); err != nil {
		return result, fmt.Errorf("failed to parse ActivityPub collection: %w", err)
	}

	// Count items in the collection
	if items, ok := collection["orderedItems"].([]interface{}); ok {
		result.Success = len(items)
	}

	return result, nil
}

// Helper functions for performing the actual import operations

func followAccount(ctx context.Context, username, targetAccount string) error {
	// Resolve the account via WebFinger if needed
	actorID, err := resolveAccount(ctx, targetAccount)
	if err != nil {
		return err
	}

	// Create follow relationship in DynamoDB
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("FOLLOWING#%s", actorID)},
		"ActorID":    &types.AttributeValueMemberS{Value: actorID},
		"CreatedAt":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		"FollowType": &types.AttributeValueMemberS{Value: "following"},
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	// TODO: Send Follow activity to the remote actor

	return err
}

func blockAccount(ctx context.Context, username, targetAccount string) error {
	// Resolve the account
	actorID, err := resolveAccount(ctx, targetAccount)
	if err != nil {
		return err
	}

	// Create block in DynamoDB
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("BLOCK#%s", actorID)},
		"ActorID":   &types.AttributeValueMemberS{Value: actorID},
		"CreatedAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

func muteAccount(ctx context.Context, username, targetAccount string, hideNotifications bool) error {
	// Resolve the account
	actorID, err := resolveAccount(ctx, targetAccount)
	if err != nil {
		return err
	}

	// Create mute in DynamoDB
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":                &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK":                &types.AttributeValueMemberS{Value: fmt.Sprintf("MUTE#%s", actorID)},
		"ActorID":           &types.AttributeValueMemberS{Value: actorID},
		"HideNotifications": &types.AttributeValueMemberBOOL{Value: hideNotifications},
		"CreatedAt":         &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

func bookmarkStatus(ctx context.Context, username, statusURL string) error {
	// Extract status ID from URL
	// This is simplified - would need proper URL parsing
	statusID := strings.TrimPrefix(statusURL, baseURL+"/")

	// Create bookmark in DynamoDB
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("BOOKMARK#%s", statusID)},
		"StatusID":  &types.AttributeValueMemberS{Value: statusID},
		"StatusURL": &types.AttributeValueMemberS{Value: statusURL},
		"CreatedAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

func createOrUpdateList(ctx context.Context, username, listName string) (string, error) {
	// Generate list ID
	listID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create list in DynamoDB
	now := time.Now()
	item := map[string]types.AttributeValue{
		"PK":            &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		"SK":            &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
		"ListID":        &types.AttributeValueMemberS{Value: listID},
		"Title":         &types.AttributeValueMemberS{Value: listName},
		"RepliesPolicy": &types.AttributeValueMemberS{Value: "list"}, // Default
		"CreatedAt":     &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
	}

	_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return listID, err
}

func addToList(ctx context.Context, username, listID, accountAddress string) error {
	// Resolve the account
	actorID, err := resolveAccount(ctx, accountAddress)
	if err != nil {
		return err
	}

	// Add member to list
	item := map[string]types.AttributeValue{
		"PK":      &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
		"SK":      &types.AttributeValueMemberS{Value: fmt.Sprintf("MEMBER#%s", actorID)},
		"ActorID": &types.AttributeValueMemberS{Value: actorID},
		"AddedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

func resolveAccount(ctx context.Context, accountAddress string) (string, error) {
	// If it's already a full actor ID, return it
	if strings.HasPrefix(accountAddress, "https://") {
		return accountAddress, nil
	}

	// Parse account address (user@domain)
	parts := strings.Split(accountAddress, "@")
	if len(parts) != 2 {
		// Assume local user if no domain
		return fmt.Sprintf("%s/users/%s", baseURL, accountAddress), nil
	}

	username := parts[0]
	domain := parts[1]

	// Check if it's a local user
	if domain == strings.TrimPrefix(baseURL, "https://") {
		return fmt.Sprintf("%s/users/%s", baseURL, username), nil
	}

	// Use WebFinger to resolve remote account
	// This is a simplified version - would need proper WebFinger client
	// webfingerURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=acct:%s@%s", domain, username, domain)

	// TODO: Make HTTP request and parse WebFinger response
	// For now, construct a likely actor ID
	return fmt.Sprintf("https://%s/users/%s", domain, username), nil
}

// Helper functions for status updates

func downloadFromS3(ctx context.Context, key string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	}

	result, err := s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func updateImportStatus(ctx context.Context, importID, status string, completionData map[string]interface{}, errorMsg string) error {
	updateExpr := "SET #status = :status, UpdatedAt = :updated"
	exprAttrNames := map[string]string{
		"#status": "Status",
	}
	exprAttrValues := map[string]types.AttributeValue{
		":status":  &types.AttributeValueMemberS{Value: status},
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if completionData != nil {
		if total, ok := completionData["total"].(int); ok {
			updateExpr += ", Total = :total"
			exprAttrValues[":total"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", total)}
		}
		if success, ok := completionData["success"].(int); ok {
			updateExpr += ", SuccessCount = :success"
			exprAttrValues[":success"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", success)}
		}
		if skipped, ok := completionData["skipped"].(int); ok {
			updateExpr += ", SkipCount = :skipped"
			exprAttrValues[":skipped"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", skipped)}
		}
		if failed, ok := completionData["failed"].(int); ok {
			updateExpr += ", ErrorCount = :failed"
			exprAttrValues[":failed"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", failed)}
		}
		if errors, ok := completionData["errors"].([]string); ok && len(errors) > 0 {
			// Store first few errors
			maxErrors := 10
			if len(errors) > maxErrors {
				errors = errors[:maxErrors]
			}
			errorsJSON, _ := json.Marshal(errors)
			updateExpr += ", Errors = :errors"
			exprAttrValues[":errors"] = &types.AttributeValueMemberS{Value: string(errorsJSON)}
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
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("IMPORT#%s", importID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("IMPORT#%s", importID)},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrValues,
	})

	return err
}

func updateImportProgress(ctx context.Context, importID string, progress int) error {
	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("IMPORT#%s", importID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("IMPORT#%s", importID)},
		},
		UpdateExpression: aws.String("SET Progress = :progress"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":progress": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", progress)},
		},
	})

	return err
}
