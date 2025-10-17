# GraphQL Stub Implementation Plan

## 1. Relationship Mutations

**Files to Modify**:
- `graph/mutation_resolvers_relationships.go`
- `pkg/services/relationships/service.go`
- `pkg/storage/repositories/relationship_repository.go`

### 1.1. `UnfollowActor`

**Resolver Location**: `graph/mutation_resolvers_relationships.go`

**Implementation Steps**:

1.  **Repository Layer** (`relationship_repository.go`):
    *   Create a new method `DeleteFollow(ctx, followerID, followingID) error`.
    *   This method should find and delete the `Follow` record where the `PK` matches `USER#{followerID}` and the `SK` matches `FOLLOWING#{followingID}`.

2.  **Service Layer** (`service.go`):
    *   Create a new method `Unfollow(ctx, followerID, followingID) error`.
    *   This service method will call the `DeleteFollow` repository method.
    *   It should also handle any necessary federation logic, such as sending an `Undo(Follow)` activity.

3.  **GraphQL Resolver** (`mutation_resolvers_relationships.go`):
    *   Implement `UnfollowActor` to call the `Unfollow` service method.
    *   Follow the existing pattern: require auth, get the service from the registry, call the service method, and handle errors.
    *   Return `true` on success.

### 1.2. `UnblockActor`

**Resolver Location**: `graph/mutation_resolvers_relationships.go`

**Implementation Steps**:

1.  **Repository Layer** (`relationship_repository.go`):
    *   Create a new method `DeleteBlock(ctx, blockerID, blockedID) error`.
    *   This method should delete the `Block` record where the `PK` is `USER#{blockerID}` and `SK` is `BLOCKING#{blockedID}`.

2.  **Service Layer** (`service.go`):
    *   Create a new method `Unblock(ctx, blockerID, blockedID) error`.
    *   This method will call the `DeleteBlock` repository method.
    *   It should also handle federation by sending an `Undo(Block)` activity.

3.  **GraphQL Resolver** (`mutation_resolvers_relationships.go`):
    *   Implement `UnblockActor` to call the `Unblock` service method.
    *   Maintain consistency with the `BlockActor` resolver.

### 1.3. `UnmuteActor`

**Resolver Location**: `graph/mutation_resolvers_relationships.go`

**Implementation Steps**:

1.  **Repository Layer** (`relationship_repository.go`):
    *   Create a method `DeleteMute(ctx, muterID, mutedID) error`.
    *   This method will delete the `Mute` record.

2.  **Service Layer** (`service.go`):
    *   Create a method `Unmute(ctx, muterID, mutedID) error`.
    *   This method will call the `DeleteMute` repository method.

3.  **GraphQL Resolver** (`mutation_resolvers_relationships.go`):
    *   Implement `UnmuteActor` to call the `Unmute` service method, following the pattern of the `MuteActor` resolver.

## 2. Account Preferences Mutation

### 2.1. `UpdateStreamingPreferences`

**File to Modify**: `graph/mutation_resolvers_accounts.go`

**Implementation Steps**:

1.  **Repository Layer** (`pkg/storage/repositories/user_repository.go`):
    *   The `UpdatePreferences` method already exists and is suitable for this use case. No changes are needed here.

2.  **GraphQL Resolver** (`mutation_resolvers_accounts.go`):
    *   Implement `UpdateStreamingPreferences` to use the `UserRepository`.
    *   Create a map from the input model (`model.StreamingPreferencesInput`) to a `map[string]interface{}`.
    *   Call `r.Registry.User().UpdatePreferences(ctx, username, preferencesMap)`.
    *   Return the updated `UserPreferences` object.

## 3. Quote-Related Mutation

### 3.1. `WithdrawFromQuotes`

**Files to Modify**:
- `graph/mutation_resolvers_quotes.go`
- `pkg/services/quotes/service.go`
- `pkg/storage/repositories/quote_repository.go`

**Implementation Steps**:

1.  **Repository Layer** (`quote_repository.go`):
    *   Create a method `WithdrawQuotes(ctx, noteID, userID) (int, error)`.
    *   This method should update all quotes of `noteID` created by `userID` to set a `withdrawn` flag to `true`.
    *   It should return the count of withdrawn quotes.

2.  **Service Layer** (`service.go`):
    *   Create a method `WithdrawFromQuotes(ctx, noteID, userID) (*models.Object, int, error)`.
    *   This method will call the `WithdrawQuotes` repository method.
    *   It should also fetch the original note to be returned in the payload.

3.  **GraphQL Resolver** (`mutation_resolvers_quotes.go`):
    *   Implement `WithdrawFromQuotes`.
    *   It should call the `WithdrawFromQuotes` service method.
    *   Construct and return the `WithdrawQuotePayload` with the success status, the original note, and the count of withdrawn quotes.
