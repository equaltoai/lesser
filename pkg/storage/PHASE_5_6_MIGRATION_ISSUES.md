# Phase 5.6: StorageAdapter Removal - Comprehensive Issues and Migration Plan

## Overview
Phase 5.6 involves completely removing the StorageAdapter layer and migrating all code to use the repository pattern directly. This is a significant architectural change affecting hundreds of files.

## Current State Analysis

### 1. Completed Work
- ✅ All service packages (auth, federation, mastodon, etc.) migrated to RepositoryStorage
- ✅ All Lambda functions migrated to use repositories
- ✅ GraphQL resolver migrated to use repositories
- ✅ MockRepositoryStorage implemented
- ✅ cmd/api/main.go - StorageAdapter initialization removed

### 2. Major Outstanding Issues

#### A. Lift Handler Files (CRITICAL - Hundreds of references)
The cmd/api/lift directory has extensive usage of `h.store` that needs migration to `h.repos`:

**Files with h.store references (partial list):**
- status_pins.go (15+ references)
- trends.go (1 reference) 
- status_interactions.go (6+ references)
- conversations.go (17+ references)
- webfinger.go (1 reference)
- lists.go (15+ references)
- notes.go (8+ references)
- oauth.go (5+ references)
- translation.go (2 references)
- interactions.go (45+ references)
- bookmarks.go (14+ references)
- relationships.go (15+ references)
- discovery.go (8+ references)
- imports.go (2 references)
- follow_requests.go (15+ references)
- accounts.go (30+ references)
- filters.go (20+ references)
- media.go (10+ references)
- mutes.go (15+ references)
- tags.go (25+ references)
- admin.go (50+ references)
- announcements.go (10+ references)
- search.go (5+ references)
- markers.go (5+ references)
- custom_emojis.go (5+ references)
- scheduled_statuses.go (10+ references)
- reports.go (20+ references)
- endorsements.go (10+ references)
- domain_blocks.go (10+ references)
- preferences.go (5+ references)
- push_subscriptions.go (10+ references)
- timelines.go (30+ references)
- statuses.go (50+ references)
- polls.go (10+ references)
- exports.go (5+ references)
- recovery.go (10+ references)
- wallet.go (10+ references)
- webauthn.go (10+ references)
- ai.go (10+ references)
- debug.go (5+ references)
- misc.go (20+ references)
- metrics.go (10+ references)

**Total estimated references: 500+**

#### B. Method Mapping Complexity
The migration isn't a simple find/replace because:

1. **Direct Storage Methods → Repository Methods**
   ```go
   // OLD: Direct method on storage
   h.store.GetActor(ctx, username)
   
   // NEW: Must use specific repository
   h.repos.Actor().GetActor(ctx, username)
   ```

2. **Methods That Don't Exist on Repositories**
   Some methods on storage.Storage may not have direct equivalents on repositories:
   - Methods that span multiple domains
   - Convenience methods that need to be decomposed
   - Methods that were never properly migrated

3. **Different Method Signatures**
   Repository methods may have different signatures than storage methods:
   - Different parameter types
   - Different return types
   - Different error handling

#### C. Type Compatibility Issues
- `storage.Storage` interface methods return different types than repository methods
- Need to handle type conversions throughout the codebase
- Model types may differ between storage and repository layers

#### D. Testing Infrastructure
- Test files still use MockStorage/MockStorageAdapter
- Need to update all test files to use MockRepositoryStorage
- Test utilities may depend on storage.Storage interface

## Migration Strategy

### Phase 5.6.3: Lift Handler Migration (Current Focus)

#### Step 1: Create Method Mapping Documentation
First, create a comprehensive mapping of all storage methods to their repository equivalents:

```markdown
# Storage to Repository Method Mapping

## Actor Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetActor(ctx, username) | Actor().GetActor(ctx, username) | Direct mapping |
| GetActorByID(ctx, id) | Actor().GetActorByID(ctx, id) | Direct mapping |
| CreateActor(ctx, actor) | Actor().CreateActor(ctx, actor) | Direct mapping |
| UpdateActor(ctx, updates) | Actor().UpdateActor(ctx, updates) | Direct mapping |
| GetActorWithMetadata(ctx, username) | Actor().GetActorWithMetadata(ctx, username) | Direct mapping |
| GetActorByNumericID(ctx, id) | Actor().GetByNumericID(ctx, id) | Method name change |

## Object Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetObject(ctx, id) | Object().Get(ctx, id) | Method name change |
| CreateObject(ctx, obj) | Object().Create(ctx, obj) | Method name change |
| UpdateObject(ctx, id, updates) | Object().Update(ctx, id, updates) | Method name change |
| DeleteObject(ctx, id) | Object().Delete(ctx, id) | Method name change |

## Account Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetUser(ctx, username) | Account().GetUser(ctx, username) | Direct mapping |
| CreateUser(ctx, user) | Account().CreateUser(ctx, user) | Direct mapping |
| UpdateUser(ctx, username, updates) | Account().UpdateUser(ctx, username, updates) | Direct mapping |
| AuthenticateUser(ctx, username, password) | Account().AuthenticateUser(ctx, username, password) | Direct mapping |

## Relationship Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| IsFollowing(ctx, follower, following) | Relationship().IsFollowing(ctx, follower, following) | Direct mapping |
| CreateFollow(ctx, follower, following, activityID) | Relationship().CreateFollow(ctx, follower, following, activityID) | Direct mapping |
| RemoveFollow(ctx, follower, following) | Relationship().RemoveFollow(ctx, follower, following) | Direct mapping |
| GetFollowers(ctx, username, limit, cursor) | Relationship().GetFollowers(ctx, username, limit, cursor) | Direct mapping |
| GetFollowing(ctx, username, limit, cursor) | Relationship().GetFollowing(ctx, username, limit, cursor) | Direct mapping |
| GetFollowersCount(ctx, actorID) | Relationship().CountFollowers(ctx, actorID) | Method name change |
| GetFollowingCount(ctx, actorID) | Relationship().CountFollowing(ctx, actorID) | Method name change |

## Timeline Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetHomeTimeline(ctx, userID, limit, cursor) | Timeline().GetHomeTimeline(ctx, userID, limit, cursor) | Direct mapping |
| GetPublicTimeline(ctx, local, limit, cursor) | Timeline().GetPublicTimeline(ctx, local, limit, cursor) | Direct mapping |
| AddToTimeline(ctx, entry) | Timeline().AddEntry(ctx, entry) | Method name change |

## Notification Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetNotifications(ctx, userID, limit, cursor) | Notification().GetNotifications(ctx, userID, limit, cursor) | Direct mapping |
| CreateNotification(ctx, notif) | Notification().Create(ctx, notif) | Method name change |
| MarkNotificationRead(ctx, id) | Notification().MarkAsRead(ctx, id) | Method name change |

## Like/Favorite Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| CreateLike(ctx, like) | Like().Create(ctx, like) | Method name change |
| DeleteLike(ctx, actorID, objectID) | Like().Delete(ctx, actorID, objectID) | Method name change |
| GetLike(ctx, actorID, objectID) | Like().Get(ctx, actorID, objectID) | Method name change |
| CountObjectLikes(ctx, objectID) | Like().CountForObject(ctx, objectID) | Method name change |

## Complex/Multi-Repository Methods
| Storage Method | Repository Path | Notes |
|---------------|-----------------|-------|
| GetStatusesByLink(ctx, url, limit) | Status().GetByLink(ctx, url, limit) | Need Status repository |
| GetActiveUserCount(ctx, days) | Analytics().GetActiveUserCount(ctx, days) | Need Analytics repository |
| GetStorageUsage(ctx) | Analytics().GetStorageUsage(ctx) | Need Analytics repository |
| GetUserPreferences(ctx, username) | Account().GetPreferences(ctx, username) | Part of Account |
```

#### Step 2: Create Migration Script
Create an automated migration script that can handle the bulk of the conversions:

```go
// tools/migrate_storage_to_repos.go
package main

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

type MethodMapping struct {
    OldPattern   string
    NewPattern   string
    RequiresImport bool
}

var methodMappings = []MethodMapping{
    // Actor methods
    {`h\.store\.GetActor\(`, `h.repos.Actor().GetActor(`, false},
    {`h\.store\.GetActorByID\(`, `h.repos.Actor().GetActorByID(`, false},
    {`h\.store\.CreateActor\(`, `h.repos.Actor().CreateActor(`, false},
    {`h\.store\.UpdateActor\(`, `h.repos.Actor().UpdateActor(`, false},
    
    // Object methods
    {`h\.store\.GetObject\(`, `h.repos.Object().Get(`, false},
    {`h\.store\.CreateObject\(`, `h.repos.Object().Create(`, false},
    {`h\.store\.UpdateObject\(`, `h.repos.Object().Update(`, false},
    {`h\.store\.DeleteObject\(`, `h.repos.Object().Delete(`, false},
    
    // Account/User methods
    {`h\.store\.GetUser\(`, `h.repos.Account().GetUser(`, false},
    {`h\.store\.CreateUser\(`, `h.repos.Account().CreateUser(`, false},
    {`h\.store\.UpdateUser\(`, `h.repos.Account().UpdateUser(`, false},
    
    // Add more mappings...
}

func migrateFile(filePath string) error {
    // Read file
    // Apply regex replacements
    // Handle special cases
    // Write file back
}
```

#### Step 3: Manual Review Categories
After automated migration, categorize remaining issues:

1. **Methods requiring decomposition**
   - Complex methods that touch multiple repositories
   - Need to be broken down into multiple repository calls

2. **Methods with no direct mapping**
   - May need to implement new repository methods
   - Or refactor the calling code

3. **Type conversion issues**
   - Different return types between storage and repository
   - Need explicit type conversions

4. **Error handling differences**
   - Repository methods may return different errors
   - Need to update error handling logic

### Phase 5.6.4: GraphQL Dataloader Migration
- Update dataloader to use RepositoryStorage
- Fix any type mismatches
- Update dataloader initialization

### Phase 5.6.5: Test File Migration
- Replace MockStorage with MockRepositoryStorage
- Update test expectations
- Fix test compilation errors

### Phase 5.6.6-7: Cleanup
- Delete StorageAdapter files
- Delete storage.Storage interface
- Remove legacy DynamoDB implementations

### Phase 5.6.8: Final Validation
- Full build verification
- Run all tests
- Integration testing

## Risk Assessment

### High Risk Areas
1. **Lift Handlers** - Core API functionality, hundreds of changes
2. **Complex Methods** - Methods that span multiple domains
3. **Type Safety** - Ensuring all type conversions are correct
4. **Test Coverage** - Maintaining test coverage during migration

### Mitigation Strategies
1. **Incremental Migration** - Migrate one file at a time
2. **Automated Testing** - Run tests after each file migration
3. **Type Checking** - Use Go's type system to catch issues
4. **Code Review** - Review each migrated file carefully

## Time Estimate
- Method Mapping Documentation: 2-3 hours
- Migration Script Development: 3-4 hours
- Automated Migration Run: 1 hour
- Manual Review/Fixes: 8-10 hours per 100 references (40-50 hours total)
- Testing/Validation: 5-10 hours
- **Total: 50-70 hours of work**

## Next Steps
1. Complete the method mapping documentation
2. Develop and test the migration script
3. Run automated migration on a subset of files
4. Validate the approach
5. Complete full migration
6. Extensive testing
7. Cleanup and documentation

## Success Criteria
- All h.store references removed from lift handlers
- All tests passing
- No runtime errors
- Clean separation between layers
- Improved maintainability