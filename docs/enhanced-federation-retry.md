# Enhanced Federation Retry Implementation

## Overview

This document describes the implementation of enhanced federation retry capabilities for critical ActivityPub activities (Delete and Flag) in Lesser. The implementation addresses the audit findings regarding placeholder comments for federation propagation.

## Key Features

### 1. Delete Activity Federation with Mention Delivery

**Location**: `graph/schema.resolvers.go` lines 1021-1027

**Implementation**:
- Replaced placeholder comment with `deliverDeleteToMentions()` function
- Extracts mentions from the original object being deleted
- Delivers delete notifications to mentioned users via ActivityPub federation
- Uses enhanced retry policy for critical delete propagation

**Key Functions**:
- `deliverDeleteToMentions()`: Extracts mentions and initiates federation
- Handles both `Note` and `Article` object types
- Sets up ActivityPub recipients properly in the `To` field

### 2. Flag Activity Federation with Proper Queuing

**Location**: `graph/schema.resolvers.go` lines 2338-2342

**Implementation**:
- Replaced TODO comment with `deliverFlagActivityWithRetry()` function
- Delivers flag activities to remote instance moderators
- Uses polynomial retry policy for reliable flag propagation
- Extracts target domain and constructs moderation inbox URLs

**Key Functions**:
- `deliverFlagActivityWithRetry()`: Initiates flag federation with retry
- Targets instance moderator inboxes for flag activities
- Records federation activity for tracking

### 3. Polynomial Retry Policy

**Requirements Met**:
- **Formula**: `delay = attempt_count + 15 seconds + jitter`
- **Maximum Attempts**: 25 attempts
- **Maximum Duration**: 20 days
- **Jitter**: 0-5 seconds random addition to prevent thundering herd

**Implementation**:
- `EnhancedRetryProcessor`: Handles polynomial retry logic
- `calculatePolynomialDelay()`: Implements the retry formula
- SQS-based queuing with delay for scalable retry processing

### 4. Partial Federation Failure Tracking

**Features**:
- Tracks successful vs failed deliveries per inbox
- Records partial success scenarios
- Maintains retry state across attempts
- Provides detailed failure reasons and metrics

**Data Tracking**:
- `FederationActivity` records with enhanced metadata
- Success/failure counters per inbox
- Retry attempt history
- Performance metrics for analysis

## Architecture Components

### Core Files

1. **Graph Resolvers** (`graph/schema.resolvers.go`)
   - Enhanced delete/flag federation calls
   - Integration with retry processors
   - Fallback to basic retry tracking

2. **Enhanced Retry Processor** (`pkg/federation/enhanced_retry.go`)
   - Polynomial retry logic implementation
   - SQS message handling
   - Partial failure tracking
   - Federation activity recording

3. **Lambda Processor** (`cmd/enhanced-federation-processor/main.go`)
   - SQS event handling for retry queue
   - Lambda-optimized DynamoDB initialization
   - Enhanced retry message processing

### Federation Flow

1. **User Action**: Delete/Flag operation initiated via GraphQL
2. **Immediate Attempt**: Try federation delivery immediately
3. **Failure Handling**: On failure, queue for enhanced retry
4. **Retry Processing**: Lambda processes retry queue with polynomial delay
5. **Status Tracking**: Record success/failure state in DynamoDB
6. **Final Resolution**: Either succeed or exhaust retries after 20 days

## Configuration

### Environment Variables

- `ENHANCED_RETRY_QUEUE_URL`: SQS queue URL for enhanced retry processing
- `FEDERATION_QUEUE_URL`: Standard federation queue URL (fallback)
- `DOMAIN_NAME`: Instance domain for ActivityPub operations
- `DYNAMODB_TABLE`: Main table for federation activity tracking

### SQS Message Format

```json
{
  "delivery_id": "enhanced_abc123...",
  "activity": { "ActivityPub Object" },
  "signing_actor_id": "username",
  "activity_type": "delete|flag",
  "retry_count": 0,
  "max_retries": 25,
  "retry_policy": "polynomial",
  "max_retry_duration": "480h0m0s",
  "created_at": "2025-08-08T...",
  "recipients": ["https://remote.instance/users/someone"],
  "failed_inboxes": {},
  "successful_inboxes": []
}
```

## Retry Policy Details

### Polynomial Delay Calculation

```go
func calculatePolynomialDelay(attempt int) time.Duration {
    baseDelay := time.Duration(attempt)*time.Second + 15*time.Second
    jitter := time.Duration(generateJitter()) * time.Second // 0-5 seconds
    return baseDelay + jitter
}
```

### Retry Schedule Examples

- Attempt 1: 16-21 seconds (1 + 15 + jitter)
- Attempt 2: 17-22 seconds (2 + 15 + jitter)
- Attempt 5: 20-25 seconds (5 + 15 + jitter)
- Attempt 10: 25-30 seconds (10 + 15 + jitter)
- Attempt 25: 40-45 seconds (25 + 15 + jitter)

### Failure Conditions

- **Max Retries**: 25 attempts exceeded
- **Max Duration**: 20 days since creation
- **Permanent Errors**: HTTP 410, 404 on actor endpoints
- **Circuit Breaker**: Instance marked as permanently unreachable

## Integration Points

### GraphQL Mutations

- `deleteObject`: Now includes mention delivery
- `flagObject`: Now includes enhanced flag federation
- Backward compatible with existing clients

### Federation Infrastructure

- Integrates with existing `DeliveryService`
- Uses established `FederationStorage` interfaces
- Leverages current ActivityPub signing and delivery

### Monitoring and Observability

- CloudWatch metrics for retry success rates
- DynamoDB federation activity tracking
- Structured logging for debugging
- Cost tracking for federation operations

## Testing

### Unit Tests Needed

- [ ] `deliverDeleteToMentions()` function
- [ ] `deliverFlagActivityWithRetry()` function  
- [ ] `calculatePolynomialDelay()` formula
- [ ] Partial failure tracking logic

### Integration Tests Needed

- [ ] End-to-end delete federation with mentions
- [ ] Flag activity federation to remote instances
- [ ] Retry queue processing with Lambda
- [ ] Failure scenarios and fallback behavior

### Load Tests

- [ ] High-volume delete operations
- [ ] Concurrent flag submissions
- [ ] Retry queue backlog handling
- [ ] DynamoDB performance under load

## Deployment Considerations

### Infrastructure Changes

1. **New SQS Queue**: Enhanced retry queue with DLQ
2. **New Lambda**: Enhanced federation processor
3. **DynamoDB Capacity**: Increased for federation tracking
4. **IAM Permissions**: SQS and DynamoDB access

### Rollback Strategy

- Feature flags for enhanced retry
- Fallback to basic retry tracking
- Gradual rollout per instance
- Monitoring dashboards for health

### Performance Impact

- **Minimal Latency**: Immediate attempts preserve user experience
- **Improved Reliability**: Critical activities now federate reliably
- **Cost Efficiency**: Polynomial retry reduces unnecessary attempts
- **Scalability**: SQS-based processing handles high volumes

## Compliance and Standards

- **ActivityPub Spec**: Full compliance with Delete/Flag activities
- **W3C Standards**: Proper Activity Streams 2.0 formatting  
- **Federation Best Practices**: Respectful retry policies
- **Privacy Considerations**: Delete propagation for GDPR compliance

## Future Enhancements

1. **Smart Retry**: Machine learning for optimal retry timing
2. **Priority Queues**: Different retry policies per activity importance
3. **Bulk Operations**: Batch processing for related activities
4. **Instance Health**: Integration with federation health scoring
5. **Real-time Metrics**: Live dashboards for federation health