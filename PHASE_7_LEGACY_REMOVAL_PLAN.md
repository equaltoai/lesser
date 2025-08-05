# Phase 7: Complete Legacy Code Removal Plan

## Overview
This plan addresses the complete removal of all legacy storage patterns from the codebase, finishing the migration to the repository pattern with DynamORM.

## Current State Analysis

### 1. Major Legacy Components Remaining
- **StorageAdapter** (`pkg/storage/dynamorm/adapter.go`) - 10,052 lines
- **Direct AWS SDK usage** in 20+ files
- **Adapter patterns** in command-line tools
- **Test infrastructure** with commented legacy references
- **Build scripts** referencing deleted interfaces

### 2. Critical Path Dependencies
```
StorageAdapter removal depends on:
├── All handlers using repositories directly ✓
├── Command-line tools updated to repositories
├── Federation services using repository pattern
└── Test infrastructure fully migrated
```

## Execution Plan

### Phase 7.2: Remove Massive StorageAdapter (Priority: HIGH)
**Goal**: Eliminate the 10k+ line adapter.go file

1. **Inventory Usage**
   - Find all imports of `dynamorm.StorageAdapter`
   - Identify which methods are still being called
   - Map to repository equivalents

2. **Migration Strategy**
   - Start with least-used methods
   - Move any unique logic to appropriate repositories
   - Update callers incrementally

3. **Validation**
   - Ensure no compilation errors
   - Run integration tests
   - Delete adapter.go

**Estimated effort**: 2-3 days

### Phase 7.3: Replace Direct AWS SDK DynamoDB Usage (Priority: HIGH)
**Goal**: Convert all direct DynamoDB calls to DynamORM

**Files to update**:
```
cmd/outbox/integration_test.go
cmd/stream-router/main.go
cmd/api/lift/metrics.go
cmd/api/lift/misc.go
cmd/api/lift/exports.go
cmd/api/lift/imports.go
cmd/api/lift/ai.go
cmd/cost-aggregator/main.go
graph/resolver.go
pkg/translation/aws_translate.go
pkg/cost/storage.go
pkg/cost/dynamodb_wrapper.go
```

**Strategy**:
1. Group by functionality (cost tracking, metrics, exports, etc.)
2. Create/update repositories for each group
3. Replace AWS SDK calls with repository methods
4. Test each component independently

**Estimated effort**: 3-4 days

### Phase 7.4: Update Command-line Tools (Priority: HIGH)
**Goal**: Remove adapter patterns from CLI tools

**Tools to update**:
```
cmd/moderation-processor/main.go (repositoryStorageAdapter)
cmd/federation-delivery/main.go (FederationStorageAdapter)
cmd/auth/main.go
cmd/configure-instance/main.go
cmd/init-deploy/main.go
cmd/push-delivery/main.go
cmd/webfinger/main.go
```

**Strategy**:
1. Create minimal interfaces for each tool's needs
2. Wire repositories directly
3. Remove adapter layers
4. Test each tool independently

**Estimated effort**: 2 days

### Phase 7.5: Clean Up Test File References (Priority: MEDIUM)
**Goal**: Remove all commented legacy code from tests

**Tasks**:
1. Remove all `// MockStorageAdapter` comments
2. Delete `.backup` files
3. Remove `.disabled` test files or migrate them
4. Clean up mock implementations
5. Update test documentation

**Estimated effort**: 1 day

### Phase 7.6: Update Build Scripts and Tooling (Priority: MEDIUM)
**Goal**: Fix scripts that reference old patterns

**Files to update**:
```
scripts/generate_mocks.go (references deleted interface.go)
examples/serverless_circuit_breaker_example.go
Any CI/CD scripts
```

**Estimated effort**: 0.5 days

### Phase 7.7: Final Validation and Cleanup (Priority: HIGH)
**Goal**: Ensure complete removal and system stability

**Checklist**:
- [ ] No references to `storage.Storage` interface
- [ ] No imports of `aws-sdk-go.*dynamodb` (except in DynamORM internals)
- [ ] No `StorageAdapter` usage
- [ ] All tests compile and pass
- [ ] Documentation updated
- [ ] Performance benchmarks run

**Estimated effort**: 1 day

## Implementation Order

### Week 1
1. **Day 1-2**: Phase 7.3 - Start replacing direct AWS SDK usage
   - Focus on metrics.go, misc.go, exports.go first
   - These are isolated and easier to test

2. **Day 3-4**: Phase 7.4 - Update command-line tools
   - Start with federation-delivery and moderation-processor
   - These have clear adapter patterns to remove

3. **Day 5**: Phase 7.5 - Clean up test files
   - Quick wins to reduce codebase noise

### Week 2
1. **Day 6-7**: Phase 7.2 - Begin StorageAdapter removal
   - Most complex task, needs focused effort
   - Start with least-used methods

2. **Day 8**: Phase 7.2 continued
   - Complete adapter removal
   - Fix any compilation issues

3. **Day 9**: Phase 7.6 & 7.7
   - Update scripts and tooling
   - Run final validation

## Success Criteria

1. **Zero Legacy Code**
   - No `storage.Storage` interface references
   - No `StorageAdapter` usage
   - No direct AWS SDK DynamoDB calls

2. **Clean Architecture**
   - All components use repository pattern
   - Clear separation of concerns
   - No adapter layers

3. **Functional System**
   - All tests pass
   - All CLI tools work
   - No performance regression

## Risk Mitigation

1. **Gradual Migration**
   - Make small, testable changes
   - Keep the system functional at each step
   - Commit frequently

2. **Testing Strategy**
   - Run tests after each component update
   - Use integration tests for CLI tools
   - Monitor for performance issues

3. **Rollback Plan**
   - Tag current state before starting
   - Keep detailed notes of changes
   - Be prepared to revert if needed

## Notes

- The StorageAdapter removal (7.2) is the most complex task
- Direct AWS SDK usage (7.3) has the most files but simpler changes
- Command-line tools (7.4) are critical for operations
- Test cleanup (7.5) can be done incrementally
- Final validation (7.7) is crucial for confidence

Total estimated effort: 9-11 days