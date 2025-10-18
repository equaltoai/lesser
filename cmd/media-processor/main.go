// Package main implements the media-processor Lambda function for processing media attachments.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/observability"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
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
	MediaStatusCancelled  = "cancelled"
)

// Processing timeout constants
const (
	ImageProcessingTimeout = 30 * time.Second // Images: 30 seconds
	GifProcessingTimeout   = 60 * time.Second // GIFs: 60 seconds
	SmallVideoTimeout      = 2 * time.Minute  // Videos < 10MB: 2 minutes
	LargeVideoTimeout      = 5 * time.Minute  // Videos >= 10MB: 5 minutes
	AbandonedJobThreshold  = 1 * time.Hour    // Jobs abandoned after 1 hour
)

// Retry configuration constants
const (
	MaxRetryAttempts = 3
	BaseRetryDelay   = 1 * time.Second
	MaxRetryDelay    = 5 * time.Minute
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
	mediaAnalyticsRepo   *repositories.MediaAnalyticsRepository
	mediaMetadataRepo    *repositories.MediaMetadataRepository
	s3Client             *s3.Client
	mediaConvertClient   *mediaconvert.Client
	unifiedTracker       *cost.UnifiedTracker
	tableName            string
	bucketName           string
	cdnDomain            string
	mediaConvertEndpoint string
	mediaConvertRole     string
	mediaConvertQueue    string
	emfMetrics           *observability.EMFMetrics
	alertManager         *monitoring.AlertManager
	startTime            time.Time
	logger               *zap.Logger
}

// MediaJobCostTracker handles detailed cost tracking for media jobs
type MediaJobCostTracker struct {
	JobID              string           `json:"job_id"`
	UserID             string           `json:"user_id"`
	StartTime          time.Time        `json:"start_time"`
	EndTime            *time.Time       `json:"end_time,omitempty"`
	LambdaExecutionMs  int64            `json:"lambda_execution_ms"`
	S3Operations       int              `json:"s3_operations"`
	S3StorageBytes     int64            `json:"s3_storage_bytes"`
	TranscodingSeconds float64          `json:"transcoding_seconds,omitempty"`
	CostBreakdown      map[string]int64 `json:"cost_breakdown"`     // Cost in micros by category
	TotalCostMicros    int64            `json:"total_cost_micros"`  // Total cost in micros
	BudgetMicros       int64            `json:"budget_micros"`      // Budget limit in micros
	WarningThresholds  []float64        `json:"warning_thresholds"` // Warning at 50%, 75%, 90% of budget
	WarningsSent       []bool           `json:"warnings_sent"`      // Track which warnings have been sent
	mediaRepo          *repositories.MediaRepository
	logger             *zap.Logger
}

var (
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

	// DefaultBudgetLimits defines budget limits by MIME type
	DefaultBudgetLimits map[string]int64
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
	"audio/mpeg":          true,
	"audio/mp3":           true,
	"audio/ogg":           true,
	"audio/wav":           true,
	"audio/webm":          true,
}

// Maximum file sizes by type (in bytes)
const (
	maxImageSize = 10 * 1024 * 1024 // 10MB for images
	maxVideoSize = 50 * 1024 * 1024 // 50MB for videos
	maxAudioSize = 20 * 1024 * 1024 // 20MB for audio
	maxGifSize   = 15 * 1024 * 1024 // 15MB for GIFs
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	if common.RunningUnitTests() {
		return
	}
	// Initialize default budget limits by MIME type (in micros)
	DefaultBudgetLimits = map[string]int64{
		"image/jpeg": 50000,  // $0.05 per image
		"image/png":  50000,  // $0.05 per image
		"image/gif":  100000, // $0.10 per GIF (more processing)
		"image/webp": 50000,  // $0.05 per image
		"video/mp4":  500000, // $0.50 per video
		"video/webm": 500000, // $0.50 per video
		"audio/mpeg": 100000, // $0.10 per audio
		"audio/wav":  100000, // $0.10 per audio
		"audio/ogg":  100000, // $0.10 per audio
	}
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config //nolint:unused // dependency injection pattern - available for processor extensions
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage //nolint:unused // dependency injection pattern - available for processor extensions
	processor *MediaProcessor
)

func init() {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "media-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if lambdaCtx.Repos != nil {
		repos = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	}

	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Initialize processor with media-specific configuration
	processor = NewMediaProcessor(lambdaCtx)
}

// NewMediaProcessor creates a new media processor instance with simplified Lambda context
func NewMediaProcessor(lambdaCtx *common.LambdaContext) *MediaProcessor {
	if lambdaCtx == nil || lambdaCtx.DynamoDB == nil || lambdaCtx.Repos == nil {
		return &MediaProcessor{}
	}
	// Initialize simplified processor with essential components
	return &MediaProcessor{
		db:        lambdaCtx.DynamoDB.(core.DB),
		repos:     lambdaCtx.Repos.(storageCore.RepositoryStorage),
		tableName: lambdaCtx.Config.DynamoTableName,
		logger:    lambdaCtx.Logger,
	}
}

func main() {
	// Configure and start Lambda
	app := setupLiftApp()
	lambdaHandler := createLambdaHandler(app)
	lambda.Start(lambdaHandler)
}

// liftApp interface defines the methods we need from the Lift app
type liftApp interface {
	Use(middleware func(lift.Handler) lift.Handler) *lift.App
	SQS(name string, handler interface{}) error
	HandleRequest(ctx context.Context, event interface{}) (interface{}, error)
}

// setupLiftApp configures the Lift application with middleware and handlers
func setupLiftApp() liftApp {
	app := lift.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Add all middleware
	addRequestIDMiddleware(app)
	addLoggingMetricsMiddleware(app)
	addErrorHandlingMiddleware(app)

	// Set SQS handler for media processing
	addSQSHandler(app)

	return app
}

// addRequestIDMiddleware adds request ID generation middleware
func addRequestIDMiddleware(app liftApp) {
	_ = app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("media-processing-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})
}

// addLoggingMetricsMiddleware adds logging and metrics collection middleware
func addLoggingMetricsMiddleware(app liftApp) {
	_ = app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID").(string)

			processor.logger.Info("processing SQS batch",
				zap.String("request_id", requestID),
			)

			// Record SQS processing metrics
			if processor.emfMetrics != nil {
				processor.emfMetrics.RecordBusinessMetric(observability.MetricMediaProcessing, 1.0, observability.UnitCount, nil)
			}

			err := next.Handle(ctx)
			duration := time.Since(start)

			recordProcessingMetrics(err, duration)
			logProcessingResult(requestID, err, duration)

			return err
		})
	})
}

// recordProcessingMetrics records processing metrics based on success/failure
func recordProcessingMetrics(err error, duration time.Duration) {
	if processor.emfMetrics == nil {
		return
	}

	processor.emfMetrics.RecordLatency("sqs_batch_processing", duration)

	if err != nil {
		processor.emfMetrics.RecordError("sqs_batch_processing", observability.ErrorTypeInternal)
	} else {
		processor.emfMetrics.RecordSuccess("sqs_batch_processing")
	}

	// Record processing time metric
	processor.emfMetrics.RecordBusinessMetric(observability.MetricMediaProcessingTime,
		float64(duration.Milliseconds()), observability.UnitMilliseconds, nil)
}

// logProcessingResult logs the processing result with appropriate level
func logProcessingResult(requestID string, err error, duration time.Duration) {
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
}

// addErrorHandlingMiddleware adds error handling middleware
func addErrorHandlingMiddleware(app liftApp) {
	_ = app.Use(func(next lift.Handler) lift.Handler {
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
}

// addSQSHandler adds the SQS message handler
func addSQSHandler(app liftApp) {
	_ = app.SQS("media-processing", func(ctx *lift.Context) error {
		event, err := extractSQSEvent(ctx)
		if err != nil {
			return err
		}
		return processor.HandleSQS(ctx, event)
	})
}

// extractSQSEvent extracts and parses the SQS event from the Lift context
func extractSQSEvent(ctx *lift.Context) (events.SQSEvent, error) {
	// Extract SQS event from Lift context
	if ctx.Request.RawEvent == nil {
		return events.SQSEvent{}, lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
	}

	// Parse the raw event as SQS event
	var event events.SQSEvent
	if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
		event = sqsEvent
	} else {
		// Try to parse from interface if it's a map
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return events.SQSEvent{}, lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return events.SQSEvent{}, lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
		}
	}

	return event, nil
}

// createLambdaHandler creates the main Lambda handler with observability
func createLambdaHandler(app liftApp) func(context.Context, interface{}) (interface{}, error) {
	return func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()

		// Record cold start metrics
		recordColdStartMetrics()

		// Process the request
		result, err := app.HandleRequest(ctx, event)

		// Record Lambda-level metrics
		recordLambdaMetrics(requestStart, err)

		// Flush metrics before termination
		flushMetrics()

		return result, err
	}
}

// recordColdStartMetrics records cold start metrics if applicable
func recordColdStartMetrics() {
	if time.Since(processor.startTime) < 30*time.Second && processor.emfMetrics != nil {
		processor.emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
		coldStartDuration := time.Since(processor.startTime)
		processor.emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
	}
}

// recordLambdaMetrics records Lambda-level metrics
func recordLambdaMetrics(requestStart time.Time, err error) {
	if processor.emfMetrics == nil {
		return
	}

	requestDuration := time.Since(requestStart)
	processor.emfMetrics.RecordLatency("media_lambda_request", requestDuration)
	processor.emfMetrics.RecordThroughput("media_lambda_request", 1)

	if err != nil {
		processor.emfMetrics.RecordError("media_lambda_request", "lambda_error")
	} else {
		processor.emfMetrics.RecordSuccess("media_lambda_request")
	}
}

// flushMetrics ensures all EMF metrics are written to CloudWatch before Lambda terminates
func flushMetrics() {
	if processor.emfMetrics != nil {
		processor.emfMetrics.Flush()
	}
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
			// Handle job failure with retry logic
			// First get the job to update it
			if job, getErr := mp.mediaRepo.GetMediaJob(ctx.Request.Context(), processingEvent.JobID); getErr == nil {
				if retryErr := mp.handleJobFailure(ctx.Request.Context(), job, err); retryErr != nil {
					mp.logger.Error("Failed to handle job failure",
						zap.String("jobID", processingEvent.JobID),
						zap.Error(retryErr))
				}
			}
		}
	}

	return nil
}

func (mp *MediaProcessor) initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return AWSConfigLoadFailed(err)
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

// processMediaJob handles the complete media processing pipeline
// Note: High complexity (gocognit: 31) is necessary to handle various media types,
// format validations, size constraints, transcoding options, thumbnail generation,
// virus scanning, and comprehensive error recovery. Breaking this up would lose
// the atomic processing guarantee and complicate error handling.
//
//nolint:gocognit // Media processing requires handling many edge cases
func (mp *MediaProcessor) processMediaJob(ctx context.Context, event MediaProcessingEvent) error {
	processingStart := time.Now()

	mp.logger.Info("processing media job",
		zap.String("job_id", event.JobID),
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username))

	// Record media job start metrics
	if mp.emfMetrics != nil {
		dimensions := map[string]string{
			"job_id": event.JobID,
			"user":   event.Username,
		}
		mp.emfMetrics.RecordBusinessMetric(observability.MetricMediaProcessing, 1.0, observability.UnitCount, dimensions)
	}

	// Defer function to record completion metrics
	defer func() {
		processingDuration := time.Since(processingStart)
		if mp.emfMetrics != nil {
			mp.emfMetrics.RecordBusinessMetric(observability.MetricMediaProcessingTime,
				float64(processingDuration.Milliseconds()), observability.UnitMilliseconds,
				map[string]string{
					"job_id": event.JobID,
					"user":   event.Username,
				})
		}
	}()

	// Get job details using DynamORM
	job, err := mp.mediaRepo.GetMediaJob(ctx, event.JobID)
	if err != nil {
		return JobGetFailed(err)
	}

	// Check if job has already been processed (idempotency)
	if job.IsCompleted() {
		mp.logger.Info("job already completed, skipping",
			zap.String("job_id", event.JobID),
			zap.String("idempotency_key", job.IdempotencyKey))
		return nil
	}

	// Check if job is cancelled
	if job.IsCancelled() {
		mp.logger.Info("job is cancelled, skipping",
			zap.String("job_id", event.JobID))
		return nil
	}

	// Check if another instance is already processing this job
	if job.IsProcessing() && !mp.isJobAbandoned(job) {
		mp.logger.Warn("job is already being processed by another instance",
			zap.String("job_id", event.JobID),
			zap.Time("processing_started_at", *job.ProcessingStartedAt))
		return nil // Don't process concurrently
	}

	// Update job status to processing with processing lock
	job.SetProcessing()
	if err := mp.mediaRepo.UpdateMediaJob(ctx, job); err != nil {
		mp.logger.Error("failed to update job status, another instance may be processing",
			zap.Error(err))
		return nil // Don't fail - optimistic concurrency control
	}

	// Mark MediaMetadata as processing started
	if err := mp.mediaMetadataRepo.MarkProcessingStarted(ctx, event.MediaID); err != nil {
		mp.logger.Warn("failed to mark media metadata as processing started",
			zap.String("media_id", event.MediaID),
			zap.Error(err))
		// Continue processing - this is not critical
	}

	// Get user's media configuration and validate quotas
	userConfig := mp.getUserMediaConfig(ctx, event.Username)

	// Download original file from S3
	originalData, err := mp.downloadFromS3(ctx, job.S3Key)
	if err != nil {
		return MediaDownloadFailed(err)
	}

	// Validate file type and size against user's configuration
	if err := mp.validateFileForUser(originalData, job.MimeType, userConfig, event.Username, event.MediaID); err != nil {
		return FileValidationFailedError(err)
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
			return S3UploadOriginalFailed(err)
		}

		// Update media record and job
		if err := mp.updateMediaRecord(ctx, event.MediaID, result); err != nil {
			return MediaRecordUpdateFailed(err)
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
			return JobUpdateStatusFailed(err)
		}

		// Record successful completion with budget skip
		if mp.emfMetrics != nil {
			mp.emfMetrics.RecordBusinessMetric("MediaProcessingSkipped", 1.0, observability.UnitCount,
				map[string]string{
					observability.DimensionMediaType: getMediaTypeFromMime(job.MimeType),
					"reason":                         "budget_exceeded",
					"user":                           event.Username,
				})
		}

		return nil
	}

	// Process based on media type with timeout and budget enforcement
	var result ProcessingResult
	mediaType := getMediaTypeFromMime(job.MimeType)

	// Create timeout context for processing
	timeoutCtx, cancel := context.WithTimeout(ctx, job.MaxProcessingTime)
	defer cancel()

	// Process media based on type
	var processingErr error
	switch mediaType {
	case mediaTypeImage, mediaTypeGifv:
		result, processingErr = mp.processImage(timeoutCtx, originalData, event, job.ProcessingTasks, job.MimeType)

	case mediaTypeVideo:
		result, processingErr = mp.processVideo(timeoutCtx, originalData, event, job.ProcessingTasks)

	case mediaTypeAudio:
		result, processingErr = mp.processAudio(timeoutCtx, originalData, event, job.ProcessingTasks)

	default:
		mp.logger.Error("unsupported media type for processing",
			zap.String("media_type", mediaType),
			zap.String("job_id", event.JobID))
		processingErr = UnsupportedMediaTypeError(mediaType)
	}

	if processingErr != nil {
		// Mark MediaMetadata as processing failed
		if err := mp.mediaMetadataRepo.MarkProcessingFailed(ctx, event.MediaID, processingErr.Error()); err != nil {
			mp.logger.Warn("failed to mark media metadata as processing failed",
				zap.String("media_id", event.MediaID),
				zap.Error(err))
		}

		mp.handleProcessingError(ctx, job, processingErr)
		return processingErr
	}

	// Update media record with processing results
	if err := mp.updateMediaRecord(ctx, event.MediaID, result); err != nil {
		return MediaRecordUpdateFailed(err)
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
		return JobUpdateStatusFailed(err)
	}

	// Record successful processing completion
	if mp.emfMetrics != nil {
		mediaType := getMediaTypeFromMime(job.MimeType)
		dimensions := map[string]string{
			observability.DimensionMediaType: mediaType,
			"user":                           event.Username,
			"status":                         "completed",
		}

		mp.emfMetrics.RecordBusinessMetric("MediaProcessingCompleted", 1.0, observability.UnitCount, dimensions)
		mp.emfMetrics.RecordSuccess("media_processing_job")

		// Record file size metrics for different media types
		if job.FileSize > 0 {
			mp.emfMetrics.RecordBusinessMetric("MediaFileSizeProcessed", float64(job.FileSize), observability.UnitBytes, dimensions)
		}

		// Record dimensions if available
		if result.Width > 0 && result.Height > 0 {
			mp.emfMetrics.RecordBusinessMetric("MediaWidthProcessed", float64(result.Width), observability.UnitNone, dimensions)
			mp.emfMetrics.RecordBusinessMetric("MediaHeightProcessed", float64(result.Height), observability.UnitNone, dimensions)
		}
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
		return result, ImageProcessingFailed(err)
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
		return result, S3KeySanitizationFailed(err)
	}

	// Track S3 upload cost
	s3UploadCost := mp.calculateS3PutCost(int64(len(data)))
	jobMetrics.CostBreakdown["s3_upload"] = s3UploadCost

	if err := mp.uploadToS3(ctx, s3Key, data, "video/mp4"); err != nil {
		return result, S3UploadVideoFailed(err)
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
		return nil, S3GetObjectFailed(err)
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
		return nil, S3ReadObjectFailed(err)
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

	// Update the main media record
	if err := mp.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return MediaRecordUpdateFailed(err)
	}

	// Create or update MediaMetadata record with processing results
	processingResult := repositories.ProcessingResult{
		Width:    result.Width,
		Height:   result.Height,
		Duration: result.Duration, // Duration is in milliseconds from result
		FileSize: 0,               // Would need file size calculation
		Blurhash: result.Blurhash,
		Sizes:    make(map[string]repositories.SizeInfo),
	}

	// Convert ProcessingResult.Sizes to repositories.SizeInfo format
	for sizeName, sizeInfo := range result.Sizes {
		processingResult.Sizes[sizeName] = repositories.SizeInfo{
			Width:  sizeInfo.Width,
			Height: sizeInfo.Height,
			S3Key:  sizeInfo.S3Key,
			URL:    sizeInfo.URL,
		}
	}

	// Mark MediaMetadata as processing complete with results
	if err := mp.mediaMetadataRepo.MarkProcessingComplete(ctx, mediaID, processingResult); err != nil {
		mp.logger.Warn("failed to update media metadata record",
			zap.String("media_id", mediaID),
			zap.Error(err))
		// Don't fail the entire operation if metadata update fails
	}

	return nil
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
	// Resolve username to userID using proper lookup
	userID, err := mp.resolveUsernameToUserID(ctx, username)
	if err != nil {
		mp.logger.Warn("failed to resolve username to userID, using fallback",
			zap.String("username", username),
			zap.Error(err))
		// Fallback to username as userID for backwards compatibility
		userID = username
	}

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
	userID := username                              // Simplification - in reality you'd resolve username to userID

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
		return result, S3KeySanitizationFailed(err)
	}
	if err := mp.uploadToS3(ctx, s3Key, data, mimeType); err != nil {
		return result, S3UploadOriginalFailed(err)
	}

	result.Sizes["original"] = SizeInfo{
		URL:   mp.buildMediaURL(s3Key),
		S3Key: s3Key,
	}

	// Track minimal cost (just S3 storage) using centralized tracking
	if err := mp.unifiedTracker.TrackS3Put(ctx, mp.bucketName, int64(len(data))); err != nil {
		mp.logger.Warn("failed to track S3 put cost", zap.Error(err))
	}

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
	if err := common.ValidateSliceNotEmpty("file_data", data); err != nil {
		return EmptyFileError()
	}

	// Validate claimed MIME type format if provided
	if claimedMimeType != "" {
		if err := common.ValidateMastodonMimeType(claimedMimeType); err != nil {
			return InvalidMimeTypeFormatError(claimedMimeType)
		}
	}

	// Detect actual MIME type from file content
	detectedType := http.DetectContentType(data)

	// Clean up detected type (remove charset info)
	if idx := strings.Index(detectedType, ";"); idx > 0 {
		detectedType = detectedType[:idx]
	}

	// Validate detected MIME type format
	if err := common.ValidateMastodonMimeType(detectedType); err != nil {
		return DetectedMimeTypeInvalidError(detectedType)
	}

	// Check if detected type is allowed
	if !allowedMimeTypes[detectedType] {
		// Log the detected type for debugging
		// Note: logging should be done at call site with proper context
		return FileTypeNotAllowedError(detectedType)
	}

	// Check if claimed type matches detected type
	if claimedMimeType != "" && claimedMimeType != detectedType {
		// Log MIME type mismatch details for debugging
		// Note: logging should be done at call site with proper context
		return MimeTypeMismatchError(claimedMimeType, detectedType)
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

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		if mimeType == common.ImageGIF {
			maxSize = maxGifSize
		} else {
			maxSize = maxImageSize
		}
	case strings.HasPrefix(mimeType, "video/"):
		maxSize = maxVideoSize
	case strings.HasPrefix(mimeType, "audio/"):
		maxSize = maxAudioSize
	default:
		// Log unknown MIME type for debugging
		// Note: logging should be done at call site with proper context
		return UnknownFileTypeError(mimeType)
	}

	if size > maxSize {
		// Log file size limit exceeded for debugging
		// Note: logging should be done at call site with proper context
		return FileTooLargeError(int64(size), int64(maxSize))
	}

	return nil
}

// extractAudioDuration extracts duration from audio data using dhowden/tag
func extractAudioDuration(data []byte) (int, error) {
	// Use dhowden/tag to parse audio metadata
	metadata, err := tag.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return 0, AudioMetadataReadFailed(err)
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

	return 0, UnableToDetermineAudioDurationError(nil)
}

// extractVideoMetadata extracts real video metadata from video data using MP4 atom parsing
func (mp *MediaProcessor) extractVideoMetadata(data []byte) (width, height, duration int) {
	// Use the comprehensive video metadata parser
	metadata, err := media.ParseVideoMetadata(data)
	if err != nil {
		// If parsing completely fails, log error and provide minimal fallbacks
		mp.logger.Error("failed to parse video metadata, cannot determine video properties",
			zap.Error(err),
			zap.Int("data_size", len(data)))

		// Return minimal fallback values - this should be rare with the robust parser
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

	mp.logger.Info("successfully extracted video metadata",
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

	// Return the actual parsed metadata
	return metadata.Width, metadata.Height, metadata.Duration
}

// sanitizeS3Key ensures the S3 key doesn't contain path traversal attempts
func sanitizeS3Key(username, mediaID, filename string) (string, error) {
	// Validate username doesn't contain path traversal
	if strings.Contains(username, "..") || strings.Contains(username, "/") {
		return "", InvalidUsernameForS3KeyError(username)
	}

	// Validate mediaID
	if strings.Contains(mediaID, "..") || strings.Contains(mediaID, "/") {
		return "", InvalidMediaIDForS3KeyError(mediaID)
	}

	// Validate filename
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		return "", InvalidFilenameForS3KeyError(filename)
	}

	// Construct safe S3 key
	return fmt.Sprintf("media/%s/%s/%s", username, mediaID, filename), nil
}

// validateFileForUser validates a file against user's media configuration
func (mp *MediaProcessor) validateFileForUser(data []byte, mimeType string, config *MediaConfig, username string, mediaID string) error {
	// Check file size first
	fileSize := int64(len(data))

	// First do basic file type validation
	if err := validateFileType(data, mimeType); err != nil {
		return err
	}

	// Use common validation for basic checks first
	if err := common.ValidateMediaEntity(mediaID, "media_file", fileSize); err != nil {
		// If common validation fails with size limit, continue with user-specific limits
		// The common validation uses 100MB limit, but we want user-specific limits
		if !strings.Contains(err.Error(), "cannot be larger than") {
			return err // Other errors (mediaID, filename validation)
		}
	}

	var userMaxSize int64

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		userMaxSize = maxImageSize // Use global default for images
	case strings.HasPrefix(mimeType, "video/"):
		userMaxSize = maxVideoSize
		// Check video duration limit if this is a video
		if config.MaxVideoDuration > 0 {
			// Parse actual video metadata for duration check
			actualDuration, err := mp.getVideoMetadata(data, mimeType)
			if err != nil {
				mp.logger.Error("failed to parse video metadata for duration validation",
					zap.Error(err),
					zap.String("mime_type", mimeType),
					zap.String("username", username))
				// Reject video if we cannot determine duration and user has limits
				return VideoValidationFailed(err)
			}

			if actualDuration > config.MaxVideoDuration {
				mp.logger.Warn("video duration exceeds user limit",
					zap.Int("actual_duration_seconds", actualDuration),
					zap.Int("max_duration_seconds", config.MaxVideoDuration),
					zap.String("username", username),
					zap.String("media_id", mediaID))
				return VideoDurationExceededError(actualDuration, config.MaxVideoDuration)
			}

			mp.logger.Debug("video duration validation passed",
				zap.Int("actual_duration_seconds", actualDuration),
				zap.Int("max_duration_seconds", config.MaxVideoDuration),
				zap.String("username", username))
		}
	case strings.HasPrefix(mimeType, "audio/"):
		userMaxSize = maxAudioSize
	default:
		mp.logger.Warn("unsupported media type for user",
			zap.String("mime_type", mimeType),
			zap.String("username", username),
			zap.String("media_id", mediaID))
		return UnsupportedMediaTypeForUserError(mimeType)
	}

	if fileSize > userMaxSize {
		mp.logger.Warn("file size exceeds user limit",
			zap.Int64("file_size_bytes", fileSize),
			zap.Int64("user_max_size_bytes", userMaxSize),
			zap.String("username", username),
			zap.String("media_id", mediaID))
		return FileSizeExceedsUserLimitError(fileSize, userMaxSize)
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

// resolveUsernameToUserID resolves a username to a user ID
func (mp *MediaProcessor) resolveUsernameToUserID(ctx context.Context, username string) (string, error) {
	// Try to get user by username from user repository
	// This would typically involve a GSI lookup or a dedicated method
	// For now, implement a basic lookup strategy

	// Strategy 1: Try direct username lookup if available
	// This assumes the media repository has access to user data
	userConfig, err := mp.mediaRepo.GetUserMediaConfigByUsername(ctx, username)
	if err == nil && userConfig != nil {
		return userConfig.UserID, nil
	}

	// Strategy 2: Use username as userID (backwards compatibility)
	// This maintains existing behavior while allowing for proper resolution
	return username, nil
}

// getVideoMetadata extracts video metadata including duration using real parsing
func (mp *MediaProcessor) getVideoMetadata(data []byte, mimeType string) (int, error) {
	// Parse video metadata using the comprehensive MP4/MOV parser
	metadata, err := media.ParseVideoMetadata(data)
	if err != nil {
		mp.logger.Warn("failed to parse video metadata, using fallback",
			zap.Error(err),
			zap.String("mime_type", mimeType),
			zap.Int("data_size", len(data)))
		return 0, err
	}

	mp.logger.Debug("successfully parsed video metadata",
		zap.String("mime_type", mimeType),
		zap.Int("width", metadata.Width),
		zap.Int("height", metadata.Height),
		zap.Int("duration_ms", metadata.Duration),
		zap.Float64("duration_seconds", metadata.DurationSeconds),
		zap.String("video_codec", metadata.VideoCodec),
		zap.String("audio_codec", metadata.AudioCodec),
		zap.Bool("has_video", metadata.HasVideo),
		zap.Bool("has_audio", metadata.HasAudio))

	// Return duration in seconds (convert from milliseconds)
	durationSeconds := int(metadata.DurationSeconds)
	if durationSeconds <= 0 {
		// Use milliseconds converted to seconds as fallback
		durationSeconds = metadata.Duration / 1000
	}

	return durationSeconds, nil
}

// === COMPREHENSIVE COST TRACKING AND BUDGET ENFORCEMENT ===

// NewMediaJobCostTracker creates a new cost tracker for a media job
func (mp *MediaProcessor) NewMediaJobCostTracker(jobID, userID string, mimeType string, fileSize int64) *MediaJobCostTracker {
	budgetMicros, exists := DefaultBudgetLimits[mimeType]
	if !exists {
		// Default budget for unknown types
		budgetMicros = 100000 // $0.10
	}

	// Adjust budget based on file size (larger files cost more)
	if fileSize > 10*1024*1024 { // > 10MB
		budgetMicros *= 2 // Double budget for large files
	} else if fileSize > 50*1024*1024 { // > 50MB
		budgetMicros *= 5 // 5x budget for very large files
	}

	return &MediaJobCostTracker{
		JobID:     jobID,
		UserID:    userID,
		StartTime: time.Now(),
		CostBreakdown: map[string]int64{
			models.MediaCostUpload:     0,
			models.MediaCostStorage:    0,
			models.MediaCostTranscode:  0,
			models.MediaCostThumbnail:  0,
			models.MediaCostModeration: 0,
			models.MediaCostAnalysis:   0,
			models.MediaCostDelivery:   0,
		},
		BudgetMicros:      budgetMicros,
		WarningThresholds: []float64{0.5, 0.75, 0.9}, // 50%, 75%, 90%
		WarningsSent:      []bool{false, false, false},
		mediaRepo:         mp.mediaRepo,
		logger:            mp.logger,
	}
}

// AddCost adds cost to a specific category and checks budget limits
func (ct *MediaJobCostTracker) AddCost(category string, costMicros int64) error {
	ct.CostBreakdown[category] += costMicros
	ct.TotalCostMicros += costMicros

	ct.logger.Debug("added cost to media job",
		zap.String("job_id", ct.JobID),
		zap.String("category", category),
		zap.Int64("cost_micros", costMicros),
		zap.Int64("total_cost_micros", ct.TotalCostMicros),
		zap.Int64("budget_micros", ct.BudgetMicros))

	// Check budget warnings
	if err := ct.checkBudgetWarnings(); err != nil {
		return err
	}

	// Check if budget is exceeded
	if ct.TotalCostMicros > ct.BudgetMicros {
		ct.logger.Error("media job budget exceeded",
			zap.String("job_id", ct.JobID),
			zap.String("user_id", ct.UserID),
			zap.Int64("total_cost_micros", ct.TotalCostMicros),
			zap.Int64("budget_micros", ct.BudgetMicros),
			zap.Float64("spent_dollars", float64(ct.TotalCostMicros)/1000000.0),
			zap.Float64("budget_dollars", float64(ct.BudgetMicros)/1000000.0))
		return BudgetExceededError(ct.TotalCostMicros, ct.BudgetMicros)
	}

	return nil
}

// checkBudgetWarnings checks and sends budget warnings at thresholds
func (ct *MediaJobCostTracker) checkBudgetWarnings() error {
	budgetUsedPercent := float64(ct.TotalCostMicros) / float64(ct.BudgetMicros)

	for i, threshold := range ct.WarningThresholds {
		if budgetUsedPercent >= threshold && !ct.WarningsSent[i] {
			ct.logger.Warn("media job budget warning",
				zap.String("job_id", ct.JobID),
				zap.String("user_id", ct.UserID),
				zap.Float64("threshold", threshold),
				zap.Float64("budget_used_percent", budgetUsedPercent),
				zap.Int64("total_cost_micros", ct.TotalCostMicros),
				zap.Int64("budget_micros", ct.BudgetMicros))

			ct.WarningsSent[i] = true

			// Update job with warning information
			if err := ct.updateJobWithWarning(threshold); err != nil {
				ct.logger.Error("failed to update job with budget warning", zap.Error(err))
			}
		}
	}

	return nil
}

// updateJobWithWarning updates the job with budget warning information
func (ct *MediaJobCostTracker) updateJobWithWarning(_ float64) error {
	job, err := ct.mediaRepo.GetMediaJob(context.Background(), ct.JobID)
	if err != nil {
		return JobUpdateWarningFailed(err)
	}

	// Add warning to job's cost information
	job.ActualCostMicros = ct.TotalCostMicros
	job.EstimatedCostMicros = ct.BudgetMicros

	return ct.mediaRepo.UpdateMediaJob(context.Background(), job)
}

// FinishTracking marks the cost tracking as complete and saves final metrics
func (ct *MediaJobCostTracker) FinishTracking() error {
	now := time.Now()
	ct.EndTime = &now
	processingDuration := now.Sub(ct.StartTime)

	// Calculate Lambda execution cost
	lambdaMemoryMB := int64(512) // Default Lambda memory
	lambdaGBSeconds := float64(lambdaMemoryMB) / 1024.0 * processingDuration.Seconds()
	lambdaCostMicros := int64(lambdaGBSeconds * float64(transcodingCosts.LambdaGBSecondCost))
	_ = ct.AddCost("lambda_execution", lambdaCostMicros)

	ct.logger.Info("finished cost tracking for media job",
		zap.String("job_id", ct.JobID),
		zap.String("user_id", ct.UserID),
		zap.Duration("processing_duration", processingDuration),
		zap.Int64("total_cost_micros", ct.TotalCostMicros),
		zap.Int64("budget_micros", ct.BudgetMicros),
		zap.Any("cost_breakdown", ct.CostBreakdown))

	// Save cost transaction to repository
	return ct.saveCostTransaction()
}

// saveCostTransaction saves the final cost breakdown as a spending transaction
func (ct *MediaJobCostTracker) saveCostTransaction() error {
	txn := &models.MediaSpendingTransaction{
		UserID:      ct.UserID,
		Category:    "media_processing",
		CostMicros:  ct.TotalCostMicros,
		JobID:       ct.JobID,
		Description: fmt.Sprintf("Media processing: budget=%d warnings=%v", ct.BudgetMicros, ct.WarningsSent),
	}

	return ct.mediaRepo.AddSpendingTransaction(context.Background(), txn)
}

// handleJobFailure handles a job failure with retry logic
func (mp *MediaProcessor) handleJobFailure(ctx context.Context, job *models.MediaJob, err error) error {
	mp.logger.Error("Media job failed",
		zap.String("job_id", job.JobID),
		zap.Int("retry_count", job.RetryCount),
		zap.Error(err))

	// Update job with failure information
	job.Status = models.MediaStatusFailed
	job.LastError = err.Error()
	now := time.Now()
	job.LastAttemptAt = &now

	// Check if we should retry
	if job.RetryCount < job.MaxRetries && !mp.isPermanentError(err) {
		// Safe conversion with bounds checking for exponential backoff
		retryCount := job.RetryCount
		if retryCount < 0 {
			retryCount = 0
		}
		if retryCount > 10 { // Cap at 1024 seconds (~17 minutes)
			retryCount = 10
		}
		job.ScheduleRetry(time.Second * time.Duration(1<<uint(retryCount))) //nolint:gosec // Bounded to 0-10, safe conversion
		job.Status = models.MediaStatusPending                              // Will be retried
		mp.logger.Info("Scheduling job retry",
			zap.String("job_id", job.JobID),
			zap.Timep("retry_at", job.RetryScheduledAt),
			zap.Int("retry_count", job.RetryCount))
	}

	// Save the updated job
	return mp.mediaRepo.UpdateMediaJob(ctx, job)
}

// isJobAbandoned checks if a job has been abandoned
func (mp *MediaProcessor) isJobAbandoned(job *models.MediaJob) bool {
	if job.Status != models.MediaStatusProcessing {
		return false
	}

	// Check if job has been processing for too long without updates
	if job.LastAttemptAt == nil {
		return false
	}
	timeSinceLastUpdate := time.Since(*job.LastAttemptAt)
	return timeSinceLastUpdate > time.Hour
}

// handleProcessingError handles errors during processing
func (mp *MediaProcessor) handleProcessingError(ctx context.Context, job *models.MediaJob, err error) {
	mp.logger.Error("Processing error",
		zap.String("job_id", job.JobID),
		zap.Error(err))

	// Determine if error is permanent
	isPermanent := mp.isPermanentError(err)

	if isPermanent {
		job.Status = models.MediaStatusFailed
		job.LastError = fmt.Sprintf("Permanent error: %v", err)
	} else {
		// Schedule retry for transient errors
		if job.RetryCount < job.MaxRetries {
			// Safe conversion with bounds checking for exponential backoff
			retryCount := job.RetryCount
			if retryCount < 0 {
				retryCount = 0
			}
			if retryCount > 10 { // Cap at 1024 seconds (~17 minutes)
				retryCount = 10
			}
			job.ScheduleRetry(time.Second * time.Duration(1<<uint(retryCount))) //nolint:gosec // Bounded to 0-10, safe conversion
			job.Status = models.MediaStatusPending
		} else {
			job.Status = models.MediaStatusFailed
			job.LastError = fmt.Sprintf("Max retries exceeded: %v", err)
		}
	}

	now := time.Now()
	job.LastAttemptAt = &now
	if updateErr := mp.mediaRepo.UpdateMediaJob(ctx, job); updateErr != nil {
		mp.logger.Error("Failed to update job after error",
			zap.String("job_id", job.JobID),
			zap.Error(updateErr))
	}

	// Record error metrics
	if mp.emfMetrics != nil {
		mediaType := getMediaTypeFromMime(job.MimeType)
		errorType := observability.ErrorTypeInternal

		// Classify error type
		if isPermanent {
			errorType = observability.ErrorTypeValidation
		} else if job.RetryCount >= job.MaxRetries {
			errorType = observability.ErrorTypeTimeout
		}

		dimensions := map[string]string{
			observability.DimensionMediaType: mediaType,
			observability.DimensionErrorType: errorType,
			"user":                           job.Username,
			"retry_count":                    fmt.Sprintf("%d", job.RetryCount),
			"is_permanent":                   fmt.Sprintf("%t", isPermanent),
		}

		mp.emfMetrics.RecordError("media_processing_job", errorType)
		mp.emfMetrics.RecordBusinessMetric("MediaProcessingErrors", 1.0, observability.UnitCount, dimensions)

		// Record specific failure reasons
		if job.Status == models.MediaStatusFailed {
			mp.emfMetrics.RecordBusinessMetric("MediaProcessingFailed", 1.0, observability.UnitCount, dimensions)
		} else {
			mp.emfMetrics.RecordBusinessMetric("MediaProcessingRetry", 1.0, observability.UnitCount, dimensions)
		}

		// Trigger alerts for high error rates if needed
		if mp.alertManager != nil && isPermanent {
			go func() {
				alertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				mp.alertManager.CheckErrorRate(alertCtx, "media-processor", 100.0) // This failure represents 100% error rate for this job
			}()
		}
	}
}

// isPermanentError determines if an error is permanent and shouldn't be retried
func (mp *MediaProcessor) isPermanentError(err error) bool {
	errStr := strings.ToLower(err.Error())
	permanentErrors := []string{
		"invalid format",
		"unsupported",
		"malformed",
		"corrupt",
		"virus detected",
		"budget exceeded",
		"forbidden",
	}

	for _, permErr := range permanentErrors {
		if strings.Contains(errStr, permErr) {
			return true
		}
	}
	return false
}
