# Storage Layer Consolidation Plan

## Overview
This document outlines the step-by-step plan to eliminate ~20,000 lines of duplicated code in the storage layer by removing the StorageAdapter and consolidating repositories.

## Execution Strategy
- Each task will be implemented using the lift-dynamorm-expert agent
- After EVERY task, we will verify:
  1. `go build ./...` - No compilation errors
  2. `make test` - All tests pass
  3. `make lint` - No linting issues
- We will NOT proceed to the next task if there are any failures

## Phase 1: Eliminate StorageAdapter (Highest Priority)
Goal: Remove the 10,057-line StorageAdapter by using repositories directly

### Task 1.1: Create Repository Factory and Interfaces
**Files to create/modify:**
- `/pkg/storage/factory.go` - New repository factory
- `/pkg/storage/repositories/interfaces.go` - Consolidated repository interfaces

**Implementation:**
```go
// factory.go
type RepositoryFactory struct {
    db *core.DynamoClient
    logger *zap.Logger
    
    // Repository instances
    actorRepo      ActorRepository
    authRepo       AuthRepository
    objectRepo     ObjectRepository
    // ... etc
}

func NewRepositoryFactory(db *core.DynamoClient, logger *zap.Logger) *RepositoryFactory {
    return &RepositoryFactory{
        db:         db,
        logger:     logger,
        actorRepo:  NewActorRepository(db, logger),
        authRepo:   NewAuthRepository(db, logger),
        objectRepo: NewObjectRepository(db, logger),
        // ... initialize all repositories
    }
}
```

**Verification:**
```bash
go build ./pkg/storage/...
```

### Task 1.2: Migrate Auth Endpoints
**Files to modify:**
- `/cmd/api/lift/auth.go`
- `/cmd/auth/main.go`
- `/cmd/auth-api/main.go`

**Changes:**
1. Replace `storage.Storage` with `*storage.RepositoryFactory`
2. Update all auth handler calls from `s.storage.CreateUser()` to `s.repos.AuthRepo().CreateUser()`
3. Update OAuth flows to use repositories directly

**Verification:**
```bash
go build ./cmd/api/... ./cmd/auth/... ./cmd/auth-api/...
JWT_SECRET=test-secret go test ./cmd/api/lift/auth_test.go -v
```

### Task 1.3: Migrate Actor/Account Endpoints
**Files to modify:**
- `/cmd/api/lift/actors.go`
- `/cmd/api/lift/accounts.go`
- `/cmd/api/lift/profiles.go`

**Changes:**
1. Replace storage calls with direct repository calls
2. Update actor lookups: `s.storage.GetActor()` → `s.repos.ActorRepo().GetActorByUsername()`
3. Update account operations similarly

**Verification:**
```bash
go build ./cmd/api/...
JWT_SECRET=test-secret go test ./cmd/api/lift/*actor* -v
```

### Task 1.4: Migrate Object/Status Endpoints
**Files to modify:**
- `/cmd/api/lift/statuses.go`
- `/cmd/api/lift/objects.go`
- `/cmd/processor-note/main.go`

**Changes:**
1. Update status CRUD operations
2. Replace `s.storage.CreateObject()` → `s.repos.ObjectRepo().CreateObject()`
3. Update timeline operations

**Verification:**
```bash
go build ./cmd/api/... ./cmd/processor-note/...
make test-api
```

### Task 1.5: Migrate Timeline Endpoints
**Files to modify:**
- `/cmd/api/lift/timelines.go`
- `/cmd/timeline-fanout/main.go`

**Changes:**
1. Update timeline queries to use repositories
2. Replace complex timeline operations
3. Ensure streaming still works

**Verification:**
```bash
go build ./cmd/api/... ./cmd/timeline-fanout/...
make test-load  # Run k6 timeline tests
```

### Task 1.6: Migrate Federation Endpoints
**Files to modify:**
- `/cmd/inbox/main.go`
- `/cmd/outbox/main.go`
- `/cmd/api/lift/federation.go`

**Changes:**
1. Update ActivityPub handlers
2. Replace federation storage calls
3. Ensure signature verification still works

**Verification:**
```bash
go build ./cmd/inbox/... ./cmd/outbox/...
make test-federation
```

### Task 1.7: Complete Migration and Remove StorageAdapter
**Files to modify:**
- All remaining files using StorageAdapter
- Delete `/pkg/storage/dynamorm/adapter.go`

**Verification:**
```bash
# Ensure no references remain
grep -r "StorageAdapter" ./pkg ./cmd | wc -l  # Should be 0
go build ./...
make test
```

## Phase 2: Create Base Repository Pattern

### Task 2.1: Create BaseRepository
**Files to create:**
- `/pkg/storage/repositories/base_repository.go`

**Implementation:**
```go
type BaseRepository[T any] struct {
    db     *core.DynamoClient
    logger *zap.Logger
}

func (r *BaseRepository[T]) GetByKey(ctx context.Context, pk, sk string, model T) error {
    return r.db.WithContext(ctx).Model(model).
        Where("PK", "=", pk).
        Where("SK", "=", sk).
        First(model)
}

func (r *BaseRepository[T]) Create(ctx context.Context, model T) error {
    return r.db.WithContext(ctx).Model(model).Create()
}

// ... other CRUD operations
```

### Task 2.2-2.4: Refactor Repositories to Use BaseRepository
**For each repository:**
1. Embed BaseRepository
2. Remove duplicated CRUD methods
3. Keep only domain-specific logic

**Verification after each:**
```bash
go build ./pkg/storage/repositories/...
make test
```

## Phase 3: Consolidate Similar Repositories

### Task 3.1: Merge User and Actor Repositories
**Actions:**
1. Create new `/pkg/storage/repositories/account_repository.go`
2. Merge logic from both repositories
3. Update all references
4. Delete old repositories

### Task 3.2: Merge Auth Repositories
**Actions:**
1. Combine AuthRepository, OAuthRepository, WebAuthnRepository
2. Create unified authentication repository
3. Update all auth flows

### Task 3.3: Consolidate Moderation Repositories
**Actions:**
1. Merge all moderation-related repositories
2. Create single moderation repository with sub-modules

## Phase 4: Extract Common Patterns

### Task 4.1: Create Query Builder Utilities
**Files to create:**
- `/pkg/storage/repositories/utils/query_builder.go`
- `/pkg/storage/repositories/utils/pagination.go`

## Phase 5: Final Architecture

### Task 5.1: Create Minimal Storage Interface
**Implementation:**
```go
type Storage interface {
    Accounts() AccountRepository
    Objects() ObjectRepository
    Activities() ActivityRepository
    Social() SocialRepository
    Federation() FederationRepository
    Moderation() ModerationRepository
    Analytics() AnalyticsRepository
}
```

## Success Criteria
- [ ] All tests pass after each step
- [ ] No compilation errors
- [ ] StorageAdapter completely removed
- [ ] ~20,000 lines of code eliminated
- [ ] Performance benchmarks show improvement
- [ ] Cost tracking still works correctly

## Rollback Plan
1. Keep StorageAdapter available but deprecated during Phase 1
2. Use feature flags to switch between implementations
3. Monitor error rates and performance metrics
4. Quick revert possible via git if needed