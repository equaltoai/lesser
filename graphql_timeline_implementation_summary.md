# GraphQL Timeline & Search Implementation Summary

## Completed: Timeline & Search Queries (Week 3-4, Day 1)

### What Was Implemented

**1. Timeline Query with Multiple Types:**
- ✅ PUBLIC timeline - Shows all public posts
- ✅ LOCAL timeline - Shows only local public posts  
- ✅ HOME timeline - Shows posts from followed accounts (requires auth)
- ✅ HASHTAG timeline - Shows posts with specific hashtag
- ✅ LIST timeline - Shows posts from accounts in a list
- ⏳ DIRECT timeline - Not yet implemented (marked as TODO)

**2. Search Query with Multiple Types:**
- ✅ Search accounts - Find users by username/display name
- ✅ Search statuses - Full-text search on posts
- ✅ Search hashtags - Find trending hashtags
- ✅ Search all - Combined search across all types

**Key Features Added:**
1. **Pagination Support**
   - Cursor-based pagination with `first` and `after` parameters
   - Default limit of 20, max 100
   - Proper PageInfo with hasNextPage, startCursor, endCursor

2. **DataLoader Integration**
   - All objects loaded via DataLoader to prevent N+1 queries
   - Actors loaded through DataLoader for each post

3. **Cost Tracking**
   - DynamoDB reads tracked based on limit
   - Cost info added to response extensions

4. **Error Handling**
   - Proper validation for required parameters (hashtag, listID)
   - Authentication check for HOME timeline
   - Graceful handling of missing objects

5. **Type Conversion**
   - Shared `convertToGraphQLObject` method for consistent conversion
   - Support for Note, Article, and Image types
   - Proper visibility derivation from To/CC fields

### Code Structure

```go
// Timeline resolver handles all timeline types
func (r *queryResolver) Timeline(ctx context.Context, typeArg model.TimelineType, ...) {
    // 1. Parse parameters and set defaults
    // 2. Route to appropriate storage method based on type
    // 3. Load timeline entries from storage
    // 4. Batch load objects using DataLoader
    // 5. Convert to GraphQL objects
    // 6. Build connection with edges and pageInfo
    // 7. Track costs in response extensions
}
```

### Performance Characteristics

**Without DataLoader (N+1 problem):**
- 1 query for timeline entries
- N queries for post objects
- N queries for post authors
- Total: 1 + 2N queries

**With DataLoader (current implementation):**
- 1 query for timeline entries
- 1 batched query for all post objects
- 1 batched query for all unique authors
- Total: 3 queries regardless of N

### Search Implementation Details

**Search Features:**
1. **Multi-type Search**
   - `type: "accounts"` - Search only user accounts
   - `type: "statuses"` - Search only posts
   - `type: "hashtags"` - Search only hashtags
   - `type: "all"` - Search everything (default)

2. **DataLoader Integration**
   - Status objects loaded via DataLoader
   - Prevents N+1 queries when loading search results

3. **Error Handling**
   - Individual search failures don't break "all" search
   - Graceful degradation when some search types fail

### Next Steps

**Priority 1: Instance Metrics Enhancement**
- Replace mock data with real CloudWatch metrics
- Add caching to reduce API calls

**Priority 3: Notifications Query**
```graphql
query Notifications($types: [NotificationType!], $first: Int, $after: Cursor) {
  notifications(types: $types, first: $first, after: $after) {
    edges {
      node {
        id
        type
        createdAt
      }
    }
  }
}
```

### Testing Queries

**Timeline Queries:**
```graphql
# Test public timeline
query PublicTimeline {
  timeline(type: PUBLIC, first: 20) {
    edges {
      node {
        id
        content
        actor {
          username
        }
        createdAt
      }
      cursor
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}

# Test hashtag timeline
query HashtagTimeline {
  timeline(type: HASHTAG, hashtag: "golang", first: 10) {
    edges {
      node {
        id
        content
        tags {
          name
        }
      }
    }
  }
}
```

**Search Queries:**
```graphql
# Search everything
query SearchAll {
  search(query: "golang", first: 20) {
    edges {
      node {
        id
        content
        actor {
          username
        }
        createdAt
      }
    }
    pageInfo {
      hasNextPage
    }
  }
}

# Search only accounts
query SearchAccounts {
  search(query: "john", type: "accounts", first: 10) {
    edges {
      node {
        actor {
          id
          username
          displayName
        }
      }
    }
  }
}

# Search hashtags
query SearchHashtags {
  search(query: "tech", type: "hashtags") {
    edges {
      node {
        content
        tags {
          name
        }
      }
    }
  }
}
```

### TODOs
- [ ] Implement getUsernameFromContext for proper authentication
- [ ] Add boost metadata when IsBoost is true
- [ ] Implement DIRECT timeline type
- [ ] Add proper counts (replies, likes, shares) from storage
- [ ] Add timeline caching for better performance
- [ ] Consider implementing timeline filters (media only, etc.) 