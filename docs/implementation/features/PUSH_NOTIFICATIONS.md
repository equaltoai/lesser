# Push Notifications Implementation for Lesser

## Overview

This document describes the push notification implementation for Lesser, enabling real-time notifications for mentions, follows, favorites, and other activities.

## Components Implemented

### 1. Data Models
- **Push Subscription Models** (`cmd/api/models/push.go`)
  - `PushSubscription`: Main subscription model with endpoint, keys, and alerts
  - `PushSubscriptionAlerts`: Configurable notification types
  - `PushNotification`: Notification payload structure

### 2. Storage Layer
- **Storage Interface** (`pkg/storage/interface.go`)
  - Added push subscription CRUD operations
  - Added VAPID key storage methods
  
- **DynamoDB Implementation** (`pkg/storage/dynamodb/push_subscriptions.go`)
  - Push subscription storage with endpoint hashing for deduplication
  - VAPID key storage in instance configuration
  
- **Mock Storage** (`internal/testutil/mocks/storage.go`)
  - Added mock implementations for testing

### 3. VAPID Key Management
- **Configure Instance Command** (`cmd/configure-instance/main.go`)
  - Added `-generate-vapid` flag to generate VAPID keys
  - Uses P-256 ECDSA key generation
  - Stores keys securely in DynamoDB

### 4. API Endpoints
- **Push Subscription Handlers** (`cmd/api/handlers/push_subscriptions.go`)
  - `GET /api/v1/push/subscription`: Get current subscription
  - `POST /api/v1/push/subscription`: Create new subscription
  - `PUT /api/v1/push/subscription`: Update subscription alerts
  - `DELETE /api/v1/push/subscription`: Remove subscription

- **Updated Handlers**
  - Apps handler returns VAPID public key during app registration
  - Instance handler returns VAPID public key in configuration

### 5. Push Notification Service
- **Notification Queue Service** (`pkg/notifications/push.go`)
  - Queues notifications to SQS for asynchronous delivery
  - Formats notification titles based on type
  - Gracefully handles when push notifications aren't configured

### 6. Push Delivery Lambda
- **Delivery Lambda** (`cmd/push-delivery/main.go`)
  - Processes messages from SQS queue
  - Implements Web Push Protocol with encryption
  - VAPID JWT signing for authentication
  - Handles subscription cleanup for invalid endpoints

### 7. Activity Processing
- **Updated Activity Processor** (`cmd/activity-processor/main.go`)
  - Sends push notifications for:
    - Follow activities
    - Favorite activities
    - Mention notifications
    - Reblog activities

## Infrastructure Requirements

The following infrastructure needs to be added to Pulumi:

```typescript
// SQS Queue for push notifications
const pushNotificationQueue = new aws.sqs.Queue("push-notifications", {
    visibilityTimeoutSeconds: 300, // 5 minutes
    messageRetentionSeconds: 86400, // 24 hours
});

// Push Delivery Lambda
const pushDeliveryLambda = new aws.lambda.Function("push-delivery", {
    runtime: aws.lambda.Runtime.CustomAL2023,
    architectures: ["arm64"],
    code: new pulumi.asset.FileArchive("../bin/push-delivery.zip"),
    handler: "bootstrap",
    environment: {
        variables: {
            DYNAMO_TABLE_NAME: dynamoTable.name,
            PUSH_NOTIFICATION_QUEUE_URL: pushNotificationQueue.url,
        },
    },
    timeout: 30,
    memorySize: 256,
});

// SQS Event Source for Lambda
new aws.lambda.EventSourceMapping("push-delivery-sqs", {
    eventSourceArn: pushNotificationQueue.arn,
    functionName: pushDeliveryLambda.name,
    batchSize: 10,
});

// Update other Lambda functions with queue URL
// Add to API Lambda, Activity Processor, etc:
PUSH_NOTIFICATION_QUEUE_URL: pushNotificationQueue.url,
```

## Usage

### 1. Generate VAPID Keys
```bash
./bin/configure-instance -generate-vapid
```

### 2. Client Registration
Mastodon clients will receive the VAPID public key when:
- Registering an app (`POST /api/v1/apps`)
- Getting instance information (`GET /api/v2/instance`)

### 3. Subscribe to Push Notifications
```javascript
// Client-side example
const subscription = await registration.pushManager.subscribe({
  userVisibleOnly: true,
  applicationServerKey: vapidPublicKey
});

// Send to server
fetch('/api/v1/push/subscription', {
  method: 'POST',
  headers: {
    'Authorization': 'Bearer ' + accessToken,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    subscription: {
      endpoint: subscription.endpoint,
      keys: {
        p256dh: base64url(subscription.getKey('p256dh')),
        auth: base64url(subscription.getKey('auth'))
      }
    },
    data: {
      alerts: {
        follow: true,
        favourite: true,
        reblog: true,
        mention: true,
        poll: true
      }
    }
  })
});
```

## Security Considerations

1. **VAPID Keys**: Private keys are stored securely in DynamoDB and never exposed via API
2. **Encryption**: All push payloads are encrypted using Web Push Protocol (aes128gcm)
3. **Authentication**: VAPID JWT ensures only our server can send notifications
4. **Subscription Management**: Endpoints are hashed to prevent enumeration

## Testing

1. **Generate VAPID Keys**:
   ```bash
   ./bin/configure-instance -generate-vapid
   ```

2. **Deploy Infrastructure**: 
   ```bash
   cd infra && pulumi up
   ```

3. **Test with Mastodon Client**:
   - Use a client that supports push notifications (e.g., official Mastodon app)
   - Enable notifications in client settings
   - Trigger test activities (follow, mention, etc.) 