# Timeline Empty Status Fix

## Issue
The home timeline was returning statuses with empty fields (id, content, created_at, uri, url) even though the timeline entry existed in DynamoDB with the correct data.

## Root Cause Analysis

### Initial Problem
The timeline entry stores the PostID as a full URL: `"https://lesser.host/objects/1750512085-0UQB3duo"`

The initial fix attempted to extract just the ID portion (`"1750512085-0UQB3duo"`) before calling GetObject.

### Real Root Cause
After deeper investigation, the actual issue was that objects in DynamoDB are stored with their FULL URL as the key:
```json
"PK": {
  "S": "OBJECT#https://lesser.host/objects/1750512085-0UQB3duo"
}
```

But the GetObject function was constructing the key with just the ID:
```go
"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", id)}
```

This mismatch meant GetObject was looking for `"OBJECT#1750512085-0UQB3duo"` which doesn't exist.

## Fix
Updated GetObject to handle both full URLs and bare IDs by constructing the full URL when needed:

```go
// If the ID doesn't start with http, construct the full URL
objectKey := id
if !strings.HasPrefix(id, "http") {
    objectKey = fmt.Sprintf("%s/objects/%s", s.getDomainURL(), id)
}
```

## Files Changed
- `/pkg/storage/dynamodb/objects.go`: Updated GetObject to construct full URL for object keys
- `/pkg/storage/dynamodb/client.go`: Added getDomainURL() helper method
- `/cmd/api/handlers/timelines.go`: Added debug logging (can be removed after verification)

## Testing
After deploying this fix, the timeline should return complete status objects with all fields populated:
- id
- content
- created_at
- uri
- url
- account information
- interaction counts

The fix maintains compatibility with both storage patterns:
1. Objects stored with full URLs as keys (current pattern)
2. Lookups using just the ID portion (as done by the timeline handler)