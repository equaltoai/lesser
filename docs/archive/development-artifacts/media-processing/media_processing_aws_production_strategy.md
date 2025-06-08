# Media Processing Production Strategy - AWS Managed Services

## Current Challenge
- ffmpeg in Lambda requires custom layers or container images
- Binary dependencies are hard to maintain
- Cold starts with large binaries impact performance
- Not truly serverless

## AWS Managed Service Options

### Option 1: AWS Elemental MediaConvert (Recommended for Video)
**Pros:**
- Fully managed, no binaries to maintain
- Professional-grade video processing
- Automatic thumbnail generation
- Multiple output formats
- Pay per minute of video processed

**Cons:**
- More expensive than Lambda ($0.024/min for HD)
- Overkill for simple metadata extraction
- Async processing only

**Implementation:**
```go
func processVideoWithMediaConvert(ctx context.Context, s3Input string) (ProcessingResult, error) {
    client := mediaconvert.NewFromConfig(cfg)
    
    // Create job for video processing
    input := &mediaconvert.CreateJobInput{
        Role: aws.String(mediaConvertRole),
        Settings: &types.JobSettings{
            Inputs: []types.Input{{
                FileInput: aws.String(s3Input),
            }},
            OutputGroups: []types.OutputGroup{{
                OutputGroupSettings: &types.OutputGroupSettings{
                    FileGroupSettings: &types.FileGroupSettings{
                        Destination: aws.String(s3OutputPath),
                    },
                },
                Outputs: []types.Output{{
                    VideoDescription: &types.VideoDescription{
                        CodecSettings: &types.VideoCodecSettings{
                            H264Settings: &types.H264Settings{
                                RateControlMode: types.H264RateControlModeQvbr,
                                QvbrSettings: &types.H264QvbrSettings{
                                    QvbrQualityLevel: aws.Int32(7),
                                },
                            },
                        },
                    },
                    // Generate thumbnails
                    ContainerSettings: &types.ContainerSettings{
                        Container: types.ContainerTypeMp4,
                    },
                }},
            }},
        },
    }
    
    result, err := client.CreateJob(ctx, input)
    // Poll for completion or use EventBridge
}
```

### Option 2: Lambda Container Images (Pragmatic for All Media)
**Pros:**
- Can include ffmpeg in container (up to 10GB)
- Still serverless pricing model
- Full control over processing
- Works for audio and video

**Cons:**
- Container image to maintain
- Cold starts can be slow
- 15-minute timeout limit

**Implementation:**
```dockerfile
FROM public.ecr.aws/lambda/provided:al2
# Install ffmpeg
RUN yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm
RUN yum install -y ffmpeg
COPY bootstrap ${LAMBDA_RUNTIME_DIR}
COPY main ${LAMBDA_TASK_ROOT}
CMD ["main"]
```

### Option 3: Hybrid Approach (Recommended for Lesser)

**Use Lambda + Lightweight Libraries for Metadata:**
```go
import (
    "github.com/h2non/filetype"
    "github.com/dhowden/tag" // for audio metadata
    "github.com/strukturag/libheif/go/heif" // for image metadata
)

func getMediaMetadataLightweight(ctx context.Context, data []byte) (ProcessingResult, error) {
    result := ProcessingResult{}
    
    // Detect file type
    kind, _ := filetype.Match(data)
    
    switch kind.MIME.Type {
    case "video":
        // For video, we need duration - trigger MediaInfo Lambda
        return triggerMediaInfoLambda(ctx, data)
    case "audio":
        // Use lightweight library for audio metadata
        metadata, err := tag.ReadFrom(bytes.NewReader(data))
        if err == nil && metadata.Raw()["duration"] != nil {
            result.Duration = parseDuration(metadata.Raw()["duration"])
        }
    case "image":
        // Already handled by existing code
    }
    
    return result, nil
}
```

**Use MediaConvert for Heavy Processing:**
- Transcoding
- Thumbnail generation at scale
- Format conversions

### Option 4: AWS Batch with Fargate (For Complex Workflows)
**Pros:**
- No time limits
- Full ffmpeg capabilities
- Cost-effective for batch processing

**Cons:**
- Not real-time
- More complex to set up

## Recommended Architecture for Lesser

```
┌─────────────────┐
│ Media Upload    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Lambda (Fast)   │─────► Metadata extraction
│ Container Image │       (using lightweight libs)
└────────┬────────┘
         │
         ├─── Images ──► Process locally (current code)
         │
         ├─── Audio ───► Extract duration with Go libs
         │              Generate simple waveform
         │
         └─── Video ───► Option A: MediaInfo in Lambda container
                        Option B: MediaConvert job for thumbnails
                        Option C: Return basic info, process async
```

## Implementation Plan

### Phase 1: Quick Win (1 day)
Replace ffmpeg calls with lightweight libraries where possible:
```go
// Audio duration without ffmpeg
import "github.com/tcolgate/mp3"

func getAudioDurationLightweight(data []byte) (int, error) {
    decoder := mp3.NewDecoder(bytes.NewReader(data))
    var duration float64
    var frame mp3.Frame
    
    for {
        if err := decoder.Decode(&frame); err != nil {
            if err == io.EOF {
                break
            }
            return 0, err
        }
        duration += frame.Duration().Seconds()
    }
    
    return int(duration * 1000), nil
}
```

### Phase 2: Container Lambda (3 days)
1. Create Dockerfile with ffmpeg
2. Build and push to ECR
3. Update Lambda to use container
4. Test with production workloads

### Phase 3: MediaConvert Integration (1 week)
1. Set up MediaConvert job templates
2. Implement job submission
3. Handle async callbacks
4. Cost optimization

## Cost Comparison

| Method | Cost per 1000 videos | Processing Time | Maintenance |
|--------|---------------------|-----------------|-------------|
| Lambda + ffmpeg layer | ~$0.50 | 2-5s | High |
| Lambda Container | ~$0.75 | 3-8s | Medium |
| MediaConvert | ~$24.00 | 30-60s | Low |
| Hybrid (metadata only) | ~$0.25 | 1-2s | Low |

## Decision Matrix

For Lesser's use case:
- ✅ **Hybrid approach** for production
- ✅ Extract metadata with lightweight Go libraries
- ✅ Generate simple thumbnails/waveforms in Lambda
- ✅ Defer heavy processing (transcoding) to MediaConvert if needed
- ✅ Use container images only if absolutely necessary

This gives you a true serverless solution without ffmpeg dependency! 