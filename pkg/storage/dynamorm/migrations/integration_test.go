//go:build integration
// +build integration

package migrations

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestMigration for integration tests
type TestMigration struct {
	BaseMigration
	UpFunc   func(context.Context, core.DB) error
	DownFunc func(context.Context, core.DB) error
}

func (m *TestMigration) Up(ctx context.Context, db core.DB) error {
	if m.UpFunc != nil {
		return m.UpFunc(ctx, db)
	}
	return nil
}

func (m *TestMigration) Down(ctx context.Context, db core.DB) error {
	if m.DownFunc != nil {
		return m.DownFunc(ctx, db)
	}
	return nil
}

func setupTestDB(t *testing.T) (core.DB, func()) {
	// Use DynamoDB Local for testing
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	
	config := session.Config{
		Region:   "us-east-1",
		Endpoint: endpoint,
		AccessKeyID:     "fakeMyKeyId",
		SecretAccessKey: "fakeSecretAccessKey",
	}
	
	db, err := dynamorm.New(config)
	require.NoError(t, err)
	
	// Create test table
	// Note: In real integration tests, you would create the table using AWS SDK
	
	cleanup := func() {
		// Cleanup test data
		// Note: In real integration tests, you would delete the test table
	}
	
	return db, cleanup
}

func TestMigrator_Integration_MigrateAndRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	db, cleanup := setupTestDB(t)
	defer cleanup()
	
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry()
	migrator := NewMigrator(db, registry, logger)
	
	ctx := context.Background()
	
	// Create test migrations
	var migration1Applied, migration2Applied bool
	
	migration1 := &TestMigration{
		BaseMigration: NewBaseMigration(
			"test_migration_1",
			20240101120000,
			"Test migration 1",
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			migration1Applied = true
			return nil
		},
		DownFunc: func(ctx context.Context, db core.DB) error {
			migration1Applied = false
			return nil
		},
	}
	
	migration2 := &TestMigration{
		BaseMigration: NewBaseMigration(
			"test_migration_2",
			20240102120000,
			"Test migration 2",
			"test_migration_1", // Depends on migration1
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			migration2Applied = true
			return nil
		},
		DownFunc: func(ctx context.Context, db core.DB) error {
			migration2Applied = false
			return nil
		},
	}
	
	// Register migrations
	require.NoError(t, registry.Register(migration1))
	require.NoError(t, registry.Register(migration2))
	
	// Test initial state
	pending, err := migrator.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 2)
	
	// Test migration execution
	err = migrator.MigrateAll(ctx)
	require.NoError(t, err)
	
	// Verify migrations were applied
	assert.True(t, migration1Applied)
	assert.True(t, migration2Applied)
	
	// Verify no pending migrations
	pending, err = migrator.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 0)
	
	// Verify migration history
	history, err := migrator.GetMigrationHistory(ctx)
	require.NoError(t, err)
	assert.Len(t, history, 2)
	
	// Test rollback
	err = migrator.RollbackLast(ctx)
	require.NoError(t, err)
	
	// Verify migration2 was rolled back
	assert.True(t, migration1Applied)
	assert.False(t, migration2Applied)
	
	// Verify pending migrations
	pending, err = migrator.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, "test_migration_2", pending[0].ID())
}

func TestMigrator_Integration_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	db, cleanup := setupTestDB(t)
	defer cleanup()
	
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry()
	migrator := NewMigrator(db, registry, logger)
	
	ctx := context.Background()
	
	// Create test migration
	var migrationApplied bool
	
	migration := &TestMigration{
		BaseMigration: NewBaseMigration(
			"dry_run_test",
			20240101120000,
			"Dry run test migration",
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			migrationApplied = true
			return nil
		},
	}
	
	registry.Register(migration)
	
	// Test dry run
	err := migrator.Migrate(ctx, MigrateOptions{DryRun: true})
	require.NoError(t, err)
	
	// Verify migration was not actually applied
	assert.False(t, migrationApplied)
	
	// Verify migration is still pending
	pending, err := migrator.GetPendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}

func TestMigrator_Integration_FailedMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	db, cleanup := setupTestDB(t)
	defer cleanup()
	
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry()
	migrator := NewMigrator(db, registry, logger)
	
	ctx := context.Background()
	
	// Create migrations where second one fails
	migration1 := &TestMigration{
		BaseMigration: NewBaseMigration(
			"success_migration",
			20240101120000,
			"Successful migration",
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			return nil
		},
	}
	
	migration2 := &TestMigration{
		BaseMigration: NewBaseMigration(
			"failing_migration",
			20240102120000,
			"Failing migration",
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			return errors.New("migration failed")
		},
	}
	
	registry.Register(migration1)
	registry.Register(migration2)
	
	// Attempt to migrate all
	err := migrator.MigrateAll(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "migration failed")
	
	// Verify first migration was applied but second wasn't
	applied, err := migrator.GetAppliedMigrations(ctx)
	require.NoError(t, err)
	assert.True(t, applied["success_migration"])
	assert.False(t, applied["failing_migration"])
	
	// Verify migration history shows failure
	history, err := migrator.GetMigrationHistory(ctx)
	require.NoError(t, err)
	
	var failedMigration *MigrationHistory
	for _, h := range history {
		if h.ID == "failing_migration" {
			failedMigration = h
			break
		}
	}
	
	require.NotNil(t, failedMigration)
	assert.Equal(t, "failed", failedMigration.Status)
	assert.Contains(t, failedMigration.Error, "migration failed")
}

func TestMigrator_Integration_ConcurrentMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	db, cleanup := setupTestDB(t)
	defer cleanup()
	
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry()
	
	ctx := context.Background()
	
	// Create a migration
	migration := &TestMigration{
		BaseMigration: NewBaseMigration(
			"concurrent_test",
			20240101120000,
			"Concurrent test migration",
		),
		UpFunc: func(ctx context.Context, db core.DB) error {
			// Simulate long-running migration
			time.Sleep(100 * time.Millisecond)
			return nil
		},
	}
	
	registry.Register(migration)
	
	// Create two migrators
	migrator1 := NewMigrator(db, registry, logger)
	migrator2 := NewMigrator(db, registry, logger)
	
	// Try to run migrations concurrently
	errChan := make(chan error, 2)
	
	go func() {
		errChan <- migrator1.MigrateAll(ctx)
	}()
	
	go func() {
		// Small delay to ensure migrator1 gets lock first
		time.Sleep(10 * time.Millisecond)
		errChan <- migrator2.MigrateAll(ctx)
	}()
	
	// Collect results
	err1 := <-errChan
	err2 := <-errChan
	
	// One should succeed, one should fail with lock error
	if err1 == nil {
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "locked")
	} else {
		assert.NoError(t, err2)
		assert.Contains(t, err1.Error(), "locked")
	}
}