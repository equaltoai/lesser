# Lift-DynamORM Expert Instructions Template

## Standard Instructions for Each Task

### Pre-Implementation Requirements
Always provide these to the agent:

```
CRITICAL: Storage Consolidation Task [X.Y] - [Task Name]

CONTEXT:
- We are eliminating the StorageAdapter layer to remove ~20,000 lines of duplicated code
- The goal is to use repositories directly instead of through the adapter
- This must be done incrementally with zero breakage

CURRENT STATE:
- StorageAdapter at /pkg/storage/dynamorm/adapter.go delegates to repositories
- Handlers currently use storage.Storage interface
- We need to migrate to direct repository usage

YOUR TASK:
[Specific task description]

FILES TO ANALYZE:
1. [List exact files to read and understand]
2. [Include both source and destination files]

FILES TO MODIFY:
1. [List exact files that need changes]
2. [Be specific about what changes are needed]

IMPLEMENTATION REQUIREMENTS:
1. Preserve ALL existing functionality exactly
2. Maintain the same error handling behavior
3. Keep the same method signatures where possible
4. Update imports to use repository packages directly
5. Ensure cost tracking continues to work
6. NO breaking changes to public APIs

SPECIFIC PATTERNS TO FOLLOW:
- Replace: storage.GetActor(ctx, username)
- With: repos.ActorRepo().GetActorByUsername(ctx, username)

VERIFICATION STEPS YOU MUST INCLUDE:
1. Show the exact changes made
2. Confirm no AWS SDK imports added
3. Verify compilation: go build ./...
4. List any potential breaking changes

DO NOT:
- Add new features or improvements
- Change business logic
- Modify database key patterns
- Add AWS SDK imports
- Create new dependencies
```

### Post-Implementation Verification Checklist

After EVERY agent task, run these commands:

```bash
# 1. Check compilation
go build ./...

# 2. Run relevant tests
JWT_SECRET=test-secret go test ./[relevant-package] -v -count=1

# 3. Check for accidental AWS SDK usage
grep -r "github.com/aws/aws-sdk-go" [modified-files] | wc -l  # Must be 0

# 4. Verify no references to StorageAdapter in modified files
grep -r "StorageAdapter" [modified-files] | wc -l  # Should decrease

# 5. Run linter on modified files
golangci-lint run [modified-files]

# 6. If auth-related, run auth tests
JWT_SECRET=test-secret go test ./pkg/auth -v

# 7. If federation-related, run federation tests
make test-federation

# 8. Check that cost tracking still works
grep -r "cost.Track" [modified-files]  # Should still exist where needed
```

### Example Task Instructions

#### For Task 1.1 (Repository Factory):
```
Create a new RepositoryFactory that initializes all repositories with proper dependencies. This factory will replace the StorageAdapter's initialization logic. The factory should:

1. Initialize all repositories with the DynamoDB client
2. Handle circular dependencies between repositories  
3. Provide getter methods for each repository type
4. Be created once at application startup

Key repositories to include:
- ActorRepository (depends on nothing)
- UserRepository (depends on ActorRepository)
- ObjectRepository (depends on ActorRepository)
- AuthRepository (depends on UserRepository)
[... list all repositories and their dependencies]
```

#### For Migration Tasks (1.2-1.7):
```
Migrate [endpoint-group] from using StorageAdapter to using repositories directly:

1. Update the handler struct to include RepositoryFactory instead of Storage
2. Replace all storage.Method() calls with repos.RepoType().Method() calls
3. Ensure error handling remains identical
4. Update any helper functions that take Storage as a parameter
5. Verify cost tracking calls are preserved

Common replacements:
- s.storage.GetActor() → s.repos.Actor().GetActorByUsername()
- s.storage.CreateObject() → s.repos.Object().CreateObject()
- s.storage.CreateUser() → s.repos.Auth().CreateUser()
[... provide specific mappings for this endpoint group]
```

### Critical Reminders for Agent

1. **Incremental Changes Only**: Each task should be completable independently
2. **Zero Breaking Changes**: Public APIs must remain identical
3. **Preserve All Behavior**: Including error cases and nil returns
4. **Test After Each Step**: Never proceed if tests fail
5. **Cost Tracking**: Must continue to work exactly as before

### Recovery Instructions

If a task fails verification:

```
RECOVERY NEEDED for Task [X.Y]

The implementation failed because:
[Specific failure reason]

Please fix ONLY the failing issue:
1. [Specific fix needed]
2. [Don't change anything else]

Rerun verification after fix.
```