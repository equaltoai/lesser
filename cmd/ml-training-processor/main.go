// Package main implements the ml-training-processor Lambda function for processing ML training job state changes.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// DynamoDB stream event type constants
const (
	eventNameInsert = "INSERT"
	eventNameModify = "MODIFY"
)

// Training job status constants
const (
	statusInProgress = "IN_PROGRESS"
	statusCompleted  = "COMPLETED"
	statusFailed     = "FAILED"
	statusTimeout    = "TIMEOUT"
)

// MLTrainingProcessor handles ML training job lifecycle events
type MLTrainingProcessor struct {
	db               core.DB
	tableName        string
	logger           *zap.Logger
	bedrockClient    bedrockJobGetter
	s3Client         s3ObjectGetter
	moderationMLRepo moderationMLRepository
}

// Global variables for standardized Lambda initialization
var (
	lambdaCtx *common.LambdaContext
	processor *MLTrainingProcessor
)

type bedrockJobGetter interface {
	GetModelCustomizationJob(ctx context.Context, params *bedrock.GetModelCustomizationJobInput, optFns ...func(*bedrock.Options)) (*bedrock.GetModelCustomizationJobOutput, error)
}

type s3ObjectGetter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type moderationMLRepository interface {
	CreatePollRequest(ctx context.Context, pollReq *models.MLPollRequest) error
	GetTrainingJob(ctx context.Context, jobARN string) (*models.ModelTrainingJob, error)
	UpdateTrainingJob(ctx context.Context, job *models.ModelTrainingJob) error
	CreateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error
	GetActiveModelVersion(ctx context.Context) (*models.ModerationModelVersion, error)
	UpdateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error
}

var (
	runningUnitTestsFn       = common.RunningUnitTests
	mustInitializeLambdaFn   = common.MustInitializeLambda
	initializeWithDefaultsFn = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	newMLTrainingProcessorFn = NewMLTrainingProcessor
	lambdaStartFn            = lambda.Start
	newBedrockClientFn       = func(cfg aws.Config) bedrockJobGetter { return bedrock.NewFromConfig(cfg) }
	newS3ClientFn            = func(cfg aws.Config) s3ObjectGetter { return s3.NewFromConfig(cfg) }
	newModerationMLRepoFn    = func(db core.DB, tableName string, logger *zap.Logger) moderationMLRepository {
		return repositories.NewModerationMLRepository(db, tableName, logger)
	}
	writeStreamingEventFn = func(ctx context.Context, db core.DB, event *models.StreamingEvent) error {
		return db.WithContext(ctx).Model(event).Create()
	}
)

func init() {
	initializeMLTrainingOnStart()
}

func initializeMLTrainingOnStart() {
	if runningUnitTestsFn() {
		return
	}

	if err := initializeMLTraining(); err != nil {
		lambdaCtx.Logger.Fatal("failed to create ML training processor", zap.Error(err))
	}
}

func initializeMLTraining() error {
	// Standardized Lambda initialization for background processors
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "ml-training-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Initialize with processor-specific defaults
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		lambdaCtx.Logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Initialize processor
	var initErr error
	processor, initErr = newMLTrainingProcessorFn()
	if initErr != nil {
		return initErr
	}

	return nil
}

// NewMLTrainingProcessor creates a new ML training processor
func NewMLTrainingProcessor() (*MLTrainingProcessor, error) {
	// Get config from the initialized lambda context
	globalCfg := lambdaCtx.AWSServices.Config

	// Use the standardized database connection
	db := lambdaCtx.DynamoDB.(core.DB)
	tableName := lambdaCtx.Config.DynamoTableName

	// Initialize Bedrock client
	bedrockClient := newBedrockClientFn(globalCfg)

	// Initialize S3 client for metrics parsing
	s3Client := newS3ClientFn(globalCfg)

	// Initialize repositories
	moderationMLRepo := newModerationMLRepoFn(db, tableName, lambdaCtx.Logger)

	return &MLTrainingProcessor{
		db:               db,
		tableName:        tableName,
		logger:           lambdaCtx.Logger,
		bedrockClient:    bedrockClient,
		s3Client:         s3Client,
		moderationMLRepo: moderationMLRepo,
	}, nil
}

// HandleStream processes DynamoDB stream events for training jobs.
func (p *MLTrainingProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	p.logger.Info("processing ML training job batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("record_count", len(event.Records)),
	)

	// Process all records, collecting errors but not failing fast
	var errs []error
	for _, record := range event.Records {
		if err := p.processRecord(ctx, record); err != nil {
			p.logger.Error("failed to process record",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			errs = append(errs, err)
			// Continue processing other records
		}
	}

	// Log partial failures but don't return error
	if len(errs) > 0 {
		p.logger.Warn("partial batch failure",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Int("failed_count", len(errs)),
			zap.Int("total_count", len(event.Records)),
		)
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func (p *MLTrainingProcessor) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	logger := p.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_name", record.EventName),
		zap.String("event_id", record.EventID),
	)

	// Determine the entity type from the stream record
	entityType, err := stream.GetEventType(record)
	if err != nil {
		logger.Debug("failed to get entity type", zap.Error(err))
		return nil // Skip records we can't identify
	}

	logger = logger.With(zap.String("entity_type", entityType))

	// Route to appropriate handler based on entity type
	switch entityType {
	case "MLJOB", "ML_TRAINING_JOB":
		if record.EventName == eventNameModify {
			return p.processJobStatusChange(ctx, record)
		}
	case "MLPOLL", "ML_POLL_REQUEST":
		if record.EventName == eventNameInsert {
			return p.processPollRequest(ctx, record)
		}
	default:
		logger.Debug("ignoring event type")
		return nil
	}

	return nil
}

// processPollRequest handles poll request records
func (p *MLTrainingProcessor) processPollRequest(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Unmarshal the poll request from the stream
	var pollReq models.MLPollRequest
	if err := stream.UnmarshalItem(record, &pollReq); err != nil {
		p.logger.Debug("failed to unmarshal poll request from stream",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err))
		return nil // Skip records we can't unmarshal
	}

	// Only process pending polls that are due
	if pollReq.Status != "PENDING" {
		return nil
	}

	if time.Now().Before(pollReq.NextPollAfter) {
		// Not yet time to poll
		return nil
	}

	p.logger.Info("processing poll request",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("job_id", pollReq.JobID),
		zap.String("job_name", pollReq.JobName),
		zap.Int("attempt", pollReq.Attempt),
	)

	// Check for timeout
	if pollReq.Attempt >= pollReq.MaxAttempts {
		p.logger.Error("training job polling timed out",
			zap.String("job_id", pollReq.JobID),
			zap.Int("attempts", pollReq.Attempt))
		p.markJobAsTimeout(context.Background(), pollReq.JobID)
		return nil
	}

	// Get job status from Bedrock
	input := &bedrock.GetModelCustomizationJobInput{
		JobIdentifier: aws.String(pollReq.JobID),
	}

	output, err := p.bedrockClient.GetModelCustomizationJob(context.Background(), input)
	if err != nil {
		p.logger.Error("failed to get job status from Bedrock",
			zap.String("job_id", pollReq.JobID),
			zap.Error(err))
		// Schedule retry
		return p.scheduleNextPoll(pollReq, 60*time.Second)
	}

	status := output.Status
	p.logger.Info("bedrock training job status",
		zap.String("job_id", pollReq.JobID),
		zap.String("status", string(status)),
		zap.Int("attempt", pollReq.Attempt),
	)

	switch status {
	case types.ModelCustomizationJobStatusCompleted:
		// Update job status to COMPLETED
		return p.updateJobStatus(context.Background(), pollReq.JobID, statusCompleted, output)

	case types.ModelCustomizationJobStatusFailed:
		// Update job status to FAILED
		return p.updateJobStatus(context.Background(), pollReq.JobID, statusFailed, output)

	case types.ModelCustomizationJobStatusStopped:
		// Update job status to FAILED (stopped is treated as failure)
		return p.updateJobStatus(context.Background(), pollReq.JobID, statusFailed, output)

	case types.ModelCustomizationJobStatusInProgress:
		// Update to IN_PROGRESS and continue polling
		if err := p.updateJobStatus(context.Background(), pollReq.JobID, statusInProgress, output); err != nil {
			return err
		}
		return p.scheduleNextPoll(pollReq, 60*time.Second)

	default:
		p.logger.Warn("unexpected job status",
			zap.String("job_id", pollReq.JobID),
			zap.String("status", string(status)))
		// Continue polling for unknown statuses
		return p.scheduleNextPoll(pollReq, 60*time.Second)
	}
}

// scheduleNextPoll creates a new poll request for the next check
func (p *MLTrainingProcessor) scheduleNextPoll(currentPoll models.MLPollRequest, delay time.Duration) error {
	nextPoll := &models.MLPollRequest{
		JobID:         currentPoll.JobID,
		JobName:       currentPoll.JobName,
		Attempt:       currentPoll.Attempt + 1,
		MaxAttempts:   currentPoll.MaxAttempts,
		NextPollAfter: time.Now().Add(delay),
		Status:        "PENDING",
	}

	if err := p.moderationMLRepo.CreatePollRequest(context.Background(), nextPoll); err != nil {
		p.logger.Error("failed to schedule next poll",
			zap.String("job_id", currentPoll.JobID),
			zap.Error(err))
		return err
	}

	p.logger.Debug("scheduled next poll",
		zap.String("job_id", currentPoll.JobID),
		zap.Int("next_attempt", nextPoll.Attempt),
		zap.Time("next_poll_after", nextPoll.NextPollAfter))

	return nil
}

// processJobStatusChange handles training job status updates
func (p *MLTrainingProcessor) processJobStatusChange(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Extract status from new and old images
	oldStatus := getStatusFromStreamImage(record.Change.OldImage)
	newStatus := getStatusFromStreamImage(record.Change.NewImage)

	// Only process if status actually changed
	if oldStatus == newStatus {
		return nil
	}

	// Unmarshal the full job
	var job models.ModelTrainingJob
	if err := stream.UnmarshalItem(record, &job); err != nil {
		p.logger.Error("failed to unmarshal training job",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err))
		return nil
	}

	p.logger.Info("training job status changed",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("job_id", job.JobID),
		zap.String("old_status", oldStatus),
		zap.String("new_status", newStatus),
	)

	// Handle completion or failure
	switch newStatus {
	case statusCompleted:
		return p.handleJobCompletion(ctx, &job)
	case statusFailed, statusTimeout:
		return p.handleJobFailure(ctx, &job)
	default:
		// Other status changes don't require action
		return nil
	}
}

func getStatusFromStreamImage(image map[string]events.DynamoDBAttributeValue) string {
	for _, key := range []string{"status", "Status"} {
		if attr, ok := image[key]; ok && attr.DataType() == events.DataTypeString {
			return attr.String()
		}
	}
	return ""
}

// updateJobStatus updates the training job status in DynamoDB
func (p *MLTrainingProcessor) updateJobStatus(ctx context.Context, jobARN, status string, output *bedrock.GetModelCustomizationJobOutput) error {
	logger := p.logger.With(
		zap.String("job_arn", jobARN),
		zap.String("status", status),
	)

	// Get the existing job
	job, err := p.moderationMLRepo.GetTrainingJob(ctx, jobARN)
	if err != nil {
		logger.Error("failed to get training job", zap.Error(err))
		return err
	}

	// Capture old status before updating (for logging)
	oldStatus := job.Status

	// Update job fields
	job.Status = status

	// Extract additional info from Bedrock response
	if output.OutputModelArn != nil {
		job.ModelARN = aws.ToString(output.OutputModelArn)
	}
	if output.FailureMessage != nil {
		job.ErrorMessage = aws.ToString(output.FailureMessage)
	}

	// Extract training metrics if available
	if status == statusCompleted {
		job.CompletedAt = time.Now()

		// Calculate training time
		if output.CreationTime != nil && output.LastModifiedTime != nil {
			duration := output.LastModifiedTime.Sub(*output.CreationTime)
			job.Metrics.TrainingTime = int(duration.Seconds())
		}

		// Try to extract metrics from Bedrock TrainingMetrics first
		if output.TrainingMetrics != nil {
			logger.Info("extracting training metrics from Bedrock response")

			metrics := p.extractMetricsFromBedrockOutput(output.TrainingMetrics)
			job.Metrics.Accuracy = metrics.Accuracy
			job.Metrics.Precision = metrics.Precision
			job.Metrics.Recall = metrics.Recall
			job.Metrics.F1Score = metrics.F1Score

			logger.Info("successfully extracted metrics from Bedrock TrainingMetrics",
				zap.Float64("accuracy", metrics.Accuracy),
				zap.Float64("precision", metrics.Precision),
				zap.Float64("recall", metrics.Recall),
				zap.Float64("f1_score", metrics.F1Score))
		} else if output.OutputDataConfig != nil && output.OutputDataConfig.S3Uri != nil {
			// Fall back to parsing from S3 output if TrainingMetrics not available
			s3OutputPath := aws.ToString(output.OutputDataConfig.S3Uri)
			logger.Info("TrainingMetrics not available in response, parsing from S3",
				zap.String("s3_uri", s3OutputPath))

			metrics, err := p.parseMetricsFromS3(ctx, s3OutputPath)
			if err != nil {
				logger.Warn("failed to parse metrics from S3, using defaults",
					zap.String("s3_uri", s3OutputPath),
					zap.Error(err))
				// Use default values if parsing fails
				job.Metrics.Accuracy = 0.0
				job.Metrics.Precision = 0.0
				job.Metrics.Recall = 0.0
				job.Metrics.F1Score = 0.0
			} else {
				// Use parsed metrics
				job.Metrics.Accuracy = metrics.Accuracy
				job.Metrics.Precision = metrics.Precision
				job.Metrics.Recall = metrics.Recall
				job.Metrics.F1Score = metrics.F1Score
				logger.Info("successfully parsed metrics from S3",
					zap.Float64("accuracy", metrics.Accuracy),
					zap.Float64("precision", metrics.Precision),
					zap.Float64("recall", metrics.Recall),
					zap.Float64("f1_score", metrics.F1Score))
			}
		} else {
			// No metrics available - use defaults
			logger.Warn("no training metrics available from Bedrock or S3 output",
				zap.String("job_id", job.JobID))
			job.Metrics.Accuracy = 0.0
			job.Metrics.Precision = 0.0
			job.Metrics.Recall = 0.0
			job.Metrics.F1Score = 0.0
		}
	}

	// Update the job in DynamoDB (this triggers the MODIFY stream event)
	if err := p.moderationMLRepo.UpdateTrainingJob(ctx, job); err != nil {
		logger.Error("failed to update training job", zap.Error(err))
		return err
	}

	logger.Info("updated training job status",
		zap.String("old_status", oldStatus),
		zap.String("new_status", status),
	)

	return nil
}

// handleJobCompletion creates a ModerationModelVersion when training completes
func (p *MLTrainingProcessor) handleJobCompletion(ctx *lift.Context, job *models.ModelTrainingJob) error {
	logger := p.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("job_id", job.JobID),
	)

	logger.Info("handling training job completion")

	// Extract model version from ARN
	modelVersion := extractVersionFromARN(job.ModelARN)

	// Deactivate existing model versions
	if err := p.deactivateExistingModels(context.Background()); err != nil {
		logger.Warn("failed to deactivate existing models", zap.Error(err))
		// Non-fatal - continue with creating new version
	}

	// Create new model version record
	modelVersionRecord := &models.ModerationModelVersion{
		VersionID:      modelVersion,
		TrainingJobID:  job.JobID,
		TrainingStatus: statusCompleted,
		Accuracy:       job.Metrics.Accuracy,
		Precision:      job.Metrics.Precision,
		Recall:         job.Metrics.Recall,
		F1Score:        job.Metrics.F1Score,
		SamplesUsed:    job.DatasetSamples,
		TrainingTime:   job.Metrics.TrainingTime,
		IsActive:       true, // Mark as active
		ModelARN:       job.ModelARN,
		Metadata: map[string]interface{}{
			"base_model":  job.BaseModelID,
			"dataset_key": job.DatasetS3Key,
			"job_name":    job.JobName,
			"trained_at":  time.Now().Format(time.RFC3339),
		},
	}

	if err := p.moderationMLRepo.CreateModelVersion(context.Background(), modelVersionRecord); err != nil {
		logger.Error("failed to create model version", zap.Error(err))
		return err
	}

	logger.Info("created new model version",
		zap.String("version_id", modelVersion),
		zap.String("model_arn", job.ModelARN),
		zap.Float64("accuracy", job.Metrics.Accuracy),
		zap.Float64("precision", job.Metrics.Precision),
		zap.Float64("recall", job.Metrics.Recall),
		zap.Float64("f1_score", job.Metrics.F1Score),
	)

	// Emit training completed event
	if err := p.emitTrainingEvent(context.Background(), job.JobID, "MODEL_TRAINING_COMPLETED", map[string]interface{}{
		"job_id":       job.JobID,
		"job_name":     job.JobName,
		"status":       "COMPLETED",
		"model_arn":    job.ModelARN,
		"version_id":   modelVersion,
		"accuracy":     job.Metrics.Accuracy,
		"precision":    job.Metrics.Precision,
		"recall":       job.Metrics.Recall,
		"f1_score":     job.Metrics.F1Score,
		"completed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		logger.Warn("failed to emit training completed event", zap.Error(err))
		// Non-fatal
	}

	return nil
}

// deactivateExistingModels deactivates all currently active model versions
func (p *MLTrainingProcessor) deactivateExistingModels(ctx context.Context) error {
	// Get active model version
	activeModel, err := p.moderationMLRepo.GetActiveModelVersion(ctx)
	if err != nil {
		// No active model found, nothing to deactivate
		return nil
	}

	// Mark as inactive
	activeModel.IsActive = false
	if err := p.moderationMLRepo.UpdateModelVersion(ctx, activeModel); err != nil {
		return fmt.Errorf("failed to deactivate model %s: %w", activeModel.VersionID, err)
	}

	p.logger.Info("deactivated previous model version",
		zap.String("version_id", activeModel.VersionID))

	return nil
}

// handleJobFailure logs training job failures and emits failure event
func (p *MLTrainingProcessor) handleJobFailure(ctx *lift.Context, job *models.ModelTrainingJob) error {
	logger := p.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("job_id", job.JobID),
		zap.String("error", job.ErrorMessage),
	)

	logger.Error("training job failed",
		zap.String("base_model", job.BaseModelID),
		zap.Int("dataset_samples", job.DatasetSamples),
	)

	// Emit training failed event
	if err := p.emitTrainingEvent(context.Background(), job.JobID, "MODEL_TRAINING_FAILED", map[string]interface{}{
		"job_id":    job.JobID,
		"job_name":  job.JobName,
		"status":    "FAILED",
		"error":     job.ErrorMessage,
		"failed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		logger.Warn("failed to emit training failed event", zap.Error(err))
		// Non-fatal
	}

	return nil
}

// markJobAsTimeout marks a job as timed out
func (p *MLTrainingProcessor) markJobAsTimeout(ctx context.Context, jobARN string) {
	job, err := p.moderationMLRepo.GetTrainingJob(ctx, jobARN)
	if err != nil {
		p.logger.Error("failed to get training job for timeout", zap.Error(err))
		return
	}

	job.Status = statusTimeout
	job.ErrorMessage = "Training job polling timed out after maximum attempts"
	job.CompletedAt = time.Now()

	if err := p.moderationMLRepo.UpdateTrainingJob(ctx, job); err != nil {
		p.logger.Error("failed to mark job as timeout", zap.Error(err))
	}
}

// parseMetricsFromS3 downloads and parses training metrics from S3 output
func (p *MLTrainingProcessor) parseMetricsFromS3(ctx context.Context, s3URI string) (*models.TrainingMetrics, error) {
	// Parse S3 URI (format: s3://bucket/key/path/)
	uri := strings.TrimPrefix(s3URI, "s3://")
	parts := strings.SplitN(uri, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid S3 URI format: %s", s3URI)
	}

	bucket := parts[0]
	keyPrefix := parts[1]

	// Look for common metrics file names
	metricsFiles := []string{
		"metrics.json",
		"training_results.json",
		"evaluation_results.json",
		"model_metrics.json",
	}

	var lastErr error
	for _, fileName := range metricsFiles {
		key := strings.TrimSuffix(keyPrefix, "/") + "/" + fileName

		p.logger.Debug("attempting to download metrics file",
			zap.String("bucket", bucket),
			zap.String("key", key))

		// Download metrics file from S3
		getObjectInput := &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}

		result, err := p.s3Client.GetObject(ctx, getObjectInput)
		if err != nil {
			lastErr = err
			p.logger.Debug("metrics file not found",
				zap.String("bucket", bucket),
				zap.String("key", key),
				zap.Error(err))
			continue
		}
		defer func() {
			_ = result.Body.Close()
		}()

		// Read and parse metrics JSON
		var metricsContent strings.Builder
		_, err = io.Copy(&metricsContent, result.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read metrics content: %w", err)
			continue
		}

		// Parse metrics JSON
		metrics, err := parseBedrockMetricsJSON(metricsContent.String())
		if err != nil {
			lastErr = fmt.Errorf("failed to parse metrics JSON: %w", err)
			continue
		}

		p.logger.Info("successfully downloaded and parsed metrics from S3",
			zap.String("bucket", bucket),
			zap.String("key", key))

		return metrics, nil
	}

	// If we get here, none of the metrics files were found
	if lastErr != nil {
		return nil, fmt.Errorf("failed to find or parse metrics from S3: %w", lastErr)
	}
	return nil, fmt.Errorf("no metrics files found in S3 output path: %s", s3URI)
}

// extractMetricsFromBedrockOutput extracts training metrics from Bedrock's TrainingMetrics document
func (p *MLTrainingProcessor) extractMetricsFromBedrockOutput(trainingMetrics *types.TrainingMetrics) *models.TrainingMetrics {
	metrics := &models.TrainingMetrics{}

	if trainingMetrics == nil {
		return metrics
	}

	rawMetrics, err := p.marshalToRawMetrics(trainingMetrics)
	if err != nil {
		return metrics
	}

	// Try extracting from different possible structures
	p.extractDirectMetrics(rawMetrics, metrics)
	p.extractNestedMetrics(rawMetrics, "validation_metrics", metrics)
	p.extractNestedMetrics(rawMetrics, "evaluation", metrics)

	return metrics
}

// marshalToRawMetrics converts TrainingMetrics to a raw map
func (p *MLTrainingProcessor) marshalToRawMetrics(trainingMetrics *types.TrainingMetrics) (map[string]interface{}, error) {
	metricsJSON, err := json.Marshal(trainingMetrics)
	if err != nil {
		p.logger.Warn("failed to marshal TrainingMetrics to JSON", zap.Error(err))
		return nil, err
	}

	var rawMetrics map[string]interface{}
	if err := json.Unmarshal(metricsJSON, &rawMetrics); err != nil {
		p.logger.Warn("failed to unmarshal TrainingMetrics JSON", zap.Error(err))
		return nil, err
	}

	return rawMetrics, nil
}

// extractDirectMetrics extracts metrics from top-level fields
func (p *MLTrainingProcessor) extractDirectMetrics(rawMetrics map[string]interface{}, metrics *models.TrainingMetrics) {
	p.extractFloatMetric(rawMetrics, "accuracy", &metrics.Accuracy)
	p.extractFloatMetric(rawMetrics, "precision", &metrics.Precision)
	p.extractFloatMetric(rawMetrics, "recall", &metrics.Recall)

	// Try f1_score first, then f1
	if !p.extractFloatMetric(rawMetrics, "f1_score", &metrics.F1Score) {
		p.extractFloatMetric(rawMetrics, "f1", &metrics.F1Score)
	}
}

// extractNestedMetrics extracts metrics from nested structures
func (p *MLTrainingProcessor) extractNestedMetrics(rawMetrics map[string]interface{}, key string, metrics *models.TrainingMetrics) {
	nested, ok := rawMetrics[key]
	if !ok {
		return
	}

	metricsMap, ok := nested.(map[string]interface{})
	if !ok {
		return
	}

	p.extractFloatMetric(metricsMap, "accuracy", &metrics.Accuracy)
	p.extractFloatMetric(metricsMap, "precision", &metrics.Precision)
	p.extractFloatMetric(metricsMap, "recall", &metrics.Recall)

	// Try f1_score first, then f1
	if !p.extractFloatMetric(metricsMap, "f1_score", &metrics.F1Score) {
		p.extractFloatMetric(metricsMap, "f1", &metrics.F1Score)
	}
}

// extractFloatMetric extracts a float64 value from a map and assigns it to target
// Returns true if extraction was successful
func (p *MLTrainingProcessor) extractFloatMetric(data map[string]interface{}, key string, target *float64) bool {
	val, ok := data[key]
	if !ok {
		return false
	}

	floatVal, ok := val.(float64)
	if !ok {
		return false
	}

	*target = floatVal
	return true
}

// parseBedrockMetricsJSON parses Bedrock training metrics JSON format
func parseBedrockMetricsJSON(jsonContent string) (*models.TrainingMetrics, error) {
	// Try parsing as a generic JSON object first
	var rawMetrics map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &rawMetrics); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}

	metrics := &models.TrainingMetrics{}

	// Extract metrics from various possible structures
	// Bedrock may use different formats depending on the model and task type

	// Try direct fields
	if val, ok := rawMetrics["accuracy"].(float64); ok {
		metrics.Accuracy = val
	}
	if val, ok := rawMetrics["precision"].(float64); ok {
		metrics.Precision = val
	}
	if val, ok := rawMetrics["recall"].(float64); ok {
		metrics.Recall = val
	}
	if val, ok := rawMetrics["f1_score"].(float64); ok {
		metrics.F1Score = val
	} else if val, ok := rawMetrics["f1"].(float64); ok {
		metrics.F1Score = val
	}

	// Try nested validation_metrics structure
	if validationMetrics, ok := rawMetrics["validation_metrics"].(map[string]interface{}); ok {
		if val, ok := validationMetrics["accuracy"].(float64); ok {
			metrics.Accuracy = val
		}
		if val, ok := validationMetrics["precision"].(float64); ok {
			metrics.Precision = val
		}
		if val, ok := validationMetrics["recall"].(float64); ok {
			metrics.Recall = val
		}
		if val, ok := validationMetrics["f1_score"].(float64); ok {
			metrics.F1Score = val
		} else if val, ok := validationMetrics["f1"].(float64); ok {
			metrics.F1Score = val
		}
	}

	// Try nested evaluation structure
	if evaluation, ok := rawMetrics["evaluation"].(map[string]interface{}); ok {
		if val, ok := evaluation["accuracy"].(float64); ok {
			metrics.Accuracy = val
		}
		if val, ok := evaluation["precision"].(float64); ok {
			metrics.Precision = val
		}
		if val, ok := evaluation["recall"].(float64); ok {
			metrics.Recall = val
		}
		if val, ok := evaluation["f1_score"].(float64); ok {
			metrics.F1Score = val
		} else if val, ok := evaluation["f1"].(float64); ok {
			metrics.F1Score = val
		}
	}

	// Validate that we got at least some metrics
	if metrics.Accuracy == 0 && metrics.Precision == 0 && metrics.Recall == 0 && metrics.F1Score == 0 {
		return nil, fmt.Errorf("no valid metrics found in JSON")
	}

	return metrics, nil
}

// extractVersionFromARN extracts version identifier from model ARN
func extractVersionFromARN(arn string) string {
	// Example ARN: arn:aws:bedrock:us-east-1:123456789012:custom-model/anthropic.claude-v2:1:12k/abcd1234
	arn = strings.TrimSpace(arn)
	if arn == "" {
		return fmt.Sprintf("v%d", time.Now().Unix())
	}

	parts := strings.Split(arn, "/")
	last := parts[len(parts)-1]
	if last != "" {
		return last
	}

	return fmt.Sprintf("v%d", time.Now().Unix())
}

// emitTrainingEvent emits a training lifecycle event to DynamoDB for stream processing
func (p *MLTrainingProcessor) emitTrainingEvent(ctx context.Context, jobID, eventType string, payload map[string]interface{}) error {
	event := &models.StreamingEvent{
		EventID:    fmt.Sprintf("evt_%d_ml_training_%s", time.Now().UnixNano(), jobID),
		EventType:  eventType,
		TargetType: "ml_training",
		TargetID:   jobID,
		Payload:    payload,
		CreatedAt:  time.Now(),
		TTL:        time.Now().Add(24 * time.Hour).Unix(),
	}

	// Update keys for GSI indexing
	event.UpdateKeys()

	// Write to DynamoDB - stream processors will pick it up
	if err := writeStreamingEventFn(ctx, p.db, event); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	p.logger.Debug("emitted training event",
		zap.String("event_type", eventType),
		zap.String("job_id", jobID))

	return nil
}

func main() {
	app := lift.New()
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.RequestID())))
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.Recover())))

	_ = app.DynamoDB("*", func(ctx *lift.Context) error {
		records, err := ctx.DynamoDBRecords()
		if err != nil {
			return err
		}
		return processor.HandleStream(ctx, events.DynamoDBEvent{Records: records})
	})

	lambdaStartFn(app.HandleRequest)
}
