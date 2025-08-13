# Phase 2: Content Features - Progress Report

## Overview
Phase 2 focuses on enhanced status functionality, scheduled posts, and hashtag features for the Lesser ActivityPub implementation.

## Completed Features

### 2.1 Status Enhancements - Mostly Complete

#### ✅ Status Pinning
- **Implemented**: `POST /api/v1/statuses/:id/pin` - Pin status to profile
- **Implemented**: `POST /api/v1/statuses/:id/unpin` - Unpin from profile
- **Features**:
  - Maximum 5 pinned statuses per user (Mastodon-compatible limit)
  - Ownership validation (can only pin your own statuses)
  - Proper error handling for duplicate pins and limit exceeded
  - DynamoDB storage implementation with efficient queries

#### ✅ Conversation Muting
- **Implemented**: `POST /api/v1/statuses/:id/mute` - Mute conversation
- **Implemented**: `POST /api/v1/statuses/:id/unmute` - Unmute conversation
- **Features**:
  - Optional duration support for temporary mutes
  - TTL-based auto-expiration in DynamoDB
  - Idempotent operations
  - Conversation tracking (currently uses status ID as conversation ID)

#### ✅ Status Information
- **Implemented**: `GET /api/v1/statuses/:id/source` - View source
  - Returns raw text content before HTML processing
  - Includes spoiler text if present
  - Simple and lightweight response
  
- **Implemented**: `GET /api/v1/statuses/:id/history` - View edit history
  - Returns all versions of the status
  - Includes edit timestamps
  - Shows content changes over time
  - Current version included as most recent edit

#### ✅ Status Editing
- **Already Implemented**: `PUT /api/v1/statuses/:id` - Edit status
  - Full edit history tracking via UpdateHistory
  - Automatic versioning
  - Previous state preservation
  - Update activity generation for federation

### 2.2 Scheduled Statuses - Complete

#### ✅ Storage Implementation
- **Implemented**: `pkg/storage/dynamodb/scheduled_statuses.go`
  - Full CRUD operations
  - Query by user and scheduled time
  - GSI for efficient due status queries
  - Published status tracking

#### ✅ API Endpoints
- **Implemented**: `GET /api/v1/scheduled_statuses` - List scheduled
  - Pagination support with Link headers
  - Filtering by published status
  - Ownership validation

- **Implemented**: `GET /api/v1/scheduled_statuses/:id` - View single
  - Returns full status parameters
  - Media attachment placeholders

- **Implemented**: `PUT /api/v1/scheduled_statuses/:id` - Update
  - Can update scheduled time
  - Validation for future dates (5+ minutes)

- **Implemented**: `DELETE /api/v1/scheduled_statuses/:id` - Cancel
  - Removes from schedule
  - Ownership validation

#### ✅ Integration with Status Creation
- Modified `POST /api/v1/statuses` to support `scheduled_at` parameter
- Validates scheduled time is in the future
- Returns ScheduledStatus response instead of Status when scheduled

#### Storage Layer Updates
- Added new data structures:
  - `StatusPin` - Tracks pinned statuses per user
  - `ConversationMute` - Tracks muted conversations with optional expiration
  - `ScheduledStatus` - Complete scheduled post storage
  - `UpdateHistory` - Already existed for edit history
- DynamoDB implementations:
  - `pkg/storage/dynamodb/status_pins.go` - Complete implementation
  - `pkg/storage/dynamodb/conversation_mutes.go` - Complete implementation
  - `pkg/storage/dynamodb/scheduled_statuses.go` - Complete implementation
- Updated storage interface with all Phase 2 methods

## Remaining Tasks for Phase 2

### 2.1 Status Enhancements (Remaining)

#### Translation
- [ ] `POST /api/v1/statuses/:id/translate` - Translate status
  - Integrate with translation service (AWS Translate or similar)
  - Cache translations
  - Support language detection
  
- [ ] `GET /api/v1/instance/translation_languages` - Supported languages
  - Return list of supported language pairs
  - Include language codes and names

### 2.2 Scheduled Statuses (Remaining)

#### Scheduling Service
- [ ] Create Lambda function for scheduled publishing
  - Poll for due statuses using GetDueScheduledStatuses
  - Create actual status when scheduled time arrives
  - Mark as published after successful creation
  - Handle failures and retries
  - Clean up old published scheduled statuses

### 2.3 Tags & Hashtags (Not Started)

#### Hashtag Following
- [ ] Storage for hashtag follows
- [ ] `GET /api/v1/tags/:id` - View hashtag info
- [ ] `POST /api/v1/tags/:id/follow` - Follow hashtag
- [ ] `POST /api/v1/tags/:id/unfollow` - Unfollow hashtag
- [ ] `GET /api/v1/followed_tags` - List followed hashtags

#### Featured Tags
- [ ] Storage for featured tags per user
- [ ] `GET /api/v1/featured_tags` - List featured
- [ ] `POST /api/v1/featured_tags` - Feature a tag
- [ ] `DELETE /api/v1/featured_tags/:id` - Unfeature
- [ ] `GET /api/v1/featured_tags/suggestions` - Suggestions based on usage
- [ ] `GET /api/v1/accounts/:id/featured_tags` - Account's featured tags

## Implementation Notes

### DynamoDB Key Design
- Status pins: `PK=USER#{username}#PINS, SK=STATUS#{statusId}`
- Conversation mutes: `PK=USER#{username}#CONV_MUTES, SK=CONV#{conversationId}`
- Scheduled statuses: `PK=USER#{username}#SCHEDULED, SK=ID#{id}`
- Update history: `PK=OBJECT#{objectId}#HISTORY, SK=VERSION#{version}`

### Scheduled Status Publishing
The storage layer is ready for a background service to:
1. Query due statuses with `GetDueScheduledStatuses`
2. Create the actual status via the standard creation flow
3. Mark as published with `MarkScheduledStatusPublished`
4. Could be implemented as:
   - Lambda with CloudWatch Events trigger
   - ECS task with cron schedule
   - Step Functions workflow

### Testing
- Created `test_phase2_status_features.py` for integration testing
- Tests needed for:
  - Status source endpoint
  - Status history endpoint  
  - Scheduled status CRUD operations
  - Scheduled status creation flow

## Architecture Decisions

### Why DynamoDB TTL for Temporary Mutes?
- Automatic cleanup without background jobs
- Cost-effective for temporary data
- No maintenance overhead

### Scheduled Status Design
- Stores full status parameters for future publishing
- GSI enables efficient queries for due statuses
- Published flag prevents double-publishing
- Scan operation for single status lookup is acceptable due to low volume

### Edit History Storage
- Uses existing UpdateHistory structure
- Versions stored with padded numbers for proper sorting
- Previous state stored as JSON for flexibility
- No limit on history entries (could add TTL if needed)

## Estimated Timeline

- **Week 1**: ✅ Status pinning, conversation muting, source/history, scheduled statuses
- **Week 2**: Translation integration and scheduling service
- **Week 3**: Hashtag following and featured tags
- **Week 4**: Testing, bug fixes, and polish

## Dependencies

### External Services Needed
- Translation service (AWS Translate recommended)
- Scheduling trigger (CloudWatch Events or EventBridge)

### Infrastructure Updates
- DynamoDB table already supports new patterns
- Lambda function needed for scheduled post publishing

## Success Metrics
- All Phase 2 endpoints implemented and tested
- Mastodon client compatibility verified
- Performance within acceptable limits (<200ms for pin/mute operations)
- Storage costs remain reasonable
- Scheduled posts publish within 1 minute of scheduled time 