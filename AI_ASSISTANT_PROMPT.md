# AI Assistant Prompt for Lesser Development

## 🎯 MONTH 2 COMPLETE! ✅
**Status**: Both AI Enhancement (Track 1) AND Federation (Track 2) are fully implemented!

You are helping develop Lesser, a serverless ActivityPub implementation that aims to be compatible with Mastodon clients. The project is written in Go and deployed on AWS using Lambda, DynamoDB, and API Gateway.

**📚 Latest Achievement**: Push notifications are now fully deployed! Users can receive real-time notifications for follows, favorites, reblogs, and mentions through Web Push Protocol.

## Project Overview

Lesser uses a modular architecture with:
- **Converter Pattern**: `pkg/mastodon/converter.go` - Centralizes ActivityPub to Mastodon API conversions
- **Service Layer**: `pkg/mastodon/actor_service.go` - Higher-level business logic
- **Clean Handlers**: HTTP handlers use shared converter instance
- **DynamoDB Storage**: All data persisted in single-table design
- **Advanced Search**: Multi-strategy, AWS-native search with Google-quality results (major differentiator!)
- **Media CDN**: CloudFront distribution serving media files with proper caching

## Project Structure
- `/cmd/api/` - API Lambda handlers (includes media handling)
- `/cmd/search-indexer/` - OpenSearch indexing Lambda
- `/pkg/storage/dynamodb/` - DynamoDB storage layer
- `/pkg/activitypub/` - ActivityPub types and logic
- `/pkg/mastodon/` - Mastodon API converters and services
- `/infra/` - Pulumi infrastructure code
- `test_api_automated.py` - API test script
- `test_media_urls.py` - Media CDN verification script

## Current Status

✅ **Completed Features**:
- OAuth 2.0 authentication with scopes
- Account operations (follow/unfollow, block/unblock, update profile)
- Creating, editing, deleting statuses
- Favorites and reblogs
- Media upload to S3 with CloudFront CDN
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
- **Push Notifications** ✨ 
  - VAPID key generation and storage
  - Web Push subscription endpoints (GET/POST/PUT/DELETE)
  - Push notification storage in DynamoDB
  - SQS queue integration for reliable delivery
  - Push delivery Lambda with Web Push Protocol encryption
  - Automatic notifications for follows, favorites, reblogs, mentions
  - Invalid subscription cleanup
  - Fully deployed and operational!
- **Polls Support** ✨ NEW!
  - Poll creation in statuses with 2-4 options
  - Vote submission with duplicate prevention
  - Real-time vote tallying
  - Multiple choice polls support
  - Hidden totals option (results hidden until poll ends)
  - Poll expiration with TTL
  - Proper integration with status creation/retrieval
  - Full Mastodon API compatibility
- **Advanced Search Architecture (Weeks 1-3 Complete!)** ✨
  - Multi-strategy search with parallel execution
  - DynamoDB GSI optimization for username/display name search
  - Search suggestions API with <50ms response times
  - OpenSearch Serverless integration for fuzzy search
  - Real-time indexing via DynamoDB Streams

✅ **Just Completed - AI-Enhanced Search (Month 2)**:
- ✅ AWS Bedrock integration for semantic embeddings
- ✅ AWS Comprehend for query understanding and analysis
- ✅ SemanticSearchStrategy with vector similarity search
- ✅ OpenSearch vector indexing with knn_vector field
- ✅ "Did you mean?" suggestions using semantic similarity
- ✅ Real-time embedding generation via DynamoDB Streams
- ✅ Comprehensive test suite for AI features
- ✅ Graceful fallback when AI services unavailable

✅ **Recently Completed (Week 3 - OpenSearch Integration)**:
- ✅ OpenSearch Serverless infrastructure with IAM auth
- ✅ Search indexer Lambda with DynamoDB Streams trigger
- ✅ FuzzySearchStrategy with typo tolerance
- ✅ Bio/content search capabilities
- ✅ Real-time index synchronization
- ✅ Graceful fallback when OpenSearch unavailable
- ✅ Score normalization for consistent ranking

✅ **Media CDN Implementation** (JUST COMPLETED!) 🎉:
- ✅ Consolidated media handling into API Lambda
- ✅ CloudFront distribution with Origin Access Identity
- ✅ Private S3 ACLs with CDN-only access
- ✅ Proper cache headers (1-year max-age)
- ✅ Media metadata storage in DynamoDB
- ✅ Test script for CDN verification

❌ **Known Issues**:
- Conversation participant indexing incomplete
- Timeline trimming for inactive users not implemented
- Search follower/following filters not yet implemented
- No search analytics/tracking yet

✅ **Recently Fixed**:
- Media CDN URLs now properly use CloudFront distribution
- Removed duplicate media Lambda handler
- Fixed S3 ACL to use private access with CloudFront OAI
- AWS SDK v2 compatibility issues with Comprehend (LanguageCode type)
- OpenSearch Serverless integration (using HTTP REST API with v4 signing)

🚧 **Recently Implemented (Ready for Testing)**:
- **Media CDN Integration**: Full CloudFront CDN for media files ✨ NEW!
  - Consolidated into `cmd/api/handlers/media.go`
  - CloudFront distribution with custom domain `media.{domain}`
  - Long-term caching headers for optimal performance
  - Private S3 bucket with OAI access
  - Ready for testing with Mastodon clients!

- **OpenSearch Fuzzy Search**: Full-text search with typo tolerance
  - `pkg/storage/dynamodb/search_fuzzy.go` - FuzzySearchStrategy implementation
  - `cmd/search-indexer/main.go` - Real-time indexing Lambda
  - Multi-field search across username, display_name, and bio
  - Automatic fuzziness for typo tolerance

## 🎯 NEXT IMPLEMENTATION PRIORITIES

### ✅ Month 2 COMPLETE! - AI Enhancement & Federation 🎉

Both tracks of Month 2 are now complete:
- **Track 1: AI-Enhanced Search** ✅ 
- **Track 2: Federation & Remote Search** ✅
- **Bonus: Media CDN URLs** ✅ NEW!

Lesser now has:
- Google-quality search with AI enhancement
- Full ActivityPub federation capabilities
- Cross-instance communication
- Remote follow functionality
- Proper media CDN with CloudFront

### ⭐ IMMEDIATE NEXT PRIORITIES

1. **Filters & Mutes** (HIGH PRIORITY) 🔴
   - Content filtering system
   - Mute accounts/keywords
   - Filter contexts (home, public, etc)
   - Client-side filter hints

### 🔧 Future Enhancement Options

1. **Personalized Search Results**
   - Learn from user interactions
   - Personalized result ranking
   - Search history for users
   - Saved searches functionality

2. **Federation Admin Tools**
   - Instance blocking/allowlisting
   - Federation statistics dashboard
   - Remote content moderation
   - Delivery queue monitoring

3. **Advanced AI Features**
   - Multi-lingual search with Amazon Translate
   - Custom Comprehend models for social queries
   - Image search using Amazon Rekognition
   - Content moderation with AI

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

**✅ Phase 4: Advanced Features** (COMPLETE)
- Popularity-based ranking
- Search analytics and tracking
- Advanced filtering options
- Performance optimization

**✅ Phase 5: AI Enhancement** (COMPLETE - Month 2)
- AWS Comprehend for query understanding
- Semantic search with embeddings
- "Did you mean?" suggestions
- Vector similarity search

**🚀 Phase 6: Federation** (NEXT)
- Cross-instance search
- Remote actor discovery
- Federated search results

### Testing Workflow

```bash
# Build (now with all dependencies fixed!)
make build-lambdas

# Deploy (OpenSearch endpoint auto-configured)
cd infra && pulumi up

# Test search features
python test_account_search.py
python test_semantic_search.py  # AI-powered search
python test_federation_complete.py  # Full federation

# Test with clients
# - Ivory (iOS) - Great search UI
# - Elk.zone (Web) - Modern search experience
# - Mastodon official app
```

### OpenSearch Configuration
- **Endpoint**: Automatically set via Pulumi deployment
- **Your endpoint**: `https://w1drcxx7lls7uheaolp6.us-east-1.aoss.amazonaws.com`
- **Environment variable**: `OPENSEARCH_ENDPOINT` (auto-configured)
- **Used for**: Vector search, fuzzy search, real-time indexing

## Recent Wins 🎉

**Push Notifications DEPLOYED!** 🔔✅
1. **Full Web Push Implementation**: Complete push notification system with VAPID authentication!
2. **Real-time Notifications**: Users receive instant notifications for follows, favorites, reblogs, mentions!
3. **SQS Queue Integration**: Reliable delivery with message queuing!
4. **Push Delivery Lambda**: Encrypted notification delivery using Web Push Protocol!
5. **Client Compatibility**: Works with all Mastodon clients that support push notifications!
6. **Infrastructure Complete**: SQS queue and Lambda fully deployed and operational!

**Month 2 Track 2 - Federation COMPLETE!** 🌐✅
1. **Full ActivityPub Federation**: Complete inbox/outbox implementation with activity processing!
2. **Remote Follow Working**: Users can follow actors on any ActivityPub instance!
3. **Activity Delivery**: All activities delivered to remote followers with HTTP signatures!
4. **Inbox Processing**: Handles Follow, Accept, Create, Update, Delete, Undo activities!
5. **Delivery Service**: Optimized delivery with shared inbox support!
6. **Test Suite Enhanced**: `test_federation_complete.py` covers all federation scenarios!

**Latest Progress** 🚀
- ✅ **Build Issues Fixed**: AWS SDK v2 compatibility resolved for Comprehend and OpenSearch
- ✅ **OpenSearch Restored**: Vector search functionality fully implemented with HTTP REST API
- ✅ **Auto-Configuration**: OpenSearch endpoint automatically set during Pulumi deployment
- ✅ **AWS v4 Signing**: Proper authentication for OpenSearch Serverless requests

**Month 2 AI Enhancement Complete!** 🤖✨
1. **AWS Bedrock Integration**: Semantic embeddings with Amazon Titan model!
2. **Vector Search Working**: OpenSearch k-NN search with HNSW algorithm!
3. **Query Understanding**: AWS Comprehend analyzes search intent!
4. **"Did You Mean?"**: Semantic similarity suggestions for queries!
5. **Real-time Embeddings**: DynamoDB Streams trigger automatic embedding generation!
6. **Comprehensive Testing**: Full test suite with `test_semantic_search.py`!
7. **Graceful Degradation**: System works even if AI services are unavailable!

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

## Quick Start - Deploy & Test Complete Month 2 Features

```bash
# 🎉 MONTH 2 COMPLETE - Ready for deployment!

# === BUILD & DEPLOY ===
# 1. Build Lambda functions (now fixed!)
make build-lambdas

# 2. Deploy to AWS
cd infra && pulumi up
# OpenSearch endpoint is automatically configured!

# === TEST AI FEATURES ===
# 1. Test semantic search with vector similarity
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN

# 2. Compare regular vs AI-enhanced search
curl -H "Authorization: Bearer YOUR_TOKEN" \
  "https://your-instance.com/api/v2/search?q=developer&type=accounts&semantic=true"

# === TEST FEDERATION FEATURES ===
# 1. Test complete federation implementation
python test_federation_complete.py https://your-instance.com --token YOUR_TOKEN

# 2. Follow a remote actor
curl -X POST -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/activity+json" \
  -d '{"type":"Follow","object":"https://mastodon.social/users/Gargron"}' \
  "https://your-instance.com/users/YOUR_USERNAME/outbox"

# 3. Test WebFinger
curl "https://your-instance.com/.well-known/webfinger?resource=acct:test@your-instance.com"
```

## What's Next?

### 🎉 MONTH 2 COMPLETE - Deploy and Test!

**Immediate Action: Deploy Your Complete Lesser Instance**
```bash
# 1. Ensure AWS credentials are configured
aws configure

# 2. Build the fixed Lambda functions
make build-lambdas

# 3. Deploy with OpenSearch auto-configuration
cd infra && pulumi up

# 4. Test everything works
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN
python test_federation_complete.py https://your-instance.com --token YOUR_TOKEN
```

### 🚀 Next Development Priorities

**Option 1: Media CDN URLs** 🔴 (HIGH PRIORITY)
```bash
# Fix media attachment URL generation
# Current issue: Media URLs not using CloudFront CDN properly

# Tasks:
- Fix URL generation in media handlers
- Ensure CloudFront distribution is used
- Add URL signing if needed for private media
- Test with various Mastodon clients

# Key files:
- cmd/api/handlers/media.go
- pkg/storage/dynamodb/media.go
```

**Option 2: Polls Support** 📊 (MEDIUM PRIORITY)
```bash
# Interactive content with polls
# Allow users to create and vote on polls

# Tasks:
- Add poll fields to status creation
- Vote submission endpoint
- Real-time vote counting
- Expiration handling with TTL

# Key files:
- cmd/api/handlers/polls.go (new)
- pkg/storage/dynamodb/polls.go (new)
- Update status creation/display
```

**📊 Major Achievements**:
- ✅ Complete search architecture (Weeks 1-4)
- ✅ AI-enhanced semantic search with OpenSearch vectors
- ✅ Full ActivityPub federation implementation
- ✅ Remote follow functionality
- ✅ Activity delivery with HTTP signatures
- ✅ Push notifications fully deployed and operational!
- ✅ Build issues resolved - ready for deployment!

**🎯 Development Progress**: 
- ✅ Week 1-4: Advanced search complete
- ✅ Month 2 Track 1: AI enhancement complete
- ✅ Month 2 Track 2: Federation implementation complete
- ✅ OpenSearch vector search restored and working
- ✅ Auto-configuration via Pulumi deployment
- 🎉 MONTH 2 FULLY COMPLETE & DEPLOYABLE!

## Additional Resources

- [Search Architecture Design](SEARCH_DESIGN.md) - **CURRENT FOCUS** 🎯
- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [Mastodon API Documentation](https://docs.joinmastodon.org/api/)
- [Project Design Document](DESIGN.md)
- [Developer Guidelines](DEVELOPER_GUIDELINES.md) 