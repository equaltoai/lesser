package notes

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// Service provides methods for managing community notes
type Service struct {
	storage core.RepositoryStorage
	logger  *zap.Logger
}

// NewService creates a new notes service
func NewService(storage core.RepositoryStorage, logger *zap.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
	}
}

// CreateNote creates a new community note
func (s *Service) CreateNote(ctx context.Context, note *CommunityNote) error {
	note.ID = GenerateNoteID()
	note.CreatedAt = time.Now()
	note.UpdatedAt = time.Now()
	note.VisibilityStatus = VisibilityPending
	note.Score = 0

	return s.StoreNote(ctx, note)
}

// VoteOnNote records a vote on a note
func (s *Service) VoteOnNote(ctx context.Context, vote *Vote) error {
	return s.StoreVote(ctx, vote)
}

// RecalculateNoteScore recalculates a note's score based on current votes
func (s *Service) RecalculateNoteScore(ctx context.Context, noteID string) error {
	// Get the note
	note, err := s.GetNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Get all votes
	votes, err := s.GetVotesForNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Calculate new score
	newScore := CalculateNoteScore(note, votes)
	newStatus := DetermineVisibilityStatus(newScore)

	// Update the note
	return s.UpdateNoteScore(ctx, noteID, newScore, newStatus)
}

// CheckRateLimit checks if a user can create more notes
func (s *Service) CheckRateLimit(ctx context.Context, userID string, reputation float64) (bool, int) {
	limit := CalculateNoteLimit(reputation)
	return s.CheckNoteRateLimit(ctx, userID, limit)
}

// StoreNote stores a community note
func (s *Service) StoreNote(ctx context.Context, note *CommunityNote) error {
	// Convert to storage note
	storageNote := convertToStorageNote(note)

	// Create note using storage adapter
	err := s.storage.CommunityNote().CreateCommunityNote(ctx, storageNote)
	if err != nil {
		return fmt.Errorf("failed to store note: %w", err)
	}
	return nil
}

// GetNote retrieves a note by ID
func (s *Service) GetNote(ctx context.Context, noteID string) (*CommunityNote, error) {
	// Get note from storage
	storageNote, err := s.storage.CommunityNote().GetCommunityNote(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}
	// Convert to notes.CommunityNote
	return convertFromStorageNote(storageNote), nil
}

// GetVisibleNotes retrieves visible notes for an object
func (s *Service) GetVisibleNotes(ctx context.Context, objectID string) ([]CommunityNote, error) {
	// Get visible notes from storage
	storageNotes, err := s.storage.CommunityNote().GetVisibleCommunityNotes(ctx, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	// Convert to notes.CommunityNote
	notes := make([]CommunityNote, len(storageNotes))
	for i, storageNote := range storageNotes {
		notes[i] = *convertFromStorageNote(storageNote)
	}
	return notes, nil
}

// StoreVote stores a vote on a note
func (s *Service) StoreVote(ctx context.Context, vote *Vote) error {
	// Convert to storage vote
	storageVote := convertToStorageVote(vote)

	// Create vote using storage
	err := s.storage.CommunityNote().CreateCommunityNoteVote(ctx, storageVote)
	if err != nil {
		return fmt.Errorf("failed to store vote: %w", err)
	}
	return nil
}

// GetVotesForNote retrieves all votes for a note
func (s *Service) GetVotesForNote(ctx context.Context, noteID string) ([]Vote, error) {
	// Get votes from storage
	storageVotes, err := s.storage.CommunityNote().GetCommunityNoteVotes(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}
	// Convert to notes.Vote
	votes := make([]Vote, len(storageVotes))
	for i, storageVote := range storageVotes {
		votes[i] = convertFromStorageVote(storageVote)
	}
	return votes, nil
}

// GetUserVotes retrieves a user's votes on specific notes
func (s *Service) GetUserVotes(ctx context.Context, userID string, noteIDs []string) (map[string]Vote, error) {
	// Get all user votes for these notes at once
	storageVotes, err := s.storage.CommunityNote().GetUserCommunityNoteVotes(ctx, userID, noteIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get user votes: %w", err)
	}

	// Convert to notes.Vote
	userVotes := make(map[string]Vote)
	for noteID, vote := range storageVotes {
		if vote != nil {
			userVotes[noteID] = convertFromStorageVote(vote)
		}
	}
	return userVotes, nil
}

// GetNotesByAuthor retrieves notes created by a specific author
func (s *Service) GetNotesByAuthor(ctx context.Context, authorID string, limit int32) ([]CommunityNote, error) {
	// Get notes by author from storage
	storageNotes, _, err := s.storage.CommunityNote().GetCommunityNotesByAuthor(ctx, authorID, int(limit), "")
	if err != nil {
		return nil, fmt.Errorf("failed to query author notes: %w", err)
	}
	// Convert to notes.CommunityNote
	notes := make([]CommunityNote, len(storageNotes))
	for i, storageNote := range storageNotes {
		notes[i] = *convertFromStorageNote(storageNote)
	}
	return notes, nil
}

// UpdateNoteScore updates a note's score and visibility status
func (s *Service) UpdateNoteScore(ctx context.Context, noteID string, score float64, status VisibilityStatus) error {
	// Update note score and status using storage
	err := s.storage.CommunityNote().UpdateCommunityNoteScore(ctx, noteID, score, string(status))
	if err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}
	return nil
}

// CheckNoteRateLimit checks if a user can create more notes
func (s *Service) CheckNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int) {
	// Get user's note count in the last 24 hours
	// Since we don't have this method on CommunityNote repository, we'll use GetNotesByAuthor
	// Safe conversion with bounds check
	queryLimit := limit + 1
	var queryLimit32 int32
	if queryLimit > int(^int32(0)) {
		queryLimit32 = ^int32(0) // Max int32
	} else if queryLimit < 0 {
		queryLimit32 = 0
	} else {
		queryLimit32 = int32(queryLimit)
	}
	notes, err := s.GetNotesByAuthor(ctx, userID, queryLimit32)
	if err != nil {
		return false, 0
	}

	// Count notes from last 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	count := 0
	for _, note := range notes {
		if note.CreatedAt.After(cutoff) {
			count++
		}
	}

	// Check if under limit
	return count < limit, limit - count
}
