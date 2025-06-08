# Practical Media Processing Implementation for Lesser

## What Team 1 Should Actually Implement

### 1. Fix the Immediate Problem (Day 1)
Remove the hardcoded stubs and make media uploads work:

```go
// cmd/media-processor/main.go

func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
    result := ProcessingResult{
        Sizes: make(map[string]SizeInfo),
    }
    
    // REMOVE hardcoded values
    // result.Width = 1920   // DELETE THIS
    // result.Height = 1080  // DELETE THIS
    // result.Duration = 30000 // DELETE THIS
    
    // For now, just upload the original and mark as processing
    videoKey := fmt.Sprintf("media/%s/%s/video.mp4", event.Username, event.MediaID)
    if err := uploadToS3(ctx, videoKey, data, "video/mp4"); err != nil {
        return result, fmt.Errorf("failed to upload video: %w", err)
    }
    
    result.Sizes["original"] = SizeInfo{
        URL:   buildMediaURL(videoKey),
        S3Key: videoKey,
    }
    
    // Mark for async processing
    result.Width = 0  // 0 means "not yet processed"
    result.Height = 0
    result.Duration = 0
    
    logger.Info("video uploaded, metadata pending",
        zap.String("media_id", event.MediaID))
    
    return result, nil
}

func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
    result := ProcessingResult{}
    
    // Try to get duration with lightweight library
    duration, err := getAudioDurationFromMetadata(data)
    if err != nil {
        logger.Warn("could not extract audio duration", zap.Error(err))
        duration = 0 // 0 means unknown
    }
    result.Duration = duration
    
    // Upload original
    audioKey := fmt.Sprintf("media/%s/%s/audio.mp3", event.Username, event.MediaID)
    if err := uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
        return result, fmt.Errorf("failed to upload audio: %w", err)
    }
    
    result.Sizes = map[string]SizeInfo{
        "original": {
            URL:   buildMediaURL(audioKey),
            S3Key: audioKey,
        },
    }
    
    return result, nil
}

// Helper function using lightweight library
func getAudioDurationFromMetadata(data []byte) (int, error) {
    // Try tag metadata first (fastest)
    metadata, err := tag.ReadFrom(bytes.NewReader(data))
    if err == nil {
        // Check for duration in metadata
        if raw := metadata.Raw(); raw != nil {
            if duration, ok := raw["duration"]; ok {
                // Convert to milliseconds
                if d, ok := duration.(float64); ok {
                    return int(d * 1000), nil
                }
            }
        }
    }
    
    // If no metadata, return 0 (unknown)
    return 0, nil
}
```

### 2. Add go.mod Dependencies

```bash
go get github.com/dhowden/tag
```

### 3. Update Media Record Handling

```go
// In updateMediaRecord function, handle 0 values appropriately
if result.Width == 0 && result.Height == 0 {
    // Don't update dimensions if they're unknown
    // Remove width/height from update expression
}
```

### 4. API Response Handling

The API should handle media with unknown metadata gracefully:

```go
// In the API response
{
    "id": "media123",
    "type": "video",
    "url": "https://cdn.example.com/media/user/media123/video.mp4",
    "preview_url": null,  // No thumbnail yet
    "meta": {
        "width": null,    // Unknown
        "height": null,   // Unknown
        "duration": null  // Unknown
    },
    "processing": true   // New field to indicate processing status
}
```

### 5. Future Enhancement (Not Part of Week 3-4)

Create a separate Lambda for async video processing:

```go
// cmd/video-metadata-extractor/main.go (FUTURE)
// This could use MediaInfo binary or MediaConvert
// Triggered by EventBridge when video uploads complete
```

## Testing Your Implementation

```go
func TestProcessVideoWithoutFFmpeg(t *testing.T) {
    // Test that video uploads work
    data := []byte("fake video data")
    event := MediaProcessingEvent{
        MediaID: "test123",
        Username: "testuser",
    }
    
    result, err := processVideo(context.Background(), data, event, nil)
    
    assert.NoError(t, err)
    assert.Equal(t, 0, result.Width)  // Unknown dimensions are OK
    assert.Equal(t, 0, result.Height)
    assert.Equal(t, 0, result.Duration)
    assert.NotEmpty(t, result.Sizes["original"].URL)
}
```

## What This Achieves

1. ✅ Removes hardcoded stub values
2. ✅ Makes media upload functional
3. ✅ Has a clear production deployment path
4. ✅ Doesn't block on video metadata extraction
5. ✅ Audio duration works for some formats
6. ✅ No ffmpeg dependency

## What Gets Deferred

1. ⏸️ Video thumbnails (use MediaConvert later)
2. ⏸️ Video duration extraction (needs heavy processing)
3. ⏸️ Audio waveforms (nice-to-have)
4. ⏸️ Video transcoding (use MediaConvert later)

## Key Message for Team 1

**Don't let perfect be the enemy of good.** Get media uploads working without stubs, even if metadata extraction is limited. The system can function with unknown video dimensions - it's better than hardcoded fake values.

Focus on making the core flow work:
1. User uploads media ✅
2. Media is stored in S3 ✅
3. URL is returned to user ✅
4. Media can be viewed/played ✅

Everything else is enhancement. 