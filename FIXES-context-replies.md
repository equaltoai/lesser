# Fix for Lesser API Context Endpoint Not Returning Replies

## Problem Summary
The `/api/v1/statuses/:id/context` endpoint returns empty descendants arrays even when statuses have replies. Additionally, `replies_count` shows 0 for all statuses.

## Root Causes
1. **No GetReplies implementation**: The context handler has a TODO comment where replies should be fetched
2. **No GSI for inReplyTo queries**: No efficient way to find all objects that reply to a given status ID
3. **RepliesCount always 0**: The converter hardcodes this value to 0

## Proposed Solution

### 1. Add GSI for Reply Queries

First, we need to add a new Global Secondary Index (GSI) to efficiently query replies:

```go
// In pkg/storage/dynamodb/objects.go

// Add new GSI fields to ObjectRecord struct
type ObjectRecord struct {
    PK        string    `dynamodbav:"PK"`
    SK        string    `dynamodbav:"SK"`
    GSI1PK    string    `dynamodbav:"GSI1PK,omitempty"` // Actor's objects timeline
    GSI1SK    string    `dynamodbav:"GSI1SK,omitempty"` // Published timestamp
    GSI2PK    string    `dynamodbav:"GSI2PK,omitempty"` // For replies: REPLY#<parent-id>
    GSI2SK    string    `dynamodbav:"GSI2SK,omitempty"` // Published timestamp for sorting
    Object    *Object   `dynamodbav:"Object"`
    CreatedAt time.Time `dynamodbav:"CreatedAt"`
    UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
}
```

### 2. Update CreateObject to Index Replies

```go
// In CreateObject method, add GSI2 fields for replies
if obj.InReplyTo != nil && *obj.InReplyTo != "" {
    record.GSI2PK = fmt.Sprintf("REPLY#%s", *obj.InReplyTo)
    record.GSI2SK = obj.Published.Format(time.RFC3339)
}
```

### 3. Implement GetReplies Method

Add to `pkg/storage/interface.go`:
```go
// GetReplies retrieves all replies to a given object
GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]interface{}, string, error)
// CountReplies counts the number of replies to an object
CountReplies(ctx context.Context, objectID string) (int, error)
```

Implementation in `pkg/storage/dynamodb/objects.go`:
```go
func (s *dynamoDBStorage) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]interface{}, string, error) {
    log := common.WithContext(ctx)
    
    if limit <= 0 || limit > 100 {
        limit = 20
    }
    
    input := &dynamodb.QueryInput{
        TableName:              s.getTableName(),
        IndexName:              aws.String("GSI2"),
        KeyConditionExpression: aws.String("GSI2PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLY#%s", objectID)},
        },
        Limit:            aws.Int32(int32(limit)),
        ScanIndexForward: aws.Bool(true), // Oldest first
    }
    
    if cursor != "" {
        input.ExclusiveStartKey = map[string]types.AttributeValue{
            "GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLY#%s", objectID)},
            "GSI2SK": &types.AttributeValueMemberS{Value: cursor},
        }
    }
    
    result, err := s.client.Query(ctx, input)
    if err != nil {
        log.Error("failed to query replies",
            zap.String("object_id", objectID),
            zap.Error(err))
        return nil, "", fmt.Errorf("failed to query replies: %w", err)
    }
    
    replies := make([]interface{}, 0, len(result.Items))
    for _, item := range result.Items {
        var record ObjectRecord
        if err := s.UnmarshalItem(item, &record); err != nil {
            log.Warn("failed to unmarshal reply",
                zap.Error(err))
            continue
        }
        
        // Convert to appropriate format
        replies = append(replies, record.Object)
    }
    
    var nextCursor string
    if result.LastEvaluatedKey != nil {
        if sk, ok := result.LastEvaluatedKey["GSI2SK"].(*types.AttributeValueMemberS); ok {
            nextCursor = sk.Value
        }
    }
    
    return replies, nextCursor, nil
}

func (s *dynamoDBStorage) CountReplies(ctx context.Context, objectID string) (int, error) {
    log := common.WithContext(ctx)
    
    input := &dynamodb.QueryInput{
        TableName:              s.getTableName(),
        IndexName:              aws.String("GSI2"),
        KeyConditionExpression: aws.String("GSI2PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("REPLY#%s", objectID)},
        },
        Select: aws.String("COUNT"),
    }
    
    result, err := s.client.Query(ctx, input)
    if err != nil {
        log.Error("failed to count replies",
            zap.String("object_id", objectID),
            zap.Error(err))
        return 0, fmt.Errorf("failed to count replies: %w", err)
    }
    
    return int(result.Count), nil
}
```

### 4. Update HandleGetStatusContext

Replace the TODO section in `cmd/api/handlers/statuses.go`:
```go
// Get descendants (replies to this status)
descendants := []models.Status{}
replies, _, err := h.store.GetReplies(ctx, objectID, 20, "")
if err == nil {
    for _, reply := range replies {
        // Get actor for reply
        var replyActor *activitypub.Actor
        var attributedTo string
        switch o := reply.(type) {
        case *activitypub.Note:
            attributedTo = o.AttributedTo
        case *Object:
            attributedTo = o.AttributedTo
        case map[string]interface{}:
            if attr, ok := o["attributedTo"].(string); ok {
                attributedTo = attr
            }
        }
        
        if attributedTo != "" {
            username := h.converter.ExtractUsernameFromActorID(attributedTo)
            if username != "" {
                replyActor, _ = h.store.GetActor(ctx, username)
            }
        }
        
        status := h.converter.ObjectToStatus(reply, replyActor)
        descendants = append(descendants, status)
        
        // Optionally get nested replies (careful about depth)
        // This would require recursive calls with depth limiting
    }
} else {
    h.logger.Warn("failed to get replies",
        zap.String("object_id", objectID),
        zap.Error(err))
}
```

### 5. Update Status Creation to Track Reply Count

In HandleCreateStatus when creating a reply:
```go
// Handle reply
if req.InReplyToID != "" {
    note.InReplyTo = req.InReplyToID
    
    // Update parent status reply count
    if err := h.store.IncrementReplyCount(ctx, req.InReplyToID); err != nil {
        h.logger.Warn("failed to increment reply count",
            zap.String("parent_status_id", req.InReplyToID),
            zap.Error(err))
    }
    
    // Record reply engagement for trending
    if err := h.store.RecordStatusEngagement(ctx, req.InReplyToID, "reply", actor.ID); err != nil {
        h.logger.Warn("failed to record reply engagement",
            zap.String("parent_status_id", req.InReplyToID),
            zap.Error(err))
    }
}
```

### 6. Add Reply Count Tracking

Create a new method to track reply counts:
```go
// In pkg/storage/dynamodb/objects.go
func (s *dynamoDBStorage) IncrementReplyCount(ctx context.Context, objectID string) error {
    input := &dynamodb.UpdateItemInput{
        TableName: s.getTableName(),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", objectID)},
            "SK": &types.AttributeValueMemberS{Value: "STATS"},
        },
        UpdateExpression: aws.String("ADD reply_count :inc"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":inc": &types.AttributeValueMemberN{Value: "1"},
        },
    }
    
    _, err := s.client.UpdateItem(ctx, input)
    return err
}
```

### 7. Update Converter to Include Reply Count

In the ObjectToStatus converter, fetch the actual reply count:
```go
// In HandleGetStatus and other places where status is returned
replyCount, _ := h.store.CountReplies(ctx, objectID)
status.RepliesCount = replyCount
```

## Infrastructure Changes Required

1. **Add GSI2 to DynamoDB table** with:
   - Partition Key: GSI2PK
   - Sort Key: GSI2SK
   - Projection: ALL

2. **Migration for existing data**:
   - Scan all objects with inReplyTo field
   - Update them with appropriate GSI2PK and GSI2SK values

## Testing Recommendations

1. Create a status
2. Create multiple replies to that status
3. Call GET /api/v1/statuses/:id/context
4. Verify descendants array contains the replies
5. Check that replies_count is accurate

## Alternative Approach (Without GSI)

If adding a GSI is not immediately possible, a temporary solution could be:
- Scan all objects and filter by inReplyTo field (inefficient but functional)
- Cache reply counts in a separate item type
- Use the conversation tracking that already exists

## Cost Considerations

- New GSI will increase storage costs slightly
- Query costs for fetching replies will be minimal (single query operation)
- Consider implementing pagination for statuses with many replies