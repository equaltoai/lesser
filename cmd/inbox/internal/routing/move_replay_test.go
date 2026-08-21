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

func TestInboxMoveReplayAcceptsExistingMigrationAndTombstone(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Move{}))

	repos, err := factory.NewRepositoryFactory(db, models.MainTableName, zap.NewNop())
	require.NoError(t, err)
	handler := &InboxHandler{storageAdapter: repos}

	migration := models.NewMove(
		"https://remote.example/activities/move-1",
		"https://remote.example/users/alice",
		"https://new.example/users/alice",
	)
	migration.SetTTL(time.Now().Add(30 * 24 * time.Hour))
	require.NoError(t, handler.storeMoveMigration(context.Background(), migration))
	require.NoError(t, handler.storeMoveMigration(context.Background(), migration),
		"redelivery after the migration write must continue to follower side effects")

	require.NoError(t, handler.createAccountTombstone(
		context.Background(),
		"https://remote.example/users/alice",
		"https://new.example/users/alice",
	))
	require.NoError(t, handler.createAccountTombstone(
		context.Background(),
		"https://remote.example/users/alice",
		"https://new.example/users/alice",
	), "a replayed account tombstone must be idempotent")

	require.Len(t, fake.Items(models.MainTableName), 2)
}
