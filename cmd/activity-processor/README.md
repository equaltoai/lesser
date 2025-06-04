# Activity Processor

The Activity Processor is a crucial component of Lesser that processes activities from DynamoDB Streams and completes the federation loop.

## Overview

This Lambda function is triggered by DynamoDB Streams whenever activities are created in the database. It:

1. **Processes Inbox Activities**: Updates local state based on incoming activities
2. **Processes Outbox Activities**: Delivers activities to remote servers

## Inbox Activity Processing

When activities arrive in a user's inbox, the processor handles:

- **Follow**: Creates a pending follow relationship
- **Accept**: Marks a follow as accepted, updating followers/following
- **Reject**: Marks a follow as rejected and cleans up
- **Create**: Stores the created object (Note, Article, etc.)
- **Like**: Stores the like relationship (TODO: full implementation)
- **Announce**: Stores the boost (TODO: implementation needed)
- **Undo**: Reverses previous activities (TODO: implementation needed)

## Outbox Activity Processing

When local users create activities, the processor:

1. Extracts all recipients from `to`, `cc`, `bto`, and `bcc` fields
2. Filters out special addresses (public) and local users
3. Fetches recipient actor profiles to get inbox URLs
4. Signs requests with HTTP signatures
5. Delivers activities to remote inboxes
6. Handles delivery failures (TODO: implement retry logic)

## HTTP Signature Implementation

All outgoing requests are signed using:
- RSA-SHA256 algorithm
- Headers: `(request-target)`, `host`, `date`, `digest`, `content-type`
- Private keys stored in DynamoDB (encrypted with KMS in production)

## Testing

Run tests with:
```bash
go test -v
```

Current test coverage: ~78.5%

## Environment Variables

- `JWT_SECRET`: Required for config initialization
- `DOMAIN`: The domain of this instance
- `TABLE_NAME`: DynamoDB table name
- `AWS_REGION`: AWS region (defaults to us-east-1)

## Future Improvements

1. Implement retry logic for failed deliveries
2. Add support for Undo activities
3. Implement Announce (boost) processing
4. Add collection resolution for followers/following
5. Implement shared inbox delivery optimization
6. Add metrics and monitoring
7. Implement rate limiting for outgoing requests 