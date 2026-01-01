// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockAnnouncementRepository is a mock implementation of interfaces.AnnouncementRepository
// using testify/mock for expectation-based testing.
type MockAnnouncementRepository struct {
	mock.Mock
}

// NewMockAnnouncementRepository creates a new mock announcement repository
func NewMockAnnouncementRepository() *MockAnnouncementRepository {
	return &MockAnnouncementRepository{}
}

// CreateAnnouncement mocks the CreateAnnouncement method
func (m *MockAnnouncementRepository) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

// GetAnnouncement mocks the GetAnnouncement method
func (m *MockAnnouncementRepository) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announcement), args.Error(1)
}

// GetAnnouncements mocks the GetAnnouncements method
func (m *MockAnnouncementRepository) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	args := m.Called(ctx, active)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Announcement), args.Error(1)
}

// GetAnnouncementsPaginated mocks the GetAnnouncementsPaginated method
func (m *MockAnnouncementRepository) GetAnnouncementsPaginated(ctx context.Context, active bool, limit int, cursor string) ([]*storage.Announcement, string, error) {
	args := m.Called(ctx, active, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announcement), args.String(1), args.Error(2)
}

// GetAnnouncementsByAdmin mocks the GetAnnouncementsByAdmin method
func (m *MockAnnouncementRepository) GetAnnouncementsByAdmin(ctx context.Context, adminUsername string, limit int, cursor string) ([]*storage.Announcement, string, error) {
	args := m.Called(ctx, adminUsername, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announcement), args.String(1), args.Error(2)
}

// UpdateAnnouncement mocks the UpdateAnnouncement method
func (m *MockAnnouncementRepository) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

// DeleteAnnouncement mocks the DeleteAnnouncement method
func (m *MockAnnouncementRepository) DeleteAnnouncement(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// DismissAnnouncement mocks the DismissAnnouncement method
func (m *MockAnnouncementRepository) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	args := m.Called(ctx, username, announcementID)
	return args.Error(0)
}

// IsDismissed mocks the IsDismissed method
func (m *MockAnnouncementRepository) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	args := m.Called(ctx, username, announcementID)
	return args.Bool(0), args.Error(1)
}

// GetDismissedAnnouncements mocks the GetDismissedAnnouncements method
func (m *MockAnnouncementRepository) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// AddAnnouncementReaction mocks the AddAnnouncementReaction method
func (m *MockAnnouncementRepository) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

// RemoveAnnouncementReaction mocks the RemoveAnnouncementReaction method
func (m *MockAnnouncementRepository) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

// GetAnnouncementReactions mocks the GetAnnouncementReactions method
func (m *MockAnnouncementRepository) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	args := m.Called(ctx, announcementID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]string), args.Error(1)
}

// Ensure MockAnnouncementRepository implements interfaces.AnnouncementRepository
var _ interfaces.AnnouncementRepository = (*MockAnnouncementRepository)(nil)
