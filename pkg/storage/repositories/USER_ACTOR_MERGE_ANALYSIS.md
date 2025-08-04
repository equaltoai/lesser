# User and Actor Repository Merge Analysis

## Current State

### UserRepository (3261 lines)
- Handles USER# keys for authentication/account data
- Also handles ACTOR# keys for ActivityPub data
- Contains methods for:
  - User CRUD (CreateUser, GetUser, UpdateUser, DeleteUser)
  - Authentication (login tracking, password reset)
  - Social features (follows, blocks, mutes, bookmarks)
  - Timeline management
  - Trust/vouch system
  - Account notes/pins
  - Conversation muting

### ActorRepository (704 lines)
- Handles ACTOR# keys for ActivityPub actors
- Contains methods for:
  - Actor CRUD (CreateActor, GetActor, UpdateActor, DeleteActor)
  - Actor search and discovery
  - Account suggestions
  - Remote actor caching
  - Private key management

## Key Overlaps

1. **Both handle ACTOR# keys**
   - UserRepository: Lines 748, 797, 2063, 2103
   - ActorRepository: All operations

2. **Similar operations**
   - User creation vs Actor creation
   - User profiles vs Actor profiles
   - Search functionality

3. **Conceptual overlap**
   - In ActivityPub, a User has an associated Actor
   - User = authentication/account entity
   - Actor = ActivityPub federation entity

## Proposed Merge Strategy

### 1. Create Unified AccountRepository
Combines User (authentication) and Actor (federation) into a single repository since they represent the same entity from different perspectives.

### 2. Key Patterns to Preserve
- USER#{username} -> METADATA (authentication data)
- ACTOR#{username} -> PROFILE (ActivityPub data)
- Maintain all GSI patterns for both

### 3. Method Organization

#### Account Management
- CreateAccount (creates both User and Actor)
- GetAccount (returns combined data)
- UpdateAccount (updates relevant parts)
- DeleteAccount (removes both)

#### Authentication Methods
- GetUserByUsername
- GetUserByEmail
- ValidatePassword
- UpdatePassword
- etc.

#### ActivityPub Methods
- GetActor
- GetActorPrivateKey
- UpdateActorProfile
- CacheRemoteActor
- etc.

#### Social Features
- Follow/Unfollow
- Block/Unblock
- Mute/Unmute
- Bookmarks
- etc.

### 4. Benefits of Merging

1. **Reduced Duplication**
   - Single source of truth for account data
   - Consistent handling of username lookups
   - Shared helper methods

2. **Atomic Operations**
   - Create User + Actor in single transaction
   - Delete ensures both are removed
   - Updates can be coordinated

3. **Simplified API**
   - One repository for all account operations
   - Clear separation of concerns within repository
   - Easier to maintain

### 5. Implementation Plan

#### Phase 1: Create AccountRepository Structure
- Define new repository with BaseRepository
- Organize methods into logical sections
- Plan data model relationships

#### Phase 2: Migrate Core Methods
- Start with CRUD operations
- Migrate authentication methods
- Move ActivityPub methods

#### Phase 3: Migrate Social Features
- Move follow/block/mute operations
- Migrate timeline methods
- Transfer trust/vouch system

#### Phase 4: Update Dependencies
- Update Handler to use AccountRepository
- Update factory to provide AccountRepository
- Remove old repositories

## Challenges

1. **Size**: Combined repository will be ~4000 lines
2. **Testing**: Need to ensure all tests still pass
3. **Dependencies**: Many handlers depend on these repositories
4. **Transactions**: Some operations need atomicity

## Recommendation

Proceed with merge but organize code into logical sections:
- account_repository_auth.go (authentication methods)
- account_repository_actor.go (ActivityPub methods)  
- account_repository_social.go (follows, blocks, etc.)
- account_repository_timeline.go (timeline operations)
- account_repository.go (core CRUD and shared methods)