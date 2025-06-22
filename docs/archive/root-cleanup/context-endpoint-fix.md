# Context Endpoint Fix - Descendants Issue

## Issue
The `/api/v1/statuses/:id/context` endpoint was returning empty descendants arrays even when statuses had replies. The ancestors were working correctly.

## Root Causes
1. The GetReplies method was already implemented but not being used
2. The context handler had the descendants code commented out with a TODO
3. Reply counts were not being tracked when replies were created
4. The GetReplies method needed to handle full URL object IDs (similar to previous fixes)

## Fixes Applied

### 1. Updated GetReplies Implementation (replies.go)
- Added proper imports (cost, strings)
- Updated to handle both full URLs and bare IDs for object lookups
- Fixed all related methods (CountReplies, IncrementReplyCount, updateReplyCount)

### 2. Added Methods to Storage Interface (interface.go)
```go
// Reply operations
GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]interface{}, string, error)
CountReplies(ctx context.Context, objectID string) (int, error)
IncrementReplyCount(ctx context.Context, objectID string) error
```

### 3. Enabled Descendants in Context Handler (statuses.go)
- Uncommented and updated the descendants code
- Now fetches up to 100 replies using GetReplies
- Properly converts replies to status format with actor information

### 4. Added Reply Count Tracking (statuses.go)
- When creating a status with InReplyToID, now increments the parent's reply count
- Uses IncrementReplyCount to update the STATS record

## How It Works
1. When a reply is created, the parent status's reply count is incremented
2. The context endpoint calls GetReplies to fetch all objects where inReplyTo matches the status ID
3. GetReplies uses a table scan with filter (inefficient but functional until a GSI is added)
4. Reply counts are cached in STATS records to avoid expensive scans

## Performance Note
The current implementation uses table scans to find replies, which is inefficient. For production, a Global Secondary Index (GSI) should be added:
- GSI2PK: REPLY#<parent-object-id>
- GSI2SK: <timestamp>

This would allow efficient querying of replies by parent ID.

## Testing
After deployment:
1. Create a status
2. Create replies to that status
3. Call `/api/v1/statuses/:id/context` on the parent
4. Verify descendants array contains the replies
5. Verify replies_count is accurate (once that's also implemented in converters)

## Files Changed
- `/pkg/storage/dynamodb/replies.go` - Fixed GetReplies implementation
- `/pkg/storage/interface.go` - Added reply methods to interface
- `/cmd/api/handlers/statuses.go` - Enabled descendants, added reply count tracking
- `/pkg/storage/dynamodb/client.go` - Added getDomainURL() method