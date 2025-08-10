# Lesser WebSocket Streaming Implementation

## Overview

Lesser now supports real-time activity streaming via WebSocket connections, enabling clients to receive live updates for timelines, notifications, and other events. This implementation follows the Mastodon streaming API specification while leveraging AWS serverless architecture.

## WebSocket-Only Architecture

Lesser uses **WebSocket connections only** and does not support Server-Sent Events (SSE) for streaming. This architectural decision is driven by serverless constraints and performance optimization.

### Why No SSE Support?

**Serverless Lambda Limitations:**
- Lambda functions have a maximum timeout of 15 minutes
- Long-running HTTP connections are not suitable for Lambda architecture
- SSE requires persistent HTTP connections that can't be maintained across Lambda invocations
- Automatic scaling would break existing SSE connections

**Cost Optimization:**
- Lambda billing is per-request, not per-connection-time
- Long-running SSE connections would result in expensive timeout-based billing
- WebSocket API Gateway provides dedicated infrastructure for persistent connections
- Pay-per-message model is more cost-effective than pay-per-connection-time

**Performance Benefits:**
- WebSocket API Gateway handles 10,000+ concurrent connections natively
- Bi-directional communication enables efficient subscription management
- Connection lifecycle management is handled by AWS infrastructure
- Automatic dead connection detection and cleanup

### Client Implementation Notes

**Traditional Mastodon Clients:**
Most Mastodon clients support both WebSocket and SSE streaming. Lesser's WebSocket-only approach is fully compatible with existing Mastodon client libraries that auto-negotiate connection types.

**Web Browser Support:**  
Modern browsers support WebSocket connections natively. If you're building a web client, use the WebSocket API instead of EventSource for SSE:

```javascript
// ✅ Use WebSocket (supported)
const ws = new WebSocket('wss://lesser.example.com/api/v1/streaming?access_token=' + token);

// ❌ Don't use SSE (not supported)
// const eventSource = new EventSource('/api/v1/streaming?access_token=' + token);
```

**Mobile App Considerations:**
WebSocket connections are more efficient on mobile devices as they allow proper connection lifecycle management and battery optimization through connection pooling.

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

WebSocket streaming can be tested using:
- WebSocket client libraries in your preferred programming language
- Command-line tools like `wscat` for quick testing
- Mastodon client applications that support streaming

Example connection:
```bash
wscat -c "wss://lesser.example.com/api/v1/streaming?access_token=YOUR_TOKEN"
```

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