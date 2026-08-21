package migrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	tabletheory "github.com/theory-cloud/tabletheory/v3"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func newRollbackReworkMigrator(t *testing.T, downErr, historyErr error) (*Migrator, *mockMigration) {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	query := new(dynamormMocks.MockQuery)
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("Create").Return(nil).Once()
	query.On("All", mock.Anything).Run(func(args mock.Arguments) {
		destination := args.Get(0).(*[]MigrationHistory)
		*destination = []MigrationHistory{{
			ID:      "migration-1",
			Version: 1,
			Status:  migrationStatusApplied,
		}}
	}).Return(nil).Once()
	query.On("CreateOrUpdate").Return(historyErr).Once()
	query.On("Delete").Return(nil).Once()

	migration := &mockMigration{
		BaseMigration: NewBaseMigration("migration-1", 1, "migration one"),
		downErr:       downErr,
	}
	registry := NewRegistry()
	registry.MustRegister(migration)

	t.Cleanup(func() {
		db.AssertExpectations(t)
		query.AssertExpectations(t)
	})
	return NewMigrator(db, registry, zap.NewNop()), migration
}

func TestMigratorUpdateMigrationStatusRegistersAndUpsertsV305(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&MigrationStatus{}))
	// MigrationHistory used the same legacy attribute-name tag form. Register it
	// here as well so future status-path tests cannot hide a history-model failure.
	require.NoError(t, db.CreateTable(&MigrationHistory{}))

	migrator := NewMigrator(db, NewRegistry(), zap.NewNop())
	status := &MigrationStatus{
		LastMigrationID: "migration-1",
		LastVersion:     1,
		IsLocked:        true,
		LockedBy:        "executor-1",
		LockedAt:        time.Unix(1_700_000_000, 0).UTC(),
	}
	require.NoError(t, migrator.UpdateMigrationStatus(context.Background(), status))

	items := fake.Items("MigrationStatuses")
	require.Len(t, items, 1)
	for _, attribute := range []string{
		"lastMigrationID",
		"lastVersion",
		"updatedAt",
		"isLocked",
		"lockedBy",
		"lockedAt",
	} {
		require.Contains(t, items[0], attribute, "stored migration attribute name must remain stable")
	}
	require.NotContains(t, items[0], "last_migration_id")
	require.NotContains(t, items[0], "last_version")
	require.NotContains(t, items[0], "updated_at")

	status.LastMigrationID = "migration-2"
	status.LastVersion = 2
	status.IsLocked = false
	require.NoError(t, migrator.UpdateMigrationStatus(context.Background(), status))
	require.Len(t, fake.Items("MigrationStatuses"), 1, "status updates must overwrite the deterministic current row")

	persisted, err := migrator.GetMigrationStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, MigrationStatusKey, persisted.PK)
	require.Equal(t, StatusCurrent, persisted.SK)
	require.Equal(t, "migration-2", persisted.LastMigrationID)
	require.Equal(t, int64(2), persisted.LastVersion)
	require.False(t, persisted.IsLocked)
}

func TestMigratorRunThenRollbackPersistsHistoryTransitionsV305(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&MigrationStatus{}))
	require.NoError(t, db.CreateTable(&MigrationHistory{}))

	migration := &mockMigration{
		BaseMigration: NewBaseMigration("migration-1", 1, "migration one"),
	}
	registry := NewRegistry()
	registry.MustRegister(migration)
	migrator := NewMigrator(db, registry, zap.NewNop())

	require.NoError(t, migrator.Run(context.Background()))
	require.Equal(t, 1, migration.upCalls)
	require.NoError(t, migrator.Rollback(context.Background()))
	require.Equal(t, 1, migration.downCalls)

	histories, err := migrator.GetMigrationHistory(context.Background())
	require.NoError(t, err)
	require.Len(t, histories, 1, "history transitions must replace the deterministic migration row")
	require.Equal(t, "rolled_back", histories[0].Status)
}

func TestGSIHelperReplaysDeterministicMigrationRecordV305(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&GSIMigrationRecord{}))

	helper, err := NewGSIHelper(db, "lesser-test", zap.NewNop())
	require.NoError(t, err)
	definition := GSIDefinition{
		Name:           "GSI7",
		HashKey:        "gsi7PK",
		HashKeyType:    "S",
		ProjectionType: projectionTypeAll,
	}

	require.NoError(t, helper.CreateGSI(context.Background(), definition))
	require.NoError(t, helper.CreateGSI(context.Background(), definition))

	items := fake.Items("GSIMigrationRecords")
	require.Len(t, items, 1, "GSI migration retries must replace the deterministic tracking row")
	require.Contains(t, items[0], "status")
}

func TestMigratorRollbackReportsExecutionAndHistoryErrors(t *testing.T) {
	t.Run("down failure with successful history is returned", func(t *testing.T) {
		downErr := errors.New("down failed")
		migrator, migration := newRollbackReworkMigrator(t, downErr, nil)

		err := migrator.Rollback(context.Background())
		require.ErrorIs(t, err, downErr)
		require.Equal(t, 1, migration.downCalls)
	})

	t.Run("history failure after successful down is returned", func(t *testing.T) {
		historyErr := errors.New("history failed")
		migrator, migration := newRollbackReworkMigrator(t, nil, historyErr)

		err := migrator.Rollback(context.Background())
		require.ErrorIs(t, err, historyErr)
		require.Equal(t, 1, migration.downCalls)
	})

	t.Run("down and history failures are both returned", func(t *testing.T) {
		downErr := errors.New("down failed")
		historyErr := errors.New("history failed")
		migrator, migration := newRollbackReworkMigrator(t, downErr, historyErr)

		err := migrator.Rollback(context.Background())
		require.ErrorIs(t, err, downErr)
		require.ErrorIs(t, err, historyErr)
		require.Equal(t, 1, migration.downCalls)
	})

	t.Run("successful down and history remain successful", func(t *testing.T) {
		migrator, migration := newRollbackReworkMigrator(t, nil, nil)

		require.NoError(t, migrator.Rollback(context.Background()))
		require.Equal(t, 1, migration.downCalls)
	})
}
