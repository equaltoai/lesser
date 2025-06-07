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

4. **Storage Implementation** (`pkg/storage/dynamodb/trends.go`)
   - Complete DynamoDB implementation for trending data
   - Support for all trend types: hashtags, statuses, links

5. **Storage Interface Updates**
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
- [x] DynamoDB storage implementation
- [x] Routes wired in main.go

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