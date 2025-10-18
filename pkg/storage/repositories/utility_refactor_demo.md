# Repository Utility Refactoring Demo

This document demonstrates the improvements achieved through the Phase 4 utility refactoring.

## Before/After Comparison

### Key Generation (Before)
```go
// Inconsistent, error-prone key generation scattered throughout
err := r.Get(ctx, fmt.Sprintf("USER#%s", username), "METADATA", user)
err := r.db.WithContext(ctx).Model(&actorModel).
    Where("PK", "=", fmt.Sprintf("ACTOR#%s", username)).
    Where("SK", "=", "PROFILE").
    First(&actorModel)
```

### Key Generation (After)
```go
// Consistent, centralized key generation
pk := Utils.Keys.UserKey(username)
err := r.Get(ctx, pk, SKMetadata, user)

pk := Utils.Keys.ActorKey(username)
err := r.db.WithContext(ctx).Model(&actorModel).
    Where("PK", "=", pk).
    Where("SK", "=", SKProfile).
    First(&actorModel)
```

### Error Handling (Before)
```go
// Inconsistent error handling, repetitive code
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        return nil, common.UserNotFoundError{Username: username}
    }
    return nil, fmt.Errorf("failed to get user: %w", err)
}

if errors.IsConditionFailed(err) {
    return fmt.Errorf("WebAuthn credential %s already exists", credential.ID)
}
```

### Error Handling (After)
```go
// Standardized error handling with consistent messages
if err != nil {
    return nil, ErrorHandler.HandleGetError(err, EntityUser, username)
}

// Consistent error handling for all operation types
return ErrorHandler.HandleCreateError(err, EntityWebAuthnCredential, credential.ID)
```

### TTL Management (Before)
```go
// Hard-coded TTL values, inconsistent across codebase
if data.ExpiresAt.IsZero() {
    data.ExpiresAt = time.Now().Add(10 * time.Minute)
}
```

### TTL Management (After)
```go
// Standardized TTL values, easy to modify centrally
if data.ExpiresAt.IsZero() {
    data.ExpiresAt = data.CreatedAt.Add(StandardTTLs.OAuthState)
}
```

### Validation (Before)
```go
// No validation or scattered validation logic
// Follow creates a follow relationship between users
func (r *AccountRepository) Follow(ctx context.Context, followerUsername, followedUsername string) error {
    // Direct actor lookup without validation
    follower, err := r.GetActor(ctx, followerUsername)
    // ...
}
```

### Validation (After)
```go
// Consistent validation using utility functions
func (r *AccountRepository) Follow(ctx context.Context, followerUsername, followedUsername string) error {
    // Validate usernames using validation utility
    if !Utils.Validation.IsValidUsername(followerUsername) {
        return common.ValidationError{Field: "follower", Message: "invalid username"}
    }
    if !Utils.Validation.IsValidUsername(followedUsername) {
        return common.ValidationError{Field: "followed", Message: "invalid username"}
    }
    // ...
}
```

## Benefits Achieved

### 1. Code Consistency
- All repositories now use identical patterns for common operations
- Key generation follows the same format across all models
- Error messages are standardized and predictable

### 2. Maintainability
- Single point of change for key patterns, TTL values, and error messages
- Easy to update validation rules across all repositories
- Centralized constants reduce magic strings

### 3. Reduced Code Duplication
- Common patterns extracted to reusable utilities
- Less repetitive error handling code
- Standardized validation and pagination logic

### 4. Improved Developer Experience
- Clear, consistent API across all repository methods
- Predictable error messages for debugging
- Self-documenting code through utility function names

### 5. Easier Testing
- Consistent patterns make it easier to write comprehensive tests
- Standardized error types enable better error testing
- Centralized utilities can be tested independently

## Implementation Statistics

### Files Created:
- `utils.go`: Key generation, time utilities, validation, pagination (257 lines)
- `errors.go`: Standardized error handling patterns (132 lines)  
- `query_utils.go`: Common query patterns with pagination (322 lines)

### Files Refactored:
- `account_repository.go`: Core methods updated to use utilities
- `account_repository_social.go`: Added validation and error handling
- `account_repository_oauth.go`: Standardized TTL and error handling
- `account_repository_webauthn.go`: Consistent error handling

### Lines of Code Impact:
- **Before**: ~150 lines of repetitive key generation, error handling
- **After**: ~20 lines using utilities (87% reduction in boilerplate)
- **Utilities**: 711 lines of reusable, well-tested utility functions

## Query Pattern Examples

The `QueryUtils` class provides standardized patterns for:

1. **User Relationship Queries**: Following, followers, blocked users
2. **Time Range Queries**: Activity logs, timeline entries
3. **GSI Status Queries**: Pending requests, active sessions
4. **Count Queries**: Follower counts, status counts
5. **Existence Checks**: Duplicate prevention
6. **Batch Operations**: Efficient bulk deletes

## Next Steps

This refactoring provides a solid foundation for:
- Phase 5: Creating a minimal Storage interface with repository factory
- Easier migration of remaining repositories to DynamORM patterns  
- Consistent testing patterns across all repositories
- Simplified onboarding for new developers

The utility pattern established here can be extended to other repository types and provides a template for maintaining consistency as the codebase grows.