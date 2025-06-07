# Trends & Discovery Implementation Guide

## Overview
This document outlines the implementation of Mastodon API's Trends & Discovery features (section 3.2) in Lesser.

## Architecture

### Components Created

1. **Trends Service** (`pkg/trends/service.go`)
   - Core trending logic with pluggable algorithms
   - Time-decay algorithm for trend scoring
   - Support for hashtags, statuses, and links

2. **API Handlers**
   - `cmd/api/handlers/trends.go` - Trend endpoints
   - `cmd/api/handlers/discovery.go` - Directory and suggestions

3. **Lambda Function** (`cmd/trend-aggregator/main.go`)
   - Periodic aggregation of trending data
   - Scheduled via EventBridge (hourly/daily)

4. **Storage Interface Updates**
   - Added trending-related methods to `pkg/storage/interface.go`
   - New types: `TrendingHashtag`, `TrendingStatus`, `TrendingLink`

## DynamoDB Schema Design

### Trending Data Storage

#### Hashtag Trends
```
PK: TREND#HASHTAG#<hashtag>
SK: TREND#<timestamp>
Attributes:
  - UsageCount: Number
  - UniqueUsers: Number
  - LastUsed: String (ISO timestamp)
  - FirstSeen: String (ISO timestamp)
  - TrendScore: Number
  - TTL: Number (7 days)
```

#### Status Trends
```
PK: TREND#STATUS#<status_id>
SK: TREND#<timestamp>
Attributes:
  - Engagements: Number
  - TrustScore: Number
  - AuthorID: String
  - Content: String (first 500 chars)
  - URL: String
  - PublishedAt: String
  - TrendScore: Number
  - TTL: Number (7 days)
```

#### Link Trends
```
PK: TREND#LINK#<url_hash>
SK: TREND#<timestamp>
Attributes:
  - URL: String
  - ShareCount: Number
  - UniqueSharers: Number
  - Title: String
  - Description: String
  - Image: String
  - Type: String
  - TrendScore: Number
  - TTL: Number (7 days)
```

### GSI for Trending Queries
```
GSI8 (Trending Index):
  PK: TREND_TYPE#<type>#<time_bucket>
  SK: SCORE#<padded_score>#<id>
```

This allows efficient queries for top trending items by type and time period.

## Implementation Status

### Completed ✅
- [x] Core trending service with algorithm interface
- [x] Time-decay trending algorithm
- [x] API handlers for all trend endpoints
- [x] Discovery handlers (directory, suggestions)
- [x] Lambda function structure for aggregation
- [x] Storage interface updates

### TODO 📝

#### 1. Storage Implementation
- [ ] Implement trending methods in `pkg/storage/dynamodb/trends.go`
- [ ] Add GSI8 to DynamoDB table for trend queries
- [ ] Implement TTL cleanup for old trends

#### 2. Trend Aggregation
- [ ] Complete hashtag trend aggregation in Lambda
- [ ] Complete status trend aggregation
- [ ] Implement link extraction and aggregation
- [ ] Add trust score integration

#### 3. API Integration
- [ ] Wire up trend handlers in `cmd/api/main.go`
- [ ] Add trend service initialization
- [ ] Configure Lambda deployment

#### 4. Discovery Features
- [ ] Implement proper account discovery flags
- [ ] Add follow suggestion algorithm
- [ ] Store dismissed suggestions
- [ ] Implement "similar accounts" logic

#### 5. Link Timeline
- [ ] Create link timeline storage pattern
- [ ] Implement link extraction from statuses
- [ ] Add link timeline endpoint

## API Endpoints

### Implemented Handlers
- `GET /api/v1/trends` - Mixed trends
- `GET /api/v1/trends/statuses` - Trending posts
- `GET /api/v1/trends/tags` - Trending hashtags  
- `GET /api/v1/trends/links` - Trending links
- `GET /api/v1/timelines/link` - Link timeline
- `GET /api/v1/directory` - Profile directory
- `GET /api/v1/suggestions` - Follow suggestions (v1)
- `GET /api/v2/suggestions` - Follow suggestions (v2)
- `DELETE /api/v1/suggestions/:account_id` - Remove suggestion

## Next Steps

1. **Create DynamoDB Implementation**
   ```go
   // pkg/storage/dynamodb/trends.go
   func (s *dynamoDBStorage) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error
   func (s *dynamoDBStorage) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
   // ... etc
   ```

2. **Update main.go Routes**
   ```go
   // Initialize trend service
   trendService := trends.NewService(storage)
   trendHandlers := handlers.NewTrendHandlers(trendService)
   discoveryHandlers := handlers.NewDiscoveryHandlers(storage)
   
   // Add routes
   r.Get("/api/v1/trends", trendHandlers.GetTrends)
   r.Get("/api/v1/trends/statuses", trendHandlers.GetTrendingStatuses)
   // ... etc
   ```

3. **Deploy Lambda Function**
   - Build and deploy trend-aggregator Lambda
   - Configure EventBridge rule for periodic execution
   - Set appropriate IAM permissions for DynamoDB access

4. **Integration Points**
   - Hook into status creation to record hashtag usage
   - Hook into like/boost activities for engagement tracking
   - Extract and index links from new statuses

## Trending Algorithm Details

The default algorithm uses:
- **Time Decay**: Recent activity weighted more heavily (2-hour half-life)
- **Engagement Factor**: Likes, boosts, replies increase score
- **Diversity Factor**: Unique users valued over repeat usage
- **Trust Factor**: Higher trust users contribute more to trends

Score calculation:
```
score = usageCount * ageFactor * (1 + engagementFactor) * (1 + diversityFactor) * (1 + trustFactor)
```

## Lesser-Specific Enhancements

1. **Trust Integration**: Trending items from high-trust users rank higher
2. **Moderation Mesh**: Flagged content excluded from trends
3. **Cost Tracking**: Monitor trending calculation costs
4. **Community Notes**: Notes can affect trend scores
5. **Federation Aware**: Separate local vs federated trends

## Testing Considerations

- Unit tests for trending algorithm
- Integration tests for trend aggregation
- Performance tests for large datasets
- Mock data for development/testing 