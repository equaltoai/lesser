# Federation Enhancements

## Overview

Lesser now includes advanced federation capabilities:

1. **Relay Support** - Connect to ActivityPub relays for broader reach
2. **Authorized Fetch** - Secure federation with signature verification
3. **Instance Allowlist Mode** - Restrict federation to approved instances
4. **Improved Delivery Retry** - Exponential backoff with SQS dead letter queues

## Relay Support

### What are Relays?

ActivityPub relays are specialized servers that help distribute content between instances that don't directly follow each other. When you follow a relay:
- Your public posts are sent to the relay
- The relay forwards them to all other subscribed instances
- You receive public posts from all other subscribed instances

### Implementation

**RelayService** (`pkg/federation/relay.go`)
- Subscribe/unsubscribe to relays
- Handle relay Accept/Reject responses
- Forward public activities to relays
- Process incoming relayed activities

### API Endpoints

```bash
# Subscribe to a relay
POST /api/v1/admin/relays
{
  "inbox_url": "https://relay.example.com/inbox"
}

# List active relays
GET /api/v1/admin/relays

# Unsubscribe from a relay
DELETE /api/v1/admin/relays/:id
```

### DynamoDB Schema

```
# Relay Information
PK: RELAY#<relay_url>
SK: INFO
Attributes:
  - URL: String
  - InboxURL: String
  - Active: Boolean
  - CreatedAt: ISO8601
  - LastSeenAt: ISO8601
  
# Active Relays Index (GSI)
GSI: RELAY_STATUS
PK: ACTIVE#true
SK: RELAY#<relay_url>
```

## Authorized Fetch

### What is Authorized Fetch?

Authorized Fetch (also known as Secure Mode) requires HTTP signatures on all ActivityPub GET requests. This:
- Prevents unauthorized access to non-public content
- Enables better instance-level blocking
- Provides audit trails of who fetches what

### Implementation

**AuthorizedFetchService** (`pkg/federation/authorized_fetch.go`)
- Sign outgoing GET requests
- Verify incoming GET request signatures
- Cache actor public keys for performance
- Handle the actor document bootstrapping problem

### Configuration

```bash
# Enable authorized fetch mode
AUTHORIZED_FETCH_ENABLED=true

# Allow unsigned fetches for actor documents (recommended)
ALLOW_UNSIGNED_ACTOR_FETCH=true
```

### Request Flow

```
Incoming Fetch Request
         ↓
Check Signature Header
         ↓
   Extract KeyID
         ↓
Fetch Actor (may be unsigned)
         ↓
  Verify Signature
         ↓
   Return Content
```

## Instance Allowlist Mode

### Configuration

When domain allowlist is enabled, only instances on the allowlist can federate:

```bash
# Enable allowlist mode
FEDERATION_MODE=allowlist

# Or use instance configuration
{
  "federation": {
    "mode": "allowlist",
    "allowed_domains": [
      "mastodon.social",
      "fosstodon.org"
    ]
  }
}
```

### DynamoDB Schema

```
# Domain Allow List
PK: DOMAIN_ALLOW#<domain>
SK: CONFIG
Attributes:
  - Domain: String
  - CreatedAt: ISO8601
  - CreatedBy: String (admin username)
  - Notes: String (optional)
```

## Improved Delivery Retry

### SQS Integration

Federation delivery now uses SQS for reliable, scalable delivery:

```
Activity Created
      ↓
Determine Recipients
      ↓
Queue to SQS (federation-queue)
      ↓
Lambda Processor
      ↓
Success → Complete
Failure → Retry with backoff
      ↓
Max Retries → DLQ
```

### Retry Configuration

```javascript
{
  "maxRetries": 5,
  "backoffStrategy": "exponential",
  "backoffBase": 60, // seconds
  "maxBackoff": 3600 // 1 hour
}
```

### Monitoring

CloudWatch metrics track:
- Delivery success/failure rates
- Average delivery time
- Queue depth
- DLQ messages

## Security Considerations

### Relay Security
- Only subscribe to trusted relays
- Monitor relay activity for spam
- Implement rate limiting per relay
- Consider content filtering from relays

### Authorized Fetch Trade-offs
- Increased CPU usage for signature verification
- Potential compatibility issues with older software
- Actor document bootstrapping challenges
- Cache invalidation complexity

### Allowlist Management
- Regular review of allowed instances
- Clear criteria for addition/removal
- Communication channel with allowed instances
- Backup federation data before changes

## Performance Optimization

### Relay Performance
- Batch activities to relays when possible
- Use shared inbox for relay delivery
- Monitor relay response times
- Implement relay health checks

### Signature Caching
- Cache verified signatures for 5 minutes
- Cache actor public keys for 24 hours
- Use DynamoDB for distributed cache
- Monitor cache hit rates

### Delivery Optimization
- Group deliveries by shared inbox
- Parallel delivery to different domains
- Connection pooling per domain
- Adaptive timeout based on domain

## Future Enhancements

1. **Relay Discovery** - Automatic discovery of compatible relays
2. **Relay Filtering** - Content-based filtering of relay traffic
3. **Signature Algorithms** - Support for Ed25519 signatures
4. **Delivery Reports** - Detailed delivery status reporting
5. **Federation Analytics** - Track federation patterns and health

## Troubleshooting

### Relay Issues
```bash
# Check relay status
aws dynamodb get-item \
  --table-name lesser-table \
  --key '{"PK": {"S": "RELAY#https://relay.example.com"}, "SK": {"S": "INFO"}}'

# View relay activity
aws logs filter-log-events \
  --log-group-name /aws/lambda/federation-delivery \
  --filter-pattern "relay.example.com"
```

### Authorized Fetch Issues
```bash
# Test signature verification
curl -H "Accept: application/activity+json" \
     -H "Signature: keyId=\"...\",headers=\"...\",signature=\"...\"" \
     https://your-instance.com/users/alice

# Check signature cache
aws dynamodb query \
  --table-name lesser-table \
  --key-condition-expression "PK = :pk" \
  --expression-attribute-values '{":pk": {"S": "CACHE#SIGNATURE"}}'
```

### Delivery Failures
```bash
# Check SQS dead letter queue
aws sqs receive-message \
  --queue-url https://sqs.region.amazonaws.com/account/lesser-federation-dlq

# Retry failed deliveries
aws sqs send-message \
  --queue-url https://sqs.region.amazonaws.com/account/lesser-federation-queue \
  --message-body '{"delivery_id": "...", "retry_count": 0}'
``` 