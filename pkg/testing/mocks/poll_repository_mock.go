// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockPollRepository is a mock implementation of interfaces.PollRepository
// using testify/mock for expectation-based testing.
type MockPollRepository struct {
	mock.Mock
}

// NewMockPollRepository creates a new mock poll repository
func NewMockPollRepository() *MockPollRepository {
	return &MockPollRepository{}
}

// CreatePoll mocks the CreatePoll method
func (m *MockPollRepository) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

// GetPoll mocks the GetPoll method
func (m *MockPollRepository) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// GetPollByStatusID mocks the GetPollByStatusID method
func (m *MockPollRepository) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// VoteOnPoll mocks the VoteOnPoll method
func (m *MockPollRepository) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	args := m.Called(ctx, pollID, voterID, choices)
	return args.Error(0)
}

// GetPollVotes mocks the GetPollVotes method
func (m *MockPollRepository) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]int), args.Error(1)
}

// HasUserVoted mocks the HasUserVoted method
func (m *MockPollRepository) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	args := m.Called(ctx, pollID, userID)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).([]int), args.Error(2)
}

// Ensure MockPollRepository implements interfaces.PollRepository
var _ interfaces.PollRepository = (*MockPollRepository)(nil)
