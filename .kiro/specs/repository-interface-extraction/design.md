# Design Document: Repository Interface Extraction

## Overview

This design document describes the refactoring of `core.RepositoryStorage` to return interfaces instead of concrete repository types. The current architecture returns concrete pointer types (e.g., `*repositories.UserRepository`), which prevents unit testing of code that depends on repositories because concrete types cannot be mocked.

The solution introduces a new `pkg/storage/interfaces/` package containing interface definitions for all 60+ repositories, enabling mock and in-memory implementations for testing.

## Architecture

### Current State

```
┌─────────────────────────────────────────────────────────────┐
│                    Consumer Code                             │
│  (e.g., pkg/reputation/vouch.go, graph resolvers)           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              core.RepositoryStorage (interface)              │
│  User() *repositories.UserRepository  ◄── Concrete type!    │
│  Account() *repositories.AccountRepository                   │
│  ...60+ more methods returning concrete types                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              repositories.UserRepository                     │
│  (DynamoDB-backed, cannot be mocked)                        │
└─────────────────────────────────────────────────────────────┘
```

### Target State

```
┌─────────────────────────────────────────────────────────────┐
│                    Consumer Code                             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              core.RepositoryStorage (interface)              │
│  User() interfaces.UserRepository  ◄── Interface type!      │
│  Account() interfaces.AccountRepository                      │
│  ...60+ more methods returning interface types               │
└─────────────────────────────────────────────────────────────┘
                    │                    │
          ┌────────┴────────┐   ┌───────┴────────┐
          ▼                 ▼   ▼                ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│ Production Impl  │  │   Mock Impl      │  │ In-Memory Impl   │
│ (DynamoDB)       │  │ (testify/mock)   │  │ (map-based)      │
└──────────────────┘  └──────────────────┘  └──────────────────┘
```

## Components and Interfaces

### Package Structure

```
pkg/storage/
├── core/
│   └── interfaces.go          # RepositoryStorage interface (modified)
├── interfaces/
│   ├── user.go                # UserRepository interface
│   ├── account.go             # AccountRepository interface
│   ├── actor.go               # ActorRepository interface
│   └── ...                    # One file per repository interface
├── repositories/
│   └── ...                    # Existing concrete implementations (unchanged)
└── adapters/
    └── dynamorm_storage.go    # Existing adapter (unchanged internally)

pkg/testing/
├── mocks/
│   ├── user_repository_mock.go
│   ├── account_repository_mock.go
│   └── ...
├── inmemory/
│   ├── user_repository.go
│   ├── account_repository.go
│   └── ...
└── mock_repository_storage.go  # Enhanced MockRepositoryStorage
```

### Interface Definitions

Each repository interface will be defined in `pkg/storage/interfaces/` with all public methods from the concrete implementation.

```go
// pkg/storage/interfaces/user.go
package interfaces

import (
    "context"
    "github.com/equaltoai/lesser/pkg/storage"
)

// UserRepository defines the interface for user operations
type UserRepository interface {
    // Core CRUD operations
    CreateUser(ctx context.Context, user *storage.User) error
    GetUser(ctx context.Context, username string) (*storage.User, error)
    GetUserByEmail(ctx context.Context, email string) (*storage.User, error)
    UpdateUser(ctx context.Context, username string, updates map[string]any) error
    DeleteUser(ctx context.Context, username string) error
    ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error)
    
    // Count operations
    GetActiveUserCount(ctx context.Context, days int) (int64, error)
    GetTotalUserCount(ctx context.Context) (int64, error)
    
    // OAuth provider operations
    GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error)
    LinkProviderAccount(ctx context.Context, username, provider, providerID string) error
    UnlinkProviderAccount(ctx context.Context, username, provider string) error
    GetLinkedProviders(ctx context.Context, username string) ([]string, error)
    
    // Account pins (endorsed accounts)
    CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error
    DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error
    GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error)
    IsAccountPinned(ctx context.Context, username, actorID string) (bool, error)
    
    // Account notes
    CreateAccountNote(ctx context.Context, note *storage.AccountNote) error
    GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error)
    UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error
    DeleteAccountNote(ctx context.Context, username, targetActorID string) error
    
    // Reputation operations
    StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error
    GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error)
    GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error)
    GetUserTrustScore(ctx context.Context, userID string) (float64, error)
    
    // Vouch operations
    CreateVouch(ctx context.Context, vouch *storage.Vouch) error
    GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error)
    RevokeVouch(ctx context.Context, vouchID string) error
    GetVouchesForActor(ctx context.Context, actorID string) ([]*storage.Vouch, error)
    GetVouchesByActor(ctx context.Context, actorID string) ([]*storage.Vouch, error)
}
```

### Modified RepositoryStorage Interface

```go
// pkg/storage/core/interfaces.go
package core

import (
    "github.com/equaltoai/lesser/pkg/storage/interfaces"
    dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
    "go.uber.org/zap"
)

// RepositoryStorage provides access to all repository implementations
type RepositoryStorage interface {
    // Repository access methods - return interfaces instead of concrete types
    Account() interfaces.AccountRepository
    Bookmark() interfaces.BookmarkRepository
    Actor() interfaces.ActorRepository
    Object() interfaces.ObjectRepository
    Activity() interfaces.ActivityRepository
    Timeline() interfaces.TimelineRepository
    Notification() interfaces.NotificationRepository
    Like() interfaces.LikeRepository
    Moderation() interfaces.ModerationRepository
    List() interfaces.ListRepository
    Media() interfaces.MediaRepository
    MediaMetadata() interfaces.MediaMetadataRepository
    Poll() interfaces.PollRepository
    PushSubscription() interfaces.PushSubscriptionRepository
    Hashtag() interfaces.HashtagRepository
    ScheduledStatus() interfaces.ScheduledStatusRepository
    Announcement() interfaces.AnnouncementRepository
    DomainBlock() interfaces.DomainBlockRepository
    Relationship() interfaces.RelationshipRepository
    Instance() interfaces.InstanceRepository
    Federation() interfaces.FederationRepository
    Recovery() interfaces.RecoveryRepository
    Analytics() interfaces.TrendingRepository
    Social() interfaces.SocialRepository
    User() interfaces.UserRepository
    Status() interfaces.StatusRepository
    Cost() interfaces.TrackingRepository
    WebSocketCost() interfaces.WebSocketCostRepository
    Trust() interfaces.TrustRepository
    Search() interfaces.SearchRepository
    Relay() interfaces.RelayRepository
    CommunityNote() interfaces.CommunityNoteRepository
    Emoji() interfaces.EmojiRepository
    RateLimit() interfaces.RateLimitRepository
    Conversation() interfaces.ConversationRepository
    Marker() interfaces.MarkerRepository
    FeaturedTag() interfaces.FeaturedTagRepository
    AI() interfaces.AIRepository
    Export() interfaces.ExportRepository
    Import() interfaces.ImportRepository
    DLQ() interfaces.DLQRepository
    MetricRecord() interfaces.MetricRecordRepository
    CloudWatchMetrics() interfaces.CloudWatchMetricsRepository
    StreamingCloudWatch() interfaces.StreamingCloudWatchRepository
    Audit() interfaces.AuditRepository
    OAuth() interfaces.OAuthRepository
    DNSCache() interfaces.DNSCacheRepository
    Filter() interfaces.FilterRepository
    Thread() interfaces.ThreadRepository
    Severance() interfaces.SeveranceRepository
    ModerationML() interfaces.ModerationMLRepository
    Quote() interfaces.QuoteRepository
    MediaAnalytics() interfaces.MediaAnalyticsRepository
    MediaPopularity() interfaces.MediaPopularityRepository
    MediaSession() interfaces.MediaSessionRepository
    StreamingConnection() interfaces.StreamingConnectionRepository

    // CMS Repositories
    Article() interfaces.ArticleRepository
    Draft() interfaces.DraftRepository
    Revision() interfaces.RevisionRepository
    Series() interfaces.SeriesRepository
    Category() interfaces.CategoryRepository
    Publication() interfaces.PublicationRepository
    PublicationMember() interfaces.PublicationMemberRepository

    // Utility methods
    GetDB() dynamormCore.DB
    GetTableName() string
    GetLogger() *zap.Logger
}
```

### Mock Implementation Pattern

```go
// pkg/testing/mocks/user_repository_mock.go
package mocks

import (
    "context"
    "github.com/equaltoai/lesser/pkg/storage"
    "github.com/stretchr/testify/mock"
)

// MockUserRepository is a mock implementation of interfaces.UserRepository
type MockUserRepository struct {
    mock.Mock
}

func (m *MockUserRepository) CreateUser(ctx context.Context, user *storage.User) error {
    args := m.Called(ctx, user)
    return args.Error(0)
}

func (m *MockUserRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
    args := m.Called(ctx, username)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*storage.User), args.Error(1)
}

// ... all other methods follow the same pattern
```

### In-Memory Implementation Pattern

```go
// pkg/testing/inmemory/user_repository.go
package inmemory

import (
    "context"
    "sync"
    "github.com/equaltoai/lesser/pkg/storage"
)

// InMemoryUserRepository is a thread-safe in-memory implementation
type InMemoryUserRepository struct {
    mu    sync.RWMutex
    users map[string]*storage.User
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
    return &InMemoryUserRepository{
        users: make(map[string]*storage.User),
    }
}

func (r *InMemoryUserRepository) CreateUser(ctx context.Context, user *storage.User) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if _, exists := r.users[user.Username]; exists {
        return storage.ErrAlreadyExists
    }
    r.users[user.Username] = user
    return nil
}

func (r *InMemoryUserRepository) GetUser(ctx context.Context, username string) (*storage.User, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    user, exists := r.users[username]
    if !exists {
        return nil, storage.ErrNotFound
    }
    return user, nil
}

// ... all other methods
```

### Enhanced MockRepositoryStorage

```go
// pkg/testing/mock_repository_storage.go
package testing

import (
    "github.com/equaltoai/lesser/pkg/storage/interfaces"
    "github.com/equaltoai/lesser/pkg/testing/inmemory"
    "github.com/equaltoai/lesser/pkg/testing/mocks"
)

// MockRepositoryStorage provides configurable repository implementations for testing
type MockRepositoryStorage struct {
    userRepo     interfaces.UserRepository
    accountRepo  interfaces.AccountRepository
    // ... all other repositories
}

// Option configures MockRepositoryStorage
type Option func(*MockRepositoryStorage)

// WithUserRepository sets a custom user repository implementation
func WithUserRepository(repo interfaces.UserRepository) Option {
    return func(s *MockRepositoryStorage) {
        s.userRepo = repo
    }
}

// NewMockRepositoryStorage creates a new MockRepositoryStorage with in-memory defaults
func NewMockRepositoryStorage(opts ...Option) *MockRepositoryStorage {
    s := &MockRepositoryStorage{
        userRepo:    inmemory.NewInMemoryUserRepository(),
        accountRepo: inmemory.NewInMemoryAccountRepository(),
        // ... initialize all with in-memory implementations
    }
    
    for _, opt := range opts {
        opt(s)
    }
    
    return s
}

func (s *MockRepositoryStorage) User() interfaces.UserRepository {
    return s.userRepo
}

// ... all other repository accessors
```

## Data Models

No changes to existing data models. The interfaces will use the existing `storage.*` types (e.g., `storage.User`, `storage.Account`).

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Interface Method Completeness

*For any* concrete repository type and its corresponding interface, all public methods of the concrete type SHALL be present in the interface with matching signatures.

**Validates: Requirements 1.2**

### Property 2: Return Type Correctness

*For any* method in RepositoryStorage that returns a repository, the return type SHALL be an interface type from `pkg/storage/interfaces/`, not a concrete pointer type.

**Validates: Requirements 1.3, 4.3**

### Property 3: Mock Implementation Coverage

*For any* repository interface defined in `pkg/storage/interfaces/`, there SHALL exist a corresponding mock implementation in `pkg/testing/mocks/` that implements all interface methods.

**Validates: Requirements 2.1**

### Property 4: In-Memory Implementation Coverage

*For any* repository interface defined in `pkg/storage/interfaces/`, there SHALL exist a corresponding in-memory implementation in `pkg/testing/inmemory/` that implements all interface methods.

**Validates: Requirements 3.1**

### Property 5: In-Memory Round-Trip Consistency

*For any* data stored in an in-memory repository, retrieving that data SHALL return an equivalent value to what was stored.

**Validates: Requirements 3.2**

### Property 6: In-Memory Thread Safety

*For any* in-memory repository, concurrent read and write operations from multiple goroutines SHALL not cause data races or panics.

**Validates: Requirements 3.4**

## Error Handling

### Backward Compatibility Errors

If the refactoring introduces breaking changes:
1. Compilation errors in existing code indicate interface mismatch
2. Runtime errors indicate behavioral changes
3. Test failures indicate regression

### Mock Configuration Errors

- If a mock method is called without expectations set, testify/mock will panic with a clear message
- If expectations are not met, `mock.AssertExpectations(t)` will fail the test

### In-Memory Repository Errors

- `storage.ErrNotFound` - when querying non-existent data
- `storage.ErrAlreadyExists` - when creating duplicate data
- Standard Go errors for invalid operations

## Testing Strategy

### Unit Tests

Unit tests will verify:
- Each interface method signature matches the concrete implementation
- Mock implementations correctly implement interfaces
- In-memory implementations correctly implement interfaces
- MockRepositoryStorage correctly returns configured implementations

### Property-Based Tests

Property-based tests will verify the correctness properties using `gopter` or `rapid`:

1. **Interface Completeness Test**: Use reflection to compare method sets
2. **Round-Trip Test**: Generate random data, store, retrieve, compare
3. **Thread-Safety Test**: Run concurrent operations, verify no races

### Integration Tests

Integration tests will verify:
- Existing code continues to work with the new interface-based approach
- MockRepositoryStorage can be used in place of real storage
- All 60+ repositories can be mocked and tested

### Test Configuration

- Property-based tests: minimum 100 iterations
- Thread-safety tests: 100 goroutines, 1000 operations each
- Each property test tagged with: **Feature: repository-interface-extraction, Property N: {property_text}**

## Phased Implementation

### Phase 1: UserRepository (Proof of Concept)

1. Create `pkg/storage/interfaces/user.go` with UserRepository interface
2. Create `pkg/testing/mocks/user_repository_mock.go`
3. Create `pkg/testing/inmemory/user_repository.go`
4. Update `core.RepositoryStorage` to return `interfaces.UserRepository`
5. Verify all existing code compiles and tests pass
6. Write property tests for UserRepository

### Phase 2: Top 10 Most-Used Repositories

Based on usage analysis, extract interfaces for:
1. AccountRepository
2. ActorRepository
3. StatusRepository
4. TimelineRepository
5. NotificationRepository
6. RelationshipRepository
7. ObjectRepository
8. ActivityRepository
9. TrustRepository
10. ModerationRepository

### Phase 3: All Remaining Repositories

Extract interfaces for all remaining ~50 repositories following the established pattern.

## Migration Notes

### For Existing Code

No changes required. The concrete implementations satisfy the interfaces, so existing code that receives a `*repositories.UserRepository` can still use it where `interfaces.UserRepository` is expected.

### For New Tests

```go
// Before (impossible to test)
func TestVouchManager(t *testing.T) {
    // Cannot mock core.RepositoryStorage because it returns concrete types
}

// After (fully testable)
func TestVouchManager(t *testing.T) {
    mockUserRepo := &mocks.MockUserRepository{}
    mockUserRepo.On("GetUser", mock.Anything, "testuser").Return(&storage.User{...}, nil)
    
    storage := testing.NewMockRepositoryStorage(
        testing.WithUserRepository(mockUserRepo),
    )
    
    manager := reputation.NewVouchManager(storage)
    // Now we can test VouchManager in isolation!
}
```
