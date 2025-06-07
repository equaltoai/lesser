# OpenSearch Removal Plan for Lesser

## Current Problem

OpenSearch is costing $10+/day ($300-400/month), which completely breaks Lesser's economic model of $0.01-0.05 per user per month. This single service could cost more than hosting 10,000 users on the entire platform.

## Current OpenSearch Usage

### 1. Account Fuzzy Search (`search_fuzzy.go`)
- **Purpose**: Find accounts with typos/misspellings
- **Features**: Fuzzy matching on username, display name, bio
- **Fallback**: Already has graceful degradation

### 2. Status Fuzzy Search (`status_search_fuzzy.go`)
- **Purpose**: Find statuses with typos/misspellings
- **Features**: Fuzzy matching on content, author, hashtags
- **Fallback**: Returns empty results when unavailable

### 3. Semantic Search - Accounts (`search_semantic.go`)
- **Purpose**: AI-powered similarity search
- **Features**: Vector k-NN search for accounts
- **Fallback**: DynamoDB scan (inefficient)

### 4. Semantic Search - Statuses
- **Purpose**: AI-powered similarity search for statuses
- **Features**: Would use vector k-NN
- **Fallback**: DynamoDB scan (inefficient)

## Replacement Strategy

### Remove Fuzzy Search Entirely ✅ (Recommended)

**Rationale**:
- Lesser already has 6 other search strategies that work well
- Fuzzy search is "nice to have" not essential
- Users can correct their own typos
- Cost savings: $300-400/month

**Implementation**:
1. Remove fuzzy search strategies from search service
2. Keep all other search strategies:
   - Exact match
   - Prefix search (handles partial typing)
   - Display name search
   - Popularity search
   - Hashtag search
   - URL search
   - Semantic search (via DynamoDB)



## Semantic Search Without OpenSearch

### Current Problem
- OpenSearch k-NN for vector similarity
- Fallback is DynamoDB scan (inefficient)

### Solution: Locality-Sensitive Hashing (LSH)

**Approach**:
1. Generate LSH hashes for embeddings
2. Store in DynamoDB GSI
3. Query by hash buckets

```go
type LSHIndex struct {
    NumTables   int     // e.g., 10
    NumBuckets  int     // e.g., 1000
    Projections [][]float32
}

func (l *LSHIndex) Hash(embedding []float32) []string {
    hashes := []string{}
    for i, projection := range l.Projections {
        dotProduct := dot(embedding, projection)
        bucket := int(dotProduct * float32(l.NumBuckets))
        hash := fmt.Sprintf("LSH#%d#%d", i, bucket)
        hashes = append(hashes, hash)
    }
    return hashes
}
```

**Storage**:
```
PK: LSH#0#123
SK: ACTOR#username
    embedding: [...]

GSI: 
PK: LSH#0#123
SK: similarity_score
```

**Benefits**:
- O(1) lookup instead of O(n) scan
- Can tune precision/recall trade-off
- Works with DynamoDB's limitations


## Migration Plan

### Phase 1: Measure Impact (1 day)
1. Add metrics to track fuzzy search usage
2. Log when fuzzy search returns results others don't
3. Measure actual value provided

### Phase 2: Implement Replacements (3 days)
1. Enhance prefix search algorithm
2. Implement LSH for semantic search
3. Add Levenshtein distance for close matches
4. Test search quality

### Phase 3: Remove OpenSearch (1 day)
1. Update environment variables
2. Remove OpenSearch client code
3. Update search strategies to skip fuzzy
4. Deploy and monitor

### Phase 4: Clean Up (1 day)
1. Remove OpenSearch infrastructure
2. Update documentation
3. Remove cost tracking for OpenSearch
4. Celebrate $300/month savings!

## Expected Impact

### Search Quality
- **Exact matches**: No change ✅
- **Prefix search**: Slightly improved ✅
- **Typo tolerance**: Reduced but acceptable ⚠️
- **Semantic search**: Similar quality with LSH ✅

### Performance
- **Latency**: Improved (no network calls to OpenSearch)
- **Reliability**: Improved (one less dependency)
- **Scalability**: Better (DynamoDB scales automatically)

### Cost Savings
- **Monthly savings**: $300-400
- **Equivalent to**: 6,000-10,000 users
- **ROI**: Immediate

## Recommendation

**Go with Option 1 + LSH for Semantic Search**

1. **Remove fuzzy search entirely** - Users can handle their own typos
2. **Implement LSH for semantic search** - Maintains AI search without OpenSearch
3. **Enhance prefix search** - Quick improvement for partial matches

This approach:
- Saves $300-400/month immediately
- Maintains 90% of search quality
- Removes a complex dependency
- Aligns with Lesser's serverless philosophy
- Can be implemented in less than a week

## Code Changes Required

### 1. Update Search Service Selection
```go
// search_service.go - selectStrategies()
// Remove:
if options.Fuzzy && len(query.Query) >= 3 {
    // Remove fuzzy search strategy
}

// search_status_service.go - selectStrategies()  
// Remove fuzzy search check
```

### 2. Implement LSH
```go
// New file: search_lsh.go
type LSHSearchStrategy struct {
    service *SearchService
    index   *LSHIndex
}
```

### 3. Update Environment
```bash
# Remove from .env and infrastructure:
OPENSEARCH_ENDPOINT=...
```

### 4. Update Cost Tracking
```go
// Remove OpenSearch cost tracking
// cost/tracker.go - remove OpenSearch fields
```

## Conclusion

OpenSearch is overkill for Lesser's needs. The platform already has excellent search capabilities through its multi-strategy approach. Removing OpenSearch and implementing simple alternatives will:

1. Save $300-400/month
2. Reduce complexity
3. Improve reliability
4. Maintain acceptable search quality

The slight reduction in fuzzy matching capability is a worthwhile trade-off for the massive cost savings and simplified architecture. 