// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockActorRepository is a mock implementation of interfaces.ActorRepository
// using testify/mock for expectation-based testing.
type MockActorRepository struct {
	mock.Mock
}

// NewMockActorRepository creates a new mock actor repository
func NewMockActorRepository() *MockActorRepository {
	return &MockActorRepository{}
}

// Core actor operations

// CreateActor mocks the CreateActor method
func (m *MockActorRepository) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

// GetActor mocks the GetActor method
func (m *MockActorRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetActorByUsername mocks the GetActorByUsername method
func (m *MockActorRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetActorByNumericID mocks the GetActorByNumericID method
func (m *MockActorRepository) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	args := m.Called(ctx, numericID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetActorWithMetadata mocks the GetActorWithMetadata method
func (m *MockActorRepository) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	args := m.Called(ctx, username)
	var actor *activitypub.Actor
	var metadata *storage.ActorMetadata
	if args.Get(0) != nil {
		actor = args.Get(0).(*activitypub.Actor)
	}
	if args.Get(1) != nil {
		metadata = args.Get(1).(*storage.ActorMetadata)
	}
	return actor, metadata, args.Error(2)
}

// GetActorPrivateKey mocks the GetActorPrivateKey method
func (m *MockActorRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

// UpdateActor mocks the UpdateActor method
func (m *MockActorRepository) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// UpdateActorLastStatusTime mocks the UpdateActorLastStatusTime method
func (m *MockActorRepository) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// SetActorFields mocks the SetActorFields method
func (m *MockActorRepository) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	args := m.Called(ctx, username, fields)
	return args.Error(0)
}

// DeleteActor mocks the DeleteActor method
func (m *MockActorRepository) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// Search and discovery

// SearchAccounts mocks the SearchAccounts method
func (m *MockActorRepository) SearchAccounts(ctx context.Context, query string, limit int, resolve bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, resolve, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// GetSearchSuggestions mocks the GetSearchSuggestions method
func (m *MockActorRepository) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchSuggestion), args.Error(1)
}

// GetAccountSuggestions mocks the GetAccountSuggestions method
func (m *MockActorRepository) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// Migration operations

// UpdateAlsoKnownAs mocks the UpdateAlsoKnownAs method
func (m *MockActorRepository) UpdateAlsoKnownAs(ctx context.Context, username string, alsoKnownAs []string) error {
	args := m.Called(ctx, username, alsoKnownAs)
	return args.Error(0)
}

// UpdateMovedTo mocks the UpdateMovedTo method
func (m *MockActorRepository) UpdateMovedTo(ctx context.Context, username string, movedTo string) error {
	args := m.Called(ctx, username, movedTo)
	return args.Error(0)
}

// CheckAlsoKnownAs mocks the CheckAlsoKnownAs method
func (m *MockActorRepository) CheckAlsoKnownAs(ctx context.Context, username string, targetActorID string) (bool, error) {
	args := m.Called(ctx, username, targetActorID)
	return args.Bool(0), args.Error(1)
}

// GetActorMigrationInfo mocks the GetActorMigrationInfo method
func (m *MockActorRepository) GetActorMigrationInfo(ctx context.Context, username string) (*interfaces.MigrationInfo, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.MigrationInfo), args.Error(1)
}

// RemoveAccountSuggestion mocks the RemoveAccountSuggestion method
func (m *MockActorRepository) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	args := m.Called(ctx, userID, targetID)
	return args.Error(0)
}

// GetCachedRemoteActor mocks the GetCachedRemoteActor method
func (m *MockActorRepository) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	args := m.Called(ctx, handle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// Ensure MockActorRepository implements interfaces.ActorRepository
var _ interfaces.ActorRepository = (*MockActorRepository)(nil)
