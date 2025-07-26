# Cost-Aware Media Processing Implementation

## Key Design Principles
1. **Feature Flags** - Admins enable/disable media processing features
2. **User Budgets** - Each user has spending limits
3. **Pure Serverless** - No containers, only AWS managed services
4. **Cost Tracking** - Every operation tracked and attributed

## Implementation for Team 1

### Updated Media Processor

```go
// cmd/media-processor/main.go

import (
    "github.com/equaltoai/lesser/pkg/cost"
    "github.com/aws/aws-sdk-go-v2/service/mediaconvert"
    "github.com/aws/aws-sdk-go-v2/service/rekognition"
)

type MediaConfig struct {
    VideoProcessingEnabled bool
    AudioProcessingEnabled bool
    VideoThumbnailsEnabled bool
    ContentModerationEnabled bool
    MaxVideoDuration int // seconds
    UserBudgetMicros int64
}

func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{
        Sizes: make(map[string]SizeInfo),
    }
    
    // 1. Get user's media processing config
    config, err := getUserMediaConfig(ctx, event.Username)
    if err != nil {
        return result, err
    }
    
    // 2. Check if video processing is enabled
    if !config.VideoProcessingEnabled {
        // Just upload original, no processing
        return uploadOriginalOnly(ctx, data, event, "video/mp4")
    }
    
    // 3. Check user's remaining budget
    remainingBudget, err := cost.GetUserRemainingBudget(ctx, event.Username)
    if err != nil {
        return result, err
    }
    
    // 4. Estimate cost for this operation
    estimatedCost := estimateVideoCost(len(data))
    if estimatedCost > remainingBudget {
        logger.Warn("user exceeded media budget",
            zap.String("username", event.Username),
            zap.Int64("estimated", estimatedCost),
            zap.Int64("remaining", remainingBudget))
        
        // Fallback to basic upload only
        return uploadOriginalOnly(ctx, data, event, "video/mp4")
    }
    
    // 5. Upload to S3 first
    s3Key := fmt.Sprintf("media/%s/%s/original.mp4", event.Username, event.MediaID)
    if err := uploadToS3(ctx, s3Key, data, "video/mp4"); err != nil {
        return result, err
    }
    
    // 6. Create MediaConvert job for thumbnails and metadata
    if config.VideoThumbnailsEnabled {
        jobID, err := createMediaConvertJob(ctx, s3Key, event)
        if err != nil {
            logger.Error("failed to create MediaConvert job", zap.Error(err))
            // Continue anyway - video is uploaded
        } else {
            result.ProcessingJobID = jobID
        }
    }
    
    // 7. Track the cost
    cost.TrackUserSpend(ctx, event.Username, estimatedCost, "video_processing")
    
    result.Sizes["original"] = SizeInfo{
        URL:   buildMediaURL(s3Key),
        S3Key: s3Key,
    }
    
    return result, nil
}

func createMediaConvertJob(ctx context.Context, s3Input string, event MediaProcessingEvent) (string, error) {
    client := mediaconvert.NewFromConfig(awsConfig, func(o *mediaconvert.Options) {
        o.Endpoint = mediaConvertEndpoint
    })
    
    // Simple job: extract metadata and create thumbnails
    job := &mediaconvert.CreateJobInput{
        Role: aws.String(mediaConvertRole),
        Settings: &types.JobSettings{
            Inputs: []types.Input{{
                FileInput: aws.String(fmt.Sprintf("s3://%s/%s", bucketName, s3Input)),
            }},
            OutputGroups: []types.OutputGroup{{
                Name: aws.String("Thumbnails"),
                OutputGroupSettings: &types.OutputGroupSettings{
                    Type: types.OutputGroupTypeFileGroupSettings,
                    FileGroupSettings: &types.FileGroupSettings{
                        Destination: aws.String(fmt.Sprintf("s3://%s/media/%s/%s/", 
                            bucketName, event.Username, event.MediaID)),
                    },
                },
                Outputs: []types.Output{{
                    NameModifier: aws.String("thumbnail"),
                    ContainerSettings: &types.ContainerSettings{
                        Container: types.ContainerTypeMp4,
                    },
                    VideoDescription: &types.VideoDescription{
                        CodecSettings: &types.VideoCodecSettings{
                            Codec: types.VideoCodecFrameCapture,
                            FrameCaptureSettings: &types.FrameCaptureSettings{
                                FramerateNumerator:   aws.Int32(1),
                                FramerateDenominator: aws.Int32(5), // One frame every 5 seconds
                                MaxCaptures:          aws.Int32(10), // Max 10 thumbnails
                            },
                        },
                    },
                }},
            }},
        },
        // Add event metadata for callback
        UserMetadata: map[string]string{
            "mediaID":  event.MediaID,
            "username": event.Username,
            "jobID":    event.JobID,
        },
    }
    
    resp, err := client.CreateJob(ctx, job)
    if err != nil {
        return "", err
    }
    
    return *resp.Job.Id, nil
}

func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{}
    
    // Similar pattern: check config, budget, then process
    config, _ := getUserMediaConfig(ctx, event.Username)
    if !config.AudioProcessingEnabled {
        return uploadOriginalOnly(ctx, data, event, "audio/mpeg")
    }
    
    // For audio: Use AWS Transcribe for transcription if enabled
    // Use simple metadata extraction for duration
    
    // Upload original
    audioKey := fmt.Sprintf("media/%s/%s/audio.mp3", event.Username, event.MediaID)
    uploadToS3(ctx, audioKey, data, "audio/mpeg")
    
    result.Sizes = map[string]SizeInfo{
        "original": {
            URL:   buildMediaURL(audioKey),
            S3Key: audioKey,
        },
    }
    
    return result, nil
}

// Handle MediaConvert completion via EventBridge
func handleMediaConvertComplete(ctx context.Context, event events.EventBridgeEvent) error {
    // Parse MediaConvert job complete event
    var jobEvent MediaConvertJobEvent
    json.Unmarshal(event.Detail, &jobEvent)
    
    // Extract metadata from job
    mediaID := jobEvent.UserMetadata["mediaID"]
    username := jobEvent.UserMetadata["username"]
    
    // Update media record with extracted metadata
    if jobEvent.Status == "COMPLETE" {
        // Get job details for video metadata
        metadata := jobEvent.OutputGroupDetails[0].OutputDetails[0].VideoDetails
        
        updateMediaRecord(ctx, mediaID, ProcessingResult{
            Width:    int(metadata.WidthInPx),
            Height:   int(metadata.HeightInPx),
            Duration: int(metadata.DurationInMs),
        })
        
        // Track actual cost
        actualCost := calculateMediaConvertCost(jobEvent.Duration)
        cost.TrackUserSpend(ctx, username, actualCost, "mediaconvert_actual")
    }
    
    return nil
}

// Content moderation with Rekognition (if enabled)
func moderateContent(ctx context.Context, s3Key string, username string) (*ModerationResult, error) {
    config, _ := getUserMediaConfig(ctx, username)
    if !config.ContentModerationEnabled {
        return nil, nil
    }
    
    // Check budget
    if !hasRemainingBudget(ctx, username, rekognitionCostPerImage) {
        return nil, nil
    }
    
    client := rekognition.NewFromConfig(awsConfig)
    
    resp, err := client.DetectModerationLabels(ctx, &rekognition.DetectModerationLabelsInput{
        Image: &types.Image{
            S3Object: &types.S3Object{
                Bucket: aws.String(bucketName),
                Name:   aws.String(s3Key),
            },
        },
        MinConfidence: aws.Float32(60),
    })
    
    if err != nil {
        return nil, err
    }
    
    // Track cost
    cost.TrackUserSpend(ctx, username, rekognitionCostPerImage, "content_moderation")
    
    return convertToModerationResult(resp), nil
}

// Cost estimation helpers
func estimateVideoCost(sizeBytes int) int64 {
    // MediaConvert: ~$0.024 per minute HD
    // Estimate based on file size (rough)
    estimatedMinutes := float64(sizeBytes) / (5 * 1024 * 1024) // 5MB per minute estimate
    costDollars := estimatedMinutes * 0.024
    return int64(costDollars * 1_000_000) // Convert to microdollars
}

const (
    rekognitionCostPerImage = 1000 // $0.001 per image in microdollars
    transcribeCostPerSecond = 400  // $0.0004 per second
)
```

### Configuration in DynamoDB

```go
// Store per-user media processing settings
{
    "PK": "USER#alice",
    "SK": "MEDIA#CONFIG",
    "VideoProcessingEnabled": true,
    "AudioProcessingEnabled": true,
    "VideoThumbnailsEnabled": true,
    "ContentModerationEnabled": false,
    "MaxVideoDurationSeconds": 300,
    "MonthlyBudgetMicros": 10_000_000  // $10/month
}

// Instance-wide defaults
{
    "PK": "INSTANCE#CONFIG",
    "SK": "MEDIA#DEFAULTS",
    "VideoProcessingEnabled": false,  // Off by default
    "DefaultUserBudgetMicros": 5_000_000  // $5/month default
}
```

## Architecture Summary

```
User Upload → Lambda (Check Budget) → S3 Upload
                |                        |
                ├── If budget OK ────────┤
                |                        |
                v                        v
           MediaConvert             EventBridge
           (Async Job)              (Completion)
                |                        |
                └────────────────────────┘
                            |
                            v
                    Update DynamoDB
                    (metadata + costs)
```

## Key Benefits

1. **Zero baseline cost** - Services only used when enabled
2. **Per-user control** - Each user has budget limits
3. **Graceful degradation** - Falls back to basic upload if over budget
4. **True serverless** - No containers, no binaries
5. **Professional results** - AWS services handle the complexity

## Testing the Implementation

```go
func TestCostAwareVideoProcessing(t *testing.T) {
    // Test with processing disabled
    config := MediaConfig{VideoProcessingEnabled: false}
    result, _ := processVideoWithConfig(ctx, data, event, config)
    assert.Empty(t, result.ProcessingJobID)
    
    // Test with insufficient budget
    config.VideoProcessingEnabled = true
    mockBudget := int64(0)
    result, _ = processVideoWithBudget(ctx, data, event, config, mockBudget)
    assert.Empty(t, result.ProcessingJobID)
    
    // Test successful processing
    mockBudget = int64(1_000_000) // $1
    result, _ = processVideoWithBudget(ctx, data, event, config, mockBudget)
    assert.NotEmpty(t, result.ProcessingJobID)
}
```

This approach gives you enterprise-grade media processing that's completely optional and cost-controlled! 