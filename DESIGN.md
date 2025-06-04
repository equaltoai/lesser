# Lesser - Serverless ActivityPub Design Document

## Overview

Lesser is a serverless ActivityPub implementation designed to minimize hosting costs while providing full ActivityPub compliance. It leverages AWS Lambda for compute, DynamoDB for storage, and Pulumi for infrastructure as code.

## Core ActivityPub Requirements

### 1. Actor Model
- **Actor Types**: Person, Service, Group, Organization, Application
- **Required Properties**: id, type, inbox, outbox, preferredUsername
- **Optional Properties**: followers, following, liked, shares, publicKey

### 2. Collections
- **Inbox**: Receives activities from other actors
- **Outbox**: Stores activities created by the actor
- **Followers**: Collection of actors following this actor
- **Following**: Collection of actors this actor follows
- **Liked**: Collection of objects this actor has liked

### 3. Activities
Core activity types to support:
- **Create**: Create new objects (posts, notes, etc.)
- **Update**: Modify existing objects
- **Delete**: Remove objects
- **Follow**: Follow another actor
- **Accept/Reject**: Respond to follow requests
- **Like**: Express approval of an object
- **Announce**: Share/boost an object
- **Undo**: Reverse a previous activity

### 4. Objects
- **Note**: Basic text posts
- **Article**: Long-form content
- **Image**: Photo posts
- **Video**: Video content
- **Document**: Generic file attachments

### 5. Federation Requirements
- **WebFinger**: Actor discovery via .well-known/webfinger
- **HTTP Signatures**: Authentication for server-to-server communication
- **Activity Delivery**: POST activities to remote inboxes
- **Content Negotiation**: Support ActivityStreams JSON-LD format

## Architecture

### Lambda Functions (`cmd/`)

1. **cmd/webfinger**
   - Handles `.well-known/webfinger` requests
   - Returns actor information for discovery

2. **cmd/actor**
   - GET: Returns actor profile
   - Supports content negotiation (HTML vs ActivityStreams)

3. **cmd/inbox**
   - POST: Receives activities from other servers
   - Validates HTTP signatures
   - Processes incoming activities

4. **cmd/outbox**
   - GET: Returns actor's public activities
   - POST: Creates new activities (client-to-server)

5. **cmd/collections**
   - GET: Returns followers/following/liked collections
   - Supports pagination

6. **cmd/activity-processor**
   - Background Lambda for processing activities
   - Triggered by SQS/DynamoDB streams
   - Handles federation (delivering to remote servers)

7. **cmd/media**
   - Handles media uploads to S3
   - Generates thumbnails
   - Returns media URLs

### Data Models (DynamoDB)

#### Table: Actors
```
PK: ACTOR#{username}
SK: PROFILE
Attributes:
  - id: string (full actor URL)
  - type: string (Person, Service, etc.)
  - username: string
  - displayName: string
  - summary: string (bio)
  - publicKey: string
  - privateKey: string (encrypted)
  - inbox: string (URL)
  - outbox: string (URL)
  - followers: string (URL)
  - following: string (URL)
  - createdAt: timestamp
```

#### Table: Activities
```
PK: ACTOR#{username}
SK: ACTIVITY#{timestamp}#{id}
GSI1PK: INBOX#{username}
GSI1SK: {timestamp}
Attributes:
  - id: string (activity URL)
  - type: string (Create, Follow, etc.)
  - actor: string (actor URL)
  - object: JSON
  - to: array
  - cc: array
  - published: timestamp
```

#### Table: Relationships
```
PK: FOLLOW#{follower_username}
SK: FOLLOWING#{followed_username}
GSI1PK: FOLLOW#{followed_username}
GSI1SK: FOLLOWER#{follower_username}
Attributes:
  - id: string (follow activity URL)
  - state: string (pending, accepted, rejected)
  - createdAt: timestamp
```

#### Table: Objects
```
PK: OBJECT#{id}
SK: VERSION#{timestamp}
Attributes:
  - id: string (object URL)
  - type: string (Note, Article, etc.)
  - content: string
  - attributedTo: string (actor URL)
  - published: timestamp
  - updated: timestamp
  - to: array
  - cc: array
  - inReplyTo: string (optional)
  - attachments: array
```

### API Gateway Setup

```
/
├── .well-known/
│   └── webfinger          → cmd/webfinger
├── users/
│   ├── {username}         → cmd/actor
│   ├── {username}/inbox   → cmd/inbox
│   ├── {username}/outbox  → cmd/outbox
│   ├── {username}/followers → cmd/collections
│   ├── {username}/following → cmd/collections
│   └── {username}/liked   → cmd/collections
├── activities/
│   └── {id}              → cmd/activity
└── objects/
    └── {id}              → cmd/object
```

### Security & Authentication

1. **HTTP Signatures**
   - Verify signatures on all incoming federation requests
   - Sign all outgoing federation requests
   - Use RSA-SHA256

2. **API Keys** (for client-to-server)
   - JWT tokens for authenticated endpoints
   - Refresh token rotation

3. **Rate Limiting**
   - API Gateway throttling
   - DynamoDB conditional writes

### Background Processing

1. **SQS Queues**
   - Delivery queue for outgoing activities
   - Processing queue for incoming activities
   - Media processing queue

2. **DynamoDB Streams**
   - Trigger activity delivery on new outbox items
   - Update follower counts
   - Generate notifications

### Cost Optimization Strategies

1. **Lambda**
   - Use ARM-based Graviton2 processors
   - Optimize cold starts with lightweight runtime
   - Implement connection pooling for HTTP clients

2. **DynamoDB**
   - Use on-demand billing for variable traffic
   - Implement efficient query patterns
   - Archive old activities to S3

3. **API Gateway**
   - Enable caching for actor profiles
   - Use CloudFront for static content
   - Implement request validation

4. **S3**
   - Lifecycle policies for media
   - Intelligent tiering
   - CloudFront CDN for media delivery

## Implementation Phases

### Phase 1: Core ActivityPub (MVP)
- [ ] Actor profiles
- [ ] WebFinger discovery
- [ ] Basic inbox/outbox
- [ ] HTTP signatures
- [ ] Follow/Accept activities

### Phase 2: Content Creation
- [ ] Create Note activities
- [ ] Delete activities
- [ ] Update activities
- [ ] Basic timeline

### Phase 3: Social Features
- [ ] Like activities
- [ ] Announce (boost) activities
- [ ] Replies and threads
- [ ] Mentions and notifications

### Phase 4: Media Support
- [ ] Image uploads
- [ ] Video uploads
- [ ] Thumbnail generation
- [ ] CDN delivery

### Phase 5: Advanced Features
- [ ] Search functionality
- [ ] Moderation tools
- [ ] Instance blocks
- [ ] Custom emojis

## Estimated Costs (per month)

### Small Instance (100 users, 10K activities/month)
- Lambda: ~$5
- DynamoDB: ~$10
- API Gateway: ~$3
- S3: ~$5
- **Total: ~$23/month**

### Medium Instance (1K users, 100K activities/month)
- Lambda: ~$50
- DynamoDB: ~$50
- API Gateway: ~$30
- S3: ~$20
- **Total: ~$150/month**

### Large Instance (10K users, 1M activities/month)
- Lambda: ~$200
- DynamoDB: ~$200
- API Gateway: ~$100
- S3: ~$100
- **Total: ~$600/month**

## Next Steps

1. Set up Go module structure
2. Implement WebFinger endpoint
3. Create actor profile endpoint
4. Set up DynamoDB tables with Pulumi
5. Implement HTTP signature verification
6. Create inbox endpoint for receiving activities 