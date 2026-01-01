// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockObjectRepository is a mock implementation of interfaces.ObjectRepository
// using testify/mock for expectation-based testing.
type MockObjectRepository struct {
	mock.Mock
}

// NewMockObjectRepository creates a new mock object repository
func NewMockObjectRepository() *MockObjectRepository {
	return &MockObjectRepository{}
}

// ===== Core Object Operations =====

// CreateObject mocks the CreateObject method
func (m *MockObjectRepository) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// GetObject mocks the GetObject method
func (m *MockObjectRepository) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

// UpdateObject mocks the UpdateObject method
func (m *MockObjectRepository) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// UpdateObjectWithHistory mocks the UpdateObjectWithHistory method
func (m *MockObjectRepository) UpdateObjectWithHistory(ctx context.Context, object any, updatedBy string) error {
	args := m.Called(ctx, object, updatedBy)
	return args.Error(0)
}

// DeleteObject mocks the DeleteObject method
func (m *MockObjectRepository) DeleteObject(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// GetObjectsByActor mocks the GetObjectsByActor method
func (m *MockObjectRepository) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// ===== Status Operations =====

// GetStatus mocks the GetStatus method
func (m *MockObjectRepository) GetStatus(ctx context.Context, statusID string) (any, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0), args.Error(1)
}

// GetUserStatusCount mocks the GetUserStatusCount method
func (m *MockObjectRepository) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// GetStatusReplyCount mocks the GetStatusReplyCount method
func (m *MockObjectRepository) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

// ===== Reply Operations =====

// CountObjectReplies mocks the CountObjectReplies method
func (m *MockObjectRepository) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// CountReplies mocks the CountReplies method
func (m *MockObjectRepository) CountReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// GetReplies mocks the GetReplies method
func (m *MockObjectRepository) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// IncrementReplyCount mocks the IncrementReplyCount method
func (m *MockObjectRepository) IncrementReplyCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// GetReplyCount mocks the GetReplyCount method
func (m *MockObjectRepository) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// ===== Tombstone Operations =====

// TombstoneObject mocks the TombstoneObject method
func (m *MockObjectRepository) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

// CreateTombstone mocks the CreateTombstone method
func (m *MockObjectRepository) CreateTombstone(ctx context.Context, tombstone *models.Tombstone) error {
	args := m.Called(ctx, tombstone)
	return args.Error(0)
}

// GetTombstone mocks the GetTombstone method
func (m *MockObjectRepository) GetTombstone(ctx context.Context, objectID string) (*models.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Tombstone), args.Error(1)
}

// IsTombstoned mocks the IsTombstoned method
func (m *MockObjectRepository) IsTombstoned(ctx context.Context, objectID string) (bool, error) {
	args := m.Called(ctx, objectID)
	return args.Bool(0), args.Error(1)
}

// GetTombstonesByActor mocks the GetTombstonesByActor method
func (m *MockObjectRepository) GetTombstonesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Tombstone), args.String(1), args.Error(2)
}

// GetTombstonesByType mocks the GetTombstonesByType method
func (m *MockObjectRepository) GetTombstonesByType(ctx context.Context, formerType string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	args := m.Called(ctx, formerType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Tombstone), args.String(1), args.Error(2)
}

// CleanupExpiredTombstones mocks the CleanupExpiredTombstones method
func (m *MockObjectRepository) CleanupExpiredTombstones(ctx context.Context, batchSize int) (int, error) {
	args := m.Called(ctx, batchSize)
	return args.Int(0), args.Error(1)
}

// ReplaceObjectWithTombstone mocks the ReplaceObjectWithTombstone method
func (m *MockObjectRepository) ReplaceObjectWithTombstone(ctx context.Context, objectID, formerType, deletedBy string) error {
	args := m.Called(ctx, objectID, formerType, deletedBy)
	return args.Error(0)
}


// ===== Update History Operations =====

// CreateUpdateHistory mocks the CreateUpdateHistory method
func (m *MockObjectRepository) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

// GetUpdateHistory mocks the GetUpdateHistory method
func (m *MockObjectRepository) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// GetObjectHistory mocks the GetObjectHistory method
func (m *MockObjectRepository) GetObjectHistory(ctx context.Context, objectID string) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// ===== Collection Operations =====

// AddToCollection mocks the AddToCollection method
func (m *MockObjectRepository) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

// RemoveFromCollection mocks the RemoveFromCollection method
func (m *MockObjectRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

// GetCollectionItems mocks the GetCollectionItems method
func (m *MockObjectRepository) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CollectionItem), args.String(1), args.Error(2)
}

// IsInCollection mocks the IsInCollection method
func (m *MockObjectRepository) IsInCollection(ctx context.Context, collection, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

// CountCollectionItems mocks the CountCollectionItems method
func (m *MockObjectRepository) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// ===== Quote Operations =====

// CountQuotes mocks the CountQuotes method
func (m *MockObjectRepository) CountQuotes(ctx context.Context, noteID string) (int, error) {
	args := m.Called(ctx, noteID)
	return args.Int(0), args.Error(1)
}

// CountWithdrawnQuotes mocks the CountWithdrawnQuotes method
func (m *MockObjectRepository) CountWithdrawnQuotes(ctx context.Context, noteID string) (int, error) {
	args := m.Called(ctx, noteID)
	return args.Int(0), args.Error(1)
}

// CreateQuoteRelationship mocks the CreateQuoteRelationship method
func (m *MockObjectRepository) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	args := m.Called(ctx, quote)
	return args.Error(0)
}

// GetQuotesForNote mocks the GetQuotesForNote method
func (m *MockObjectRepository) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	args := m.Called(ctx, noteID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.QuoteRelationship), args.String(1), args.Error(2)
}

// IsQuoted mocks the IsQuoted method
func (m *MockObjectRepository) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	args := m.Called(ctx, actorID, noteID)
	return args.Bool(0), args.Error(1)
}

// WithdrawQuote mocks the WithdrawQuote method
func (m *MockObjectRepository) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	args := m.Called(ctx, quoteNoteID)
	return args.Error(0)
}

// WithdrawStatusFromQuotes mocks the WithdrawStatusFromQuotes method
func (m *MockObjectRepository) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// UpdateQuotePermissions mocks the UpdateQuotePermissions method
func (m *MockObjectRepository) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	args := m.Called(ctx, statusID, permissions)
	return args.Error(0)
}

// IsQuoteAllowed mocks the IsQuoteAllowed method
func (m *MockObjectRepository) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	args := m.Called(ctx, statusID, quoterID)
	return args.Bool(0), args.Error(1)
}

// GetQuoteType mocks the GetQuoteType method
func (m *MockObjectRepository) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	args := m.Called(ctx, statusID)
	return args.String(0), args.Error(1)
}

// IsWithdrawnFromQuotes mocks the IsWithdrawnFromQuotes method
func (m *MockObjectRepository) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	args := m.Called(ctx, statusID)
	return args.Bool(0), args.Error(1)
}

// GetQuotesOfStatus mocks the GetQuotesOfStatus method
func (m *MockObjectRepository) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// ===== Thread Operations =====

// GetMissingReplies mocks the GetMissingReplies method
func (m *MockObjectRepository) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// MarkThreadAsSynced mocks the MarkThreadAsSynced method
func (m *MockObjectRepository) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// SyncThreadFromRemote mocks the SyncThreadFromRemote method
func (m *MockObjectRepository) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// SyncMissingRepliesFromRemote mocks the SyncMissingRepliesFromRemote method
func (m *MockObjectRepository) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetThreadContext mocks the GetThreadContext method
func (m *MockObjectRepository) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ThreadContext), args.Error(1)
}

// Ensure MockObjectRepository implements interfaces.ObjectRepository
var _ interfaces.ObjectRepository = (*MockObjectRepository)(nil)
