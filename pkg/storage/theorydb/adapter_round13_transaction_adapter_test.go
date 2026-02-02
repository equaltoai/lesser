package theorydb

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageInterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTransactionAdapter_CoversCoreMethods(t *testing.T) {
	db := newPermissiveDynamormDB(t)
	logger := zap.NewNop()

	repoStorage := &permissiveRepositoryStorage{
		SimpleRepositoryStorage: &SimpleRepositoryStorage{
			db:        db,
			tableName: "test-table",
			logger:    logger,
		},
		actor:        repositories.NewActorRepository(db, "test-table", logger),
		activity:     repositories.NewActivityRepository(db, "test-table", logger, nil),
		user:         repositories.NewUserRepository(db, "test-table", logger),
		account:      repositories.NewAccountRepository(db, "test-table", "example.com", logger),
		object:       repositories.NewObjectRepository(db, "test-table", "example.com", logger),
		status:       repositories.NewStatusRepository(db, "test-table", logger, nil),
		timeline:     repositories.NewTimelineRepository(db, "test-table", logger, nil),
		relationship: repositories.NewRelationshipRepository(db, "test-table", logger),
		like:         repositories.NewLikeRepository(db, "test-table", logger),
		notification: repositories.NewNotificationRepository(db, "test-table", logger, nil),
	}

	adapter := NewStorageAdapter(repoStorage)
	ctx := context.Background()

	tx, err := adapter.BeginTransaction(ctx)
	require.NoError(t, err)

	ta, ok := tx.(*transactionAdapter)
	require.True(t, ok)
	require.True(t, ta.IsActive())
	require.Equal(t, ctx, ta.GetContext())

	require.NoError(t, ta.Rollback())
	require.False(t, ta.IsActive())
	require.NoError(t, ta.Rollback())
	require.Error(t, ta.Commit())

	tx2, err := adapter.BeginTransaction(ctx)
	require.NoError(t, err)
	ta2 := tx2.(*transactionAdapter)
	require.NoError(t, ta2.Commit())
	require.False(t, ta2.IsActive())

	err = adapter.ExecuteInTransaction(ctx, func(_ storageInterfaces.Transaction) error {
		return errors.New("boom")
	})
	require.Error(t, err)

	// Exercise Tx* methods on a fresh transaction.
	tx3, err := adapter.BeginTransaction(ctx)
	require.NoError(t, err)
	ta3 := tx3.(*transactionAdapter)

	require.Error(t, ta3.TxCreateActor(&activitypub.Actor{}, ""))
	require.Error(t, ta3.TxUpdateActor(&activitypub.Actor{}))
	require.NoError(t, ta3.TxDeleteActor("alice"))

	require.Error(t, ta3.TxCreateUser("wrong-type"))
	require.Error(t, ta3.TxUpdateUser("wrong-type"))
	require.NoError(t, ta3.TxDeleteUser("alice"))

	_ = ta3.TxCreateUser(&storage.User{Username: "alice"})
	_ = ta3.TxUpdateUser(&storage.User{Username: "alice"})

	require.NoError(t, ta3.TxCreateObject(map[string]any{"id": "x", "type": "Note"}))
	require.NoError(t, ta3.TxUpdateObject("ignored", map[string]any{"id": "x", "type": "Note"}))
	require.NoError(t, ta3.TxDeleteObject("object-1"))

	require.Error(t, ta3.TxCreateActivity(&activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "x", Type: "Create"}}))
	require.Error(t, ta3.TxUpdateActivity(&activitypub.Activity{}))
	require.Error(t, ta3.TxDeleteActivity("activity-1"))

	require.NoError(t, ta3.TxCreateRelationship("alice", "bob", "activity-1"))
	require.NoError(t, ta3.TxRemoveRelationship("alice", "bob"))
	require.NoError(t, ta3.TxUpdateRelationshipStatus("alice", "bob", "accepted"))
}
