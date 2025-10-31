# GraphQL Validation Coverage

## Currently Tested (Comprehensive Script)

### Account Management
- ✅ Get Actor
- ✅ Update Profile

### Content Creation
- ✅ Create Note
- ✅ Create Reply

### Social Interactions
- ✅ Follow Actor
- ✅ Unfollow Actor
- ✅ Get Relationship
- ✅ Like Post
- ✅ Unlike Post
- ✅ Boost Post
- ✅ Unboost Post
- ✅ Bookmark Post

### Content Operations
- ✅ Delete Post

### Timeline Queries
- ✅ Public Timeline
- ✅ Home Timeline
- ✅ Local Timeline
- ✅ Hashtag Timeline

### Search
- ✅ Search Statuses
- ✅ Search Accounts
- ✅ Search Hashtags

### Notifications
- ✅ Get Notifications

### Lists
- ✅ Get Lists

### Media
- ✅ Get Media Library

### Relationships
- ✅ Get Followers
- ✅ Get Following

### Discovery
- ✅ Profile Directory
- ✅ Suggestions

### Lesser Enhancements
- ✅ Instance Metrics
- ✅ Cost Breakdown

---

## Newly Added (Expanded Script)

### Conversations
- ✅ Get Conversations
- ⚠️ Get Conversation (by ID) - needs conversation ID
- ⚠️ Mark Conversation as Read - needs conversation ID
- ⚠️ Delete Conversation - needs conversation ID

### Lists Operations
- ✅ Create List
- ✅ Get List
- ✅ Get List Accounts
- ✅ Add Accounts to List
- ✅ Remove Accounts from List
- ✅ Update List
- ✅ Delete List

### Media Operations
- ✅ Get Media Library (already covered, but included for completeness)
- ⚠️ Upload Media - requires file upload (not tested)
- ⚠️ Update Media - requires media ID (not tested)

### Scheduled Statuses
- ✅ Schedule Status
- ✅ Get Scheduled Status
- ✅ Get Scheduled Statuses
- ✅ Update Scheduled Status
- ✅ Cancel Scheduled Status

### Custom Emojis
- ✅ Get Custom Emojis
- ⚠️ Create Emoji - requires admin permissions and image upload
- ⚠️ Update Emoji - requires admin permissions
- ⚠️ Delete Emoji - requires admin permissions

### Push Subscriptions
- ✅ Get Push Subscription
- ⚠️ Register Push Subscription - requires WebPush keys
- ⚠️ Update Push Subscription - requires WebPush keys
- ⚠️ Delete Push Subscription

### User Preferences
- ✅ Get User Preferences
- ✅ Update User Preferences

### Block/Mute Operations
- ✅ Block Actor
- ✅ Unblock Actor
- ✅ Mute Actor
- ✅ Unmute Actor
- ✅ Update Relationship

### Pin/Unpin Operations
- ✅ Pin Object
- ✅ Unpin Object

### Notification Operations
- ✅ Get Notifications (with filters)
- ✅ Clear Notifications
- ⚠️ Dismiss Notification - needs notification ID

### Quote Posts
- ✅ Create Quote Note
- ✅ Get Object with Quotes
- ✅ Update Quote Permissions
- ✅ Withdraw From Quotes

### Hashtag Following
- ✅ Get Hashtag
- ✅ Follow Hashtag
- ✅ Get Followed Hashtags
- ✅ Hashtag Timeline
- ✅ Multi Hashtag Timeline
- ✅ Suggested Hashtags
- ✅ Update Hashtag Notifications
- ✅ Unfollow Hashtag

### Thread Synchronization
- ✅ Get Thread Context
- ✅ Sync Missing Replies
- ⚠️ Sync Thread - requires note URL

### Severed Relationships
- ✅ Get Severed Relationships
- ⚠️ Acknowledge Severance - needs severed relationship ID
- ⚠️ Attempt Reconnection - needs severed relationship ID

### Trust & Moderation
- ✅ Get Moderation Queue
- ✅ Get Trust Graph
- ✅ Update Trust
- ✅ Flag Object

### Community Notes
- ✅ Add Community Note
- ✅ Vote Community Note

### AI Analysis
- ✅ Get AI Capabilities
- ✅ Get AI Stats
- ✅ Request AI Analysis
- ✅ Get AI Analysis

### Debug Operations
- ✅ Explain Object
- ✅ Get Federation Status

---

## Not Yet Tested (Available in Schema)

### Phase 2 Extensions (Federation & Cost Management)
- ⚠️ federationCosts
- ⚠️ instanceHealthReport
- ⚠️ costProjections
- ⚠️ mediaStreamUrl
- ⚠️ supportedBitrates
- ⚠️ moderationPatterns
- ⚠️ moderationEffectiveness
- ⚠️ federationLimits
- ⚠️ instanceBudgets
- ⚠️ federationHealth

### Phase 3 Extensions (Advanced Features)
- ⚠️ federationMap
- ⚠️ instanceRelationships
- ⚠️ federationFlow
- ⚠️ streamingAnalytics
- ⚠️ popularStreams
- ⚠️ bandwidthUsage
- ⚠️ moderationDashboard
- ⚠️ patternEffectiveness
- ⚠️ moderatorActivity
- ⚠️ performanceMetrics
- ⚠️ slowQueries
- ⚠️ infrastructureHealth

### Subscriptions (WebSocket)
- ⚠️ All subscriptions require WebSocket connection (not tested via HTTP)

---

## Summary

**Total Tests Added:** ~60+ new test cases
**Coverage Improvement:** ~50% → ~75% of available GraphQL operations
**Critical Missing:** Phase 2/3 federation features, admin operations, WebSocket subscriptions

The expanded validation script focuses on user-facing features that can be tested via HTTP GraphQL queries and mutations.

