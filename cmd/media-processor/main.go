// Package main implements the media-processor Lambda function for processing media attachments.
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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dhowden/tag"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/media"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// Media type constants
const (
	mediaTypeImage = "image"
	mediaTypeVideo = "video"
	mediaTypeAudio = "audio"
	mediaTypeGifv  = "gifv"
)

// Media processing status constants
const (
	MediaStatusProcessing = "processing"
	MediaStatusCompleted  = "completed"
	MediaStatusFailed     = "failed"
)

// Media file extensions
const (
	extJPG  = ".jpg"
	extPNG  = ".png"
	extGIF  = ".gif"
	extWebP = ".webp"
)

// Media MIME types
const (
	mimeJPEG = "image/jpeg"
	mimePNG  = "image/png"
	mimeWebP = "image/webp"
)

// MediaProcessor handles media processing from SQS messages using Lift and DynamORM
type MediaProcessor struct {
	db                   core.DB
	repos                storageCore.RepositoryStorage
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
	// Global transcoding cost tracker with current AWS pricing
	transcodingCosts = TranscodingCostTracker{
		SDCostPerMinute:    15000, // $0.015 per minute
		HDCostPerMinute:    30000, // $0.030 per minute
		UHDCostPerMinute:   45000, // $0.045 per minute
		LambdaGBSecondCost: 17,    // $0.0000166667 per GB-second
		S3StorageCost:      23000, // $0.023 per GB/month
		CloudFrontCost:     85000, // $0.085 per GB
		RekognitionCost:    1000,  // $0.001 per image
		ThumbnailCost:      100,   // $0.0001 per thumbnail
	}
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

// TranscodingCostTracker tracks detailed transcoding costs
type TranscodingCostTracker struct {
	// MediaConvert costs by quality tier (in microdollars per minute)
	SDCostPerMinute  int64 // $0.015 per minute = 15,000 microdollars
	HDCostPerMinute  int64 // $0.030 per minute = 30,000 microdollars
	UHDCostPerMinute int64 // $0.045 per minute = 45,000 microdollars

	// Lambda audio processing costs (in microdollars per GB-second)
	LambdaGBSecondCost int64 // $0.0000166667 per GB-second = 16.6667 microdollars

	// S3 storage costs (in microdollars per GB per month)
	S3StorageCost int64 // $0.023 per GB/month = 23,000 microdollars

	// CloudFront delivery costs (in microdollars per GB)
	CloudFrontCost int64 // $0.085 per GB = 85,000 microdollars

	// Rekognition costs (in microdollars per image)
	RekognitionCost int64 // $0.001 per image = 1,000 microdollars

	// Thumbnail generation costs (in microdollars per thumbnail)
	ThumbnailCost int64 // $0.0001 per thumbnail = 100 microdollars
}

// TranscodingJobMetrics holds metrics for a specific transcoding job
type TranscodingJobMetrics struct {
	JobID            string            `json:"job_id"`
	MediaID          string            `json:"media_id"`
	Username         string            `json:"username"`
	InputFormat      string            `json:"input_format"`
	InputSize        int64             `json:"input_size"`
	InputDuration    int64             `json:"input_duration"`  // milliseconds
	OutputVariants   map[string]string `json:"output_variants"` // quality -> format
	OutputSizes      map[string]int64  `json:"output_sizes"`    // quality -> size in bytes
	ProcessingTimeMs int64             `json:"processing_time_ms"`
	TotalCostMicros  int64             `json:"total_cost_micros"`
	CostBreakdown    map[string]int64  `json:"cost_breakdown"` // service -> cost
	StartedAt        time.Time         `json:"started_at"`
	CompletedAt      *time.Time        `json:"completed_at,omitempty"`
	Status           string            `json:"status"`
	ErrorMessage     string            `json:"error_message,omitempty"`
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
	common.ImageJPEG:      true,
	common.ImageJPG:       true,
	common.ImagePNG:       true,
	common.ImageGIF:       true,
	common.ImageWEBP:      true,
	common.VideoMP4:       true,
	common.VideoWEBM:      true,
	common.VideoQuickTime: true,
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

	// Load AWS config for CloudWatch
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Initialize repository factory
	repos, err := factory.NewRepositoryFactory(db, cfg.DynamoTableName, awsConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize media repository directly for backward compatibility
	mediaRepo := repositories.NewMediaRepository(db, cfg.DynamoTableName, logger)

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
		repos:                repos,
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
	_ = app.SQS("media-processing", func(ctx *lift.Context) error {
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
			if updateErr := mp.updateJobStatus(ctx.Request.Context(), processingEvent.JobID, MediaStatusFailed, nil, err.Error()); updateErr != nil {
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
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username))

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

	// Get user's media configuration and validate quotas
	userConfig := mp.getUserMediaConfig(ctx, event.Username)

	// Download original file from S3
	originalData, err := mp.downloadFromS3(ctx, job.S3Key)
	if err != nil {
		return fmt.Errorf("failed to download original: %w", err)
	}

	// Validate file type and size against user's configuration
	if err := mp.validateFileForUser(originalData, job.MimeType, userConfig, event.Username); err != nil {
		return fmt.Errorf("file validation failed: %w", err)
	}

	// Check user's remaining budget before processing
	remainingBudget := mp.getUserRemainingBudget(ctx, event.Username)

	// Estimate cost for processing
	estimatedCost := mp.estimateProcessingCost(originalData, job.MimeType)
	if remainingBudget > 0 && estimatedCost > remainingBudget {
		mp.logger.Warn("user has insufficient budget for processing",
			zap.String("username", event.Username),
			zap.Int64("estimated_cost", estimatedCost),
			zap.Int64("remaining_budget", remainingBudget))

		// Fall back to basic upload only
		result, err := mp.uploadOriginalOnly(ctx, originalData, event, job.MimeType)
		if err != nil {
			return fmt.Errorf("failed to upload original file: %w", err)
		}

		// Update media record and job
		if err := mp.updateMediaRecord(ctx, event.MediaID, result); err != nil {
			return fmt.Errorf("failed to update media record: %w", err)
		}

		resultsMap := map[string]any{
			"width":       result.Width,
			"height":      result.Height,
			"duration":    result.Duration,
			"blurhash":    result.Blurhash,
			"preview_url": result.PreviewURL,
			"sizes":       result.Sizes,
			"note":        "Processing skipped due to budget limits",
		}
		job.SetCompleted(resultsMap)
		if err := mp.mediaRepo.UpdateMediaJob(ctx, job); err != nil {
			return fmt.Errorf("failed to update job status: %w", err)
		}

		return nil
	}

	// Process based on media type
	var result ProcessingResult
	mediaType := getMediaTypeFromMime(job.MimeType)

	switch mediaType {
	case mediaTypeImage, mediaTypeGifv:
		result, err = mp.processImage(ctx, originalData, event, job.ProcessingTasks, job.MimeType)
		if err != nil {
			return fmt.Errorf("failed to process image: %w", err)
		}

	case mediaTypeVideo:
		result, err = mp.processVideo(ctx, originalData, event, job.ProcessingTasks)
		if err != nil {
			return fmt.Errorf("failed to process video: %w", err)
		}

	case mediaTypeAudio:
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
		"width":             result.Width,
		"height":            result.Height,
		"duration":          result.Duration,
		"blurhash":          result.Blurhash,
		"preview_url":       result.PreviewURL,
		"sizes":             result.Sizes,
		"processing_job_id": result.ProcessingJobID,
	}
	job.SetCompleted(resultsMap)
	if err := mp.mediaRepo.UpdateMediaJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	mp.logger.Info("media processing completed",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username))

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

	// Track comprehensive image processing costs
	imageMetrics := &TranscodingJobMetrics{
		JobID:          event.JobID,
		MediaID:        event.MediaID,
		Username:       event.Username,
		InputFormat:    mimeType,
		InputSize:      int64(len(data)),
		OutputVariants: make(map[string]string),
		OutputSizes:    make(map[string]int64),
		CostBreakdown:  make(map[string]int64),
		StartedAt:      time.Now(),
		Status:         MediaStatusCompleted,
	}

	// Calculate processing costs by operation
	imageProcessingCost := int64(200) // $0.0002 for image processing
	imageMetrics.CostBreakdown["lambda_processing"] = imageProcessingCost

	// Calculate storage costs for all variants
	totalStorageBytes := int64(len(data)) // Original data
	for sizeName, processed := range processedImages {
		variantSize := int64(len(processed.Data))
		totalStorageBytes += variantSize
		imageMetrics.OutputVariants[sizeName] = processed.Format
		imageMetrics.OutputSizes[sizeName] = variantSize
	}

	// S3 costs
	uploadCost := mp.calculateS3PutCost(totalStorageBytes) * int64(len(processedImages))
	storageCost := mp.calculateS3StorageCost(totalStorageBytes)
	imageMetrics.CostBreakdown["s3_upload"] = uploadCost
	imageMetrics.CostBreakdown["s3_storage"] = storageCost

	// Calculate total costs
	imageMetrics.TotalCostMicros = 0
	for _, cost := range imageMetrics.CostBreakdown {
		imageMetrics.TotalCostMicros += cost
	}

	// Track detailed image processing costs
	mp.trackTranscodingCosts(ctx, imageMetrics)

	// Update user's storage usage
	if err := mp.updateStorageUsageForUser(ctx, event.Username, totalStorageBytes); err != nil {
		mp.logger.Warn("failed to update storage usage", zap.Error(err))
	}

	mp.logger.Info("image processing completed with cost tracking",
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username),
		zap.Int64("total_cost_micros", imageMetrics.TotalCostMicros),
		zap.Int("variants_generated", len(processedImages)))

	return result, nil
}

func (mp *MediaProcessor) processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string) (ProcessingResult, error) {
	result := ProcessingResult{
		Sizes: make(map[string]SizeInfo),
	}

	// Initialize transcoding job metrics
	jobMetrics := &TranscodingJobMetrics{
		JobID:          event.JobID,
		MediaID:        event.MediaID,
		Username:       event.Username,
		InputFormat:    "video/mp4",
		InputSize:      int64(len(data)),
		OutputVariants: make(map[string]string),
		OutputSizes:    make(map[string]int64),
		CostBreakdown:  make(map[string]int64),
		StartedAt:      time.Now(),
		Status:         MediaStatusProcessing,
	}

	// 1. Get user's media processing config
	config := mp.getUserMediaConfig(ctx, event.Username)

	// 2. Check if video processing is enabled
	if !config.VideoProcessingEnabled {
		mp.logger.Info("video processing disabled for user", zap.String("username", event.Username))
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 3. Check user's remaining budget
	remainingBudget := mp.getUserRemainingBudget(ctx, event.Username)

	// 4. Extract video metadata for cost estimation
	width, height, duration := mp.extractVideoMetadata(data)
	jobMetrics.InputDuration = int64(duration)
	result.Width = width
	result.Height = height
	result.Duration = duration

	// 5. Estimate comprehensive transcoding costs
	transcodingPlan, totalEstimatedCost := mp.estimateTranscodingCosts(jobMetrics, config)
	if totalEstimatedCost > remainingBudget {
		mp.logger.Warn("user exceeded media budget for transcoding",
			zap.String("username", event.Username),
			zap.Int64("estimated_cost", totalEstimatedCost),
			zap.Int64("remaining_budget", remainingBudget),
			zap.Any("transcoding_plan", transcodingPlan))

		// Fallback to basic upload only
		return mp.uploadOriginalOnly(ctx, data, event, "video/mp4")
	}

	// 6. Upload original to S3 first
	s3Key, err := sanitizeS3Key(event.Username, event.MediaID, "original.mp4")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}

	// Track S3 upload cost
	s3UploadCost := mp.calculateS3PutCost(int64(len(data)))
	jobMetrics.CostBreakdown["s3_upload"] = s3UploadCost

	if err := mp.uploadToS3(ctx, s3Key, data, "video/mp4"); err != nil {
		return result, fmt.Errorf("failed to upload video: %w", err)
	}

	// Track S3 storage cost (monthly, prorated)
	s3StorageCost := mp.calculateS3StorageCost(int64(len(data)))
	jobMetrics.CostBreakdown["s3_storage"] = s3StorageCost

	// 7. Create MediaConvert job for transcoding if enabled
	var mediaConvertJobID string
	if config.VideoThumbnailsEnabled && mp.mediaConvertRole != "" {
		mediaConvertJobID, err = mp.createEnhancedMediaConvertJob(ctx, s3Key, event, transcodingPlan)
		if err != nil {
			mp.logger.Error("failed to create MediaConvert job", zap.Error(err))
			jobMetrics.Status = MediaStatusFailed
			jobMetrics.ErrorMessage = err.Error()
			// Continue anyway - video is uploaded
		} else {
			result.ProcessingJobID = mediaConvertJobID
			jobMetrics.CostBreakdown["mediaconvert"] = transcodingPlan.MediaConvertCost
			mp.logger.Info("created enhanced MediaConvert job",
				zap.String("job_id", mediaConvertJobID),
				zap.String("media_id", event.MediaID),
				zap.Int64("estimated_cost", transcodingPlan.MediaConvertCost))
		}
	}

	// 8. Generate thumbnails if requested
	if sliceContains(tasks, "thumbnails") {
		thumbnailCost := mp.generateVideoThumbnails(ctx, event, transcodingPlan)
		jobMetrics.CostBreakdown["thumbnails"] = thumbnailCost
	}

	// 9. Calculate total actual costs
	jobMetrics.TotalCostMicros = 0
	for _, cost := range jobMetrics.CostBreakdown {
		jobMetrics.TotalCostMicros += cost
	}

	// 10. Track detailed transcoding costs
	mp.trackTranscodingCosts(ctx, jobMetrics)

	result.Sizes["original"] = SizeInfo{
		URL:   mp.buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// 11. Update user's storage usage (original + estimated transcoded variants)
	estimatedVariantSize := mp.estimateVariantStorageSize(int64(len(data)), transcodingPlan)
	if err := mp.updateStorageUsageForUser(ctx, event.Username, int64(len(data))+estimatedVariantSize); err != nil {
		mp.logger.Warn("failed to update storage usage", zap.Error(err))
	}

	// Mark job as completed
	now := time.Now()
	jobMetrics.CompletedAt = &now
	jobMetrics.ProcessingTimeMs = now.Sub(jobMetrics.StartedAt).Milliseconds()
	if jobMetrics.Status == MediaStatusProcessing {
		jobMetrics.Status = MediaStatusCompleted
	}

	mp.logger.Info("video transcoding completed",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username),
		zap.Int64("total_cost_micros", jobMetrics.TotalCostMicros),
		zap.Int64("processing_time_ms", jobMetrics.ProcessingTimeMs))

	return result, nil
}

func (mp *MediaProcessor) processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string) (ProcessingResult, error) {
	// Use the enhanced audio processing with detailed cost tracking
	return mp.processAudioWithCostTracking(ctx, data, event, tasks)
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
	case MediaStatusProcessing:
		job.SetProcessing()
	case MediaStatusCompleted:
		if result != nil {
			if resultMap, ok := result.(map[string]any); ok {
				job.SetCompleted(resultMap)
			} else {
				job.SetCompleted(map[string]any{"result": result})
			}
		} else {
			job.SetCompleted(map[string]any{})
		}
	case MediaStatusFailed:
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
			Status:  MediaStatusProcessing,
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
			FileSize:    0,  // Would need to be calculated or stored
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
		if mimeType == common.ImageGIF {
			return mediaTypeGifv
		}
		return mediaTypeImage
	} else if strings.HasPrefix(mimeType, "video/") {
		return mediaTypeVideo
	} else if strings.HasPrefix(mimeType, "audio/") {
		return mediaTypeAudio
	}
	return "unknown"
}

func getExtensionFromProcessedFormat(format string) string {
	switch format {
	case "jpeg":
		return extJPG
	case "png":
		return extPNG
	case "gif":
		return extGIF
	case "webp":
		return extWebP
	default:
		return extJPG
	}
}

func getMimeTypeFromFormat(format string) string {
	switch format {
	case "jpeg":
		return mimeJPEG
	case "png":
		return mimePNG
	case "gif":
		return common.ImageGIF
	case "webp":
		return mimeWebP
	default:
		return mimeJPEG
	}
}

// Helper functions for cost-aware processing

func (mp *MediaProcessor) getUserMediaConfig(ctx context.Context, username string) *MediaConfig {
	// First, try to get the user ID from username
	// In a real implementation, you'd want to resolve username to userID
	// For now, we'll use username as userID for simplicity
	userID := username

	// Get user media configuration from repository
	userConfig, err := mp.mediaRepo.GetUserMediaConfig(ctx, userID)
	if err != nil {
		// If config doesn't exist, create default config
		if strings.Contains(err.Error(), "not found") {
			defaultConfig := &models.UserMediaConfig{
				UserID:   userID,
				Username: username,
				PlanTier: "free",
			}

			if createErr := mp.mediaRepo.CreateUserMediaConfig(ctx, defaultConfig); createErr != nil {
				mp.logger.Error("failed to create default user media config",
					zap.String("username", username),
					zap.Error(createErr))
				// Fall back to in-memory default
				return mp.getDefaultMediaConfig()
			}
			userConfig = defaultConfig
		} else {
			mp.logger.Error("failed to get user media config", zap.Error(err))
			return mp.getDefaultMediaConfig()
		}
	}

	// Convert to MediaConfig struct
	return &MediaConfig{
		VideoProcessingEnabled:   userConfig.VideoProcessingEnabled,
		AudioProcessingEnabled:   userConfig.AudioProcessingEnabled,
		VideoThumbnailsEnabled:   userConfig.VideoThumbnailsEnabled,
		ContentModerationEnabled: userConfig.ContentModerationEnabled,
		MaxVideoDuration:         userConfig.MaxVideoDuration,
		UserBudgetMicros:         userConfig.MonthlyBudgetMicros,
	}
}

func (mp *MediaProcessor) getDefaultMediaConfig() *MediaConfig {
	return &MediaConfig{
		VideoProcessingEnabled:   true,
		AudioProcessingEnabled:   true,
		VideoThumbnailsEnabled:   true,
		ContentModerationEnabled: false,
		MaxVideoDuration:         300,
		UserBudgetMicros:         5_000_000, // $5 default
	}
}

func (mp *MediaProcessor) getUserRemainingBudget(ctx context.Context, username string) int64 {
	// Get user's media configuration to get budget limits
	config := mp.getUserMediaConfig(ctx, username)

	// Get current month's spending
	now := time.Now()
	currentPeriod := now.Format(common.MonthFormat) // YYYY-MM format
	userID := username                     // Simplification - in reality you'd resolve username to userID

	spending, err := mp.mediaRepo.GetMediaSpending(ctx, userID, currentPeriod)
	if err != nil {
		// If no spending record exists, user has full budget available
		if strings.Contains(err.Error(), "not found") {
			return config.UserBudgetMicros
		}
		mp.logger.Error("failed to get user spending",
			zap.String("username", username),
			zap.Error(err))
		// Return full budget on error to avoid blocking users
		return config.UserBudgetMicros
	}

	// Calculate remaining budget
	remaining := config.UserBudgetMicros - spending.TotalSpendMicros
	if remaining < 0 {
		return 0
	}

	return remaining
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
	case mimeJPEG:
		ext = extJPG
	case mimePNG:
		ext = extPNG
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

func estimateVideoCost(sizeBytes int) int64 {
	// MediaConvert: ~$0.024 per minute HD
	// Estimate based on file size (rough approximation)
	estimatedMinutes := float64(sizeBytes) / (5 * 1024 * 1024) // 5MB per minute estimate
	costDollars := estimatedMinutes * 0.024
	return int64(costDollars * 1_000_000) // Convert to microdollars
}

// TranscodingPlan represents a detailed plan for transcoding operations
type TranscodingPlan struct {
	QualityLevels      []string         `json:"quality_levels"`       // ["480p", "720p", "1080p"]
	ExpectedOutputs    map[string]int64 `json:"expected_outputs"`     // quality -> expected file size
	MediaConvertCost   int64            `json:"mediaconvert_cost"`    // Total MediaConvert cost
	ThumbnailCount     int              `json:"thumbnail_count"`      // Number of thumbnails to generate
	ThumbnailCost      int64            `json:"thumbnail_cost"`       // Total thumbnail generation cost
	StorageCost        int64            `json:"storage_cost"`         // S3 storage cost for variants
	AnalysisEnabled    bool             `json:"analysis_enabled"`     // Whether to run Rekognition analysis
	AnalysisCost       int64            `json:"analysis_cost"`        // Rekognition analysis cost
	TotalEstimatedCost int64            `json:"total_estimated_cost"` // Sum of all costs
}

// estimateTranscodingCosts creates a detailed transcoding plan and cost estimate
func (mp *MediaProcessor) estimateTranscodingCosts(metrics *TranscodingJobMetrics, config *MediaConfig) (*TranscodingPlan, int64) {
	plan := &TranscodingPlan{
		QualityLevels:   []string{},
		ExpectedOutputs: make(map[string]int64),
	}

	// Determine quality levels based on input resolution and user config
	_, height := getResolutionFromMetrics(metrics)
	durationMinutes := float64(metrics.InputDuration) / (1000 * 60) // Convert ms to minutes

	// Always include original quality, then add transcoded qualities
	if height >= 2160 { // 4K input
		plan.QualityLevels = []string{"2160p", "1080p", "720p", "480p"}
	} else if height >= 1080 { // 1080p input
		plan.QualityLevels = []string{"1080p", "720p", "480p"}
	} else if height >= 720 { // 720p input
		plan.QualityLevels = []string{"720p", "480p"}
	} else { // Lower resolution input
		plan.QualityLevels = []string{"480p"}
	}

	// Calculate MediaConvert costs by quality level
	for _, quality := range plan.QualityLevels {
		var costPerMinute int64
		var expectedSizeRatio float64

		switch quality {
		case "2160p": // 4K/UHD
			costPerMinute = transcodingCosts.UHDCostPerMinute
			expectedSizeRatio = 1.0 // Keep same size
		case "1080p": // HD
			costPerMinute = transcodingCosts.HDCostPerMinute
			expectedSizeRatio = 0.6 // 60% of original
		case "720p": // HD
			costPerMinute = transcodingCosts.HDCostPerMinute
			expectedSizeRatio = 0.4 // 40% of original
		case "480p": // SD
			costPerMinute = transcodingCosts.SDCostPerMinute
			expectedSizeRatio = 0.25 // 25% of original
		default:
			costPerMinute = transcodingCosts.HDCostPerMinute
			expectedSizeRatio = 0.5
		}

		// Calculate cost for this quality level
		qualityCost := int64(float64(costPerMinute) * durationMinutes)
		plan.MediaConvertCost += qualityCost

		// Estimate output file size
		expectedSize := int64(float64(metrics.InputSize) * expectedSizeRatio)
		plan.ExpectedOutputs[quality] = expectedSize
	}

	// Calculate thumbnail costs
	if config.VideoThumbnailsEnabled {
		// Generate thumbnails at different time intervals
		plan.ThumbnailCount = int(durationMinutes) + 1 // One thumbnail per minute + preview
		if plan.ThumbnailCount > 10 {
			plan.ThumbnailCount = 10 // Cap at 10 thumbnails
		}
		plan.ThumbnailCost = int64(plan.ThumbnailCount) * transcodingCosts.ThumbnailCost
	}

	// Calculate storage costs for all variants
	totalVariantSize := int64(0)
	for _, size := range plan.ExpectedOutputs {
		totalVariantSize += size
	}
	plan.StorageCost = mp.calculateS3StorageCost(totalVariantSize)

	// Calculate analysis costs if enabled
	if config.ContentModerationEnabled {
		plan.AnalysisEnabled = true
		// Analyze preview thumbnail + sample frames
		analysisImages := int64(plan.ThumbnailCount + 3) // Thumbnails + 3 sample frames
		plan.AnalysisCost = analysisImages * transcodingCosts.RekognitionCost
	}

	// Sum up total estimated cost
	plan.TotalEstimatedCost = plan.MediaConvertCost + plan.ThumbnailCost + plan.StorageCost + plan.AnalysisCost

	mp.logger.Debug("estimated transcoding costs",
		zap.String("media_id", metrics.MediaID),
		zap.Float64("duration_minutes", durationMinutes),
		zap.Strings("quality_levels", plan.QualityLevels),
		zap.Int64("mediaconvert_cost", plan.MediaConvertCost),
		zap.Int64("thumbnail_cost", plan.ThumbnailCost),
		zap.Int64("storage_cost", plan.StorageCost),
		zap.Int64("analysis_cost", plan.AnalysisCost),
		zap.Int64("total_estimated_cost", plan.TotalEstimatedCost))

	return plan, plan.TotalEstimatedCost
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

	// Check if claimed type matches detected type
	if claimedMimeType != "" && claimedMimeType != detectedType {
		return fmt.Errorf("claimed MIME type %s does not match detected type %s", claimedMimeType, detectedType)
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
		if mimeType == common.ImageGIF {
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
		// Found track number in metadata, using format-specific parsing
		_ = track // Use track number for future processing
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

// extractVideoMetadata extracts real video metadata from video data using MP4 atom parsing
func (mp *MediaProcessor) extractVideoMetadata(data []byte) (width, height, duration int) {
	// Use the new comprehensive video metadata parser
	metadata, err := media.ParseVideoMetadata(data)
	if err != nil {
		// If parsing fails, provide reasonable defaults based on file size
		mp.logger.Warn("failed to parse video metadata, using defaults", zap.Error(err))
		
		sizeMB := len(data) / (1024 * 1024)
		switch {
		case sizeMB > 100: // Large file, likely HD or higher
			return 1920, 1080, 60000 // 1 minute default
		case sizeMB > 50: // Medium file, likely 720p
			return 1280, 720, 45000 // 45 seconds default
		default: // Small file, likely SD
			return 854, 480, 30000 // 30 seconds default
		}
	}

	mp.logger.Debug("extracted video metadata",
		zap.Int("width", metadata.Width),
		zap.Int("height", metadata.Height),
		zap.Int("duration_ms", metadata.Duration),
		zap.Float64("duration_seconds", metadata.DurationSeconds),
		zap.String("video_codec", metadata.VideoCodec),
		zap.String("audio_codec", metadata.AudioCodec),
		zap.Bool("has_video", metadata.HasVideo),
		zap.Bool("has_audio", metadata.HasAudio),
		zap.Int64("bitrate", metadata.Bitrate),
		zap.Float64("frame_rate", metadata.FrameRate))

	// Return the parsed metadata
	return metadata.Width, metadata.Height, metadata.Duration
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

// validateFileForUser validates a file against user's media configuration
func (mp *MediaProcessor) validateFileForUser(data []byte, mimeType string, config *MediaConfig, username string) error {
	// First do basic file type validation
	if err := validateFileType(data, mimeType); err != nil {
		return err
	}

	// Check file size against user's limits
	fileSize := int64(len(data))
	var userMaxSize int64

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		userMaxSize = maxImageSize // Use global defaults for now
	case strings.HasPrefix(mimeType, "video/"):
		userMaxSize = maxVideoSize
		// Check video duration limit if this is a video
		if config.MaxVideoDuration > 0 {
			// For now, we'll do a rough size-based check
			// Real implementation would parse video metadata
			estimatedDuration := estimateVideoDurationFromSize(fileSize)
			if estimatedDuration > config.MaxVideoDuration {
				return fmt.Errorf("video duration exceeds user limit of %d seconds", config.MaxVideoDuration)
			}
		}
	case strings.HasPrefix(mimeType, "audio/"):
		userMaxSize = maxAudioSize
	default:
		return fmt.Errorf("unsupported media type: %s", mimeType)
	}

	if fileSize > userMaxSize {
		return fmt.Errorf("file size %d bytes exceeds user limit of %d bytes", fileSize, userMaxSize)
	}

	mp.logger.Debug("file validation passed",
		zap.String("username", username),
		zap.String("mime_type", mimeType),
		zap.Int64("file_size", fileSize),
		zap.Int64("user_max_size", userMaxSize))

	return nil
}

// estimateProcessingCost estimates the cost of processing a media file
func (mp *MediaProcessor) estimateProcessingCost(data []byte, mimeType string) int64 {
	fileSize := int64(len(data))

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		// Image processing: ~$0.0001 per image
		return 100 // 100 microdollars = $0.0001
	case strings.HasPrefix(mimeType, "video/"):
		// Video processing: estimate based on size and MediaConvert pricing
		return estimateVideoCost(int(fileSize))
	case strings.HasPrefix(mimeType, "audio/"):
		// Audio processing: minimal cost
		return 50 // 50 microdollars = $0.00005
	default:
		return 100 // Default minimal cost
	}
}

// estimateVideoDurationFromSize provides a rough estimate of video duration from file size
func estimateVideoDurationFromSize(sizeBytes int64) int {
	// Very rough estimation: assume average bitrate of 1 Mbps
	// This is just for quota checking - real implementation would parse video metadata
	const avgBitrateBytesPerSecond = 125000 // 1 Mbps in bytes/second

	estimatedSeconds := int(sizeBytes / avgBitrateBytesPerSecond)
	if estimatedSeconds < 1 {
		estimatedSeconds = 1
	}

	return estimatedSeconds
}

// updateStorageUsageForUser updates a user's storage usage statistics
func (mp *MediaProcessor) updateStorageUsageForUser(ctx context.Context, username string, additionalBytes int64) error {
	userID := username // Simplification

	// Get user's media config
	config, err := mp.mediaRepo.GetUserMediaConfig(ctx, userID)
	if err != nil {
		// If config doesn't exist, skip storage tracking
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}

	// Update storage usage
	config.UpdateStorageUsage(additionalBytes)

	// Save updated config
	return mp.mediaRepo.UpdateUserMediaConfig(ctx, config)
}
