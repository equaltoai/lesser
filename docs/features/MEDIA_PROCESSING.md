# Lesser Media Processing Pipeline

Lesser implements a comprehensive, serverless media processing pipeline that handles images, videos, audio, and animated content with production-grade reliability and cost optimization. This document details the complete media architecture and processing workflows.

## Architecture Overview

Lesser's media processing system is designed for serverless efficiency, cost optimization, and scalable performance:

### Core Components
- **Media Processor Lambda** (`cmd/media-processor/main.go`)
- **AWS MediaConvert** integration for video transcoding  
- **S3 Storage** with CloudFront CDN distribution
- **DynamoDB** for metadata and job tracking
- **SQS** for reliable async processing
- **Cost Tracking** for every processing operation

### Serverless Design Principles
```go
// Package streaming provides serverless-optimized media streaming functionality.
//
// SERVERLESS DESIGN PRINCIPLES:
//
//  1. No Background Processes: This package avoids long-running goroutines,
//     timers, or polling mechanisms that are incompatible with Lambda's
//     execution model.
//
//  2. DynamoDB TTL for Cleanup: Session and cache expiration is handled
//     automatically by DynamoDB TTL rather than manual cleanup processes.
//
//  3. Stateless Operations: Each Lambda invocation operates independently
//     without relying on shared in-memory state between invocations.
//
//  4. Cost-Optimized: Uses DynamoDB on-demand billing and avoids unnecessary
//     operations that would increase costs in a serverless environment.
```

## Supported Media Types

### Image Processing
**Supported Formats**: JPEG, PNG, WebP, GIF
**Processing Features**:
- Thumbnail generation (multiple sizes)
- Format optimization and conversion
- Metadata extraction and EXIF processing
- Blurhash generation for progressive loading
- Content-aware compression

#### Image Processing Pipeline
```go
const (
    mediaTypeImage = "image"
    ImageProcessingTimeout = 30 * time.Second
)

// MIME types supported
const (
    mimeJPEG = "image/jpeg"
    mimePNG  = "image/png"
    mimeWebP = "image/webp"
)
```

### Video Processing  
**Supported Formats**: MP4, MOV, WebM, AVI
**Processing Features**:
- Multiple quality transcoding (240p, 480p, 720p, 1080p)
- Adaptive bitrate streaming (HLS/DASH)
- Thumbnail/poster frame extraction
- Video metadata analysis
- Codec optimization (H.264, H.265, VP9)

#### Video Processing Configuration
```go
const (
    mediaTypeVideo = "video"
    SmallVideoTimeout = 2 * time.Minute  // Videos < 10MB
    LargeVideoTimeout = 5 * time.Minute  // Videos >= 10MB
)
```

### Audio Processing
**Supported Formats**: MP3, WAV, FLAC, OGG
**Processing Features**:
- Format normalization
- Bitrate optimization
- Metadata extraction (ID3 tags)
- Waveform generation
- Audio compression

### Animated Content (GIF/Gifv)
**Processing Features**:
- GIF to MP4 conversion (Gifv)
- Size optimization
- Frame rate adjustment
- Loop preservation

```go
const (
    mediaTypeGifv = "gifv"
    GifProcessingTimeout = 60 * time.Second
)
```

## Processing Workflows

### Media Upload Flow

#### 1. Initial Upload
```go
// Upload triggers media processing job
type MediaProcessor struct {
    db                   core.DB
    repos                storageCore.RepositoryStorage
    mediaRepo            *repositories.MediaRepository
    s3Client             *s3.Client
    mediaConvertClient   *mediaconvert.Client
    costTracker          *cost.Tracker
}
```

#### 2. Job Creation
1. **Media Detection**: Identify media type and format
2. **Job Creation**: Create processing job in DynamoDB
3. **Queue Message**: Send processing message to SQS
4. **Status Tracking**: Set initial status to "processing"

#### 3. Processing Execution
1. **Lambda Trigger**: SQS message triggers media processor
2. **File Download**: Securely download from S3
3. **Processing**: Apply transformations based on media type
4. **Upload Results**: Store processed variants to S3
5. **Metadata Update**: Update database with results
6. **CDN Invalidation**: Clear CloudFront cache if needed

### Status Tracking

```go
const (
    MediaStatusProcessing = "processing"
    MediaStatusCompleted  = "completed"
    MediaStatusFailed     = "failed"
    MediaStatusCancelled  = "cancelled"
)
```

## Video Processing with AWS MediaConvert

### MediaConvert Integration
**File**: `cmd/media-processor/transcoding_helpers.go`

Lesser integrates with AWS MediaConvert for professional video processing:

#### Job Configuration
```go
// Service and cost tracking constants
const (
    serviceMediaConvert = "mediaconvert"
    costCategoryProcessing = "processing"
    costCategoryStorage    = "storage"
    costCategoryBandwidth  = "bandwidth"
)
```

#### Quality Variants
Lesser generates multiple quality levels for adaptive streaming:

- **Source**: Original quality preserved
- **1080p**: 1920x1080, 5000 kbps
- **720p**: 1280x720, 3000 kbps  
- **480p**: 854x480, 1500 kbps
- **240p**: 426x240, 500 kbps

#### Cost Calculation
```go
// Calculate MediaConvert processing cost
func (mp *MediaProcessor) calculateS3StorageCost(sizeBytes int64) int64 {
    // S3 Standard storage: $0.023 per GB per month
    sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
    monthlyCostMicros := int64(sizeGB * float64(transcodingCosts.S3StorageCost))
    return monthlyCostMicros
}
```

## Video Metadata Extraction

### Comprehensive Metadata Analysis
**File**: `pkg/media/video_metadata.go`

Lesser extracts detailed video metadata for optimization:

```go
type VideoMetadata struct {
    Width            int     `json:"width"`
    Height           int     `json:"height"`
    Duration         int     `json:"duration"`
    DurationSeconds  float64 `json:"duration_seconds"`
    VideoCodec       string  `json:"video_codec"`
    AudioCodec       string  `json:"audio_codec"`
    HasAudio         bool    `json:"has_audio"`
    HasVideo         bool    `json:"has_video"`
    Bitrate          int64   `json:"bitrate"`
    FrameRate        float64 `json:"frame_rate"`
    FileSize         int64   `json:"file_size"`
}
```

### MP4/MOV Parsing
- **Atom-level parsing**: Direct MP4 container analysis
- **Codec detection**: Identify video/audio codecs
- **Duration calculation**: Precise timing information
- **Bitrate estimation**: Quality assessment

## Streaming and Adaptive Delivery

### Streaming Architecture  
**File**: `pkg/media/streaming/streamer.go`

Lesser implements adaptive streaming for optimal user experience:

#### Streaming Components
```go
type Streamer struct {
    config           *StreamingConfig
    storage          MediaStorage
    hlsGenerator     *HLSGenerator
    dashGenerator    *DASHGenerator
    bandwidthTracker *BandwidthTracker
    qualitySelector  QualitySelector
    sessionManager   *SessionManager
    analytics        core.RepositoryStorage
}
```

### HLS (HTTP Live Streaming)
- **Playlist generation**: M3U8 manifests
- **Segment creation**: Optimized chunk sizes
- **Quality adaptation**: Bandwidth-based selection
- **CDN optimization**: CloudFront integration

### DASH (Dynamic Adaptive Streaming)
- **Manifest generation**: MPD files
- **Segment optimization**: Efficient delivery
- **Multi-codec support**: H.264, H.265, VP9
- **Audio/video separation**: Flexible streaming

## Cost Optimization

### Processing Cost Tracking
Lesser tracks costs for every media operation:

#### Cost Categories
```go
const (
    costCategoryProcessing = "processing"
    costCategoryStorage    = "storage"
    costCategoryBandwidth  = "bandwidth"
    costCategoryCompute    = "compute"
)
```

#### S3 Cost Calculation
```go
func (mp *MediaProcessor) calculateS3PutCost(_ int64) int64 {
    // S3 PUT requests cost $0.0005 per 1,000 requests
    putCost := int64(500) // $0.0005 = 500 microdollars per 1,000 requests
    return putCost / 1000 // Cost per single PUT request
}
```

### Processing Optimization
1. **Format Detection**: Skip unnecessary conversions
2. **Quality Assessment**: Avoid upscaling
3. **Batch Processing**: Group similar operations
4. **Cache Utilization**: Reuse processed variants

## Reliability and Error Handling

### Retry Logic
```go
const (
    MaxRetryAttempts = 3
    BaseRetryDelay   = 1 * time.Second
    MaxRetryDelay    = 5 * time.Minute
)
```

#### Retry Strategies
- **Exponential Backoff**: Increasing delays between attempts
- **Jittered Retry**: Prevent thundering herd
- **Max Attempt Limits**: Prevent infinite loops
- **Error Classification**: Different retry policies by error type

### Timeout Management
```go
const (
    ImageProcessingTimeout = 30 * time.Second
    GifProcessingTimeout   = 60 * time.Second  
    SmallVideoTimeout      = 2 * time.Minute
    LargeVideoTimeout      = 5 * time.Minute
    AbandonedJobThreshold  = 1 * time.Hour
)
```

### Error Recovery
- **Graceful Degradation**: Serve original if processing fails
- **Partial Success**: Use completed variants
- **Manual Retry**: Admin tools for stuck jobs
- **Monitoring Alerts**: Automated failure detection

## Security and Content Safety

### Upload Security
- **File Type Validation**: Strict MIME type checking
- **Size Limits**: Configurable upload limits
- **Virus Scanning**: Optional malware detection
- **Content Validation**: Prevent malicious uploads

### Content Moderation
**Integration**: `pkg/moderation/advanced/engine.go`

- **AI Content Analysis**: AWS Rekognition integration
- **NSFW Detection**: Adult content identification
- **Violence Detection**: Harmful content filtering
- **Custom Rules**: Configurable moderation policies

### Privacy Protection
- **Metadata Stripping**: Remove EXIF location data
- **Content Warnings**: Sensitive content handling
- **Access Control**: Respect privacy settings

## Storage and CDN

### S3 Storage Strategy
- **Bucket Organization**: Logical folder structure
- **Lifecycle Policies**: Automated archiving
- **Encryption**: Server-side encryption at rest
- **Cross-Region Replication**: Optional disaster recovery

### CloudFront CDN
- **Global Distribution**: Edge caching worldwide
- **Compression**: Automatic gzip compression
- **Cache Optimization**: Intelligent cache headers
- **Invalidation**: Automated cache clearing

## Performance Monitoring

### Metrics Collection
**File**: `pkg/observability/constants.go`

Lesser tracks comprehensive media metrics:

```go
const (
    // Media metrics
    MetricMediaProcessing      = "MediaProcessing"
    MetricMediaProcessingTime  = "MediaProcessingTime"
    MetricMediaUpload          = "MediaUpload"
    MetricMediaTranscoding     = "MediaTranscoding"
    MetricMediaStorage         = "MediaStorage"
)
```

### Analytics Tracking
- **Processing Time**: Track performance trends
- **Success Rates**: Monitor processing reliability  
- **Cost Analysis**: Track spending by media type
- **Quality Metrics**: Monitor output quality
- **User Experience**: Track delivery performance

## API Integration

### REST API Endpoints
**File**: `cmd/api/lift/media_v2.go`

#### Upload Endpoints
- `POST /api/v1/media` - Upload media with processing
- `PUT /api/v1/media/:id` - Update media metadata
- `GET /api/v1/media/:id` - Get media information
- `DELETE /api/v1/media/:id` - Delete media and variants

#### Processing Status
- `GET /api/v1/media/:id/status` - Get processing status
- `POST /api/v1/media/:id/retry` - Retry failed processing

### GraphQL Integration
Media processing is fully integrated with Lesser's GraphQL API:

#### Mutations
- `uploadMedia` - Upload with real-time status
- `updateMediaMetadata` - Update media information  
- `deleteMedia` - Remove media and variants

#### Queries
- `media` - Get media information and variants
- `mediaJob` - Get processing job status

## Configuration

### Environment Variables
```bash
# Media processing configuration
MEDIA_BUCKET_NAME=your-media-bucket
CDN_DOMAIN=media.your-instance.com
MEDIA_CONVERT_ENDPOINT=https://mediaconvert.region.amazonaws.com
MAX_UPLOAD_SIZE_MB=40
ENABLE_VIDEO_PROCESSING=true

# Quality settings
VIDEO_MAX_WIDTH=1920
VIDEO_MAX_HEIGHT=1080
IMAGE_MAX_WIDTH=2048
IMAGE_MAX_HEIGHT=2048

# Processing timeouts
IMAGE_TIMEOUT_SECONDS=30
VIDEO_TIMEOUT_SECONDS=300
ABANDONED_JOB_HOURS=1
```

### Cost Limits
```bash
# Processing cost controls
MAX_PROCESSING_COST_MICROS=50000  # $0.05 per job
DAILY_PROCESSING_BUDGET_DOLLARS=10
MONTHLY_STORAGE_BUDGET_DOLLARS=50
```

## Testing

### Test Coverage
**Files**:
- `cmd/media-processor/unit_test.go`
- `pkg/media/video_metadata_test.go`  
- `pkg/media/streaming/keyframe_test.go`

#### Unit Tests
- Media type detection
- Metadata extraction
- Cost calculation
- Error handling scenarios

#### Integration Tests  
- End-to-end processing workflow
- S3 upload/download operations
- MediaConvert job submission
- CDN cache invalidation

### Testing Commands
```bash
# Run media processing tests
make test-media

# Test specific media processor functionality
go test ./cmd/media-processor/...

# Test video metadata extraction
go test ./pkg/media/...

# Run streaming tests
go test ./pkg/media/streaming/...
```

## Troubleshooting

### Common Issues

#### Processing Failures
```bash
# Check processing job status
aws dynamodb get-item \
  --table-name YourTable \
  --key '{"PK":{"S":"MEDIA#123"},"SK":{"S":"JOB#456"}}'

# Check MediaConvert job
aws mediaconvert describe-job --id <job-id>
```

#### Upload Problems
```bash
# Test S3 upload permissions
aws s3 cp test-file.jpg s3://your-bucket/test/

# Check Lambda function logs
aws logs filter-log-events \
  --log-group-name /aws/lambda/media-processor
```

#### CDN Issues
```bash
# Test CDN delivery
curl -I https://media.your-instance.com/path/to/media.jpg

# Invalidate CDN cache
aws cloudfront create-invalidation \
  --distribution-id E123456789 \
  --paths "/media/*"
```

### Monitoring and Alerts

#### Key Metrics to Monitor
- Processing success rate (>95%)
- Average processing time (by media type)
- Cost per processing job
- Storage growth rate
- CDN hit ratio

#### Alert Configuration
```go
// Alert thresholds for media processing
const (
    AlertMediaProcessingFailureRate = 10.0  // 10% failure rate
    AlertMediaProcessingTime        = 300   // 5 minutes for videos
    AlertMediaCostPerJob           = 5000   // $0.005 per job
)
```

## Best Practices

### Upload Optimization
1. **Client-side validation**: Check file types before upload
2. **Progress tracking**: Show upload progress to users
3. **Resumable uploads**: Handle network interruptions
4. **Size optimization**: Compress images before upload

### Processing Efficiency
1. **Batch operations**: Group similar processing tasks
2. **Quality assessment**: Don't upscale low-quality media
3. **Format selection**: Choose optimal output formats
4. **Cache utilization**: Reuse processed variants

### Cost Management
1. **Budget alerts**: Set spending thresholds
2. **Quality tiers**: Offer different processing levels
3. **Retention policies**: Archive old media automatically
4. **Usage analytics**: Track cost per user

### Security Considerations
1. **Input validation**: Strictly validate all uploads
2. **Content scanning**: Check for malicious content
3. **Access control**: Implement proper permissions
4. **Audit logging**: Track all media operations

Lesser's media processing pipeline provides enterprise-grade reliability while maintaining cost efficiency through intelligent optimization and serverless architecture. The system scales automatically and provides detailed tracking of both performance and costs.