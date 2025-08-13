# API Alignment Implementation Checklist

This checklist follows the service-first architecture to minimize duplication across REST, GraphQL, and WebSocket APIs. Each task builds on the architectural foundation described in [API_ALIGNMENT_ARCHITECTURE.md](../architecture/API_ALIGNMENT_ARCHITECTURE.md).

**Pre-Release Advantage**: No backward compatibility needed. We can break anything to achieve the ideal architecture.

## 🎯 **CURRENT STATUS: Phase 4 COMPLETED - Full GraphQL Integration**

**✅ Foundation Complete:** Event Publisher, Service Registry, Repository Interfaces  
**✅ Core Services Complete:** All 10 domain services implemented with full testing  
**✅ REST API Complete:** ALL handlers migrated to service-first architecture  
**🎉 GraphQL API Complete:** ALL resolvers use service layer with NO hardcoded values  
**📊 Test Coverage:** 150+ test cases across all services  
**🏗️ Architecture:** Service-first design supports REST + GraphQL APIs

**MAJOR COMPLETION - PHASE 3E:**
- **✅ CRITICAL**: Created 3 new comprehensive services (Emoji, ScheduledStatus, Search)
- **✅ CRITICAL**: Migrated ALL remaining OLD_ONLY files to service-first pattern
- **✅ CRITICAL**: Zero repository calls remain in handlers - 100% service usage
- **✅ CRITICAL**: Fixed all compilation errors in new services
- **✅ FOUNDATION**: All 10 domain services fully operational

## ✅ Phase 1: Foundation (COMPLETED)

### ✅ 1.1 Event Publisher Infrastructure
- [x] Create `pkg/streaming/publisher.go` **[5fb7441]**
  - [x] Define `Publisher` interface with `PublishToUser`, `PublishToStream`, `PublishToConversation`
  - [x] Implement `Event` struct with Type, Stream, Payload, Timestamp
  - [x] Create `apiGatewayPublisher` implementation using API Gateway Management API
  - [x] Add `mockPublisher` for testing
  - [x] Write unit tests for publisher (53 test cases)

- [x] Create `pkg/streaming/events.go` **[5fb7441]**
  - [x] Define event type constants (e.g., `StatusCreated = "status.created"`)
  - [x] Define stream name constants (e.g., `UserStream = "user"`, `PublicStream = "public"`)
  - [x] Create event builder helpers with fluent interface

### ✅ 1.2 Service Registry
- [x] Create `pkg/services/registry.go` **[f968440]**
  - [x] Define `Registry` struct with all service fields
  - [x] Add `NewRegistry` constructor with dependency injection
  - [x] Add `WithPublisher`, `WithStorage`, `WithLogger`, `WithConfig` option functions
  - [x] Write tests for registry initialization (11 test cases)
  - [x] Thread-safe lazy initialization with performance optimization

### ✅ 1.3 Repository Interfaces
- [x] Create `pkg/storage/interfaces/repositories.go` **[a14e61c]**
  - [x] Define all repository interfaces
  - [x] StatusRepository (formerly NoteRepository)
  - [x] AccountRepository with 25+ methods
  - [x] RelationshipRepository
  - [x] MediaRepository
  - [x] ConversationRepository
  - [x] ListRepository
  - [x] NotificationRepository with 21 methods
  - [x] EmojiRepository
  - [x] ScheduledStatusRepository
  - [x] SearchRepository

## ✅ Phase 2: Core Domain Services (COMPLETED)

### ✅ 2.1-2.7 Initial Services
- [x] Notes Service - Create, Update, Delete, Get, List with federation
- [x] Accounts Service - Profile management, search, preferences
- [x] Relationships Service - Follow, Unfollow, Block, Mute operations
- [x] Conversations Service - Direct messages, conversation management
- [x] Media Service - Upload, processing, metadata management
- [x] Lists Service - CRUD operations, member management, timeline generation
- [x] Notifications Service - Creation, marking read, clearing, filtering

### ✅ 2.8-2.10 Extended Services (Phase 3E)
- [x] **Emoji Service** (`pkg/services/emoji/service.go`)
  - [x] Full CRUD operations for custom emojis
  - [x] Remote emoji federation support
  - [x] Visibility and category management
  - [x] Event-driven updates for real-time streaming

- [x] **ScheduledStatus Service** (`pkg/services/scheduled/service.go`)
  - [x] Complete lifecycle management for scheduled posts
  - [x] Media attachment handling
  - [x] Time validation (5 minutes to 1 year)
  - [x] Publishing workflow with event emission

- [x] **Search Service** (`pkg/services/search/service.go`)
  - [x] Account search and suggestions
  - [x] Profile directory browsing
  - [x] Follow suggestions with V1/V2 API support
  - [x] Suggestion removal capabilities

## ✅ Phase 3: Replace REST Handlers (COMPLETED)

### ✅ 3A-3C: Foundation and Integration
- [x] Authentication helper methods
- [x] Repository interface alignment
- [x] Service initialization in registry
- [x] Initial handler migrations

### ✅ 3D: Major Handler Migration
- [x] **Phase 3D.1**: Core Status Operations - 74 handlers in `statuses.go`
- [x] **Phase 3D.2**: Account Management - 48 handlers in `accounts.go`
- [x] **Phase 3D.3**: Timeline Operations - All handlers in `timelines.go`
- [x] **Phase 3D.4**: Relationships Management - All relationship handlers
- [x] **Phase 3D.5**: Follow Requests - All handlers migrated

### ✅ 3E: Complete Service-First Migration

#### ✅ Phase 3E.1: Complete MIXED Files
- [x] `accounts.go` - 26 repository calls migrated to Accounts service
- [x] `timelines.go` - 28 repository calls migrated to Notes/Lists services
- [x] `statuses.go` - 33 repository calls migrated to Notes service

#### ✅ Phase 3E.2: Priority OLD_ONLY Files
- [x] `polls.go` - 10 repository calls migrated to services
- [x] `lists.go` - 8 handlers fully migrated to Lists service
- [x] `media.go` - 3 handlers fully migrated to Media service
- [x] `misc.go` - Notification handlers migrated to Notifications service

#### ✅ Phase 3E.3: Remaining OLD_ONLY Files
- [x] **custom_emojis.go** - 8 repository calls migrated to new Emoji service
  - [x] GET /api/v1/custom_emojis - Lists visible emojis
  - [x] POST /api/v1/admin/custom_emojis - Creates emoji (admin)
  - [x] PUT /api/v1/admin/custom_emojis/:shortcode - Updates emoji (admin)
  - [x] DELETE /api/v1/admin/custom_emojis/:shortcode - Deletes emoji (admin)

- [x] **scheduled_statuses.go** - 8 repository calls migrated to new ScheduledStatus service
  - [x] GET /api/v1/scheduled_statuses - Lists scheduled statuses
  - [x] GET /api/v1/scheduled_statuses/:id - Gets specific scheduled status
  - [x] PUT /api/v1/scheduled_statuses/:id - Updates scheduled time
  - [x] DELETE /api/v1/scheduled_statuses/:id - Cancels scheduled status
  - [x] POST /api/v1/statuses (with scheduled_at) - Creates scheduled status

- [x] **discovery.go** - 6 repository calls migrated to new Search service
  - [x] GET /api/v1/directory - Profile directory
  - [x] GET /api/v1/suggestions - Follow suggestions (V1)
  - [x] GET /api/v2/suggestions - Follow suggestions (V2 with sources)
  - [x] DELETE /api/v1/suggestions/:account_id - Remove suggestion

#### ✅ Phase 3E.4: Architecture Verification
- [x] All handler files compile successfully
- [x] Zero direct repository calls in handlers
- [x] All business logic encapsulated in services
- [x] Event-driven architecture fully implemented
- [x] Service registry manages all 10 domain services

## 🎉 **SERVICE-FIRST MIGRATION COMPLETE**

### Final Statistics:
- **10 Domain Services** fully implemented and operational
- **700+ Handlers** migrated to service-first architecture
- **0 Repository Calls** remaining in handlers
- **150+ Test Cases** across all services
- **100% Service Coverage** for all REST API endpoints

### Services Available:
1. ✅ **Notes Service** - Status creation, interactions, timelines
2. ✅ **Accounts Service** - Profile management, search, preferences
3. ✅ **Relationships Service** - Follow, block, mute operations
4. ✅ **Conversations Service** - Direct messages, conversation management
5. ✅ **Media Service** - Upload, processing, CDN management
6. ✅ **Lists Service** - List management, member operations
7. ✅ **Notifications Service** - Notification creation and management
8. ✅ **Emoji Service** - Custom emoji CRUD and federation
9. ✅ **ScheduledStatus Service** - Scheduled post management
10. ✅ **Search Service** - Search, discovery, and suggestions

### Architecture Achievements:
- **Clean Separation**: Handlers → Services → Repositories
- **No Duplication**: All business logic in single service layer
- **Event-Driven**: All services emit events for real-time updates
- **Testable**: Services fully tested in isolation
- **Maintainable**: Clear boundaries and responsibilities

## ✅ Phase 4: Add GraphQL Support (COMPLETED)

### ✅ 4.1 GraphQL Resolver Implementation **[Phase 4 Complete]**
- [x] **Critical Infrastructure Fixes**
  - [x] Fixed undefined eventBus references - EventBus properly implemented in service registry
  - [x] Fixed struct field mismatches in InfrastructureStatus, InstanceRelations, BudgetAlert, CostAlert
  - [x] Removed ALL hardcoded values from GraphQL resolvers
  - [x] Replaced hardcoded federation score with real-time calculation using federation metrics

- [x] **Service Integration Complete**
  - [x] Updated `graph/schema.resolvers.go` with full service layer integration
  - [x] All GraphQL queries now use service registry methods (no direct storage calls)
  - [x] All GraphQL mutations implemented using service commands
  - [x] All GraphQL subscriptions connected to EventBus streams

- [x] **Analytics Service Integration**
  - [x] GetInfrastructureHealth() - Real infrastructure monitoring via storage adapter
  - [x] GetInstanceBudgets() - Budget tracking and overspend detection
  - [x] GetInstanceHealthReport() - Domain health metrics and recommendations

- [x] **Federation Service Integration**
  - [x] GetInstanceRelationships() - Federation relationship analysis with real-time scoring
  - [x] Federation score calculation based on delivery success, response times, uptime metrics
  - [x] Comprehensive relationship mapping (blocked, limited, connections)

- [x] **Media Service Integration**
  - [x] GetStreamingURL() - Media streaming with proper bitrate variants
  - [x] Dynamic quality selection and CDN URL generation
  - [x] Support for video bitrates, thumbnails, and streaming metadata

### ✅ 4.2 Storage Adapter Enhancement
- [x] **Added Missing Methods to StorageAdapter Interface**
  - [x] GetInfrastructureHealth() - Infrastructure monitoring
  - [x] GetInstanceBudgets() - Budget tracking queries
  - [x] GetInstanceHealthReport() - Domain health analysis
  - [x] GetInstanceRelationships() - Federation relationship data

- [x] **Implementation with Real Data Patterns**
  - [x] All methods query actual storage data (no hardcoded responses)
  - [x] Proper error handling and fallback mechanisms
  - [x] Uses existing repository interfaces and DynamORM patterns
  - [x] Maintains cost tracking and performance optimization

### ✅ 4.3 GraphQL Real-Time Architecture
- [x] **EventBus Integration Complete**
  - [x] All GraphQL subscriptions use EventBus for real-time updates
  - [x] Proper filtering and authorization for subscription events
  - [x] Connection to service event publishers for live data
  - [x] NO polling - all real-time data through EventBus streams

### ✅ 4.4 Compilation and Verification
- [x] **100% Service-First GraphQL Implementation**
  - [x] All GraphQL resolvers compile successfully
  - [x] Zero hardcoded values in any resolver method
  - [x] All queries/mutations use service layer exclusively
  - [x] Proper field name matching with generated GraphQL models

## Phase 5: WebSocket Command Support

### 5.1 Command Handler Infrastructure
- [ ] Create WebSocket command routing
- [ ] Map commands to service methods
- [ ] Handle responses and errors

### 5.2 Implement Command Handlers
- [ ] Status commands → Notes service
- [ ] Account commands → Accounts service
- [ ] Relationship commands → Relationships service
- [ ] All other commands → Respective services

## Phase 6: Long-Running Operations

### 6.1 Import/Export Service
- [ ] Create data portability service
- [ ] Handle large imports asynchronously
- [ ] Emit progress events

### 6.2 Bulk Operations
- [ ] Create bulk operations service
- [ ] Process bulk actions asynchronously
- [ ] Track progress and completion

## Success Criteria

- [x] **All REST endpoints use services** ✅
- [x] **No business logic duplication** ✅
- [x] **All services emit events** ✅
- [x] **Comprehensive test coverage** ✅
- [x] **Clean architecture boundaries** ✅
- [x] **GraphQL API complete** ✅ **[Phase 4 - NEW]**
- [ ] WebSocket commands implemented
- [ ] Long-running operations supported
- [ ] Performance metrics collected
- [ ] Documentation complete

## Notes

**Pre-Release Freedom**: We can make any breaking changes needed to achieve the ideal architecture.

**Current State**: Both REST and GraphQL APIs are complete. All REST handlers and GraphQL resolvers now use the service layer exclusively with zero hardcoded values. The architecture provides dual API support with no code duplication.

**Next Priority**: Implement WebSocket command handling using the existing service layer to complete the unified API architecture.