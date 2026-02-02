package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMigrator_Migrate_NonDryRun_ExecutesAndUpdatesStatus_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	mig := &mockMigration{BaseMigration: NewBaseMigration("m1", 1, "m1")}
	registry.MustRegister(mig)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe() // acquireLock + history/status writes
	q.On("Delete").Return(nil).Maybe() // releaseLock
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()
	q.On("First", mock.Anything).Return(errors.New("not found")).Maybe()

	require.NoError(t, m.Migrate(context.Background(), MigrateOptions{}))
	require.Equal(t, 1, mig.upCalls)
}

func TestMigrator_Migrate_TargetFiltersPending_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	m1 := &mockMigration{BaseMigration: NewBaseMigration("m1", 1, "m1")}
	m2 := &mockMigration{BaseMigration: NewBaseMigration("m2", 2, "m2")}
	m3 := &mockMigration{BaseMigration: NewBaseMigration("m3", 3, "m3")}
	registry.MustRegister(m1)
	registry.MustRegister(m2)
	registry.MustRegister(m3)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()
	q.On("First", mock.Anything).Return(errors.New("not found")).Maybe()

	require.NoError(t, m.Migrate(context.Background(), MigrateOptions{Target: "m2"}))
	require.Equal(t, 1, m1.upCalls)
	require.Equal(t, 1, m2.upCalls)
	require.Equal(t, 0, m3.upCalls)
}

func TestMigrator_Migrate_NoPending_ReturnsNil_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{{ID: "m1", Version: 1, Status: StatusApplied}}
	}).Return(nil).Once()

	require.NoError(t, m.Migrate(context.Background(), MigrateOptions{}))
}

func TestMigrator_Migrate_DependencyOrderError_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	registry.MustRegister(&MockMigration{id: "a", version: 1, description: "a", dependencies: []string{"b"}})
	registry.MustRegister(&MockMigration{id: "b", version: 2, description: "b", dependencies: []string{"a"}})

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()

	err := m.Migrate(context.Background(), MigrateOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to order migrations")
}

func TestMigrator_MigrateDown_RollsBackToTarget_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	m2 := &mockMigration{BaseMigration: NewBaseMigration("m2", 2, "m2")}
	m3 := &mockMigration{BaseMigration: NewBaseMigration("m3", 3, "m3")}
	registry.MustRegister(m2)
	registry.MustRegister(m3)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m3", Version: 3, Status: StatusApplied},
			{ID: "m2", Version: 2, Status: StatusApplied},
			{ID: "m1", Version: 1, Status: StatusApplied},
		}
	}).Return(nil).Once()

	require.NoError(t, m.MigrateDown(context.Background(), "m2"))
	require.Equal(t, 1, m3.downCalls)
	require.Equal(t, 1, m2.downCalls)
}

func TestMigrator_MigrateDown_NoHistory_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()

	err := m.MigrateDown(context.Background(), "m1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no migrations to rollback")
}

func TestMigrator_MigrateDown_MissingMigrationInRegistry_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "missing", Version: 1, Status: StatusApplied},
		}
	}).Return(nil).Once()

	err := m.MigrateDown(context.Background(), "missing")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in registry")
}

func TestMigrator_GetPendingMigrations_ReturnsRegistryPending_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()
	registry.MustRegister(&mockMigration{BaseMigration: NewBaseMigration("m1", 1, "m1")})
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Once()

	pending, err := m.GetPendingMigrations(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1)
}
