package migrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type mockMigration struct {
	BaseMigration

	upCalls   int
	downCalls int
	upErr     error
	downErr   error
}

func (m *mockMigration) Up(_ context.Context, _ core.DB) error {
	m.upCalls++
	return m.upErr
}

func (m *mockMigration) Down(_ context.Context, _ core.DB) error {
	m.downCalls++
	return m.downErr
}

func newMigratorTestDB() (*dynamormMocks.MockDB, *dynamormMocks.MockQuery) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()

	return db, q
}

func TestNewMigrator_ExecutorFallback_Round22(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "")
	db, _ := newMigratorTestDB()
	registry := NewRegistry()

	m := NewMigrator(db, registry, zap.NewNop())
	require.Equal(t, "local", m.executor)
}

func TestMigrator_GetAppliedMigrations_Round22(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = []MigrationHistory{
			{ID: "m1", Status: StatusApplied},
			{ID: "m2", Status: "failed"},
		}
	}).Return(nil).Once()

	applied, err := m.GetAppliedMigrations(context.Background())
	require.NoError(t, err)
	require.True(t, applied["m1"])
	require.False(t, applied["m2"])
}

func TestMigrator_GetMigrationStatus_NotFoundReturnsEmpty_Round22(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("First", mock.Anything).Return(errors.New("not found")).Once()

	status, err := m.GetMigrationStatus(context.Background())
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Empty(t, status.PK)
	require.Empty(t, status.SK)
}

func TestMigrator_acquireLock_Round22_StaleLockUpdates(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(errors.New("already exists")).Once()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*MigrationStatus)
		dest.IsLocked = true
		dest.LockedBy = "other"
		dest.LockedAt = time.Now().Add(-11 * time.Minute)
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	require.NoError(t, m.acquireLock(context.Background()))
}

func TestMigrator_Run_NoPending_Round22(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()
	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe() // lock + history/status writes
	q.On("Delete").Return(nil).Maybe() // release lock
	q.On("All", mock.Anything).Return(nil).Maybe()

	require.NoError(t, m.Run(context.Background()))
}

func TestMigrator_Run_ExecutesMigration_Round22(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()

	mig := &mockMigration{
		BaseMigration: NewBaseMigration("m1", 1, "m1"),
	}
	registry.MustRegister(mig)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("Create").Return(nil).Maybe() // lock + history/status writes
	q.On("Delete").Return(nil).Maybe() // release lock
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]MigrationHistory)
		*dest = nil
	}).Return(nil).Maybe()
	q.On("First", mock.Anything).Return(errors.New("not found")).Maybe()

	require.NoError(t, m.Run(context.Background()))
	require.Equal(t, 1, mig.upCalls)
}

func TestMigrator_Migrate_DryRun_Round22(t *testing.T) {
	db, q := newMigratorTestDB()
	registry := NewRegistry()

	mig := &mockMigration{
		BaseMigration: NewBaseMigration("m1", 1, "m1"),
	}
	registry.MustRegister(mig)

	m := NewMigrator(db, registry, zap.NewNop())

	q.On("All", mock.Anything).Return(nil).Maybe()

	require.NoError(t, m.Migrate(context.Background(), MigrateOptions{DryRun: true}))
	require.Equal(t, 0, mig.upCalls)
}
