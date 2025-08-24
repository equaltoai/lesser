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

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Service name constants
const (
	serviceMediaConvert = "mediaconvert"
	serviceRekognition  = "rekognition"
	serviceThumbnails   = "thumbnails"
	serviceS3Upload     = "s3_upload"
	serviceS3Storage    = "s3_storage"
	serviceCloudFront   = "cloudfront"
)

// Cost category constants
const (
	costCategoryProcessing = "processing"
	costCategoryStorage    = "storage"
	costCategoryBandwidth  = "bandwidth"
	costCategoryCompute    = "compute"
)

// calculateS3PutCost calculates the cost of S3 PUT operations
func (mp *MediaProcessor) calculateS3PutCost(_ int64) int64 {
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
func (mp *MediaProcessor) generateVideoThumbnails(_ context.Context, event MediaProcessingEvent, plan *TranscodingPlan) int64 {
	// This would integrate with the actual thumbnail generation logic
	// For now, return the estimated cost
	mp.logger.Debug("generating video thumbnails",
		zap.String("media_id", event.MediaID),
		zap.Int("thumbnail_count", plan.ThumbnailCount),
		zap.Int64("thumbnail_cost", plan.ThumbnailCost))

	return plan.ThumbnailCost
}

// trackTranscodingCosts records detailed transcoding costs and metrics
func (mp *MediaProcessor) trackTranscodingCosts(ctx context.Context, metrics *TranscodingJobMetrics) {
	// Track individual cost components (legacy spending transactions)
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

	const formatUnknown = "unknown"

	// NEW: Create enhanced MediaAnalytics with variant-level cost tracking
	if mp.mediaAnalyticsRepo != nil {
		analytics := &models.MediaAnalytics{}
		analytics.SetGeneralEvent("processing_completed", metrics.MediaID, metrics.Username)

		// Set media format based on input format
		format := formatUnknown
		if strings.Contains(metrics.InputFormat, "video") {
			format = "video"
		} else if strings.Contains(metrics.InputFormat, "audio") {
			format = "audio"
		} else if strings.Contains(metrics.InputFormat, "image") {
			format = "image"
		}
		analytics.Format = format

		// Set duration if available
		if metrics.InputDuration > 0 {
			analytics.Duration = float64(metrics.InputDuration) / 1000.0 // Convert ms to seconds
		}

		// Create variant cost entries based on output variants
		for variant := range metrics.OutputVariants {
			variantCost := models.MediaVariantCost{
				VariantKey:       variant,
				ProcessingTimeMs: metrics.ProcessingTimeMs,
				OutputSizeBytes:  metrics.OutputSizes[variant], // if available
			}

			// Parse variant information (assuming format: "resolution_codec_bitrate")
			parts := strings.Split(variant, "_")
			if len(parts) >= 3 {
				variantCost.Resolution = parts[0]
				variantCost.Codec = parts[1]
				_, _ = fmt.Sscanf(parts[2], "%d", &variantCost.Bitrate)
			} else {
				// Fallback for non-standard variant names
				variantCost.Resolution = variant
				variantCost.Codec = "unknown"
			}

			// Distribute costs proportionally across variants using simple division
			// Future enhancement: Use actual processing time/complexity weighting
			variantCount := len(metrics.OutputVariants)
			if variantCount > 0 {
				variantCost.ProcessingCost = metrics.CostBreakdown[serviceMediaConvert] / int64(variantCount)
				variantCost.StorageCost = metrics.CostBreakdown[serviceS3Storage] / int64(variantCount)
				variantCost.TotalCost = variantCost.ProcessingCost + variantCost.StorageCost
			}

			// Set quality metrics (would be populated by actual transcoding results)
			variantCost.CompressionRatio = 0.7 // Default assumption
			variantCost.DeliveryCount = 1      // Initial delivery

			analytics.AddVariantCost(variantCost.Resolution, variantCost.Codec, variantCost.Bitrate, variantCost)
		}

		// If no variants, create a single "original" variant
		if err := common.ValidateSliceNotEmpty("outputVariants", metrics.OutputVariants); err != nil {
			originalCost := models.MediaVariantCost{
				VariantKey:       "original",
				Resolution:       "original",
				Codec:            "original",
				Bitrate:          0,
				ProcessingCost:   metrics.TotalCostMicros,
				StorageCost:      metrics.CostBreakdown[serviceS3Storage],
				TotalCost:        metrics.TotalCostMicros,
				ProcessingTimeMs: metrics.ProcessingTimeMs,
				OutputSizeBytes:  metrics.InputSize,
				CompressionRatio: 1.0,
				DeliveryCount:    1,
			}
			analytics.AddVariantCost("original", "original", 0, originalCost)
		}

		// Set service-specific costs
		analytics.MediaConvertCost = metrics.CostBreakdown[serviceMediaConvert]
		analytics.S3StorageCost = metrics.CostBreakdown[serviceS3Storage]
		analytics.LambdaCost = metrics.CostBreakdown["lambda_processing"]

		// Record the analytics
		if err := mp.mediaAnalyticsRepo.RecordMediaAnalytics(ctx, analytics); err != nil {
			mp.logger.Error("failed to record media analytics with variant costs",
				zap.String("media_id", metrics.MediaID),
				zap.Error(err))
		} else {
			mp.logger.Debug("recorded enhanced media analytics",
				zap.String("media_id", metrics.MediaID),
				zap.String("format", format),
				zap.Int("variant_count", len(analytics.VariantCosts)),
				zap.Int64("total_variant_cost", analytics.TotalVariantCost))
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

}

// getCategoryFromService maps service to cost category
func (mp *MediaProcessor) getCategoryFromService(service string) string {
	switch service {
	case serviceMediaConvert:
		return costCategoryProcessing
	case serviceS3Upload, serviceS3Storage:
		return costCategoryStorage
	case serviceCloudFront:
		return costCategoryBandwidth
	case serviceRekognition:
		return costCategoryProcessing
	case serviceThumbnails:
		return costCategoryProcessing
	default:
		return costCategoryCompute
	}
}

// getOperationFromService maps service to specific operation
func (mp *MediaProcessor) getOperationFromService(service string) string {
	switch service {
	case serviceMediaConvert:
		return "video_transcode"
	case serviceS3Upload:
		return "storage_put"
	case serviceS3Storage:
		return "storage_monthly"
	case serviceCloudFront:
		return "cdn_transfer"
	case serviceRekognition:
		return "content_analysis"
	case serviceThumbnails:
		return "thumbnail_generation"
	default:
		return "media_process"
	}
}

// getUnitsFromService calculates service-specific units consumed
func (mp *MediaProcessor) getUnitsFromService(service string, metrics *TranscodingJobMetrics) int64 {
	switch service {
	case serviceMediaConvert:
		// Return minutes of video processed
		return metrics.InputDuration / (1000 * 60) // Convert ms to minutes
	case serviceRekognition:
		// Return number of images analyzed
		return int64(len(metrics.OutputVariants)) // Approximate
	case serviceThumbnails:
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
	}
	if sizeMB > 50 { // Medium file, likely 720p
		return 1280, 720
	}
	return 854, 480 // Small file, likely SD
}

// estimateVariantStorageSize estimates total storage needed for all transcoded variants
func (mp *MediaProcessor) estimateVariantStorageSize(_ int64, plan *TranscodingPlan) int64 {
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
	if err := common.ValidateRequiredParam("mediaConvertRole", mp.mediaConvertRole); err != nil {
		return "", MediaConvertRoleNotConfigured()
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
		// Safe conversion to int32 with bounds checking
		var maxCaptures int32
		if plan.ThumbnailCount > int(^int32(0)) {
			maxCaptures = ^int32(0) // Max int32
		} else if plan.ThumbnailCount <= 0 {
			maxCaptures = 1 // At least 1 thumbnail
		} else {
			maxCaptures = int32(plan.ThumbnailCount)
		}

		thumbnailOutput := mctypes.Output{
			NameModifier: aws.String("_thumb"),
			VideoDescription: &mctypes.VideoDescription{
				CodecSettings: &mctypes.VideoCodecSettings{
					Codec: mctypes.VideoCodecFrameCapture,
					FrameCaptureSettings: &mctypes.FrameCaptureSettings{
						FramerateNumerator:   int32(1),
						FramerateDenominator: int32(60), // 1 frame every 60 seconds
						MaxCaptures:          maxCaptures,
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
			"username":         event.Username,
			"media_id":         event.MediaID,
			"job_id":           event.JobID,
			"estimated_cost":   fmt.Sprintf("%d", plan.MediaConvertCost),
			"quality_levels":   strings.Join(plan.QualityLevels, ","),
			"thumbnail_count":  fmt.Sprintf("%d", plan.ThumbnailCount),
			"analysis_enabled": fmt.Sprintf("%t", plan.AnalysisEnabled),
			"processing_tier":  "enhanced", // Mark as enhanced processing
		},
	}

	result, err := mp.mediaConvertClient.CreateJob(ctx, createJobInput)
	if err != nil {
		return "", EnhancedMediaConvertJobCreationFailed(err)
	}

	return aws.ToString(result.Job.Id), nil
}

// createQualityOutput creates a MediaConvert output for a specific quality level
func (mp *MediaProcessor) createQualityOutput(quality string) mctypes.Output {
	var width, height, bitrate int32
	nameModifier := fmt.Sprintf("_%s", quality)

	switch quality {
	case "2160p":
		width, height, bitrate = 3840, 2160, 8000000 // 8 Mbps for 4K
	case "1080p":
		width, height, bitrate = 1920, 1080, 5000000 // 5 Mbps for 1080p
	case "720p":
		width, height, bitrate = 1280, 720, 2500000 // 2.5 Mbps for 720p
	case "480p":
		width, height, bitrate = 854, 480, 1000000 // 1 Mbps for 480p
	default:
		width, height, bitrate = 1280, 720, 2500000 // Default to 720p
	}

	return mctypes.Output{
		NameModifier: aws.String(nameModifier),
		VideoDescription: &mctypes.VideoDescription{
			CodecSettings: &mctypes.VideoCodecSettings{
				Codec: mctypes.VideoCodecH264,
				H264Settings: &mctypes.H264Settings{
					Bitrate:         bitrate,
					CodecProfile:    mctypes.H264CodecProfileMain,
					CodecLevel:      mctypes.H264CodecLevelAuto,
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
						Bitrate:      int32(128000),
						SampleRate:   int32(48000),
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
func (mp *MediaProcessor) processAudioWithCostTracking(ctx context.Context, data []byte, event MediaProcessingEvent, _ []string) (ProcessingResult, error) {
	result := ProcessingResult{}

	// Initialize audio processing metrics
	audioMetrics := &TranscodingJobMetrics{
		JobID:          event.JobID,
		MediaID:        event.MediaID,
		Username:       event.Username,
		InputFormat:    "audio/mpeg",
		InputSize:      int64(len(data)),
		OutputVariants: make(map[string]string),
		OutputSizes:    make(map[string]int64),
		CostBreakdown:  make(map[string]int64),
		StartedAt:      time.Now(),
		Status:         "processing",
	}

	// Get user's media processing config
	config := mp.getUserMediaConfig(ctx, event.Username)

	// Check if audio processing is enabled
	if !config.AudioProcessingEnabled {
		mp.logger.Info("audio processing disabled for user", zap.String("username", event.Username))
		return mp.uploadOriginalOnly(ctx, data, event, "audio/mpeg")
	}

	// Check user's remaining budget
	remainingBudget := mp.getUserRemainingBudget(ctx, event.Username)

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
		return result, S3KeySanitizationAudioFailed(err)
	}
	if err := mp.uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
		return result, AudioUploadFailed(err)
	}

	// Track costs
	audioMetrics.CostBreakdown[serviceS3Upload] = uploadCost
	audioMetrics.CostBreakdown[serviceS3Storage] = storageCost
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
	mp.trackTranscodingCosts(ctx, audioMetrics)

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
