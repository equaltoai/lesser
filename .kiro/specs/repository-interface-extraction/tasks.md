# Implementation Plan: Repository Interface Extraction

## Overview

This implementation plan refactors `core.RepositoryStorage` to return interfaces instead of concrete repository types, enabling proper unit testing through mock injection. The work is divided into three phases: UserRepository (proof of concept), Top 10 repositories, and all remaining repositories.

## Tasks

- [x] 1. Set up interface package structure
  - Create `pkg/storage/interfaces/` directory
  - Create `pkg/storage/interfaces/doc.go` with package documentation
  - Create `pkg/testing/inmemory/` directory
  - Create `pkg/testing/inmemory/doc.go` with package documentation
  - _Requirements: 1.1, 2.1, 3.1_

- [x] 2. Phase 1: UserRepository Interface Extraction
  - [x] 2.1 Create UserRepository interface
    - Create `pkg/storage/interfaces/user.go`
    - Define interface with all public methods from `repositories.UserRepository`
    - Include CreateUser, GetUser, GetUserByEmail, UpdateUser, DeleteUser, ListUsers
    - Include GetActiveUserCount, GetTotalUserCount
    - Include OAuth provider methods (GetUserByProviderID, LinkProviderAccount, etc.)
    - Include AccountPin methods (CreateAccountPin, DeleteAccountPin, etc.)
    - Include AccountNote methods
    - Include Reputation methods (StoreReputation, GetReputation, etc.)
    - Include Vouch methods (CreateVouch, GetVouch, RevokeVouch, etc.)
    - _Requirements: 1.1, 1.2_

  - [x] 2.2 Create MockUserRepository
    - Create `pkg/testing/mocks/user_repository_mock.go`
    - Implement all UserRepository interface methods using testify/mock
    - Each method should call `m.Called()` and return appropriate types
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 2.3 Create InMemoryUserRepository
    - Create `pkg/testing/inmemory/user_repository.go`
    - Implement thread-safe storage using sync.RWMutex and maps
    - Implement all UserRepository interface methods
    - Return appropriate errors (ErrNotFound, ErrAlreadyExists)
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 2.4 Write property test for interface completeness
    - **Property 1: Interface Method Completeness**
    - Use reflection to verify UserRepository interface has all public methods from concrete type
    - Test: `TestUserRepositoryInterfaceCompleteness` in `pkg/testing/inmemory/user_repository_test.go`
    - **Validates: Requirements 1.2**

  - [x] 2.5 Write property test for in-memory round-trip
    - **Property 5: In-Memory Round-Trip Consistency**
    - Generate random User data, store in InMemoryUserRepository, retrieve, compare
    - Tests: `TestUserRepositoryRoundTrip*` in `pkg/testing/inmemory/user_repository_test.go`
    - **Validates: Requirements 3.2**

  - [x] 2.6 Write property test for thread safety
    - **Property 6: In-Memory Thread Safety**
    - Run concurrent read/write operations on InMemoryUserRepository
    - Verify no data races using -race flag
    - Tests: `TestUserRepositoryThreadSafety*` in `pkg/testing/inmemory/user_repository_test.go`
    - **Validates: Requirements 3.4**

- [x] 3. Update RepositoryStorage interface
  - [x] 3.1 Modify core.RepositoryStorage to return interfaces
    - Update `pkg/storage/core/interfaces.go`
    - Change `User() *repositories.UserRepository` to `User() interfaces.UserRepository`
    - Add import for `pkg/storage/interfaces`
    - _Requirements: 1.3, 5.1_

  - [x] 3.2 Update DynamORM storage adapter
    - Ensure `pkg/storage/adapters/dynamorm_storage.go` still compiles
    - Concrete `*repositories.UserRepository` satisfies `interfaces.UserRepository`
    - _Requirements: 5.1, 5.2_

  - [x] 3.3 Write property test for return type correctness
    - **Property 2: Return Type Correctness**
    - Use reflection to verify User() returns interface type
    - **Validates: Requirements 1.3, 4.3**

- [x] 4. Checkpoint - Phase 1 Complete
  - Run `go build ./...` to verify compilation
  - Run `go test ./...` to verify all existing tests pass
  - Ensure all tests pass, ask the user if questions arise.
  - _Requirements: 5.1, 5.2, 5.3, 6.1_

- [x] 5. Create MockRepositoryStorage
  - [x] 5.1 Create enhanced MockRepositoryStorage
    - Create `pkg/testing/mock_repository_storage.go`
    - Implement RepositoryStorage interface
    - Use functional options pattern for configuration
    - Default to in-memory implementations
    - Allow injection of custom mocks via WithUserRepository, etc.
    - _Requirements: 4.1, 4.2, 4.3_

  - [x] 5.2 Write unit tests for MockRepositoryStorage
    - Test default in-memory behavior
    - Test custom mock injection
    - Test that all repository accessors return correct types
    - _Requirements: 4.1, 4.2, 4.3, 4.4_

- [x] 6. Demonstrate testability with VouchManager
  - [x] 6.1 Refactor VouchManager to use interfaces
    - Update `pkg/reputation/vouch.go` to accept `core.RepositoryStorage`
    - Verify it works with both real and mock storage
    - _Requirements: 1.4, 5.2_

  - [x] 6.2 Write unit tests for VouchManager using mocks
    - Create `pkg/reputation/vouch_mock_test.go`
    - Test CreateVouch, RevokeVouch, GetVouchByID using MockRepositoryStorage
    - Demonstrate that VouchManager is now fully testable
    - _Requirements: 6.1, 6.4_

- [x] 7. Checkpoint - Phase 1 Validation
  - Run full test suite with coverage
  - Verify VouchManager tests pass with mocks
  - Ensure all tests pass, ask the user if questions arise.
  - _Requirements: 5.3, 6.1, 6.4_

- [-] 8. Phase 2: Top 10 Repository Interfaces
  - [x] 8.1 Create AccountRepository interface and implementations
    - Create `pkg/storage/interfaces/account.go`
    - Create `pkg/testing/mocks/account_repository_mock.go`
    - Create `pkg/testing/inmemory/account_repository.go`
    - Update RepositoryStorage.Account() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.2 Create ActorRepository interface and implementations
    - Create `pkg/storage/interfaces/actor.go`
    - Create `pkg/testing/mocks/actor_repository_mock.go`
    - Create `pkg/testing/inmemory/actor_repository.go`
    - Update RepositoryStorage.Actor() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.3 Create StatusRepository interface and implementations
    - Create `pkg/storage/interfaces/status.go`
    - Create `pkg/testing/mocks/status_repository_mock.go`
    - Create `pkg/testing/inmemory/status_repository.go`
    - Update RepositoryStorage.Status() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.4 Create TimelineRepository interface and implementations
    - Create `pkg/storage/interfaces/timeline.go`
    - Create `pkg/testing/mocks/timeline_repository_mock.go`
    - Create `pkg/testing/inmemory/timeline_repository.go`
    - Update RepositoryStorage.Timeline() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.5 Create NotificationRepository interface and implementations
    - Create `pkg/storage/interfaces/notification.go`
    - Create `pkg/testing/mocks/notification_repository_mock.go`
    - Create `pkg/testing/inmemory/notification_repository.go`
    - Update RepositoryStorage.Notification() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.6 Create RelationshipRepository interface and implementations
    - Create `pkg/storage/interfaces/relationship.go`
    - Create `pkg/testing/mocks/relationship_repository_mock.go`
    - Create `pkg/testing/inmemory/relationship_repository.go`
    - Update RepositoryStorage.Relationship() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.7 Create ObjectRepository interface and implementations
    - Create `pkg/storage/interfaces/object.go`
    - Create `pkg/testing/mocks/object_repository_mock.go`
    - Create `pkg/testing/inmemory/object_repository.go`
    - Update RepositoryStorage.Object() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.8 Create ActivityRepository interface and implementations
    - Create `pkg/storage/interfaces/activity.go`
    - Create `pkg/testing/mocks/activity_repository_mock.go`
    - Create `pkg/testing/inmemory/activity_repository.go`
    - Update RepositoryStorage.Activity() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.9 Create TrustRepository interface and implementations
    - Create `pkg/storage/interfaces/trust.go`
    - Create `pkg/testing/mocks/trust_repository_mock.go`
    - Create `pkg/testing/inmemory/trust_repository.go`
    - Update RepositoryStorage.Trust() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 8.10 Create ModerationRepository interface and implementations
    - Create `pkg/storage/interfaces/moderation.go`
    - Create `pkg/testing/mocks/moderation_repository_mock.go`
    - Create `pkg/testing/inmemory/moderation_repository.go`
    - Update RepositoryStorage.Moderation() return type
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

- [x] 9. Checkpoint - Phase 2 Complete
  - Run `go build ./...` to verify compilation
  - Run `go test ./...` to verify all existing tests pass
  - Ensure all tests pass, ask the user if questions arise.
  - _Requirements: 5.1, 5.2, 5.3, 6.2_

- [x] 10. Write property tests for Phase 2 repositories
  - [x] 10.1 Write mock coverage property test
    - **Property 3: Mock Implementation Coverage**
    - Verify all 10 repository interfaces have corresponding mocks
    - **Validates: Requirements 2.1**

  - [x] 10.2 Write in-memory coverage property test
    - **Property 4: In-Memory Implementation Coverage**
    - Verify all 10 repository interfaces have corresponding in-memory implementations
    - **Validates: Requirements 3.1**

- [-] 11. Phase 3: Remaining Repository Interfaces
  - [x] 11.1 Create interfaces for remaining repositories (batch 1)
    - BookmarkRepository, LikeRepository, ListRepository, MediaRepository
    - MediaMetadataRepository, PollRepository, PushSubscriptionRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 11.2 Create interfaces for remaining repositories (batch 2)
    - HashtagRepository, ScheduledStatusRepository, AnnouncementRepository
    - DomainBlockRepository, InstanceRepository, FederationRepository
    - RecoveryRepository, TrendingRepository, SocialRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 11.3 Create interfaces for remaining repositories (batch 3)
    - TrackingRepository, WebSocketCostRepository, SearchRepository
    - RelayRepository, CommunityNoteRepository, EmojiRepository
    - RateLimitRepository, ConversationRepository, MarkerRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 11.4 Create interfaces for remaining repositories (batch 4)
    - FeaturedTagRepository, AIRepository, ExportRepository
    - ImportRepository, DLQRepository, MetricRecordRepository
    - CloudWatchMetricsRepository, StreamingCloudWatchRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [x] 11.5 Create interfaces for remaining repositories (batch 5)
    - AuditRepository, OAuthRepository, DNSCacheRepository
    - FilterRepository, ThreadRepository, SeveranceRepository
    - ModerationMLRepository, QuoteRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [ ] 11.6 Create interfaces for remaining repositories (batch 6)
    - MediaAnalyticsRepository, MediaPopularityRepository
    - MediaSessionRepository, StreamingConnectionRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

  - [ ] 11.7 Create interfaces for CMS repositories
    - ArticleRepository, DraftRepository, RevisionRepository
    - SeriesRepository, CategoryRepository, PublicationRepository
    - PublicationMemberRepository
    - _Requirements: 1.1, 1.2, 1.3, 2.1, 3.1_

- [ ] 12. Final Checkpoint - All Phases Complete
  - Run `go build ./...` to verify compilation
  - Run `go test ./...` to verify all existing tests pass
  - Run `go test -race ./...` to verify no data races
  - Verify coverage meets baseline
  - Ensure all tests pass, ask the user if questions arise.
  - _Requirements: 5.1, 5.2, 5.3, 6.3, 6.4_

## Notes

- All tasks including property-based tests are required
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Phase 1 is the proof of concept - if issues arise, address them before Phase 2
- Concrete repository types automatically satisfy their interfaces (Go duck typing)
- No changes needed to existing production code - only interface definitions and test utilities
