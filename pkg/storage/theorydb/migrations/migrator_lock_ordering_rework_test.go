package migrations

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
	tabletheory "github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type staleLockRaceDB struct {
	*fakedb.Fake
	readsComplete chan struct{}
	firstUpdated  chan struct{}
	readCount     atomic.Int32
}

func newStaleLockRaceDB() *staleLockRaceDB {
	return &staleLockRaceDB{
		Fake:          fakedb.New(),
		readsComplete: make(chan struct{}),
		firstUpdated:  make(chan struct{}),
	}
}

func (f *staleLockRaceDB) GetItem(
	ctx context.Context,
	input *dynamodb.GetItemInput,
	options ...func(*dynamodb.Options),
) (*dynamodb.GetItemOutput, error) {
	output, err := f.Fake.GetItem(ctx, input, options...)
	if f.readCount.Add(1) == 2 {
		close(f.readsComplete)
	}
	<-f.readsComplete
	return output, err
}

func (f *staleLockRaceDB) UpdateItem(
	ctx context.Context,
	input *dynamodb.UpdateItemInput,
	options ...func(*dynamodb.Options),
) (*dynamodb.UpdateItemOutput, error) {
	owner := stringAttributeValue(input.ExpressionAttributeValues)
	if owner == "contender-2" {
		<-f.firstUpdated
	}

	output, err := f.Fake.UpdateItem(ctx, input, options...)
	if owner == "contender-1" {
		close(f.firstUpdated)
	}
	return output, err
}

func stringAttributeValue(values map[string]types.AttributeValue) string {
	for _, value := range values {
		if stringValue, ok := value.(*types.AttributeValueMemberS); ok {
			if stringValue.Value == "contender-1" || stringValue.Value == "contender-2" {
				return stringValue.Value
			}
		}
	}
	return ""
}

type unreadableLockDB struct {
	*fakedb.Fake
	err error
}

func (f *unreadableLockDB) GetItem(
	context.Context,
	*dynamodb.GetItemInput,
	...func(*dynamodb.Options),
) (*dynamodb.GetItemOutput, error) {
	return nil, f.err
}

func (f *unreadableLockDB) Query(
	context.Context,
	*dynamodb.QueryInput,
	...func(*dynamodb.Options),
) (*dynamodb.QueryOutput, error) {
	return nil, f.err
}

func TestMigratorAcquireLockRacingStaleTakeoverRejectsSecond(t *testing.T) {
	fake := newStaleLockRaceDB()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&MigrationStatus{}))

	staleLock := &MigrationStatus{
		PK:       migrationLockPK,
		SK:       "ACTIVE",
		IsLocked: true,
		LockedBy: "stale-holder",
		LockedAt: time.Now().Add(-11 * time.Minute),
	}
	require.NoError(t, db.Model(staleLock).Create())

	first := NewMigrator(db, NewRegistry(), zap.NewNop())
	first.executor = "contender-1"
	second := NewMigrator(db, NewRegistry(), zap.NewNop())
	second.executor = "contender-2"

	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	go func() { firstResult <- first.acquireLock(context.Background()) }()
	go func() { secondResult <- second.acquireLock(context.Background()) }()

	require.NoError(t, <-firstResult)
	require.Error(t, <-secondResult, "a contender that observed the same stale lock must lose after the first takeover")
}

func TestMigratorAcquireLockCreateFailureAndUnreadableRowReturnsError(t *testing.T) {
	readErr := errors.New("lock row unavailable")
	fake := &unreadableLockDB{Fake: fakedb.New(), err: readErr}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&MigrationStatus{}))
	require.NoError(t, db.Model(&MigrationStatus{
		PK:       migrationLockPK,
		SK:       "ACTIVE",
		IsLocked: true,
		LockedBy: "holder",
		LockedAt: time.Now(),
	}).Create())

	migrator := NewMigrator(db, NewRegistry(), zap.NewNop())
	err = migrator.acquireLock(context.Background())
	require.ErrorIs(t, err, readErr)
}

func TestMigratorRollbackUsesHighestNumericVersion(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&MigrationStatus{}))
	require.NoError(t, db.CreateTable(&MigrationHistory{}))

	lowVersion := &mockMigration{BaseMigration: NewBaseMigration("z-low-version", 2, "low version")}
	highVersion := &mockMigration{BaseMigration: NewBaseMigration("a-high-version", 10, "high version")}
	registry := NewRegistry()
	registry.MustRegister(lowVersion)
	registry.MustRegister(highVersion)

	for _, migration := range []Migration{lowVersion, highVersion} {
		history := &MigrationHistory{
			ID:      migration.ID(),
			Version: migration.Version(),
			Status:  migrationStatusApplied,
		}
		history.PK, history.SK = history.GetTableKeys()
		require.NoError(t, db.Model(history).Create())
	}

	migrator := NewMigrator(db, registry, zap.NewNop())
	require.NoError(t, migrator.Rollback(context.Background()))
	require.Equal(t, 1, highVersion.downCalls, "rollback must select the highest numeric migration version")
	require.Zero(t, lowVersion.downCalls)
}
