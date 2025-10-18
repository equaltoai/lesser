# Agent Implementation Prompts

## 🎯 Phase 1: Mastodon Parity (Week 1-2)

### PHASE 1.1: Hashtag Following System

⚠️ **STATUS**: Phase 1.1 implementation in progress with required fixes.

**Current Work Breakdown**:

**PART A: Issues #1-4 Remediation** (Fixes before 1.1.1)
- Document: `/docs/PHASE_1_1_FINAL_FIXES.md`
- Fixes: Mutation payloads, notification settings, event payloads
- Duration: 2.5-3 hours
- Must complete before 1.1.1

**PART B: Phase 1.1.1 Blocker - Subscription Harmonization** (After Part A)
- Document: `/docs/PHASE_1_1_1_BLOCKER.md`
- Task: Align hashtag subscriptions with SubscriptionManager
- Duration: 4-6 hours
- Blocker for Phase 1.1 completion

**Original PROMPT 1.1** (Reference only):
- Original implementation outline: See lines below (for historical reference)
- ⚠️ NOT current - use the specific fix documents above
- Schema already exists: `/graph/schema.graphql` lines 653-663

---

### PROMPT 1.2: Thread Synchronization

```
TASK: Implement Thread Synchronization for Lesser GraphQL (Phase 1.2)

CONTEXT:
- Phase: Phase 1: Mastodon Parity (Critical for federation completeness)
- Effort: 3-4 days (Day 1-4)
- Operations: 3 total (all missing)
- Current Status: Schema defined, no implementation
- Dependencies: Federation service, ActivityPub handlers
- Blocking: Phase 1 completion, Full thread context visibility
- Dependency: Phase 1.1 must be complete and shipped first

REQUIREMENTS:

1. THREAD SERVICE (Day 1-2):
   Create pkg/services/threads/service.go implementing:
   
   type Service interface {
       FindThreadRoot(ctx context.Context, noteID string) (string, error)
       GetThreadTree(ctx context.Context, rootID string, depth int) (*ThreadTree, error)
       FindMissingReplies(ctx context.Context, rootID string) ([]*MissingReply, error)
       SyncRemoteThread(ctx context.Context, noteURL string, depth int) (*SyncResult, error)
       SyncMissingReplies(ctx context.Context, noteID string) (*SyncResult, error)
       GetThreadContext(ctx context.Context, noteID string) (*ThreadContext, error)
   }

2. STORAGE MODELS (Day 1):
   Create in pkg/storage/models/:
   - ThreadSync: Track sync operations (PK: thread#{rootID}, SK: sync#{timestamp})
   - ThreadNode: Metadata for notes in threads (PK: note#{noteID}, SK: thread_meta)
   - MissingReply: Track detected gaps (PK: thread#{rootID}, SK: missing#{noteID})

3. FEDERATION INTEGRATION (Day 2):
   Integrate with existing federation package:
   - FetchRemoteNote(ctx, noteURL) - GET note via ActivityPub
   - FetchRemoteReplies(ctx, noteURL) - GET replies collection
   - ValidateThreadIntegrity(ctx, notes) - Verify consistency
   - Handle errors: unreachable instances, deleted notes, auth failures, circular refs

4. THREAD LOGIC:
   - Walk up inReplyTo chain to find root
   - Walk down replies collections to find all descendants
   - Detect gaps: replies collection shows more replies than stored
   - Calculate sync status: complete vs partial
   - Track last activity timestamp

5. GRAPHQL RESOLVERS (Day 2-3):
   Implement in graph/schema.resolvers.go:
   
   Query Resolver:
   - threadContext(noteId) -> ThreadContext returning:
     - rootPost: Object (the original post)
     - ancestors: [Object] (posts this replies to, ordered)
     - descendants: [Object] (replies, ordered by depth then date)
     - participantsCount: Int
     - missingCount: Int
     - syncStatus: COMPLETE | PARTIAL | NONE
     - lastActivity: Time

   Mutation Resolvers:
   - syncThread(noteUrl, depth) -> SyncThreadPayload returning:
     - success: Boolean
     - notesAdded: Int
     - notesUpdated: Int
     - errorCount: Int
     - syncStatus: SyncStatus
   
   - syncMissingReplies(noteId) -> SyncRepliesPayload returning:
     - success: Boolean
     - repliesSynced: Int
     - newRepliesCount: Int

6. ERROR HANDLING:
   Create pkg/services/threads/errors.go with:
   - ErrThreadNotFound
   - ErrInvalidThreadDepth
   - ErrRemoteInstanceUnreachable
   - ErrThreadIntegrityViolation
   - ErrCircularReference

ACCEPTANCE CRITERIA:

FUNCTIONALITY:
- [ ] Can identify thread root by walking inReplyTo chain
- [ ] Can build complete thread context (ancestors + descendants)
- [ ] Correctly detects missing replies in collection
- [ ] Respects depth limit in traversal (max recommended: 10)
- [ ] Handles deleted/inaccessible remote notes gracefully
- [ ] Can sync remote thread by URL
- [ ] Can fill in missing replies
- [ ] Thread context includes all required metadata
- [ ] Sync operations are idempotent

TESTING:
- [ ] Unit tests for thread traversal logic (80%+ coverage)
- [ ] Test finding root with various chain lengths
- [ ] Test tree building with different depths
- [ ] Test missing reply detection
- [ ] Test federation error handling
- [ ] Test circular reference detection
- [ ] Test idempotency (syncing twice doesn't duplicate)
- [ ] Integration tests with mock ActivityPub responses

CODE QUALITY:
- [ ] Uses services.Registry for dependencies
- [ ] Proper error handling with custom types
- [ ] Input validation for URLs and depths
- [ ] Cost tracking for remote fetches
- [ ] Logging at appropriate levels
- [ ] No unbounded traversal (respects depth)
- [ ] Pagination not needed (trees are bounded)
- [ ] Follows resolver pattern from schema.resolvers.go

DOCUMENTATION:
- [ ] Comments on traversal algorithm
- [ ] Schema documentation for ThreadContext
- [ ] Error type documentation
- [ ] README.md with usage examples
- [ ] Any federation integration notes

REFERENCE FILES:
- Schema: /graph/schema.graphql (lines 665-667)
- Federation: /pkg/federation/ (understand existing federation handlers)
- ActivityPub: /pkg/activitypub/ (understand collection handling)
- Example Service: /pkg/services/notes/service.go
- Example Resolver Pattern: /graph/schema.resolvers.go
- Existing Thread Example: Look at Object.inReplyTo navigation

ARCHITECTURAL NOTES:
- Thread fetching should be async with job queue (can return 202 Accepted initially)
- Cache thread contexts for performance
- Use dataloader to prevent N+1 when loading tree nodes
- Track sync status to show to users
- Limit depth to prevent DOS (suggest max depth 10)

EXECUTION STRATEGY:
1. Build thread traversal logic (walk chains) - foundational
2. Add federation integration - requires ActivityPub knowledge
3. Create storage models - straightforward
4. Implement service methods - uses above
5. Add GraphQL resolvers - uses service
6. Write comprehensive tests - should cover all scenarios
7. Add documentation - last step

WHEN COMPLETE:
Report back with:
- Files created/modified list
- Test coverage percentage (target: 80%+)
- Maximum depth tested
- Any federation integration notes
- Any issues or edge cases discovered
- Ready for Phase 2 confirmation
```

---

## 🎯 Phase 2: Federation & Monitoring (Week 3-4)

### PROMPT 2.1: Phase 2 Alert Subscriptions

[To be generated after Phase 1 completion]

### PROMPT 2.2: Media Streaming Completion

[To be generated after Phase 1 completion]

---

## 🎯 Phase 3: Visualization & Analytics (Week 5-6)

[Prompts to be generated as we progress]

---

## 📋 How to Use These Prompts

### For Phase 1.1 (Current Work)
1. **Agent**: Read `/docs/PHASE_1_1_FINAL_FIXES.md` first (4 issues to fix)
2. **Agent**: Read `/docs/PHASE_1_1_1_BLOCKER.md` second (subscription refactor)
3. **Agent**: Execute fixes in order
4. **PM**: Review against checklists
5. **Agent**: Report back when complete

### For Phase 1.2 (Starting Next)
1. **Agent**: Copy PROMPT 1.2 above
2. **Agent**: Execute REQUIREMENTS step by step
3. **Agent**: Meet all ACCEPTANCE CRITERIA before reporting
4. **Agent**: Report completion with metrics
5. **PM**: Mark todo complete, generate Phase 2 prompts

### For Future Phases
1. **PM**: Generates specific prompt document (like PHASE_1_1_FINAL_FIXES.md)
2. **PM**: References it from AGENT_PROMPTS.md
3. **Agent**: Executes
4. **Cycle** repeats

## 🔄 Feedback Loop

After each implementation:
1. PM reviews code against criteria
2. PM identifies any gaps
3. PM generates followup prompt if needed
4. PM updates CLAUDE.md status dashboard
5. PM schedules next feature
