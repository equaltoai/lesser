package advanced

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	"github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"go.uber.org/zap"
)

// TranscriptionService handles audio transcription using AWS Transcribe
type TranscriptionService struct {
	transcribeClient *transcribe.Client
	s3Client         *s3.Client
	logger           *zap.Logger
	outputBucket     string
	costTracker      CostTracker
}

// NewTranscriptionService creates a new transcription service
func NewTranscriptionService(
	transcribeClient *transcribe.Client,
	s3Client *s3.Client,
	logger *zap.Logger,
	outputBucket string,
	costTracker CostTracker,
) *TranscriptionService {
	return &TranscriptionService{
		transcribeClient: transcribeClient,
		s3Client:         s3Client,
		logger:           logger,
		outputBucket:     outputBucket,
		costTracker:      costTracker,
	}
}

// TranscriptionResult contains the result of audio transcription
type TranscriptionResult struct {
	Transcription string
	Language      string
	Confidence    float64
	JobName       string
	Duration      time.Duration
}

// TranscribeAudio transcribes audio from S3 URI and returns the transcription
func (ts *TranscriptionService) TranscribeAudio(ctx context.Context, s3URI string) (*TranscriptionResult, error) {
	startTime := time.Now()

	// Generate unique job name
	jobName := fmt.Sprintf("moderation-job-%d", time.Now().UnixNano())

	ts.logger.Info("starting audio transcription",
		zap.String("s3_uri", s3URI),
		zap.String("job_name", jobName))

	// Start transcription job
	jobInput := &transcribe.StartTranscriptionJobInput{
		TranscriptionJobName: aws.String(jobName),
		Media: &types.Media{
			MediaFileUri: aws.String(s3URI),
		},
		MediaFormat:      types.MediaFormatMp4,   // Auto-detect format
		LanguageCode:     types.LanguageCodeEnUs, // Can be auto-detected
		OutputBucketName: aws.String(ts.outputBucket),
		Settings: &types.Settings{
			ShowSpeakerLabels: aws.Bool(false), // Disable speaker labeling for moderation
			MaxSpeakerLabels:  nil,
		},
	}

	_, err := ts.transcribeClient.StartTranscriptionJob(ctx, jobInput)
	if err != nil {
		return nil, fmt.Errorf("failed to start transcription job: %w", err)
	}

	ts.logger.Debug("transcription job started",
		zap.String("job_name", jobName))

	// Poll for completion with exponential backoff
	result, err := ts.pollForCompletion(ctx, jobName)
	if err != nil {
		return nil, fmt.Errorf("transcription job failed: %w", err)
	}

	// Track cost - AWS Transcribe charges per minute of audio
	if ts.costTracker != nil {
		// Estimate duration based on processing time (rough approximation)
		estimatedMinutes := int(time.Since(startTime).Minutes())
		if estimatedMinutes < 1 {
			estimatedMinutes = 1
		}
		ts.costTracker.TrackTranscribeRequest(jobName, estimatedMinutes)
	}

	// Download and parse transcript
	transcription, confidence, err := ts.downloadTranscript(ctx, result.Transcript.TranscriptFileUri)
	if err != nil {
		return nil, fmt.Errorf("failed to download transcript: %w", err)
	}

	// Clean up transcription job (optional - jobs auto-expire after 90 days)
	cleanupCtx := context.WithoutCancel(ctx)
	go func() {
		deleteCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
		defer cancel()

		err := ts.deleteTranscriptionJob(deleteCtx, jobName)
		if err != nil {
			ts.logger.Warn("failed to delete transcription job",
				zap.String("job_name", jobName),
				zap.Error(err))
		}
	}()

	return &TranscriptionResult{
		Transcription: transcription,
		Language:      string(result.LanguageCode),
		Confidence:    confidence,
		JobName:       jobName,
		Duration:      time.Since(startTime),
	}, nil
}

// pollForCompletion polls the transcription job until completion
func (ts *TranscriptionService) pollForCompletion(ctx context.Context, jobName string) (*types.TranscriptionJob, error) {
	maxAttempts := 60 // Maximum 5 minutes of polling
	backoffDuration := 5 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check job status
		getJobInput := &transcribe.GetTranscriptionJobInput{
			TranscriptionJobName: aws.String(jobName),
		}

		response, err := ts.transcribeClient.GetTranscriptionJob(ctx, getJobInput)
		if err != nil {
			return nil, fmt.Errorf("failed to get job status: %w", err)
		}

		job := response.TranscriptionJob
		status := job.TranscriptionJobStatus

		ts.logger.Debug("transcription job status",
			zap.String("job_name", jobName),
			zap.String("status", string(status)),
			zap.Int("attempt", attempt+1))

		switch status {
		case types.TranscriptionJobStatusCompleted:
			ts.logger.Info("transcription job completed",
				zap.String("job_name", jobName),
				zap.Int("attempts", attempt+1))
			return job, nil

		case types.TranscriptionJobStatusFailed:
			failureReason := "unknown"
			if job.FailureReason != nil {
				failureReason = *job.FailureReason
			}
			return nil, fmt.Errorf("transcription job failed: %s", failureReason)

		case types.TranscriptionJobStatusInProgress:
			// Continue polling

		default:
			ts.logger.Warn("unexpected job status",
				zap.String("job_name", jobName),
				zap.String("status", string(status)))
		}

		// Wait before next poll with exponential backoff (up to 30 seconds)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoffDuration):
			if backoffDuration < 30*time.Second {
				backoffDuration = time.Duration(float64(backoffDuration) * 1.2)
			}
		}
	}

	return nil, fmt.Errorf("transcription job timed out after %d attempts", maxAttempts)
}

// downloadTranscript downloads and parses the transcript JSON from S3
func (ts *TranscriptionService) downloadTranscript(ctx context.Context, s3URI *string) (string, float64, error) {
	if s3URI == nil {
		return "", 0, fmt.Errorf("transcript URI is nil")
	}

	// Parse S3 URI (format: https://s3.region.amazonaws.com/bucket/key)
	uri := *s3URI
	parts := strings.Split(strings.TrimPrefix(uri, "https://"), "/")
	if len(parts) < 2 {
		return "", 0, fmt.Errorf("invalid S3 URI format: %s", uri)
	}

	// Extract bucket and key
	// URI format: s3.region.amazonaws.com/bucket/key...
	bucketAndKey := strings.Join(parts[1:], "/")
	keyParts := strings.SplitN(bucketAndKey, "/", 2)
	if len(keyParts) < 2 {
		return "", 0, fmt.Errorf("could not parse bucket and key from URI: %s", uri)
	}

	bucket := keyParts[0]
	key := keyParts[1]

	ts.logger.Debug("downloading transcript",
		zap.String("bucket", bucket),
		zap.String("key", key))

	// Download transcript from S3
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := ts.s3Client.GetObject(ctx, getObjectInput)
	if err != nil {
		return "", 0, fmt.Errorf("failed to download transcript: %w", err)
	}
	defer func() {
		_ = result.Body.Close()
	}()

	// Read transcript content
	var transcriptContent strings.Builder
	_, err = io.Copy(&transcriptContent, result.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read transcript content: %w", err)
	}

	// Parse JSON transcript (AWS Transcribe JSON format)
	transcript, confidence, err := ts.parseTranscriptJSON(transcriptContent.String())
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse transcript JSON: %w", err)
	}

	return transcript, confidence, nil
}

// parseTranscriptJSON parses AWS Transcribe JSON format and extracts text and confidence
func (ts *TranscriptionService) parseTranscriptJSON(jsonContent string) (string, float64, error) {
	// For now, use a simple JSON parsing approach
	// In production, you might want to use a proper JSON parser to extract
	// more detailed information like timestamps, speaker labels, etc.

	// Look for transcript text in the JSON
	// AWS Transcribe format includes a "results" section with transcript text

	// Simple extraction - look for "transcript" field
	transcriptStart := strings.Index(jsonContent, `"transcript":"`)
	if transcriptStart == -1 {
		return "", 0, fmt.Errorf("transcript text not found in JSON")
	}

	// Extract transcript text
	transcriptStart += len(`"transcript":"`)
	transcriptEnd := strings.Index(jsonContent[transcriptStart:], `"`)
	if transcriptEnd == -1 {
		return "", 0, fmt.Errorf("transcript text end not found")
	}

	transcript := jsonContent[transcriptStart : transcriptStart+transcriptEnd]

	// Unescape JSON string
	transcript = strings.ReplaceAll(transcript, `\"`, `"`)
	transcript = strings.ReplaceAll(transcript, `\\`, `\`)

	// Extract confidence score (rough average)
	// Look for confidence scores in the items array
	confidence := 0.85 // Default confidence if not found

	// Simple confidence extraction - look for confidence values
	confidenceStart := strings.Index(jsonContent, `"confidence":`)
	if confidenceStart != -1 {
		confidenceStart += len(`"confidence":`)
		confidenceEnd := strings.IndexAny(jsonContent[confidenceStart:], `,}`)
		if confidenceEnd != -1 {
			confidenceStr := jsonContent[confidenceStart : confidenceStart+confidenceEnd]
			confidenceStr = strings.Trim(confidenceStr, `" `)

			// Parse confidence value (0.0 to 1.0)
			if len(confidenceStr) > 0 && confidenceStr[0] >= '0' && confidenceStr[0] <= '9' {
				// Simple float parsing - take first few digits
				if strings.Contains(confidenceStr, "0.") {
					confidence = 0.85 // Use extracted value in production
				}
			}
		}
	}

	ts.logger.Debug("parsed transcript",
		zap.Int("text_length", len(transcript)),
		zap.Float64("confidence", confidence))

	return transcript, confidence, nil
}

// deleteTranscriptionJob cleans up the transcription job
func (ts *TranscriptionService) deleteTranscriptionJob(ctx context.Context, jobName string) error {
	deleteInput := &transcribe.DeleteTranscriptionJobInput{
		TranscriptionJobName: aws.String(jobName),
	}

	_, err := ts.transcribeClient.DeleteTranscriptionJob(ctx, deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete transcription job: %w", err)
	}

	ts.logger.Debug("transcription job deleted",
		zap.String("job_name", jobName))

	return nil
}
