// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockThreadRepository is a mock implementation of interfaces.ThreadRepository
// using testify/mock for expectation-based testing.
type MockThreadRepository struct {
	mock.Mock
}

// NewMockThreadRepository creates a new mock thread repository
func NewMockThreadRepository() *MockThreadRepository {
	return &MockThreadRepository{}
}

// ===== Thread Sync Operations =====

// SaveThreadSync mocks the SaveThreadSync method
func (m *MockThreadRepository) SaveThreadSync(ctx context.Context, sync *models.ThreadSync) error {
	args := m.Called(ctx, sync)
	return args.Error(0)
}

// GetThreadSync mocks the GetThreadSync method
func (m *MockThreadRepository) GetThreadSync(ctx context.Context, statusID string) (*models.ThreadSync, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadSync), args.Error(1)
}

// ===== Thread Node Operations =====

// SaveThreadNode mocks the SaveThreadNode method
func (m *MockThreadRepository) SaveThreadNode(ctx context.Context, node *models.ThreadNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

// GetThreadNodes mocks the GetThreadNodes method
func (m *MockThreadRepository) GetThreadNodes(ctx context.Context, rootStatusID string) ([]*models.ThreadNode, error) {
	args := m.Called(ctx, rootStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ThreadNode), args.Error(1)
}

// GetThreadNode mocks the GetThreadNode method
func (m *MockThreadRepository) GetThreadNode(ctx context.Context, rootStatusID, statusID string) (*models.ThreadNode, error) {
	args := m.Called(ctx, rootStatusID, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadNode), args.Error(1)
}

// GetThreadNodeByStatusID mocks the GetThreadNodeByStatusID method
func (m *MockThreadRepository) GetThreadNodeByStatusID(ctx context.Context, statusID string) (*models.ThreadNode, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadNode), args.Error(1)
}

// BulkSaveThreadNodes mocks the BulkSaveThreadNodes method
func (m *MockThreadRepository) BulkSaveThreadNodes(ctx context.Context, nodes []*models.ThreadNode) error {
	args := m.Called(ctx, nodes)
	return args.Error(0)
}

// ===== Missing Reply Operations =====

// MarkMissingReplies mocks the MarkMissingReplies method
func (m *MockThreadRepository) MarkMissingReplies(ctx context.Context, rootStatusID, parentStatusID string, replyIDs []string) error {
	args := m.Called(ctx, rootStatusID, parentStatusID, replyIDs)
	return args.Error(0)
}

// GetMissingReplies mocks the GetMissingReplies method
func (m *MockThreadRepository) GetMissingReplies(ctx context.Context, rootStatusID string) ([]*models.MissingReply, error) {
	args := m.Called(ctx, rootStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MissingReply), args.Error(1)
}

// SaveMissingReply mocks the SaveMissingReply method
func (m *MockThreadRepository) SaveMissingReply(ctx context.Context, missing *models.MissingReply) error {
	args := m.Called(ctx, missing)
	return args.Error(0)
}

// DeleteMissingReply mocks the DeleteMissingReply method
func (m *MockThreadRepository) DeleteMissingReply(ctx context.Context, rootStatusID, replyID string) error {
	args := m.Called(ctx, rootStatusID, replyID)
	return args.Error(0)
}

// GetPendingMissingReplies mocks the GetPendingMissingReplies method
func (m *MockThreadRepository) GetPendingMissingReplies(ctx context.Context, limit int) ([]*models.MissingReply, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MissingReply), args.Error(1)
}

// ===== Thread Context Operations =====

// GetThreadContext mocks the GetThreadContext method
func (m *MockThreadRepository) GetThreadContext(ctx context.Context, statusID string) (*interfaces.ThreadContextResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.ThreadContextResult), args.Error(1)
}

// Ensure MockThreadRepository implements interfaces.ThreadRepository
var _ interfaces.ThreadRepository = (*MockThreadRepository)(nil)
