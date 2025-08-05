# S3 Lifecycle Policies Implementation

## Overview

Comprehensive S3 lifecycle policies have been implemented to optimize storage costs and automate data management across all S3 buckets in the Lesser infrastructure.

## Key Features

### 1. Media Bucket Lifecycle Policies

The media bucket implements intelligent tiering and cleanup:

#### Storage Class Transitions
- **30 days**: Standard → Infrequent Access (45% cost reduction)
- **90 days**: IA → Glacier Instant Retrieval (68% cost reduction)
- **180 days**: Glacier IR → Glacier Flexible Retrieval (95% cost reduction)

#### Automatic Cleanup
- **Incomplete uploads**: Aborted after 7 days
- **Temp files** (`temp/`): Deleted after 1 day
- **Orphaned uploads** (`uploads/orphaned/`): Deleted after 30 days
- **Old thumbnails** (`thumbnails/`): Deleted after 1 year
- **Deleted accounts** (`accounts/deleted/`): Removed after 30 days (GDPR compliance)
- **Deleted content** (`deleted/`): Moved to Glacier after 7 days, deleted after 90 days

#### Avatar Optimization
- Avatars moved to Infrequent Access after 60 days (rarely change)

### 2. Log Bucket Policies

Optimized for write-once, rarely-read pattern:
- **30 days**: Standard → Infrequent Access
- **60 days**: IA → Glacier
- **Retention**:
  - Production: 365 days
  - Staging: 90 days
  - Development: 30 days

### 3. CloudFront Log Policies

High-volume, low-value logs with aggressive optimization:
- **7 days**: Standard → Infrequent Access
- **30 days**: IA → Glacier
- **90 days**: Deleted

### 4. Backup Bucket Policies

Tiered retention based on backup frequency:
- **Daily backups**: 30-day retention
- **Weekly backups**: 90-day retention
- **Monthly backups**: 365-day retention
- **Failed backups**: Deleted after 7 days

Storage optimization:
- **7 days**: Standard → Infrequent Access
- **30 days**: IA → Glacier
- **90 days**: Glacier → Deep Archive

### 5. Export Bucket Policies

User data exports with temporary storage:
- **Exports**: Deleted after 30 days
- **Completed exports**: Archived to Glacier after 7 days
- **Incomplete uploads**: Aborted after 1 day

### 6. Cache Bucket Policies

Aggressive cleanup for temporary data:
- **All cache**: Expires after 1 day
- **Temp cache** (`temp/`): Expires after 1 hour

## Cost Savings

### Estimated Monthly Savings by Environment

| Environment | Media Bucket | Logs | Exports | Cache | Total |
|------------|-------------|------|---------|-------|-------|
| Production | $150 | $30 | $20 | $10 | $210 |
| Staging | $25 | $10 | $5 | $5 | $45 |
| Development | $5 | $5 | $2 | $2 | $14 |

### Storage Class Cost Comparison (per TB/month)

| Storage Class | Cost | Use Case |
|--------------|------|----------|
| Standard | $23.00 | Frequently accessed (< 30 days) |
| Standard-IA | $12.50 | Infrequently accessed (30-90 days) |
| Glacier Instant | $4.00 | Archive with instant retrieval |
| Glacier Flexible | $1.00 | Archive with 1-12 hour retrieval |
| Deep Archive | $0.20 | Long-term archive (90+ days) |

## Implementation Files

- **Core Implementation**: `/infra/cdk/constructs/s3_lifecycle.go`
- **Media Bucket Integration**: `/infra/cdk/stacks/lesser_stack.go`
- **CloudFront Logs Integration**: `/infra/cdk/constructs/cloudfront_caching.go`
- **Example Usage**: `/examples/s3_lifecycle_example.go`

## Usage

### Apply Lifecycle Policies to Existing Bucket

```go
import "cdk/constructs"

// Apply policies to an existing bucket
constructs.ApplyS3LifecyclePolicies(&constructs.S3LifecycleConfig{
    Environment: "production",
    Bucket:      myBucket,
    BucketType:  "media", // or "logs", "backups", "cloudfront-logs"
})
```

### Create New Bucket with Lifecycle Policies

```go
// Create export bucket with built-in lifecycle policies
exportBucket := constructs.CreateExportBucketWithLifecycle(stack, "production")

// Create cache bucket with aggressive cleanup
cacheBucket := constructs.CreateCacheBucketWithLifecycle(stack, "production")
```

### Calculate Cost Savings

```go
// Get estimated savings metrics
metrics := constructs.CalculateLifecycleSavings(bucket, "production")
fmt.Printf("Estimated monthly savings: $%.2f\n", metrics.EstimatedMonthlySave)
```

## Compliance Considerations

### GDPR Compliance
- Deleted user data removed within 30 days
- Account deletion triggers media cleanup
- Audit trail maintained in Glacier for legal requirements

### Data Retention
- Production logs kept for 1 year
- Backups follow 30/90/365 day retention policy
- Deleted content archived before permanent deletion

### Security
- All buckets use S3-managed encryption
- Versioning enabled in production
- Public access blocked on all buckets

## Monitoring

### CloudWatch Metrics
- Lifecycle transition events
- Storage class distribution
- Cost optimization metrics
- Expiration events

### Alerts
- Failed lifecycle transitions
- Unexpected storage growth
- Cost threshold breaches

## Best Practices

1. **Test in Development First**: Always test lifecycle policies in development environment
2. **Monitor Transitions**: Watch CloudWatch metrics for successful transitions
3. **Review Access Patterns**: Adjust transition timings based on actual usage
4. **Cost Analysis**: Regularly review cost savings and adjust policies
5. **Compliance Checks**: Ensure policies meet regulatory requirements

## Future Enhancements

1. **Intelligent Tiering**: Implement S3 Intelligent-Tiering for automatic optimization
2. **Custom Metrics**: Build dashboard for lifecycle policy effectiveness
3. **ML-Based Optimization**: Use access patterns to predict optimal transition times
4. **Cross-Region Replication**: Add lifecycle policies for replicated buckets
5. **Batch Operations**: Implement bulk transitions for existing objects