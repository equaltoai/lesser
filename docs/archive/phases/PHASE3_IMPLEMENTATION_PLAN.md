# Phase 3 Implementation Plan: Discovery & Trends

## Overview
This document outlines the implementation plan for Phase 3 of the Mastodon API compatibility, focusing on enhanced search functionality and trending content discovery.

## Current State
- ✅ Basic search endpoint (`GET /api/v1/search`) implemented
- ✅ Advanced account search with multiple strategies
- ✅ Search suggestions and caching
- ❌ Search v2 endpoint (grouped results)
- ❌ Hashtag search
- ❌ Status/post search (beyond URL lookup)
- ❌ Trends system

## Phase 3.1: Enhanced Search Implementation

### 3.1.1 Search v2 Endpoint
**File**: `cmd/api/handlers/misc.go`

The v2 search endpoint differs from v1 by returning grouped results:

```go
// v1 format (current):
{
  "accounts": [...],
  "statuses": [...],
  "hashtags": [...]
}

// v2 format (needed):
{
  "accounts": [...],
  "statuses": [...],
  "hashtags": []
}
```

**Tasks**:
1. Create `HandleSearchV2` function that calls existing search logic
2. No structural changes needed - v2 just has same format as v1
3. Wire up in `main.go` router

### 3.1.2 Hashtag Search Implementation
**Current TODO**: `misc.go:146`

**Tasks**:
1. Create hashtag storage in DynamoDB:
   - PK: `HASHTAG#<tag>`
   - SK: `METADATA`
   - Store usage count, first seen, last used
   
2. Create GSI for hashtag search:
   - GSI3PK: `HASHTAG_SEARCH#<first-2-chars>`
   - GSI3SK: `<tag-lowercase>`

3. Implement hashtag indexing:
   - Extract hashtags when creating statuses
   - Update hashtag usage counts
   - Track trending metrics

4. Implement search logic:
   ```go
   // In HandleSearch, update hashtag search section
   if searchType == "" || searchType == "hashtags" {
       results := h.searchHashtags(ctx, query, limit)
       result.Hashtags = append(result.Hashtags, results...)
   }
   ```

### 3.1.3 Status Search Implementation
**Current TODO**: `misc.go:113`

**Tasks**:
1. Status indexing in DynamoDB:
   - Create GSI for content search
   - GSI5PK: `STATUS_SEARCH#<date>`
   - GSI5SK: `<status-id>`
   - Include searchable content in attributes

2. Full-text search options:
   - Option A: Use OpenSearch for status content (if available)
   - Option B: Use DynamoDB with word tokenization
   - Option C: Basic substring matching in content

3. Implement search logic:
   ```go
   // In HandleSearch, enhance status search section
   if searchType == "" || searchType == "statuses" {
       // Current: Only searches by URL
       // Add: Content search, hashtag filtering, author filtering
       results := h.searchStatuses(ctx, query, options)
       result.Statuses = append(result.Statuses, results...)
   }
   ```

### 3.1.4 Search Filters & Personalization
**TODOs**: 
- `search_service.go:157` - Extract user context
- `search_service.go:190` - Language detection

**Tasks**:
1. User context extraction:
   ```go
   // In Search function
   userID := "" // Extract from auth context
   if claims := auth.GetClaimsFromContext(ctx); claims != nil {
       userID = claims.Username
   }
   ```

2. Implement search filters:
   - `following` - Only accounts user follows
   - `local` - Only local accounts/content
   - `language` - Filter by language

3. Language detection:
   - Use AWS Comprehend for language detection
   - Cache detected languages per user/status
   - Default to instance default language

## Phase 3.2: Trends & Discovery System

### 3.2.1 Trend Tracking Service
**File**: Create `pkg/trends/service.go`

```go
type TrendingService struct {
    store      storage.Storage
    calculator *TrendCalculator
    cache      *TrendCache
}

type Trend struct {
    Name     string
    URL      string
    History  []TrendHistory
    Score    float64
    Category string // hashtags, links, posts
}
```

### 3.2.2 Trend Calculation Algorithm
Implement time-decay algorithm:
- Track interactions (uses, shares, favorites)
- Apply time decay (recent activity weighs more)
- Consider velocity (rate of change)
- Account for spam/manipulation

### 3.2.3 Trend Storage
DynamoDB schema:
```
// Trending hashtags
PK: TREND#HASHTAG#<date>
SK: <tag-name>
Attributes: score, uses, accounts, velocity

// Trending links
PK: TREND#LINK#<date>
SK: <link-hash>
Attributes: url, score, shares, domain

// Trending posts
PK: TREND#POST#<date>
SK: <status-id>
Attributes: score, interactions, author
```

### 3.2.4 Trend Endpoints

1. **GET /api/v1/trends**
   ```go
   func (h *Handler) HandleGetTrends(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
       // Return mixed trending content
       limit := 10 // default
       
       trends := h.trendService.GetMixedTrends(ctx, limit)
       return common.OK(trends), nil
   }
   ```

2. **GET /api/v1/trends/statuses**
   - Return trending posts
   - Include engagement metrics

3. **GET /api/v1/trends/tags**
   - Return trending hashtags
   - Include usage history

4. **GET /api/v1/trends/links**
   - Return trending links
   - Group by domain

### 3.2.5 Discovery Features

1. **GET /api/v1/directory**
   - Profile directory endpoint
   - Filter by:
     - Local/remote
     - Recently active
     - New arrivals
     - Most followed

2. **GET /api/v1/suggestions** (v1)
   - Follow suggestions based on:
     - Common follows
     - Similar interests (hashtags)
     - Engagement patterns

3. **GET /api/v2/suggestions** (v2)
   - Enhanced format with reasons
   - Categories: similar_to_recently_followed, popular, etc.

## Implementation Schedule

### Week 1: Search Enhancements
- [ ] Day 1-2: Implement search v2 endpoint
- [ ] Day 3-4: Hashtag search with storage
- [ ] Day 5: Status search implementation

### Week 2: Search Filters & Language
- [ ] Day 1-2: User context and filtering
- [ ] Day 3: Language detection
- [ ] Day 4-5: Testing and optimization

### Week 3: Trend System Foundation
- [ ] Day 1-2: Trend tracking service
- [ ] Day 3-4: Trend calculation algorithm
- [ ] Day 5: Trend storage implementation

### Week 4: Trend Endpoints & Discovery
- [ ] Day 1-2: Implement trend endpoints
- [ ] Day 3: Directory endpoint
- [ ] Day 4-5: Suggestions endpoints

## Testing Plan

### Search Testing
1. Test v2 endpoint returns same format as v1
2. Verify hashtag search with various queries
3. Test status search with content matching
4. Verify filters work correctly
5. Test language detection accuracy

### Trends Testing
1. Verify trend calculation algorithm
2. Test time decay functionality
3. Ensure trends update properly
4. Test pagination and limits
5. Verify spam resistance

## Migration Considerations

1. **Hashtag Migration**:
   - Scan existing statuses to extract hashtags
   - Build initial hashtag index
   - Backfill usage statistics

2. **Search Index Migration**:
   - Index existing statuses for search
   - Build GSI data for efficient queries

3. **Trend Initialization**:
   - Calculate initial trends from recent data
   - Warm up trend cache

## Performance Optimizations

1. **Search Caching**:
   - Cache popular searches
   - Cache trending items
   - Use Redis/ElastiCache if available

2. **Batch Processing**:
   - Batch trend calculations
   - Async hashtag indexing
   - Background search indexing

3. **Query Optimization**:
   - Use appropriate GSIs
   - Limit scan operations
   - Implement pagination properly

## Security Considerations

1. **Search Privacy**:
   - Respect block relationships
   - Honor account privacy settings
   - Filter deleted/suspended accounts

2. **Trend Manipulation**:
   - Rate limit trend interactions
   - Detect anomalous patterns
   - Weight by account reputation

3. **Content Filtering**:
   - Filter NSFW content from trends
   - Apply word filters
   - Respect instance policies 