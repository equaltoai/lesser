# Phase 5: User Features Implementation Plan

## Overview
This document outlines the implementation plan for Phase 5 of the Mastodon API compatibility project, focusing on user-specific features including reports, domain blocks, endorsements, markers, and preferences.

## Implementation Status
1. **Reports System** ✅ COMPLETED (HIGH - User Safety)
2. **Domain Blocks** ✅ COMPLETED (MEDIUM - User Control)
3. **User Collections/Endorsements** 🚧 IN PROGRESS (MEDIUM - Social Features)
4. **Preferences** ⏳ TODO (MEDIUM - User Experience)
5. **Markers** ⏳ TODO (LOW - Timeline Positions)

## 5.3 Reports System ✅ COMPLETED

### Overview
The reports system allows users to report accounts, statuses, or other content for moderation review. Lesser's implementation integrates with the existing reactive moderation mesh, automatically creating moderation events for community review.

### API Endpoints
- `POST /api/v1/reports` - File a report ✅ IMPLEMENTED

### Request Parameters
```json
{
  "account_id": "string",       // Required: Account being reported
  "status_ids": ["string"],     // Optional: Specific statuses to report
  "comment": "string",          // Optional: Additional context (max 1000 chars)
  "forward": boolean,           // Optional: Forward to remote instance
  "category": "string",         // Optional: spam, violation, other
  "rule_ids": ["integer"]       // Optional: Rule violations
}
```

### Response Format
```json
{
  "id": "22",
  "action_taken": false,
  "action_taken_at": null,
  "category": "violation",
  "comment": "Spam content",
  "forwarded": true,
  "created_at": "2025-01-20T10:00:00.000Z",
  "status_ids": ["123456"],
  "rule_ids": [3],
  "target_account": {
    "id": "108",
    "username": "spammer",
    "acct": "spammer@example.com"
  }
}
```

### Storage Design (DynamoDB)
```
Reports Table Pattern:
- PK: REPORT#<uuid>
- SK: REPORT
- GSI1PK: USER#<reporter_username>
- GSI1SK: REPORT#<timestamp>
- GSI2PK: REPORTED#<target_account_id>
- GSI2SK: REPORT#<timestamp>
- GSI3PK: STATUS#<resolved|pending>
- GSI3SK: REPORT#<timestamp>

Attributes:
- ID: string (UUID)
- ReporterID: string
- TargetAccountID: string
- StatusIDs: []string
- Comment: string
- Category: string
- RuleIDs: []int
- Forwarded: bool
- Resolved: bool
- ActionTaken: string
- ActionTakenAt: *time.Time
- AssignedTo: string
- CreatedAt: time.Time
- UpdatedAt: time.Time
- ModerationEventID: string (link to moderation system)
```

### Integration with Moderation System
1. When a report is created, automatically create a moderation event
2. Link the report to the moderation event for tracking
3. Update report status based on moderation consensus
4. Track reporter reliability for future trust scoring

### Implementation Files
- `pkg/storage/storage.go` - Add report interfaces
- `pkg/storage/dynamodb/reports.go` - DynamoDB implementation
- `cmd/api/handlers/reports.go` - API handlers
- `cmd/api/models/mastodon.go` - Report data models

## 5.2 Domain Blocks ✅ COMPLETED

### Overview
Allow users to block entire domains at the user level, hiding all content from those domains. Implemented with efficient DynamoDB storage patterns and proper validation.

### API Endpoints
- `GET /api/v1/domain_blocks` - List blocked domains (paginated) ✅ IMPLEMENTED
- `POST /api/v1/domain_blocks` - Block a domain ✅ IMPLEMENTED
- `DELETE /api/v1/domain_blocks` - Unblock a domain ✅ IMPLEMENTED

### Storage Design (DynamoDB)
```
Domain Blocks Pattern:
- PK: USER#<username>
- SK: DOMAIN_BLOCK#<domain>

Attributes:
- Domain: string
- CreatedAt: time.Time
```

### Implementation Considerations
1. Apply domain blocks when fetching timelines
2. Filter out statuses from blocked domains
3. Hide accounts from blocked domains in search results
4. Prevent following accounts from blocked domains

### Implementation Files
- `pkg/storage/dynamodb/domain_blocks.go` - Storage implementation
- `cmd/api/handlers/domain_blocks.go` - API handlers
- Update timeline handlers to respect domain blocks

## 5.1 User Collections/Endorsements (MEDIUM PRIORITY)

### Overview
Allow users to endorse (feature) other accounts on their profile.

### API Endpoints
- `GET /api/v1/endorsements` - List endorsed accounts
- `POST /api/v1/accounts/:id/pin` - Add endorsement (already implemented for pins)
- `POST /api/v1/accounts/:id/unpin` - Remove endorsement (already implemented)

### Storage Design
Reuse existing pin implementation:
```
Pattern:
- PK: USER#<username>
- SK: PINNED_ACCOUNT#<account_id>
```

### Implementation Notes
- Maximum of 4 endorsed accounts per user
- Return endorsed accounts in profile API responses
- Add `endorsed` field to relationship responses

## 5.5 Preferences (MEDIUM PRIORITY)

### Overview
User preferences for various Mastodon client settings.

### API Endpoints
- `GET /api/v1/preferences` - Get current preferences
- `PATCH /api/v1/preferences` - Update preferences

### Preference Structure
```json
{
  "posting:default:visibility": "public|unlisted|private|direct",
  "posting:default:sensitive": false,
  "posting:default:language": "en",
  "reading:expand:media": "default|show_all|hide_all",
  "reading:expand:spoilers": false,
  "reading:autoplay:gifs": true
}
```

### Storage Design (DynamoDB)
```
Pattern:
- PK: USER#<username>
- SK: PREFERENCES

Attributes:
- Preferences: map[string]interface{}
- UpdatedAt: time.Time
```

### Implementation Files
- `pkg/storage/dynamodb/user_preferences.go` - Extend existing preferences
- `cmd/api/handlers/preferences.go` - New preference handlers

## 5.4 Markers (LOW PRIORITY)

### Overview
Save and restore timeline reading positions across clients.

### API Endpoints
- `GET /api/v1/markers` - Get saved positions
- `POST /api/v1/markers` - Update positions

### Marker Types
- `home` - Home timeline position
- `notifications` - Notifications position

### Request Format
```json
{
  "home": {
    "last_read_id": "103194548672408537"
  },
  "notifications": {
    "last_read_id": "35098814"
  }
}
```

### Storage Design (DynamoDB)
```
Pattern:
- PK: USER#<username>
- SK: MARKER#<timeline_type>

Attributes:
- LastReadID: string
- UpdatedAt: time.Time
- Version: int (for optimistic locking)
```

### Implementation Files
- `pkg/storage/dynamodb/markers.go` - Storage implementation
- `cmd/api/handlers/markers.go` - API handlers

## Common Implementation Tasks

### 1. Update Storage Interface
```go
// In pkg/storage/storage.go
type Storage interface {
    // ... existing methods ...
    
    // Reports
    CreateReport(ctx context.Context, report *Report) error
    GetReport(ctx context.Context, id string) (*Report, error)
    GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*Report, string, error)
    UpdateReportStatus(ctx context.Context, id string, resolved bool, actionTaken string) error
    
    // Domain Blocks
    AddDomainBlock(ctx context.Context, username, domain string) error
    RemoveDomainBlock(ctx context.Context, username, domain string) error
    GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
    IsBlockedDomain(ctx context.Context, username, domain string) (bool, error)
    
    // Markers
    SaveMarker(ctx context.Context, username, timeline string, lastReadID string) error
    GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*Marker, error)
    
    // Preferences (extend existing)
    UpdatePreferences(ctx context.Context, username string, prefs map[string]interface{}) error
}
```

### 2. Update Data Models
```go
// In cmd/api/models/mastodon.go

type Report struct {
    ID              string           `json:"id"`
    ActionTaken     bool            `json:"action_taken"`
    ActionTakenAt   *string         `json:"action_taken_at"`
    Category        string          `json:"category"`
    Comment         string          `json:"comment"`
    Forwarded       bool            `json:"forwarded"`
    CreatedAt       string          `json:"created_at"`
    StatusIDs       []string        `json:"status_ids"`
    RuleIDs         []int           `json:"rule_ids"`
    TargetAccount   *Account        `json:"target_account"`
}

type Marker struct {
    LastReadID string `json:"last_read_id"`
    UpdatedAt  string `json:"updated_at"`
    Version    int    `json:"version"`
}

type Preferences struct {
    PostingDefaultVisibility  string `json:"posting:default:visibility"`
    PostingDefaultSensitive   bool   `json:"posting:default:sensitive"`
    PostingDefaultLanguage    string `json:"posting:default:language"`
    ReadingExpandMedia        string `json:"reading:expand:media"`
    ReadingExpandSpoilers     bool   `json:"reading:expand:spoilers"`
    ReadingAutoplayGifs       bool   `json:"reading:autoplay:gifs"`
}
```

### 3. Wire Routes in main.go
```go
// Reports
authGroup.POST("/reports", h.CreateReport)

// Domain blocks
authGroup.GET("/domain_blocks", h.GetDomainBlocks)
authGroup.POST("/domain_blocks", h.CreateDomainBlock)
authGroup.DELETE("/domain_blocks", h.DeleteDomainBlock)

// Endorsements (reuse existing pin endpoints)
authGroup.GET("/endorsements", h.GetEndorsements)

// Markers
authGroup.GET("/markers", h.GetMarkers)
authGroup.POST("/markers", h.UpdateMarkers)

// Preferences
authGroup.GET("/preferences", h.GetPreferences)
authGroup.PATCH("/preferences", h.UpdatePreferences)
```

## Testing Plan

### 1. Unit Tests
- Test each storage method with various inputs
- Test handler validation and error cases
- Test integration with existing systems

### 2. Integration Tests
- Test report creation and moderation flow
- Test domain blocking affects timeline filtering
- Test marker synchronization across sessions
- Test preference persistence

### 3. Manual Testing
- Test with Mastodon web client
- Test with mobile apps (Tusky, Ivory, etc.)
- Verify cross-client marker synchronization

## Timeline
- Week 1: Reports system + Domain blocks
- Week 2: Endorsements + Preferences + Markers

## Completion Summary

### ✅ Completed (January 2025)
1. **Reports System** 
   - Full DynamoDB implementation with multiple GSI patterns
   - Integration with reactive moderation mesh
   - Automatic moderation event creation
   - Support for reporting accounts and statuses
   - Category and rule violation tracking
   
2. **Domain Blocks**
   - Complete API implementation for all 3 endpoints
   - Efficient DynamoDB storage with USER#username pattern
   - Domain validation and error handling
   - Pagination support for listing blocked domains

### 🚧 Next Steps
1. **User Collections/Endorsements** - Reuse existing pin implementation
2. **Preferences** - Extend existing preference storage
3. **Markers** - Implement timeline position tracking
4. **Integration Tasks**:
   - Add domain block filtering to timeline queries
   - Update search to exclude blocked domains
   - Ensure reports show in moderation queue 