# Stub Fix Tickets - Prioritized List

## Priority 1: Critical User-Facing Features (Week 1)

### STUB-001: Fix Import/Export List Functions
**Severity**: CRITICAL  
**Files**: 
- `cmd/api/handlers/imports.go:342` - `getUserImportJobs()`
- `cmd/api/handlers/exports.go:314` - `getUserExportJobs()`

**Description**: Functions return empty arrays instead of querying DynamoDB  
**Impact**: Users cannot see their import/export history  
**Estimate**: 2 days  
**Assignee**: _____

**Acceptance Criteria**:
- [ ] Both functions query GSI1 correctly
- [ ] Status filtering works when provided
- [ ] Pagination implemented
- [ ] Integration tests added
- [ ] Manual test passes

---

### STUB-002: Fix Export Data Generation - Followers/Following
**Severity**: CRITICAL  
**File**: `cmd/export-generator/main.go`  
**Functions**: 
- `getFollowers()` - line 574
- `getFollowing()` - line 581
- `getFollowingActors()` - line 630
- `getFollowersActors()` - line 636

**Description**: Core social graph exports return empty data  
**Impact**: Users cannot export their social connections  
**Estimate**: 2 days  
**Assignee**: _____

---

### STUB-003: Fix Export Data Generation - User Content
**Severity**: CRITICAL  
**File**: `cmd/export-generator/main.go`  
**Functions**:
- `getOutbox()` - line 623 (user's posts)
- `getLikes()` - line 642
- `getBookmarks()` - line 618
- `getBookmarksForExport()` - line 648

**Description**: User content exports return empty data  
**Impact**: GDPR compliance issue - users cannot export their content  
**Estimate**: 3 days  
**Assignee**: _____

---

## Priority 2: GraphQL Quick Fixes (Week 1)

### STUB-004: Replace GraphQL Panics with Errors
**Severity**: HIGH  
**File**: `graph/schema.resolvers.go`  
**Functions**: All 58 panic functions

**Description**: GraphQL resolvers panic instead of returning errors  
**Impact**: Any GraphQL query crashes the application  
**Estimate**: 1 day  
**Assignee**: _____

**Quick Fix**: Replace all panics with error returns:
```go
return nil, fmt.Errorf("GraphQL API is under development")
```

---

## Priority 3: Export Completeness (Week 2)

### STUB-005: Fix Export Data Generation - Safety Features
**Severity**: HIGH  
**File**: `cmd/export-generator/main.go`  
**Functions**:
- `getBlocks()` - line 588
- `getMutes()` - line 600
- `getListsWithMembers()` - line 606
- `getListsForExport()` - line 654

**Description**: Safety and organization features return empty data  
**Impact**: Users lose blocks/mutes when migrating  
**Estimate**: 2 days  
**Assignee**: _____

---

## Priority 4: Media Processing (Week 2)

### STUB-006: Implement Video Processing
**Severity**: HIGH  
**File**: `cmd/media-processor/main.go:277`  
**Function**: `processVideo()`

**Description**: Returns hardcoded 30-second duration for all videos  
**Impact**: Incorrect metadata displayed to users  
**Estimate**: 3 days  
**Assignee**: _____

**Requirements**:
- [ ] FFmpeg integration for metadata extraction
- [ ] Thumbnail generation
- [ ] Duration extraction
- [ ] Resolution detection

---

### STUB-007: Implement Audio Processing
**Severity**: HIGH  
**File**: `cmd/media-processor/main.go:297`  
**Function**: `processAudio()`

**Description**: Returns hardcoded 3-minute duration for all audio  
**Impact**: Incorrect metadata displayed to users  
**Estimate**: 2 days  
**Assignee**: _____

---

## Priority 5: GraphQL Implementation (Week 3-4)

### STUB-008: Implement Core GraphQL Query Resolvers
**Severity**: MEDIUM  
**File**: `graph/schema.resolvers.go`  
**Priority Functions**:
- Actor field resolvers (10 functions)
- Object query resolver
- Timeline query resolver

**Estimate**: 5 days  
**Assignee**: _____

---

### STUB-009: Implement GraphQL Mutation Resolvers
**Severity**: MEDIUM  
**File**: `graph/schema.resolvers.go`  
**Functions**: All mutation resolvers (12 functions)

**Estimate**: 5 days  
**Assignee**: _____

---

### STUB-010: Implement GraphQL Subscription Resolvers
**Severity**: LOW  
**File**: `graph/schema.resolvers.go`  
**Functions**: All subscription resolvers (5 functions)

**Estimate**: 3 days  
**Assignee**: _____

---

## Priority 6: Other Stubs (As Time Permits)

### STUB-011: Trends System Error Handling
**Severity**: LOW  
**File**: `pkg/storage/dynamodb/trends.go`  
**Note**: This is defensive coding, not a true stub - deprioritize

---

## Tracking Template

```markdown
## Daily Standup Template
**Date**: _____
**Completed Yesterday**:
- [ ] STUB-___ : Function name - Status

**Working on Today**:
- [ ] STUB-___ : Function name

**Blockers**:
- 

**Stubs Fixed**: ___/27
```

## Success Metrics

### Week 1 Goals
- [ ] STUB-001 Complete (Import/Export Lists)
- [ ] STUB-002 Complete (Followers/Following)
- [ ] STUB-004 Complete (GraphQL Panics)
- [ ] 0 panics in production code

### Week 2 Goals
- [ ] STUB-003 Complete (User Content)
- [ ] STUB-005 Complete (Safety Features)
- [ ] STUB-006 & 007 Complete (Media Processing)
- [ ] All export functions return real data

### Week 3-4 Goals
- [ ] STUB-008, 009, 010 Complete (GraphQL)
- [ ] All "for now" comments removed
- [ ] Integration test suite complete

## Assignment Guidelines

1. **Pair Programming**: Assign 2 developers per critical ticket
2. **Code Review**: Different reviewer than implementer
3. **Testing**: Dedicated QA for each completed ticket
4. **Documentation**: Update as part of ticket completion

## Notes

- Start with STUB-001 as it blocks testing other export features
- STUB-004 is a quick win - assign to someone while others work on complex fixes
- Media processing requires FFmpeg setup - coordinate with DevOps
- GraphQL can be done incrementally - implement most-used queries first 