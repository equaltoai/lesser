# MockStorage Methods Index

This document provides a comprehensive index of all methods implemented in the MockStorage test utility (`internal/testutil/mocks/storage.go`).

## Overview

The MockStorage implementation provides mock versions of all storage interface methods for testing purposes. It includes:

- **MockStorage**: Uses testify's mock framework for controlled testing
- **BaseMockStorage**: Provides no-op implementations for simple testing
- **TimelineMethods**: Embedded helper for timeline-related methods

## Method Categories

### Actor Operations
- `CreateActor(ctx, actor, privateKey) error`
- `GetActor(ctx, username) (*Actor, error)`
- `GetActorByNumericID(ctx, numericID) (*Actor, error)`
- `GetActorWithMetadata(ctx, username) (*Actor, *ActorMetadata, error)`
- `GetActorPrivateKey(ctx, username) (string, error)`
- `UpdateActor(ctx, actor) error`
- `UpdateActorLastStatusTime(ctx, username) error`
- `SetActorFields(ctx, username, fields) error`
- `DeleteActor(ctx, username) error`
- `SearchAccounts(ctx, query, limit, followingOnly, offset) ([]*Actor, error)`
- `GetSearchSuggestions(ctx, prefix) ([]SearchSuggestion, error)`
- `CacheRemoteActor(ctx, handle, actor, ttl) error`
- `GetCachedRemoteActor(ctx, handle) (*Actor, error)`

### Activity Operations
- `CreateActivity(ctx, activity) error`
- `GetActivity(ctx, id) (*Activity, error)`
- `GetOutboxActivities(ctx, username, limit, cursor) ([]*Activity, string, error)`
- `GetInboxActivities(ctx, username, limit, cursor) ([]*Activity, string, error)`
- `RecordActivity(ctx, activityType, actorID, timestamp) error`

### Object Operations
- `CreateObject(ctx, object) error`
- `GetObject(ctx, id) (interface{}, error)`
- `UpdateObject(ctx, object) error`
- `DeleteObject(ctx, id) error`
- `GetObjectsByActor(ctx, actorID, cursor, limit) ([]interface{}, string, error)`
- `CountObjectReplies(ctx, objectID) (int, error)`
- `TombstoneObject(ctx, objectID, deletedBy) error`
- `GetTombstone(ctx, objectID) (*Tombstone, error)`

### Update History Operations
- `CreateUpdateHistory(ctx, history) error`
- `GetUpdateHistory(ctx, objectID, limit) ([]*UpdateHistory, error)`

### Relationship Operations
- `CreateFollow(ctx, followerUsername, followedUsername, followActivityID) error`
- `AcceptFollow(ctx, followerUsername, followedUsername) error`
- `RejectFollow(ctx, followerUsername, followedUsername) error`
- `RemoveFollow(ctx, followerUsername, followedUsername) error`
- `GetFollowers(ctx, username, limit, cursor) ([]string, string, error)`
- `GetFollowing(ctx, username, limit, cursor) ([]string, string, error)`
- `IsFollowing(ctx, followerUsername, followedUsername) (bool, error)`
- `GetPendingFollowRequests(ctx, username, limit, cursor) ([]string, string, error)`
- `GetFollowRequestState(ctx, followerUsername, followedUsername) (string, error)`
- `GetFollowerCount(ctx, actorID) (int, error)`
- `GetFollowingCount(ctx, actorID) (int, error)`
- `RemoveFromFollowers(ctx, username, followerUsername) error`

### Collection Operations
- `GetCollection(ctx, username, collectionType, limit, cursor) (*OrderedCollectionPage, error)`
- `AddToCollection(ctx, collection, item) error`
- `RemoveFromCollection(ctx, collection, itemID) error`
- `GetCollectionItems(ctx, collection, limit, cursor) ([]*CollectionItem, string, error)`
- `IsInCollection(ctx, collection, itemID) (bool, error)`
- `CountCollectionItems(ctx, collection) (int, error)`

### OAuth Operations
- `CreateAuthorizationCode(ctx, code) error`
- `GetAuthorizationCode(ctx, code) (*AuthorizationCode, error)`
- `DeleteAuthorizationCode(ctx, code) error`
- `CreateRefreshToken(ctx, token) error`
- `GetRefreshToken(ctx, token) (*RefreshToken, error)`
- `DeleteRefreshToken(ctx, token) error`
- `CreateOAuthClient(ctx, client) error`
- `GetOAuthClient(ctx, clientID) (*OAuthClient, error)`
- `UpdateOAuthClient(ctx, clientID, updates) error`
- `DeleteOAuthClient(ctx, clientID) error`
- `ListOAuthClients(ctx, limit, cursor) ([]*OAuthClient, string, error)`
- `StoreOAuthState(ctx, state, data) error`
- `GetOAuthState(ctx, state) (*OAuthState, error)`
- `DeleteOAuthState(ctx, state) error`

### User Operations
- `CreateUser(ctx, user) error`
- `GetUser(ctx, username) (*User, error)`
- `GetUserByEmail(ctx, email) (*User, error)`
- `UpdateUser(ctx, username, updates) error`
- `DeleteUser(ctx, username) error`
- `ListUsers(ctx, limit, cursor) ([]*User, string, error)`
- `GetActiveUserCount(ctx, days) (int64, error)`
- `GetUserByProviderID(ctx, provider, providerID) (*User, error)`
- `LinkProviderAccount(ctx, username, provider, providerID) error`
- `UnlinkProviderAccount(ctx, username, provider) error`
- `GetLinkedProviders(ctx, username) ([]string, error)`

### Instance Metrics Operations
- `GetTotalUserCount(ctx) (int64, error)`
- `GetTotalStatusCount(ctx) (int64, error)`
- `GetTotalDomainCount(ctx) (int64, error)`
- `GetWeeklyActivity(ctx, weekTimestamp) (*WeeklyActivity, error)`
- `GetContactAccount(ctx) (*ActorRecord, error)`

### Recovery Operations
- `StoreRecoveryToken(ctx, key, data) error`
- `GetRecoveryToken(ctx, key) (map[string]interface{}, error)`
- `DeleteRecoveryToken(ctx, key) error`
- `GetRecoveryCodes(ctx, username) ([]*RecoveryCodeItem, error)`
- `StoreRecoveryCode(ctx, username, code) error`
- `MarkRecoveryCodeUsed(ctx, username, codeHash) error`
- `CountUnusedRecoveryCodes(ctx, username) (int, error)`
- `DeleteAllRecoveryCodes(ctx, username) error`
- `GetActiveRecoveryRequests(ctx, username) ([]*SocialRecoveryRequest, error)`
- `StoreRecoveryRequest(ctx, request) error`
- `GetRecoveryRequest(ctx, requestID) (*SocialRecoveryRequest, error)`
- `UpdateRecoveryRequest(ctx, request) error`
- `DeleteRecoveryRequest(ctx, requestID) error`

### Like Operations
- `CreateLike(ctx, like) error`
- `GetLike(ctx, actor, object) (*Like, error)`
- `DeleteLike(ctx, actor, object) error`
- `GetObjectLikes(ctx, objectID, limit, cursor) ([]*Like, string, error)`
- `GetActorLikes(ctx, actorID, limit, cursor) ([]*Like, string, error)`
- `CountObjectLikes(ctx, objectID) (int, error)`
- `CascadeDeleteLikes(ctx, objectID) error`

### Announce Operations
- `CreateAnnounce(ctx, announce) error`
- `GetAnnounce(ctx, actor, object) (*Announce, error)`
- `DeleteAnnounce(ctx, actor, object) error`
- `GetObjectAnnounces(ctx, objectID, limit, cursor) ([]*Announce, string, error)`
- `GetActorAnnounces(ctx, actorID, limit, cursor) ([]*Announce, string, error)`
- `CountObjectAnnounces(ctx, objectID) (int, error)`
- `CascadeDeleteAnnounces(ctx, objectID) error`

### Block Operations
- `CreateBlock(ctx, block) error`
- `GetBlock(ctx, actor, blockedActor) (*Block, error)`
- `DeleteBlock(ctx, actor, blockedActor) error`
- `GetBlockedActors(ctx, actor, limit, cursor) ([]*Block, string, error)`
- `GetBlockedByActors(ctx, actor, limit, cursor) ([]*Block, string, error)`
- `IsBlocked(ctx, actor, targetActor) (bool, error)`
- `IsBlockedBidirectional(ctx, actor1, actor2) (bool, error)`

### Flag Operations (Content Moderation)
- `CreateFlag(ctx, flag) error`
- `GetFlag(ctx, id) (*Flag, error)`
- `GetFlagsByObject(ctx, objectID, limit, cursor) ([]*Flag, string, error)`
- `GetFlagsByActor(ctx, actorID, limit, cursor) ([]*Flag, string, error)`
- `GetPendingFlags(ctx, limit, cursor) ([]*Flag, string, error)`
- `UpdateFlagStatus(ctx, id, status, reviewedBy, reviewNote) error`
- `CountPendingFlags(ctx) (int, error)`

### Move Operations (Account Migration)
- `CreateMove(ctx, move) error`
- `GetMove(ctx, actor) (*Move, error)`
- `GetMoveByTarget(ctx, target) ([]*Move, error)`
- `HasMovedFrom(ctx, oldActor, newActor) (bool, error)`

### Timeline Operations
- `WriteToTimeline(ctx, timeline) error`
- `WriteToTimelines(ctx, entries) error`
- `GetHomeTimeline(ctx, username, limit, cursor) ([]*TimelineEntry, string, error)`
- `GetPublicTimeline(ctx, local, limit, cursor) ([]*TimelineEntry, string, error)`
- `GetListTimeline(ctx, listID, limit, cursor) ([]*TimelineEntry, string, error)`
- `GetHashtagTimeline(ctx, hashtag, local, limit, cursor) ([]*TimelineEntry, string, error)`
- `DeleteFromTimeline(ctx, timelineType, timelineID, entryID) error`
- `DeleteExpiredTimelineEntries(ctx, before) error`
- `FanOutPost(ctx, activity) error`

### Instance Configuration Operations
- `GetInstanceRules(ctx) ([]InstanceRule, error)`
- `SetInstanceRules(ctx, rules) error`
- `GetExtendedDescription(ctx) (string, time.Time, error)`
- `SetExtendedDescription(ctx, description) error`

### Bookmark Operations
- `CreateBookmark(ctx, username, objectID) error`
- `RemoveBookmark(ctx, username, objectID) error`
- `GetBookmarks(ctx, username, limit, cursor) ([]string, string, error)`
- `IsBookmarked(ctx, username, objectID) (bool, error)`

### Conversation Operations
- `CreateConversation(ctx, conversation) error`
- `GetConversation(ctx, id) (*Conversation, error)`
- `GetConversationByParticipants(ctx, participants) (*Conversation, error)`
- `UpdateConversationLastStatus(ctx, id, lastStatusID) error`
- `MarkConversationRead(ctx, id, username) error`
- `DeleteConversation(ctx, id) error`
- `GetUserConversations(ctx, username, limit, cursor) ([]*Conversation, string, error)`
- `AddParticipantToConversation(ctx, conversationID, participantID) error`

### List Operations
- `CreateList(ctx, username, title, repliesPolicy) (*List, error)`
- `GetList(ctx, listID) (*List, error)`
- `GetListsForUser(ctx, username) ([]*List, error)`
- `UpdateList(ctx, listID, updates) error`
- `DeleteList(ctx, listID) error`
- `AddAccountsToList(ctx, listID, accountIDs) error`
- `RemoveAccountsFromList(ctx, listID, accountIDs) error`
- `GetListAccounts(ctx, listID) ([]string, error)`
- `IsAccountInList(ctx, listID, accountID) (bool, error)`
- `GetListsContainingAccount(ctx, accountID, username) ([]*List, error)`

### Notification Operations
- `CreateNotification(ctx, notification) error`
- `GetNotification(ctx, id) (*Notification, error)`
- `GetNotifications(ctx, username, limit, cursor) ([]*Notification, string, error)`
- `GetNotificationsFiltered(ctx, username, filter) ([]*Notification, string, error)`
- `MarkNotificationAsRead(ctx, id) error`
- `MarkAllNotificationsAsRead(ctx, username) error`
- `DeleteNotification(ctx, id) error`
- `ClearNotifications(ctx, username) error`
- `CountUnreadNotifications(ctx, username) (int, error)`

### Push Notification Operations
- `CreatePushSubscription(ctx, username, subscription) error`
- `GetPushSubscription(ctx, username, subscriptionID) (*PushSubscription, error)`
- `GetUserPushSubscriptions(ctx, username) ([]*PushSubscription, error)`
- `UpdatePushSubscription(ctx, username, subscriptionID, alerts) error`
- `DeletePushSubscription(ctx, username, subscriptionID) error`
- `DeleteAllPushSubscriptions(ctx, username) error`

### VAPID Key Operations
- `GetVAPIDKeys(ctx) (*VAPIDKeys, error)`
- `SetVAPIDKeys(ctx, keys) error`

### Poll Operations
- `CreatePoll(ctx, poll) error`
- `GetPoll(ctx, pollID) (*Poll, error)`
- `GetPollByStatusID(ctx, statusID) (*Poll, error)`
- `VoteOnPoll(ctx, pollID, voterID, choices) error`
- `GetPollVotes(ctx, pollID) (map[string][]int, error)`
- `HasUserVoted(ctx, pollID, userID) (bool, []int, error)`

### Mute Operations
- `CreateMute(ctx, mute) error`
- `GetMute(ctx, actor, mutedActor) (*Mute, error)`
- `DeleteMute(ctx, actor, mutedActor) error`
- `GetMutedActors(ctx, actor, limit, cursor) ([]*Mute, string, error)`
- `IsMuted(ctx, actor, targetActor) (bool, error)`

### Filter Operations (v2)
- `CreateFilter(ctx, filter) error`
- `GetFilter(ctx, filterID) (*Filter, error)`
- `GetFiltersForUser(ctx, username) ([]*Filter, error)`
- `UpdateFilter(ctx, filterID, updates) error`
- `DeleteFilter(ctx, filterID) error`
- `AddFilterKeyword(ctx, filterID, keyword) error`
- `GetFilterKeywords(ctx, filterID) ([]*FilterKeyword, error)`
- `UpdateFilterKeyword(ctx, keywordID, updates) error`
- `DeleteFilterKeyword(ctx, keywordID) error`
- `AddFilterStatus(ctx, filterID, status) error`
- `GetFilterStatuses(ctx, filterID) ([]*FilterStatus, error)`
- `DeleteFilterStatus(ctx, statusID) error`

### Moderation Operations
- `CreateModerationEvent(ctx, event) error`
- `GetModerationEvent(ctx, eventID) (*ModerationEvent, error)`
- `GetModerationQueuePaginated(ctx, limit, cursor) ([]*ModerationQueueItem, string, error)`
- `GetModerationEventsByObject(ctx, objectID, limit, cursor) ([]*ModerationEvent, string, error)`
- `GetModerationEventsByActor(ctx, actorID, limit, cursor) ([]*ModerationEvent, string, error)`
- `AddModerationReview(ctx, review) error`
- `GetModerationReviews(ctx, eventID) ([]*ModerationReview, error)`
- `CreateModerationDecision(ctx, decision) error`
- `GetModerationDecision(ctx, objectID) (*ModerationDecision, error)`
- `GetModerationHistory(ctx, objectID) (*ModerationHistory, error)`
- `GetModerationEvents(ctx, filter, limit, cursor) ([]*ModerationEvent, string, error)`
- `CreateAdminReview(ctx, eventID, adminID, action, reason) error`
- `GetReviewerStats(ctx, reviewerID) (*ReviewerStats, error)`
- `GetModerationQueue(ctx, filter) ([]*ModerationQueueItem, error)`
- `StoreModerationDecision(ctx, decision) error`
- `UpdateModerationDecision(ctx, contentID, review) error`
- `CreateModerationPattern(ctx, pattern) error`
- `GetModerationPattern(ctx, patternID) (*ModerationPattern, error)`
- `GetModerationPatterns(ctx, active, severity, limit) ([]*ModerationPattern, error)`
- `UpdateModerationPattern(ctx, pattern) error`
- `DeleteModerationPattern(ctx, patternID) error`
- `RecordPatternMatch(ctx, patternID, matched, timestamp) error`

### Trust Operations
- `CreateTrustRelationship(ctx, relationship) error`
- `GetTrustRelationship(ctx, trusterID, trusteeID, category) (*TrustRelationship, error)`
- `UpdateTrustRelationship(ctx, relationship) error`
- `DeleteTrustRelationship(ctx, trusterID, trusteeID, category) error`
- `GetTrustRelationships(ctx, trusterID, limit, cursor) ([]*TrustRelationship, string, error)`
- `GetTrustedByRelationships(ctx, trusteeID, limit, cursor) ([]*TrustRelationship, string, error)`
- `GetTrustScore(ctx, actorID, category) (*TrustScore, error)`
- `UpdateTrustScore(ctx, score) error`
- `RecordTrustUpdate(ctx, update) error`
- `GetAllTrustRelationships(ctx, limit) ([]*TrustRelationship, error)`

### Account Pin Operations (Endorsed Accounts)
- `CreateAccountPin(ctx, pin) error`
- `DeleteAccountPin(ctx, username, pinnedActorID) error`
- `GetAccountPins(ctx, username) ([]*AccountPin, error)`
- `IsAccountPinned(ctx, username, actorID) (bool, error)`
- `CreateAccountNote(ctx, note) error`
- `GetAccountNote(ctx, username, targetActorID) (*AccountNote, error)`
- `UpdateAccountNote(ctx, note) error`
- `DeleteAccountNote(ctx, username, targetActorID) error`

### Status Pinning Operations
- `CreateStatusPin(ctx, pin) error`
- `DeleteStatusPin(ctx, username, statusID) error`
- `GetStatusPins(ctx, username) ([]*StatusPin, error)`
- `IsStatusPinned(ctx, username, statusID) (bool, error)`
- `CountUserPinnedStatuses(ctx, username) (int, error)`

### Conversation Muting Operations
- `CreateConversationMute(ctx, mute) error`
- `DeleteConversationMute(ctx, username, conversationID) error`
- `IsConversationMuted(ctx, username, conversationID) (bool, error)`
- `GetMutedConversations(ctx, username) ([]string, error)`

### Scheduled Status Operations
- `CreateScheduledStatus(ctx, scheduled) error`
- `GetScheduledStatus(ctx, id) (*ScheduledStatus, error)`
- `GetScheduledStatuses(ctx, username, limit, cursor) ([]*ScheduledStatus, string, error)`
- `UpdateScheduledStatus(ctx, scheduled) error`
- `DeleteScheduledStatus(ctx, id) error`
- `GetDueScheduledStatuses(ctx, before, limit) ([]*ScheduledStatus, error)`
- `MarkScheduledStatusPublished(ctx, id) error`

### Hashtag Operations
- `FollowHashtag(ctx, userID, hashtag) error`
- `UnfollowHashtag(ctx, userID, hashtag) error`
- `IsFollowingHashtag(ctx, userID, hashtag) (bool, error)`
- `GetFollowedHashtags(ctx, userID, limit, cursor) ([]string, string, error)`
- `IndexHashtag(ctx, hashtag, statusID, authorID, visibility) error`
- `SearchHashtags(ctx, query, limit) ([]*Hashtag, error)`
- `GetHashtagInfo(ctx, hashtag) (*Hashtag, error)`
- `GetHashtagUsageHistory(ctx, hashtag, days) ([]int64, error)`

### Featured Tags
- `CreateFeaturedTag(ctx, userID, tagName) (*FeaturedTag, error)`
- `DeleteFeaturedTag(ctx, userID, featuredTagID) error`
- `GetFeaturedTags(ctx, userID) ([]*FeaturedTag, error)`
- `GetTagSuggestions(ctx, userID, limit) ([]string, error)`

### Language Detection and User Preferences
- `GetUserLanguagePreference(ctx, username) (string, error)`
- `SetUserLanguagePreference(ctx, username, language) error`
- `GetUserPreferences(ctx, username) (*UserPreferences, error)`
- `UpdateUserPreferences(ctx, username, preferences) error`
- `GetAllPreferences(ctx, username) (map[string]interface{}, error)`
- `SetPreference(ctx, username, key, value) error`
- `GetPreference(ctx, username, key) (interface{}, error)`
- `UpdatePreferences(ctx, username, prefs) error`

### Search Operations
- `SearchStatuses(ctx, query, limit) ([]*StatusSearchResult, error)`
- `SearchStatusesWithOptions(ctx, query, options) ([]*StatusSearchResult, error)`
- `SearchStatusesByURL(ctx, url) (*StatusSearchResult, error)`
- `TrackSearchQuery(ctx, userID, query, resultCount) error`
- `GetPopularSearchQueries(ctx, limit, timeWindow) ([]SearchQueryStats, error)`
- `GetUserSearchHistory(ctx, userID, limit) ([]SearchHistoryEntry, error)`
- `GenerateSearchSuggestions(ctx, userID, partialQuery, limit) ([]string, error)`

### Trending Operations
- `RecordHashtagUsage(ctx, hashtag, statusID, authorID) error`
- `RecordStatusEngagement(ctx, statusID, engagementType, userID) error`
- `RecordLinkShare(ctx, url, statusID, authorID) error`
- `GetTrendingHashtags(ctx, since, limit) ([]*TrendingHashtag, error)`
- `GetTrendingStatuses(ctx, since, limit) ([]*TrendingStatus, error)`
- `GetTrendingLinks(ctx, since, limit) ([]*TrendingLink, error)`

### Announcement Operations
- `CreateAnnouncement(ctx, announcement) error`
- `GetAnnouncement(ctx, id) (*Announcement, error)`
- `GetAnnouncements(ctx, active) ([]*Announcement, error)`
- `UpdateAnnouncement(ctx, announcement) error`
- `DeleteAnnouncement(ctx, id) error`
- `DismissAnnouncement(ctx, username, announcementID) error`
- `IsDismissed(ctx, username, announcementID) (bool, error)`
- `GetDismissedAnnouncements(ctx, username) ([]string, error)`
- `AddAnnouncementReaction(ctx, username, announcementID, emojiName) error`
- `RemoveAnnouncementReaction(ctx, username, announcementID, emojiName) error`
- `GetAnnouncementReactions(ctx, announcementID) (map[string][]string, error)`

### Custom Emoji Operations
- `CreateCustomEmoji(ctx, emoji) error`
- `GetCustomEmoji(ctx, shortcode) (*CustomEmoji, error)`
- `GetCustomEmojis(ctx) ([]*CustomEmoji, error)`
- `UpdateCustomEmoji(ctx, emoji) error`
- `DeleteCustomEmoji(ctx, shortcode) error`
- `GetCustomEmojisByCategory(ctx, category) ([]*CustomEmoji, error)`

### Report Operations
- `CreateReport(ctx, report) error`
- `GetReport(ctx, id) (*Report, error)`
- `GetUserReports(ctx, username, limit, cursor) ([]*Report, string, error)`
- `GetReportsByTarget(ctx, targetAccountID, limit, cursor) ([]*Report, string, error)`
- `GetReportsByStatus(ctx, status, limit, cursor) ([]*Report, string, error)`
- `UpdateReportStatus(ctx, id, status, actionTaken, moderatorID) error`
- `AssignReport(ctx, reportID, assignedTo) error`
- `UnassignReport(ctx, reportID) error`
- `GetReportStats(ctx, username) (*ReportStats, error)`

### Marker Operations
- `GetMarkers(ctx, username, timelines) (map[string]*Marker, error)`
- `SaveMarker(ctx, username, timeline, lastReadID, version) error`

### Domain Block Operations
- `AddDomainBlock(ctx, username, domain) error`
- `RemoveDomainBlock(ctx, username, domain) error`
- `IsBlockedDomain(ctx, username, domain) (bool, error)`
- `GetUserDomainBlocks(ctx, username, limit, cursor) ([]string, string, error)`
- `CreateDomainBlock(ctx, block) error`
- `GetDomainBlock(ctx, id) (*InstanceDomainBlock, error)`
- `GetDomainBlocks(ctx, limit, cursor) ([]*InstanceDomainBlock, string, error)`
- `UpdateDomainBlock(ctx, id, updates) error`
- `DeleteDomainBlock(ctx, id) error`
- `CreateInstanceDomainBlock(ctx, block) error`
- `GetInstanceDomainBlock(ctx, domain) (*InstanceDomainBlock, error)`
- `GetInstanceDomainBlockByID(ctx, id) (*InstanceDomainBlock, error)`
- `ListInstanceDomainBlocks(ctx, limit, cursor) ([]*InstanceDomainBlock, string, error)`
- `UpdateInstanceDomainBlock(ctx, domain, updates) error`
- `DeleteInstanceDomainBlock(ctx, domain) error`
- `IsDomainBlocked(ctx, domain) (bool, *InstanceDomainBlock, error)`
- `IsInstanceDomainBlocked(ctx, domain) (bool, *InstanceDomainBlock, error)`

### Domain Allow Operations
- `CreateDomainAllow(ctx, allow) error`
- `GetDomainAllows(ctx, limit, cursor) ([]*DomainAllow, string, error)`
- `DeleteDomainAllow(ctx, id) error`

### Email Domain Block Operations
- `CreateEmailDomainBlock(ctx, block) error`
- `GetEmailDomainBlocks(ctx, limit, cursor) ([]*EmailDomainBlock, string, error)`
- `DeleteEmailDomainBlock(ctx, id) error`

### Session Operations
- `CreateSession(ctx, session) error`
- `GetSession(ctx, sessionID) (*Session, error)`
- `GetSessionByRefreshToken(ctx, refreshToken) (*Session, error)`
- `UpdateSession(ctx, session) error`
- `DeleteSession(ctx, sessionID) error`
- `GetUserSessions(ctx, username) ([]*Session, error)`

### Device Operations
- `CreateDevice(ctx, device) error`
- `GetDevice(ctx, deviceID) (*Device, error)`
- `UpdateDevice(ctx, device) error`
- `GetUserDevices(ctx, username) ([]*Device, error)`

### WebAuthn Operations
- `StoreWebAuthnChallenge(ctx, challenge) error`
- `GetWebAuthnChallenge(ctx, challengeID) (*WebAuthnChallenge, error)`
- `DeleteWebAuthnChallenge(ctx, challengeID) error`
- `StoreWebAuthnCredential(ctx, credential) error`
- `GetWebAuthnCredential(ctx, credentialID) (*WebAuthnCredential, error)`
- `GetUserWebAuthnCredentials(ctx, username) ([]*WebAuthnCredential, error)`
- `UpdateWebAuthnCredential(ctx, credential) error`
- `DeleteWebAuthnCredential(ctx, credentialID) error`

### Wallet Operations
- `StoreWalletChallenge(ctx, challenge) error`
- `GetWalletChallenge(ctx, challengeID) (*WalletChallenge, error)`
- `DeleteWalletChallenge(ctx, challengeID) error`
- `StoreWalletCredential(ctx, credential) error`
- `GetWalletCredential(ctx, walletType, address) (*WalletCredential, error)`
- `GetUserWalletCredentials(ctx, username) ([]*WalletCredential, error)`
- `UpdateWalletLastUsed(ctx, username, address) error`
- `DeleteWalletCredential(ctx, username, address) error`

### Trustee Operations
- `StoreTrustee(ctx, username, trustee) error`
- `GetTrustees(ctx, username) ([]*TrusteeConfig, error)`
- `UpdateTrusteeConfirmed(ctx, username, trusteeActorID, confirmed) error`
- `DeleteTrustee(ctx, username, trusteeActorID) error`

### Rate Limiting Operations
- `RecordLoginAttempt(ctx, identifier, success) error`
- `GetLoginAttemptCount(ctx, identifier, since) (int, error)`
- `ClearLoginAttempts(ctx, identifier) error`
- `IsRateLimited(ctx, identifier) (bool, time.Time, error)`

### Reputation Operations
- `StoreReputation(ctx, actorID, reputation) error`
- `GetReputation(ctx, actorID) (*Reputation, error)`
- `GetReputationHistory(ctx, actorID, limit) ([]*Reputation, error)`

### Vouch Operations
- `CreateVouch(ctx, vouch) error`
- `GetVouch(ctx, vouchID) (*Vouch, error)`
- `GetVouchesByActor(ctx, actorID, activeOnly) ([]*Vouch, error)`
- `GetVouchesForActor(ctx, actorID, activeOnly) ([]*Vouch, error)`
- `UpdateVouchStatus(ctx, vouchID, active, revokedAt) error`
- `GetMonthlyVouchCount(ctx, actorID, year, month) (int, error)`

### Community Note Operations
- `CreateCommunityNote(ctx, note) error`
- `GetCommunityNote(ctx, noteID) (*CommunityNote, error)`
- `GetVisibleCommunityNotes(ctx, objectID) ([]*CommunityNote, error)`
- `UpdateCommunityNoteScore(ctx, noteID, score, status) error`
- `CreateCommunityNoteVote(ctx, vote) error`
- `GetUserCommunityNoteVotes(ctx, userID, noteIDs) (map[string]*CommunityNoteVote, error)`
- `CheckCommunityNoteRateLimit(ctx, userID, limit) (bool, int, error)`
- `GetCommunityNotesByAuthor(ctx, authorID, limit, cursor) ([]*CommunityNote, string, error)`
- `GetCommunityNoteVotes(ctx, noteID) ([]*CommunityNoteVote, error)`

### DNS Cache Operations
- `GetDNSCache(ctx, hostname) (*DNSCacheEntry, error)`
- `SetDNSCache(ctx, entry) error`

### Quote Operations
- `CreateQuoteRelationship(ctx, quote) error`
- `GetQuotesForNote(ctx, noteID, limit, cursor) ([]*QuoteRelationship, string, error)`
- `IsQuoted(ctx, actorID, noteID) (bool, error)`
- `WithdrawQuote(ctx, quoteNoteID) error`
- `CountQuotes(ctx, noteID) (int, error)`

### Federation Operations
- `RecordFederationActivity(ctx, activity) error`
- `GetFederationStatistics(ctx, startTime, endTime) (*FederationStats, error)`
- `CalculateFederationClusters(ctx) ([]*InstanceCluster, error)`
- `GetFederationCosts(ctx, startTime, endTime, limit, cursor) ([]*FederationCost, string, error)`
- `GetFederationEdges(ctx, domains) ([]*FederationEdge, error)`
- `GetFederationNodes(ctx, depth) ([]*FederationNode, error)`
- `GetInstanceConnections(ctx, domain, connectionType) ([]*InstanceConnection, error)`
- `GetInstanceMetadata(ctx, domain) (*InstanceMetadata, error)`
- `GetInstanceHealthReport(ctx, domain, period) (*InstanceHealthReport, error)`
- `GetKnownInstances(ctx, limit, cursor) ([]*InstanceInfo, string, error)`
- `GetInstanceInfo(ctx, domain) (*InstanceInfo, error)`
- `UpsertInstanceInfo(ctx, info) error`
- `GetRecentInstanceConnections(ctx, domain, since) ([]*InstanceConnection, error)`
- `GetStrongestConnectionsByType(ctx, connectionType, limit) ([]*FederationEdge, error)`
- `StoreFederationTimeSeries(ctx, data) error`
- `StoreInstanceCluster(ctx, cluster) error`
- `UpdateFederationEdge(ctx, edge) error`
- `UpdateFederationNode(ctx, node) error`
- `UpdateInstanceMetadata(ctx, metadata) error`

### Cost Operations
- `GetCostProjections(ctx, period) (*CostProjection, error)`

### Streaming Operations
- `GetStreamingPreferences(ctx, username) (*StreamingPreferences, error)`
- `UpdateStreamingPreferences(ctx, prefs) error`
- `GetStreamingPreferenceHistory(ctx, username, limit) ([]*StreamingPreferences, error)`
- `GetStreamingPreferencesByDevice(ctx, username, deviceID) (*StreamingPreferences, error)`
- `UpdateDeviceStreamingPreferences(ctx, prefs, deviceID) error`
- `SyncStreamingPreferences(ctx, username, sourceDeviceID) error`
- `ResolvePreferenceConflict(ctx, username, strategy) (*StreamingPreferences, error)`

### Reply Operations
- `GetReplies(ctx, objectID, limit, cursor) ([]interface{}, string, error)`
- `CountReplies(ctx, objectID) (int, error)`
- `IncrementReplyCount(ctx, objectID) error`
- `IncrementReblogCount(ctx, objectID) error`

### Status Operations
- `GetStatusCount(ctx, actorID) (int, error)`
- `GetLatestStatus(ctx, actorID) (*StatusSearchResult, error)`
- `IncrementFalseReports(ctx, username) error`

## Recently Added Methods

The following methods were recently added to MockStorage to resolve compilation errors:

### Follow Request Methods
- `RejectFollowRequest(ctx, followerID, targetID) error`
- `HasPendingFollowRequest(ctx, requesterID, targetID) (bool, error)`
- `HasFollowRequest(ctx, requesterID, targetID) (bool, error)`

### Notification Methods
- `IsNotificationEnabled(ctx, userID, targetID) (bool, error)`
- `IsNotificationMuted(ctx, userID, targetID) (bool, error)`

### Moderation Methods
- `GetModerationQueueCount(ctx) (int, error)`
- `GetOpenReportsCount(ctx) (int, error)`

### User and Trust Methods
- `GetUserTrustScore(ctx, userID) (float64, error)`
- `RemoveAccountSuggestion(ctx, userID, targetID) error`
- `IsEndorsed(ctx, userID, targetID) (bool, error)`
- `GetRelationshipNote(ctx, userID, targetID) (*AccountNote, error)`

### Status Methods
- `GetStatusReplyCount(ctx, statusID) (int, error)`
- `GetUserStatusCount(ctx, userID) (int, error)`
- `GetStatus(ctx, statusID) (interface{}, error)`
- `GetScheduledStatusMedia(ctx, statusID) ([]interface{}, error)`
- `GetStatusesByLink(ctx, linkURL, limit) ([]interface{}, error)`

### Storage and Analytics Methods
- `GetStorageUsage(ctx) (interface{}, error)`
- `GetStorageHistory(ctx, days) ([]interface{}, error)`
- `GetUserGrowthHistory(ctx, days) ([]interface{}, error)`

### Trend Methods
- `GetRecentHashtags(ctx, since, limit) ([]*TrendingHashtag, error)`
- `StoreHashtagTrend(ctx, trend) error`
- `GetRecentStatusesWithEngagement(ctx, since, limit) ([]*TrendingStatus, error)`
- `StoreStatusTrend(ctx, trend) error`
- `GetRecentLinks(ctx, since, limit) ([]*TrendingLink, error)`
- `StoreLinkTrend(ctx, trend) error`

### Miscellaneous Methods
- `GetRulesByCategory(ctx, category) ([]InstanceRule, error)`
- `GetUserAppConsent(ctx, userID, appID) (bool, error)`
- `UnmarkAllMediaAsSensitive(ctx, username) error`

All missing methods have been successfully added to the MockStorage implementation.

## Usage

### MockStorage (with testify/mock)
```go
mockStore := &mocks.MockStorage{}
mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
// Use mockStore in tests
```

### BaseMockStorage (simple no-op)
```go
mockStore := &mocks.BaseMockStorage{}
// All methods return zero values/nil/empty slices
```

### TimelineMethods (embedded helper)
```go
type CustomMock struct {
    mocks.TimelineMethods
    // Add other methods as needed
}
``` 