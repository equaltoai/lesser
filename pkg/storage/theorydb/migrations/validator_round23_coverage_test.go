package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidator_ValidateAll_DependencyError_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	// Circular dependency: GetInOrder should fail.
	registry.MustRegister(&MockMigration{id: "a", version: 1, description: "a", dependencies: []string{"b"}})
	registry.MustRegister(&MockMigration{id: "b", version: 2, description: "b", dependencies: []string{"a"}})

	m := NewMigrator(db, registry, zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()

	result, err := v.ValidateAll(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	require.Equal(t, "dependency_error", result.Errors[0].Type)
}

func TestValidator_ValidateMigration_NotFoundInRegistry_Round23(t *testing.T) {
	t.Parallel()

	db, _ := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	result, err := v.ValidateMigration(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	require.Equal(t, "not_found", result.Errors[0].Type)
}

func TestValidator_ValidateMigration_ProducesErrorsAndWarnings_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	// Duplicate version warning: dupe has the same version as bad.
	registry.MustRegister(&MockMigration{id: "dupe", version: 0, description: "dupe"})

	bad := &MockMigration{
		id:           "bad id",
		version:      0,
		description:  "bad",
		dependencies: []string{"existing_dep", "missing_dep"},
	}
	registry.MustRegister(bad)
	registry.MustRegister(&MockMigration{id: "existing_dep", version: 2, description: "dep"})

	m := NewMigrator(db, registry, zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "bad id", Version: 0, Status: StatusApplied},
		}
	}).Return(nil).Once()

	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*MigrationHistory)
		dest.Status = "failed"
		dest.Error = "boom"
		dest.Checksum = "different"
	}).Return(nil).Once()

	result, err := v.ValidateMigration(context.Background(), "bad id")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Valid)

	require.NotEmpty(t, result.Errors)
	require.NotEmpty(t, result.Warnings)

	var errorTypes []string
	for _, validationErr := range result.Errors {
		errorTypes = append(errorTypes, validationErr.Type)
	}
	require.Contains(t, errorTypes, "already_applied")
	require.Contains(t, errorTypes, "invalid_id")
	require.Contains(t, errorTypes, "invalid_version")
	require.Contains(t, errorTypes, "missing_dependency")
	require.Contains(t, errorTypes, "unapplied_dependency")

	var warningTypes []string
	for _, validationWarn := range result.Warnings {
		warningTypes = append(warningTypes, validationWarn.Type)
	}
	require.Contains(t, warningTypes, "duplicate_version")
	require.Contains(t, warningTypes, "previous_failure")
	require.Contains(t, warningTypes, "checksum_mismatch")
}

func TestValidator_ValidateRollback_NoMigrations_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Maybe()

	result, err := v.ValidateRollback(context.Background(), RollbackOptions{})
	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "no_migrations", result.Warnings[0].Type)
}

func TestValidator_ValidateRollback_DependencyConflict_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	// Reverse dependency to force a conflict when rolling back "m2".
	registry.MustRegister(&MockMigration{id: "m1", version: 1, description: "m1", dependencies: []string{"m2"}})
	registry.MustRegister(&MockMigration{id: "m2", version: 2, description: "m2"})

	m := NewMigrator(db, registry, zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Version: 1, Status: StatusApplied},
			{ID: "m2", Version: 2, Status: StatusApplied},
		}
	}).Return(nil).Maybe()

	result, err := v.ValidateRollback(context.Background(), RollbackOptions{Target: "m1"})
	require.NoError(t, err)
	require.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	require.Equal(t, "dependency_conflict", result.Errors[0].Type)

	var warningTypes []string
	for _, w := range result.Warnings {
		warningTypes = append(warningTypes, w.Type)
	}
	require.Contains(t, warningTypes, "down_method_warning")
}

func TestValidator_checkDownMethodImplemented_GSIMigration_Round23(t *testing.T) {
	t.Parallel()

	db, _ := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	gsi := NewGSIMigration("gsi", 1, "gsi", "table", GSIDefinition{Name: "gsi1"})
	require.NoError(t, v.checkDownMethodImplemented(gsi))
}

func TestValidator_ValidateAll_PropagatesAppliedMigrationsError_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()
	registry.MustRegister(&MockMigration{id: "m1", version: 1, description: "m1"})

	m := NewMigrator(db, registry, zap.NewNop())
	v := NewValidator(m, zap.NewNop())

	q.On("All", mock.Anything).Return(errors.New("db failure")).Once()

	_, err := v.ValidateAll(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get applied migrations")
}
