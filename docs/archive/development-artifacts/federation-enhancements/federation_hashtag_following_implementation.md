# Hashtag Following Implementation Guide for Lesser

## 🎯 Overview

Hashtag following is a fundamental feature missing from many ActivityPub implementations. Lesser will implement comprehensive hashtag following with advanced notification controls.

## 🏗️ Architecture

### Core Components

```go
// pkg/hashtags/types.go
type Hashtag struct {
    Name         string    // Normalized hashtag (lowercase, no #)
    DisplayName  string    // Original casing
    FollowerCount int64
    PostCount    int64
    TrendingScore float64
    FirstSeen    time.Time
    LastUsed     time.Time
}

type HashtagFollow struct {
    ID           string
    UserID       string
    Hashtag      string
    CreatedAt    time.Time
    NotifyLevel  NotificationLevel
    Muted        bool
    MutedUntil   *time.Time
}

type NotificationLevel string

const (
    NotifyAll      NotificationLevel = "all"
    NotifyMutuals  NotificationLevel = "mutuals"
    NotifyFollowing NotificationLevel = "following"
    NotifyNone     NotificationLevel = "none"
)
```

## 📦 Storage Implementation

### DynamoDB Schema

**Table: hashtag-follows**
```
PK: userID
SK: hashtag#<name>
Attributes:
  - followID
  - createdAt
  - notifyLevel
  - muted
  - mutedUntil
```

**GSI: follows-by-hashtag**
```
PK: hashtag
SK: userID
Attributes:
  - followID
  - createdAt
  - notifyLevel
```

**Table: hashtag-stats**
```
PK: hashtag
SK: STATS
Attributes:
  - followerCount
  - postCount
  - trendingScore
  - lastUpdated
```

## 🔄 Federation Protocol

### Follow Activity

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    "https://lesser.social/ns/hashtags"
  ],
  "type": "Follow",
  "id": "https://example.com/activities/12345",
  "actor": "https://example.com/users/alice",
  "object": {
    "type": "Hashtag",
    "name": "#golang",
    "href": "https://example.com/tags/golang"
  },
  "_:followMeta": {
    "notifyLevel": "mutuals",
    "autoBoost": false
  }
}
```

### Unfollow Activity

```json
{
  "type": "Undo",
  "actor": "https://example.com/users/alice",
  "object": {
    "type": "Follow",
    "id": "https://example.com/activities/12345",
    "object": {
      "type": "Hashtag",
      "name": "#golang"
    }
  }
}
```

## 💻 GraphQL Implementation

### Schema

```graphql
type Hashtag {
  name: String!
  displayName: String!
  url: String!
  followerCount: Int!
  postCount: Int!
  trendingScore: Float!
  isFollowing: Boolean!
  followedAt: DateTime
  posts(first: Int, after: String): PostConnection!
  relatedHashtags: [Hashtag!]!
}

type HashtagFollow {
  id: ID!
  hashtag: Hashtag!
  user: Actor!
  createdAt: DateTime!
  notifyLevel: NotificationLevel!
  muted: Boolean!
  mutedUntil: DateTime
}

enum NotificationLevel {
  ALL
  MUTUALS
  FOLLOWING
  NONE
}

extend type Query {
  hashtag(name: String!): Hashtag
  followedHashtags(
    first: Int
    after: String
  ): HashtagFollowConnection!
  
  hashtagTimeline(
    hashtag: String!
    onlyMedia: Boolean
    first: Int
    after: String
  ): PostConnection!
  
  multiHashtagTimeline(
    hashtags: [String!]!
    mode: HashtagMode!
    first: Int
    after: String
  ): PostConnection!
}

enum HashtagMode {
  ANY  # Posts with any of the hashtags
  ALL  # Posts with all hashtags
}

extend type Mutation {
  followHashtag(
    hashtag: String!
    notifyLevel: NotificationLevel
  ): HashtagFollowPayload!
  
  unfollowHashtag(hashtag: String!): UnfollowHashtagPayload!
  
  updateHashtagNotifications(
    hashtag: String!
    notifyLevel: NotificationLevel!
    muted: Boolean
    mutedUntil: DateTime
  ): UpdateHashtagNotificationsPayload!
  
  muteHashtag(
    hashtag: String!
    duration: Int # minutes
  ): MuteHashtagPayload!
}
```

### Resolver Implementation

```go
// graph/schema.resolvers.go

func (r *mutationResolver) FollowHashtag(ctx context.Context, hashtag string, notifyLevel *model.NotificationLevel) (*model.HashtagFollowPayload, error) {
    // 1. Authenticate
    user := auth.UserFromContext(ctx)
    if user == nil {
        return nil, errors.New("authentication required")
    }

    // 2. Normalize hashtag
    normalizedTag := normalizeHashtag(hashtag)

    // 3. Check if already following
    existing, _ := r.Storage.GetHashtagFollow(ctx, user.ID, normalizedTag)
    if existing != nil {
        return nil, errors.New("already following this hashtag")
    }

    // 4. Create follow
    follow := &hashtags.HashtagFollow{
        ID:          generateID(),
        UserID:      user.ID,
        Hashtag:     normalizedTag,
        CreatedAt:   time.Now(),
        NotifyLevel: defaultNotifyLevel(notifyLevel),
    }

    // 5. Store follow
    if err := r.Storage.CreateHashtagFollow(ctx, follow); err != nil {
        return nil, err
    }

    // 6. Update hashtag stats
    r.Storage.IncrementHashtagFollowers(ctx, normalizedTag)

    // 7. Create Follow activity
    activity := &activitypub.Activity{
        Type:  "Follow",
        Actor: user.URL,
        Object: map[string]interface{}{
            "type": "Hashtag",
            "name": "#" + normalizedTag,
            "href": fmt.Sprintf("%s/tags/%s", r.Config.InstanceURL, normalizedTag),
        },
    }

    // 8. Queue for federation
    r.FederationQueue.Send(ctx, activity)

    // 9. Track costs
    r.CostTracker.TrackDynamoWrite(2) // Follow + stats update

    return &model.HashtagFollowPayload{
        HashtagFollow: modelHashtagFollow(follow),
    }, nil
}

func (r *queryResolver) HashtagTimeline(ctx context.Context, hashtag string, onlyMedia *bool, first *int, after *string) (*model.PostConnection, error) {
    // 1. Normalize hashtag
    normalizedTag := normalizeHashtag(hashtag)

    // 2. Build query
    query := &storage.TimelineQuery{
        Hashtag:   normalizedTag,
        OnlyMedia: onlyMedia != nil && *onlyMedia,
        Limit:     defaultLimit(first),
        Cursor:    after,
    }

    // 3. Fetch posts
    posts, nextCursor, err := r.Storage.GetHashtagTimeline(ctx, query)
    if err != nil {
        return nil, err
    }

    // 4. Track costs
    r.CostTracker.TrackDynamoRead(len(posts))

    // 5. Build connection
    return &model.PostConnection{
        Edges: toPostEdges(posts),
        PageInfo: &model.PageInfo{
            HasNextPage: nextCursor != "",
            EndCursor:   nextCursor,
        },
    }, nil
}
```

## 🔔 Advanced Notification System

### Notification Rules

```go
type HashtagNotificationRule struct {
    UserID       string
    Hashtag      string
    Rules        []NotificationCondition
    LastModified time.Time
}

type NotificationCondition struct {
    Type      ConditionType
    Value     interface{}
    Action    NotificationAction
}

type ConditionType string

const (
    ConditionAuthorFollowed    ConditionType = "author_followed"
    ConditionAuthorMutual      ConditionType = "author_mutual"
    ConditionMinEngagement     ConditionType = "min_engagement"
    ConditionLanguage          ConditionType = "language"
    ConditionTimeOfDay         ConditionType = "time_of_day"
    ConditionKeywordPresent    ConditionType = "keyword_present"
    ConditionKeywordAbsent     ConditionType = "keyword_absent"
)
```

### Smart Notification Filtering

```go
func (n *NotificationService) ShouldNotifyHashtagPost(ctx context.Context, post *Post, userID string) bool {
    // 1. Get user's hashtag follows
    follows := n.Storage.GetUserHashtagFollows(ctx, userID)
    
    // 2. Check which followed hashtags are in the post
    matchedHashtags := findMatchingHashtags(post.Hashtags, follows)
    if len(matchedHashtags) == 0 {
        return false
    }
    
    // 3. Apply notification rules
    for _, hashtag := range matchedHashtags {
        follow := follows[hashtag]
        
        // Check basic notification level
        switch follow.NotifyLevel {
        case NotifyNone:
            continue
        case NotifyMutuals:
            if !n.isMutual(ctx, userID, post.AuthorID) {
                continue
            }
        case NotifyFollowing:
            if !n.isFollowing(ctx, userID, post.AuthorID) {
                continue
            }
        }
        
        // Check advanced rules
        if n.checkAdvancedRules(ctx, post, userID, hashtag) {
            return true
        }
    }
    
    return false
}
```

## 📊 Timeline Generation

### Combined Hashtag Timeline

```go
func (s *Storage) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, mode string, limit int, cursor string) ([]*Post, string, error) {
    if mode == "ANY" {
        // Union of posts with any hashtag
        return s.getHashtagUnion(ctx, hashtags, limit, cursor)
    } else if mode == "ALL" {
        // Intersection of posts with all hashtags
        return s.getHashtagIntersection(ctx, hashtags, limit, cursor)
    }
    return nil, "", errors.New("invalid mode")
}

func (s *Storage) getHashtagUnion(ctx context.Context, hashtags []string, limit int, cursor string) ([]*Post, string, error) {
    // Use DynamoDB batch get with multiple hashtag queries
    var queries []dynamodb.QueryInput
    for _, tag := range hashtags {
        queries = append(queries, buildHashtagQuery(tag, limit, cursor))
    }
    
    // Execute queries in parallel
    results := s.executeBatchQueries(ctx, queries)
    
    // Merge and sort results
    posts := mergeSortPosts(results, limit)
    
    return posts, generateCursor(posts), nil
}
```

## 🎨 UI/UX Considerations

### Hashtag Discovery

```go
type HashtagSuggestion struct {
    Hashtag       string
    Reason        string // "trending", "related", "popular_with_follows"
    Score         float64
    PostCount     int64
    FollowerCount int64
}

func (s *SuggestionService) GetHashtagSuggestions(ctx context.Context, userID string) ([]*HashtagSuggestion, error) {
    // 1. Get user's current hashtags
    following := s.Storage.GetUserHashtagFollows(ctx, userID)
    
    // 2. Analyze user's engagement
    engaged := s.analyzeUserHashtagEngagement(ctx, userID)
    
    // 3. Find related hashtags
    related := s.findRelatedHashtags(ctx, following, engaged)
    
    // 4. Add trending hashtags
    trending := s.getTrendingHashtags(ctx)
    
    // 5. Score and rank suggestions
    suggestions := s.rankSuggestions(related, trending, userID)
    
    return suggestions[:10], nil
}
```

## 🔍 Search Integration

### Hashtag Search

```go
type HashtagSearchResult struct {
    Hashtag       *Hashtag
    IsFollowing   bool
    MutualFollowers []string
    RecentPosts   []*Post
}

func (s *SearchService) SearchHashtags(ctx context.Context, query string, userID string) ([]*HashtagSearchResult, error) {
    // 1. Fuzzy search hashtags
    matches := s.fuzzySearchHashtags(ctx, query)
    
    // 2. Enhance with user context
    results := make([]*HashtagSearchResult, 0, len(matches))
    for _, hashtag := range matches {
        result := &HashtagSearchResult{
            Hashtag:     hashtag,
            IsFollowing: s.isFollowing(ctx, userID, hashtag.Name),
        }
        
        // Add mutual followers
        result.MutualFollowers = s.getMutualHashtagFollowers(ctx, userID, hashtag.Name)
        
        // Add recent posts preview
        result.RecentPosts = s.getRecentHashtagPosts(ctx, hashtag.Name, 3)
        
        results = append(results, result)
    }
    
    return results, nil
}
```

## 📈 Analytics & Insights

### Hashtag Analytics

```go
type HashtagAnalytics struct {
    Hashtag         string
    FollowerGrowth  []DataPoint
    PostFrequency   []DataPoint
    EngagementRate  float64
    TopContributors []string
    PeakHours       []int
    RelatedHashtags []string
}

func (a *AnalyticsService) GetHashtagAnalytics(ctx context.Context, hashtag string, period time.Duration) (*HashtagAnalytics, error) {
    // Comprehensive hashtag analytics for users
    // Helps users discover best times to post
    // Shows hashtag community health
}
```

## 🧪 Testing

### Unit Tests

```go
func TestHashtagFollowing(t *testing.T) {
    // Test follow/unfollow operations
    // Test notification level changes
    // Test muting functionality
}

func TestHashtagTimeline(t *testing.T) {
    // Test single hashtag timeline
    // Test multi-hashtag with ANY mode
    // Test multi-hashtag with ALL mode
    // Test pagination
}

func TestHashtagNotifications(t *testing.T) {
    // Test notification filtering
    // Test advanced rules
    // Test muting periods
}
```

## 🚀 Performance Optimizations

### Caching Strategy

```go
type HashtagCache struct {
    // Cache popular hashtag timelines
    TimelineCache *cache.LRU
    
    // Cache hashtag stats
    StatsCache *cache.TTL
    
    // Cache user's followed hashtags
    FollowCache *cache.TTL
}
```

### Timeline Pre-computation

```go
// Pre-compute timelines for trending hashtags
func (s *TimelineService) PrecomputeTrendingTimelines(ctx context.Context) {
    trending := s.getTrendingHashtags(ctx, 100)
    
    for _, hashtag := range trending {
        timeline := s.computeHashtagTimeline(ctx, hashtag.Name)
        s.cache.SetTimeline(hashtag.Name, timeline, 5*time.Minute)
    }
}
```

## 🎯 Success Metrics

1. **Adoption**: 40% of users follow at least one hashtag
2. **Engagement**: 25% increase in hashtag post discovery
3. **Retention**: Users following hashtags have 30% better retention
4. **Performance**: < 100ms timeline generation
5. **Federation**: Seamless hashtag following across instances

## 🏁 Conclusion

Lesser's hashtag following implementation provides:
- **Comprehensive following** with advanced notifications
- **Flexible timelines** (single, multi, union, intersection)
- **Smart filtering** to reduce notification noise
- **Federation support** for cross-instance hashtag communities
- **Analytics** to help users optimize their hashtag usage

This positions Lesser as having the most advanced hashtag system in the Fediverse. 