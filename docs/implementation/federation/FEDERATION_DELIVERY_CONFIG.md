# Federation Delivery Configuration

This document describes the configuration options for federation delivery retries with optional SQS support.

## Overview

The federation delivery system supports both synchronous and asynchronous delivery modes:

- **Synchronous (default)**: Activities are delivered immediately during the request
- **Asynchronous**: Activities are queued to SQS for background processing with retries

## Environment Variables

### Core Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FEDERATION_DELIVERY_MODE` | `sync` | Delivery mode: `sync` or `async` |
| `FEDERATION_QUEUE_URL` | - | SQS queue URL for async delivery (optional) |
| `FEDERATION_MAX_RETRIES` | `5` | Maximum retry attempts for failed deliveries |

### Queue Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FEDERATION_DELIVERY_QUEUE_URL` | - | Main federation delivery queue URL |
| `FEDERATION_DLQ_URL` | - | Dead letter queue URL for permanently failed deliveries |

### Retry Behavior

| Variable | Default | Description |
|----------|---------|-------------|
| `FEDERATION_RETRY_BASE_DELAY` | `60` | Base retry delay in seconds |
| `FEDERATION_RETRY_MAX_DELAY` | `3600` | Maximum retry delay in seconds |
| `FEDERATION_HEALTH_CHECK_ENABLED` | `true` | Enable target health assessment |

## Delivery Modes

### Synchronous Mode (Default)

```bash
# Explicitly set sync mode
FEDERATION_DELIVERY_MODE=sync

# Or leave unset - defaults to sync for backwards compatibility
```

**Behavior:**
- Activities delivered immediately during request processing
- No SQS queues required
- Failures return immediately to caller
- Suitable for low-volume instances or development

### Asynchronous Mode

```bash
# Required for async mode
FEDERATION_DELIVERY_MODE=async
FEDERATION_QUEUE_URL=https://sqs.region.amazonaws.com/account/queue-name

# Optional - falls back to sync if not configured
FEDERATION_DLQ_URL=https://sqs.region.amazonaws.com/account/dlq-name
```

**Behavior:**
- Activities queued to SQS for background processing
- Automatic retries with exponential backoff
- Failed deliveries moved to DLQ after max retries
- Suitable for high-volume production instances

## Error Classification

The system automatically classifies delivery errors:

### Permanent Errors (No Retry)
- HTTP 4xx errors (except 429 rate limiting)
- Signature verification failures
- Blocked domains
- Invalid request formats

### Temporary Errors (Retry with Backoff)
- HTTP 5xx server errors
- Network timeouts and connection failures
- HTTP 429 rate limiting
- DNS resolution failures

## Retry Strategy

### Exponential Backoff
- Initial delay: 1 minute
- Each retry doubles the delay
- Maximum delay: 60 minutes
- Jitter added to prevent thundering herd

### Error-Specific Adjustments
- **Rate Limiting (429)**: 3x longer backoff
- **Server Errors (5xx)**: 2x longer backoff
- **Timeouts**: 1.5x longer backoff
- **Network Issues**: Standard backoff

### Health-Based Delays
The system monitors target instance health and adjusts retry delays:
- High error rate (>50%): Skip delivery, 3x backoff
- Moderate errors (>20% on retry): Skip delivery, 2x backoff
- Slow responses (>30s): Skip delivery, 2x backoff
- Stale instances (>24h): Skip delivery, 4x backoff

## Monitoring and Observability

### Structured Logging
All delivery attempts log structured data:
```json
{
  "delivery_id": "delivery_abc123_1234567890",
  "activity_id": "https://example.com/activities/1",
  "activity_type": "Create",
  "target_inbox": "https://remote.social/inbox",
  "error_type": "temporary",
  "retry_count": 2,
  "backoff_minutes": 4
}
```

### Metrics
The system records metrics for:
- Delivery success/failure rates
- Retry counts by error type
- Backoff delays by target domain
- Queue depth and processing time

### Delivery Status Tracking
Each delivery is tracked in DynamoDB:
- Status: pending, delivered, failed, retrying, permanently_failed
- Attempt counts and error details
- Next retry schedules
- Automatic cleanup via TTL

## Backwards Compatibility

The system maintains full backwards compatibility:

1. **Default Behavior**: Synchronous delivery when no SQS is configured
2. **Graceful Fallback**: Falls back to sync if SQS operations fail
3. **Configuration Detection**: Automatically detects available SQS configuration
4. **Existing Code**: No changes required to existing federation calls

## Infrastructure Requirements

### SQS Queues
```yaml
FederationQueue:
  Type: AWS::SQS::Queue
  Properties:
    MessageRetentionPeriod: 1209600  # 14 days
    VisibilityTimeout: 300           # 5 minutes
    DelaySeconds: 0
    RedrivePolicy:
      deadLetterTargetArn: !GetAtt FederationDLQ.Arn
      maxReceiveCount: 3

FederationDLQ:
  Type: AWS::SQS::Queue
  Properties:
    MessageRetentionPeriod: 1209600  # 14 days
```

### Lambda Configuration
```yaml
FederationDelivery:
  Type: AWS::Lambda::Function
  Properties:
    Environment:
      Variables:
        FEDERATION_DELIVERY_MODE: async
        FEDERATION_QUEUE_URL: !Ref FederationQueue
        FEDERATION_DLQ_URL: !Ref FederationDLQ
        FEDERATION_MAX_RETRIES: 5
```

### IAM Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:SendMessage",
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes",
        "sqs:ChangeMessageVisibility"
      ],
      "Resource": [
        "arn:aws:sqs:*:*:federation-queue*",
        "arn:aws:sqs:*:*:federation-dlq*"
      ]
    }
  ]
}
```

## Usage Examples

### Basic Delivery
```go
// Automatic mode selection based on configuration
err := deliveryService.QueueDelivery(ctx, activity, targetInbox, signingActor)
if err != nil {
    // Handle delivery error
}
```

### Force Synchronous Delivery
```go
// Always deliver synchronously
err := deliveryService.DeliverActivity(ctx, activity, targetInbox, signingActor)
```

### Check Configuration
```go
deliveryMode := os.Getenv("FEDERATION_DELIVERY_MODE")
if deliveryMode == "async" {
    log.Info("Using async federation delivery")
} else {
    log.Info("Using synchronous federation delivery")
}
```

## Troubleshooting

### Common Issues

1. **SQS Access Denied**
   - Verify IAM permissions include SQS operations
   - Check queue URLs are correct
   - Ensure Lambda execution role has SQS access

2. **High Failure Rates**
   - Check target instance health
   - Review error classification logs
   - Monitor retry patterns

3. **Queue Buildup**
   - Increase Lambda concurrency
   - Check for stuck messages
   - Review DLQ for permanent failures

4. **Fallback to Sync**
   - Verify SQS configuration
   - Check network connectivity to SQS
   - Review CloudWatch logs for SQS errors

### Debug Logging
Enable detailed logging:
```bash
LOG_LEVEL=debug
FEDERATION_DELIVERY_DEBUG=true
```

This will log all delivery decisions and retry calculations.