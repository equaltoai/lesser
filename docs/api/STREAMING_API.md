# Lesser WebSocket Streaming Implementation

## Overview

Lesser now supports real-time activity streaming via WebSocket connections, enabling clients to receive live updates for timelines, notifications, and other events. This implementation follows the Mastodon streaming API specification while leveraging AWS serverless architecture.

## Architecture

### Components

1. **WebSocket API Gateway** - Manages WebSocket connections and routes messages
2. **Streaming Lambda** (`cmd/streaming`) - Handles connection management and subscriptions
3. **Stream Router Lambda** (`cmd/stream-router`) - Processes DynamoDB streams and broadcasts events
4. **DynamoDB Tables**:
   - `lesser-streaming-connections` - Active WebSocket connections
   - `lesser-streaming-subscriptions` - Stream subscriptions by connection

### Flow

```
Client → WebSocket API Gateway → Streaming Lambda
                                      ↓
                              Connection Table
                                      ↓
DynamoDB Streams → Stream Router → API Gateway Management API → Client
```

## API Endpoints

### WebSocket Endpoint
```
wss://{domain}/api/v1/streaming?access_token={token}
```

### Authentication
- Pass OAuth access token as `access_token` query parameter
- Or send `Authorization: Bearer {token}` header (less common for WebSocket)

## Message Protocol

### Client Messages

#### Subscribe
```json
{
  "type": "subscribe",
  "stream": "public"
}
```

#### Unsubscribe
```json
{
  "type": "unsubscribe",
  "stream": "public"
}
```

#### Ping
```json
{
  "type": "ping"
}
```

### Server Messages

#### Stream Event
```json
{
  "event": "update",
  "payload": { /* Mastodon Status object */ },
  "stream": "public"
}
```

#### Subscription Confirmation
```json
{
  "type": "subscribed",
  "stream": "public",
  "payload": {
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

#### Error
```json
{
  "type": "error",
  "payload": {
    "error": "Invalid stream: unknown",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

## Available Streams

- `public` - All public statuses
- `public:local` - Public statuses from local users
- `public:remote` - Public statuses from remote instances
- `user` - User's home timeline (requires auth)
- `user:notification` - User's notifications (requires auth)
- `list:{id}` - List timeline (requires auth)
- `direct` - Direct messages (requires auth)
- `hashtag:{tag}` - Hashtag timeline
- `hashtag:local:{tag}` - Local hashtag timeline

## Event Types

- `update` - New status posted
- `delete` - Status deleted
- `notification` - New notification
- `status.update` - Status edited
- `announcement` - New announcement
- `announcement.delete` - Announcement deleted

## Implementation Details

### Connection Management
- Connections stored in DynamoDB with 24-hour TTL
- Automatic cleanup on disconnect
- Connection state includes user ID and subscribed streams

### Event Routing
- DynamoDB streams trigger the stream router Lambda
- Events are filtered and routed based on:
  - Status visibility
  - User relationships (followers, mentions)
  - Stream subscriptions
- Dead connections are automatically cleaned up

### Scalability
- WebSocket API Gateway handles up to 10,000 concurrent connections per route
- Lambda functions scale automatically
- DynamoDB pay-per-request billing
- Parallel processing of stream records

### Cost Optimization
- Connections expire after 24 hours
- Efficient subscription queries using partition keys
- Batch processing of DynamoDB stream records
- No polling - events pushed only when data changes

## Testing

Use the provided test script:

```bash
python test_streaming.py https://lesser.example.com your_access_token
```

The test script will:
1. Connect to the WebSocket endpoint
2. Subscribe to various streams
3. Listen for events
4. Test concurrent connections
5. Verify proper cleanup on disconnect

## Monitoring

### CloudWatch Metrics
- WebSocket connection count
- Message delivery success/failure rates
- Lambda invocation counts and errors
- DynamoDB read/write capacity

### Debugging
- Connection IDs logged for tracing
- Failed message deliveries logged with reasons
- Automatic cleanup of stale subscriptions

## Future Enhancements

1. **Filtering** - Client-side event filtering
2. **Presence** - Online status for users
3. **Typing Indicators** - Show when users are typing
4. **Read Receipts** - Track message read status
5. **Custom Streams** - Application-specific event streams
6. **Stream Compression** - Reduce bandwidth usage
7. **Regional Endpoints** - Multi-region support

## Security Considerations

- OAuth tokens validated on connection
- Connections authenticated before subscription
- Private streams require appropriate permissions
- Rate limiting at API Gateway level
- Automatic connection expiry

## Compliance

The implementation follows:
- Mastodon Streaming API specification
- ActivityPub protocol for event types
- OAuth 2.0 for authentication
- WebSocket protocol standards 