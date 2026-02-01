# DynamORM Migrations

This package provides a comprehensive migration system for DynamoDB using DynamORM, supporting schema changes, GSI management, and data migrations with rollback capabilities.

## Features

- **Migration Tracking**: Stores migration history in DynamoDB
- **Dependency Management**: Ensures migrations run in the correct order
- **Rollback Support**: Safely rollback migrations with dependency checking
- **GSI Helpers**: Utilities for managing Global Secondary Indexes
- **Validation**: Pre-execution validation to catch issues early
- **Concurrent Safety**: Lock mechanism prevents concurrent migrations
- **Dry Run**: Preview changes without executing them

## Usage

### Creating a Migration

```go
package migrations

import (
    "context"
    "github.com/pay-theory/dynamorm/pkg/core"
)

type AddUserEmailIndex struct {
    BaseMigration
}

func NewAddUserEmailIndex() *AddUserEmailIndex {
    return &AddUserEmailIndex{
        BaseMigration: NewBaseMigration(
            "20240115_add_user_email_index",
            20240115120000,
            "Add GSI for user email lookups",
            // Optional dependencies
        ),
    }
}

func (m *AddUserEmailIndex) Up(ctx context.Context, db core.DB) error {
    // Apply migration logic
    return nil
}

func (m *AddUserEmailIndex) Down(ctx context.Context, db core.DB) error {
    // Rollback logic
    return nil
}
```

### Using GSI Helpers

```go
func NewAddActivityIndex() Migration {
    return NewGSIMigration(
        "20240116_add_activity_index",
        20240116120000,
        "Add GSI for activity queries",
        "lesser-production", // table name
        GSIDefinition{
            Name:           "GSI7",
            HashKey:        "gsi7PK",
            HashKeyType:    "S",
            RangeKey:       "gsi7SK", 
            RangeKeyType:   "S",
            ProjectionType: "ALL",
        },
    )
}
```

### Registering Migrations

```go
func init() {
    // Register migrations in init functions
    MustRegister(NewAddUserEmailIndex())
    MustRegister(NewAddActivityIndex())
}
```

### Running Migrations

```go
// Initialize migrator
db, _ := dynamorm.GetClient(ctx)
registry := GetRegistry()
logger, _ := zap.NewProduction()
migrator := NewMigrator(db, registry, logger)

// Run all pending migrations
err := migrator.MigrateAll(ctx)

// Run up to a specific migration
err := migrator.MigrateTo(ctx, "20240115_add_user_email_index")

// Dry run to preview changes
err := migrator.Migrate(ctx, MigrateOptions{DryRun: true})
```

### Rolling Back Migrations

```go
// Rollback last migration
err := migrator.RollbackLast(ctx)

// Rollback to a specific migration (exclusive)
err := migrator.RollbackTo(ctx, "20240115_add_user_email_index")

// Rollback multiple migrations
err := migrator.RollbackSteps(ctx, 3)
```

### Validation

```go
// Create validator
validator := NewValidator(migrator, logger)

// Validate all pending migrations
result, err := validator.ValidateAll(ctx)
if !result.Valid {
    fmt.Println(result.Format())
}

// Validate rollback plan
result, err := validator.ValidateRollback(ctx, RollbackOptions{Steps: 1})
```

## Migration History

Migration history is stored in DynamoDB with the following structure:

- **PK**: `MIGRATION#HISTORY`
- **SK**: Migration ID
- **Attributes**: version, description, applied_at, applied_by, checksum, status, error

## Best Practices

1. **Naming Convention**: Use format `YYYYMMDD_description` (e.g., `20240115_add_user_index`)
2. **Version Numbers**: Use format `YYYYMMDDHHMMSS` for ordering
3. **Dependencies**: Only depend on migrations that must run before
4. **Idempotency**: Ensure migrations can be safely re-run
5. **Testing**: Test both Up and Down methods
6. **Small Changes**: Keep migrations focused on single changes
7. **No Data Loss**: Ensure Down methods don't cause data loss

## Lambda Integration

For Lambda functions that need to run migrations:

```go
func init() {
    // Register models for cold start optimization
    dynamorm.InitializeModels(&MigrationHistory{}, &MigrationStatus{})
}

func handler(ctx context.Context, event any) error {
    // Get Lambda-optimized client
    db, err := dynamorm.GetLambdaClient(ctx)
    if err != nil {
        return err
    }
    
    // Run migrations
    migrator := NewMigrator(db, GetRegistry(), logger)
    if err := migrator.MigrateAll(ctx); err != nil {
        return err
    }
    
    // Continue with handler logic...
}
```

## Testing

Run unit tests:
```bash
go test ./pkg/storage/dynamorm/migrations
```

Run integration tests (requires DynamoDB Local):
```bash
go test -tags=integration ./pkg/storage/dynamorm/migrations
```
