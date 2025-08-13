# Handler Integration Index - Complete Migration Plan

## Current Status Overview

**Total Handlers**: 84 across 73 files
- ✅ **Fully Integrated**: 18 handlers (service-first architecture)
- ❌ **Needs Integration**: 58 handlers (business logic violations)
- ⚠️ **Partial/Review**: 8 handlers

## Phase 1: CRITICAL - Legacy Migration (HIGH PRIORITY)

**These handlers have MAJOR business logic violations and must be migrated first**

### 🚨 accounts.go (14 handlers) - **URGENT**
**Current**: Direct ActivityPub creation, federation logic, manual S3 operations
**Target**: Use `accounts_full.go` patterns with `h.registry.Accounts()`
**Handlers**:
- `HandleRegistrationLift()` → Use Accounts service registration
- `HandleVerifyCredentialsLift()` → Replace with `accounts_full.go` version
- `HandleUpdateCredentialsLift()` → Replace with `accounts_full.go` version  
- `HandleGetAccountLift()` → Replace with `accounts_full.go` version
- `HandleAccountLookupLift()` → Use Accounts service
- `HandleGetAccountFollowersLift()` → Replace with `accounts_full.go` version
- `HandleGetAccountFollowingLift()` → Replace with `accounts_full.go` version
- `HandleGetFamiliarFollowersLift()` → Use Relationships service
- `HandlePinAccountLift()` → Use Accounts/Social service
- `HandleUnpinAccountLift()` → Use Accounts/Social service
- `HandleSetAccountNoteLift()` → Use Social service
- `HandleRemoveFromFollowersLift()` → Use Relationships service
- `HandleActivityPubFollowersLift()` → ActivityPub generation (check if needed)
- `HandleActivityPubFollowingLift()` → ActivityPub generation (check if needed)

### 🚨 statuses.go (6 handlers) - **URGENT**
**Current**: Direct ActivityPub creation, federation delivery, streaming violations
**Target**: Use `statuses_full.go` patterns with `h.registry.Notes()`
**Handlers**:
- `HandleCreateStatusLift()` → Replace with `statuses_full.go` version
- `HandleDeleteStatusLift()` → Replace with `statuses_full.go` version
- `HandleUpdateStatusLift()` → Use Notes service UpdateNote
- `HandleGetStatusLift()` → Replace with `statuses_full.go` version
- `HandleGetStatusContextLift()` → Use Notes service for thread building
- `HandleGetAccountStatusesLift()` → Replace with `accounts_full.go` version

### 🚨 interactions.go (9 handlers) - **URGENT**
**Current**: Federation delivery, ActivityPub creation, manual relationship logic
**Target**: Use `h.registry.Relationships()` and `h.registry.Notes()`
**Handlers**:
- `HandleFollowLift()` → Replace with `relationships_full.go` version
- `HandleUnfollowLift()` → Replace with `relationships_full.go` version
- `HandleBlockLift()` → Replace with `relationships_full.go` version
- `HandleUnblockLift()` → Replace with `relationships_full.go` version
- `HandleGetBlocksLift()` → Use Relationships service
- `HandleFavoriteLift()` → Use Notes service (like/unlike)
- `HandleUnfavoriteLift()` → Use Notes service
- `HandleReblogLift()` → Use Notes service (boost/unboost)
- `HandleUnreblogLift()` → Use Notes service

### 🚨 relationships.go (1 handler) - **URGENT**
**Current**: Manual relationship building, multiple repo calls
**Target**: Use `relationships_full.go` patterns
**Handlers**:
- `HandleGetRelationshipsLift()` → Replace with `relationships_full.go` version

## Phase 2: Standard Mastodon API (MEDIUM PRIORITY)

**Core Mastodon API handlers that need service integration**

### lists.go (8 handlers)
**Service**: `h.registry.Lists()`
**Handlers**:
- `HandleGetListsLift()` → Use Lists service ListLists
- `HandleCreateListLift()` → Use Lists service CreateList
- `HandleGetListLift()` → Use Lists service GetList
- `HandleUpdateListLift()` → Use Lists service UpdateList
- `HandleDeleteListLift()` → Use Lists service DeleteList
- `HandleGetListAccountsLift()` → Use Lists service GetListMembers
- `HandleAddAccountsToListLift()` → Use Lists service AddToList
- `HandleRemoveAccountsFromListLift()` → Use Lists service RemoveFromList

### media.go (3 handlers)
**Service**: `h.registry.Media()`
**Handlers**:
- `HandleUploadMediaLift()` → Use Media service UploadMedia
- `HandleGetMediaLift()` → Use Media service GetMedia
- `HandleUpdateMediaLift()` → Use Media service UpdateMedia

### misc.go (4 notification handlers)
**Service**: `h.registry.Notifications()`
**Handlers**:
- `HandleGetNotificationsLift()` → Use Notifications service ListNotifications
- `HandleGetNotificationLift()` → Use Notifications service GetNotification  
- `HandleClearNotificationsLift()` → Use Notifications service Clear
- `HandleDismissNotificationLift()` → Use Notifications service MarkAsRead

### conversations.go (1 remaining handler)
**Service**: `h.registry.Conversations()`
**Handlers**:
- `HandleDeleteConversationLift()` → Needs service method or direct repo (acceptable)

## Phase 3: Lesser Extensions (LOW PRIORITY)

**Lesser-specific features that can be handled later**

### timelines.go (1 handler)
- `HandleGetHomeTimelineLift()` → Complex timeline generation (custom approach)

### search.go (1 handler)  
- `HandleAccountSearchLift()` → May need Search service or direct repo

### apps.go (7 handlers)
- App registration and management (mostly working, check for violations)

### Various Extensions (40+ handlers)
- **reputation.go** (7 handlers) - Web of trust system
- **debug.go** (12 handlers) - Debug utilities  
- **moderation.go** (4 handlers) - Moderation tools
- **webauthn.go** (6 handlers) - WebAuthn authentication
- **trends.go** (5 handlers) - Trending content
- **translation.go** (2 handlers) - Content translation
- **And many others...** - Lesser-specific features

## ✅ Already Completed (18 handlers)

### accounts_full.go (6 handlers) ✅
- All using `h.registry.Accounts()` service properly

### relationships_full.go (7 handlers) ✅  
- All using `h.registry.Relationships()` service properly

### statuses_full.go (3 handlers) ✅
- All using `h.registry.Notes()` service properly

### conversations.go (2 handlers) ✅
- Using `h.registry.Conversations()` service properly

## Implementation Strategy

### Immediate Actions (Phase 1)
1. **accounts.go** → Create service-based replacements following `accounts_full.go` pattern
2. **statuses.go** → Create service-based replacements following `statuses_full.go` pattern
3. **interactions.go** → Integrate with existing services (Relationships/Notes)
4. **relationships.go** → Replace with `relationships_full.go` pattern

### Success Criteria for Each Handler
- ✅ Handler reduced to <25 lines
- ✅ Uses `h.authenticateWithScope()` pattern
- ✅ Calls service via `h.registry.ServiceName()`
- ✅ No direct repository calls for core business logic
- ✅ No ActivityPub object creation
- ✅ No federation delivery logic
- ✅ No manual streaming events
- ✅ Compiles successfully

### Validation Commands
```bash
# Check for business logic violations
grep -r "activitypub\.\|federation\.\|streamQueue\." cmd/api/lift/*.go

# Count service integrations
grep -r "registry\." cmd/api/lift/*.go | wc -l

# Verify compilation
go build ./cmd/api/lift/
```

## Next Steps

1. **Start with accounts.go** - Most critical violations
2. **Use lift-dynamorm-expert agent** with precise task instructions
3. **Follow established patterns** from `*_full.go` files
4. **Test each handler individually** before moving to next
5. **Update routing** to use migrated handlers
6. **Remove legacy handlers** once migration complete

This systematic approach will eliminate all business logic violations and complete the service-first architecture migration.