package lift

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
)

// MockPollRepository is a mock implementation of PollRepository for testing
type MockPollRepository struct {
	mock.Mock
}

// GetPoll retrieves a poll by ID
func (m *MockPollRepository) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// CreatePoll creates a new poll
func (m *MockPollRepository) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

// UpdatePoll updates an existing poll
func (m *MockPollRepository) UpdatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

// DeletePoll deletes a poll
func (m *MockPollRepository) DeletePoll(ctx context.Context, pollID string) error {
	args := m.Called(ctx, pollID)
	return args.Error(0)
}

// VoteOnPoll records a vote on a poll
func (m *MockPollRepository) VoteOnPoll(ctx context.Context, pollID string, username string, choices []int) error {
	args := m.Called(ctx, pollID, username, choices)
	return args.Error(0)
}

// GetPollVote retrieves a user's vote on a poll
func (m *MockPollRepository) GetPollVote(ctx context.Context, pollID string, username string) ([]int, error) {
	args := m.Called(ctx, pollID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int), args.Error(1)
}

// HasVoted checks if a user has voted on a poll
func (m *MockPollRepository) HasVoted(ctx context.Context, pollID string, username string) (bool, error) {
	args := m.Called(ctx, pollID, username)
	return args.Bool(0), args.Error(1)
}

// GetStatusPoll retrieves a poll associated with a status
func (m *MockPollRepository) GetStatusPoll(ctx context.Context, statusID string) (*storage.Poll, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// UpdatePollCounts updates the vote counts for a poll
func (m *MockPollRepository) UpdatePollCounts(ctx context.Context, pollID string, voteCounts map[string]int) error {
	args := m.Called(ctx, pollID, voteCounts)
	return args.Error(0)
}

// GetExpiredPolls retrieves polls that have expired
// func (m *MockPollRepository) GetExpiredPolls(ctx context.Context, limit int) ([]*storage.Poll, error) {
// 	args := m.Called(ctx, limit)
// 	if args.Get(0) == nil {
// 		return nil, args.Error(1)
// 	}
// 	return args.Get(0).([]*storage.Poll), args.Error(1)
// }
// 
// // ClosePoll marks a poll as closed
// func (m *MockPollRepository) ClosePoll(ctx context.Context, pollID string) error {
// 	args := m.Called(ctx, pollID)
// 	return args.Error(0)
// }
