# Notification Processor

The notification-processor is a Phase 2 Lambda function that handles the delivery of notifications across supported channels: push notifications and WebSocket connections.

## Overview

This processor consumes messages from an SQS queue containing notification delivery requests and attempts to deliver notifications through the requested channels. It supports retry logic, delivery tracking, and cost monitoring.

## Architecture

The notification processor follows the established DynamORM patterns used throughout Lesser:

- **Repository Pattern**: Uses `NotificationRepository` for database operations
- **Lambda Optimization**: Initializes DynamORM with Lambda-specific optimizations
- **Error Handling**: Implements proper error handling and retry logic
- **Cost Tracking**: Tracks delivery costs for each channel

## Delivery Channels

### Push Notifications
- Uses AWS SNS for push delivery to mobile devices
- Supports FCM (Android) and APNS (iOS) endpoints
- Includes notification payload with action data
- Cost: ~$0.0005 per push notification

### WebSocket Delivery
- Integrates with existing streaming infrastructure
- Delivers real-time notifications to active connections
- Handles multiple concurrent connections per user
- Cost: ~$0.0001 per WebSocket message

## Message Format

The processor expects SQS messages with the following JSON format:

```json
{
  "notification_id": "notif_123456",
  "user_id": "user_789",
  "channels": ["push", "websocket"],
  "priority": "high",
  "retry_count": 0,
  "scheduled_at": "2024-01-15T10:30:00Z"
}
```

### Fields

- `notification_id`: ID of the notification to deliver
- `user_id`: Target user for the notification
- `channels`: Array of delivery channels to attempt
- `priority`: Delivery priority (high, medium, low)
- `retry_count`: Current retry attempt (0-based)
- `scheduled_at`: Optional scheduled delivery time

## Configuration

The processor requires the following environment variables:

```bash
# Required
DOMAIN=your-instance.com
DYNAMODB_TABLE_NAME=lesser-main
AWS_REGION=us-east-1

# WebSocket configuration
WEBSOCKET_ENDPOINT=wss://your-websocket-api.execute-api.region.amazonaws.com/stage

# AWS service configuration (automatic from IAM role)
# - SNS permissions for push notifications
# - API Gateway Management API permissions for WebSocket
```

## Retry Logic

The processor implements exponential backoff retry logic:

1. **Initial Attempt**: Immediate delivery attempt
2. **First Retry**: After 30 seconds if failed
3. **Second Retry**: After 2 minutes if failed
4. **Third Retry**: After 10 minutes if failed
5. **Final Failure**: Mark notification as permanently failed

## User Preferences

The processor respects user notification preferences:

- `push_notifications`: Enable/disable push delivery  
- `websocket_notifications`: Enable/disable real-time delivery
- `push_endpoint`: FCM/APNS endpoint for push delivery

## Delivery Tracking

Each delivery attempt is tracked with:

- **Channel**: The delivery channel used
- **Success**: Whether delivery succeeded
- **Error**: Error message if delivery failed
- **Timestamp**: When delivery was attempted
- **Cost**: Micro-dollar cost of delivery

This data is stored in the notification's `Data` field for analytics and billing.

## Testing

Run the unit tests:

```bash
cd cmd/notification-processor
go test -v
```

The tests cover:
- Message parsing and validation
- Push notification formatting
- Channel delivery logic
- Retry handling
- Cost tracking

## Performance Characteristics

- **Cold Start**: ~100ms with DynamORM Lambda optimization
- **Concurrency**: Processes up to 5 messages concurrently
- **Throughput**: ~50 notifications/second sustained
- **Cost**: $0.0001-$0.001 per notification depending on channels

## Integration with Lesser

The notification processor integrates with:

1. **Notification Creation**: Other Lambda functions queue delivery requests
2. **User Management**: Fetches user preferences and contact info
3. **Streaming Service**: Delivers real-time WebSocket notifications
4. **Analytics**: Tracks delivery metrics for monitoring

## Error Handling

The processor handles various error scenarios:

- **Invalid JSON**: Logs error and skips message
- **Missing Notification**: Logs error and marks as failed
- **Service Unavailable**: Retries with exponential backoff
- **User Preferences**: Respects disabled channels gracefully
- **Rate Limiting**: Implements circuit breaker patterns

## Monitoring

Key metrics to monitor:

- **Delivery Success Rate**: Percentage of successful deliveries
- **Retry Rate**: Percentage of messages requiring retries
- **Channel Performance**: Success rates by delivery channel
- **Cost per Notification**: Average delivery cost tracking
- **Processing Time**: End-to-end delivery latency

## Future Enhancements

Potential improvements:

1. **Batch Processing**: Group notifications for bulk delivery
2. **Smart Scheduling**: Optimize delivery timing per user
3. **A/B Testing**: Test different notification formats
4. **Rich Content**: Support for images and interactive elements
5. **Delivery Analytics**: Advanced user engagement tracking