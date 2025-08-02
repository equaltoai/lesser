package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	mctypes "github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dhowden/tag"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/media"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// MediaProcessor handles media processing from SQS messages using Lift and DynamORM
type MediaProcessor struct {
	db                   core.DB
	storage              *dynamorm.StorageAdapter
	mediaRepo            *repositories.MediaRepository
	s3Client             *s3.Client
	mediaConvertClient   *mediaconvert.Client
	costTracker          *cost.Tracker
	tableName            string
	bucketName           string
	cdnDomain            string
	mediaConvertEndpoint string
	mediaConvertRole     string
	mediaConvertQueue    string
	logger               *zap.Logger
}

var (
	processor *MediaProcessor
	cfg       *config.Config
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
	var err error
	logger := common.Logger()
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repositories
	storageAdapter := dynamorm.NewStorageAdapter(db, cfg.DynamoTableName, logger, nil)
	mediaRepo := repositories.NewMediaRepository(db, cfg.DynamoTableName, logger)

	// Set media repository in storage adapter
	storageAdapter.SetMediaRepository(mediaRepo)

	// Get configuration from environment
	bucketName := os.Getenv("S3_BUCKET_NAME")
	if bucketName == "" {
		bucketName = cfg.S3BucketName
	}

	cdnDomain := os.Getenv("CDN_DOMAIN")
	if cdnDomain == "" {
		cdnDomain = cfg.Domain // Use domain instead of CDNDomain
	}

	mediaConvertEndpoint := os.Getenv("MEDIACONVERT_ENDPOINT")
	mediaConvertRole := os.Getenv("MEDIACONVERT_ROLE_ARN")
	mediaConvertQueue := os.Getenv("MEDIACONVERT_QUEUE")
	if mediaConvertQueue == "" {
		mediaConvertQueue = "Default"
	}

	// Create processor instance
	processor = &MediaProcessor{
		db:                   db,
		storage:              storageAdapter,
		mediaRepo:            mediaRepo,
		costTracker:          cost.New(),
		tableName:            cfg.DynamoTableName,
		bucketName:           bucketName,
		cdnDomain:            cdnDomain,
		mediaConvertEndpoint: mediaConvertEndpoint,
		mediaConvertRole:     mediaConvertRole,
		mediaConvertQueue:    mediaConvertQueue,
		logger:               logger,
	}
}

func main() {
	// Create Lift app
	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("media-processing-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID").(string)

			processor.logger.Info("processing SQS batch",
				zap.String("request_id", requestID),
			)

			err := next.Handle(ctx)

			duration := time.Since(start)
			if err != nil {
				processor.logger.Error("failed to process SQS batch",
					zap.String("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration),
				)
			} else {
				processor.logger.Info("successfully processed SQS batch",
					zap.String("request_id", requestID),
					zap.Duration("duration", duration),
				)
			}

			return err
		})
	})

	// Add error handling middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				processor.logger.Error("handler error",
					zap.String("request_id", ctx.Get("requestID").(string)),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Set SQS handler for media processing
	app.SQS("media-processing", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Parse the raw event as SQS event
		var event events.SQSEvent
		if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			event = sqsEvent
		} else {
			// Try to parse from interface if it's a map
			eventBytes, err := json.Marshal(ctx.Request.RawEvent)
			if err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500).WithCause(err)
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
			}
		}

		return processor.HandleSQS(ctx, event)
	})

	lambda.Start(app.HandleRequest)
}

// HandleSQS implements the SQS handler interface for Lift
func (mp *MediaProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Initialize AWS clients
	if err := mp.initializeAWSClients(ctx.Request.Context()); err != nil {
		mp.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	mp.logger.Info("processing media processing batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)))

	// Process each message
	for _, message := range event.Records {
		var processingEvent MediaProcessingEvent
		if err := common.ParseRequestBody([]byte(message.Body), &processingEvent); err != nil {
			mp.logger.Error("failed to unmarshal event",
				zap.String("message_id", message.MessageId),
				zap.Error(err))
			continue
		}

		if err := mp.processMediaJob(ctx.Request.Context(), processingEvent); err != nil {
			mp.logger.Error("failed to process media job",
				zap.String("job_id", processingEvent.JobID),
				zap.String("media_id", processingEvent.MediaID),
				zap.Error(err))
			// Don't return error to avoid reprocessing
			// Update job status as failed
			if updateErr := mp.updateJobStatus(ctx.Request.Context(), processingEvent.JobID, "failed", nil, err.Error()); updateErr != nil {
				mp.logger.Error("Failed to update job status",
					zap.String("jobID", processingEvent.JobID),
					zap.Error(updateErr))
			}
		}
	}

	return nil
}

func (mp *MediaProcessor) initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize S3 client
	mp.s3Client = s3.NewFromConfig(awsCfg)

	// Initialize MediaConvert client
	mp.mediaConvertClient = mediaconvert.NewFromConfig(awsCfg)
	if mp.mediaConvertEndpoint != "" {
		// Custom endpoint configuration would go here if needed
		mp.logger.Info("using custom MediaConvert endpoint", zap.String("endpoint", mp.mediaConvertEndpoint))
	}

	return nil
}

func (mp *MediaProcessor) processMediaJob(ctx context.Context, event MediaProcessingEvent) error {
	mp.logger.Info("processing media job",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID))

	// Get job details using DynamORM
	job, err := mp.mediaRepo.GetMediaJob(ctx, event.JobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Update job status to processing
	job.SetProcessing()
	if err := mp.mediaRepo.UpdateMediaJob(ctx, job); err != nil {
		mp.logger.Warn("failed to update job status", zap.Error(err))
	}

	// Download original file from S3
	originalData, err := mp.downloadFromS3(ctx, job.S3Key)
	if err != nil {
		return fmt.Errorf("failed to download original: %w", err)
	}

	// Validate file type
	if err := validateFileType(originalData, job.MimeType); err != nil {
		return fmt.Errorf("file type validation failed: %w", err)
	}

	// Process based on media type
	var result ProcessingResult
	mediaType := getMediaTypeFromMime(job.MimeType)

	switch mediaType {
	case "image", "gifv":
		result, err = mp.processImage(ctx, originalData, event, job.ProcessingTasks, job.MimeType)
		if err != nil {
			return fmt.Errorf("failed to process image: %w", err)
		}

	case "video":
		result, err = mp.processVideo(ctx, originalData, event, job.ProcessingTasks)
		if err != nil {
			return fmt.Errorf("failed to process video: %w", err)
		}

	case "audio":
		result, err = mp.processAudio(ctx, originalData, event, job.ProcessingTasks)
		if err != nil {
			return fmt.Errorf("failed to process audio: %w", err)
		}

	default:
		return fmt.Errorf("unsupported media type: %s", mediaType)
	}

	// Update media record with processing results
	if err := mp.updateMediaRecord(ctx, event.MediaID, result); err != nil {
		return fmt.Errorf("failed to update media record: %w", err)
	}

	// Update job as completed
	resultsMap := map[string]any{
		"width":        result.Width,
		"height":       result.Height,
		"duration":     result.Duration,
		"blurhash":     result.Blurhash,
		"preview_url":  result.PreviewURL,
		"sizes":        result.Sizes,
		"processing_job_id": result.ProcessingJobID,
	}
	job.SetCompleted(resultsMap)
	if err := mp.mediaRepo.UpdateMediaJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	mp.logger.Info("media processing completed",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID))

	return nil
}

func (mp *MediaProcessor) processImage(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string, mimeType string) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// Check resources before processing
	monitor := common.GetLambdaMonitor()
	if err := monitor.CheckResources("image-processing-start"); err != nil {
		mp.logger.Error("resource limit approaching", zap.Error(err))
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
			mp.logger.Error("failed to sanitize S3 key",
				zap.String("username", event.Username),
				zap.String("media_id", event.MediaID),
				zap.Error(err))
			continue
		}

		// Upload to S3
		if err := mp.uploadToS3(ctx, s3Key, processed.Data, getMimeTypeFromFormat(processed.Format)); err != nil {
			mp.logger.Warn("failed to upload image size",
				zap.String("size", sizeName),
				zap.Error(err))
			continue
		}

		// Build URL
		url := mp.buildMediaURL(s3Key)

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
		switch task {
		case "exif":
			// EXIF stripping is handled automatically by re-encoding
			mp.logger.Info("EXIF data stripped during processing")
		}
	}

	return result, nil
}

func (mp *MediaProcessor) processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// 1. Get user's media processing config
	config, err := mp.getUserMediaConfig(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user media config", zap.Error(err))
		// Fall back to basic upload
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 2. Check if video processing is enabled
	if !config.VideoProcessingEnabled {
		mp.logger.Info("video processing disabled for user", zap.String("username", event.Username))
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 3. Check user's remaining budget
	remainingBudget, err := mp.getUserRemainingBudget(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user budget", zap.Error(err))
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 4. Estimate cost for this operation
	estimatedCost := estimateVideoCost(len(data))
	if estimatedCost > remainingBudget {
		mp.logger.Warn("user exceeded media budget",
			zap.String("username", event.Username),
			zap.Int64("estimated_cost", estimatedCost),
			zap.Int64("remaining_budget", remainingBudget))

		// Fallback to basic upload only
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 5. Upload to S3 first
	s3Key, err := sanitizeS3Key(event.Username, event.MediaID, "original.mp4")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := mp.uploadToS3(ctx, s3Key, data, "video/mp4"); err != nil {
		return result, fmt.Errorf("failed to upload video: %w", err)
	}

	// 6. Create MediaConvert job for thumbnails and metadata if enabled
	if config.VideoThumbnailsEnabled && mp.mediaConvertRole != "" {
		jobID, err := mp.createMediaConvertJob(ctx, s3Key, event)
		if err != nil {
			mp.logger.Error("failed to create MediaConvert job", zap.Error(err))
			// Continue anyway - video is uploaded
		} else {
			result.ProcessingJobID = jobID
			mp.logger.Info("created MediaConvert job",
				zap.String("job_id", jobID),
				zap.String("media_id", event.MediaID))
		}
	}

	// 7. Track the cost
	if err := mp.trackUserSpend(ctx, event.Username, estimatedCost, "video_processing"); err != nil {
		mp.logger.Warn("failed to track video processing cost", zap.Error(err))
	}

	result.Sizes["original"] = SizeInfo{
		URL:   mp.buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// Extract video metadata before MediaConvert processing
	width, height, duration := extractVideoMetadata(data)
	result.Width = width
	result.Height = height
	result.Duration = duration

	return result, nil
}

func (mp *MediaProcessor) processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string) (ProcessingResult, error) {
	result := ProcessingResult{}

	// 1. Get user's media processing config
	config, err := mp.getUserMediaConfig(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user media config", zap.Error(err))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 2. Check if audio processing is enabled
	if !config.AudioProcessingEnabled {
		mp.logger.Info("audio processing disabled for user", zap.String("username", event.Username))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 3. Check user's remaining budget
	remainingBudget, err := mp.getUserRemainingBudget(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user budget", zap.Error(err))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// 4. Upload original audio
	audioKey, err := sanitizeS3Key(event.Username, event.MediaID, "audio.mp3")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := mp.uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
		return result, fmt.Errorf("failed to upload audio: %w", err)
	}

	result.Sizes = map[string]SizeInfo{
		"original": {
			URL:   mp.buildMediaURL(audioKey),
			S3Key: audioKey,
		},
	}

	// 5. Extract audio duration using dhowden/tag
	duration, err := extractAudioDuration(data)
	if err != nil {
		mp.logger.Warn("failed to extract audio duration", zap.Error(err))
		duration = 0 // fallback to 0 on error
	}
	result.Duration = duration

	// 6. Track the cost (minimal for audio)
	audioCost := int64(100) // $0.0001 for basic audio processing
	if audioCost <= remainingBudget {
		if err := mp.trackUserSpend(ctx, event.Username, audioCost, "audio_processing"); err != nil {
			mp.logger.Warn("failed to track audio processing cost", zap.Error(err))
		}
	}

	// Track S3 operations
	mp.costTracker.TrackS3Put(1)
	mp.costTracker.TrackS3Storage(int64(len(data)))

	mp.logger.Info("audio processing completed",
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username))

	return result, nil
}

func (mp *MediaProcessor) downloadFromS3(ctx context.Context, key string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(mp.bucketName),
		Key:    aws.String(key),
	}

	result, err := mp.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			mp.logger.Warn("failed to close S3 object body", zap.Error(err))
		}
	}()

	// Use common.ReadRequestBody to enforce size limits
	// Use the maximum allowed size across all media types
	maxSize := int64(maxVideoSize) // 50MB is the largest allowed
	data, err := common.ReadRequestBody(result.Body, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}

	return data, nil
}

func (mp *MediaProcessor) uploadToS3(ctx context.Context, key string, data []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:       aws.String(mp.bucketName),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err := mp.s3Client.PutObject(ctx, input)
	return err
}

// updateJobStatus is replaced by direct MediaJob model updates in DynamORM
// This function is kept for compatibility with existing error handling code
func (mp *MediaProcessor) updateJobStatus(ctx context.Context, jobID, status string, result any, errorMsg string) error {
	job, err := mp.mediaRepo.GetMediaJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	switch status {
	case "processing":
		job.SetProcessing()
	case "completed":
		if result != nil {
			if resultMap, ok := result.(map[string]any); ok {
				job.SetCompleted(resultMap)
			} else {
				job.SetCompleted(map[string]any{"result": result})
			}
		} else {
			job.SetCompleted(map[string]any{})
		}
	case "failed":
		job.SetFailed(errorMsg)
	default:
		return fmt.Errorf("unknown status: %s", status)
	}

	return mp.mediaRepo.UpdateMediaJob(ctx, job)
}


func (mp *MediaProcessor) updateMediaRecord(ctx context.Context, mediaID string, result ProcessingResult) error {
	// Get the existing media record
	media, err := mp.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		// Create new media record if it doesn't exist
		media = &models.Media{
			MediaID: mediaID,
			Status:  "processing",
		}
	}

	// Update the media record with processing results
	media.SetProcessed()
	media.Width = result.Width
	media.Height = result.Height
	media.Duration = result.Duration
	media.Blurhash = result.Blurhash

	// Add variants from processing results
	if media.Variants == nil {
		media.Variants = make(map[string]models.MediaVariant)
	}

	for sizeName, sizeInfo := range result.Sizes {
		variant := models.MediaVariant{
			S3Key:       sizeInfo.S3Key,
			CDNUrl:      sizeInfo.URL,
			Width:       sizeInfo.Width,
			Height:      sizeInfo.Height,
			FileSize:    0, // Would need to be calculated or stored
			ContentType: "", // Would need to be determined from the variant
		}
		media.AddVariant(sizeName, variant)
	}

	// Set URLs
	if original, ok := result.Sizes["original"]; ok {
		media.CDNUrl = original.URL
		media.S3Key = original.S3Key
	}

	return mp.mediaRepo.UpdateMedia(ctx, media)
}

func (mp *MediaProcessor) buildMediaURL(s3Key string) string {
	if mp.cdnDomain != "" {
		return fmt.Sprintf("https://%s/%s", mp.cdnDomain, s3Key)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", mp.bucketName, s3Key)
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

func (mp *MediaProcessor) getUserMediaConfig(ctx context.Context, username string) (*MediaConfig, error) {
	// For now, return default config
	// TODO: Implement proper user media config storage with DynamORM models
	return &MediaConfig{
		VideoProcessingEnabled:   true,
		AudioProcessingEnabled:   true,
		VideoThumbnailsEnabled:   true,
		ContentModerationEnabled: false,
		MaxVideoDuration:         300,
		UserBudgetMicros:         5_000_000, // $5 default
	}, nil
}

func (mp *MediaProcessor) getUserRemainingBudget(ctx context.Context, username string) (int64, error) {
	// For now, return default budget
	// TODO: Implement proper user spending tracking with DynamORM models
	config, err := mp.getUserMediaConfig(ctx, username)
	if err != nil {
		return 0, err
	}
	return config.UserBudgetMicros, nil
}

func (mp *MediaProcessor) uploadOriginalOnly(ctx context.Context, data []byte, event MediaProcessingEvent, mimeType string) (ProcessingResult, error) {
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
	if err := mp.uploadToS3(ctx, s3Key, data, mimeType); err != nil {
		return result, fmt.Errorf("failed to upload original: %w", err)
	}

	result.Sizes["original"] = SizeInfo{
		URL:   mp.buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// Track minimal cost (just S3 storage)
	mp.costTracker.TrackS3Put(1)
	mp.costTracker.TrackS3Storage(int64(len(data)))

	return result, nil
}

func (mp *MediaProcessor) trackUserSpend(ctx context.Context, username string, amountMicros int64, category string) error {
	// For now, just log the spend
	// TODO: Implement proper user spending tracking with DynamORM models
	mp.logger.Info("tracking user spend",
		zap.String("username", username),
		zap.Int64("amount_micros", amountMicros),
		zap.String("category", category))
	return nil
}

func estimateVideoCost(sizeBytes int) int64 {
	// MediaConvert: ~$0.024 per minute HD
	// Estimate based on file size (rough approximation)
	estimatedMinutes := float64(sizeBytes) / (5 * 1024 * 1024) // 5MB per minute estimate
	costDollars := estimatedMinutes * 0.024
	return int64(costDollars * 1_000_000) // Convert to microdollars
}

func (mp *MediaProcessor) createMediaConvertJob(ctx context.Context, s3InputKey string, event MediaProcessingEvent) (string, error) {
	if mp.mediaConvertRole == "" {
		return "", fmt.Errorf("MediaConvert role not configured")
	}

	// Define input and output locations
	inputURI := fmt.Sprintf("s3://%s/%s", mp.bucketName, s3InputKey)
	baseOutputKey := fmt.Sprintf("media/%s/%s", event.Username, event.MediaID)
	outputURI := fmt.Sprintf("s3://%s/%s/", mp.bucketName, baseOutputKey)

	// Create job settings for thumbnail extraction and multiple quality variants
	jobSettings := &mctypes.JobSettings{
		Inputs: []mctypes.Input{
			{
				FileInput:     aws.String(inputURI),
				VideoSelector: &mctypes.VideoSelector{},
				AudioSelectors: map[string]mctypes.AudioSelector{
					"Audio Selector 1": {
						DefaultSelection: mctypes.AudioDefaultSelectionDefault,
					},
				},
			},
		},
		OutputGroups: []mctypes.OutputGroup{
			// MP4 output group with multiple qualities
			{
				Name: aws.String("MP4 Group"),
				OutputGroupSettings: &mctypes.OutputGroupSettings{
					Type: mctypes.OutputGroupTypeFileGroupSettings,
					FileGroupSettings: &mctypes.FileGroupSettings{
						Destination: aws.String(outputURI),
					},
				},
				Outputs: []mctypes.Output{
					// 480p output
					{
						NameModifier: aws.String("_480p"),
						VideoDescription: &mctypes.VideoDescription{
							CodecSettings: &mctypes.VideoCodecSettings{
								Codec: mctypes.VideoCodecH264,
								H264Settings: &mctypes.H264Settings{
									Bitrate: int32(1000000), // 1 Mbps
								},
							},
							Width:  int32(854),
							Height: int32(480),
						},
						AudioDescriptions: []mctypes.AudioDescription{
							{
								AudioSourceName: aws.String("Audio Selector 1"),
								CodecSettings: &mctypes.AudioCodecSettings{
									Codec: mctypes.AudioCodecAac,
									AacSettings: &mctypes.AacSettings{
										Bitrate:    int32(128000),
										SampleRate: int32(48000),
									},
								},
							},
						},
						ContainerSettings: &mctypes.ContainerSettings{
							Container: mctypes.ContainerTypeMp4,
						},
					},
					// 720p output
					{
						NameModifier: aws.String("_720p"),
						VideoDescription: &mctypes.VideoDescription{
							CodecSettings: &mctypes.VideoCodecSettings{
								Codec: mctypes.VideoCodecH264,
								H264Settings: &mctypes.H264Settings{
									Bitrate: int32(2500000), // 2.5 Mbps
								},
							},
							Width:  int32(1280),
							Height: int32(720),
						},
						AudioDescriptions: []mctypes.AudioDescription{
							{
								AudioSourceName: aws.String("Audio Selector 1"),
								CodecSettings: &mctypes.AudioCodecSettings{
									Codec: mctypes.AudioCodecAac,
									AacSettings: &mctypes.AacSettings{
										Bitrate:    int32(128000),
										SampleRate: int32(48000),
									},
								},
							},
						},
						ContainerSettings: &mctypes.ContainerSettings{
							Container: mctypes.ContainerTypeMp4,
						},
					},
				},
			},
			// Thumbnail output group
			{
				Name: aws.String("Thumbnail Group"),
				OutputGroupSettings: &mctypes.OutputGroupSettings{
					Type: mctypes.OutputGroupTypeFileGroupSettings,
					FileGroupSettings: &mctypes.FileGroupSettings{
						Destination: aws.String(outputURI),
					},
				},
				Outputs: []mctypes.Output{
					{
						NameModifier: aws.String("_thumb"),
						VideoDescription: &mctypes.VideoDescription{
							CodecSettings: &mctypes.VideoCodecSettings{
								Codec: mctypes.VideoCodecFrameCapture,
								FrameCaptureSettings: &mctypes.FrameCaptureSettings{
									FramerateNumerator:   int32(1),
									FramerateDenominator: int32(10), // 1 frame every 10 seconds
									MaxCaptures:          int32(1),  // Just one thumbnail
									Quality:              int32(80),
								},
							},
							Width:  int32(320),
							Height: int32(240),
						},
						ContainerSettings: &mctypes.ContainerSettings{
							Container: mctypes.ContainerTypeRaw,
						},
					},
				},
			},
		},
	}

	// Create the job
	createJobInput := &mediaconvert.CreateJobInput{
		Queue:    aws.String(mp.mediaConvertQueue),
		Role:     aws.String(mp.mediaConvertRole),
		Settings: jobSettings,
		UserMetadata: map[string]string{
			"username": event.Username,
			"media_id": event.MediaID,
			"job_id":   event.JobID,
		},
	}

	result, err := mp.mediaConvertClient.CreateJob(ctx, createJobInput)
	if err != nil {
		return "", fmt.Errorf("failed to create MediaConvert job: %w", err)
	}

	return aws.ToString(result.Job.Id), nil
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
		return fmt.Errorf("file type not allowed: %s", detectedType)
	}

	// Warn if claimed type doesn't match detected type
	if claimedMimeType != "" && claimedMimeType != detectedType {
		// MIME type mismatch - detected vs claimed
		// Log would be: claimed=%s detected=%s, claimedMimeType, detectedType
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

// extractAudioDuration extracts duration from audio data using dhowden/tag
func extractAudioDuration(data []byte) (int, error) {
	// Use dhowden/tag to parse audio metadata
	metadata, err := tag.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("failed to read audio metadata: %w", err)
	}

	// Get track and disc information from metadata
	track, _ := metadata.Track()
	if track != 0 {
		// This is track number, not duration - fallback to format-specific parsing
	}

	// For MP3 files, calculate duration from file size and bitrate if available
	// This is a simplified approach - for production, consider using a more robust library
	if len(data) > 1000 {
		// Very rough estimation for MP3: assume 128kbps average
		// Real implementation would parse frame headers
		estimatedSeconds := float64(len(data)) / (128 * 1000 / 8) // 128kbps in bytes/sec
		if estimatedSeconds > 0 && estimatedSeconds < 7200 {      // reasonable range (0-2 hours)
			return int(estimatedSeconds * 1000), nil // return milliseconds
		}
	}

	return 0, fmt.Errorf("unable to determine audio duration")
}

// extractVideoMetadata extracts basic video metadata from video data
func extractVideoMetadata(data []byte) (width, height, duration int) {
	// This is a simplified implementation
	// For production, use ffprobe or a video parsing library

	// Check for MP4 format markers
	if len(data) > 100 && bytes.Contains(data[:100], []byte("ftyp")) {
		// Look for moov atom which contains metadata
		if moovIndex := bytes.Index(data, []byte("moov")); moovIndex > 0 {
			// This is a very simplified approach
			// Real implementation would parse MP4 atoms properly

			// Set some reasonable defaults based on common video sizes
			// In production, parse the actual video stream headers
			width = 1920 // assume HD by default
			height = 1080
			duration = 30000 // 30 seconds default

			// Try to find tkhd atom for track header with dimensions
			if tkhdIndex := bytes.Index(data[moovIndex:], []byte("tkhd")); tkhdIndex > 0 {
				// Parse track header for actual dimensions
				// This is placeholder - real parsing would extract from binary data
				// For now, return HD defaults
			}
		}
	}

	// For other formats or if parsing fails, return reasonable defaults
	if width == 0 {
		width = 854  // 480p width
		height = 480 // 480p height
		duration = 0 // unknown duration
	}

	return width, height, duration
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
