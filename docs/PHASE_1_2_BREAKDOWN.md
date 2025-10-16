# Phase 1.2 Breakdown: Thread Synchronization

**Status**: Phase 1.1 complete, Phase 1.2 queued  
**Total Operations**: 3 (all missing)  
**Total Duration**: 3-4 days split into 2 chunks  
**Dependency**: Phase 1.1 must be complete and shipped first

---

## 📊 Why Breaking into Chunks

**Original approach**: 3-4 days, one agent (overwhelming)  
**New approach**: Two 1.5-2 day chunks with intermediate audit

**Benefits**:
- ✅ Manageable scope per chunk
- ✅ Intermediate validation point
- ✅ Clear success criteria
- ✅ Easier to catch issues early
- ✅ Prevents large rework

---

## 🎯 Phase 1.2.1: Thread Service & Storage (Chunk 1)

**Duration**: 2-3 days  
**Scope**: Foundation layer (service, storage, traversal logic)  
**Dependency**: None (can start immediately after Phase 1.1)  
**Blocker for**: Phase 1.2.2

### What Gets Built

```
├── Service Layer
│   ├── pkg/services/threads/service.go (200-300 lines)
│   ├── pkg/services/threads/errors.go
│   └── pkg/services/threads/README.md
│
├── Storage Models
│   ├── pkg/storage/models/thread_sync.go
│   ├── pkg/storage/models/thread_node.go
│   └── pkg/storage/models/missing_reply.go
│
├── Repository Methods
│   └── pkg/storage/repositories/thread_repository.go (100+ lines)
│       ├── SaveThreadSync()
│       ├── SaveThreadNode()
│       ├── GetThreadNodes()
│       ├── MarkMissingReplies()
│       └── GetThreadContext()
│
├── Federation Integration
│   └── pkg/federation/thread_fetcher.go (or extend existing)
│       ├── FetchRemoteNote()
│       ├── FetchRemoteReplies()
│       └── ValidateThreadIntegrity()
│
└── Tests
    ├── pkg/services/threads/service_test.go (80%+ coverage)
    ├── pkg/services/threads/traversal_test.go
    └── pkg/storage/repositories/thread_repository_test.go
```

### Phase 1.2.1 Deliverables

✅ **Service Implementation**
- [ ] Thread traversal logic working
- [ ] Can find thread root by walking inReplyTo
- [ ] Can build thread tree (ancestors + descendants)
- [ ] Can detect missing replies
- [ ] Respects depth limit (max 10)
- [ ] Handles circular references

✅ **Storage Implementation**
- [ ] All models created
- [ ] All repository methods working
- [ ] Can save/retrieve thread contexts
- [ ] Can track sync status

✅ **Federation Integration**
- [ ] Can fetch remote notes via ActivityPub
- [ ] Can fetch replies collections
- [ ] Handles unreachable instances gracefully
- [ ] Handles deleted notes gracefully

✅ **Testing**
- [ ] Service tests: 80%+ coverage
- [ ] Tests for all traversal cases
- [ ] Tests for federation error handling
- [ ] Tests for circular reference detection

### Success Criteria for Phase 1.2.1

**Functional**:
- [ ] `service.GetThreadContext()` works correctly
- [ ] Finds root via inReplyTo chain
- [ ] Builds complete tree (ancestors + descendants)
- [ ] Detects missing replies
- [ ] Respects depth limit
- [ ] No infinite loops
- [ ] Handles federation errors

**Code Quality**:
- [ ] Follows service pattern (like pkg/services/notes/)
- [ ] Uses services.Registry for dependencies
- [ ] Proper error handling with custom types
- [ ] Input validation
- [ ] Logging at appropriate levels
- [ ] Cost tracking for remote fetches
- [ ] No N+1 queries

**Testing**:
- [ ] Unit tests passing
- [ ] 80%+ coverage of service
- [ ] Mock federation responses
- [ ] Edge cases covered

### Report When Complete

**Tell PM**:
```
Phase 1.2.1 Complete: Thread Service & Storage

COMPLETED:
✅ Service layer (thread traversal logic)
✅ Storage models (3 models)
✅ Repository methods (6 methods)
✅ Federation integration (FetchRemoteNote, etc.)
✅ Tests (80%+ coverage)

FILES:
- Created: pkg/services/threads/service.go, errors.go, README.md
- Created: pkg/storage/models/thread_*.go (3 files)
- Created: pkg/storage/repositories/thread_repository.go
- Created: pkg/federation/thread_fetcher.go
- Tests: service_test.go, traversal_test.go, repository_test.go

TEST RESULTS:
- Unit tests: X/X passing
- Coverage: X%
- No regressions

READY FOR PHASE 1.2.2
```

---

## 🎯 Phase 1.2.2: GraphQL Resolvers (Chunk 2)

**Duration**: 1.5-2 days  
**Scope**: GraphQL layer (resolvers only)  
**Dependency**: Phase 1.2.1 must be complete  
**Blocker for**: Nothing (completes Phase 1.2)

### What Gets Built

```
├── Query Resolvers (1 operation)
│   └── Query.threadContext(noteId) -> ThreadContext
│       ├── Calls service.GetThreadContext()
│       ├── Converts to GraphQL model
│       └── Returns fully populated ThreadContext
│
├── Mutation Resolvers (2 operations)
│   ├── Mutation.syncThread(noteUrl, depth) -> SyncThreadPayload
│   │   ├── Parses and validates URL
│   │   ├── Calls service.SyncRemoteThread()
│   │   ├── Returns sync results
│   │   └── Tracks metrics
│   │
│   └── Mutation.syncMissingReplies(noteId) -> SyncRepliesPayload
│       ├── Validates note exists
│       ├── Calls service.SyncMissingReplies()
│       ├── Returns sync results
│       └── Tracks metrics
│
├── Model Conversions
│   ├── convertThreadContextToModel()
│   ├── convertThreadNodeToObject()
│   └── convertSyncResultToPayload()
│
└── Tests
    ├── graph/schema.resolvers_thread_test.go (80%+ coverage)
    ├── Integration tests (service + resolver)
    └── Regression tests (no impact on other resolvers)
```

### Phase 1.2.2 Deliverables

✅ **GraphQL Resolvers**
- [ ] Query.threadContext working
- [ ] Mutation.syncThread working
- [ ] Mutation.syncMissingReplies working
- [ ] All return fully populated models
- [ ] Error handling correct

✅ **Model Conversions**
- [ ] ThreadContext converted correctly
- [ ] Ancestor chain ordered properly
- [ ] Descendant tree structured correctly
- [ ] All non-null fields populated

✅ **Testing**
- [ ] Resolver tests: 80%+ coverage
- [ ] Integration tests (service + resolver)
- [ ] Error scenarios covered
- [ ] No regressions on other subscriptions

### Success Criteria for Phase 1.2.2

**Functional**:
- [ ] Query.threadContext returns full ThreadContext
  - [ ] rootPost: Object (populated)
  - [ ] ancestors: [Object] (ordered, populated)
  - [ ] descendants: [Object] (tree, populated)
  - [ ] participantsCount: Int
  - [ ] missingCount: Int
  - [ ] syncStatus: enum (COMPLETE/PARTIAL/NONE)
  - [ ] lastActivity: Time
- [ ] Mutation.syncThread returns SyncThreadPayload
  - [ ] success: Boolean
  - [ ] notesAdded: Int
  - [ ] notesUpdated: Int
  - [ ] errorCount: Int
  - [ ] syncStatus: enum
- [ ] Mutation.syncMissingReplies returns SyncRepliesPayload
  - [ ] success: Boolean
  - [ ] repliesSynced: Int
  - [ ] newRepliesCount: Int

**Code Quality**:
- [ ] Follows resolver pattern (like Query.actor, etc.)
- [ ] Uses converters for model population
- [ ] Proper error handling
- [ ] No schema violations
- [ ] Input validation
- [ ] Logging at appropriate levels
- [ ] Calls service correctly

**Testing**:
- [ ] All tests passing
- [ ] 80%+ coverage
- [ ] No regressions
- [ ] Federation errors handled
- [ ] Circular refs handled

### Report When Complete

**Tell PM**:
```
Phase 1.2.2 Complete: GraphQL Resolvers

COMPLETED:
✅ Query.threadContext resolver
✅ Mutation.syncThread resolver
✅ Mutation.syncMissingReplies resolver
✅ Model conversions (ThreadContext → GraphQL)
✅ Tests (80%+ coverage)

FILES MODIFIED:
- graph/schema.resolvers.go (added 3 resolvers + converters)
- graph/schema.resolvers_thread_test.go (new tests)

TEST RESULTS:
- Resolver tests: X/X passing
- Integration tests: X/X passing
- Coverage: X%
- No regressions detected

PHASE 1.2 COMPLETE - READY FOR PRODUCTION AUDIT
```

---

## 📋 How to Execute

### Timeline

**Day 1 (Phase 1.2.1 Start)**
- Agent starts Phase 1.2.1
- Builds service + storage foundation
- Estimate: 1-2 days

**Interim (PM Review)**
- PM reviews Phase 1.2.1
- Validates against criteria
- Checks for issues
- Approves or requests fixes

**Day 3-4 (Phase 1.2.2 Start)**
- Agent starts Phase 1.2.2 (only if 1.2.1 approved)
- Builds GraphQL resolvers
- Estimate: 1-2 days

**Final (PM Final Audit)**
- PM reviews Phase 1.2.2
- Validates against criteria
- Full regression testing
- Marks Phase 1.2 complete

### Instructions for Agent

**Phase 1.2.1**:
```
Break down Phase 1.2 into two parts.

PART 1: Thread Service & Storage (your work now)
Document: /docs/PHASE_1_2_BREAKDOWN.md - Phase 1.2.1 section

BUILD:
1. Thread service (traversal logic)
2. Storage models (3 models)
3. Repository methods (6 methods)
4. Federation integration
5. Tests (80%+ coverage)

WHEN COMPLETE:
- Run tests
- Report results with coverage %
- List files created/modified
- Wait for PM approval before Part 2
```

**Phase 1.2.2** (after Phase 1.2.1 approved):
```
PART 2: GraphQL Resolvers (your work next)
Document: /docs/PHASE_1_2_BREAKDOWN.md - Phase 1.2.2 section

BUILD:
1. Query.threadContext resolver
2. Mutation.syncThread resolver
3. Mutation.syncMissingReplies resolver
4. Model conversion helpers
5. Tests (80%+ coverage)

WHEN COMPLETE:
- Run tests
- Check for regressions
- Report results
- Phase 1.2 ready for production audit
```

---

## 🎯 Definition of Done (Phase 1.2 Overall)

Phase 1.2 is **COMPLETE AND SHIP-READY** when:

✅ **Part 1 (Service & Storage)**
- [ ] Service layer fully functional
- [ ] Storage models working
- [ ] Federation integration complete
- [ ] Tests passing (80%+)
- [ ] Approved by PM

✅ **Part 2 (Resolvers)**
- [ ] All 3 resolvers working
- [ ] All models fully populated
- [ ] No schema violations
- [ ] Tests passing (80%+)
- [ ] No regressions
- [ ] Approved by PM

✅ **Combined**
- [ ] Phase 1.2 operations: 3/3 complete
- [ ] Phase 1 operations: 13/13 complete ✅
- [ ] Ready to move to Phase 2

---

## 📊 Updated Progress After Phase 1.2

**Expected After Completion**:
- Phase 1: ✅ 100% complete (13 operations)
- Operations total: 70 → 73 (73%)
- Ready to begin Phase 2: Federation & Monitoring
