package dynamorm

import (
	"errors"
	"fmt"

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

	// Common fields
	db        core.DB
	tableName string
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
		db:              db,
		tableName:       tableName,
	}
}

// RepositoryAdapter is a generic adapter for repository interfaces
// It allows for adapting DynamORM repositories to existing interfaces
type RepositoryAdapter struct {
	DB        core.DB
	TableName string
}

// NewRepositoryAdapter creates a new RepositoryAdapter
func NewRepositoryAdapter(db core.DB, tableName string) *RepositoryAdapter {
	return &RepositoryAdapter{
		DB:        db,
		TableName: tableName,
	}
}

// AdapterError wraps errors from DynamORM and provides context
type AdapterError struct {
	OriginalError error
	Operation     string
	Entity        string
	ID            string
}

// Error implements the error interface
func (e *AdapterError) Error() string {
	return fmt.Sprintf("%s %s (ID: %s): %v", e.Operation, e.Entity, e.ID, e.OriginalError)
}

// Unwrap returns the original error
func (e *AdapterError) Unwrap() error {
	return e.OriginalError
}

// NewAdapterError creates a new AdapterError
func NewAdapterError(err error, operation, entity, id string) error {
	if err == nil {
		return nil
	}
	return &AdapterError{
		OriginalError: err,
		Operation:     operation,
		Entity:        entity,
		ID:            id,
	}
}

// MapRepositoryError maps DynamORM errors to storage errors with context
func MapRepositoryError(err error, operation, entity, id string) error {
	if err == nil {
		return nil
	}

	// Map common error types
	mappedErr := MapError(err)

	// Add context to the error
	return NewAdapterError(mappedErr, operation, entity, id)
}

// IsNotFoundError checks if an error is a not found error
func IsNotFoundError(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// IsConditionalCheckFailedError checks if an error is a conditional check failed error
func IsConditionalCheckFailedError(err error) bool {
	return errors.Is(err, ErrConditionalCheckFailed)
}

// IsThrottlingError checks if an error is a throttling error
func IsThrottlingError(err error) bool {
	return errors.Is(err, ErrThrottling)
}

// Implement storage.Storage interface methods as they are migrated to DynamORM
// For example:
/*
func (a *StorageAdapter) GetUser(ctx context.Context, username string) (*storage.User, error) {
	// Call the DynamORM repository
	dynamoUser, err := a.userRepo.GetUser(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUser", "User", username)
	}

	// Convert from DynamORM model to storage model
	storageUser := &storage.User{
		ID:       dynamoUser.ID,
		Username: dynamoUser.Username,
		Email:    dynamoUser.Email,
		// Map other fields...
	}

	return storageUser, nil
}
*/

// For methods not yet migrated, delegate to the original storage implementation
// This allows for incremental migration
func (a *StorageAdapter) delegateToOriginal(methodName string, args ...any) (any, error) {
	if a.originalStorage == nil {
		return nil, fmt.Errorf("method %s not implemented and no original storage available", methodName)
	}

	// Log the delegation for monitoring migration progress
	// log.Printf("Delegating %s to original storage implementation", methodName)

	// The actual delegation happens in the specific method implementations
	return nil, fmt.Errorf("delegation not implemented for method %s", methodName)
}

// Example of a delegated method:
/*
func (a *StorageAdapter) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	// Delegate to the original storage implementation
	_, err := a.delegateToOriginal("CreateActor", ctx, actor, privateKey)
	if err != nil {
		// If it's our generic error about delegation not being implemented,
		// call the original method directly
		if strings.Contains(err.Error(), "delegation not implemented") {
			return a.originalStorage.CreateActor(ctx, actor, privateKey)
		}
		return err
	}
	return nil
}
*/
