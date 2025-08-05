# Phase 7: Quick Wins and Priority Tasks

## Immediate Actions (Can be done now)

### 1. Clean Up Test References (30 minutes)
Remove all commented MockStorageAdapter references:
```bash
# Files with commented MockStorageAdapter
cmd/api/lift/filters_test.go
cmd/api/lift/reports_test.go
cmd/api/lift/webfinger_test.go
# ... and many more

# Can use sed to clean these up
```

### 2. Delete Backup Files (5 minutes)
```bash
# Remove all .backup files
rm cmd/api/lift/*.backup

# Remove disabled test files
rm cmd/api/lift/*.disabled
```

### 3. Fix generate_mocks.go Script (15 minutes)
Update `scripts/generate_mocks.go` to not reference deleted interface.go

## High Priority Tasks

### 1. Remove Simple Adapters
These adapters are thin wrappers and can be removed quickly:

#### a. federation/storage_adapter.go
- Small file (< 100 lines)
- Simple pass-through methods
- Used by federation package
- Can update callers to use repos directly

#### b. cmd/federation-delivery/main.go FederationStorageAdapter
- Local to one file
- Clear mapping to repositories
- Can be inlined

#### c. cmd/moderation-processor/main.go repositoryStorageAdapter
- Local to one file
- Limited interface
- Direct repository usage

### 2. Replace Direct AWS SDK in Lift Handlers
These files in cmd/api/lift/ use AWS SDK directly:

#### a. metrics.go
- Uses DynamoDB for cost tracking
- Should use CostTrackingRepository

#### b. misc.go
- Various utility functions
- May need new repository methods

#### c. exports.go
- Data export functionality
- Consider using existing repositories

#### d. imports.go
- Data import functionality
- Consider using existing repositories

#### e. ai.go
- AI integration
- Minimal DynamoDB usage

## Recommended Order of Attack

### Day 1: Quick Wins (2-3 hours)
1. Clean up all test file comments
2. Delete backup and disabled files
3. Fix generate_mocks.go script
4. Commit these changes

### Day 2: Remove Simple Adapters (4-6 hours)
1. Remove federation/storage_adapter.go
2. Update federation package to use repos directly
3. Remove cmd adapters (federation-delivery, moderation-processor)
4. Test each component
5. Commit working state

### Day 3: Start AWS SDK Replacement (6-8 hours)
1. Start with metrics.go (most isolated)
2. Then misc.go
3. Create any needed repository methods
4. Test thoroughly
5. Commit each file as completed

### Day 4-5: Continue AWS SDK Replacement
1. Complete remaining lift handlers
2. Move to other cmd tools
3. Focus on one component at a time

### Day 6-8: The Big One - StorageAdapter
1. Analyze which methods are actually used
2. Find all callers
3. Migrate method by method
4. Delete when complete

## Commands to Track Progress

```bash
# Count legacy references
grep -r "storage\.Storage" --include="*.go" . | grep -v vendor | wc -l
grep -r "StorageAdapter" --include="*.go" . | grep -v vendor | wc -l
grep -r "aws-sdk-go.*dynamodb" --include="*.go" . | grep -v vendor | wc -l

# Find backup files
find . -name "*.backup" -type f | wc -l
find . -name "*.disabled" -type f | wc -l

# Check build status
go build ./...
```

## Success Metrics
- Zero references to storage.Storage interface
- Zero StorageAdapter usage
- Zero direct AWS SDK imports (except in dynamorm internals)
- All tests compile
- All commands work