# Phase 4: Instance Features Implementation

## Overview
This document tracks the implementation progress of Phase 4 (Instance Features) from the Mastodon API Implementation Plan.

## Phase 4.1: Instance Information

### Completed ✅

1. **Legacy Instance API**
   - ✅ `GET /api/v1/instance` - Returns instance info in v1 format
   - Handler: `HandleGetInstanceV1` in `cmd/api/handlers/instance.go`
   - Wired in `main.go`

2. **Instance Peers**
   - ✅ `GET /api/v1/instance/peers` - Returns connected federation domains
   - Handler: `HandleGetInstancePeers` in `cmd/api/handlers/instance.go`
   - Simple implementation that checks for remote actors in the system

3. **Instance Activity**
   - ✅ `GET /api/v1/instance/activity` - Returns weekly activity statistics
   - Handler: `HandleGetInstanceActivity` in `cmd/api/handlers/instance.go`
   - Currently returns placeholder data (TODO: wire up actual metrics)

4. **Domain Blocks**
   - ✅ `GET /api/v1/instance/domain_blocks` - Returns public domain blocks
   - Handler: `HandleGetInstanceDomainBlocks` in `cmd/api/handlers/instance.go`
   - Returns empty array (TODO: implement domain block storage)

5. **Legal Documents**
   - ✅ `GET /api/v1/instance/privacy_policy` - Returns privacy policy
   - ✅ `GET /api/v1/instance/terms_of_service` - Returns terms of service
   - ✅ `GET /api/v1/instance/terms_of_service/:date` - Version-specific TOS
   - Handlers in `cmd/api/handlers/instance.go`
   - Content embedded as constants for Lambda compatibility

### Already Existed ✅
- `GET /api/v2/instance` - Instance info v2 format
- `GET /api/v1/instance/rules` - Instance rules
- `GET /api/v1/instance/extended_description` - Extended description
- `GET /api/v1/instance/translation_languages` - Translation languages

## Phase 4.2: Announcements (TODO)

### Endpoints Needed:
- [ ] `GET /api/v1/announcements` - List announcements
- [ ] `POST /api/v1/announcements/:id/dismiss` - Dismiss announcement
- [ ] `PUT /api/v1/announcements/:id/reactions/:name` - Add reaction
- [ ] `DELETE /api/v1/announcements/:id/reactions/:name` - Remove reaction

### Implementation Plan:
1. Create announcements table/storage interface
2. Add methods to DynamoDB storage:
   - `GetAnnouncements(ctx, userID)`
   - `CreateAnnouncement(ctx, announcement)`
   - `DismissAnnouncement(ctx, userID, announcementID)`
   - `AddAnnouncementReaction(ctx, userID, announcementID, reaction)`
   - `RemoveAnnouncementReaction(ctx, userID, announcementID, reaction)`
3. Create handlers in `cmd/api/handlers/announcements.go`
4. Wire up routes in `main.go`

## Phase 4.3: Custom Emojis (TODO)

### Endpoints Needed:
- [ ] `GET /api/v1/custom_emojis` - List custom emojis

### Implementation Plan:
1. Design emoji storage (S3 for images, DynamoDB for metadata)
2. Create emoji upload/management system
3. Integrate emoji rendering in status content
4. Create handler for listing emojis

## Next Steps

1. **Immediate TODOs for Phase 4.1**:
   - Wire up actual activity metrics from DynamoDB
   - Implement domain block storage and management
   - Track federation peers properly

2. **Phase 4.2 - Announcements**:
   - Design announcement data model
   - Implement storage layer
   - Create handlers
   - Test with Mastodon clients

3. **Phase 4.3 - Custom Emojis**:
   - Research Mastodon's emoji format
   - Design storage strategy
   - Implement basic emoji support

## Testing Checklist

### Instance Endpoints:
- [ ] Test `/api/v1/instance` returns v1 format
- [ ] Test `/api/v2/instance` returns v2 format
- [ ] Test `/api/v1/instance/peers` returns array
- [ ] Test `/api/v1/instance/activity` returns 12 weeks of data
- [ ] Test `/api/v1/instance/domain_blocks` returns array
- [ ] Test `/api/v1/instance/privacy_policy` returns HTML
- [ ] Test `/api/v1/instance/terms_of_service` returns HTML

### Compatibility:
- [ ] Test with Mastodon web client
- [ ] Test with Tusky/other mobile clients
- [ ] Verify format matches Mastodon's responses

## Notes

1. **Legal Documents**: Currently embedded as constants in the code. In production, consider:
   - Storing in DynamoDB with versioning
   - Loading from S3
   - Using environment variables for small instances

2. **Activity Metrics**: Need to implement proper tracking:
   - User login events
   - Status creation counts
   - Registration tracking
   - Weekly aggregation Lambda

3. **Federation Peers**: Current implementation is basic. Should:
   - Track peers when federating
   - Store in dedicated table
   - Include last contact time
   - Handle peer removal 