# Phase 7: Final Action Plan - Legacy Code Removal

## Great News! 
The 10,052-line StorageAdapter is **barely used**! Only one example file references it.

## Immediate Priority Tasks

### Task 1: Delete the Massive StorageAdapter (30 minutes)
Since only `examples/serverless_circuit_breaker_example.go` uses it:
1. Update the example to use repositories directly
2. Delete `pkg/storage/dynamorm/adapter.go` (10k lines gone!)
3. Celebrate 🎉

### Task 2: Remove Simple Adapter Files (1 hour)
These are small, local adapters:

1. **pkg/federation/storage_adapter.go**
   - RepositoryStorageAdapter (< 100 lines)
   - Update federation package to use repos directly
   
2. **cmd/federation-delivery/main.go**
   - FederationStorageAdapter (local to file)
   - Wire repos directly
   
3. **cmd/moderation-processor/main.go**
   - repositoryStorageAdapter (local to file)
   - Wire repos directly

### Task 3: Clean Up Test Files (30 minutes)
```bash
# Remove all backup files
rm cmd/api/lift/*.backup

# Remove disabled test files  
rm cmd/api/lift/*.disabled

# Clean up commented MockStorageAdapter references
# Use your editor's find/replace across all test files
```

### Task 4: Fix Scripts (15 minutes)
Update `scripts/generate_mocks.go` to remove reference to deleted interface.go

## Next Priority: Direct AWS SDK Usage

### Files Using Direct DynamoDB (by complexity):

#### Simple (1-2 hours each):
- `cmd/api/lift/metrics.go` - Cost tracking
- `cmd/api/lift/ai.go` - Minimal usage
- `pkg/translation/aws_translate.go` - Not storage related

#### Medium (2-4 hours each):
- `cmd/api/lift/misc.go` - Various utilities
- `cmd/api/lift/exports.go` - Export functionality
- `cmd/api/lift/imports.go` - Import functionality
- `cmd/cost-aggregator/main.go` - Cost aggregation

#### Complex (4-6 hours each):
- `pkg/cost/storage.go` - Core cost tracking
- `pkg/cost/dynamodb_wrapper.go` - DynamoDB wrapper
- `graph/resolver.go` - GraphQL resolver

## Execution Timeline

### Day 1 (3-4 hours) - The Big Win
Morning:
1. Update example file (30 min)
2. **DELETE adapter.go** - Remove 10k lines! (5 min)
3. Run `go build ./...` to ensure nothing breaks (10 min)
4. Commit this huge win

Afternoon:
1. Remove federation/storage_adapter.go (45 min)
2. Remove cmd adapters (45 min)
3. Clean up test files and scripts (45 min)
4. Commit clean state

### Day 2-3 - Direct AWS SDK Removal
1. Start with simple files (metrics.go, ai.go)
2. Create/update repositories as needed
3. Test each component
4. Move to medium complexity files

### Day 4-5 - Complete AWS SDK Removal
1. Tackle complex files
2. Ensure all cost tracking works
3. Update GraphQL resolver
4. Final testing

## Validation Commands

```bash
# After each major change
go build ./...
go test ./...

# Track progress
echo "=== Legacy Code Metrics ==="
echo "storage.Storage references: $(grep -r "storage\.Storage" --include="*.go" . 2>/dev/null | grep -v vendor | wc -l)"
echo "StorageAdapter references: $(grep -r "StorageAdapter" --include="*.go" . 2>/dev/null | grep -v vendor | wc -l)"
echo "Direct AWS SDK usage: $(grep -r "aws-sdk-go.*dynamodb" --include="*.go" . 2>/dev/null | grep -v vendor | wc -l)"
echo "Backup files: $(find . -name "*.backup" -type f 2>/dev/null | wc -l)"
echo "Disabled files: $(find . -name "*.disabled" -type f 2>/dev/null | wc -l)"
```

## Expected Outcome

After 5 days:
- ✅ 10k+ lines of legacy adapter code deleted
- ✅ No storage.Storage interface usage
- ✅ No StorageAdapter references
- ✅ No direct AWS SDK usage (except dynamorm internals)
- ✅ Clean, repository-based architecture
- ✅ All tests passing
- ✅ All tools working

## First Step - Do This Now!

1. Open `examples/serverless_circuit_breaker_example.go`
2. Update it to use repositories instead of StorageAdapter
3. Delete `pkg/storage/dynamorm/adapter.go`
4. Run `go build ./...`
5. Commit with message: "Remove 10k line StorageAdapter - barely used!"

This will be a HUGE win and motivate the rest of the cleanup!