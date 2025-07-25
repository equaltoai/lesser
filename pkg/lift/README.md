# Lift Multi-Tenant Authentication Infrastructure

This package implements Task 3.3 from the Phase 1 Core Infrastructure specification: "Implement Lift-native authentication middleware with multi-tenant support."

## Overview

The multi-tenant authentication infrastructure provides:

1. **Tenant Resolution**: Automatically resolve tenant context from HTTP headers, subdomains, or URL paths
2. **Data Isolation**: Ensure all database operations are scoped to the current tenant
3. **Access Control**: Validate tenant-specific permissions and prevent cross-tenant access
4. **Audit Logging**: Log all operations with tenant context for security and compliance

## Core Components

### Authentication Middleware

- `RequireAuth()` - Requires valid authentication
- `RequireScope(scope)` - Requires specific OAuth scopes
- `OptionalAuth()` - Optional authentication for public endpoints
- `RequireTenant()` - Requires tenant context resolution
- `RequireTenantWithConfig(config)` - Tenant resolution with custom configuration

### Tenant Resolution Strategies

The system attempts tenant resolution in this order:

1. **X-Tenant-ID Header**: Direct tenant specification via HTTP header
2. **Subdomain**: Extract tenant from subdomain (e.g., `tenant1.lesser.app`)
3. **URL Path**: Parse tenant from URL path (e.g., `/tenant/tenant1/api/...`)
4. **JWT Claims**: Use authenticated user's tenant information

### Data Isolation

#### TenantAwareDB

Wraps DynamORM operations with automatic tenant prefixing:

```go
// Create tenant-aware database wrapper
tadb, err := NewTenantAwareDB(baseDB, liftCtx)
if err != nil {
    return err
}

// All operations are automatically tenant-scoped
var user models.User
err = tadb.Model(&user).
    Where("PK", "=", "user#john").
    Get(&user)
```

#### Key Features

- **Automatic Prefixing**: Adds `tenant#{tenantID}#` prefix to partition keys
- **Transparent Stripping**: Removes prefixes from query results
- **Cross-Tenant Protection**: Validates access attempts across tenant boundaries
- **Shared Resources**: Supports resources shared across all tenants (e.g., system config)

### Configuration

```go
config := &TenantIsolationConfig{
    EnableStrict:         true,                    // Enforce strict tenant isolation
    AllowCrossTenantRead: false,                  // Deny cross-tenant read access
    SharedResources:      []string{"system", "config"}, // Resources shared across tenants
    DefaultTenantID:      "default",              // Fallback when no tenant resolved
}
```

### Error Handling & Logging

All operations include tenant context in logs and errors:

```go
// Tenant-aware error logging
LogTenantError(ctx, "UserCreation", err)

// Tenant-aware access logging
LogTenantAccessDenied(ctx, "admin:users", "delete")

// Structured tenant errors
tenantErr := NewTenantError(ctx, "VALIDATION_FAILED", "Invalid user data", err).
    WithDetail("field", "email").
    WithDetail("constraint", "unique")
```

## Usage Examples

### Basic Repository Pattern

```go
func (r *UserRepository) GetUser(ctx context.Context, liftCtx *lift.Context, username string) (*User, error) {
    // Create tenant-aware database wrapper
    tadb, err := NewTenantAwareDB(r.db, liftCtx)
    if err != nil {
        return nil, err
    }
    
    var user models.User
    err = tadb.WithContext(ctx).
        Model(&user).
        Where("PK", "=", fmt.Sprintf("user#%s", username)).
        Get(&user)
    
    if err != nil {
        LogTenantError(liftCtx, "GetUser", err)
        return nil, err
    }
    
    // Convert and return (tenant prefix automatically stripped)
    return convertToUser(&user), nil
}
```

### Middleware Setup

```go
// In your Lambda handler setup
las := lift.NewLiftAuthService(authService)

app := lift.NewAppBuilder().
    WithMiddleware(lift.LoggingMiddleware()).
    WithMiddleware(lift.CORSMiddleware()).
    WithMiddleware(las.RequireTenant()).        // Resolve tenant context
    WithMiddleware(las.RequireAuth()).          // Require authentication
    WithMiddleware(lift.TenantAwareLoggingMiddleware()). // Add tenant to all logs
    Build()
```

### Transaction Support

```go
func (r *UserRepository) UpdateUserWithPosts(ctx context.Context, liftCtx *lift.Context, userID string, posts []Post) error {
    tadb, err := NewTenantAwareDB(r.db, liftCtx)
    if err != nil {
        return err
    }
    
    // Transaction automatically scoped to tenant
    tx := tadb.Transaction()
    
    // Update user
    user := &models.User{PK: fmt.Sprintf("user#%s", userID)}
    tx = tx.Update(user, map[string]interface{}{
        "last_post_at": time.Now(),
    })
    
    // Create posts
    for _, post := range posts {
        postModel := &models.Post{
            PK: fmt.Sprintf("user#%s", userID),
            SK: fmt.Sprintf("post#%s", post.ID),
            // ... other fields
        }
        tx = tx.Put(postModel)
    }
    
    return tx.Execute()
}
```

## Security Considerations

1. **Tenant Validation**: All database keys are validated to prevent cross-tenant access
2. **Audit Logging**: Every operation is logged with tenant context
3. **Error Sanitization**: Errors are sanitized to prevent information leakage
4. **Configuration Flexibility**: Supports different isolation levels based on requirements

## Testing

The package includes comprehensive unit tests for:

- Tenant resolution from various sources
- Data isolation and prefix/strip operations
- Error handling and logging
- Configuration validation
- Cross-tenant access prevention

## Integration Notes

This implementation is designed to be gradually adopted:

1. **Phase 1**: Add tenant middleware to collect context
2. **Phase 2**: Wrap existing repositories with tenant-aware database connections
3. **Phase 3**: Enable strict isolation mode after validation
4. **Phase 4**: Add comprehensive audit logging and monitoring

The design allows existing code to work unchanged while providing opt-in tenant isolation capabilities.