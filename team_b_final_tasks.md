# Team B Final Tasks - Storage Optimization Completion

**Mission**: Complete the final 3% of functionality to reach 100% implementation

**Current Status**: 97% complete - Only storage optimizations and caching remain
**Estimated Time**: 1-2 days  
**Build Status**: ✅ Compiling and functional

## Final Implementation Tasks

### Task 1: Fix Hardcoded Engagement Values
**File**: `pkg/storage/dynamodb/client.go`
**Current Issue**: Returns hardcoded `1` for all engagement metrics
**Lines**: 131, 173, 215

#### Current Problematic Code:
```go
// Line 131 - Hashtag trends
UsageCount: 1, // Default value - could be enhanced with actual counts

// Line 173 - Link trends  
ShareCount: 1, // Default value - could be enhanced with actual counts

// Line 215 - Status trends
Engagements: 1, // Default value - could be enhanced with actual counts
```

#### Implementation Required:
- [ ] **GetRecentHashtags**: Calculate real usage counts from hashtag timeline
- [ ] **GetRecentLinks**: Calculate real share counts from link interactions
- [ ] **GetRecentStatusesWithEngagement**: Calculate real engagement scores
- [ ] Add aggregation queries to count actual usage/shares/engagement
- [ ] Implement proper time-windowed calculations (last 24h, 7d, 30d)

#### Expected Outcome:
```go
// Replace hardcoded values with real calculations:
hashtag := &storage.TrendingHashtag{
    Name:       tag,
    UsageCount: calculateHashtagUsage(ctx, tag, timeWindow), // Real count
}

link := &storage.TrendingLink{
    URL:        url,
    ShareCount: calculateLinkShares(ctx, url, timeWindow), // Real count  
}

status := &storage.TrendingStatus{
    ID:          statusID,
    Engagements: calculateEngagementScore(ctx, statusID), // Real score
}
```

### Task 2: Add Missing Storage Interface Methods
**File**: `cmd/status-indexer/main.go` + `pkg/storage/interface.go`
**Current Issue**: TODO comments for missing methods
**Lines**: 418, 422

#### Missing Methods to Implement:

##### 2A: Add to Storage Interface (`pkg/storage/interface.go`):
```go
// Add these methods to the Storage interface:
StoreEngagementMetrics(ctx context.Context, metrics *EngagementMetrics) error
IndexByEngagement(ctx context.Context, statusID string, bucket string) error
GetEngagementMetrics(ctx context.Context, statusID string) (*EngagementMetrics, error)
```

##### 2B: Implement in DynamoDB Client (`pkg/storage/dynamodb/client.go`):
- [ ] **StoreEngagementMetrics**: Store engagement metrics in DynamoDB with proper indexing
- [ ] **IndexByEngagement**: Create GSI entries for engagement-based discovery
- [ ] **GetEngagementMetrics**: Retrieve stored engagement metrics by status ID

##### 2C: Update Status Indexer (`cmd/status-indexer/main.go`):
- [ ] Remove TODO comments (lines 418, 422)
- [ ] Call actual storage methods instead of placeholder variables
- [ ] Implement proper engagement bucket indexing

#### Expected Implementation:
```go
// Replace these TODOs:
// TODO: Add StoreEngagementMetrics method to storage interface
// TODO: Add IndexByEngagement method to storage interface

// With actual method calls:
if err := store.StoreEngagementMetrics(ctx, engagement); err != nil {
    logger.Error("failed to store engagement metrics", zap.Error(err))
    return err
}

if err := store.IndexByEngagement(ctx, statusID, engagementBucket); err != nil {
    logger.Error("failed to index by engagement", zap.Error(err))
    return err
}
```

### Task 3: Complete Translation Result Caching
**File**: `pkg/translation/aws_translate.go`
**Current Issue**: Returns "cache not implemented"
**Lines**: 211, 226

#### Implementation Required:
- [ ] **getCachedTranslation**: Query DynamoDB for cached translation results
- [ ] **cacheTranslationResult**: Store translation results with TTL (30 days)
- [ ] Add proper cache key generation (source text hash + language pair)
- [ ] Implement cache expiration and cleanup

#### Expected Outcome:
```go
// Replace these:
return nil // TODO: Implement DynamoDB caching

// With actual caching:
func (t *AwsTranslateService) getCachedTranslation(ctx context.Context, sourceText, sourceLang, targetLang string) (*TranslationResult, error) {
    cacheKey := generateCacheKey(sourceText, sourceLang, targetLang)
    
    input := &dynamodb.GetItemInput{
        TableName: aws.String(t.tableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TRANSLATION#%s", cacheKey)},
            "SK": &types.AttributeValueMemberS{Value: "RESULT"},
        },
    }
    
    result, err := t.client.GetItem(ctx, input)
    if err != nil || result.Item == nil {
        return nil, nil // Cache miss
    }
    
    var translation TranslationResult
    if err := t.unmarshalItem(result.Item, &translation); err != nil {
        return nil, err
    }
    
    return &translation, nil
}
```

## Build Requirements (CRITICAL)

### Before Starting Work:
```bash
make fmt && make lint && make build && make test
```

### After Each Task:
```bash
make fmt && make lint && make build && make test
```

### Final Verification:
```bash
make build && echo "✅ Build successful - Task complete"
```

## Testing Requirements

### For Engagement Value Fixes:
- [ ] Unit test with real hashtag usage data
- [ ] Verify trending algorithms return accurate counts
- [ ] Test time-window calculations (24h, 7d periods)
- [ ] Load test with high-engagement content

### For Storage Interface Methods:
- [ ] Unit test all new interface methods
- [ ] Integration test with status indexer
- [ ] Verify engagement metrics are stored and retrievable
- [ ] Test GSI queries for engagement-based discovery

### For Translation Caching:
- [ ] Test cache hit/miss scenarios
- [ ] Verify TTL expiration works correctly
- [ ] Test with various language pairs
- [ ] Performance test cache lookup speed

## Database Schema Changes

### Engagement Metrics Table Structure:
```go
type EngagementMetrics struct {
    PK               string  `dynamodbav:"PK"`               // STATUS#statusID
    SK               string  `dynamodbav:"SK"`               // ENGAGEMENT#METRICS
    StatusID         string  `dynamodbav:"StatusID"`
    LikeCount        int64   `dynamodbav:"LikeCount"`
    BoostCount       int64   `dynamodbav:"BoostCount"`
    ReplyCount       int64   `dynamodbav:"ReplyCount"`
    Score            float64 `dynamodbav:"Score"`
    EngagementBucket string  `dynamodbav:"EngagementBucket"`
    TTL              int64   `dynamodbav:"TTL"`
}
```

### Translation Cache Structure:
```go
type CachedTranslation struct {
    PK          string `dynamodbav:"PK"`          // TRANSLATION#cacheKey
    SK          string `dynamodbav:"SK"`          // RESULT
    SourceText  string `dynamodbav:"SourceText"`
    SourceLang  string `dynamodbav:"SourceLang"`
    TargetLang  string `dynamodbav:"TargetLang"`
    Translation string `dynamodbav:"Translation"`
    CachedAt    int64  `dynamodbav:"CachedAt"`
    TTL         int64  `dynamodbav:"TTL"`         // 30 days from creation
}
```

## Performance Considerations

### Engagement Calculations:
- Use DynamoDB `SELECT COUNT` for efficiency
- Implement time-window filtering in queries
- Cache frequently requested engagement data
- Use batch operations for bulk updates

### Translation Caching:
- Generate consistent cache keys (hash-based)
- Implement proper TTL for automatic cleanup
- Consider cache warming for common language pairs
- Monitor cache hit rates for optimization

## Success Criteria

### Task 1 Complete When:
- [ ] Hashtag trends show real usage counts (not hardcoded 1)
- [ ] Link trends show real share counts (not hardcoded 1)
- [ ] Status trends show real engagement scores (not hardcoded 1)
- [ ] Time-windowed calculations work correctly
- [ ] No hardcoded placeholder values remain

### Task 2 Complete When:
- [ ] Storage interface includes all engagement methods
- [ ] DynamoDB client implements all new methods
- [ ] Status indexer uses real storage calls (no TODOs)
- [ ] Engagement-based discovery works end-to-end
- [ ] All integration tests pass

### Task 3 Complete When:
- [ ] Translation results are cached in DynamoDB
- [ ] Cache hits return faster than API calls
- [ ] TTL cleanup works automatically
- [ ] No "cache not implemented" returns
- [ ] Cache performance is measurably improved

## Final Deliverable

Upon completion, Team B will have delivered:
- ✅ Accurate engagement metrics with real data calculations
- ✅ Complete storage interface for engagement-based features
- ✅ High-performance translation result caching
- ✅ Optimized DynamoDB queries for trending algorithms
- ✅ **100% implementation completion for all Team B responsibilities**

## Estimated Timeline

- **Day 1**: Fix hardcoded engagement values and add storage methods
- **Day 2**: Complete translation caching and performance optimization
- **Final**: Testing, cleanup, and verification

**Target Completion**: 48 hours maximum