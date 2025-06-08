# Practical Stub Resolution Plan

## Current Situation

Lesser is a working ActivityPub implementation with stubs in specific areas. This plan focuses on systematically resolving them to achieve full functionality.

## Stub Impact Matrix

| Component | Stub Count | Impact | Effort | Priority |
|-----------|------------|--------|--------|----------|
| Export Generator | 12 functions | HIGH - Exports empty | 2-3 days | P1 |
| Import/Export Lists | 2 functions | HIGH - Can't see history | 1 day | P1 |
| GraphQL | 58 resolvers | MEDIUM - Alt API broken | 1-2 weeks | P2 |
| Media Processing | 2 functions | MEDIUM - Wrong metadata | 2-3 days | P2 |
| Misc Features | ~10 various | LOW - Minor features | Varies | P3 |

## Phase 1: Quick Wins (3-4 days)

### Fix Export Generator
The export generator has its own stub implementations instead of using the storage layer.

**Solution**: Create a storage client and wire up the functions:

```go
// Add to export-generator/main.go
var storageClient storage.Interface

func initStorage(ctx context.Context) error {
    cfg := &dynamodb.Config{
        TableName: tableName,
        Client:    dynamoClient,
    }
    storageClient = dynamodb.NewStorage(cfg)
    return nil
}

// Then update each stub:
func getFollowers(ctx context.Context, username string) ([]string, error) {
    followers, _, err := storageClient.GetFollowers(ctx, username, 1000, "")
    return followers, err
}
```

### Fix Import/Export Job Lists
Implement the DynamoDB queries:

```go
func (h *Handler) getUserImportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]interface{}, error) {
    input := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
        },
    }
    // ... implement query
}
```

## Phase 2: Core Features (1 week)

### Media Processing
Replace hardcoded values with actual processing:

```go
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
    // Use ffprobe to get real metadata
    cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", 
        "stream=width,height,duration", "-of", "json", "-")
    // ... process real video
}
```

### GraphQL Quick Fix
Replace panics with errors to prevent crashes:

```go
// Batch replace all panics
return nil, fmt.Errorf("GraphQL resolver not yet implemented")
```

## Phase 3: Full Implementation (2-3 weeks)

### GraphQL Implementation
Implement resolvers in priority order:
1. Query resolvers (read operations)
2. Mutation resolvers (write operations)  
3. Subscription resolvers (real-time)

### Remaining Features
- Search enhancements
- Advanced moderation features
- Translation caching
- Other minor stubs

## Implementation Strategy

### 1. Create Feature Branches
```bash
git checkout -b fix/export-generator-stubs
git checkout -b fix/import-export-lists
git checkout -b fix/media-processing
git checkout -b feat/graphql-implementation
```

### 2. Test-Driven Approach
For each stub:
1. Write a test that fails because of the stub
2. Implement the real functionality
3. Verify test passes
4. Manual testing

### 3. Incremental Deployment
- Deploy each fix as completed
- Don't wait for everything to be done
- Get feedback early

## Success Metrics

### Week 1
- [ ] Exports contain real data
- [ ] Import/export history visible
- [ ] GraphQL doesn't crash

### Week 2  
- [ ] Media shows correct duration
- [ ] Basic GraphQL queries work

### Week 3
- [ ] All critical stubs eliminated
- [ ] GraphQL API functional

## Tracking Progress

Use the stub checker script to monitor progress:
```bash
./check_stub_implementations.sh | grep "For now.*return.*empty"
```

Track reduction in stub count over time.

## Key Principles

1. **Fix by connection, not reimplementation** - Most features exist, just need wiring
2. **Test everything** - Each fix needs tests
3. **Deploy incrementally** - Don't wait for perfection
4. **Document as you go** - Update docs when implementing

## Next Steps

1. Start with export generator (biggest impact, moderate effort)
2. Run `./check_stub_implementations.sh` to get baseline
3. Create first feature branch
4. Begin implementation

This is a marathon, not a sprint. Each stub fixed makes Lesser more complete. 