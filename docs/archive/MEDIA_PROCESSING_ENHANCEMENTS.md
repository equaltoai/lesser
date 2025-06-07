# Media Processing Enhancements

## Overview

Lesser now includes advanced media processing capabilities that provide:

1. **Multiple resolution variants** for optimal bandwidth usage
2. **Blurhash generation** for instant placeholder images
3. **EXIF stripping** for privacy protection
4. **Modern format support** including WebP
5. **Asynchronous processing** via Lambda functions

## Features

### Image Processing

#### Multiple Sizes
Each uploaded image is processed into multiple sizes:
- **Small**: 400x400px max (thumbnail, quality 80)
- **Medium**: 800x800px max (preview, quality 85) 
- **Large**: 1920x1080px max (full view, quality 90)
- **Original**: Preserved with EXIF stripped

#### Blurhash
- Generates compact placeholder representations
- 4x3 component resolution for balance of quality and size
- Enables instant loading perception
- Fallback to neutral gray hash on error

#### Format Support
- JPEG (with quality optimization)
- PNG (lossless)
- GIF (preserved animation)
- WebP (modern format)

#### Privacy Features
- Automatic EXIF metadata stripping
- Preserves image quality while removing location/camera data

### Processing Architecture

```
User Upload → API Gateway → Media Handler → S3 (Original)
                                    ↓
                            DynamoDB (Job Queue)
                                    ↓
                            EventBridge → Media Processor Lambda
                                              ↓
                                    Process & Generate Sizes
                                              ↓
                                    Upload to S3 → Update DynamoDB
```

### S3 Storage Structure

```
/media
  /<username>
    /<media_id>
      /original.jpg     # Original with EXIF stripped
      /large.jpg        # 1920x1080 max
      /medium.jpg       # 800x800 max
      /small.jpg        # 400x400 max (thumbnail)
```

## Implementation

### Media Package (`pkg/media/`)

**blurhash.go**
- Blurhash encoding/decoding
- Optimized for small preview generation
- Uses efficient resizing before encoding

**image_processor.go**
- Multi-size image processing
- Format conversion and optimization
- EXIF stripping
- Aspect ratio preservation

### Media Processor Lambda

**Processing Tasks:**
1. Download original from S3
2. Generate blurhash
3. Create size variants
4. Strip EXIF metadata
5. Upload all variants to S3
6. Update media record with URLs and metadata

**Error Handling:**
- Graceful degradation for unsupported formats
- Fallback blurhash for processing errors
- Retry logic with exponential backoff

## API Changes

### Media Upload Response

```json
{
  "id": "123456789",
  "type": "image",
  "url": "https://cdn.example.com/media/user/123/large.jpg",
  "preview_url": "https://cdn.example.com/media/user/123/small.jpg",
  "blurhash": "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  "meta": {
    "original": {
      "width": 2048,
      "height": 1536,
      "size": "2048x1536",
      "aspect": 1.33
    },
    "small": {
      "width": 400,
      "height": 300,
      "size": "400x300",
      "aspect": 1.33
    },
    "focus": {
      "x": 0.0,
      "y": 0.0
    }
  }
}
```

## Cost Optimization

### Lambda Processing
- ~100ms average processing time per image
- Well within AWS Lambda free tier for most instances
- Pay only for actual processing time

### S3 Storage
- Intelligent tiering for older media
- CloudFront caching reduces bandwidth costs
- Lifecycle rules to archive unused media

### Bandwidth Savings
- Small thumbnails for timeline views (~20KB)
- Medium for expanded views (~100KB)
- Large only on full image view (~500KB)
- Estimated 70% bandwidth reduction vs serving originals

## Future Enhancements

### Video Processing
- Thumbnail extraction
- Multiple quality transcoding
- HLS/DASH streaming for long videos
- GIF to MP4 conversion

### Advanced Image Features
- AVIF format support
- Smart cropping with face detection
- Custom focus point handling
- Image similarity detection for duplicates

### Performance
- WebAssembly blurhash generation
- GPU-accelerated resizing
- Parallel processing for multiple uploads

## Configuration

### Environment Variables

```bash
# S3 Configuration
S3_BUCKET_NAME=your-media-bucket
CDN_DOMAIN=cdn.your-instance.com

# Processing Configuration
MAX_IMAGE_SIZE=10485760  # 10MB
ENABLE_WEBP=true
BLURHASH_COMPONENTS=4x3
```

### Lambda Configuration
- Memory: 1024MB (optimal for image processing)
- Timeout: 30 seconds
- Reserved concurrency: 10 (adjustable based on load)

## Monitoring

### CloudWatch Metrics
- Processing duration by image size
- Success/failure rates
- Format distribution
- Blurhash generation time

### Alerts
- Processing failures > 5%
- Lambda timeout warnings
- S3 upload errors
- High memory usage

## Security Considerations

1. **Input Validation**: Strict MIME type checking
2. **Size Limits**: Prevent resource exhaustion
3. **Format Bombs**: Protection against malicious images
4. **Access Control**: Signed URLs for private media 