// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// CommunityNoteRepository defines the interface for community note operations.
// This handles community-contributed notes for content moderation and fact-checking.
type CommunityNoteRepository interface {
	// ===== Core Community Note Operations =====

	// CreateCommunityNote creates a new community note
	CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error

	// GetCommunityNote retrieves a note by ID
	GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error)

	// GetVisibleCommunityNotes retrieves visible notes for an object
	GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error)

	// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
	GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error)

	// ===== Score and Analysis Operations =====

	// UpdateCommunityNoteScore updates a note's score and visibility
	UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error

	// UpdateCommunityNoteAnalysis updates AI analysis results for a note
	UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error

	// ===== Voting Operations =====

	// CreateCommunityNoteVote creates a vote on a note
	CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error

	// GetCommunityNoteVotes retrieves votes on a specific community note
	GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error)

	// GetUserCommunityNoteVotes retrieves a user's votes on specific notes
	GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error)

	// GetUserVotingHistory retrieves a user's voting history for reputation calculation
	GetUserVotingHistory(ctx context.Context, userID string, limit int) ([]*storage.CommunityNoteVote, error)
}
