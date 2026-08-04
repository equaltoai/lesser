package migrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMigrator_RollbackWithOptions_DryRun_NoAppliedMigrations_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Version: 1, Status: "failed"},
			{ID: "m2", Version: 2, Status: StatusRolledBack},
		}
	}).Return(nil).Once()

	require.NoError(t, m.RollbackWithOptions(context.Background(), RollbackOptions{DryRun: true}))
}

func TestMigrator_RollbackWithOptions_DryRun_TargetIsNewest_SelectsNone_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()
	registry.MustRegister(&MockMigration{id: "m1", version: 1, description: "m1"})
	registry.MustRegister(&MockMigration{id: "m2", version: 2, description: "m2"})

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Version: 1, Status: StatusApplied},
			{ID: "m2", Version: 2, Status: StatusApplied},
		}
	}).Return(nil).Once()

	require.NoError(t, m.RollbackWithOptions(context.Background(), RollbackOptions{DryRun: true, Target: "m2"}))
}

func TestMigrator_RollbackWithOptions_DryRun_AppliedNotInRegistry_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "missing", Version: 1, Status: StatusApplied},
		}
	}).Return(nil).Once()

	err := m.RollbackWithOptions(context.Background(), RollbackOptions{DryRun: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in registry")
}

func TestMigrator_validateRollback_DependencyConflict_Round23(t *testing.T) {
	t.Parallel()

	db, _ := newMigratorTestDB()
	registry := NewRegistry()
	registry.MustRegister(&MockMigration{id: "base", version: 1, description: "base"})
	registry.MustRegister(&MockMigration{id: "dependent", version: 2, description: "dependent", dependencies: []string{"base"}})

	m := NewMigrator(db, registry, zap.NewNop())

	toRollback := []*MigrationHistory{{ID: "base", Version: 1, Status: StatusApplied}}
	appliedMap := map[string]bool{"base": true, "dependent": true}

	err := m.validateRollback(context.Background(), toRollback, appliedMap)
	require.Error(t, err)
	require.Contains(t, err.Error(), "depends on it")
}

func TestMigrator_executeRollback_DryRun_DoesNotCallDown_Round23(t *testing.T) {
	t.Parallel()

	db, _ := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	migration := &MockMigration{id: "m1", version: 1, description: "m1"}
	require.NoError(t, m.executeRollback(context.Background(), migration, RollbackOptions{DryRun: true}))
}

func TestMigrator_executeRollback_RecordStartFails_Round23(t *testing.T) {
	t.Parallel()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Create").Return(errors.New("record start failed")).Once()

	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	migration := &MockMigration{id: "m1", version: 1, description: "m1"}

	err := m.executeRollback(context.Background(), migration, RollbackOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "record start failed")
}

func TestMigrator_executeRollback_DownFails_ReturnsDownErr_Round23(t *testing.T) {
	t.Parallel()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)
	db.On("Model", mock.Anything).Return(q).Maybe()

	create1 := q.On("Create").Return(nil).Once()
	create2 := q.On("Create").Return(nil).Once()
	mock.InOrder(create1, create2)

	migration := &MockMigration{id: "m1", version: 1, description: "m1"}
	downErr := errors.New("down failed")
	migration.On("Down", mock.Anything, mock.Anything).Return(downErr).Once()

	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	err := m.executeRollback(context.Background(), migration, RollbackOptions{})
	require.Error(t, err)
	require.Equal(t, downErr, err)
}

func TestMigrator_executeRollback_Success_RecordFailureReturnsWrap_Round23(t *testing.T) {
	t.Parallel()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)
	db.On("Model", mock.Anything).Return(q).Maybe()

	create1 := q.On("Create").Return(nil).Once()
	create2 := q.On("Create").Return(errors.New("record failed")).Once()
	mock.InOrder(create1, create2)

	migration := &MockMigration{id: "m1", version: 1, description: "m1"}
	migration.On("Down", mock.Anything, mock.Anything).Return(nil).Once()

	m := NewMigrator(db, NewRegistry(), zap.NewNop())
	err := m.executeRollback(context.Background(), migration, RollbackOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rollback succeeded but failed to record")
}

func TestMigrator_GetRollbackPlan_TargetStepsDefault_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Version: 1, Status: StatusApplied},
			{ID: "m2", Version: 2, Status: StatusApplied},
			{ID: "m3", Version: 3, Status: StatusApplied},
		}
	}).Return(nil).Maybe()

	plan, err := m.GetRollbackPlan(context.Background(), RollbackOptions{Target: "m2"})
	require.NoError(t, err)
	require.Len(t, plan, 1)
	require.Equal(t, "m3", plan[0].ID)

	plan, err = m.GetRollbackPlan(context.Background(), RollbackOptions{Steps: 2})
	require.NoError(t, err)
	require.Len(t, plan, 2)

	plan, err = m.GetRollbackPlan(context.Background(), RollbackOptions{})
	require.NoError(t, err)
	require.Len(t, plan, 1)
}

func TestMigrator_RollbackWithOptions_NonDryRun_AcquireLockFailure_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	m := NewMigrator(db, NewRegistry(), zap.NewNop())

	q.On("Create").Return(errors.New("lock already exists")).Once()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*MigrationStatus)
		dest.IsLocked = true
		dest.LockedBy = "other"
		dest.LockedAt = time.Now()
	}).Return(nil).Once()

	err := m.RollbackWithOptions(context.Background(), RollbackOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to acquire migration lock")
}

func TestMigrator_RollbackWithOptions_NonDryRun_ExecutesAndReleases_Round23(t *testing.T) {
	t.Parallel()

	db, q := newMigratorTestDB()
	registry := NewRegistry()

	migration := &MockMigration{id: "m1", version: 1, description: "m1"}
	migration.On("Down", mock.Anything, mock.Anything).Return(nil).Once()
	registry.MustRegister(migration)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe() // lock + history writes
	q.On("Delete").Return(nil).Maybe() // release lock
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Version: 1, Status: StatusApplied},
		}
	}).Return(nil).Once()

	require.NoError(t, m.RollbackWithOptions(context.Background(), RollbackOptions{Force: true}))
}
