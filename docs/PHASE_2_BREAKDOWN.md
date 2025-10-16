# Phase 2 Breakdown: Federation & Monitoring

**Status**: Phase 1 complete, Phase 2 queued  
**Total Operations**: 10 (mixed implementation/partial)  
**Total Duration**: 12-17 days split into 4 independent chunks  
**Dependency**: Phase 1 must be complete

---

## 📊 Phase 2 Strategy: Independent Chunks

**Original approach**: 12-17 days in one go (overwhelming)  
**New approach**: 4 independent features, 2-5 days each

**Why This Works**:
- ✅ Each feature can be completed in one agent session
- ✅ Independent - no blocking dependencies between chunks
- ✅ Can work in any order
- ✅ PM validates each before next
- ✅ Easy to parallelize if needed later

---

## 🎯 Phase 2.1: Alert Subscriptions (Chunk 1)

**Duration**: 2 days  
**Complexity**: ⭐ LOW (extends existing patterns)  
**Operations**: 4 subscriptions  
**Dependency**: None (start immediately after Phase 1)  
**Blocks**: Nothing

### What Gets Built

```
├── 4 New Subscription Resolvers
│   ├── Subscription.moderationAlerts(severity)
│   ├── Subscription.costAlerts(thresholdUSD)
│   ├── Subscription.budgetAlerts(domain)
│   └── Subscription.federationHealthUpdates(domain)
│
├── Subscription Manager Methods (if needed)
│   └── graph/subscription_manager.go updates
│
├── Alert Event Types
│   ├── ModerationAlert
│   ├── CostAlert
│   ├── BudgetAlert
│   └── FederationHealthUpdate
│
└── Tests
    └── Subscription tests (80%+ coverage)
```

### Phase 2.1 Acceptance Criteria

- [ ] All 4 subscriptions working
- [ ] Alerts stream in real-time
- [ ] Filtering works correctly
- [ ] Threshold/severity checks work
- [ ] Tests passing (80%+)
- [ ] No regressions

---

## 🎯 Phase 2.2: Severed Relationships (Chunk 2)

**Duration**: 3-4 days  
**Complexity**: ⭐⭐ MODERATE (new feature, federation dependent)  
**Operations**: 4 (2 queries + 2 mutations)  
**Dependency**: None (independent)  
**Blocks**: Nothing

### What Gets Built

```
├── Service Layer
│   └── pkg/services/severance/service.go
│       ├── DetectSeveredRelationships()
│       ├── GetSeveredRelationships()
│       ├── AcknowledgeSeverance()
│       └── AttemptReconnection()
│
├── Storage Models
│   ├── SeveredRelationship
│   ├── AffectedRelationship
│   └── ReconnectionAttempt
│
├── Repository Methods
│   └── pkg/storage/repositories/severance_repository.go
│
├── GraphQL Operations (4 ops)
│   ├── Query.severedRelationships()
│   ├── Query.affectedRelationships()
│   ├── Mutation.acknowledgeSeverance()
│   └── Mutation.attemptReconnection()
│
└── Tests
    └── service_test.go + resolver_test.go (80%+ coverage)
```

### Phase 2.2 Acceptance Criteria

- [ ] Can detect severed relationships
- [ ] Stores severance information
- [ ] Queries return correct data
- [ ] Mutations work correctly
- [ ] Reconnection attempts work
- [ ] Tests passing (80%+)
- [ ] No regressions

---

## 🎯 Phase 2.3: Media Streaming (Chunk 3)

**Duration**: 4-5 days  
**Complexity**: ⭐⭐⭐ HIGH (AWS integration)  
**Operations**: 3 queries + mutations  
**Dependency**: None (independent)  
**Blocks**: Nothing

### What Gets Built

```
├── AWS MediaConvert Integration
│   └── pkg/media/transcoding/transcoder.go
│       ├── SetupTranscodingJob()
│       ├── GetTranscodingStatus()
│       └── GetStreamingUrls()
│
├── HLS/DASH Manifest Generation
│   └── pkg/media/streaming/manifest_generator.go
│       ├── GenerateHLSManifest()
│       ├── GenerateDASHManifest()
│       └── GenerateSegments()
│
├── CloudFront Distribution
│   └── pkg/media/streaming/cdn.go
│       ├── SetupDistribution()
│       ├── GenerateSignedURLs()
│       └── GetStreamingURL()
│
├── GraphQL Operations (3 core)
│   ├── Query.mediaStreamUrl()
│   ├── Query.supportedBitrates()
│   ├── Mutation.requestStreamingUrl()
│   ├── Mutation.preloadMedia()
│   └── Analytics helpers
│
└── Tests
    └── integration_test.go + unit_tests (80%+ coverage)
```

### Phase 2.3 Acceptance Criteria

- [ ] MediaConvert jobs working
- [ ] HLS manifests generate correctly
- [ ] DASH manifests generate correctly
- [ ] CloudFront URLs work
- [ ] Signed URLs work
- [ ] Bitrate detection works
- [ ] Preload works
- [ ] Tests passing (80%+)
- [ ] No regressions

---

## 🎯 Phase 2.4: Moderation ML (Chunk 4)

**Duration**: 3-4 days  
**Complexity**: ⭐⭐⭐ HIGH (ML integration)  
**Operations**: 2 operations  
**Dependency**: None (independent)  
**Blocks**: Nothing

### What Gets Built

```
├── ML Training Pipeline
│   └── pkg/moderation/ml/trainer.go
│       ├── CollectTrainingData()
│       ├── BalanceDataset()
│       ├── TrainModel()
│       ├── ValidateModel()
│       └── DeployModel()
│
├── Model Versioning
│   └── pkg/moderation/ml/versions.go
│
├── Effectiveness Tracking
│   └── pkg/moderation/ml/effectiveness.go
│       ├── TrackOutcome()
│       ├── CalculateMetrics()
│       ├── GetEffectiveness()
│       └── GetTrends()
│
├── GraphQL Operations (2 ops)
│   ├── Mutation.trainModerationModel()
│   └── Query.moderationEffectiveness()
│
└── Tests
    └── service_test.go + resolver_test.go (80%+ coverage)
```

### Phase 2.4 Acceptance Criteria

- [ ] ML training pipeline works
- [ ] Model versioning works
- [ ] Effectiveness metrics calculated
- [ ] Training mutation works
- [ ] Effectiveness query works
- [ ] Tests passing (80%+)
- [ ] No regressions

---

## 📋 Execution Order (Agent Choice)

Agents can complete these in any order (all independent):

**Recommended Order** (by complexity):
1. **Phase 2.1** (2 days) - Quick win, confidence builder
2. **Phase 2.2** (3-4 days) - Moderate complexity
3. **Phase 2.3** (4-5 days) - High complexity, AWS
4. **Phase 2.4** (3-4 days) - High complexity, ML

**Or by interest**:
- Analytics/Federation people: 2.2 first
- Media/Streaming people: 2.3 first
- Moderation people: 2.4 first
- Monitoring people: 2.1 first

**Parallel possible** (if 2+ agents):
- Agent A: Phase 2.1 + 2.3
- Agent B: Phase 2.2 + 2.4

---

## 🎯 General Acceptance Criteria (All Chunks)

**Code Quality** (applies to all):
- [ ] Follows established patterns (service, repository, resolver)
- [ ] Uses services.Registry for DI
- [ ] Proper error handling with custom types
- [ ] Input validation
- [ ] Logging at appropriate levels
- [ ] Cost tracking where applicable
- [ ] No N+1 queries
- [ ] No regressions on other features

**Testing** (applies to all):
- [ ] Unit tests: 80%+ coverage
- [ ] Integration tests (if applicable)
- [ ] All tests passing
- [ ] No regressions

**Documentation** (applies to all):
- [ ] Code comments
- [ ] Schema documentation
- [ ] Error types documented
- [ ] README if new package

---

## 📋 Agent Instructions (Generic)

**For each chunk**:

```
Phase 2.X: [Feature Name]

Duration: [X] days
Complexity: [Rating]
Operations: [Count]

DOCUMENT: /docs/PHASE_2_BREAKDOWN.md (Phase 2.X section)

BUILD:
1. [Step 1]
2. [Step 2]
3. [Step 3]
...

WHEN COMPLETE:
- Run tests (report: X/X passing, X% coverage)
- Check regressions
- List files created/modified
- Report results
- Ready for next chunk

REFERENCE:
- Similar features: [Examples from codebase]
- GraphQL patterns: /graph/schema.resolvers.go
- Service patterns: /pkg/services/notes/service.go
```

---

## 📊 Progress Tracking

### Phase 2 Status Board

| Chunk | Feature | Status | Estimate | Agent |
|-------|---------|--------|----------|-------|
| 2.1 | Alert Subscriptions | ⏳ Queued | 2 days | TBD |
| 2.2 | Severed Relationships | ⏳ Queued | 3-4 days | TBD |
| 2.3 | Media Streaming | ⏳ Queued | 4-5 days | TBD |
| 2.4 | Moderation ML | ⏳ Queued | 3-4 days | TBD |
| **TOTAL** | **10 ops** | **⏳ Queued** | **12-17 days** | **4 agents/sequential** |

---

## 🎯 Definition of Done (Phase 2 Overall)

Phase 2 is **COMPLETE AND READY FOR PHASE 3** when:

✅ **Phase 2.1 (Alert Subscriptions)**
- [ ] All 4 subscriptions working
- [ ] Tests passing (80%+)
- [ ] Approved by PM

✅ **Phase 2.2 (Severed Relationships)**
- [ ] All 4 operations working
- [ ] Tests passing (80%+)
- [ ] Approved by PM

✅ **Phase 2.3 (Media Streaming)**
- [ ] All operations working
- [ ] AWS integration complete
- [ ] Tests passing (80%+)
- [ ] Approved by PM

✅ **Phase 2.4 (Moderation ML)**
- [ ] Training pipeline working
- [ ] Effectiveness tracking working
- [ ] Tests passing (80%+)
- [ ] Approved by PM

✅ **Combined**
- [ ] Phase 2: 10 operations complete
- [ ] Phase 1+2: 23 operations complete (74%)
- [ ] Ready to begin Phase 3

---

## 📈 Recommended Timeline (Sequential)

**If one agent working through all**:
- Week 1: Phase 2.1 (Days 1-2) + Phase 2.2 (Days 3-6)
- Week 2: Phase 2.3 (Days 7-11) + Phase 2.4 (Days 12-15)
- Buffer: Days 16-17 for fixes/regressions

**If multiple agents**:
- All 4 can work in parallel
- Each reports when complete
- PM validates
- Minimal path: 4-5 days (longest chunk)

---

## 🚀 Next Steps

After Phase 1 ships:
1. **Agent reviews** `/docs/PHASE_2_BREAKDOWN.md`
2. **Picks a chunk** (2.1, 2.2, 2.3, or 2.4)
3. **Executes that chunk** (2-5 days)
4. **Reports completion**
5. **PM validates**
6. **Picks next chunk** (repeat)

Simple, manageable, clear scope for each agent session.
