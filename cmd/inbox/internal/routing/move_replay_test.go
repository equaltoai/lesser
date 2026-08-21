package routing

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func TestInboxMoveReplayRefreshesMigrationAndTombstone(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Move{}))

	repos, err := factory.NewRepositoryFactory(db, models.MainTableName, zap.NewNop())
	require.NoError(t, err)
	handler := &InboxHandler{storageAdapter: repos}

	firstMigration := models.NewMove(
		"https://remote.example/activities/move-1",
		"https://remote.example/users/alice",
		"https://new.example/users/alice",
	)
	firstMigration.Published = time.Now().UTC().Add(-24 * time.Hour)
	firstMigration.SetTTL(time.Now().Add(time.Hour))
	require.NoError(t, handler.storeMoveMigration(context.Background(), firstMigration))

	secondMigration := models.NewMove(
		"https://remote.example/activities/move-2",
		firstMigration.Actor,
		firstMigration.Target,
	)
	secondMigration.Published = firstMigration.Published.Add(24 * time.Hour)
	secondMigration.SetTTL(time.Now().Add(30 * 24 * time.Hour))
	require.NoError(t, handler.storeMoveMigration(context.Background(), secondMigration),
		"a genuine repeat migration to the same target must refresh the retained record")

	var persistedMigration models.Move
	require.NoError(t, db.Model(&models.Move{}).
		Where("PK", "=", secondMigration.PK).
		Where("SK", "=", secondMigration.SK).
		First(&persistedMigration))
	require.Equal(t, secondMigration.ID, persistedMigration.ID)
	require.Equal(t, secondMigration.Published, persistedMigration.Published)
	require.Equal(t, secondMigration.TTL, persistedMigration.TTL)

	require.NoError(t, handler.createAccountTombstone(
		context.Background(),
		"https://remote.example/users/alice",
		"https://new.example/users/alice",
	))
	require.NoError(t, handler.createAccountTombstone(
		context.Background(),
		"https://remote.example/users/alice",
		"https://newer.example/users/alice",
	), "a later migration must refresh the account tombstone summary")

	var persistedTombstone models.Tombstone
	require.NoError(t, db.Model(&models.Tombstone{}).
		Where("PK", "=", "OBJECT#https://remote.example/users/alice").
		Where("SK", "=", "TOMBSTONE").
		First(&persistedTombstone))
	require.Equal(t, "Account moved to https://newer.example/users/alice", persistedTombstone.Summary)

	require.Len(t, fake.Items(models.MainTableName), 2)
}
