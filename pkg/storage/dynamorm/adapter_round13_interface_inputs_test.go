package dynamorm

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStorageAdapter_InterfaceInputs_TypedBranches(t *testing.T) {
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

	require.NoError(t, adapter.CreateUser(ctx, &storage.User{Username: "alice"}))
	require.NoError(t, adapter.UpdateUser(ctx, &storage.User{Username: "alice"}))
	require.NoError(t, adapter.UpdateUserPreferences(ctx, "alice", map[string]any{"theme": "dark"}))

	require.Error(t, adapter.CreateNotification(ctx, &models.Notification{ID: "notif-1"}))
}
