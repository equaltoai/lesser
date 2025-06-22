# Unified Boost System Implementation Plan

## Overview
Lesser already has the infrastructure for quote posts (`quotes.go`) but it's not integrated with the Mastodon API. We'll unify announces (boosts) and quotes under a single interaction model.

## Current State Analysis

### What Exists:
1. **Reblog endpoint** (`HandleReblog`) - Creates pure Announce activities
2. **Quote infrastructure** (`quotes.go`) - Tracks quote relationships but not exposed
3. **Separate counting** - Reblogs and quotes tracked independently
4. **GSI4** - Used for announce indexing

### What's Missing:
1. API endpoint accepting commentary with reblogs
2. Unified counting for reblogs + quotes
3. Quote boost creation through Mastodon API

## Implementation Steps

### Phase 1: Backend API Enhancement

#### 1.1 Update Reblog Request Model
```go
// In cmd/api/models/requests.go
type ReblogRequest struct {
    Comment    *string `json:"comment,omitempty"`    // Optional commentary
    Visibility string  `json:"visibility,omitempty"` // For quote boost visibility
}
```

#### 1.2 Modify HandleReblog Endpoint
The endpoint should:
- Accept optional comment field
- If comment is empty → Create Announce (current behavior)
- If comment exists → Create new status with quote relationship

#### 1.3 Update Status Response
```go
// Add to Status model
type Status struct {
    // ... existing fields ...
    IsQuoteBoost     bool    `json:"is_quote_boost,omitempty"`
    QuotedStatus     *Status `json:"quoted_status,omitempty"`
    QuotedStatusID   *string `json:"quoted_status_id,omitempty"`
}
```

### Phase 2: Storage Layer Updates

#### 2.1 Unified Counting
- Both announces and quotes should increment `reblogs_count`
- Update `CreateAnnounce` to also increment unified count
- Update quote creation to increment reblog count

#### 2.2 Add Quote Relationship on Status Creation
When creating a quote boost:
1. Create new status with the comment
2. Create QuoteRelationship record
3. Increment parent's reblog count
4. Fan out to timelines

### Phase 3: Query Updates

#### 3.1 Timeline Inclusion
- Quote boosts should appear in home timelines
- Should be included when fetching "reblogs" of a status
- Support filtering by boost type if needed

#### 3.2 Context Endpoint
- Include quote boosts in the context tree
- Show relationship to original status

### Phase 4: Federation

#### 4.1 Outgoing Federation
- Pure boosts → `Announce` activity (unchanged)
- Quote boosts → `Create` activity with:
  ```json
  {
    "type": "Create",
    "object": {
      "type": "Note",
      "content": "Commentary text<br><br>QT: https://original.instance/@user/status",
      "quoteUrl": "https://original.instance/@user/status",
      "_misskey_quote": "https://original.instance/@user/status"
    }
  }
  ```

#### 4.2 Incoming Federation
- Detect Create activities with status URLs as potential quotes
- Look for `quoteUrl` or `_misskey_quote` fields
- Create appropriate relationships

## Backend Implementation Details

### Modified HandleReblog Flow:

```go
func (h *Handler) HandleReblog(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    // ... existing validation ...
    
    // Parse request body if present
    var req ReblogRequest
    if request.Body != "" {
        if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
            // Treat as traditional boost if can't parse
            req = ReblogRequest{}
        }
    }
    
    // Check if this is a quote boost
    if req.Comment != nil && *req.Comment != "" {
        // Create a new status with quote relationship
        return h.createQuoteBoost(ctx, statusID, *req.Comment, req.Visibility, claims, actor)
    }
    
    // Traditional boost flow (existing code)
    // ...
}

func (h *Handler) createQuoteBoost(ctx context.Context, quotedStatusID, comment, visibility string, claims *auth.Claims, actor *activitypub.Actor) (*events.APIGatewayV2HTTPResponse, error) {
    // 1. Create new status with the comment
    // 2. Add quote relationship
    // 3. Increment reblog count on quoted status
    // 4. Return the new status with quoted_status populated
}
```

### Storage Interface Additions:

```go
// Add to storage interface
type Storage interface {
    // ... existing methods ...
    
    // Quote operations (already exist but need to be exposed)
    CreateQuoteRelationship(ctx context.Context, quote *QuoteRelationship) error
    GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*QuoteRelationship, string, error)
    IsQuoted(ctx context.Context, actorID, noteID string) (bool, error)
}
```

## Frontend (Greater) Requirements

### 1. Boost Button Behavior
```javascript
// When boost button clicked
async function handleBoostClick(status) {
    // Open composer with special boost mode
    openComposer({
        mode: 'boost',
        quotedStatus: status,
        onSubmit: async (text, visibility) => {
            if (!text || text.trim() === '') {
                // Pure boost
                await api.reblog(status.id);
            } else {
                // Quote boost
                await api.reblogWithComment(status.id, text, visibility);
            }
        }
    });
}
```

### 2. API Client Updates
```javascript
// Add to Greater's API client
async reblogWithComment(statusId, comment, visibility = 'public') {
    return this.post(`/api/v1/statuses/${statusId}/reblog`, {
        comment,
        visibility
    });
}
```

### 3. Timeline Display
- Show quote boosts with embedded quoted status
- Display "boosted with comment" indicator
- Maintain visual distinction from regular posts

## Testing Plan

1. **Unit Tests**
   - Test pure boost (no comment)
   - Test quote boost (with comment)
   - Test unified counting

2. **Integration Tests**
   - Federation with Mastodon (pure boosts)
   - Federation with Misskey/Pleroma (quote posts)
   - Timeline inclusion

3. **Edge Cases**
   - Quote boost of a quote boost
   - Boost cycles (A quotes B quotes A)
   - Visibility inheritance

## Migration Considerations

1. **Existing Boosts**: Keep as-is (pure announces)
2. **Future Boosts**: Use unified system
3. **Count Migration**: Optionally sum existing reblog + quote counts

## API Examples

### Pure Boost (Mastodon-compatible)
```bash
POST /api/v1/statuses/123/reblog
# Empty body or no comment field
```

### Quote Boost (Enhanced)
```bash
POST /api/v1/statuses/123/reblog
Content-Type: application/json

{
  "comment": "This is exactly what we need to discuss!",
  "visibility": "public"
}
```

### Response for Quote Boost
```json
{
  "id": "456",
  "created_at": "2024-01-20T12:00:00Z",
  "content": "This is exactly what we need to discuss!",
  "is_quote_boost": true,
  "quoted_status": {
    "id": "123",
    "content": "Original status content",
    // ... full status object
  },
  "reblogs_count": 0,
  "reblogged": false,
  // ... other status fields
}
```

## Next Steps

1. Implement backend changes in Lesser
2. Create Greater UI mockups for boost flow
3. Test federation compatibility
4. Deploy with feature flag
5. Monitor usage patterns

This implementation maintains full Mastodon compatibility while adding valuable functionality for users who want to add context when sharing.