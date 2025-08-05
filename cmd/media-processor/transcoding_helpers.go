package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	mctypes "github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// calculateS3PutCost calculates the cost of S3 PUT operations
func (mp *MediaProcessor) calculateS3PutCost(sizeBytes int64) int64 {
	// S3 PUT requests cost $0.0005 per 1,000 requests
	// For transcoding, we typically do 1 PUT per variant + original
	putCost := int64(500) // $0.0005 = 500 microdollars per 1,000 requests
	return putCost / 1000 // Cost per single PUT request
}

// calculateS3StorageCost calculates monthly S3 storage cost (prorated)
func (mp *MediaProcessor) calculateS3StorageCost(sizeBytes int64) int64 {
	// S3 Standard storage: $0.023 per GB per month
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
	monthlyCostMicros := int64(sizeGB * float64(transcodingCosts.S3StorageCost))
	
	// Prorate for current month (daily rate)
	daysInMonth := float64(30) // Average
	dailyCost := float64(monthlyCostMicros) / daysInMonth
	
	return int64(dailyCost) // Return daily storage cost
}

// generateVideoThumbnails generates thumbnails and tracks costs
func (mp *MediaProcessor) generateVideoThumbnails(ctx context.Context, event MediaProcessingEvent, plan *TranscodingPlan) int64 {
	// This would integrate with the actual thumbnail generation logic
	// For now, return the estimated cost
	mp.logger.Debug("generating video thumbnails",
		zap.String("media_id", event.MediaID),
		zap.Int("thumbnail_count", plan.ThumbnailCount),
		zap.Int64("thumbnail_cost", plan.ThumbnailCost))
	
	return plan.ThumbnailCost
}

// trackTranscodingCosts records detailed transcoding costs and metrics
func (mp *MediaProcessor) trackTranscodingCosts(ctx context.Context, metrics *TranscodingJobMetrics) error {
	// Track individual cost components
	for service, cost := range metrics.CostBreakdown {
		transaction := &models.MediaSpendingTransaction{
			UserID:           metrics.Username,
			Username:         metrics.Username,
			CostMicros:       cost,
			Category:         mp.getCategoryFromService(service),
			Service:          service,
			Operation:        mp.getOperationFromService(service),
			Description:      fmt.Sprintf("%s for video transcoding - media %s", service, metrics.MediaID),
			MediaID:          metrics.MediaID,
			JobID:            metrics.JobID,
			FileSize:         metrics.InputSize,
			ContentType:      metrics.InputFormat,
			ProcessingTimeMs: metrics.ProcessingTimeMs,
			BytesProcessed:   metrics.InputSize,
			UnitsConsumed:    mp.getUnitsFromService(service, metrics),
			IsError:          metrics.Status == "failed",
			ErrorMessage:     metrics.ErrorMessage,
		}

		if err := mp.mediaRepo.AddSpendingTransaction(ctx, transaction); err != nil {
			mp.logger.Error("failed to add spending transaction",
				zap.String("service", service),
				zap.Int64("cost", cost),
				zap.Error(err))
			continue
		}
	}

	// Log comprehensive transcoding metrics
	mp.logger.Info("transcoding cost tracking completed",
		zap.String("job_id", metrics.JobID),
		zap.String("media_id", metrics.MediaID),
		zap.String("username", metrics.Username),
		zap.Int64("total_cost_micros", metrics.TotalCostMicros),
		zap.Any("cost_breakdown", metrics.CostBreakdown),
		zap.String("status", metrics.Status))

	return nil
}

// getCategoryFromService maps service to cost category
func (mp *MediaProcessor) getCategoryFromService(service string) string {
	switch service {
	case "mediaconvert":
		return "processing"
	case "s3_upload", "s3_storage":
		return "storage"
	case "cloudfront":
		return "bandwidth"
	case "rekognition":
		return "processing"
	case "thumbnails":
		return "processing"
	default:
		return "compute"
	}
}

// getOperationFromService maps service to specific operation
func (mp *MediaProcessor) getOperationFromService(service string) string {
	switch service {
	case "mediaconvert":
		return "video_transcode"
	case "s3_upload":
		return "storage_put"
	case "s3_storage":
		return "storage_monthly"
	case "cloudfront":
		return "cdn_transfer"
	case "rekognition":
		return "content_analysis"
	case "thumbnails":
		return "thumbnail_generation"
	default:
		return "media_process"
	}
}

// getUnitsFromService calculates service-specific units consumed
func (mp *MediaProcessor) getUnitsFromService(service string, metrics *TranscodingJobMetrics) int64 {
	switch service {
	case "mediaconvert":
		// Return minutes of video processed
		return metrics.InputDuration / (1000 * 60) // Convert ms to minutes
	case "rekognition":
		// Return number of images analyzed
		return int64(len(metrics.OutputVariants)) // Approximate
	case "thumbnails":
		// Return number of thumbnails generated
		durationMinutes := metrics.InputDuration / (1000 * 60)
		thumbnails := durationMinutes + 1
		if thumbnails > 10 {
			thumbnails = 10
		}
		return thumbnails
	default:
		return 1 // Default unit count
	}
}

// getResolutionFromMetrics extracts width/height from job metrics
func getResolutionFromMetrics(metrics *TranscodingJobMetrics) (int, int) {
	// This would parse the actual video metadata
	// For now, return reasonable defaults based on file size
	sizeMB := metrics.InputSize / (1024 * 1024)
	if sizeMB > 100 { // Large file, likely HD or higher
		return 1920, 1080
	} else if sizeMB > 50 { // Medium file, likely 720p
		return 1280, 720
	} else { // Small file, likely SD
		return 854, 480
	}
}

// estimateVariantStorageSize estimates total storage needed for all transcoded variants
func (mp *MediaProcessor) estimateVariantStorageSize(originalSize int64, plan *TranscodingPlan) int64 {
	total := int64(0)
	for _, size := range plan.ExpectedOutputs {
		total += size
	}
	return total
}

// sliceContains checks if a slice contains a string
func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// createEnhancedMediaConvertJob creates a comprehensive MediaConvert job with cost tracking
func (mp *MediaProcessor) createEnhancedMediaConvertJob(ctx context.Context, s3InputKey string, event MediaProcessingEvent, plan *TranscodingPlan) (string, error) {
	if mp.mediaConvertRole == "" {
		return "", fmt.Errorf("MediaConvert role not configured")
	}

	// Define input and output locations
	inputURI := fmt.Sprintf("s3://%s/%s", mp.bucketName, s3InputKey)
	baseOutputKey := fmt.Sprintf("media/%s/%s", event.Username, event.MediaID)
	outputURI := fmt.Sprintf("s3://%s/%s/", mp.bucketName, baseOutputKey)

	// Create outputs for each quality level in the plan
	outputs := []mctypes.Output{}
	for _, quality := range plan.QualityLevels {
		output := mp.createQualityOutput(quality)
		outputs = append(outputs, output)
	}

	// Add thumbnail output if enabled
	thumbnailOutputs := []mctypes.Output{}
	if plan.ThumbnailCount > 0 {
		thumbnailOutput := mctypes.Output{
			NameModifier: aws.String("_thumb"),
			VideoDescription: &mctypes.VideoDescription{
				CodecSettings: &mctypes.VideoCodecSettings{
					Codec: mctypes.VideoCodecFrameCapture,
					FrameCaptureSettings: &mctypes.FrameCaptureSettings{
						FramerateNumerator:   int32(1),
						FramerateDenominator: int32(60), // 1 frame every 60 seconds
						MaxCaptures:          int32(plan.ThumbnailCount),
						Quality:              int32(80),
					},
				},
				Width:  int32(320),
				Height: int32(240),
			},
			ContainerSettings: &mctypes.ContainerSettings{
				Container: mctypes.ContainerTypeRaw,
			},
		}
		thumbnailOutputs = append(thumbnailOutputs, thumbnailOutput)
	}

	// Create job settings with comprehensive outputs
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
				Outputs: outputs,
			},
		},
	}

	// Add thumbnail output group if we have thumbnails
	if len(thumbnailOutputs) > 0 {
		thumbnailGroup := mctypes.OutputGroup{
			Name: aws.String("Thumbnail Group"),
			OutputGroupSettings: &mctypes.OutputGroupSettings{
				Type: mctypes.OutputGroupTypeFileGroupSettings,
				FileGroupSettings: &mctypes.FileGroupSettings{
					Destination: aws.String(outputURI),
				},
			},
			Outputs: thumbnailOutputs,
		}
		jobSettings.OutputGroups = append(jobSettings.OutputGroups, thumbnailGroup)
	}

	// Create the job with enhanced metadata
	createJobInput := &mediaconvert.CreateJobInput{
		Queue:    aws.String(mp.mediaConvertQueue),
		Role:     aws.String(mp.mediaConvertRole),
		Settings: jobSettings,
		UserMetadata: map[string]string{
			"username":          event.Username,
			"media_id":          event.MediaID,
			"job_id":            event.JobID,
			"estimated_cost":    fmt.Sprintf("%d", plan.MediaConvertCost),
			"quality_levels":    strings.Join(plan.QualityLevels, ","),
			"thumbnail_count":   fmt.Sprintf("%d", plan.ThumbnailCount),
			"analysis_enabled":  fmt.Sprintf("%t", plan.AnalysisEnabled),
			"processing_tier":   "enhanced", // Mark as enhanced processing
		},
	}

	result, err := mp.mediaConvertClient.CreateJob(ctx, createJobInput)
	if err != nil {
		return "", fmt.Errorf("failed to create enhanced MediaConvert job: %w", err)
	}

	return aws.ToString(result.Job.Id), nil
}

// createQualityOutput creates a MediaConvert output for a specific quality level
func (mp *MediaProcessor) createQualityOutput(quality string) mctypes.Output {
	var width, height, bitrate int32
	nameModifier := fmt.Sprintf("_%s", quality)

	switch quality {
	case "2160p":
		width, height, bitrate = 3840, 2160, 8000000  // 8 Mbps for 4K
	case "1080p":
		width, height, bitrate = 1920, 1080, 5000000  // 5 Mbps for 1080p
	case "720p":
		width, height, bitrate = 1280, 720, 2500000   // 2.5 Mbps for 720p
	case "480p":
		width, height, bitrate = 854, 480, 1000000    // 1 Mbps for 480p
	default:
		width, height, bitrate = 1280, 720, 2500000   // Default to 720p
	}

	return mctypes.Output{
		NameModifier: aws.String(nameModifier),
		VideoDescription: &mctypes.VideoDescription{
			CodecSettings: &mctypes.VideoCodecSettings{
				Codec: mctypes.VideoCodecH264,
				H264Settings: &mctypes.H264Settings{
					Bitrate: bitrate,
					CodecProfile: mctypes.H264CodecProfileMain,
					CodecLevel: mctypes.H264CodecLevelAuto,
					RateControlMode: mctypes.H264RateControlModeVbr,
				},
			},
			Width:  width,
			Height: height,
		},
		AudioDescriptions: []mctypes.AudioDescription{
			{
				AudioSourceName: aws.String("Audio Selector 1"),
				CodecSettings: &mctypes.AudioCodecSettings{
					Codec: mctypes.AudioCodecAac,
					AacSettings: &mctypes.AacSettings{
						Bitrate:    int32(128000),
						SampleRate: int32(48000),
						CodecProfile: mctypes.AacCodecProfileLc,
					},
				},
			},
		},
		ContainerSettings: &mctypes.ContainerSettings{
			Container: mctypes.ContainerTypeMp4,
		},
	}
}

// processAudioWithCostTracking enhances audio processing with detailed cost tracking
func (mp *MediaProcessor) processAudioWithCostTracking(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []string) (ProcessingResult, error) {
	result := ProcessingResult{}

	// Initialize audio processing metrics
	audioMetrics := &TranscodingJobMetrics{
		JobID:         event.JobID,
		MediaID:       event.MediaID,
		Username:      event.Username,
		InputFormat:   "audio/mpeg",
		InputSize:     int64(len(data)),
		OutputVariants: make(map[string]string),
		OutputSizes:   make(map[string]int64),
		CostBreakdown: make(map[string]int64),
		StartedAt:     time.Now(),
		Status:        "processing",
	}

	// Get user's media processing config
	config, err := mp.getUserMediaConfig(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user media config", zap.Error(err))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// Check if audio processing is enabled
	if !config.AudioProcessingEnabled {
		mp.logger.Info("audio processing disabled for user", zap.String("username", event.Username))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// Check user's remaining budget
	remainingBudget, err := mp.getUserRemainingBudget(ctx, event.Username)
	if err != nil {
		mp.logger.Error("failed to get user budget", zap.Error(err))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// Estimate audio processing costs
	lambdaProcessingCost := mp.estimateAudioProcessingCost(int64(len(data)))
	uploadCost := mp.calculateS3PutCost(int64(len(data)))
	storageCost := mp.calculateS3StorageCost(int64(len(data)))
	totalEstimatedCost := lambdaProcessingCost + uploadCost + storageCost

	if totalEstimatedCost > remainingBudget {
		mp.logger.Warn("user exceeded media budget for audio processing",
			zap.String("username", event.Username),
			zap.Int64("estimated_cost", totalEstimatedCost),
			zap.Int64("remaining_budget", remainingBudget))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// Upload original audio
	audioKey, err := sanitizeS3Key(event.Username, event.MediaID, "audio.mp3")
	if err != nil {
		return result, fmt.Errorf("failed to sanitize S3 key: %w", err)
	}
	if err := mp.uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
		return result, fmt.Errorf("failed to upload audio: %w", err)
	}

	// Track costs
	audioMetrics.CostBreakdown["s3_upload"] = uploadCost
	audioMetrics.CostBreakdown["s3_storage"] = storageCost
	audioMetrics.CostBreakdown["lambda_processing"] = lambdaProcessingCost

	result.Sizes = map[string]SizeInfo{
		"original": {
			URL:   mp.buildMediaURL(audioKey),
			S3Key: audioKey,
		},
	}

	// Extract audio duration using dhowden/tag
	duration, err := extractAudioDuration(data)
	if err != nil {
		mp.logger.Warn("failed to extract audio duration", zap.Error(err))
		duration = 0 // fallback to 0 on error
	}
	result.Duration = duration
	audioMetrics.InputDuration = int64(duration)

	// Calculate total costs
	audioMetrics.TotalCostMicros = 0
	for _, cost := range audioMetrics.CostBreakdown {
		audioMetrics.TotalCostMicros += cost
	}

	// Track detailed audio processing costs
	if err := mp.trackTranscodingCosts(ctx, audioMetrics); err != nil {
		mp.logger.Warn("failed to track audio processing costs", zap.Error(err))
	}

	// Update user's storage usage
	if err := mp.updateStorageUsageForUser(ctx, event.Username, int64(len(data))); err != nil {
		mp.logger.Warn("failed to update storage usage", zap.Error(err))
	}

	// Mark job as completed
	now := time.Now()
	audioMetrics.CompletedAt = &now
	audioMetrics.ProcessingTimeMs = now.Sub(audioMetrics.StartedAt).Milliseconds()
	audioMetrics.Status = "completed"

	mp.logger.Info("audio processing completed with cost tracking",
		zap.String("media_id", event.MediaID),
		zap.String("username", event.Username),
		zap.Int64("total_cost_micros", audioMetrics.TotalCostMicros),
		zap.Int64("processing_time_ms", audioMetrics.ProcessingTimeMs))

	return result, nil
}

// estimateAudioProcessingCost estimates Lambda processing cost for audio
func (mp *MediaProcessor) estimateAudioProcessingCost(sizeBytes int64) int64 {
	// Estimate processing time based on file size
	// Audio processing is typically fast - assume 1 second per MB
	sizeMB := float64(sizeBytes) / (1024 * 1024)
	processingTimeSeconds := sizeMB // 1 second per MB

	// Lambda pricing: $0.0000166667 per GB-second
	// Assume 512MB memory allocation = 0.5GB
	memoryGB := 0.5
	gbSeconds := memoryGB * processingTimeSeconds
	costMicros := int64(gbSeconds * float64(transcodingCosts.LambdaGBSecondCost))

	return costMicros
}