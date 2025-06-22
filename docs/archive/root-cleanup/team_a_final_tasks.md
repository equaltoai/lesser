# Team A Final Tasks - Media Processing Completion

**Mission**: Complete the final 3% of functionality to reach 100% implementation

**Current Status**: 97% complete - Only media processing enhancements remain
**Estimated Time**: 1-2 days
**Build Status**: ✅ Compiling and functional

## Final Implementation Tasks

### Task 1: Complete MediaConvert Integration
**File**: `cmd/media-processor/main.go`
**Current Issue**: Returns error "MediaConvert not configured"
**Lines**: 902-906

#### Implementation Required:
- [ ] Remove placeholder MediaConvert job creation (line 904)
- [ ] Implement actual AWS MediaConvert API calls
- [ ] Configure MediaConvert role and queue
- [ ] Add proper job status tracking
- [ ] Update video dimensions with actual transcoded output
- [ ] Generate multiple quality variants (480p, 720p, 1080p)

#### Expected Outcome:
```go
// Replace this:
logger.Warn("MediaConvert job creation not implemented")
return "", fmt.Errorf("MediaConvert not configured")

// With actual MediaConvert job creation
jobID, err := createMediaConvertJob(ctx, s3Key, bucketName)
if err != nil {
    return "", fmt.Errorf("failed to create MediaConvert job: %w", err)
}
return jobID, nil
```

### Task 2: Implement Audio Duration Extraction  
**File**: `cmd/media-processor/main.go`
**Current Issue**: Returns 0 duration with warning
**Lines**: 491-497

#### Implementation Required:
- [ ] Add audio metadata library dependency (recommend `github.com/dhowden/tag`)
- [ ] Replace placeholder duration extraction
- [ ] Extract audio bitrate, sample rate, and format information
- [ ] Add proper error handling for corrupted audio files

#### Expected Outcome:
```go
// Replace this:
result.Duration = 0
logger.Warn("audio duration extraction not implemented")

// With actual duration extraction:
duration, err := extractAudioDuration(data)
if err != nil {
    logger.Warn("failed to extract audio duration", zap.Error(err))
    duration = 0 // fallback to 0 on error
}
result.Duration = duration
```

### Task 3: Fix Video Placeholder Dimensions
**File**: `cmd/media-processor/main.go`  
**Current Issue**: Returns Width=0, Height=0, Duration=0 for videos
**Lines**: 445-448

#### Implementation Required:
- [ ] Extract video metadata before MediaConvert processing
- [ ] Use `ffprobe` or similar to get actual dimensions
- [ ] Store original dimensions even before transcoding
- [ ] Update dimensions after MediaConvert completion

## Build Requirements (CRITICAL)

### Before Starting Work:
```bash
make fmt && make lint && make build && make test
```

### After Each Task:
```bash
make fmt && make lint && make build && make test
```

### Final Verification:
```bash
make build && echo "✅ Build successful - Task complete"
```

## Testing Requirements

### For MediaConvert Integration:
- [ ] Unit test with mock MediaConvert service
- [ ] Integration test with real video file upload
- [ ] Verify job ID is returned and tracked
- [ ] Test error handling for service failures

### For Audio Duration:
- [ ] Test with various audio formats (MP3, MP4, OGG)
- [ ] Verify duration accuracy within 1 second
- [ ] Test with corrupted/invalid audio files
- [ ] Performance test with large audio files

### For Video Metadata:
- [ ] Test with various video formats (MP4, MOV, AVI)
- [ ] Verify dimensions are extracted correctly
- [ ] Test with unusual aspect ratios
- [ ] Test with very large video files

## Dependencies to Add

Add to `go.mod`:
```go
github.com/dhowden/tag v0.0.0-20230301172012-8fd2cc2c7b7b
github.com/aws/aws-sdk-go-v2/service/mediaconvert v1.30.0
```

## Configuration Required

### Environment Variables:
```bash
MEDIACONVERT_ENDPOINT=https://your-endpoint.mediaconvert.region.amazonaws.com
MEDIACONVERT_ROLE_ARN=arn:aws:iam::account:role/MediaConvertRole
MEDIACONVERT_QUEUE=Default
```

### AWS IAM Permissions:
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "mediaconvert:CreateJob",
                "mediaconvert:GetJob",
                "mediaconvert:ListJobs"
            ],
            "Resource": "*"
        }
    ]
}
```

## Success Criteria

### Task 1 Complete When:
- [ ] Video uploads trigger actual MediaConvert jobs
- [ ] Job IDs are returned and can be tracked
- [ ] Multiple quality variants are generated
- [ ] No "not implemented" warnings in logs

### Task 2 Complete When:
- [ ] Audio files return accurate duration in seconds
- [ ] Multiple audio formats supported
- [ ] No hardcoded 0 duration returns
- [ ] Graceful handling of unsupported formats

### Task 3 Complete When:
- [ ] Video uploads return actual width/height
- [ ] Original dimensions preserved before transcoding
- [ ] No placeholder 0 values for valid videos
- [ ] Metadata extraction works reliably

## Final Deliverable

Upon completion, Team A will have delivered:
- ✅ Complete video transcoding pipeline with AWS MediaConvert
- ✅ Accurate audio duration extraction for all supported formats  
- ✅ Real video metadata extraction with proper dimensions
- ✅ Production-ready media processing system
- ✅ **100% implementation completion for all Team A responsibilities**

## Estimated Timeline

- **Day 1**: MediaConvert integration and configuration
- **Day 2**: Audio duration extraction and video metadata
- **Final**: Testing, cleanup, and verification

**Target Completion**: 48 hours maximum