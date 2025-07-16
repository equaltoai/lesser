package dynamorm

import (
	"github.com/aron23/lesser/pkg/storage"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// StorageAdapter adapts the DynamORM repositories to the storage.Storage interface
// This allows for incremental migration to DynamORM while maintaining backward compatibility
type StorageAdapter struct {
	// Add repository fields as they are implemented
	// For example:
	// userRepo *UserRepository
	// actorRepo *ActorRepository
	// etc.

	// Keep a reference to the original storage implementation for methods not yet migrated
	originalStorage storage.Storage
}

// NewStorageAdapter creates a new StorageAdapter
func NewStorageAdapter(db core.DB, tableName string, originalStorage storage.Storage) *StorageAdapter {
	return &StorageAdapter{
		// Initialize repositories as they are implemented
		// For example:
		// userRepo: NewUserRepository(db, tableName),
		// actorRepo: NewActorRepository(db, tableName),
		// etc.

		originalStorage: originalStorage,
	}
}

// Implement storage.Storage interface methods as they are migrated to DynamORM
// For example:
/*
func (a *StorageAdapter) GetUser(ctx context.Context, username string) (*storage.User, error) {
	return a.userRepo.GetUser(ctx, username)
}
*/

// For methods not yet migrated, delegate to the original storage implementation
// This allows for incremental migration
// For example:
/*
func (a *StorageAdapter) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	return a.originalStorage.CreateActor(ctx, actor, privateKey)
}
*/
