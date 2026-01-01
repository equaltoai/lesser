// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// PollRepository defines the interface for poll operations.
// This handles poll creation, voting, and results retrieval.
type PollRepository interface {
	// Core poll operations
	CreatePoll(ctx context.Context, poll *storage.Poll) error
	GetPoll(ctx context.Context, pollID string) (*storage.Poll, error)
	GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error)

	// Voting operations
	VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error
	GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error)
	HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error)
}
