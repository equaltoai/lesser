package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/media"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
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
	// mediaConvertClient   *mediaconvert.Client // Placeholder - MediaConvert integration not implemented
	costTracker          *cost.Tracker
	tableName            string
	bucketName           string
	cdnDomain            string
	mediaConvertEndpoint string
	mediaConvertRole     string
)

// MediaProcessingEvent represents the event triggered for media processing
type MediaProcessingEvent struct {
	JobID    string `json:"job_id"`
	MediaID  string `json:"media_id"`
	Username string `json:"username"`
}

// ProcessingResult stores the results of each processing task
type ProcessingResult struct {
	Width           int                 `json:"width,omitempty"`
	Height          int                 `json:"height,omitempty"`
	Duration        int                 `json:"duration,omitempty"` // milliseconds
	Blurhash        string              `json:"blurhash,omitempty"`
	PreviewURL      string              `json:"preview_url,omitempty"`
	Sizes           map[string]SizeInfo `json:"sizes,omitempty"`
	ProcessingJobID string              `json:"processing_job_id,omitempty"` // MediaConvert job ID
}

// MediaConfig represents user's media processing configuration
type MediaConfig struct {
	VideoProcessingEnabled   bool  `json:"video_processing_enabled"`
	AudioProcessingEnabled   bool  `json:"audio_processing_enabled"`
	VideoThumbnailsEnabled   bool  `json:"video_thumbnails_enabled"`
	ContentModerationEnabled bool  `json:"content_moderation_enabled"`
	MaxVideoDuration         int   `json:"max_video_duration"` // seconds
	UserBudgetMicros         int64 `json:"user_budget_micros"`
}

// SizeInfo contains information about a processed size variant
type SizeInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
	S3Key  string `json:"s3_key"`
}

// Allowed MIME types for processing
var allowedMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/jpg":       true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
	"audio/mpeg":      true,
	"audio/mp3":       true,
	"audio/ogg":       true,
	"audio/wav":       true,
	"audio/webm":      true,
}

// Maximum file sizes by type (in bytes)
const (
	maxImageSize = 10 * 1024 * 1024 // 10MB for images
	maxVideoSize = 50 * 1024 * 1024 // 50MB for videos
	maxAudioSize = 20 * 1024 * 1024 // 20MB for audio
	maxGifSize   = 15 * 1024 * 1024 // 15MB for GIFs
)

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

	mediaConvertEndpoint = os.Getenv("MEDIACONVERT_ENDPOINT")
	if mediaConvertEndpoint == "" {
		logger.Warn("MEDIACONVERT_ENDPOINT not set, will use default")
	}

	mediaConvertRole = os.Getenv("MEDIACONVERT_ROLE_ARN")
	if mediaConvertRole == "" {
		logger.Warn("MEDIACONVERT_ROLE_ARN not set, MediaConvert jobs will fail")
	}

	// Initialize cost tracker
	costTracker = cost.New()
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
		if err := common.ParseRequestBody([]byte(message.Body), &event); err != nil {
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
			if updateErr := updateJobStatus(ctx, event.JobID, "failed", nil, err.Error()); updateErr != nil {
				logger.Error("Failed to update job status",
					zap.String("jobID", event.JobID),
					zap.Error(updateErr))
			}
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

	// Initialize MediaConvert client - PLACEHOLDER (MediaConvert SDK not available)
	// TODO: Uncomment when AWS MediaConvert SDK is added to go.mod
	/*
		if mediaConvertEndpoint != "" {
			mediaConvertClient = mediaconvert.NewFromConfig(cfg, func(o *mediaconvert.Options) {
				o.EndpointResolver = mediaconvert.EndpointResolverFunc(func(region string, options mediaconvert.EndpointResolverOptions) (aws.Endpoint, error) {
					return aws.Endpoint{URL: mediaConvertEndpoint}, nil
				})
			})
		} else {
			mediaConvertClient = mediaconvert.NewFromConfig(cfg)
		}
	*/

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

	// Validate file type
	if err := validateFileType(originalData, mimeType); err != nil {
		return fmt.Errorf("file type validation failed: %w", err)
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

	// Check resources before processing
	monitor := common.GetLambdaMonitor()
	if err := monitor.CheckResources("image-processing-start"); err != nil {
		logger.Error("resource limit approaching", zap.Error(err))
		return result, err
	}

	// Process image using the new media package with resource monitoring
	var processedImages map[string]*media.ProcessedImage
	err := monitor.WrapWithResourceCheck("image-resize", func() error {
		var procErr error
		processedImages, procErr = media.ProcessImage(data, mimeType)
		return procErr
	})
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
		filename := sizeName + ext

		// Sanitize S3 key to prevent path traversal
		s3Key, err := sanitizeS3Key(event.Username, event.MediaID, filename)
		if err != nil {
			logger.Error("failed to sanitize S3 key",
				zap.String("username", event.Username),
				zap.String("media_id", event.MediaID),
				zap.Error(err))
			continue
		}

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

	// 1. Get user's media processing config
	config, err := getUserMediaConfig(ctx, event.Username)
	if err != nil {
		logger.Error("failed to get user media config", zap.Error(err))
		// Fall back to basic upload
		return uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 2. Check if video processing is enabled
	if !config.VideoProcessingEnabled {
		logger.Info("video processing disabled for user", zap.String("username", event.Username))
		return uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 3. Check user's remaining budget
	remainingBudget, err := getUserRemainingBudget(ctx, event.Username)
	if err != nil {
		logger.Error("failed to get user budget", zap.Error(err))
		return uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 4. Estimate cost for this operation
	estimatedCost := estimateVideoCost(len(data))
	if estimatedCost > remainingBudget {
		logger.Warn("user exceeded media budget",
			zap.String("username", event.Username),
			zap.Int64("estimated_cost", estimatedCost),
			zap.Int64("remaining_budget", remainingBudget))

		// Fallback to basic upload only
		return uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 5. Upload to S3 first
	s3Key, err := sanitizeS3Key(event.Username, event.MediaID, "original.mp4")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := uploadToS3(ctx, s3Key, data, "video/mp4"); err != nil {
		return result, fmt.Errorf("failed to upload video: %w", err)
	}

	// 6. Create MediaConvert job for thumbnails and metadata if enabled
	if config.VideoThumbnailsEnabled && mediaConvertRole != "" {
		jobID, err := createMediaConvertJob(ctx, s3Key, event)
		if err != nil {
			logger.Error("failed to create MediaConvert job", zap.Error(err))
			// Continue anyway - video is uploaded
		} else {
			result.ProcessingJobID = jobID
			logger.Info("created MediaConvert job",
				zap.String("job_id", jobID),
				zap.String("media_id", event.MediaID))
		}
	}

	// 7. Track the cost
	trackUserSpend(ctx, event.Username, estimatedCost, "video_processing")

	result.Sizes["original"] = SizeInfo{
		URL:   buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// For now, return placeholder dimensions until MediaConvert completes
	result.Width = 0
	result.Height = 0
	result.Duration = 0

	return result, nil
}

func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
	result := ProcessingResult{}

	// 1. Get user's media processing config
	config, err := getUserMediaConfig(ctx, event.Username)
	if err != nil {
		logger.Error("failed to get user media config", zap.Error(err))
		return uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 2. Check if audio processing is enabled
	if !config.AudioProcessingEnabled {
		logger.Info("audio processing disabled for user", zap.String("username", event.Username))
		return uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 3. Check user's remaining budget
	remainingBudget, err := getUserRemainingBudget(ctx, event.Username)
	if err != nil {
		logger.Error("failed to get user budget", zap.Error(err))
		return uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 4. Upload original audio
	audioKey, err := sanitizeS3Key(event.Username, event.MediaID, "audio.mp3")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
		return result, fmt.Errorf("failed to upload audio: %w", err)
	}

	result.Sizes = map[string]SizeInfo{
		"original": {
			URL:   buildMediaURL(audioKey),
			S3Key: audioKey,
		},
	}

	// 5. Simple duration extraction (without external dependencies)
	// For production, use github.com/dhowden/tag or github.com/tcolgate/mp3
	// This is a placeholder that returns 0 duration
	result.Duration = 0
	logger.Warn("audio duration extraction not implemented - requires audio metadata library")

	// 6. Track the cost (minimal for audio)
	audioCost := int64(100) // $0.0001 for basic audio processing
	if audioCost <= remainingBudget {
		trackUserSpend(ctx, event.Username, audioCost, "audio_processing")
	}

	// Track S3 operations
	costTracker.TrackS3Put(1)
	costTracker.TrackS3Storage(int64(len(data)))

	logger.Info("audio processing completed",
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username))

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

	// Use common.ReadRequestBody to enforce size limits
	// Use the maximum allowed size across all media types
	maxSize := int64(maxVideoSize) // 50MB is the largest allowed
	data, err := common.ReadRequestBody(result.Body, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}

	return data, nil
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

// Helper functions for cost-aware processing

func getUserMediaConfig(ctx context.Context, username string) (*MediaConfig, error) {
	// Get user's media config from DynamoDB
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
			"SK": &types.AttributeValueMemberS{Value: "MEDIA#CONFIG"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get user media config: %w", err)
	}

	// Check for instance defaults if user config doesn't exist
	if result.Item == nil {
		result, err = dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "INSTANCE#CONFIG"},
				"SK": &types.AttributeValueMemberS{Value: "MEDIA#DEFAULTS"},
			},
		})

		if err != nil || result.Item == nil {
			// Return defaults if no config exists
			return &MediaConfig{
				VideoProcessingEnabled:   false,
				AudioProcessingEnabled:   false,
				VideoThumbnailsEnabled:   false,
				ContentModerationEnabled: false,
				MaxVideoDuration:         300,
				UserBudgetMicros:         5_000_000, // $5 default
			}, nil
		}
	}

	// Parse config
	var config MediaConfig
	if err := attributevalue.UnmarshalMap(result.Item, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal media config: %w", err)
	}

	return &config, nil
}

func getUserRemainingBudget(ctx context.Context, username string) (int64, error) {
	// Get current month's spending
	now := time.Now()
	monthKey := fmt.Sprintf("USER#%s#SPENDING#%d-%02d", username, now.Year(), now.Month())

	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: monthKey},
			"SK": &types.AttributeValueMemberS{Value: "TOTAL"},
		},
	})

	if err != nil {
		return 0, fmt.Errorf("failed to get user spending: %w", err)
	}

	var currentSpending int64
	if result.Item != nil {
		if v, ok := result.Item["Amount"].(*types.AttributeValueMemberN); ok {
			currentSpending, _ = strconv.ParseInt(v.Value, 10, 64)
		}
	}

	// Get user's budget
	config, err := getUserMediaConfig(ctx, username)
	if err != nil {
		return 0, err
	}

	remaining := config.UserBudgetMicros - currentSpending
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

func uploadOriginalOnly(ctx context.Context, data []byte, event MediaProcessingEvent, mimeType string) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// Determine file extension
	ext := ".bin"
	switch mimeType {
	case "video/mp4":
		ext = ".mp4"
	case "audio/mpeg":
		ext = ".mp3"
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	}

	// Upload original file
	filename := "original" + ext
	s3Key, err := sanitizeS3Key(event.Username, event.MediaID, filename)
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := uploadToS3(ctx, s3Key, data, mimeType); err != nil {
		return result, fmt.Errorf("failed to upload original: %w", err)
	}

	result.Sizes["original"] = SizeInfo{
		URL:   buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// Track minimal cost (just S3 storage)
	costTracker.TrackS3Put(1)
	costTracker.TrackS3Storage(int64(len(data)))

	return result, nil
}

func trackUserSpend(ctx context.Context, username string, amountMicros int64, category string) error {
	now := time.Now()
	monthKey := fmt.Sprintf("USER#%s#SPENDING#%d-%02d", username, now.Year(), now.Month())

	// Update monthly total
	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: monthKey},
			"SK": &types.AttributeValueMemberS{Value: "TOTAL"},
		},
		UpdateExpression: aws.String("ADD Amount :amount SET UpdatedAt = :now, Category = :cat"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":amount": &types.AttributeValueMemberN{Value: strconv.FormatInt(amountMicros, 10)},
			":now":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":cat":    &types.AttributeValueMemberS{Value: category},
		},
	})

	return err
}

func estimateVideoCost(sizeBytes int) int64 {
	// MediaConvert: ~$0.024 per minute HD
	// Estimate based on file size (rough approximation)
	estimatedMinutes := float64(sizeBytes) / (5 * 1024 * 1024) // 5MB per minute estimate
	costDollars := estimatedMinutes * 0.024
	return int64(costDollars * 1_000_000) // Convert to microdollars
}

func createMediaConvertJob(ctx context.Context, s3InputKey string, event MediaProcessingEvent) (string, error) {
	// This is a placeholder - MediaConvert requires proper configuration
	// In production, this would create a job to extract thumbnails and metadata
	logger.Warn("MediaConvert job creation not implemented - requires AWS MediaConvert setup")
	return "", fmt.Errorf("MediaConvert not configured")
}


// validateFileType checks if the file type is allowed and matches content
func validateFileType(data []byte, claimedMimeType string) error {
	// Check size first to avoid processing huge files
	if len(data) == 0 {
		return fmt.Errorf("empty file")
	}

	// Detect actual MIME type from file content
	detectedType := http.DetectContentType(data)

	// Clean up detected type (remove charset info)
	if idx := strings.Index(detectedType, ";"); idx > 0 {
		detectedType = detectedType[:idx]
	}

	// Check if detected type is allowed
	if !allowedMimeTypes[detectedType] {
		logger.Warn("file type not allowed",
			zap.String("detected_type", detectedType),
			zap.String("claimed_type", claimedMimeType))
		return fmt.Errorf("file type not allowed: %s", detectedType)
	}

	// Warn if claimed type doesn't match detected type
	if claimedMimeType != "" && claimedMimeType != detectedType {
		logger.Warn("MIME type mismatch",
			zap.String("claimed", claimedMimeType),
			zap.String("detected", detectedType))
	}

	// Check file size limits based on type
	if err := checkFileSizeLimit(data, detectedType); err != nil {
		return err
	}

	return nil
}

// checkFileSizeLimit checks if file size is within allowed limits
func checkFileSizeLimit(data []byte, mimeType string) error {
	size := len(data)
	var maxSize int
	var fileType string

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		if mimeType == "image/gif" {
			maxSize = maxGifSize
			fileType = "GIF"
		} else {
			maxSize = maxImageSize
			fileType = "image"
		}
	case strings.HasPrefix(mimeType, "video/"):
		maxSize = maxVideoSize
		fileType = "video"
	case strings.HasPrefix(mimeType, "audio/"):
		maxSize = maxAudioSize
		fileType = "audio"
	default:
		return fmt.Errorf("unknown file type: %s", mimeType)
	}

	if size > maxSize {
		return fmt.Errorf("%s file too large: %d bytes (max: %d bytes)", fileType, size, maxSize)
	}

	return nil
}

// sanitizeS3Key ensures the S3 key doesn't contain path traversal attempts
func sanitizeS3Key(username, mediaID, filename string) (string, error) {
	// Validate username doesn't contain path traversal
	if strings.Contains(username, "..") || strings.Contains(username, "/") {
		return "", fmt.Errorf("invalid username for S3 key")
	}

	// Validate mediaID
	if strings.Contains(mediaID, "..") || strings.Contains(mediaID, "/") {
		return "", fmt.Errorf("invalid media ID for S3 key")
	}

	// Validate filename
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		return "", fmt.Errorf("invalid filename for S3 key")
	}

	// Construct safe S3 key
	return fmt.Sprintf("media/%s/%s/%s", username, mediaID, filename), nil
}
