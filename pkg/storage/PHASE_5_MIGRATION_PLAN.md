# Phase 5: Migration Plan - Repository Storage Interface

## Overview

Phase 5 completes the storage consolidation by providing a clean, repository-focused interface that replaces the massive `storage.Storage` interface. This migration plan outlines the systematic approach to migrate all consumers to the new pattern.

## Architecture Changes

### Before (Current)
```go
// Massive interface with 200+ methods
type Storage interface {
    CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
    GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
    // ... 200+ more methods
}

// In Lambda handlers
store := dynamorm.NewStorageAdapter(db, tableName, logger)
actor, err := store.GetActor(ctx, username)
```

### After (Phase 5)
```go
// Clean, focused interface
type RepositoryStorage interface {
    Account() *repositories.AccountRepository
    Actor() *repositories.ActorRepository
    Object() *repositories.ObjectRepository
    // ... other repositories
    GetDB() dynamormCore.DB
    GetTableName() string
    GetLogger() *zap.Logger
}

// In Lambda handlers
repos := factory.NewRepositoryFactory(db, tableName, logger)
actor, err := repos.Actor().GetActor(ctx, username)
```

## Migration Strategy

### Phase 5.4: Migration Plan ✅ COMPLETED

**Approach**: Gradual migration with backward compatibility
- New code uses `core.RepositoryStorage` interface
- Legacy code continues using `storage.Storage` interface
- Dual support during transition period

### Phase 5.5: Update Lambda Handlers (PENDING)

**Priority Files** (Most Critical):
1. `cmd/api/main.go` - Main API Lambda
2. `cmd/auth/main.go` - Auth service Lambda
3. `cmd/*/main.go` - Other Lambda entry points

**Migration Pattern**:
```go
// OLD
store := dynamorm.NewStorageAdapter(db, tableName, logger)
liftHandler := lift.NewHandler(cfg, store, authService, logger)

// NEW
repos := factory.NewRepositoryFactory(db, tableName, logger)
liftHandler := lift.NewHandler(cfg, repos, authService, logger)
```

**Services to Update**:
- Auth services (`pkg/auth/*.go`)
- Federation services (`pkg/federation/*.go`)
- GraphQL resolvers (`graph/resolver.go`)
- API handlers (`cmd/api/lift/*.go`)

### Phase 5.6: Remove StorageAdapter Layer (PENDING)

**Goals**:
- Remove `pkg/storage/dynamorm/adapter.go` completely
- Remove legacy `storage.Storage` interface methods
- Clean up unused imports and interfaces

**Validation Required**:
- All tests pass with new interface
- No references to old Storage interface remain
- Performance is maintained or improved

### Phase 5.7: Final Validation (PENDING)

**Test Categories**:
1. **Unit Tests**: All repository tests pass
2. **Integration Tests**: API endpoints work correctly
3. **Federation Tests**: ActivityPub federation works
4. **Authentication Tests**: OAuth, WebAuthn, wallet auth
5. **Load Tests**: Performance meets requirements

## Implementation Benefits

### 1. Clean Architecture
- Single responsibility: each repository handles one domain
- Clear boundaries between different data concerns
- Repository pattern properly implemented

### 2. Better Developer Experience
- IDE auto-completion works better with concrete types
- Clear method signatures and return types
- Self-documenting repository interfaces

### 3. Easier Testing
- Mock individual repositories instead of entire storage layer
- Focused unit tests for specific domains
- Integration tests with real repository instances

### 4. Performance Improvements
- Direct repository calls eliminate adapter overhead
- Better connection pooling and resource management
- Reduced memory footprint from smaller interfaces

## Migration Checklist

### Pre-Migration Validation
- [ ] All existing tests pass
- [ ] Build succeeds for all Lambda functions
- [ ] Current functionality works as expected

### Migration Steps
- [ ] 5.5: Update Lambda handlers to use RepositoryStorage
- [ ] 5.6: Remove StorageAdapter and legacy interfaces
- [ ] 5.7: Run comprehensive validation suite

### Post-Migration Validation
- [ ] All tests pass with new interface
- [ ] API endpoints respond correctly
- [ ] Authentication flows work
- [ ] Federation operates normally
- [ ] Performance benchmarks meet targets

## Risk Mitigation

### Rollback Plan
- Keep StorageAdapter code until validation is complete
- Maintain feature flags for new vs old interface
- Document all changes for easy reversal if needed

### Testing Strategy
- Test each Lambda function individually
- Gradual rollout starting with least critical services
- Monitor logs and metrics during migration

### Communication
- Document all interface changes
- Update development team on new patterns
- Provide examples of new repository usage

## Success Metrics

1. **Code Quality**: Reduced lines of boilerplate code
2. **Maintainability**: Single point of change for repository logic
3. **Performance**: Same or better response times
4. **Developer Productivity**: Faster development with clear interfaces
5. **Test Coverage**: Improved test reliability and speed

## Timeline

- **Phase 5.5**: 1-2 hours (Update Lambda handlers)
- **Phase 5.6**: 1 hour (Remove legacy code)
- **Phase 5.7**: 1 hour (Final validation)
- **Total**: 3-4 hours for complete migration

## Next Steps

The infrastructure is now in place. The next task is to systematically update Lambda handlers and services to use the new `RepositoryStorage` interface, starting with the most critical services first.