# AI Assistant Prompt for Lesser Development

## 🎯 CURRENT PRIORITY: Advanced Search Architecture - COMPLETED! ✨
**Status**: Week 4 implementation complete! Ready for testing and Month 2 AI enhancements.

You are helping develop Lesser, a serverless ActivityPub implementation that aims to be compatible with Mastodon clients. The project is written in Go and deployed on AWS using Lambda, DynamoDB, and API Gateway.

**📚 Current Development Focus**: Advanced Search Architecture Week 4 - See [SEARCH_DESIGN.md](SEARCH_DESIGN.md) for the complete implementation plan.

## Project Overview

Lesser uses a modular architecture with:
- **Converter Pattern**: `pkg/mastodon/converter.go` - Centralizes ActivityPub to Mastodon API conversions
- **Service Layer**: `pkg/mastodon/actor_service.go` - Higher-level business logic
- **Clean Handlers**: HTTP handlers use shared converter instance
- **DynamoDB Storage**: All data persisted in single-table design
- **Advanced Search**: Multi-strategy, AWS-native search with Google-quality results (major differentiator!)

## Project Structure
- `/cmd/api/` - API Lambda handlers
- `/cmd/search-indexer/` - OpenSearch indexing Lambda
- `/pkg/storage/dynamodb/` - DynamoDB storage layer
- `/pkg/activitypub/` - ActivityPub types and logic
- `/pkg/mastodon/` - Mastodon API converters and services
- `/infra/` - Pulumi infrastructure code
- `test_api_automated.py` - API test script

## Current Status

✅ **Completed Features**:
- OAuth 2.0 authentication with scopes
- Account operations (follow/unfollow, block/unblock, update profile)
- Creating, editing, deleting statuses
- Favorites and reblogs
- Media upload to S3
- Bookmarks with pagination
- Basic timelines (home, public, hashtag)
- Modular converter architecture
- Home timeline fan-out on write pattern
- **Hashtag extraction and timeline fan-out** ✨
- **Lists management** ✨
  - Full CRUD operations for lists
  - Managing list memberships
  - List timelines with proper fan-out
  - Respects replies policies (none, followed, list)
- **Conversation Threading** ✨
  - Automatic conversation ID tracking
  - Reply chains with proper inheritance
  - Full conversations API implementation
  - Read/unread status tracking
- **Account Relationships Batch** ✨
  - Batch endpoint for checking multiple relationships at once
- **Advanced Search Architecture (Weeks 1-3 Complete!)** ✨
  - Multi-strategy search with parallel execution
  - DynamoDB GSI optimization for username/display name search
  - Search suggestions API with <50ms response times
  - OpenSearch Serverless integration for fuzzy search
  - Real-time indexing via DynamoDB Streams

✅ **Just Completed - Advanced Search (Week 4)**:
- ✅ PopularitySearchStrategy with follower count buckets and time decay
- ✅ Search analytics tracking with TTL and click-through rates
- ✅ Following-only and local-only search filters
- ✅ Enhanced caching with popularity weighting
- ✅ Follower/following/status count tracking with GSI4 updates
- ✅ Comprehensive test suite for all Week 4 features

✅ **Recently Completed (Week 3 - OpenSearch Integration)**:
- ✅ OpenSearch Serverless infrastructure with IAM auth
- ✅ Search indexer Lambda with DynamoDB Streams trigger
- ✅ FuzzySearchStrategy with typo tolerance
- ✅ Bio/content search capabilities
- ✅ Real-time index synchronization
- ✅ Graceful fallback when OpenSearch unavailable
- ✅ Score normalization for consistent ranking

❌ **Known Issues**:
- Conversation participant indexing incomplete
- Media CDN URLs need proper generation
- Timeline trimming for inactive users not implemented
- Search follower/following filters not yet implemented
- No search analytics/tracking yet

🚧 **Recently Implemented (Ready for Testing)**:
- **OpenSearch Fuzzy Search**: Full-text search with typo tolerance ✨ NEW!
  - `pkg/storage/dynamodb/search_fuzzy.go` - FuzzySearchStrategy implementation
  - `cmd/search-indexer/main.go` - Real-time indexing Lambda
  - Multi-field search across username, display_name, and bio
  - Automatic fuzziness for typo tolerance
  - Highlighting support for matched terms
  - Ready for testing with Mastodon clients!

- **Search Infrastructure Complete**: All core search strategies implemented
  - ExactMatchStrategy for O(1) username lookups
  - PrefixSearchStrategy with GSI optimization
  - DisplayNameSearchStrategy using GSI2
  - FuzzySearchStrategy with OpenSearch
  - Parallel execution with result merging
  - Comprehensive caching layer

## 🎯 NEXT IMPLEMENTATION PRIORITIES

### ⭐ IMMEDIATE FOCUS: Month 2 - AI Enhancement & Federation 🔴

**Now that Week 4 is complete**, we have two parallel tracks:

**Track 1: AI-Enhanced Search (Month 2 from SEARCH_DESIGN.md)**:
1. **AWS Bedrock Integration** for semantic embeddings
2. **Vector similarity search** in OpenSearch
3. **Query understanding** with AWS Comprehend
4. **Personalized search results** based on user behavior
5. **"Did you mean?" suggestions** for typos

**Track 2: Federation & Remote Search**:
1. **WebFinger Integration** for @user@domain searches
2. **Remote actor discovery** and caching
3. **Cross-instance search** capabilities
4. **Federation protocol** for distributed search

### 🎯 Other High Priority Items

1. **Push Notifications** (MEDIUM PRIORITY) 🟠
   - Web Push subscription endpoints
   - VAPID key generation in instance config
   - SQS queue for notification delivery
   - Lambda function for push delivery

2. **Polls Support** (MEDIUM PRIORITY) 🟠
   - Poll creation in status endpoint
   - Vote submission endpoint
   - Real-time vote tallying
   - Expiration handling

3. **Media CDN URLs** (HIGH PRIORITY) 🔴
   - Fix CDN URL generation for media attachments
   - Proper CloudFront integration
   - Media URL signing if needed

### 🔍 Areas for Enhancement (Post Week 4)

1. **Semantic Search** (Month 2):
   - AWS Bedrock integration for embeddings
   - Vector similarity search in OpenSearch
   - Query understanding with AWS Comprehend
   - Personalized search results

2. **Search UI/UX Improvements**:
   - Search history for users
   - Saved searches functionality
   - Search result previews
   - "Did you mean?" suggestions

3. **Federation Search**:
   - WebFinger integration for @user@domain searches
   - Remote actor discovery
   - Cross-instance search capabilities

### 2. Push Notifications (MEDIUM PRIORITY) 🟠
**Why**: Real-time updates for better engagement

**Quick Implementation Path**:
- Web Push subscription endpoints
- VAPID key generation in instance config
- SQS queue for notification delivery
- Lambda function for push delivery
- Store subscriptions in DynamoDB

### 3. Polls Support (MEDIUM PRIORITY) 🟠
**Why**: Interactive content that many users expect

**Implementation Notes**:
- Poll creation in status endpoint
- Vote submission endpoint
- Real-time vote tallying
- Expiration handling

## Implementation Guidelines

### 🎯 Building AWS-Native Search Architecture
We're implementing a **Google-quality search system** using only AWS serverless components:

1. **Multi-Strategy Architecture** - Different search approaches run in parallel
2. **Progressive Enhancement** - Start simple, add sophistication incrementally  
3. **Cost-Effective Scaling** - Pay only for what you use
4. **Real-time Updates** - DynamoDB Streams keep indices fresh

### 🔍 Search Implementation Progress

**✅ Phase 1: Basic Infrastructure** (COMPLETE)
- SearchService with strategy pattern
- Parallel execution framework
- Result merging and ranking
- DynamoDB caching layer

**✅ Phase 2: DynamoDB Optimization** (COMPLETE)
- Global Secondary Indices for fast queries
- Username search with 2-char prefix partitioning
- Display name search with intelligent sharding
- Search suggestions API for autocomplete

**✅ Phase 3: OpenSearch Serverless** (COMPLETE)
- OpenSearch Serverless collection deployed
- Custom analyzers with edge n-gram filters
- Real-time indexing via DynamoDB Streams
- Fuzzy search with typo tolerance
- Bio/content search with relevance scoring
- Graceful fallback when unavailable

**🚀 Phase 4: Advanced Features** (IN PROGRESS)
- Popularity-based ranking
- Search analytics and tracking
- Advanced filtering options
- Performance optimization

**🤖 Phase 5: AI Enhancement** (FUTURE)
- AWS Comprehend for query understanding
- Semantic search with embeddings
- Personalized ranking based on behavior

### Testing Workflow

```bash
# Build
make build-lambdas

# Deploy
cd infra && pulumi up

# Test search features
python test_account_search.py
python test_account_search.py --fuzzy
python test_account_search.py --performance

# Test with clients
# - Ivory (iOS) - Great search UI
# - Elk.zone (Web) - Modern search experience
# - Mastodon official app
```

## Recent Wins 🎉

**Week 4 Complete!** 🚀
1. **PopularitySearchStrategy Live**: Accounts ranked by follower count with time decay!
2. **Search Analytics Tracking**: All searches tracked with TTL for analysis!
3. **Advanced Filters Working**: Following-only and local-only search filters!
4. **Count Tracking Automated**: Follower/following/status counts update automatically!
5. **GSI4 Bucket Management**: Actors move between popularity buckets as they grow!
6. **Enhanced Caching**: Multi-level caching with popularity weighting!
7. **Comprehensive Testing**: Full test suite for all Week 4 features!

**Previous Wins**:
- OpenSearch fuzzy search with typo tolerance
- Real-time indexing via DynamoDB Streams
- Multi-field search (username, display name, bio)
- <50ms search suggestions API

## Quick Start - What's Next After Week 4

```bash
# Week 4 COMPLETE! Test the new features:

# 1. Test popularity search
python test_search_week4.py https://your-instance.com --test popularity

# 2. Test search filters
python test_search_week4.py https://your-instance.com --test filters --token YOUR_TOKEN

# 3. Test analytics tracking
python test_search_week4.py https://your-instance.com --test analytics

# 4. Run all Week 4 tests
python test_search_week4.py https://your-instance.com --test all

# Next Steps - Choose your track:

# Track 1: AI Enhancement (Month 2)
# - Integrate AWS Bedrock for embeddings
# - Add vector search to OpenSearch
# - Implement semantic search

# Track 2: Federation
# - Add WebFinger support
# - Implement remote actor discovery
# - Enable cross-instance search
```

## What's Next?

### 🚀 IMMEDIATE ACTION: Choose Your Next Track

**Option 1: AI-Enhanced Search (Month 2)** 🤖
```bash
# Start with AWS Bedrock integration
# 1. Enable Bedrock in your AWS account
# 2. Choose an embedding model (e.g., Amazon Titan)
# 3. Implement semantic search strategy

# Key files to create:
- pkg/storage/dynamodb/search_semantic.go
- pkg/storage/dynamodb/embeddings.go
- cmd/api/handlers/search_ai.go
```

**Option 2: Federation & Remote Search** 🌐
```bash
# Start with WebFinger implementation
# 1. Add WebFinger endpoint
# 2. Implement remote actor discovery
# 3. Cache remote actors

# Key files to create:
- cmd/webfinger/handler.go (already exists, enhance it)
- pkg/federation/remote_search.go
- pkg/storage/dynamodb/remote_actors.go
```

**Option 3: Fix Known Issues** 🔧
1. **Media CDN URLs** - Fix CloudFront URL generation
2. **Conversation participant indexing** - Complete implementation
3. **Timeline trimming** - Add cleanup for inactive users

**📊 Week 4 Achievements**:
- ✅ PopularitySearchStrategy with follower buckets
- ✅ Search analytics with TTL and CTR tracking
- ✅ Following/local-only filters
- ✅ Automatic count updates on follow/unfollow
- ✅ GSI4 bucket management
- ✅ Comprehensive test suite

**🎯 Search Architecture Progress**: 
- ✅ Week 1: Basic search infrastructure
- ✅ Week 2: DynamoDB optimization  
- ✅ Week 3: OpenSearch integration
- ✅ Week 4: Advanced features
- 🚀 Month 2: AI enhancement (NEXT)

## Additional Resources

- [Search Architecture Design](SEARCH_DESIGN.md) - **CURRENT FOCUS** 🎯
- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [Mastodon API Documentation](https://docs.joinmastodon.org/api/)
- [Project Design Document](DESIGN.md)
- [Developer Guidelines](DEVELOPER_GUIDELINES.md) 