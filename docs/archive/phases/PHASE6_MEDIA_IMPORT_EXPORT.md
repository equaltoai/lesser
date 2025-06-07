# Phase 6: Media & Import/Export Implementation Plan

## Overview

Phase 6 focuses on three key areas:
1. **Async Media Upload (v2)** - Non-blocking media processing with queue-based architecture
2. **Import/Export** - Data portability for user content and relationships
3. **oEmbed** - Enable embedding Lesser content in external sites

## 6.1 Media Improvements - Async Upload

### Current State
- ✅ Synchronous media upload (POST /api/v1/media) already implemented
- ✅ S3 storage with CloudFront CDN integration
- ✅ Basic file type validation and size limits
- ❌ Missing: Async processing, thumbnails, blurhash, dimension extraction

### Implementation Plan

#### 6.1.1 API Endpoint: POST /api/v2/media

**Request Format:**
```json
{
  "file": "multipart/form-data",
  "thumbnail": "multipart/form-data (optional)",
  "description": "string",
  "focus": "x,y coordinates"
}
```

**Response Format:**
```json
{
  "id": "123456789",
  "type": "image",
  "url": null,  // Not available until processed
  "preview_url": null,
  "processing": true,  // Key difference from v1
  "meta": {}
}
```

#### 6.1.2 Processing Queue Architecture

**DynamoDB Schema:**
```
Table: MediaProcessingJobs
  PK: JOB#<job_id>
  SK: JOB#<job_id>
  
  GSI1:
    PK: USER#<username>
    SK: CREATED#<timestamp>
    
  GSI2:
    PK: STATUS#<status>
    SK: CREATED#<timestamp>
  
  Attributes:
    - JobID
    - MediaID
    - Username
    - S3Key
    - MimeType
    - Status (pending, processing, completed, failed)
    - RetryCount
    - ProcessingTasks (array of tasks to perform)
    - Results (processing results)
    - CreatedAt
    - UpdatedAt
    - TTL (7 days after completion)
```

#### 6.1.3 Media Processor Lambda

**Processing Tasks:**
1. **Image Processing**
   - Generate multiple sizes (original, large, medium, small, thumbnail)
   - Extract dimensions and EXIF data
   - Generate blurhash for placeholder
   - Optimize with ImageMagick/Sharp

2. **Video Processing**
   - Extract duration and dimensions
   - Generate thumbnail at specific timestamp
   - Transcode to web-optimized format (if needed)
   - Extract first frame for preview

3. **Audio Processing**
   - Extract duration and metadata
   - Generate waveform visualization
   - Create thumbnail/cover art

**Implementation Files:**
- `cmd/api/handlers/media_v2.go` - Async upload handler
- `cmd/media-processor/main.go` - Lambda processor
- `pkg/media/processor.go` - Processing logic
- `pkg/media/blurhash.go` - Blurhash generation
- `pkg/storage/dynamodb/media_jobs.go` - Job storage

### 6.1.4 Status Polling

Clients poll for processing status:
```
GET /api/v1/media/:id
```

Returns processing status in response:
```json
{
  "id": "123456789",
  "processing": true,
  "progress": 65  // Optional percentage
}
```

## 6.2 Import/Export Implementation

### 6.2.1 Export Functionality

#### POST /api/v1/exports

**Request:**
```json
{
  "type": "archive",  // or "followers", "following", "blocks", "mutes", "lists", "bookmarks"
  "format": "activitypub",  // or "mastodon", "csv"
  "include_media": true,
  "date_range": {
    "start": "2024-01-01",
    "end": "2024-12-31"
  }
}
```

**Response:**
```json
{
  "id": "export_123",
  "status": "pending",
  "created_at": "2025-01-01T00:00:00Z",
  "download_url": null,
  "expires_at": null
}
```

#### Export Job Processing

**DynamoDB Schema:**
```
Table: ExportJobs
  PK: EXPORT#<export_id>
  SK: EXPORT#<export_id>
  
  GSI1:
    PK: USER#<username>
    SK: CREATED#<timestamp>
  
  Attributes:
    - ExportID
    - Username
    - Type
    - Format
    - Status (pending, processing, completed, failed)
    - Options (include_media, date_range, etc)
    - S3Key (when completed)
    - DownloadURL (pre-signed S3 URL)
    - ExpiresAt (download expiry)
    - FileSize
    - RecordCount
    - CreatedAt
    - CompletedAt
    - TTL (30 days after creation)
```

#### Export Generator Lambda

**Export Formats:**

1. **ActivityPub Archive**
   - Complete actor object
   - All activities (Create, Like, Announce, etc)
   - Objects (Notes, Articles, etc)
   - Media attachments
   - Collections (followers, following)

2. **Mastodon-Compatible Archive**
   - actor.json
   - outbox.json
   - likes.json
   - bookmarks.json
   - lists.json
   - media_attachments/

3. **CSV Exports**
   - followers.csv
   - following.csv
   - blocks.csv
   - mutes.csv

**Implementation Files:**
- `cmd/api/handlers/exports.go` - Export API handlers
- `cmd/export-generator/main.go` - Lambda function
- `pkg/export/activitypub.go` - ActivityPub format
- `pkg/export/mastodon.go` - Mastodon format
- `pkg/export/csv.go` - CSV format
- `pkg/storage/dynamodb/export_jobs.go` - Job storage

### 6.2.2 Import Functionality

#### POST /api/v1/imports

**Request:**
```json
{
  "type": "followers",  // or "following", "blocks", "mutes", "lists", "bookmarks"
  "data": "base64_encoded_file_content",
  "mode": "merge"  // or "overwrite"
}
```

**Response:**
```json
{
  "id": "import_456",
  "status": "pending",
  "type": "followers",
  "created_at": "2025-01-01T00:00:00Z",
  "processed": 0,
  "total": null
}
```

#### Import Processing

**DynamoDB Schema:**
```
Table: ImportJobs
  PK: IMPORT#<import_id>
  SK: IMPORT#<import_id>
  
  GSI1:
    PK: USER#<username>
    SK: CREATED#<timestamp>
  
  Attributes:
    - ImportID
    - Username
    - Type
    - Status (pending, processing, completed, failed)
    - Mode (merge, overwrite)
    - S3Key (uploaded file)
    - Format (detected format)
    - Progress (processed/total records)
    - Errors (array of error messages)
    - Results (success/skip/error counts)
    - CreatedAt
    - CompletedAt
    - TTL (7 days after completion)
```

#### Import Processor Lambda

**Processing Steps:**
1. **Format Detection**
   - Detect CSV, JSON, or archive format
   - Validate structure and required fields

2. **Data Validation**
   - Verify actor IDs exist (WebFinger lookup)
   - Check for valid URLs
   - Validate data types

3. **Conflict Resolution**
   - Handle duplicates based on mode
   - Skip invalid entries
   - Log errors for review

4. **Batch Processing**
   - Process in chunks to avoid timeouts
   - Update progress regularly
   - Handle rate limits for remote lookups

**Implementation Files:**
- `cmd/api/handlers/imports.go` - Import API handlers
- `cmd/import-processor/main.go` - Lambda function
- `pkg/import/detector.go` - Format detection
- `pkg/import/validator.go` - Data validation
- `pkg/import/processor.go` - Import logic
- `pkg/storage/dynamodb/import_jobs.go` - Job storage

## 6.3 oEmbed Implementation

### 6.3.1 oEmbed Endpoint

#### GET /api/oembed

**Query Parameters:**
- `url` - The URL to embed (required)
- `format` - json or xml (default: json)
- `maxwidth` - Maximum width of embed
- `maxheight` - Maximum height of embed

**Response:**
```json
{
  "version": "1.0",
  "type": "rich",
  "provider_name": "Lesser",
  "provider_url": "https://example.com",
  "author_name": "username",
  "author_url": "https://example.com/@username",
  "title": "Status by username",
  "html": "<iframe src='...' width='400' height='200'></iframe>",
  "width": 400,
  "height": 200,
  "cache_age": 3600
}
```

### 6.3.2 Embed HTML Generation

**Embed Types:**
1. **Status Embed**
   - Responsive iframe with status content
   - Include media previews
   - Show engagement counts
   - Link back to original

2. **Profile Embed**
   - User card with avatar and bio
   - Recent posts preview
   - Follow button (if applicable)

**Implementation Files:**
- `cmd/api/handlers/oembed.go` - oEmbed handler
- `pkg/oembed/generator.go` - HTML generation
- `pkg/oembed/templates/` - Embed templates

### 6.3.3 Discovery

Add to HTML head of status pages:
```html
<link rel="alternate" type="application/json+oembed" 
      href="https://example.com/api/oembed?url=..." 
      title="Status by username">
```

## Implementation Timeline

### Week 1: Async Media Upload
- [ ] Create media_v2 handler
- [ ] Implement job queue schema
- [ ] Create media-processor Lambda skeleton
- [ ] Add image processing with Sharp
- [ ] Implement blurhash generation
- [ ] Add progress polling to v1 endpoint

### Week 2: Export Functionality  
- [ ] Create export handlers and models
- [ ] Implement export job storage
- [ ] Create export-generator Lambda
- [ ] Implement ActivityPub archive format
- [ ] Add Mastodon-compatible format
- [ ] Implement CSV exports
- [ ] Add S3 upload and pre-signed URLs

### Week 3: Import Functionality
- [ ] Create import handlers
- [ ] Implement import job storage
- [ ] Create import-processor Lambda
- [ ] Add format detection
- [ ] Implement validation logic
- [ ] Add batch processing
- [ ] Handle WebFinger lookups

### Week 4: oEmbed & Polish
- [ ] Implement oEmbed endpoint
- [ ] Create embed templates
- [ ] Add discovery meta tags
- [ ] Test all import/export formats
- [ ] Add progress notifications
- [ ] Documentation and examples
- [ ] Performance optimization

## Testing Strategy

### Media Processing Tests
- Upload various file types and sizes
- Verify thumbnail generation
- Test blurhash accuracy
- Check dimension extraction
- Validate CDN URLs

### Import/Export Tests
- Round-trip testing (export then import)
- Large dataset handling (10k+ records)
- Format compatibility testing
- Error handling and recovery
- Progress tracking accuracy

### oEmbed Tests
- Various URL formats
- Responsive embed sizing
- Cache header validation
- HTML injection prevention

## Security Considerations

1. **Media Upload**
   - Virus scanning for uploads
   - EXIF data stripping
   - File type validation
   - Size limits enforcement

2. **Import/Export**
   - Rate limiting on jobs
   - File size limits
   - Sanitize imported data
   - Secure S3 pre-signed URLs

3. **oEmbed**
   - URL validation
   - XSS prevention in embeds
   - Cache poisoning prevention
   - Rate limiting

## Monitoring & Metrics

1. **Media Processing**
   - Processing time by file type
   - Queue depth and latency
   - Error rates by task type
   - Storage usage trends

2. **Import/Export**
   - Job completion rates
   - Average processing time
   - Format usage statistics
   - Error frequency by type

3. **oEmbed**
   - Request volume by domain
   - Cache hit rates
   - Embed rendering time
   - Error responses

## Cost Optimization

1. **Media Storage**
   - S3 lifecycle policies for old media
   - Intelligent tiering for infrequent access
   - CloudFront caching optimization
   - Compress archived exports

2. **Lambda Functions**
   - Memory sizing optimization
   - Concurrent execution limits
   - Dead letter queues for failures
   - Batch processing where possible

3. **DynamoDB**
   - TTL on completed jobs
   - On-demand pricing for job tables
   - Sparse GSIs for cost efficiency

## Next Steps

1. Create handler skeletons for all three features
2. Set up DynamoDB tables for job tracking
3. Create Lambda function projects
4. Implement core processing logic
5. Add comprehensive error handling
6. Write integration tests
7. Document API changes
8. Update Postman collection 