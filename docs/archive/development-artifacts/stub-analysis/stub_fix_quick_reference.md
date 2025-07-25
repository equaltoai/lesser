# Stub Fix Quick Reference Guide

## Identifying Stubs

### Red Flags in Code
```go
// ❌ BAD - These indicate stubs:
return []map[string]any{}, nil  // For now
return []string{}, nil  // TODO: implement
panic("not implemented")
// This would normally...
// For now, return empty...
```

### Quick Detection Commands
```bash
# Find all stubs in current directory
grep -r "For now.*return.*empty" .
grep -r "panic.*not implemented" .
grep -r "would normally" .

# Run full analysis
./check_stub_implementations.sh
```

## Fix Patterns

### Pattern 1: Empty List Returns
**Before (Stub):**
```go
func getUserImportJobs(_ context.Context, _ string, _ ...string) ([]map[string]any, error) {
    // For now, return empty to avoid errors
    return []map[string]any{}, nil
}
```

**After (Fixed):**
```go
func getUserImportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
    input := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
        },
    }
    
    result, err := h.store.GetClient().Query(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    
    // Convert and return actual data
    return convertItems(result.Items), nil
}
```

### Pattern 2: Panic Not Implemented
**Before (Stub):**
```go
func (r *actorResolver) Username(ctx context.Context, obj *activitypub.Actor) (string, error) {
    panic(fmt.Errorf("not implemented: Username - username"))
}
```

**After (Quick Fix):**
```go
func (r *actorResolver) Username(ctx context.Context, obj *activitypub.Actor) (string, error) {
    // TODO: Implement GraphQL resolver
    return "", fmt.Errorf("GraphQL API is under development")
}
```

**After (Full Fix):**
```go
func (r *actorResolver) Username(ctx context.Context, obj *activitypub.Actor) (string, error) {
    if obj == nil {
        return "", fmt.Errorf("actor is nil")
    }
    return obj.PreferredUsername, nil
}
```

### Pattern 3: Hardcoded Fake Data
**Before (Stub):**
```go
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    // For now, return some placeholder data
    result.Width = 1920
    result.Height = 1080
    result.Duration = 30000
    return result, nil
}
```

**After (Fixed):**
```go
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    // Use ffprobe to get actual video metadata
    probeResult, err := probeVideoFile(data)
    if err != nil {
        return ProcessingResult{}, fmt.Errorf("failed to probe video: %w", err)
    }
    
    return ProcessingResult{
        Width:    probeResult.Width,
        Height:   probeResult.Height,
        Duration: probeResult.DurationMs,
    }, nil
}
```

## Testing Your Fix

### 1. Write Test First
```go
func TestGetUserImportJobs_ReturnsActualData(t *testing.T) {
    // Create test data
    user := createTestUser(t)
    job1 := createImportJob(t, user, "followers")
    job2 := createImportJob(t, user, "following")
    
    // Call function
    jobs, err := getUserImportJobs(ctx, user.Username)
    
    // Verify real data returned
    assert.NoError(t, err)
    assert.Len(t, jobs, 2)
    assert.Contains(t, jobs[0]["ImportID"], job1.ID)
}
```

### 2. Manual Verification
```bash
# Test the endpoint manually
curl -H "Authorization: Bearer $TOKEN" \
  https://api.example.com/api/v1/imports

# Should return actual data, not empty array
```

### 3. Integration Test
```python
def test_import_export_list_not_empty():
    # Create import
    import_job = api.create_import(user, data)
    
    # List imports - should not be empty
    imports = api.list_imports(user)
    assert len(imports) > 0
    assert imports[0]['id'] == import_job.id
```

## Common DynamoDB Patterns

### Query with GSI
```go
input := &dynamodb.QueryInput{
    TableName: aws.String(tableName),
    IndexName: aws.String("GSI1"),
    KeyConditionExpression: aws.String("GSI1PK = :pk"),
    ExpressionAttributeValues: map[string]types.AttributeValue{
        ":pk": &types.AttributeValueMemberS{Value: key},
    },
}
```

### Query with Filter
```go
input.FilterExpression = aws.String("#status = :status")
input.ExpressionAttributeNames = map[string]string{
    "#status": "Status",
}
input.ExpressionAttributeValues[":status"] = &types.AttributeValueMemberS{
    Value: "completed",
}
```

### Pagination
```go
var allItems []map[string]any
var lastKey map[string]types.AttributeValue

for {
    if lastKey != nil {
        input.ExclusiveStartKey = lastKey
    }
    
    result, err := client.Query(ctx, input)
    if err != nil {
        return nil, err
    }
    
    allItems = append(allItems, convertItems(result.Items)...)
    
    if result.LastEvaluatedKey == nil {
        break
    }
    lastKey = result.LastEvaluatedKey
}
```

## Checklist Before Marking Complete

- [ ] No "for now" comments in code
- [ ] No panic("not implemented")
- [ ] Function returns real data from storage
- [ ] Unit tests pass with real data
- [ ] Integration test verifies end-to-end
- [ ] Manual test confirms functionality
- [ ] Error cases handled properly
- [ ] Logging added for debugging
- [ ] Documentation updated

## Resources

- [DynamoDB Query Examples](docs/dynamodb-patterns.md)
- [Integration Test Guide](tests/integration/README.md)
- [Code Review Checklist](.github/review-checklist.md)

## Getting Help

If you're stuck on a stub fix:
1. Check if similar code exists elsewhere
2. Look for the storage interface definition
3. Ask in #stub-fixes Slack channel
4. Pair with someone who's fixed similar stubs

Remember: **A working feature with limitations is better than a stub that pretends to work!** 