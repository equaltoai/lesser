// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockSocialRepository is a mock implementation of interfaces.SocialRepository
// using testify/mock for expectation-based testing.
type MockSocialRepository struct {
	mock.Mock
}

// NewMockSocialRepository creates a new mock social repository
func NewMockSocialRepository() *MockSocialRepository {
	return &MockSocialRepository{}
}

// CreateBlock mocks the CreateBlock method
func (m *MockSocialRepository) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// DeleteBlock mocks the DeleteBlock method
func (m *MockSocialRepository) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	args := m.Called(ctx, actor, blockedActor)
	return args.Error(0)
}

// GetBlock mocks the GetBlock method
func (m *MockSocialRepository) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, actor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

// IsBlocked mocks the IsBlocked method
func (m *MockSocialRepository) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

// GetBlockedUsers mocks the GetBlockedUsers method
func (m *MockSocialRepository) GetBlockedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

// GetBlockedByUsers mocks the GetBlockedByUsers method
func (m *MockSocialRepository) GetBlockedByUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

// CreateMute mocks the CreateMute method
func (m *MockSocialRepository) CreateMute(ctx context.Context, mute *storage.Mute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

// DeleteMute mocks the DeleteMute method
func (m *MockSocialRepository) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	args := m.Called(ctx, actor, mutedActor)
	return args.Error(0)
}

// GetMute mocks the GetMute method
func (m *MockSocialRepository) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	args := m.Called(ctx, actor, mutedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Mute), args.Error(1)
}

// IsMuted mocks the IsMuted method
func (m *MockSocialRepository) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

// GetMutedUsers mocks the GetMutedUsers method
func (m *MockSocialRepository) GetMutedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Mute), args.String(1), args.Error(2)
}

// CreateAnnounce mocks the CreateAnnounce method
func (m *MockSocialRepository) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

// DeleteAnnounce mocks the DeleteAnnounce method
func (m *MockSocialRepository) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// GetAnnounce mocks the GetAnnounce method
func (m *MockSocialRepository) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

// GetStatusAnnounces mocks the GetStatusAnnounces method
func (m *MockSocialRepository) GetStatusAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

// HasUserAnnounced mocks the HasUserAnnounced method
func (m *MockSocialRepository) HasUserAnnounced(ctx context.Context, actor, object string) (bool, error) {
	args := m.Called(ctx, actor, object)
	return args.Bool(0), args.Error(1)
}

// GetActorAnnounces mocks the GetActorAnnounces method
func (m *MockSocialRepository) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

// CountObjectAnnounces mocks the CountObjectAnnounces method
func (m *MockSocialRepository) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// CascadeDeleteAnnounces mocks the CascadeDeleteAnnounces method
func (m *MockSocialRepository) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// CreateAccountPin mocks the CreateAccountPin method
func (m *MockSocialRepository) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

// DeleteAccountPin mocks the DeleteAccountPin method
func (m *MockSocialRepository) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Error(0)
}

// GetAccountPins mocks the GetAccountPins method
func (m *MockSocialRepository) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.AccountPin), args.Error(1)
}

// GetAccountPinsPaginated mocks the GetAccountPinsPaginated method
func (m *MockSocialRepository) GetAccountPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.AccountPin, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.AccountPin), args.String(1), args.Error(2)
}

// IsAccountPinned mocks the IsAccountPinned method
func (m *MockSocialRepository) IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error) {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Bool(0), args.Error(1)
}

// CreateAccountNote mocks the CreateAccountNote method
func (m *MockSocialRepository) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// UpdateAccountNote mocks the UpdateAccountNote method
func (m *MockSocialRepository) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// DeleteAccountNote mocks the DeleteAccountNote method
func (m *MockSocialRepository) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	args := m.Called(ctx, username, targetActorID)
	return args.Error(0)
}

// GetAccountNote mocks the GetAccountNote method
func (m *MockSocialRepository) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, username, targetActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

// CreateStatusPin mocks the CreateStatusPin method
func (m *MockSocialRepository) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

// DeleteStatusPin mocks the DeleteStatusPin method
func (m *MockSocialRepository) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	args := m.Called(ctx, username, statusID)
	return args.Error(0)
}

// GetStatusPins mocks the GetStatusPins method
func (m *MockSocialRepository) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusPin), args.Error(1)
}

// GetStatusPinsPaginated mocks the GetStatusPinsPaginated method
func (m *MockSocialRepository) GetStatusPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.StatusPin, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.StatusPin), args.String(1), args.Error(2)
}

// IsStatusPinned mocks the IsStatusPinned method
func (m *MockSocialRepository) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	args := m.Called(ctx, username, statusID)
	return args.Bool(0), args.Error(1)
}

// ReorderStatusPins mocks the ReorderStatusPins method
func (m *MockSocialRepository) ReorderStatusPins(ctx context.Context, username string, statusIDs []string) error {
	args := m.Called(ctx, username, statusIDs)
	return args.Error(0)
}

// CountUserPinnedStatuses mocks the CountUserPinnedStatuses method
func (m *MockSocialRepository) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// Ensure MockSocialRepository implements interfaces.SocialRepository
var _ interfaces.SocialRepository = (*MockSocialRepository)(nil)
