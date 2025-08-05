# Media Transcoding Cost Tracking Implementation

This document outlines the comprehensive transcoding cost tracking system implemented for the Lesser media processor using DynamORM/Lift patterns.

## Overview

The enhanced media processor now tracks detailed costs for all transcoding operations including:
- AWS MediaConvert jobs (video transcoding)
- Lambda processing time for audio/image processing  
- S3 storage for transcoded files
- CloudFront CDN delivery costs
- Thumbnail generation costs
- Video/image analysis (Rekognition)

## Implementation Details

### 1. Enhanced Cost Tracking Structure

#### New Data Types Added:
- `TranscodingCostTracker` - Global cost rates for different AWS services
- `TranscodingJobMetrics` - Detailed metrics for individual transcoding jobs
- `TranscodingPlan` - Pre-execution cost estimation and quality planning

#### Cost Categories Tracked:
- **MediaConvert**: $0.015 per minute SD, $0.030 per minute HD, $0.045 per minute 4K
- **Lambda processing**: $0.0000166667 per GB-second 
- **S3 storage**: $0.023 per GB/month (prorated daily)
- **CloudFront delivery**: $0.085 per GB
- **Rekognition analysis**: $0.001 per image
- **Thumbnail generation**: $0.0001 per thumbnail

### 2. Enhanced Processing Functions

#### Video Processing (`processVideo`)
- **Pre-processing**: Estimates costs for all planned quality variants
- **Quality Planning**: Determines output resolutions based on input (480p, 720p, 1080p, 4K)
- **Cost Validation**: Checks user budget before starting transcoding
- **Enhanced MediaConvert Jobs**: Creates jobs with multiple quality outputs
- **Detailed Tracking**: Records costs by service and quality level

#### Audio Processing (`processAudioWithCostTracking`)  
- **Lambda Cost Calculation**: Tracks processing time and memory usage
- **Storage Cost Tracking**: S3 PUT operations and storage costs
- **Duration Analysis**: Extracts metadata for accurate cost estimation

#### Image Processing (Enhanced)
- **Variant Cost Tracking**: Tracks costs for each image size variant
- **Processing Cost Calculation**: Lambda execution costs for image manipulation
- **Storage Optimization**: Calculates storage costs for all generated variants

### 3. New Database Models

#### `TranscodingJob` Model (`/pkg/storage/models/transcoding_job.go`)
```go
type TranscodingJob struct {
    // Primary keys: PK="TRANSCODING_JOB#{jobID}", SK="JOB_METRICS"
    // GSI1: User-based queries - PK="USER_TRANSCODING#{userID}"
    // GSI2: Media-based queries - PK="MEDIA_TRANSCODING#{mediaID}"
    
    JobID            string
    MediaID          string  
    UserID           string
    JobType          string            // "video", "audio", "image"
    Status           string            // "processing", "completed", "failed"
    
    // Input/Output details
    InputFormat      string
    InputSize        int64
    OutputVariants   map[string]string // quality -> format
    OutputSizes      map[string]int64  // quality -> size
    
    // Cost breakdown (microdollars)
    TotalCostMicros        int64
    CostBreakdown          map[string]int64 // service -> cost
    MediaConvertCostMicros int64
    S3StorageCostMicros    int64
    LambdaCostMicros       int64
    
    // Efficiency metrics
    CompressionRatio     float64
    CostPerMB           float64
    ProcessingSpeedMBps float64
}
```

#### Enhanced `MediaSpending` Model
- Already existed with comprehensive cost tracking
- Now integrates with detailed transcoding job metrics
- Supports cost aggregation across services and time periods

### 4. Repository Methods Added

#### TranscodingJob Repository Methods:
- `CreateTranscodingJob(ctx, job)` - Creates new transcoding job record
- `GetTranscodingJob(ctx, jobID)` - Retrieves job by ID
- `UpdateTranscodingJob(ctx, job)` - Updates job metrics and costs
- `GetTranscodingJobsByUser(ctx, userID, limit)` - User's transcoding history
- `GetTranscodingJobsByMedia(ctx, mediaID, limit)` - Jobs for specific media
- `GetTranscodingCostsByUser(ctx, userID, timeRange)` - Aggregated cost analysis

### 5. Cost Calculation Functions

#### Key Helper Functions (`/cmd/media-processor/transcoding_helpers.go`):
- `estimateTranscodingCosts()` - Pre-execution cost estimation
- `calculateS3PutCost()` - S3 PUT operation costs
- `calculateS3StorageCost()` - Monthly storage costs (prorated)
- `trackTranscodingCosts()` - Records detailed cost transactions
- `createEnhancedMediaConvertJob()` - Creates multi-quality MediaConvert jobs
- `estimateAudioProcessingCost()` - Lambda processing costs for audio

### 6. Enhanced MediaConvert Integration

#### Multi-Quality Video Processing:
- **4K Input**: Generates 2160p, 1080p, 720p, 480p variants
- **1080p Input**: Generates 1080p, 720p, 480p variants  
- **720p Input**: Generates 720p, 480p variants
- **SD Input**: Generates 480p variant

#### Comprehensive Job Metadata:
```go
UserMetadata: map[string]string{
    "username":          event.Username,
    "media_id":          event.MediaID,
    "estimated_cost":    fmt.Sprintf("%d", plan.MediaConvertCost),
    "quality_levels":    strings.Join(plan.QualityLevels, ","),
    "thumbnail_count":   fmt.Sprintf("%d", plan.ThumbnailCount),
    "analysis_enabled":  fmt.Sprintf("%t", plan.AnalysisEnabled),
    "processing_tier":   "enhanced",
}
```

### 7. Budget Management

#### Pre-Processing Budget Validation:
- Estimates total costs before starting transcoding
- Compares against user's remaining monthly budget
- Falls back to basic upload if budget insufficient
- Tracks budget usage across all cost categories

#### Per-User Budget Limits:
- **Free Tier**: $1/month total, $0.50/month processing
- **Basic Tier**: $10/month total, $5/month processing  
- **Premium Tier**: $50/month total, $30/month processing

### 8. Cost Tracking Integration

#### Transaction-Level Tracking:
```go
transaction := &models.MediaSpendingTransaction{
    UserID:           username,
    CostMicros:       cost,
    Category:         "processing", // or "storage", "bandwidth", "compute"
    Service:          "mediaconvert", // or "s3", "lambda", "rekognition"
    Operation:        "video_transcode",
    MediaID:          mediaID,
    ProcessingTimeMs: processingTime,
    UnitsConsumed:    minutesProcessed,
}
```

#### Aggregated Spending Records:
- Updates existing `MediaSpending` records with new costs
- Tracks costs by service (MediaConvert, S3, Lambda, etc.)
- Monitors budget usage and sends warnings when approaching limits

### 9. Performance Optimizations

#### Cost-Aware Processing:
- Pre-validates user budgets before expensive operations
- Uses estimated costs to determine processing quality levels
- Implements fallback strategies for budget-constrained users

#### Lambda Optimizations:
- Uses DynamORM's Lambda-optimized database client (91% faster cold starts)
- Batches cost tracking operations
- Minimizes database round trips

### 10. Monitoring and Analytics

#### Comprehensive Logging:
```go
mp.logger.Info("transcoding cost tracking completed",
    zap.String("job_id", metrics.JobID),
    zap.String("media_id", metrics.MediaID),
    zap.String("username", metrics.Username),
    zap.Int64("total_cost_micros", metrics.TotalCostMicros),
    zap.Any("cost_breakdown", metrics.CostBreakdown),
    zap.String("status", metrics.Status))
```

#### Cost Analysis Queries:
- User spending history by time period
- Cost breakdown by AWS service
- Efficiency metrics (cost per MB, compression ratios)
- Budget utilization tracking

## Key Benefits

1. **Accurate Cost Attribution**: Every transcoding operation is precisely tracked and attributed to users
2. **Budget Enforcement**: Prevents users from exceeding their spending limits
3. **Cost Optimization**: Quality levels adjusted based on available budget
4. **Detailed Analytics**: Complete visibility into transcoding costs and efficiency
5. **Scalable Architecture**: Uses DynamoDB single-table design for efficient queries
6. **Real-time Monitoring**: Immediate cost tracking and budget validation

## Usage Examples

### Video Transcoding with Cost Tracking:
```go
// Automatic cost estimation and budget validation
estimatedCost := mp.estimateTranscodingCosts(jobMetrics, userConfig)
if estimatedCost > remainingBudget {
    // Fallback to basic upload
    return mp.uploadOriginalOnly(ctx, data, event, mimeType)
}

// Create enhanced MediaConvert job with multiple qualities
jobID := mp.createEnhancedMediaConvertJob(ctx, s3Key, event, transcodingPlan)

// Track all costs by service
mp.trackTranscodingCosts(ctx, jobMetrics)
```

### Cost Analytics Query:
```go
// Get user's transcoding costs for the last month
costs := mediaRepo.GetTranscodingCostsByUser(ctx, userID, "month")
// Returns: {"mediaconvert": 25000, "s3_storage": 5000, "total": 30000}
```

## Files Modified/Created

### Modified Files:
- `/cmd/media-processor/main.go` - Enhanced with comprehensive cost tracking
- `/pkg/storage/repositories/media_repository.go` - Added transcoding job methods
- `/pkg/storage/models/media_spending.go` - Already had comprehensive cost tracking

### New Files:
- `/cmd/media-processor/transcoding_helpers.go` - Cost calculation and tracking functions
- `/pkg/storage/models/transcoding_job.go` - Detailed transcoding job tracking model

## Future Enhancements

1. **Real-time Cost Alerts**: Push notifications when approaching budget limits
2. **Cost Optimization ML**: Machine learning to optimize quality vs. cost trade-offs
3. **Batch Processing**: Optimize costs through batch transcoding operations
4. **Reserved Capacity**: Pre-purchase MediaConvert capacity for cost savings
5. **Cost Prediction**: Predictive analytics for monthly spending forecasts

This implementation provides a comprehensive, production-ready transcoding cost tracking system that aligns with Lesser's goal of providing serverless ActivityPub at 1/100th the operational cost of traditional solutions.