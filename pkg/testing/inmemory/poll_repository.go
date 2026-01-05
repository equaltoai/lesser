// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/google/uuid"
)

// PollRepository is a thread-safe in-memory implementation of interfaces.PollRepository.
type PollRepository struct {
	mu sync.RWMutex

	// Polls: key = pollID
	polls map[string]*storage.Poll

	// Index by status: statusID -> pollID
	byStatus map[string]string

	// Votes: key = "pollID:voterID" -> []int (choices)
	votes map[string][]int
}

// NewPollRepository creates a new in-memory poll repository
func NewPollRepository() *PollRepository {
	return &PollRepository{
		polls:    make(map[string]*storage.Poll),
		byStatus: make(map[string]string),
		votes:    make(map[string][]int),
	}
}

// voteKey generates a unique key for a vote
func voteKey(pollID, voterID string) string {
	return fmt.Sprintf("%s:%s", pollID, voterID)
}

// CreatePoll creates a new poll
func (r *PollRepository) CreatePoll(_ context.Context, poll *storage.Poll) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if poll == nil {
		return fmt.Errorf("poll cannot be nil")
	}

	if poll.ID == "" {
		poll.ID = uuid.New().String()
	}

	if _, exists := r.polls[poll.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	poll.CreatedAt = now
	poll.UpdatedAt = now

	// Initialize vote counts if not set
	if poll.VotesCount == nil {
		poll.VotesCount = make([]int, len(poll.Options))
	}

	r.polls[poll.ID] = poll
	if poll.StatusID != "" {
		r.byStatus[poll.StatusID] = poll.ID
	}

	return nil
}

// GetPoll retrieves a poll by ID
func (r *PollRepository) GetPoll(_ context.Context, pollID string) (*storage.Poll, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	poll, exists := r.polls[pollID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Check if expired
	if poll.ExpiresAt != nil && poll.ExpiresAt.Before(time.Now()) {
		poll.Expired = true
	}

	return poll, nil
}

// GetPollByStatusID retrieves a poll by status ID
func (r *PollRepository) GetPollByStatusID(_ context.Context, statusID string) (*storage.Poll, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pollID, exists := r.byStatus[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	poll, exists := r.polls[pollID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Check if expired
	if poll.ExpiresAt != nil && poll.ExpiresAt.Before(time.Now()) {
		poll.Expired = true
	}

	return poll, nil
}

// VoteOnPoll records a vote on a poll
func (r *PollRepository) VoteOnPoll(_ context.Context, pollID string, voterID string, choices []int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	poll, exists := r.polls[pollID]
	if !exists {
		return storage.ErrNotFound
	}

	// Check if poll is expired
	if poll.ExpiresAt != nil && poll.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("poll has expired")
	}

	key := voteKey(pollID, voterID)

	// Check if already voted
	if _, voted := r.votes[key]; voted {
		return fmt.Errorf("user has already voted")
	}

	// Validate choices
	for _, choice := range choices {
		if choice < 0 || choice >= len(poll.Options) {
			return fmt.Errorf("invalid choice: %d", choice)
		}
	}

	// Check multiple choice
	if !poll.Multiple && len(choices) > 1 {
		return fmt.Errorf("poll does not allow multiple choices")
	}

	// Record vote
	r.votes[key] = choices

	// Update vote counts
	for _, choice := range choices {
		if poll.VotesCount == nil {
			poll.VotesCount = make([]int, len(poll.Options))
		}
		poll.VotesCount[choice]++
	}
	poll.VotersCount++
	poll.UpdatedAt = time.Now()

	return nil
}

// GetPollVotes retrieves all votes for a poll
func (r *PollRepository) GetPollVotes(_ context.Context, pollID string) (map[string][]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.polls[pollID]; !exists {
		return nil, storage.ErrNotFound
	}

	result := make(map[string][]int)
	prefix := pollID + ":"

	for key, choices := range r.votes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			voterID := key[len(prefix):]
			result[voterID] = choices
		}
	}

	return result, nil
}

// HasUserVoted checks if a user has voted on a poll
func (r *PollRepository) HasUserVoted(_ context.Context, pollID string, userID string) (bool, []int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.polls[pollID]; !exists {
		return false, nil, storage.ErrNotFound
	}

	key := voteKey(pollID, userID)
	choices, voted := r.votes[key]

	return voted, choices, nil
}

// Test helper methods

// Clear clears all data (test helper)
func (r *PollRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.polls = make(map[string]*storage.Poll)
	r.byStatus = make(map[string]string)
	r.votes = make(map[string][]int)
}

// GetPollCount returns the number of polls (test helper)
func (r *PollRepository) GetPollCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.polls)
}

// GetVoteCount returns the total number of votes (test helper)
func (r *PollRepository) GetVoteCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.votes)
}

// Ensure PollRepository implements interfaces.PollRepository
var _ interfaces.PollRepository = (*PollRepository)(nil)
