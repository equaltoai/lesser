// Package transcoding provides AWS MediaConvert integration for video transcoding
package transcoding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Service provides AWS MediaConvert transcoding operations
type Service struct {
	client            mediaConvertAPI
	logger            *zap.Logger
	endpoint          string
	role              string
	destinationBucket string
	destinationPrefix string
	queue             string
}

type mediaConvertAPI interface {
	CreateJob(ctx context.Context, params *mediaconvert.CreateJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.CreateJobOutput, error)
	GetJob(ctx context.Context, params *mediaconvert.GetJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.GetJobOutput, error)
	CancelJob(ctx context.Context, params *mediaconvert.CancelJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.CancelJobOutput, error)
}

// Config holds configuration for the transcoding service
type Config struct {
	Endpoint          string // MediaConvert endpoint
	Role              string // IAM role ARN for MediaConvert
	DestinationBucket string // S3 bucket for output
	DestinationPrefix string // S3 prefix for output
	Queue             string // MediaConvert queue ARN (optional)
}

// TranscodeRequest represents a request to transcode media
type TranscodeRequest struct {
	MediaID        string
	UserID         string
	Username       string
	SourceBucket   string
	SourceKey      string
	ContentType    string
	Duration       int
	Width          int
	Height         int
	QualityLevels  []string // ["480p", "720p", "1080p"]
	GenerateHLS    bool
	GenerateDASH   bool
	ThumbnailCount int
}

// TranscodeResult contains the results of a transcode job submission
type TranscodeResult struct {
	JobID             string
	MediaConvertJobID string
	OutputBucket      string
	OutputPrefix      string
	EstimatedCostUSD  float64
	EstimatedDuration time.Duration
	QualityLevels     []string
	Status            string
}

// JobStatus represents the status of a transcoding job
type JobStatus struct {
	JobID             string
	MediaConvertJobID string
	Status            string // "SUBMITTED", "PROGRESSING", "COMPLETE", "ERROR"
	PercentComplete   int
	ErrorMessage      string
	Outputs           []OutputInfo
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

// OutputInfo contains information about a transcoded output
type OutputInfo struct {
	Quality     string
	Format      string
	S3Key       string
	Width       int
	Height      int
	Bitrate     int
	FileSize    int64
	ManifestURL string
}

var (
	// ErrInvalidRequest is returned when the transcode request is invalid
	ErrInvalidRequest = errors.New("invalid transcode request")
	// ErrJobNotFound is returned when a job is not found
	ErrJobNotFound = errors.New("transcode job not found")
	// ErrServiceUnavailable is returned when MediaConvert is unavailable
	ErrServiceUnavailable = errors.New("transcoding service unavailable")
)

// NewService creates a new transcoding service
func NewService(awsConfig aws.Config, config Config, logger *zap.Logger) (*Service, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create MediaConvert client with custom endpoint if provided
	var client mediaConvertAPI
	if config.Endpoint != "" {
		// nolint:staticcheck // Using deprecated Endpoint for backward compatibility
		client = mediaconvert.NewFromConfig(awsConfig, func(o *mediaconvert.Options) {
			o.EndpointResolver = mediaconvert.EndpointResolverFunc(
				func(_ string, _ mediaconvert.EndpointResolverOptions) (aws.Endpoint, error) {
					return aws.Endpoint{URL: config.Endpoint}, nil
				})
		})
	} else {
		client = mediaconvert.NewFromConfig(awsConfig)
	}

	return &Service{
		client:            client,
		logger:            logger,
		endpoint:          config.Endpoint,
		role:              config.Role,
		destinationBucket: config.DestinationBucket,
		destinationPrefix: config.DestinationPrefix,
		queue:             config.Queue,
	}, nil
}

// SubmitJob submits a transcoding job to AWS MediaConvert
func (s *Service) SubmitJob(ctx context.Context, req *TranscodeRequest) (*TranscodeResult, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, errors.Join(ErrInvalidRequest, err)
	}

	jobID := uuid.New().String()
	s.logger.Info("submitting transcode job",
		zap.String("job_id", jobID),
		zap.String("media_id", req.MediaID),
		zap.String("user_id", req.UserID))

	// Build MediaConvert job settings
	settings := s.buildJobSettings(req)

	// Create job input
	input := &mediaconvert.CreateJobInput{
		Role:     aws.String(s.role),
		Settings: settings,
		UserMetadata: map[string]string{
			"job_id":   jobID,
			"media_id": req.MediaID,
			"user_id":  req.UserID,
			"username": req.Username,
		},
	}

	// Add queue if configured
	if s.queue != "" {
		input.Queue = aws.String(s.queue)
	}

	// Submit job to MediaConvert
	output, err := s.client.CreateJob(ctx, input)
	if err != nil {
		s.logger.Error("failed to submit MediaConvert job",
			zap.String("job_id", jobID),
			zap.Error(err))
		return nil, errors.Join(ErrServiceUnavailable, err)
	}

	// Calculate estimated cost (rough approximation)
	estimatedCost := s.estimateCost(req)

	result := &TranscodeResult{
		JobID:             jobID,
		MediaConvertJobID: aws.ToString(output.Job.Id),
		OutputBucket:      s.destinationBucket,
		OutputPrefix:      s.getOutputPrefix(req.MediaID),
		EstimatedCostUSD:  estimatedCost,
		EstimatedDuration: s.estimateDuration(req),
		QualityLevels:     req.QualityLevels,
		Status:            string(output.Job.Status),
	}

	s.logger.Info("transcode job submitted",
		zap.String("job_id", jobID),
		zap.String("mediaconvert_job_id", result.MediaConvertJobID),
		zap.Float64("estimated_cost_usd", estimatedCost))

	return result, nil
}

// GetJobStatus retrieves the status of a transcoding job
func (s *Service) GetJobStatus(ctx context.Context, mediaConvertJobID string) (*JobStatus, error) {
	input := &mediaconvert.GetJobInput{
		Id: aws.String(mediaConvertJobID),
	}

	output, err := s.client.GetJob(ctx, input)
	if err != nil {
		s.logger.Error("failed to get job status",
			zap.String("mediaconvert_job_id", mediaConvertJobID),
			zap.Error(err))
		return nil, errors.Join(ErrJobNotFound, err)
	}

	job := output.Job
	status := &JobStatus{
		MediaConvertJobID: aws.ToString(job.Id),
		Status:            string(job.Status),
		PercentComplete:   int(aws.ToInt32(job.JobPercentComplete)),
		CreatedAt:         aws.ToTime(job.CreatedAt),
	}

	// Extract job ID from user metadata
	if jobID, ok := job.UserMetadata["job_id"]; ok {
		status.JobID = jobID
	}

	// Add error message if job failed
	if job.ErrorMessage != nil {
		status.ErrorMessage = aws.ToString(job.ErrorMessage)
	}

	// Add completed time if job is done
	if job.Timing != nil && job.Timing.FinishTime != nil {
		completedAt := aws.ToTime(job.Timing.FinishTime)
		status.CompletedAt = &completedAt
	}

	// Parse outputs if job is complete
	if job.Status == types.JobStatusComplete && job.OutputGroupDetails != nil {
		status.Outputs = s.parseOutputs(job.OutputGroupDetails)
	}

	return status, nil
}

// CancelJob cancels a transcoding job
func (s *Service) CancelJob(ctx context.Context, mediaConvertJobID string) error {
	input := &mediaconvert.CancelJobInput{
		Id: aws.String(mediaConvertJobID),
	}

	_, err := s.client.CancelJob(ctx, input)
	if err != nil {
		s.logger.Error("failed to cancel job",
			zap.String("mediaconvert_job_id", mediaConvertJobID),
			zap.Error(err))
		return errors.Join(ErrServiceUnavailable, err)
	}

	s.logger.Info("job cancelled", zap.String("mediaconvert_job_id", mediaConvertJobID))
	return nil
}

// buildJobSettings builds the MediaConvert job settings
func (s *Service) buildJobSettings(req *TranscodeRequest) *types.JobSettings {
	sourceURL := fmt.Sprintf("s3://%s/%s", req.SourceBucket, req.SourceKey)
	outputPrefix := s.getOutputPrefix(req.MediaID)

	settings := &types.JobSettings{
		Inputs: []types.Input{
			{
				FileInput: aws.String(sourceURL),
				VideoSelector: &types.VideoSelector{
					ColorSpace: types.ColorSpaceFollow,
				},
				AudioSelectors: map[string]types.AudioSelector{
					"Audio Selector 1": {
						DefaultSelection: types.AudioDefaultSelectionDefault,
					},
				},
			},
		},
		OutputGroups: []types.OutputGroup{},
	}

	// Add HLS output group if requested
	if req.GenerateHLS {
		hlsGroup := s.buildHLSOutputGroup(outputPrefix, req.QualityLevels)
		settings.OutputGroups = append(settings.OutputGroups, hlsGroup)
	}

	// Add DASH output group if requested
	if req.GenerateDASH {
		dashGroup := s.buildDASHOutputGroup(outputPrefix, req.QualityLevels)
		settings.OutputGroups = append(settings.OutputGroups, dashGroup)
	}

	// Add thumbnail output group if requested
	if req.ThumbnailCount > 0 {
		thumbnailGroup := s.buildThumbnailOutputGroup(outputPrefix, req.ThumbnailCount)
		settings.OutputGroups = append(settings.OutputGroups, thumbnailGroup)
	}

	return settings
}

// buildHLSOutputGroup builds an HLS output group with multiple quality levels
func (s *Service) buildHLSOutputGroup(outputPrefix string, qualityLevels []string) types.OutputGroup {
	destination := fmt.Sprintf("s3://%s/%s/hls/", s.destinationBucket, outputPrefix)

	outputs := make([]types.Output, 0, len(qualityLevels))
	for _, quality := range qualityLevels {
		output := s.buildHLSOutput(quality)
		outputs = append(outputs, output)
	}

	return types.OutputGroup{
		Name: aws.String("HLS Group"),
		OutputGroupSettings: &types.OutputGroupSettings{
			Type: types.OutputGroupTypeHlsGroupSettings,
			HlsGroupSettings: &types.HlsGroupSettings{
				Destination:            aws.String(destination),
				SegmentLength:          aws.Int32(6),
				MinSegmentLength:       aws.Int32(0),
				ManifestDurationFormat: types.HlsManifestDurationFormatInteger,
				OutputSelection:        types.HlsOutputSelectionManifestsAndSegments,
				SegmentControl:         types.HlsSegmentControlSegmentedFiles,
			},
		},
		Outputs: outputs,
	}
}

// buildHLSOutput builds a single HLS output for a given quality level
func (s *Service) buildHLSOutput(quality string) types.Output {
	// Map quality to resolution and bitrate
	width, height, bitrate := s.getQualityParams(quality)

	return types.Output{
		NameModifier: aws.String(fmt.Sprintf("_%s", quality)),
		ContainerSettings: &types.ContainerSettings{
			Container: types.ContainerTypeM3u8,
		},
		VideoDescription: &types.VideoDescription{
			CodecSettings: &types.VideoCodecSettings{
				Codec: types.VideoCodecH264,
				H264Settings: &types.H264Settings{
					RateControlMode:    types.H264RateControlModeQvbr,
					MaxBitrate:         safeInt32(bitrate),
					QualityTuningLevel: types.H264QualityTuningLevelSinglePass,
				},
			},
			Width:  safeInt32(width),
			Height: safeInt32(height),
		},
		AudioDescriptions: []types.AudioDescription{
			{
				CodecSettings: &types.AudioCodecSettings{
					Codec: types.AudioCodecAac,
					AacSettings: &types.AacSettings{
						Bitrate:    aws.Int32(128000),
						CodingMode: types.AacCodingModeCodingMode20,
						SampleRate: aws.Int32(48000),
					},
				},
			},
		},
	}
}

// buildDASHOutputGroup builds a DASH output group with multiple quality levels
func (s *Service) buildDASHOutputGroup(outputPrefix string, qualityLevels []string) types.OutputGroup {
	destination := fmt.Sprintf("s3://%s/%s/dash/", s.destinationBucket, outputPrefix)

	outputs := make([]types.Output, 0, len(qualityLevels))
	for _, quality := range qualityLevels {
		output := s.buildDASHOutput(quality)
		outputs = append(outputs, output)
	}

	return types.OutputGroup{
		Name: aws.String("DASH Group"),
		OutputGroupSettings: &types.OutputGroupSettings{
			Type: types.OutputGroupTypeDashIsoGroupSettings,
			DashIsoGroupSettings: &types.DashIsoGroupSettings{
				Destination:    aws.String(destination),
				SegmentLength:  aws.Int32(6),
				FragmentLength: aws.Int32(2),
				SegmentControl: types.DashIsoSegmentControlSegmentedFiles,
			},
		},
		Outputs: outputs,
	}
}

// buildDASHOutput builds a single DASH output for a given quality level
func (s *Service) buildDASHOutput(quality string) types.Output {
	width, height, bitrate := s.getQualityParams(quality)

	return types.Output{
		NameModifier: aws.String(fmt.Sprintf("_%s", quality)),
		ContainerSettings: &types.ContainerSettings{
			Container: types.ContainerTypeMpd,
		},
		VideoDescription: &types.VideoDescription{
			CodecSettings: &types.VideoCodecSettings{
				Codec: types.VideoCodecH264,
				H264Settings: &types.H264Settings{
					RateControlMode: types.H264RateControlModeQvbr,
					MaxBitrate:      safeInt32(bitrate),
				},
			},
			Width:  safeInt32(width),
			Height: safeInt32(height),
		},
		AudioDescriptions: []types.AudioDescription{
			{
				CodecSettings: &types.AudioCodecSettings{
					Codec: types.AudioCodecAac,
					AacSettings: &types.AacSettings{
						Bitrate:    aws.Int32(128000),
						SampleRate: aws.Int32(48000),
					},
				},
			},
		},
	}
}

// buildThumbnailOutputGroup builds a thumbnail output group
func (s *Service) buildThumbnailOutputGroup(outputPrefix string, count int) types.OutputGroup {
	destination := fmt.Sprintf("s3://%s/%s/thumbnails/thumb", s.destinationBucket, outputPrefix)

	return types.OutputGroup{
		Name: aws.String("Thumbnail Group"),
		OutputGroupSettings: &types.OutputGroupSettings{
			Type: types.OutputGroupTypeFileGroupSettings,
			FileGroupSettings: &types.FileGroupSettings{
				Destination: aws.String(destination),
			},
		},
		Outputs: []types.Output{
			{
				ContainerSettings: &types.ContainerSettings{
					Container: types.ContainerTypeRaw,
				},
				VideoDescription: &types.VideoDescription{
					CodecSettings: &types.VideoCodecSettings{
						Codec: types.VideoCodecFrameCapture,
						FrameCaptureSettings: &types.FrameCaptureSettings{
							FramerateNumerator:   aws.Int32(1),
							FramerateDenominator: aws.Int32(5), // 1 frame every 5 seconds
							MaxCaptures:          safeInt32(count),
							Quality:              aws.Int32(80),
						},
					},
					Width:  safeInt32(1280),
					Height: safeInt32(720),
				},
			},
		},
	}
}

// getQualityParams returns width, height, and bitrate for a quality level
func (s *Service) getQualityParams(quality string) (width, height, bitrate int) {
	switch quality {
	case Quality2160p, Quality4K:
		return 3840, 2160, 15000000
	case Quality1080p:
		return 1920, 1080, 5000000
	case Quality720p:
		return 1280, 720, 3000000
	case Quality480p:
		return 854, 480, 1500000
	case Quality360p:
		return 640, 360, 800000
	case Quality240p:
		return 426, 240, 400000
	default:
		return 1280, 720, 3000000 // Default to 720p
	}
}

// getOutputPrefix generates the S3 output prefix for a media ID
func (s *Service) getOutputPrefix(mediaID string) string {
	if s.destinationPrefix != "" {
		return fmt.Sprintf("%s/%s", s.destinationPrefix, mediaID)
	}
	return mediaID
}

// validateRequest validates a transcode request
func (s *Service) validateRequest(req *TranscodeRequest) error {
	if req.MediaID == "" {
		return fmt.Errorf("media_id is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if req.SourceBucket == "" {
		return fmt.Errorf("source_bucket is required")
	}
	if req.SourceKey == "" {
		return fmt.Errorf("source_key is required")
	}
	if len(req.QualityLevels) == 0 {
		return fmt.Errorf("at least one quality level is required")
	}
	if !req.GenerateHLS && !req.GenerateDASH {
		return fmt.Errorf("at least one output format (HLS or DASH) is required")
	}
	return nil
}

// estimateCost estimates the cost of transcoding in USD
func (s *Service) estimateCost(req *TranscodeRequest) float64 {
	// MediaConvert pricing (rough approximation):
	// - HD (720p and below): $0.015 per minute
	// - SD (480p and below): $0.0075 per minute
	// - UHD (4K): $0.060 per minute

	durationMinutes := float64(req.Duration) / 60.0

	// Adjust based on quality levels
	hasUHD := false
	hasHD := false
	for _, quality := range req.QualityLevels {
		if quality == Quality2160p || quality == Quality4K {
			hasUHD = true
		}
		if quality == Quality1080p || quality == Quality720p {
			hasHD = true
		}
	}

	var costPerMinute float64
	if hasUHD {
		costPerMinute = 0.060
	} else if hasHD {
		costPerMinute = 0.015
	} else {
		costPerMinute = 0.0075
	}

	// Multiply by number of quality levels
	totalCost := durationMinutes * costPerMinute * float64(len(req.QualityLevels))

	return totalCost
}

// safeInt32 safely converts int to *int32 with bounds checking
func safeInt32(i int) *int32 {
	if i > 2147483647 {
		i = 2147483647
	}
	if i < -2147483648 {
		i = -2147483648
	}
	val := int32(i) // #nosec G115 -- bounds checked above
	return aws.Int32(val)
}

// estimateDuration estimates the transcoding duration
func (s *Service) estimateDuration(req *TranscodeRequest) time.Duration {
	// Rough estimate: transcoding takes about 1/10 to 1/5 of the video duration
	// depending on complexity and quality
	durationSeconds := req.Duration
	estimatedSeconds := durationSeconds / 5 * len(req.QualityLevels)
	return time.Duration(estimatedSeconds) * time.Second
}

// parseOutputs parses the output group details into OutputInfo structs
func (s *Service) parseOutputs(outputGroupDetails []types.OutputGroupDetail) []OutputInfo {
	outputs := make([]OutputInfo, 0)

	for _, group := range outputGroupDetails {
		for _, detail := range group.OutputDetails {
			if detail.VideoDetails == nil {
				continue
			}

			info := OutputInfo{
				Width:  int(aws.ToInt32(detail.VideoDetails.WidthInPx)),
				Height: int(aws.ToInt32(detail.VideoDetails.HeightInPx)),
			}

			// Try to parse output info from JSON (if available)
			if detail.VideoDetails != nil {
				jsonBytes, _ := json.Marshal(detail)
				s.logger.Debug("output detail", zap.ByteString("detail", jsonBytes))
			}

			outputs = append(outputs, info)
		}
	}

	return outputs
}

// ConvertToTranscodingJob converts a transcode result to a storage model
func (s *Service) ConvertToTranscodingJob(req *TranscodeRequest, result *TranscodeResult) *models.TranscodingJob {
	now := time.Now()

	job := &models.TranscodingJob{
		JobID:               result.JobID,
		MediaID:             req.MediaID,
		UserID:              req.UserID,
		Username:            req.Username,
		JobType:             "video",
		Status:              "processing",
		InputFormat:         req.ContentType,
		InputDuration:       int64(req.Duration * 1000), // Convert to milliseconds
		InputResolution:     fmt.Sprintf("%dx%d", req.Width, req.Height),
		QualityLevels:       result.QualityLevels,
		MediaConvertJobID:   result.MediaConvertJobID,
		StartedAt:           now,
		CreatedAt:           now,
		UpdatedAt:           now,
		EstimatedCostMicros: int64(result.EstimatedCostUSD * 1000000),
	}

	return job
}
