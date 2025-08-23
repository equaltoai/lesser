# Worker Guidance: Phase 1 - Repository Pattern Standardization

**Project**: Lesser ActivityPub Consistency and Completeness  
**Phase**: 1 - Critical Repository Pattern Standardization  
**Priority**: 🔴 CRITICAL  
**Project Manager**: Claude Code PM Instance  
**Task Tracker**: `/Users/aronprice/lesser/PROJECT_CONSISTENCY_TASKS.md`

## Your Role as Worker Instance

You are a Claude Code worker instance responsible for implementing Phase 1 of our consistency project. The PM instance has identified critical repository pattern inconsistencies affecting 93% of repositories (110+ files) that must be resolved.

## Current Situation Assessment

**CRITICAL FINDINGS:**
- Only **8 out of 110+ repositories** use the BaseRepository pattern
- **Massive code duplication** across repository implementations
- **Inconsistent cost tracking** throughout the data layer
- **No standardized CRUD operations** for most repositories

**IMMEDIATE IMPACT:**
- Technical debt affecting maintainability
- Inconsistent cost tracking (critical for serverless cost optimization)
- Testing complexity due to lack of standardization
- High risk for production deployment

## Phase 1 Objectives

1. **Audit Repository Implementation Status** (Task 1.1)
2. **Enhance BaseRepository Pattern** (Task 1.2)  
3. **Migrate Priority Repositories** (Task 1.3)
4. **Ensure Interface Compliance** (Task 1.4)

## Your Starting Assignment: Task 1.1.1

**FIRST TASK**: Create comprehensive repository audit script

### Specific Instructions:

1. **Immediate Action**: Run this command to identify repositories not using BaseRepository:
   ```bash
   find /Users/aronprice/lesser/pkg/storage/repositories -name "*_repository.go" -exec grep -L "BaseRepository" {} \; > non_base_repos.txt
   ```

2. **Analysis Required**: 
   - Count total repository files
   - Count repositories using BaseRepository vs not using it
   - Create categorization of repository types

3. **Expected Output**: 
   - File: `non_base_repos.txt` with complete list
   - Summary report of findings
   - Categorization matrix for migration planning

### Key Files to Examine:

**BaseRepository Implementation:**
- `/Users/aronprice/lesser/pkg/storage/repositories/base_repository.go` (lines 24-50)

**Repository Interface Definitions:**
- `/Users/aronprice/lesser/pkg/storage/interfaces/repositories.go` (11 interfaces defined)

**Example Repositories Using BaseRepository:**
- `/Users/aronprice/lesser/pkg/storage/repositories/account_repository.go` (partially implemented)
- Check other files for BaseRepository usage patterns

**Critical Repositories for Priority Migration:**
- `account_repository.go` 
- `status_repository.go`
- `notification_repository.go`
- `media_repository.go`
- `relationship_repository.go`

## Code Quality Standards

**MANDATORY PATTERNS:**
- All repositories must use `BaseRepository[T BaseModel]` 
- All CRUD operations must have cost tracking
- All repositories must implement their interface completely
- Use structured logging (`zap.Logger`) - NO printf statements
- Follow DynamORM patterns consistently

**ERROR HANDLING:**
- Use `fmt.Errorf()` with error wrapping
- Map DynamoDB errors to storage errors consistently
- Return meaningful error context

## Reporting Requirements

**After Completing Task 1.1.1, Report Back With:**

1. **Repository Audit Results:**
   ```
   Total repository files found: X
   Repositories using BaseRepository: X  
   Repositories NOT using BaseRepository: X
   Percentage needing migration: X%
   ```

2. **Categorization Summary:**
   - Simple CRUD repositories (easy migration)
   - Complex repositories with custom logic  
   - Special-purpose repositories

3. **Priority Assessment:**
   - Which repositories are most critical for migration
   - Estimated effort for each category
   - Potential blockers or challenges identified

4. **Next Steps Recommendation:**
   - Should we proceed to Task 1.1.2?
   - Any issues discovered that need PM attention?

## Important Context

**Legacy Compatibility Requirements:**
- MUST preserve exact key patterns (e.g., `"USER#{username}"`, `"ACTOR#{username}"`)
- MUST maintain DynamORM tag consistency  
- MUST preserve all GSI structures exactly

**Cost Tracking Integration:**
- Every repository operation must use cost tracking
- BaseRepository must integrate with `pkg/cost/` tracking service
- This is critical for serverless cost optimization targets

**Testing Requirements:**
- All repository changes must be testable
- Must not break existing functionality
- Changes should be backward compatible

## Communication Protocol

**Status Updates**: Report progress every 2-3 tasks completed
**Questions**: Ask PM immediately if you encounter:
- Breaking changes required
- Interface mismatches
- Complex migration scenarios
- Unexpected technical debt

**Success Criteria for Task 1.1.1:**
- [ ] Audit script successfully executed
- [ ] Complete list of repositories needing migration
- [ ] Clear categorization for migration planning
- [ ] Accurate percentage and impact assessment
- [ ] Recommendations for next steps

## Get Started

Begin with Task 1.1.1 immediately. The repository audit is critical for understanding the full scope of work required. Focus on accuracy and completeness - this audit will drive all subsequent migration decisions.

**Execute the find command, analyze the results, and report your findings to the PM.**

Remember: You're addressing a critical technical debt issue that affects the entire data layer. Precision and thoroughness are essential for project success.

---
*Assigned by: Claude Code PM*  
*Start Time: 2025-08-19*  
*Priority: CRITICAL - Phase 1 Foundation*