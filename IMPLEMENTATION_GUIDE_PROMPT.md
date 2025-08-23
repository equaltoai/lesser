# Implementation Guide Prompt for Lesser Application Consistency Initiative

## Session Context and Background

You are continuing a major consistency initiative for the Lesser application (a complete serverless ActivityPub implementation). A comprehensive consistency audit has been completed, revealing critical areas for improvement. Your session will begin implementing the systematic improvements outlined in the detailed task list.

### Previous Session Achievements
- **100 agents successfully deployed** for error standardization
- **4 packages completely standardized**: AUTH, CMD, FEDERATION, SERVICES (789+ fmt.Errorf eliminated)
- **Comprehensive consistency audit completed** with detailed findings
- **5-phase implementation plan created** with 108+ specific tasks

## Current Application State

### Critical Metrics (From Audit)
- **Error Standardization**: 40% complete (4/10 major packages standardized)
- **DynamORM Migration**: 30% complete (models exist, storage adapter missing)
- **fmt.Errorf Calls**: 1,729 remaining across codebase
- **AWS SDK Violations**: 23 imports (should be 0)
- **Graph Package**: 476 fmt.Errorf calls (highest concentration)
- **Architecture Consistency**: 85% compliant

### Success Stories to Build Upon
- ✅ **Services Package**: 440 error constants, 0 fmt.Errorf (exemplary implementation)
- ✅ **AUTH Package**: 155 error constants, 0 fmt.Errorf (complete)
- ✅ **Federation Package**: 164 error constants, 0 fmt.Errorf (complete)
- ✅ **CMD Package**: All 35 Lambda functions standardized

## Your Implementation Mission

You are tasked with executing **Phase 1: Critical Infrastructure Foundation** from the comprehensive task list. This phase is marked as 🔴 CRITICAL priority and must be completed before any other phases can proceed.

### Phase 1 Overview (Week 1: Days 1-7)
**Goal**: Establish core architectural components and eliminate critical violations  
**Success Gate**: DynamORM migration reaches 100%

## Phase 1 Tasks - Your Implementation Scope

### 1.1 DynamORM Storage Adapter Implementation (Priority 1)

**CRITICAL ISSUE IDENTIFIED**: The application is missing its core storage adapter - a critical architectural component that bridges the storage interface to repository implementations.

#### Task 1.1.1: Create Storage Interface Specification
- **File**: `pkg/storage/interfaces/storage.go`
- **Current State**: Interface file missing (audit revealed interface.go not found)
- **Action Required**: Define complete storage interface with all required methods
- **Pattern**: Review existing repository methods to understand interface requirements
- **Deliverable**: Complete storage interface definition

#### Task 1.1.2: Implement Storage Adapter Bridge  
- **File**: `pkg/storage/dynamorm/adapter.go`
- **Current State**: Storage adapter missing (critical architectural violation)
- **Action Required**: 
  - Bridge storage interface to repository implementations
  - Implement delegation patterns to individual repositories
  - Follow DynamORM patterns (NO AWS SDK usage)
- **Pattern**: Each adapter method delegates to appropriate repository
- **Deliverable**: Functional storage adapter with full interface compliance

#### Task 1.1.3: Create Adapter Integration Tests
- **File**: `pkg/storage/dynamorm/adapter_test.go`
- **Action Required**: Test all interface method implementations
- **Target**: 95%+ test coverage on adapter
- **Deliverable**: Comprehensive adapter test suite

#### Task 1.1.4: Update Factory to Use Adapter
- **File**: `pkg/storage/factory/factory.go`
- **Current Issue**: Factory has AWS SDK dependencies (should be eliminated)
- **Action Required**: Migrate factory to instantiate adapter instead of direct repositories
- **Deliverable**: Factory using adapter pattern exclusively

### 1.2 AWS SDK Elimination (Priority 2)

**CRITICAL VIOLATIONS**: 23 AWS SDK imports found in storage layer (should be 0 for DynamORM compliance)

#### Task 1.2.1: AWS SDK Usage Audit
- **Action Required**: Identify specific locations of all AWS SDK imports
- **Files to Check**:
  - `pkg/storage/repositories/cloudwatch_metrics_repository.go`
  - `pkg/storage/repositories/actor_repository.go`
  - `pkg/storage/factory/factory.go`
  - 20 additional files with AWS SDK v1 imports
- **Deliverable**: Complete AWS SDK usage inventory

#### Task 1.2.2: Repository Migration to DynamORM
- **Action Required**: Convert AWS SDK calls to DynamORM patterns
- **Critical Pattern**: Use DynamORM ONLY (no `github.com/aws/aws-sdk-go` imports)
- **Verification**: `grep -r "github.com/aws/aws-sdk-go" pkg/storage/` should return 0 results
- **Deliverable**: Zero AWS SDK imports in repositories

## Implementation Approach

### Use the lift-dynamorm-expert Agent

For DynamORM-related tasks, you MUST use the specialized `lift-dynamorm-expert` agent. This agent has been specifically trained for DynamORM/Lift patterns and has successfully completed 100 previous implementations.

#### Agent Usage Pattern:
```
Agent [Number]: [Task Description]
- Use subagent_type: "lift-dynamorm-expert"
- Provide specific file paths and requirements
- Emphasize NO AWS SDK usage
- Request verification commands
```

### Critical Implementation Guidelines

#### 1. DynamORM Patterns ONLY
```go
// ✅ CORRECT - Use DynamORM
var model models.User
err := r.db.WithContext(ctx).Model(&models.User{}).
    Where("PK", "=", fmt.Sprintf("USER#%s", username)).
    Where("SK", "=", "PROFILE").
    First(&model)

// ❌ WRONG - Never use AWS SDK
import "github.com/aws/aws-sdk-go-v2/service/dynamodb"
result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{...})
```

#### 2. Repository Pattern Requirements
- All storage access goes through repository interfaces
- Repositories use DynamORM models with proper tags
- StorageAdapter bridges storage.Storage interface to repositories
- NO direct calls to AWS SDK anywhere

#### 3. Key Pattern Preservation
Critical patterns that MUST be preserved:
- **Users**: PK=`USER#username`, SK=`PROFILE`
- **Actors**: PK=`ACTOR#username`, SK=`PROFILE`  
- **Objects**: PK=`object#id`, SK=`object#id`
- All existing GSI patterns and TTL logic

## Verification Commands

After each implementation step, run these verification commands:

```bash
# Verify no AWS SDK usage
grep -r "github.com/aws/aws-sdk-go" pkg/storage/ --include="*.go" | wc -l  # Must be 0

# Verify compilation
go build ./pkg/storage/...

# Verify storage adapter exists
ls -la pkg/storage/dynamorm/adapter.go

# Verify interface compliance
go test ./pkg/storage/dynamorm/ -v
```

## Success Criteria for Your Session

### Phase 1 Completion Requirements:
- [ ] ✅ Storage interface defined and comprehensive
- [ ] ✅ Storage adapter implements full storage interface  
- [ ] ✅ Zero AWS SDK imports in storage layer
- [ ] ✅ All storage tests pass
- [ ] ✅ Factory uses adapter exclusively
- [ ] ✅ Build succeeds across all packages

### Critical Metrics to Achieve:
- **DynamORM Migration**: 30% → 100% complete
- **AWS SDK Violations**: 23 → 0 imports
- **Storage Architecture**: Establish proper adapter pattern

## Next Session Handoff

When Phase 1 is complete, provide a handoff summary including:
1. **Completion Status**: All Phase 1 tasks completed
2. **Verification Results**: All success criteria met
3. **Files Modified**: List of all files created/updated
4. **Next Phase Ready**: Phase 2 can begin (Graph Package Error Standardization)

## Context Files Available

Your session has access to these context files:
- `COMPREHENSIVE_CONSISTENCY_AUDIT_REPORT.md` - Detailed audit findings
- `COMPREHENSIVE_TASK_LIST.md` - Complete 5-phase implementation plan
- `CLAUDE.md` - Project-specific guidelines and patterns
- Existing error standardization examples in AUTH, FEDERATION, SERVICES packages

## Implementation Priority

**START WITH**: Task 1.1.2 (Storage Adapter Implementation) - This is the most critical architectural component that blocks all other improvements.

Focus on creating a functional storage adapter that properly bridges the storage interface to repository implementations using pure DynamORM patterns.

## Agent Deployment Strategy

Continue the agent numbering sequence from the previous session:
- **Agent 101**: Storage interface creation
- **Agent 102**: Storage adapter implementation  
- **Agent 103**: Repository AWS SDK elimination
- **Agent 104**: Factory migration
- **Agent 105+**: Ready for Phase 2 Graph package work

## Key Success Factors

1. **Follow DynamORM Patterns Exclusively**: No AWS SDK imports allowed
2. **Use lift-dynamorm-expert Agent**: Leverage specialized knowledge
3. **Preserve Existing Patterns**: Maintain all key formats and GSI patterns
4. **Comprehensive Testing**: Ensure all changes are verified
5. **Clear Documentation**: Document adapter patterns for future maintenance

Your successful completion of Phase 1 will establish the critical foundation needed for the remaining 95%+ consistency achievement across the Lesser application.

## Ready to Begin

You are now equipped to begin Phase 1 implementation. Start with the storage adapter creation as the highest priority task, and use the lift-dynamorm-expert agent for all DynamORM-related work.

**Expected Outcome**: A fully functional DynamORM-based storage layer with zero AWS SDK dependencies, setting the foundation for complete application consistency.