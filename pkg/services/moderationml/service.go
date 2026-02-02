// Package moderationml provides ML-powered moderation capabilities using AWS Bedrock.
package moderationml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// Event types for ML training lifecycle
const (
	EventTypeModelTrainingSubmitted = "MODEL_TRAINING_SUBMITTED"
	EventTypeModelTrainingCompleted = "MODEL_TRAINING_COMPLETED"
	EventTypeModelTrainingFailed    = "MODEL_TRAINING_FAILED"
)

// Service provides ML moderation operations
type Service struct {
	repo             moderationMLRepository
	statusRepo       statusRepository
	db               core.DB
	bedrockClient    bedrockAPI
	bedrockRuntime   bedrockRuntimeAPI
	s3Client         s3API
	logger           *zap.Logger
	trainingBucket   string
	trainingRegion   string
	inferenceModel   string
	guardrailID      string
	guardrailVersion string
	roleARN          string
}

type moderationMLRepository interface {
	CreateSample(ctx context.Context, sample *models.ModerationSample) error
	GetSample(ctx context.Context, sampleID string) (*models.ModerationSample, error)
	GetActiveModelVersion(ctx context.Context) (*models.ModerationModelVersion, error)
	CreatePrediction(ctx context.Context, prediction *models.MLPrediction) error
	GetEffectivenessMetric(ctx context.Context, patternID, period string, startTime time.Time) (*models.ModerationEffectivenessMetric, error)
	GetPredictionsByModelVersion(ctx context.Context, modelVersion string, startTime, endTime time.Time, limit int) ([]*models.MLPrediction, error)
	CreateEffectivenessMetric(ctx context.Context, metric *models.ModerationEffectivenessMetric) error
	CreateTrainingJob(ctx context.Context, job *models.ModelTrainingJob) error
	CreatePollRequest(ctx context.Context, request *models.MLPollRequest) error
}

type statusRepository interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
}

type bedrockAPI interface {
	CreateModelCustomizationJob(ctx context.Context, params *bedrock.CreateModelCustomizationJobInput, optFns ...func(*bedrock.Options)) (*bedrock.CreateModelCustomizationJobOutput, error)
}

type bedrockRuntimeAPI interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

var (
	_ moderationMLRepository = (*repositories.ModerationMLRepository)(nil)
	_ statusRepository       = (*repositories.StatusRepository)(nil)
	_ bedrockAPI             = (*bedrock.Client)(nil)
	_ bedrockRuntimeAPI      = (*bedrockruntime.Client)(nil)
	_ s3API                  = (*s3.Client)(nil)
)

// Config holds configuration for the ML moderation service
type Config struct {
	TrainingBucket       string
	TrainingRegion       string
	InferenceModelID     string
	GuardrailID          string
	GuardrailVersion     string
	CustomizationRoleARN string
}

// NewService creates a new moderation ML service
func NewService(
	repo moderationMLRepository,
	awsCfg aws.Config,
	config Config,
	logger *zap.Logger,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		repo:             repo,
		bedrockClient:    bedrock.NewFromConfig(awsCfg),
		bedrockRuntime:   bedrockruntime.NewFromConfig(awsCfg),
		s3Client:         s3.NewFromConfig(awsCfg),
		logger:           logger,
		trainingBucket:   config.TrainingBucket,
		trainingRegion:   config.TrainingRegion,
		inferenceModel:   config.InferenceModelID,
		guardrailID:      config.GuardrailID,
		guardrailVersion: config.GuardrailVersion,
		roleARN:          config.CustomizationRoleARN,
	}
}

// SetDB sets the DynamoDB database connection for event emission
func (s *Service) SetDB(db core.DB) {
	s.db = db
}

// SetStatusRepository sets the status repository for content fetching
func (s *Service) SetStatusRepository(repo statusRepository) {
	s.statusRepo = repo
}

// SampleInput represents input for creating a training sample
type SampleInput struct {
	ObjectID   string
	ObjectType string
	Label      string
	ReviewerID string
	Confidence float64
	Metadata   map[string]interface{}
}

// QueueSamples adds training samples to the dataset and returns their IDs
func (s *Service) QueueSamples(ctx context.Context, samples []SampleInput) ([]string, error) {
	sampleIDs := make([]string, 0, len(samples))

	for _, input := range samples {
		// Fetch actual content for the sample
		content, err := s.fetchContentForSample(ctx, input.ObjectID, input.ObjectType)
		if err != nil {
			s.logger.Error("failed to fetch content for sample",
				zap.String("object_id", input.ObjectID),
				zap.String("object_type", input.ObjectType),
				zap.Error(err))
			return nil, fmt.Errorf("failed to fetch content for sample %s: %w", input.ObjectID, err)
		}

		// Ensure metadata map exists and add content
		if input.Metadata == nil {
			input.Metadata = make(map[string]interface{})
		}
		input.Metadata["content"] = content

		sample := &models.ModerationSample{
			ObjectID:   input.ObjectID,
			ObjectType: input.ObjectType,
			Label:      input.Label,
			ReviewerID: input.ReviewerID,
			Confidence: input.Confidence,
			Metadata:   input.Metadata,
			Timestamp:  time.Now(),
		}

		if err := s.repo.CreateSample(ctx, sample); err != nil {
			s.logger.Error("failed to queue sample",
				zap.Error(err),
				zap.String("object_id", input.ObjectID))
			return nil, fmt.Errorf("failed to queue sample: %w", err)
		}

		// Sample ID is assigned by the repository during Create
		sampleIDs = append(sampleIDs, sample.ID)
	}

	s.logger.Info("queued training samples",
		zap.Int("count", len(samples)),
		zap.Strings("sample_ids", sampleIDs))
	return sampleIDs, nil
}

// fetchContentForSample retrieves the actual content for a training sample
func (s *Service) fetchContentForSample(ctx context.Context, objectID, objectType string) (string, error) {
	switch objectType {
	case "status":
		if s.statusRepo == nil {
			return "", fmt.Errorf("status repository not configured")
		}

		status, err := s.statusRepo.GetStatus(ctx, objectID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch status: %w", err)
		}

		// Use the cached Content field or extract from Note
		content := status.Content
		if content == "" && status.Note != nil {
			note := status.Note
			content = note.Content
			// Add content warning if present
			if note.Summary != "" {
				content = note.Summary + "\n\n" + content
			}
		}

		if content == "" {
			return "", fmt.Errorf("status %s has no content", objectID)
		}

		return content, nil

	default:
		return "", fmt.Errorf("unsupported object type: %s", objectType)
	}
}

// TrainingOptions holds options for model training
type TrainingOptions struct {
	BaseModelID          string
	DatasetS3Path        string
	OutputS3Path         string
	HyperParameters      map[string]string
	MaxTrainingTime      int
	EarlyStoppingEnabled bool
}

// TrainingResult holds the result of a training job submission
type TrainingResult struct {
	Success      bool
	Status       string   // SUBMITTED, IN_PROGRESS, COMPLETED, FAILED
	JobID        string   // Bedrock job ARN
	JobName      string   // Human-readable job name
	DatasetS3Key string   // S3 key of training dataset
	ModelVersion string   // Model version (empty when SUBMITTED)
	ModelARN     string   // Model ARN (empty when SUBMITTED)
	Accuracy     float64  // Training accuracy (0 when SUBMITTED)
	Precision    float64  // Training precision (0 when SUBMITTED)
	Recall       float64  // Training recall (0 when SUBMITTED)
	F1Score      float64  // Training F1 score (0 when SUBMITTED)
	SamplesUsed  int      // Number of training samples
	TrainingTime int      // Training duration in seconds (0 when SUBMITTED)
	Improvements []string // Improvements (empty when SUBMITTED)
}

// TrainModel launches a Bedrock model customization job asynchronously
// Returns immediately with SUBMITTED status; completion handled via event bus
func (s *Service) TrainModel(ctx context.Context, tenantID, initiatedBy string, sampleIDs []string, options TrainingOptions) (*TrainingResult, error) {
	s.logger.Info("starting async model training",
		zap.String("tenant_id", tenantID),
		zap.String("initiated_by", initiatedBy),
		zap.Int("sample_count", len(sampleIDs)),
		zap.String("base_model", options.BaseModelID))

	// 1. Prepare training dataset from samples and capture S3 key
	datasetS3Key, err := s.prepareTrainingDataset(ctx, sampleIDs, options.DatasetS3Path)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare training dataset: %w", err)
	}

	// 2. Update options with captured S3 key
	options.DatasetS3Path = datasetS3Key

	// 3. Launch Bedrock training job
	jobARN, jobName, err := s.launchBedrockTraining(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to launch bedrock training: %w", err)
	}

	// 4. Create ModelTrainingJob record for tracking
	trainingJob := &models.ModelTrainingJob{
		JobID:          jobARN,
		JobName:        jobName,
		Status:         "SUBMITTED",
		TenantID:       tenantID,
		InitiatedBy:    initiatedBy,
		DatasetS3Key:   datasetS3Key,
		DatasetSamples: len(sampleIDs),
		BaseModelID:    options.BaseModelID,
		StartedAt:      time.Now(),
		Metadata: map[string]interface{}{
			"hyperparameters": options.HyperParameters,
			"output_path":     options.OutputS3Path,
		},
	}

	if err := s.repo.CreateTrainingJob(ctx, trainingJob); err != nil {
		s.logger.Error("failed to create training job record", zap.Error(err))
		// Non-fatal - job was launched
	}

	// 5. Create a poll request record - this will be picked up by the ml-training-processor
	pollRequest := &models.MLPollRequest{
		JobID:         jobARN,
		JobName:       jobName,
		Attempt:       0,
		MaxAttempts:   120,                              // 2 hours max (60s intervals)
		NextPollAfter: time.Now().Add(10 * time.Second), // First poll in 10s
		Status:        "PENDING",
	}

	if err := s.repo.CreatePollRequest(ctx, pollRequest); err != nil {
		s.logger.Error("failed to create poll request", zap.Error(err))
		// Non-fatal - job was launched and record created
	}

	// Emit training submitted event for downstream consumers
	if err := s.emitTrainingEvent(ctx, jobARN, EventTypeModelTrainingSubmitted, map[string]interface{}{
		"job_id":         jobARN,
		"job_name":       jobName,
		"status":         "SUBMITTED",
		"tenant_id":      tenantID,
		"user_id":        initiatedBy,
		"samples_used":   len(sampleIDs),
		"dataset_s3_key": datasetS3Key,
		"created_at":     time.Now().Format(time.RFC3339),
	}); err != nil {
		s.logger.Warn("failed to emit training submitted event", zap.Error(err))
		// Non-fatal - job was submitted successfully
	}

	// 6. Return immediately with SUBMITTED status
	result := &TrainingResult{
		Success:      true,
		Status:       "SUBMITTED",
		JobID:        jobARN,
		JobName:      jobName,
		DatasetS3Key: datasetS3Key,
		SamplesUsed:  len(sampleIDs),
		ModelVersion: "", // Not yet known
		ModelARN:     "", // Not yet known
		Accuracy:     0,
		Precision:    0,
		Recall:       0,
		F1Score:      0,
		TrainingTime: 0,
		Improvements: []string{},
	}

	s.logger.Info("model training job submitted",
		zap.String("job_id", jobARN),
		zap.String("job_name", jobName),
		zap.String("dataset_key", datasetS3Key))

	return result, nil
}

// ScoreContentInput represents content to be scored
type ScoreContentInput struct {
	Content      string
	ContentType  string
	ObjectID     string // For prediction tracking
	ObjectType   string // For prediction tracking
	UseGuardrail bool
}

// ScoreResult represents the ML moderation score
type ScoreResult struct {
	Score            float64
	Labels           map[string]float64
	GuardrailBlocked bool
	GuardrailReason  string
	ModelVersion     string
}

// ScoreContent scores content using the active ML model
func (s *Service) ScoreContent(ctx context.Context, input ScoreContentInput) (*ScoreResult, error) {
	// Get active model
	activeModel, err := s.repo.GetActiveModelVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("no active model available: %w", err)
	}

	// Prepare inference prompt in Claude format
	prompt := fmt.Sprintf("Human: Classify this content for moderation: %s\n\nAssistant:", input.Content)

	inferenceInput := map[string]interface{}{
		"prompt":               prompt,
		"max_tokens_to_sample": 100,
		"temperature":          0.0,
	}

	inputJSON, err := json.Marshal(inferenceInput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	// Build Bedrock runtime request
	invokeInput := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(activeModel.ModelARN),
		Body:        inputJSON,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	}

	// Add guardrail if requested
	if input.UseGuardrail && s.guardrailID != "" {
		invokeInput.GuardrailIdentifier = aws.String(s.guardrailID)
		invokeInput.GuardrailVersion = aws.String(s.guardrailVersion)
	}

	// Invoke model
	output, err := s.bedrockRuntime.InvokeModel(ctx, invokeInput)
	if err != nil {
		// Check if error is due to guardrail intervention
		errMsg := err.Error()
		if strings.Contains(errMsg, "guardrail") || strings.Contains(errMsg, "content policy") {
			s.logger.Warn("content blocked by guardrail",
				zap.Error(err),
				zap.String("content_preview", truncateString(input.Content, 100)))

			result := &ScoreResult{
				Score:            1.0, // Maximum risk score
				Labels:           map[string]float64{"blocked": 1.0},
				GuardrailBlocked: true,
				GuardrailReason:  errMsg,
				ModelVersion:     activeModel.VersionID,
			}

			// Track this prediction
			if err := s.trackPrediction(ctx, input, result); err != nil {
				s.logger.Warn("failed to track guardrail prediction", zap.Error(err))
			}

			return result, nil
		}
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	// Parse response
	var response struct {
		Completion string `json:"completion"`
		Stop       string `json:"stop_reason,omitempty"`
	}
	if err := json.Unmarshal(output.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse inference response: %w", err)
	}

	// Parse the completion to extract label and confidence
	// Format expected: " spam" or " clean" etc.
	label := strings.TrimSpace(response.Completion)
	labels := map[string]float64{
		label: 1.0, // The model's chosen label gets confidence 1.0
	}

	// Calculate overall risk score based on label
	score := calculateRiskScore(label)

	result := &ScoreResult{
		Score:        score,
		Labels:       labels,
		ModelVersion: activeModel.VersionID,
	}

	// Track this prediction for effectiveness metrics
	if err := s.trackPrediction(ctx, input, result); err != nil {
		s.logger.Warn("failed to track prediction", zap.Error(err))
		// Non-fatal - prediction succeeded even if tracking failed
	}

	s.logger.Debug("content scored",
		zap.Float64("score", result.Score),
		zap.String("label", label),
		zap.Bool("guardrail_blocked", result.GuardrailBlocked))

	return result, nil
}

// trackPrediction creates a prediction record for effectiveness tracking
func (s *Service) trackPrediction(ctx context.Context, input ScoreContentInput, result *ScoreResult) error {
	if input.ObjectID == "" || input.ObjectType == "" {
		// Skip tracking if object info not provided
		return nil
	}

	// Determine predicted label from labels map
	predictedLabel := "unknown"
	highestConfidence := 0.0
	for label, confidence := range result.Labels {
		if confidence > highestConfidence {
			predictedLabel = label
			highestConfidence = confidence
		}
	}

	prediction := &models.MLPrediction{
		PredictionID:   uuid.New().String(),
		ObjectID:       input.ObjectID,
		ObjectType:     input.ObjectType,
		ModelVersion:   result.ModelVersion,
		PredictedLabel: predictedLabel,
		Confidence:     result.Score,
		Reviewed:       false,
		Metadata: map[string]interface{}{
			"guardrail_blocked": result.GuardrailBlocked,
			"guardrail_reason":  result.GuardrailReason,
			"labels":            result.Labels,
		},
	}

	if err := s.repo.CreatePrediction(ctx, prediction); err != nil {
		return fmt.Errorf("failed to create prediction record: %w", err)
	}

	s.logger.Debug("tracked ML prediction",
		zap.String("prediction_id", prediction.PredictionID),
		zap.String("object_id", input.ObjectID),
		zap.String("predicted_label", predictedLabel),
		zap.Float64("confidence", result.Score))

	return nil
}

// calculateRiskScore maps a moderation label to a risk score (0.0 = safe, 1.0 = high risk)
func calculateRiskScore(label string) float64 {
	label = strings.ToLower(label)
	switch label {
	case "clean", "safe", "neutral":
		return 0.0
	case "spam", "low_quality":
		return 0.5
	case "hate_speech", "violence", "sexual", "harassment":
		return 0.9
	case "illegal", "csam", "terrorism":
		return 1.0
	case "blocked": // Guardrail blocked
		return 1.0
	default:
		return 0.3 // Unknown labels get moderate risk
	}
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetEffectiveness retrieves effectiveness metrics for a pattern/model
func (s *Service) GetEffectiveness(ctx context.Context, patternID string, period string) (*models.ModerationEffectivenessMetric, error) {
	// Compute time range based on period
	endTime := time.Now()
	var startTime time.Time

	switch period {
	case "hourly":
		startTime = endTime.Add(-1 * time.Hour)
	case "daily":
		startTime = endTime.Add(-24 * time.Hour)
	case "weekly":
		startTime = endTime.Add(-7 * 24 * time.Hour)
	case "monthly":
		startTime = endTime.Add(-30 * 24 * time.Hour)
	default:
		startTime = endTime.Add(-24 * time.Hour) // Default to daily
	}

	metric, err := s.repo.GetEffectivenessMetric(ctx, patternID, period, startTime)
	if err != nil {
		// If not found, try to compute it
		return s.computeEffectiveness(ctx, patternID, period, startTime, endTime)
	}

	return metric, nil
}

// prepareTrainingDataset prepares samples as JSONL for Bedrock training
// Returns the S3 key where the dataset was uploaded
func (s *Service) prepareTrainingDataset(ctx context.Context, sampleIDs []string, s3Path string) (string, error) {
	s.logger.Info("preparing training dataset", zap.Int("sample_count", len(sampleIDs)))

	// Fetch samples from repository
	var jsonlLines []string
	validSamples := 0

	for _, sampleID := range sampleIDs {
		sample, err := s.repo.GetSample(ctx, sampleID)
		if err != nil {
			s.logger.Warn("failed to fetch sample", zap.String("id", sampleID), zap.Error(err))
			continue
		}

		// Extract content from metadata - ERROR if missing
		content := ""
		if sample.Metadata != nil {
			if c, ok := sample.Metadata["content"].(string); ok {
				content = c
			}
		}
		if content == "" {
			// ERROR: Content is required for training
			return "", fmt.Errorf("sample %s has no content - cannot proceed with training", sampleID)
		}

		// Format for Bedrock fine-tuning (Claude/Titan format)
		// For moderation tasks, we use prompt-completion pairs
		trainingExample := map[string]interface{}{
			"prompt":     fmt.Sprintf("Human: Classify this content for moderation: %s\n\nAssistant:", content),
			"completion": fmt.Sprintf(" %s", sample.Label),
		}

		jsonBytes, err := json.Marshal(trainingExample)
		if err != nil {
			s.logger.Warn("failed to marshal training example",
				zap.String("sample_id", sampleID),
				zap.Error(err))
			continue
		}

		jsonlLines = append(jsonlLines, string(jsonBytes))
		validSamples++
	}

	if validSamples == 0 {
		return "", fmt.Errorf("no valid samples for training (requested: %d, valid: %d)", len(sampleIDs), validSamples)
	}

	// Create JSONL content
	jsonlContent := strings.Join(jsonlLines, "\n")

	// Determine S3 path if not provided
	if s3Path == "" {
		timestamp := time.Now().Format("20060102-150405")
		jobID := uuid.New().String()[:8]
		s3Path = fmt.Sprintf("training-data/moderation-%s-%s.jsonl", timestamp, jobID)
	}

	// Upload to S3
	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(s.trainingBucket),
		Key:         aws.String(s3Path),
		Body:        bytes.NewReader([]byte(jsonlContent)),
		ContentType: aws.String("application/jsonlines"),
	}

	_, err := s.s3Client.PutObject(ctx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload training dataset to S3: %w", err)
	}

	s.logger.Info("prepared and uploaded training dataset",
		zap.Int("samples", validSamples),
		zap.String("bucket", s.trainingBucket),
		zap.String("path", s3Path),
		zap.Int("size_bytes", len(jsonlContent)))

	return s3Path, nil
}

// launchBedrockTraining launches a Bedrock model customization job
// Returns jobARN and jobName
func (s *Service) launchBedrockTraining(ctx context.Context, options TrainingOptions) (string, string, error) {
	s.logger.Info("launching bedrock training job", zap.String("base_model", options.BaseModelID))

	// Generate unique job name
	jobName := fmt.Sprintf("moderation-training-%d", time.Now().Unix())

	// Prepare training data config
	trainingDataS3Uri := fmt.Sprintf("s3://%s/%s", s.trainingBucket, options.DatasetS3Path)
	outputDataS3Uri := fmt.Sprintf("s3://%s/models/%s", s.trainingBucket, jobName)
	if options.OutputS3Path != "" {
		outputDataS3Uri = fmt.Sprintf("s3://%s/%s", s.trainingBucket, options.OutputS3Path)
	}

	// Build training data config
	trainingDataConfig := &types.TrainingDataConfig{
		S3Uri: aws.String(trainingDataS3Uri),
	}

	// Build output data config
	outputDataConfig := &types.OutputDataConfig{
		S3Uri: aws.String(outputDataS3Uri),
	}

	// Prepare hyperparameters (defaults for fine-tuning)
	hyperParams := options.HyperParameters
	if hyperParams == nil {
		hyperParams = map[string]string{
			"epochCount":              "3",
			"batchSize":               "1",
			"learningRate":            "0.00001",
			"learningRateWarmupSteps": "0",
		}
	}

	// Use config roleARN or fail
	roleArn := s.roleARN
	if roleArn == "" {
		return "", "", fmt.Errorf("BEDROCK_CUSTOMIZATION_ROLE_ARN not configured")
	}

	// Create model customization job
	input := &bedrock.CreateModelCustomizationJobInput{
		JobName:             aws.String(jobName),
		CustomModelName:     aws.String(fmt.Sprintf("moderation-model-%d", time.Now().Unix())),
		BaseModelIdentifier: aws.String(options.BaseModelID),
		TrainingDataConfig:  trainingDataConfig,
		OutputDataConfig:    outputDataConfig,
		HyperParameters:     hyperParams,
		RoleArn:             aws.String(roleArn),
	}

	output, err := s.bedrockClient.CreateModelCustomizationJob(ctx, input)
	if err != nil {
		return "", "", fmt.Errorf("failed to create bedrock customization job: %w", err)
	}

	jobArn := aws.ToString(output.JobArn)
	s.logger.Info("launched bedrock training job",
		zap.String("job_arn", jobArn),
		zap.String("job_name", jobName),
		zap.String("base_model", options.BaseModelID),
		zap.String("role_arn", roleArn))

	return jobArn, jobName, nil
}

// computeEffectiveness computes effectiveness metrics for a time range
func (s *Service) computeEffectiveness(ctx context.Context, patternID, period string, startTime, endTime time.Time) (*models.ModerationEffectivenessMetric, error) {
	s.logger.Info("computing effectiveness metrics",
		zap.String("pattern_id", patternID),
		zap.String("period", period),
		zap.Time("start", startTime),
		zap.Time("end", endTime))

	// Query predictions for this model version in the time range
	predictions, err := s.repo.GetPredictionsByModelVersion(ctx, patternID, startTime, endTime, 1000)
	if err != nil {
		s.logger.Warn("failed to query predictions for effectiveness", zap.Error(err))
		predictions = []*models.MLPrediction{} // Fall back to empty
	}

	var truePositives, falsePositives, trueNegatives, falseNegatives int

	// Define positive labels (problematic content)
	positiveLabels := map[string]bool{
		"spam":        true,
		"hate_speech": true,
		"violence":    true,
		"harassment":  true,
		"illegal":     true,
		"blocked":     true,
	}

	// Count predictions that have been reviewed by humans
	for _, pred := range predictions {
		if !pred.Reviewed || pred.HumanLabel == "" {
			// Skip predictions that haven't been reviewed
			continue
		}

		isActuallyPositive := positiveLabels[pred.HumanLabel]
		isPredictedPositive := positiveLabels[pred.PredictedLabel]

		switch {
		case isActuallyPositive && isPredictedPositive:
			truePositives++
		case !isActuallyPositive && isPredictedPositive:
			falsePositives++
		case !isActuallyPositive && !isPredictedPositive:
			trueNegatives++
		case isActuallyPositive && !isPredictedPositive:
			falseNegatives++
		}
	}

	// If no reviewed predictions found, return baseline metrics
	if truePositives+falsePositives+trueNegatives+falseNegatives == 0 {
		s.logger.Warn("no reviewed predictions found for effectiveness computation")
	}

	metric := &models.ModerationEffectivenessMetric{
		PatternID:      patternID,
		Period:         period,
		StartTime:      startTime,
		EndTime:        endTime,
		TruePositives:  truePositives,
		FalsePositives: falsePositives,
		TrueNegatives:  trueNegatives,
		FalseNegatives: falseNegatives,
	}

	// Calculate derived metrics
	metric.CalculateMetrics()

	// Save to repository
	if err := s.repo.CreateEffectivenessMetric(ctx, metric); err != nil {
		s.logger.Warn("failed to save effectiveness metric", zap.Error(err))
		// Non-fatal - still return the computed metric
	}

	s.logger.Info("computed effectiveness metrics",
		zap.String("pattern_id", patternID),
		zap.Int("tp", truePositives),
		zap.Int("fp", falsePositives),
		zap.Int("tn", trueNegatives),
		zap.Int("fn", falseNegatives),
		zap.Float64("precision", metric.Precision),
		zap.Float64("recall", metric.Recall),
		zap.Float64("f1", metric.F1Score))

	return metric, nil
}

// emitTrainingEvent emits a training lifecycle event to DynamoDB for stream processing
func (s *Service) emitTrainingEvent(ctx context.Context, jobID, eventType string, payload map[string]interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db not configured for event emission")
	}

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
	if err := s.db.WithContext(ctx).Model(event).Create(); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	s.logger.Debug("emitted training event",
		zap.String("event_type", eventType),
		zap.String("job_id", jobID))

	return nil
}
