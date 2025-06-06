# Mastodon API Implementation Plan for Lesser

## Overview
This document outlines all remaining work needed to achieve full Mastodon API compatibility in Lesser. It combines missing API endpoints with TODO items found throughout the codebase.

## Implementation Status Summary
- ✅ Core ActivityPub functionality
- ✅ Basic Mastodon API endpoints (accounts, statuses, timelines, notifications)
- ✅ Lists, filters, bookmarks, favorites
- ✅ Media upload and management
- ✅ Push notifications
- ✅ Advanced account search with multiple strategies (exact, prefix, display name, popularity, fuzzy, semantic)
- ✅ OAuth implementation (authorize, token, discovery - only revoke missing)
- ⚠️ Conversations implemented but not routed
- ⚠️ Search v1 implemented but missing hashtag/status search
- ❌ Many secondary endpoints missing

## Phase 1: Critical Missing Endpoints (Week 1-2)

### 1.1 OAuth & Authentication
**Priority: HIGH** - OAuth is mostly implemented

#### Status:
- ✅ `GET /oauth/authorize` - OAuth authorization page (IMPLEMENTED)
- ✅ `POST /oauth/token` - Obtain token (IMPLEMENTED)
- ✅ `GET /.well-known/oauth-authorization-server` - OAuth server metadata (IMPLEMENTED)
- ✅ PKCE support (IMPLEMENTED)
- ✅ OAuth grants stored in DynamoDB (IMPLEMENTED)
- ✅ Authorization UI page (IMPLEMENTED)

#### Remaining Tasks:
- [ ] `POST /oauth/revoke` - Revoke token endpoint
- [ ] `GET /api/v1/apps/verify_credentials` - Verify app credentials

### 1.2 Conversations API
**Priority: HIGH** - Already implemented but not routed

#### Tasks:
- [ ] Wire up existing conversation handlers in main.go
  - [ ] `GET /api/v1/conversations` - List conversations
  - [ ] `DELETE /api/v1/conversations/:id` - Delete conversation
  - [ ] `POST /api/v1/conversations/:id/read` - Mark as read
- [ ] Fix conversation participant tracking (TODO in conversations.go:160)

### 1.3 Account Relationships
**Priority: HIGH** - Core social features

#### Tasks:
- [ ] Implement follower/following endpoints
  - [ ] `GET /api/v1/accounts/:id/followers` - Get account followers
  - [ ] `GET /api/v1/accounts/:id/following` - Get who account follows
  - [ ] `GET /api/v1/accounts/familiar_followers` - Find mutual followers
- [ ] Account management features
  - [ ] `POST /api/v1/accounts/:id/pin` - Pin account to profile
  - [ ] `POST /api/v1/accounts/:id/unpin` - Unpin account
  - [ ] `POST /api/v1/accounts/:id/note` - Set private note
  - [ ] `POST /api/v1/accounts/:id/remove_from_followers` - Remove follower

## Phase 2: Content Features (Week 3-4)

### 2.1 Status Enhancements
**Priority: MEDIUM** - Enhanced status functionality

#### Tasks:
- [ ] Status management
  - [ ] `POST /api/v1/statuses/:id/mute` - Mute conversation
  - [ ] `POST /api/v1/statuses/:id/unmute` - Unmute conversation
  - [ ] `POST /api/v1/statuses/:id/pin` - Pin to profile
  - [ ] `POST /api/v1/statuses/:id/unpin` - Unpin from profile
- [ ] Status information
  - [ ] `GET /api/v1/statuses/:id/source` - View source
  - [ ] `GET /api/v1/statuses/:id/history` - View edit history
  - [ ] `PUT /api/v1/statuses/:id` - Edit status (full implementation)
- [ ] Translation
  - [ ] `POST /api/v1/statuses/:id/translate` - Translate status
  - [ ] `GET /api/v1/instance/translation_languages` - Supported languages

### 2.2 Scheduled Statuses
**Priority: MEDIUM** - Scheduling functionality

#### Tasks:
- [ ] Implement scheduled status storage
- [ ] Create scheduling service
- [ ] Implement endpoints:
  - [ ] `GET /api/v1/scheduled_statuses` - List scheduled
  - [ ] `GET /api/v1/scheduled_statuses/:id` - View single
  - [ ] `PUT /api/v1/scheduled_statuses/:id` - Update
  - [ ] `DELETE /api/v1/scheduled_statuses/:id` - Cancel

### 2.3 Tags & Hashtags
**Priority: MEDIUM** - Hashtag following

#### Tasks:
- [ ] Hashtag information
  - [ ] `GET /api/v1/tags/:id` - View hashtag info
  - [ ] `POST /api/v1/tags/:id/follow` - Follow hashtag
  - [ ] `POST /api/v1/tags/:id/unfollow` - Unfollow hashtag
  - [ ] `GET /api/v1/followed_tags` - List followed hashtags
- [ ] Featured tags
  - [ ] `GET /api/v1/featured_tags` - List featured
  - [ ] `POST /api/v1/featured_tags` - Feature a tag
  - [ ] `DELETE /api/v1/featured_tags/:id` - Unfeature
  - [ ] `GET /api/v1/featured_tags/suggestions` - Suggestions
  - [ ] `GET /api/v1/accounts/:id/featured_tags` - Account's featured tags

## Phase 3: Discovery & Trends (Week 5-6)

### 3.1 Search Implementation Review
**Priority: MEDIUM** - Search is partially implemented

#### Current Status:
- ✅ Basic search endpoint (`GET /api/v1/search`) - IMPLEMENTED
- ✅ Account search with multiple strategies:
  - ✅ Exact match strategy
  - ✅ Prefix search strategy (using GSI1)
  - ✅ Display name search (using GSI2)
  - ✅ Popularity search (using GSI4)
  - ✅ Fuzzy search (OpenSearch integration)
  - ✅ Semantic search (AWS Bedrock integration)
- ✅ Search suggestions endpoint - IMPLEMENTED
- ✅ GraphQL search support - IMPLEMENTED
- ✅ Remote actor search via WebFinger - IMPLEMENTED
- ✅ Search caching and analytics - IMPLEMENTED

#### Remaining Tasks:
- [ ] `GET /api/v2/search` - Grouped search results (v1 exists, need v2 format)
- [ ] Hashtag search implementation (TODO in misc.go:146)
- [ ] Status/post search implementation (TODO in misc.go:113)
- [ ] Add language detection (TODO in search_service.go:190)
- [ ] Extract user context for personalized search (TODO in search_service.go:157)
- [ ] Implement search filters (following only, local only, etc.)

### 3.2 Trends & Discovery
**Priority: LOW** - Trending content

#### Tasks:
- [ ] Implement trending algorithm
- [ ] Create trend tracking service
- [ ] Implement endpoints:
  - [ ] `GET /api/v1/trends` - General trends
  - [ ] `GET /api/v1/trends/statuses` - Trending posts
  - [ ] `GET /api/v1/trends/tags` - Trending hashtags
  - [ ] `GET /api/v1/trends/links` - Trending links
  - [ ] `GET /api/v1/timelines/link` - Link timeline
- [ ] Discovery features:
  - [ ] `GET /api/v1/directory` - Profile directory
  - [ ] `GET /api/v1/suggestions` - Follow suggestions (v1)
  - [ ] `GET /api/v2/suggestions` - Follow suggestions (v2)
  - [ ] `DELETE /api/v1/suggestions/:account_id` - Remove suggestion

## Phase 4: Instance Features (Week 7-8)

### 4.1 Instance Information
**Priority: MEDIUM** - Server metadata

#### Tasks:
- [ ] Instance endpoints:
  - [ ] `GET /api/v1/instance` - Legacy instance info
  - [ ] `GET /api/v1/instance/peers` - Connected domains (implement peer discovery)
  - [ ] `GET /api/v1/instance/activity` - Activity statistics
  - [ ] `GET /api/v1/instance/domain_blocks` - Public domain blocks
- [ ] Legal documents:
  - [ ] `GET /api/v1/instance/privacy_policy` - Privacy policy
  - [ ] `GET /api/v1/instance/terms_of_service` - Terms of service
  - [ ] `GET /api/v1/instance/terms_of_service/:date` - Specific TOS version
- [ ] Update instance configuration (TODO in misc.go:359, 421)

### 4.2 Announcements
**Priority: LOW** - Server announcements

#### Tasks:
- [ ] Create announcements table
- [ ] Implement announcement service
- [ ] Implement endpoints:
  - [ ] `GET /api/v1/announcements` - List announcements
  - [ ] `POST /api/v1/announcements/:id/dismiss` - Dismiss
  - [ ] `PUT /api/v1/announcements/:id/reactions/:name` - Add reaction
  - [ ] `DELETE /api/v1/announcements/:id/reactions/:name` - Remove reaction

### 4.3 Custom Emojis
**Priority: LOW** - Custom emoji support

#### Tasks:
- [ ] Design emoji storage
- [ ] Implement emoji service
- [ ] `GET /api/v1/custom_emojis` - List custom emojis
- [ ] Add emoji support to status rendering

## Phase 5: User Features (Week 9-10)

### 5.1 User Collections
**Priority: MEDIUM** - User endorsements

#### Tasks:
- [ ] `GET /api/v1/endorsements` - View endorsed accounts
- [ ] Implement endorsement storage and logic

### 5.2 Domain Blocks
**Priority: MEDIUM** - User-level domain blocking

#### Tasks:
- [ ] `GET /api/v1/domain_blocks` - View blocked domains
- [ ] `POST /api/v1/domain_blocks` - Block a domain
- [ ] `DELETE /api/v1/domain_blocks` - Unblock a domain

### 5.3 Reports
**Priority: HIGH** - User safety

#### Tasks:
- [ ] `POST /api/v1/reports` - File a report
- [ ] Create report handling system
- [ ] Integrate with moderation system

### 5.4 Markers
**Priority: LOW** - Timeline position

#### Tasks:
- [ ] `GET /api/v1/markers` - Get timeline positions
- [ ] `POST /api/v1/markers` - Save timeline positions
- [ ] Store markers in DynamoDB

### 5.5 Preferences
**Priority: MEDIUM** - User preferences

#### Tasks:
- [ ] Design preference storage (TODO in main.go:560)
- [ ] `GET /api/v1/preferences` - Get preferences (currently hardcoded)
- [ ] `PATCH /api/v1/preferences` - Update preferences

## Phase 6: Media & Import/Export (Week 11-12)

### 6.1 Media Improvements
**Priority: MEDIUM** - Async media upload

#### Tasks:
- [ ] `POST /api/v2/media` - Async media upload
- [ ] Implement async processing queue

### 6.2 Import/Export
**Priority: LOW** - Data portability

#### Tasks:
- [ ] `POST /api/v1/import` - Import data
- [ ] `POST /api/v1/export` - Export data
- [ ] Support various export formats

### 6.3 oEmbed
**Priority: LOW** - Embedding support

#### Tasks:
- [ ] `GET /api/oembed` - oEmbed endpoint
- [ ] Generate embed codes for statuses

## Infrastructure & Technical Debt

### Security Enhancements
- [ ] Encrypt actor private keys with AWS KMS (TODO in actor.go:37, 203)
- [ ] Implement instance trust checking (TODO in crypto.go:285)
- [ ] Add PEM key loading support (TODO in crypto.go:26)

### Data Model Improvements
- [ ] Store actor creation time (TODO in converter_impl.go:43)
- [ ] Track last status time (TODO in converter_impl.go:47)
- [ ] Add support for actor fields (TODO in converter_impl.go:49)
- [ ] Implement proper activity tracking (TODO in users.go:279)
- [ ] Delete associated data on user deletion (TODO in users.go:220, actor.go:319)

### Federation Improvements
- [ ] Implement SQS queue for reliable delivery (TODO in delivery.go:329)
- [ ] Implement trust propagation through network (TODO in trust.go:390)
- [ ] Complete trust graph queries (TODO in service.go:172)
- [ ] Complete moderation event queries (TODO in service.go:176)
- [ ] Query community notes and helpful votes (TODO in service.go:195)
- [ ] Implement vouch import (TODO in service.go:320)

### Configuration
- [ ] Make reputation table name configurable (TODO in vouch.go:287)
- [ ] Get instance URL from config (TODO in calculator.go:29)
- [ ] Make languages configurable (TODO in instance.go:45)
- [ ] Add Link header for pagination (TODO in headers.go:51)

### Search & Discovery
- [ ] Update search service to use DynamoDBAPI interface (TODO in client.go:103)
- [ ] Implement follower tracking for search (TODO in search_helpers.go:62)

## Admin API (Future Phase)

### Admin Endpoints
- [ ] All `/api/v1/admin/*` endpoints for server administration
- [ ] Admin dashboard
- [ ] Moderation tools
- [ ] Server management

## Testing & Documentation

### Testing Requirements
- [ ] Add integration tests for all new endpoints
- [ ] Test OAuth flow end-to-end
- [ ] Test federation with real Mastodon instances
- [ ] Performance testing for trending/discovery features

### Documentation
- [ ] Update API documentation
- [ ] Add OAuth setup guide
- [ ] Document configuration options
- [ ] Create migration guides

## Priority Matrix

### Critical (Blocks Mastodon clients)
1. Conversations routing (handlers exist, just need wiring)
2. OAuth revoke endpoint (minor, but needed for proper token management)
3. Basic account relationships (followers/following lists)

### High (Core functionality)
1. Reports system
2. Status pinning/muting
3. Account follower/following lists

### Medium (Enhanced features)
1. Scheduled statuses
2. Hashtag following
3. Search v2
4. User preferences
5. Domain blocks

### Low (Nice to have)
1. Trends and discovery
2. Announcements
3. Custom emojis
4. Import/export
5. Directory

## Estimated Timeline
- **Week 1**: Wire up conversations, OAuth revoke, and account relationships
- **Weeks 2-3**: Core content features (status operations, scheduled posts)
- **Weeks 4-5**: Discovery and trends
- **Weeks 6-7**: Instance features
- **Weeks 8-9**: User features (reports, domain blocks, preferences)
- **Weeks 10-11**: Media and import/export
- **Ongoing**: Infrastructure improvements and technical debt

Total estimated time: 10-11 weeks for full Mastodon API compatibility

**Note**: Since OAuth is already implemented (except revoke endpoint) and conversations are already coded (just need routing), the timeline has been reduced by approximately 1-2 weeks.

## Key Implementation Highlights

### Search Architecture
Lesser has an advanced search implementation that goes beyond basic Mastodon search:
- **Multi-strategy search**: Combines exact match, prefix, display name, and popularity strategies
- **AI-powered search**: Optional fuzzy search (OpenSearch) and semantic search (AWS Bedrock)
- **Optimized indexes**: Uses DynamoDB GSIs for efficient searching:
  - GSI1: Username prefix search
  - GSI2: Display name search  
  - GSI4: Popularity-based search (by follower count buckets)
- **Search analytics**: Tracks search queries and results for improving relevance
- **Caching layer**: Reduces repeated searches
- **Remote search**: WebFinger integration for federated actor discovery

The main gaps are:
- Status/post search (currently only searches by exact URL)
- Hashtag search (basic implementation exists but incomplete)
- v2 search endpoint format (v1 returns flat results, v2 groups by type) 