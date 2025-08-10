# Complete Storage to Repository Method Mapping

This document provides a comprehensive mapping of all storage.Storage interface methods to their repository equivalents.

## Actor Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetActor(ctx, username)` | `Actor().GetActor(ctx, username)` | ✅ Direct | |
| `GetActorByID(ctx, id)` | `Actor().GetActorByID(ctx, id)` | ✅ Direct | |
| `GetActorByNumericID(ctx, numID)` | `Actor().GetByNumericID(ctx, numID)` | ⚠️ Renamed | |
| `GetActorWithMetadata(ctx, username)` | `Actor().GetActorWithMetadata(ctx, username)` | ✅ Direct | |
| `CreateActor(ctx, actor)` | `Actor().CreateActor(ctx, actor)` | ✅ Direct | |
| `UpdateActor(ctx, username, updates)` | `Actor().UpdateActor(ctx, username, updates)` | ✅ Direct | |
| `UpdateActorLastStatusTime(ctx, actorID, time)` | `Actor().UpdateLastStatusTime(ctx, actorID, time)` | ⚠️ Renamed | |
| `SetActorFields(ctx, username, fields)` | `Actor().SetFields(ctx, username, fields)` | ⚠️ Renamed | |
| `SearchActors(ctx, query, limit, offset)` | `Actor().Search(ctx, query, limit, offset)` | ⚠️ Renamed | |

## Object/Status Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetObject(ctx, id)` | `Object().Get(ctx, id)` | ⚠️ Renamed | |
| `CreateObject(ctx, obj)` | `Object().Create(ctx, obj)` | ⚠️ Renamed | |
| `UpdateObject(ctx, id, updates)` | `Object().Update(ctx, id, updates)` | ⚠️ Renamed | |
| `DeleteObject(ctx, id)` | `Object().Delete(ctx, id)` | ⚠️ Renamed | |
| `GetObjectsByActor(ctx, actorID, limit, cursor)` | `Object().GetByActor(ctx, actorID, limit, cursor)` | ⚠️ Renamed | |
| `GetStatusesByLink(ctx, url, limit)` | `Status().GetByLink(ctx, url, limit)` | 🔄 Different Repo | |
| `CountObjectLikes(ctx, objectID)` | `Like().CountForObject(ctx, objectID)` | 🔄 Different Repo | |
| `CountObjectAnnounces(ctx, objectID)` | `Object().CountAnnounces(ctx, objectID)` | ⚠️ Renamed | |
| `GetObjectLikes(ctx, objectID, limit, cursor)` | `Like().GetForObject(ctx, objectID, limit, cursor)` | 🔄 Different Repo | |
| `GetObjectAnnounces(ctx, objectID, limit, cursor)` | `Object().GetAnnounces(ctx, objectID, limit, cursor)` | ⚠️ Renamed | |

## Account/User Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetUser(ctx, username)` | `Account().GetUser(ctx, username)` | ✅ Direct | |
| `GetUserByEmail(ctx, email)` | `Account().GetUserByEmail(ctx, email)` | ✅ Direct | |
| `CreateUser(ctx, user)` | `Account().CreateUser(ctx, user)` | ✅ Direct | |
| `UpdateUser(ctx, username, updates)` | `Account().UpdateUser(ctx, username, updates)` | ✅ Direct | |
| `DeleteUser(ctx, username)` | `Account().DeleteUser(ctx, username)` | ✅ Direct | |
| `AuthenticateUser(ctx, username, password)` | `Account().AuthenticateUser(ctx, username, password)` | ✅ Direct | |
| `GetUserPreferences(ctx, username)` | `Account().GetPreferences(ctx, username)` | ⚠️ Renamed | |
| `UpdateUserPreferences(ctx, username, prefs)` | `Account().UpdatePreferences(ctx, username, prefs)` | ⚠️ Renamed | |
| `GetActiveUserCount(ctx, days)` | `Analytics().GetActiveUserCount(ctx, days)` | 🔄 Different Repo | |
| `GetTotalUserCount(ctx)` | `Analytics().GetTotalUserCount(ctx)` | 🔄 Different Repo | |

## Relationship Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `IsFollowing(ctx, follower, following)` | `Relationship().IsFollowing(ctx, follower, following)` | ✅ Direct | |
| `CreateFollow(ctx, follower, following, activityID)` | `Relationship().CreateFollow(ctx, follower, following, activityID)` | ✅ Direct | |
| `RemoveFollow(ctx, follower, following)` | `Relationship().RemoveFollow(ctx, follower, following)` | ✅ Direct | |
| `AcceptFollow(ctx, follower, following)` | `Relationship().AcceptFollow(ctx, follower, following)` | ✅ Direct | |
| `GetFollowers(ctx, username, limit, cursor)` | `Relationship().GetFollowers(ctx, username, limit, cursor)` | ✅ Direct | |
| `GetFollowing(ctx, username, limit, cursor)` | `Relationship().GetFollowing(ctx, username, limit, cursor)` | ✅ Direct | |
| `GetFollowersCount(ctx, username)` | `Relationship().CountFollowers(ctx, username)` | ✅ Implemented | Takes username not actorID |
| `GetFollowingCount(ctx, username)` | `Relationship().CountFollowing(ctx, username)` | ✅ Implemented | Takes username not actorID |
| `HasFollowRequest(ctx, follower, following)` | `Relationship().HasFollowRequest(ctx, follower, following)` | ✅ Direct | |
| `GetFollowRequest(ctx, follower, following)` | `Relationship().GetFollowRequest(ctx, follower, following)` | ✅ Direct | |
| `GetPendingFollowRequests(ctx, username, limit, cursor)` | `Relationship().GetPendingFollowRequests(ctx, username, limit, cursor)` | ✅ Direct | |
| `AcceptFollowRequest(ctx, follower, following)` | `Relationship().AcceptFollowRequest(ctx, follower, following)` | ✅ Direct | |
| `RejectFollowRequest(ctx, follower, following)` | `Relationship().RejectFollowRequest(ctx, follower, following)` | ✅ Direct | |

## Block Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateBlock(ctx, block)` | `Relationship().CreateBlock(ctx, block)` | ✅ Direct | |
| `DeleteBlock(ctx, blocker, blocked)` | `Relationship().DeleteBlock(ctx, blocker, blocked)` | ✅ Direct | |
| `GetBlock(ctx, blocker, blocked)` | `Relationship().GetBlock(ctx, blocker, blocked)` | ✅ Direct | |
| `GetBlockedActors(ctx, actorID, limit, cursor)` | `Relationship().GetBlockedActors(ctx, actorID, limit, cursor)` | ✅ Direct | |

## Mute Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateMute(ctx, mute)` | `Relationship().CreateMute(ctx, mute)` | ✅ Direct | |
| `DeleteMute(ctx, muter, muted)` | `Relationship().DeleteMute(ctx, muter, muted)` | ✅ Direct | |
| `GetMute(ctx, muter, muted)` | `Relationship().GetMute(ctx, muter, muted)` | ✅ Direct | |
| `GetMutedActors(ctx, username, limit, cursor)` | `Relationship().GetMutedActors(ctx, username, limit, cursor)` | ✅ Direct | |

## List Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateList(ctx, owner, title, repliesPolicy)` | `List().Create(ctx, owner, title, repliesPolicy)` | ⚠️ Renamed | |
| `GetList(ctx, listID)` | `List().Get(ctx, listID)` | ⚠️ Renamed | |
| `UpdateList(ctx, listID, updates)` | `List().Update(ctx, listID, updates)` | ⚠️ Renamed | |
| `DeleteList(ctx, listID)` | `List().Delete(ctx, listID)` | ⚠️ Renamed | |
| `GetListsForUser(ctx, username)` | `List().GetForUser(ctx, username)` | ⚠️ Renamed | |
| `GetListAccounts(ctx, listID)` | `List().GetAccounts(ctx, listID)` | ⚠️ Renamed | |
| `AddAccountsToList(ctx, listID, accountIDs)` | `List().AddAccounts(ctx, listID, accountIDs)` | ⚠️ Renamed | |
| `RemoveAccountsFromList(ctx, listID, accountIDs)` | `List().RemoveAccounts(ctx, listID, accountIDs)` | ⚠️ Renamed | |

## Timeline Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetHomeTimeline(ctx, userID, limit, cursor)` | `Timeline().GetHomeTimeline(ctx, userID, limit, cursor)` | ✅ Direct | |
| `GetPublicTimeline(ctx, local, limit, cursor)` | `Timeline().GetPublicTimeline(ctx, local, limit, cursor)` | ✅ Direct | |
| `GetHashtagTimeline(ctx, tag, limit, cursor)` | `Timeline().GetHashtagTimeline(ctx, tag, limit, cursor)` | ✅ Direct | |
| `GetListTimeline(ctx, listID, limit, cursor)` | `Timeline().GetListTimeline(ctx, listID, limit, cursor)` | ✅ Direct | |
| `AddToTimeline(ctx, entry)` | `Timeline().AddEntry(ctx, entry)` | ⚠️ Renamed | |
| `RemoveFromTimeline(ctx, userID, statusID)` | `Timeline().RemoveEntry(ctx, userID, statusID)` | ⚠️ Renamed | |

## Notification Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateNotification(ctx, notif)` | `Notification().Create(ctx, notif)` | ⚠️ Renamed | |
| `GetNotifications(ctx, userID, types, limit, cursor)` | `Notification().GetNotifications(ctx, userID, types, limit, cursor)` | ✅ Direct | |
| `MarkNotificationRead(ctx, notifID)` | `Notification().MarkAsRead(ctx, notifID)` | ⚠️ Renamed | |
| `MarkAllNotificationsRead(ctx, userID)` | `Notification().MarkAllAsRead(ctx, userID)` | ⚠️ Renamed | |
| `DeleteNotification(ctx, notifID)` | `Notification().Delete(ctx, notifID)` | ⚠️ Renamed | |
| `GetUnreadNotificationCount(ctx, userID)` | `Notification().CountUnread(ctx, userID)` | ⚠️ Renamed | |

## Like/Favorite Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateLike(ctx, like)` | `Like().Create(ctx, like)` | ⚠️ Renamed | |
| `DeleteLike(ctx, actorID, objectID)` | `Like().Delete(ctx, actorID, objectID)` | ⚠️ Renamed | |
| `GetLike(ctx, actorID, objectID)` | `Like().Get(ctx, actorID, objectID)` | ⚠️ Renamed | |
| `GetLikedObjects(ctx, actorID, limit, cursor)` | `Like().GetLikedObjects(ctx, actorID, limit, cursor)` | ✅ Direct | |

## Bookmark Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateBookmark(ctx, userID, objectID)` | `Account().AddBookmark(ctx, userID, objectID)` | ⚠️ Renamed | |
| `RemoveBookmark(ctx, userID, objectID)` | `Account().RemoveBookmark(ctx, userID, objectID)` | ✅ Direct | |
| `GetBookmarks(ctx, userID, limit, cursor)` | `Account().GetBookmarks(ctx, userID, limit, cursor)` | ✅ Direct | |
| `IsBookmarked(ctx, userID, objectID)` | `Account().IsBookmarked(ctx, userID, objectID)` | ✅ Direct | |

## Conversation Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetConversation(ctx, convID)` | `Conversation().Get(ctx, convID)` | ⚠️ Renamed | |
| `GetUserConversations(ctx, userID, limit, cursor)` | `Conversation().GetForUser(ctx, userID, limit, cursor)` | ⚠️ Renamed | |
| `CreateConversation(ctx, conv)` | `Conversation().Create(ctx, conv)` | ⚠️ Renamed | |
| `DeleteConversation(ctx, convID)` | `Conversation().Delete(ctx, convID)` | ⚠️ Renamed | |
| `MarkConversationRead(ctx, convID, userID)` | `Conversation().MarkAsRead(ctx, convID, userID)` | ⚠️ Renamed | |
| `CreateConversationMute(ctx, mute)` | `Conversation().Mute(ctx, mute)` | ⚠️ Renamed | |
| `DeleteConversationMute(ctx, userID, convID)` | `Conversation().Unmute(ctx, userID, convID)` | ⚠️ Renamed | |
| `IsConversationMuted(ctx, userID, convID)` | `Conversation().IsMuted(ctx, userID, convID)` | ⚠️ Renamed | |

## Media Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateMedia(ctx, media)` | `Media().Create(ctx, media)` | ⚠️ Renamed | |
| `GetMedia(ctx, mediaID)` | `Media().Get(ctx, mediaID)` | ⚠️ Renamed | |
| `UpdateMedia(ctx, mediaID, updates)` | `Media().Update(ctx, mediaID, updates)` | ⚠️ Renamed | |
| `DeleteMedia(ctx, mediaID)` | `Media().Delete(ctx, mediaID)` | ⚠️ Renamed | |
| `GetMediaByUser(ctx, userID, limit, cursor)` | `Media().GetByUser(ctx, userID, limit, cursor)` | ⚠️ Renamed | |

## OAuth Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetOAuthApp(ctx, clientID)` | `Account().GetOAuthApp(ctx, clientID)` | ✅ Direct | |
| `SaveOAuthState(ctx, state)` | `Account().SaveOAuthState(ctx, state)` | ✅ Direct | |
| `GetOAuthState(ctx, state)` | `Account().GetOAuthState(ctx, state)` | ✅ Direct | |
| `CreateAuthorizationCode(ctx, code)` | `Account().CreateAuthorizationCode(ctx, code)` | ✅ Direct | |
| `GetUserAppConsent(ctx, userID, clientID)` | `Account().GetUserAppConsent(ctx, userID, clientID)` | ✅ Direct | |

## Activity Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateActivity(ctx, activity)` | `Activity().Create(ctx, activity)` | ⚠️ Renamed | |
| `GetActivity(ctx, activityID)` | `Activity().Get(ctx, activityID)` | ⚠️ Renamed | |
| `GetActivitiesByActor(ctx, actorID, limit, cursor)` | `Activity().GetByActor(ctx, actorID, limit, cursor)` | ⚠️ Renamed | |

## Search Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `SearchAccounts(ctx, query, limit, following, offset)` | `Search().SearchAccounts(ctx, query, limit, following, offset)` | ✅ Direct | |
| `SearchStatuses(ctx, query, accountID, limit, offset)` | `Search().SearchStatuses(ctx, query, accountID, limit, offset)` | ✅ Direct | |
| `SearchHashtags(ctx, query, limit, offset)` | `Search().SearchHashtags(ctx, query, limit, offset)` | ✅ Direct | |

## Community Note Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `CreateCommunityNote(ctx, note)` | `CommunityNote().Create(ctx, note)` | ⚠️ Renamed | |
| `GetCommunityNote(ctx, noteID)` | `CommunityNote().Get(ctx, noteID)` | ⚠️ Renamed | |
| `GetVisibleCommunityNotes(ctx, objectID)` | `CommunityNote().GetVisible(ctx, objectID)` | ⚠️ Renamed | |
| `CreateCommunityNoteVote(ctx, vote)` | `CommunityNote().CreateVote(ctx, vote)` | ⚠️ Renamed | |
| `GetCommunityNotesByAuthor(ctx, authorID, limit, cursor)` | `CommunityNote().GetByAuthor(ctx, authorID, limit, cursor)` | ⚠️ Renamed | |
| `CheckCommunityNoteRateLimit(ctx, userID, limit)` | `CommunityNote().CheckRateLimit(ctx, userID, limit)` | ⚠️ Renamed | |

## Instance/Federation Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetInstance(ctx, domain)` | `Instance().Get(ctx, domain)` | ⚠️ Renamed | |
| `CreateInstance(ctx, instance)` | `Instance().Create(ctx, instance)` | ⚠️ Renamed | |
| `UpdateInstance(ctx, domain, updates)` | `Instance().Update(ctx, domain, updates)` | ⚠️ Renamed | |
| `GetBlockedInstances(ctx, limit, cursor)` | `Instance().GetBlocked(ctx, limit, cursor)` | ⚠️ Renamed | |

## Special/Complex Methods

| Storage Method | Repository Path | Status | Notes |
|----------------|-----------------|---------|--------|
| `GetStatusCount(ctx, authorID)` | `Status().CountStatusesByAuthor(ctx, authorID)` | ✅ Implemented | |
| `RecordStatusEngagement(ctx, statusID, type, actorID)` | `Analytics().RecordEngagement(ctx, statusID, type, actorID)` | 🔄 Different Repo | |
| `GetStorageUsage(ctx)` | `Analytics().GetStorageUsage(ctx)` | 🔄 Different Repo | |
| `GetStorageHistory(ctx, days)` | `Analytics().GetStorageHistory(ctx, days)` | 🔄 Different Repo | |
| `GetUserGrowthHistory(ctx, days)` | `Analytics().GetUserGrowthHistory(ctx, days)` | 🔄 Different Repo | |
| `GetAccountSuggestions(ctx, userID, limit)` | `Social().GetSuggestions(ctx, userID, limit)` | 🔄 Different Repo | |
| `RemoveAccountSuggestion(ctx, userID, suggestionID)` | `Social().RemoveSuggestion(ctx, userID, suggestionID)` | 🔄 Different Repo | |

## Methods With Non-Obvious Locations

These methods exist in repositories but are not where you might expect:

1. **Announce/Reblog Methods** - ✅ Implemented in SocialRepository
   - `GetAnnounce(ctx, actorID, objectID)` → `Social().GetAnnounce(ctx, actorID, objectID)`
   - `DeleteAnnounce(ctx, actorID, objectID)` → `Social().DeleteAnnounce(ctx, actorID, objectID)`

2. **Status Pin Methods** - ✅ Implemented in SocialRepository
   - `CreateStatusPin(ctx, pin)` → `Social().CreateStatusPin(ctx, pin)`
   - `DeleteStatusPin(ctx, userID, objectID)` → `Social().DeleteStatusPin(ctx, userID, objectID)`
   - `GetStatusPins(ctx, userID)` → `Social().GetStatusPins(ctx, userID)`

3. **Endorsement Methods** - ✅ Implemented
   - `IsEndorsed(ctx, endorser, endorsed)` → `Relationship().IsEndorsed(ctx, endorser, endorsed)`
   - `CreateEndorsement(ctx, endorsement)` → `Relationship().CreateEndorsement(ctx, endorsement)`
   - `DeleteEndorsement(ctx, endorser, endorsed)` → `Relationship().DeleteEndorsement(ctx, endorser, endorsed)`
   - `GetEndorsements(ctx, userID, limit, cursor)` → `Relationship().GetEndorsements(ctx, userID, limit, cursor)`

4. **Account Note Methods** - ✅ Implemented (with different names)
   - `GetAccountNote(ctx, owner, target)` → `User().GetAccountNote(ctx, owner, target)` OR `Social().GetAccountNote(ctx, owner, target)`
   - `SetAccountNote(ctx, owner, target, note)` → Use `User().UpdateAccountNote(ctx, owner, target, note)` OR `Social().UpdateAccountNote(ctx, owner, target, note)`

## Methods Still Need Work

1. **Endorsement Methods** - ✅ Implemented in RelationshipRepository:
   - `CreateEndorsement(ctx, endorsement)` - ✅ Implemented with follow validation and limit enforcement
   - `DeleteEndorsement(ctx, endorser, endorsed)` - ✅ Implemented  
   - `GetEndorsements(ctx, userID, limit, cursor)` - ✅ Implemented with pagination

2. **Parameter Type Mismatches** - Methods exist but with different parameter types:
   - Count methods in lift handlers expect actorID but repositories use username
   - Need to convert actorID to username before calling repository methods

## Legend
- ✅ Direct: Method exists with same name and signature
- ⚠️ Renamed: Method exists but with different name
- 🔄 Different Repo: Method exists in a different repository
- ❌ Missing: Method needs to be implemented