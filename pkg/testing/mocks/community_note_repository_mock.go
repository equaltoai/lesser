// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockCommunityNoteRepository is a mock implementation of interfaces.CommunityNoteRepository
// using testify/mock for expectation-based testing.
type MockCommunityNoteRepository struct {
	mock.Mock
}

// NewMockCommunityNoteRepository creates a new mock community note repository
func NewMockCommunityNoteRepository() *MockCommunityNoteRepository {
	return &MockCommunityNoteRepository{}
}

// ===== Core Community Note Operations =====

// CreateCommunityNote mocks the CreateCommunityNote method
func (m *MockCommunityNoteRepository) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// GetCommunityNote mocks the GetCommunityNote method
func (m *MockCommunityNoteRepository) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CommunityNote), args.Error(1)
}

// GetVisibleCommunityNotes mocks the GetVisibleCommunityNotes method
func (m *MockCommunityNoteRepository) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNote), args.Error(1)
}

// GetCommunityNotesByAuthor mocks the GetCommunityNotesByAuthor method
func (m *MockCommunityNoteRepository) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CommunityNote), args.String(1), args.Error(2)
}

// ===== Score and Analysis Operations =====

// UpdateCommunityNoteScore mocks the UpdateCommunityNoteScore method
func (m *MockCommunityNoteRepository) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	args := m.Called(ctx, noteID, score, status)
	return args.Error(0)
}

// UpdateCommunityNoteAnalysis mocks the UpdateCommunityNoteAnalysis method
func (m *MockCommunityNoteRepository) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	args := m.Called(ctx, noteID, sentiment, objectivity, sourceQuality)
	return args.Error(0)
}

// ===== Voting Operations =====

// CreateCommunityNoteVote mocks the CreateCommunityNoteVote method
func (m *MockCommunityNoteRepository) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

// GetCommunityNoteVotes mocks the GetCommunityNoteVotes method
func (m *MockCommunityNoteRepository) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

// GetUserCommunityNoteVotes mocks the GetUserCommunityNoteVotes method
func (m *MockCommunityNoteRepository) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, noteIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.CommunityNoteVote), args.Error(1)
}

// GetUserVotingHistory mocks the GetUserVotingHistory method
func (m *MockCommunityNoteRepository) GetUserVotingHistory(ctx context.Context, userID string, limit int) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

// Ensure MockCommunityNoteRepository implements interfaces.CommunityNoteRepository
var _ interfaces.CommunityNoteRepository = (*MockCommunityNoteRepository)(nil)
