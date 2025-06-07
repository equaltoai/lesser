# Timeline Implementation Design for Lesser

## Overview
This document outlines the ideal timeline implementation for Lesser, optimized for serverless architecture and DynamoDB.

## Design Principles
1. **Cost-effective**: Minimize DynamoDB read/write units
2. **Performant**: Sub-second timeline loads
3. **Scalable**: Handle both small instances and celebrity accounts
4. **Flexible**: Support various timeline types and filters

## Timeline Types

### 1. Home Timeline
Shows posts from accounts the user follows.

**Storage Design:**
```
Primary Table:
PK: TIMELINE#HOME#<username>
SK: <timestamp>#<post-id>
Attributes:
  - postId: String
  - actorId: String
  - content: String (first 280 chars for preview)
  - hasMedia: Boolean
  - isReply: Boolean
  - isBoost: Boolean
  - boostedBy: String (if applicable)
  - visibility: String
  - createdAt: String
  - expiresAt: Number (TTL for auto-cleanup)
```

**Write Strategy:**
- Posts from users with < 1000 followers: Fan-out on write
- Posts from users with > 1000 followers: Fan-in on read
- Use DynamoDB Streams → Lambda for async timeline updates

**Read Strategy:**
```python
# Pseudocode for reading home timeline
def get_home_timeline(username, cursor=None, limit=20):
    # 1. Get pre-computed timeline entries
    push_posts = query(
        PK="TIMELINE#HOME#{username}",
        SK_begins_with=cursor or "9999999999",
        limit=limit,
        scan_forward=False  # Newest first
    )
    
    # 2. Get celebrity follows (if needed)
    if len(push_posts) < limit:
        celebrity_follows = get_celebrity_follows(username)
        pull_posts = get_recent_posts_from(celebrity_follows)
        
    # 3. Merge and sort
    return merge_and_sort(push_posts, pull_posts)
```

### 2. Public Timeline
Shows all public posts from the instance.

**Storage Design:**
```
Global Secondary Index:
GSI1PK: TIMELINE#PUBLIC#<LOCAL|FEDERATED>
GSI1SK: <timestamp>#<post-id>
Attributes: (same as home timeline)
```

**Write Strategy:**
- Only index posts with visibility="public"
- Separate local vs federated for filtering
- Use TTL to auto-expire old entries (e.g., 7 days)

### 3. List Timelines
For Mastodon lists feature.

**Storage Design:**
```
PK: TIMELINE#LIST#<list-id>
SK: <timestamp>#<post-id>
```

## Optimization Techniques

### 1. Timeline Pruning
- Set TTL on timeline entries (e.g., 30 days)
- Keeps storage costs down
- Users rarely scroll back months

### 2. Smart Caching
```javascript
// Lambda@Edge for timeline caching
const timelineCache = {
    key: `timeline:${type}:${userId}:${page}`,
    ttl: 60, // 1 minute for active users
    staleWhileRevalidate: 300 // 5 minutes
};
```

### 3. Batch Writing
```javascript
// Use DynamoDB batch writes for fan-out
async function fanOutToTimelines(post, followerIds) {
    const chunks = chunk(followerIds, 25); // DynamoDB batch limit
    
    for (const batch of chunks) {
        const items = batch.map(followerId => ({
            PutRequest: {
                Item: {
                    PK: `TIMELINE#HOME#${followerId}`,
                    SK: `${timestamp}#${post.id}`,
                    // ... other attributes
                }
            }
        }));
        
        await dynamodb.batchWriteItem({ RequestItems: { [TABLE]: items }});
    }
}
```

### 4. Progressive Enhancement
```javascript
// Start with basic timeline, enhance with interactions
async function enhanceTimelineItems(items, currentUser) {
    // Basic items returned immediately
    const enhanced = await Promise.all(items.map(async item => {
        // Parallel fetch additional data
        const [likes, boosts, bookmarks] = await Promise.all([
            checkIfLiked(item.id, currentUser),
            checkIfBoosted(item.id, currentUser),
            checkIfBookmarked(item.id, currentUser)
        ]);
        
        return { ...item, liked: likes, boosted: boosts, bookmarked: bookmarks };
    }));
    
    return enhanced;
}
```

## Migration Path

### Phase 1: Basic Implementation
- Simple fan-out on read for all timelines
- Direct queries to actor posts
- Basic merge and sort

### Phase 2: Optimized Home Timeline
- Add timeline-specific storage
- Implement fan-out on write for small accounts
- Add DynamoDB Streams processing

### Phase 3: Full Optimization
- Celebrity account handling
- Timeline caching
- Progressive enhancement
- List timelines

## Cost Analysis

### Fan-out on Write Costs
- 1 post from user with 100 followers:
  - 1 write to outbox: 1 WCU
  - 100 writes to timelines: 100 WCU
  - Total: 101 WCU (~$0.00013)

### Fan-out on Read Costs
- 1 timeline request with 20 follows:
  - 1 query per follow: 20 RCU
  - Merge and sort: CPU only
  - Total: 20 RCU (~$0.0000052)

### Hybrid Approach (Recommended)
- Balances costs based on follower count
- Predictable performance
- Scales to millions of users

## Implementation Priority

1. **MVP**: Simple pull-based timeline
2. **V1**: Public timeline with GSI
3. **V2**: Hybrid home timeline
4. **V3**: Full optimization with caching

## Sample DynamoDB Queries

### Home Timeline Query
```javascript
const params = {
    TableName: TABLE_NAME,
    KeyConditionExpression: 'PK = :pk AND SK < :sk',
    ExpressionAttributeValues: {
        ':pk': `TIMELINE#HOME#${username}`,
        ':sk': cursor || '9999999999'
    },
    Limit: limit,
    ScanIndexForward: false // Newest first
};
```

### Public Timeline Query
```javascript
const params = {
    TableName: TABLE_NAME,
    IndexName: 'GSI1',
    KeyConditionExpression: 'GSI1PK = :pk AND GSI1SK < :sk',
    ExpressionAttributeValues: {
        ':pk': `TIMELINE#PUBLIC#LOCAL`,
        ':sk': cursor || '9999999999'
    },
    Limit: limit,
    ScanIndexForward: false
};
```

## Performance Targets

- Home timeline load: < 200ms (p95)
- Public timeline load: < 100ms (p95)
- Timeline write fan-out: < 5s (async)
- Cache hit rate: > 80% for active users 