package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/media"
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
	cdnDomain    string
)

// MediaProcessingEvent represents the event triggered for media processing
type MediaProcessingEvent struct {
	JobID    string `json:"job_id"`
	MediaID  string `json:"media_id"`
	Username string `json:"username"`
}

// ProcessingResult stores the results of each processing task
type ProcessingResult struct {
	Width      int                 `json:"width,omitempty"`
	Height     int                 `json:"height,omitempty"`
	Duration   int                 `json:"duration,omitempty"` // milliseconds
	Blurhash   string              `json:"blurhash,omitempty"`
	PreviewURL string              `json:"preview_url,omitempty"`
	Sizes      map[string]SizeInfo `json:"sizes,omitempty"`
}

// SizeInfo contains information about a processed size variant
type SizeInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
	S3Key  string `json:"s3_key"`
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

	cdnDomain = os.Getenv("CDN_DOMAIN")
	if cdnDomain == "" {
		logger.Warn("CDN_DOMAIN not set, will use S3 URLs")
	}

	tableName = os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE_NAME environment variable not set")
	}
}

func main() {
	lambda.Start(handleMediaProcessing)
}

func handleMediaProcessing(ctx context.Context, sqsEvent events.SQSEvent) error {
	// Initialize AWS clients
	if err := initializeAWSClients(ctx); err != nil {
		logger.Error("failed to initialize AWS clients", zap.Error(err))
		return err
	}

	// Process each message
	for _, message := range sqsEvent.Records {
		var event MediaProcessingEvent
		if err := json.Unmarshal([]byte(message.Body), &event); err != nil {
			logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.Error(err))
			continue
		}

		if err := processMediaJob(ctx, event); err != nil {
			logger.Error("failed to process media job",
				zap.String("job_id", event.JobID),
				zap.String("media_id", event.MediaID),
				zap.Error(err))
			// Don't return error to avoid reprocessing
			// Update job status as failed
			updateJobStatus(ctx, event.JobID, "failed", nil, err.Error())
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

func processMediaJob(ctx context.Context, event MediaProcessingEvent) error {
	logger.Info("processing media job",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID))

	// Get job details from DynamoDB
	jobData, err := getJobData(ctx, event.JobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Update job status to processing
	if err := updateJobStatus(ctx, event.JobID, "processing", nil, ""); err != nil {
		logger.Warn("failed to update job status", zap.Error(err))
	}

	// Get processing tasks
	tasks, _ := jobData["ProcessingTasks"].([]interface{})
	s3Key := jobData["S3Key"].(string)
	mimeType := jobData["MimeType"].(string)

	// Download original file from S3
	originalData, err := downloadFromS3(ctx, s3Key)
	if err != nil {
		return fmt.Errorf("failed to download original: %w", err)
	}

	// Process based on media type
	var result ProcessingResult
	mediaType := getMediaTypeFromMime(mimeType)

	switch mediaType {
	case "image", "gifv":
		result, err = processImage(ctx, originalData, event, tasks, mimeType)
		if err != nil {
			return fmt.Errorf("failed to process image: %w", err)
		}

	case "video":
		result, err = processVideo(ctx, originalData, event, tasks)
		if err != nil {
			return fmt.Errorf("failed to process video: %w", err)
		}

	case "audio":
		result, err = processAudio(ctx, originalData, event, tasks)
		if err != nil {
			return fmt.Errorf("failed to process audio: %w", err)
		}

	default:
		return fmt.Errorf("unsupported media type: %s", mediaType)
	}

	// Update media record with processing results
	if err := updateMediaRecord(ctx, event.MediaID, result); err != nil {
		return fmt.Errorf("failed to update media record: %w", err)
	}

	// Update job as completed
	if err := updateJobStatus(ctx, event.JobID, "completed", result, ""); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	logger.Info("media processing completed",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID))

	return nil
}

func processImage(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}, mimeType string) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// Process image using the new media package
	processedImages, err := media.ProcessImage(data, mimeType)
	if err != nil {
		return result, fmt.Errorf("failed to process image: %w", err)
	}

	// Get original image info
	if original, ok := processedImages["original"]; ok {
		result.Width = original.Width
		result.Height = original.Height
		result.Blurhash = original.Blurhash
	}

	// Upload each processed size to S3
	for sizeName, processed := range processedImages {
		// Generate S3 key for this size
		ext := getExtensionFromProcessedFormat(processed.Format)
		s3Key := fmt.Sprintf("media/%s/%s/%s%s",
			event.Username, event.MediaID, sizeName, ext)

		// Upload to S3
		if err := uploadToS3(ctx, s3Key, processed.Data, getMimeTypeFromFormat(processed.Format)); err != nil {
			logger.Warn("failed to upload image size",
				zap.String("size", sizeName),
				zap.Error(err))
			continue
		}

		// Build URL
		url := buildMediaURL(s3Key)

		// Store size info
		result.Sizes[sizeName] = SizeInfo{
			Width:  processed.Width,
			Height: processed.Height,
			URL:    url,
			S3Key:  s3Key,
		}

		// Set preview URL to small size
		if sizeName == "small" {
			result.PreviewURL = url
		}
	}

	// Process specific tasks that aren't handled by default
	for _, task := range tasks {
		taskStr, _ := task.(string)
		switch taskStr {
		case "exif":
			// EXIF stripping is handled automatically by re-encoding
			logger.Info("EXIF data stripped during processing")
		}
	}

	return result, nil
}

func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// TODO: Implement video processing
	// This would use ffmpeg or similar to:
	// 1. Extract dimensions and duration
	// 2. Generate thumbnail from first/middle frame
	// 3. Transcode if needed
	// 4. Generate preview clip

	logger.Warn("video processing not yet implemented")

	// For now, return some placeholder data
	result.Width = 1920
	result.Height = 1080
	result.Duration = 30000 // 30 seconds in milliseconds

	return result, nil
}

func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
	result := ProcessingResult{}

	// TODO: Implement audio processing
	// This would:
	// 1. Extract duration and metadata
	// 2. Generate waveform visualization
	// 3. Extract/generate cover art

	logger.Warn("audio processing not yet implemented")

	// For now, return some placeholder data
	result.Duration = 180000 // 3 minutes in milliseconds

	return result, nil
}

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

func uploadToS3(ctx context.Context, key string, data []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err := s3Client.PutObject(ctx, input)
	return err
}

func getJobData(ctx context.Context, jobID string) (map[string]interface{}, error) {
	// Get job from DynamoDB
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("JOB#%s", jobID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("JOB#%s", jobID)},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("job not found")
	}

	// Convert to map
	jobData := make(map[string]interface{})
	for k, v := range result.Item {
		if sv, ok := v.(*types.AttributeValueMemberS); ok {
			jobData[k] = sv.Value
		} else if nv, ok := v.(*types.AttributeValueMemberN); ok {
			jobData[k] = nv.Value
		} else if lv, ok := v.(*types.AttributeValueMemberL); ok {
			var list []interface{}
			for _, item := range lv.Value {
				if itemStr, ok := item.(*types.AttributeValueMemberS); ok {
					list = append(list, itemStr.Value)
				}
			}
			jobData[k] = list
		}
	}

	return jobData, nil
}

func updateJobStatus(ctx context.Context, jobID, status string, result interface{}, errorMsg string) error {
	updateExpr := "SET #status = :status, UpdatedAt = :updated"
	exprAttrNames := map[string]string{
		"#status": "Status",
	}
	exprAttrValues := map[string]types.AttributeValue{
		":status":  &types.AttributeValueMemberS{Value: status},
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	if result != nil {
		// Marshal result to JSON string
		resultJSON, _ := json.Marshal(result)
		updateExpr += ", Results = :results"
		exprAttrValues[":results"] = &types.AttributeValueMemberS{Value: string(resultJSON)}
	}

	if errorMsg != "" {
		updateExpr += ", Error = :error"
		exprAttrValues[":error"] = &types.AttributeValueMemberS{Value: errorMsg}
	}

	if status == "processing" {
		updateExpr += ", GSI2PK = :gsi2pk, GSI2SK = :gsi2sk"
		exprAttrValues[":gsi2pk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", status)}
		exprAttrValues[":gsi2sk"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("UPDATED#%s", time.Now().Format(time.RFC3339))}
	}

	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("JOB#%s", jobID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("JOB#%s", jobID)},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrValues,
	})

	return err
}

func updateMediaRecord(ctx context.Context, mediaID string, result ProcessingResult) error {
	updateExpr := "SET Processing = :false, UpdatedAt = :updated"
	exprAttrValues := map[string]types.AttributeValue{
		":false":   &types.AttributeValueMemberBOOL{Value: false},
		":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	// Build final URL (use original or largest size)
	if original, ok := result.Sizes["original"]; ok {
		updateExpr += ", #url = :url"
		exprAttrValues[":url"] = &types.AttributeValueMemberS{Value: original.URL}
	}

	if result.PreviewURL != "" {
		updateExpr += ", PreviewURL = :preview"
		exprAttrValues[":preview"] = &types.AttributeValueMemberS{Value: result.PreviewURL}
	}

	if result.Blurhash != "" {
		updateExpr += ", Blurhash = :blurhash"
		exprAttrValues[":blurhash"] = &types.AttributeValueMemberS{Value: result.Blurhash}
	}

	if result.Width > 0 {
		updateExpr += ", Width = :width, Height = :height"
		exprAttrValues[":width"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.Width)}
		exprAttrValues[":height"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.Height)}
	}

	if result.Duration > 0 {
		updateExpr += ", Duration = :duration"
		exprAttrValues[":duration"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.Duration)}
	}

	exprAttrNames := map[string]string{}
	if strings.Contains(updateExpr, "#url") {
		exprAttrNames["#url"] = "URL"
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MEDIA#%s", mediaID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeValues: exprAttrValues,
	}

	if len(exprAttrNames) > 0 {
		input.ExpressionAttributeNames = exprAttrNames
	}

	_, err := dynamoClient.UpdateItem(ctx, input)
	return err
}

func buildMediaURL(s3Key string) string {
	if cdnDomain != "" {
		return fmt.Sprintf("https://%s/%s", cdnDomain, s3Key)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, s3Key)
}

func getMediaTypeFromMime(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		if mimeType == "image/gif" {
			return "gifv"
		}
		return "image"
	} else if strings.HasPrefix(mimeType, "video/") {
		return "video"
	} else if strings.HasPrefix(mimeType, "audio/") {
		return "audio"
	}
	return "unknown"
}

func getExtensionFromProcessedFormat(format string) string {
	switch format {
	case "jpeg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func getMimeTypeFromFormat(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
