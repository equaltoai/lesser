# Phase 2.3 Error Handling - Systematic Elimination Plan

## 🎯 OBJECTIVE: ZERO fmt.Errorf Remaining

**Current State**: 585 fmt.Errorf calls across 119 files  
**Target State**: 0 fmt.Errorf calls  
**Approach**: Systematic elimination by package priority

## 📊 SYSTEMATIC BREAKDOWN BY PACKAGE

### BATCH 1: Services Package (pkg/services/) - 185 fmt.Errorf
**Priority**: Highest (core business logic)

| File | Count | Batch |
|------|-------|-------|
| importexport/service.go | 24 | 1A |
| conversations/service.go | 22 | 1A |
| emoji/service.go | 22 | 1A |
| scheduled/service.go | 14 | 1B |
| cost/realtime_aggregation_service.go | 13 | 1B |
| quotes/quote_service.go | 11 | 1B |
| notifications/service.go | 11 | 1B |
| notes/service.go | 9 | 1C |
| aws_storage_client.go | 7 | 1C |
| search/service.go | 6 | 1C |
| bulk/service.go | 5 | 1C |
| aws_queue_service.go | 5 | 1C |
| cost/analytics_service.go | 3 | 1C |
| registry.go | 3 | 1C |
| storage_adapter.go | 2 | 1C |
| [Others] | ~28 | 1C |

**Batch 1A Target**: 68 → 0 fmt.Errorf (3 largest files)  
**Batch 1B Target**: 49 → 0 fmt.Errorf (4 medium files)  
**Batch 1C Target**: 68 → 0 fmt.Errorf (remaining files)

### BATCH 2: Commands Package (cmd/) - 150 fmt.Errorf
**Priority**: High (Lambda entry points)

| File | Count | Batch |
|------|-------|-------|
| push-delivery/main.go | 25 | 2A |
| moderation-processor/main.go | 22 | 2A |
| stream-router/main.go | 19 | 2A |
| notification-processor/main.go | 13 | 2B |
| websocket-cost-aggregator/main.go | 10 | 2B |
| note-processor/main.go | 9 | 2B |
| streaming/main.go | 8 | 2B |
| outbox/main.go | 8 | 2B |
| [Others] | ~36 | 2C |

**Batch 2A Target**: 66 → 0 fmt.Errorf (3 largest Lambda functions)  
**Batch 2B Target**: 48 → 0 fmt.Errorf (5 medium Lambda functions)  
**Batch 2C Target**: 36 → 0 fmt.Errorf (remaining Lambda functions)

### BATCH 3: Auth Package (pkg/auth/) - 120 fmt.Errorf
**Priority**: High (security critical)

| File | Count | Batch |
|------|-------|-------|
| webauthn.go | 18 | 3A |
| secrets_manager.go | 16 | 3A |
| service.go | 14 | 3A |
| wallet.go | 12 | 3A |
| social_recovery.go | 8 | 3B |
| session.go | 8 | 3B |
| session_lifecycle.go | 8 | 3B |
| recovery_federation.go | 8 | 3B |
| recovery_codes.go | 6 | 3B |
| session_security.go | 4 | 3B |
| [Others] | ~18 | 3B |

**Batch 3A Target**: 60 → 0 fmt.Errorf (4 largest auth files)  
**Batch 3B Target**: 60 → 0 fmt.Errorf (remaining auth files)

### BATCH 4: Federation Package (pkg/federation/) - 80 fmt.Errorf
**Priority**: Medium (federation logic)

| File | Count | Batch |
|------|-------|-------|
| routing/route_manager.go | 28 | 4A |
| routing/query_optimizer.go | 10 | 4A |
| routing/instance_registry.go | 9 | 4A |
| signature_service.go | 9 | 4A |
| sync/threads.go | 6 | 4B |
| routing/metrics.go | 5 | 4B |
| [Others] | ~13 | 4B |

**Batch 4A Target**: 56 → 0 fmt.Errorf (4 largest federation files)  
**Batch 4B Target**: 24 → 0 fmt.Errorf (remaining federation files)

### BATCH 5: Other Packages - 50 fmt.Errorf
**Priority**: Lower (activitypub, graph, misc)

| Package | Estimated Count | Batch |
|---------|----------------|-------|
| pkg/activitypub/ | ~20 | 5A |
| graph/ | ~15 | 5A |
| pkg/misc | ~15 | 5A |

**Batch 5A Target**: 50 → 0 fmt.Errorf (all remaining packages)

## 🚀 EXECUTION STRATEGY

### Phase 1: Services Package (Week 1)
**Days 1-2**: Batch 1A (importexport, conversations, emoji) - 68 → 0
**Days 3-4**: Batch 1B (scheduled, cost, quotes, notifications) - 49 → 0  
**Days 5-7**: Batch 1C (notes, aws_storage, search, bulk, others) - 68 → 0

### Phase 2: Commands Package (Week 2)
**Days 1-2**: Batch 2A (push-delivery, moderation-processor, stream-router) - 66 → 0
**Days 3-4**: Batch 2B (notification-processor, websocket-cost, note-processor, streaming, outbox) - 48 → 0
**Days 5-7**: Batch 2C (remaining Lambda functions) - 36 → 0

### Phase 3: Auth Package (Week 3)  
**Days 1-3**: Batch 3A (webauthn, secrets_manager, service, wallet) - 60 → 0
**Days 4-7**: Batch 3B (remaining auth files) - 60 → 0

### Phase 4: Federation & Cleanup (Week 4)
**Days 1-3**: Batch 4A (route_manager, query_optimizer, instance_registry, signature_service) - 56 → 0
**Days 4-5**: Batch 4B (remaining federation files) - 24 → 0
**Days 6-7**: Batch 5A (activitypub, graph, misc packages) - 50 → 0

## 📋 IMPLEMENTATION TASKS

### Pre-Work: Error Constants Expansion
**Before starting elimination, ensure services/errors.go has constants for:**
- Import/export operations (20+ constants)
- Conversation management (15+ constants)
- Emoji processing (15+ constants)
- Scheduled jobs (10+ constants)
- Cost tracking (10+ constants)
- Quote operations (10+ constants)
- Notification processing (15+ constants)
- Auth operations (25+ constants)
- Federation routing (20+ constants)
- Lambda operations (15+ constants)

**Target**: 500+ total error constants before elimination begins

### Elimination Pattern for Each File:
1. **Analyze fmt.Errorf patterns** in the file
2. **Add missing error constants** to appropriate package (services/auth/federation/errors.go)
3. **Replace ALL fmt.Errorf** with typed constants
4. **Use wrapping** for context: `fmt.Errorf("%w: context", services.ErrConstant)`
5. **Verify compilation** and functionality
6. **Confirm 0 fmt.Errorf** in file before moving to next

## 🎯 SUCCESS METRICS

### Daily Targets:
- **Week 1**: Services 185 → 0 (26 files/day average)
- **Week 2**: Commands 150 → 0 (21 files/day average)  
- **Week 3**: Auth 120 → 0 (17 files/day average)
- **Week 4**: Federation+Cleanup 130 → 0 (18 files/day average)

### Weekly Verification:
```bash
# Week 1 - Services should be 0
rg "fmt\.Errorf" /Users/aronprice/lesser/pkg/services/ --type go | wc -l

# Week 2 - Commands should be 0  
rg "fmt\.Errorf" /Users/aronprice/lesser/cmd/ --type go | wc -l

# Week 3 - Auth should be 0
rg "fmt\.Errorf" /Users/aronprice/lesser/pkg/auth/ --type go | wc -l

# Week 4 - Everything should be 0
rg "fmt\.Errorf" /Users/aronprice/lesser/cmd/ /Users/aronprice/lesser/graph/ /Users/aronprice/lesser/pkg/services/ /Users/aronprice/lesser/pkg/auth/ /Users/aronprice/lesser/pkg/federation/ /Users/aronprice/lesser/pkg/activitypub/ --type go | wc -l
```

### Final Completion Criteria:
- [ ] **0 fmt.Errorf** in pkg/services/
- [ ] **0 fmt.Errorf** in cmd/
- [ ] **0 fmt.Errorf** in pkg/auth/
- [ ] **0 fmt.Errorf** in pkg/federation/
- [ ] **0 fmt.Errorf** in pkg/activitypub/
- [ ] **0 fmt.Errorf** in graph/
- [ ] **500+ error constants** across all packages
- [ ] **100% compilation** success
- [ ] **All tests pass** after elimination

## 🔥 NO SHORTCUTS - NO EXCEPTIONS

**Every single fmt.Errorf must be eliminated.**  
**Every single file must reach 0.**  
**No "good enough" - only 100% completion.**

**Progress tracking: 585 → 0**