package notes

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
)

var (
	storageAdapter storage.Storage
)

// SetStorageAdapter allows external packages to set the storage adapter
func SetStorageAdapter(adapter storage.Storage) {
	storageAdapter = adapter
}

// convertToStorageNote converts notes.CommunityNote to storage.CommunityNote
func convertToStorageNote(note *CommunityNote) *storage.CommunityNote {
	// Convert sources from []Source to []string
	sources := make([]string, len(note.Sources))
	for i, source := range note.Sources {
		sources[i] = source.URL // Use URL as the simple string representation
	}

	return &storage.CommunityNote{
		ID:               note.ID,
		ObjectID:         note.ObjectID,
		ObjectType:       note.ObjectType,
		AuthorID:         note.AuthorID,
		Content:          note.Content,
		Language:         note.Language,
		Sources:          sources,
		HelpfulVotes:     note.HelpfulVotes,
		NotHelpfulVotes:  note.NotHelpfulVotes,
		Score:            note.Score,
		VisibilityStatus: string(note.VisibilityStatus),
		Sentiment:        note.Sentiment,
		Objectivity:      note.Objectivity,
		SourceQuality:    note.SourceQuality,
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
	}
}

// convertFromStorageNote converts storage.CommunityNote to notes.CommunityNote
func convertFromStorageNote(storageNote *storage.CommunityNote) *CommunityNote {
	// Convert sources from []string to []Source (simplified conversion)
	sources := make([]Source, len(storageNote.Sources))
	for i, sourceURL := range storageNote.Sources {
		sources[i] = Source{
			URL:         sourceURL,
			Title:       "", // Not stored in simplified format
			Domain:      "", // Not stored in simplified format
			Reliability: 0,  // Not stored in simplified format
		}
	}

	return &CommunityNote{
		ID:               storageNote.ID,
		ObjectID:         storageNote.ObjectID,
		ObjectType:       storageNote.ObjectType,
		AuthorID:         storageNote.AuthorID,
		Content:          storageNote.Content,
		Language:         storageNote.Language,
		Sources:          sources,
		HelpfulVotes:     storageNote.HelpfulVotes,
		NotHelpfulVotes:  storageNote.NotHelpfulVotes,
		Score:            storageNote.Score,
		VisibilityStatus: VisibilityStatus(storageNote.VisibilityStatus),
		Sentiment:        storageNote.Sentiment,
		Objectivity:      storageNote.Objectivity,
		SourceQuality:    storageNote.SourceQuality,
		CreatedAt:        storageNote.CreatedAt,
		UpdatedAt:        storageNote.UpdatedAt,
	}
}

// convertToStorageVote converts notes.Vote to storage.CommunityNoteVote
func convertToStorageVote(vote *Vote) *storage.CommunityNoteVote {
	return &storage.CommunityNoteVote{
		NoteID:    vote.NoteID,
		VoterID:   vote.VoterID,
		VoteType:  string(vote.VoteType),
		Helpful:   vote.VoteType == VoteHelpful,
		Weight:    vote.Weight,
		CreatedAt: vote.CreatedAt,
	}
}

// convertFromStorageVote converts storage.CommunityNoteVote to notes.Vote
func convertFromStorageVote(storageVote *storage.CommunityNoteVote) Vote {
	return Vote{
		NoteID:    storageVote.NoteID,
		VoterID:   storageVote.VoterID,
		VoterRep:  0, // Not stored in storage layer
		VoteType:  VoteType(storageVote.VoteType),
		Reason:    "", // Not stored in storage layer  
		Weight:    storageVote.Weight,
		CreatedAt: storageVote.CreatedAt,
	}
}

// StoreNote stores a community note using the storage adapter
func StoreNote(ctx context.Context, note *CommunityNote) error {
	if storageAdapter == nil {
		return fmt.Errorf("storage adapter not initialized")
	}

	// Generate ID if not provided
	if note.ID == "" {
		note.ID = GenerateNoteID()
	}

	// Convert to storage note
	storageNote := convertToStorageNote(note)

	// Create note using storage adapter
	err := storageAdapter.CreateCommunityNote(ctx, storageNote)
	if err != nil {
		return fmt.Errorf("failed to store note: %w", err)
	}

	// Update the original note with any changes made by storage layer
	note.CreatedAt = storageNote.CreatedAt
	note.UpdatedAt = storageNote.UpdatedAt

	return nil
}

// GetNote retrieves a note by ID using the storage adapter
func GetNote(ctx context.Context, noteID string) (*CommunityNote, error) {
	if storageAdapter == nil {
		return nil, fmt.Errorf("storage adapter not initialized")
	}

	// Get note from storage adapter
	storageNote, err := storageAdapter.GetCommunityNote(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	// Convert to notes type
	note := convertFromStorageNote(storageNote)
	return note, nil
}

// GetVisibleNotes retrieves visible notes for an object using the storage adapter
func GetVisibleNotes(ctx context.Context, objectID string) ([]CommunityNote, error) {
	if storageAdapter == nil {
		return nil, fmt.Errorf("storage adapter not initialized")
	}

	// Get visible notes from storage adapter
	storageNotes, err := storageAdapter.GetVisibleCommunityNotes(ctx, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}

	// Convert to notes type
	notes := make([]CommunityNote, 0, len(storageNotes))
	for _, storageNote := range storageNotes {
		note := convertFromStorageNote(storageNote)
		notes = append(notes, *note)
	}

	return notes, nil
}

// StoreVote stores a vote on a note using the storage adapter
func StoreVote(ctx context.Context, vote *Vote) error {
	if storageAdapter == nil {
		return fmt.Errorf("storage adapter not initialized")
	}

	// Convert to storage vote
	storageVote := convertToStorageVote(vote)

	// Create vote using storage adapter
	err := storageAdapter.CreateCommunityNoteVote(ctx, storageVote)
	if err != nil {
		return fmt.Errorf("failed to store vote: %w", err)
	}

	return nil
}

// GetVotesForNote retrieves all votes for a note using the storage adapter
func GetVotesForNote(ctx context.Context, noteID string) ([]Vote, error) {
	if storageAdapter == nil {
		return nil, fmt.Errorf("storage adapter not initialized")
	}

	// Get votes from storage adapter
	storageVotes, err := storageAdapter.GetCommunityNoteVotes(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}

	// Convert to notes type
	votes := make([]Vote, 0, len(storageVotes))
	for _, storageVote := range storageVotes {
		vote := convertFromStorageVote(storageVote)
		votes = append(votes, vote)
	}

	return votes, nil
}

// GetUserVotes retrieves a user's votes on specific notes using the storage adapter
func GetUserVotes(ctx context.Context, userID string, noteIDs []string) (map[string]Vote, error) {
	if storageAdapter == nil {
		return nil, fmt.Errorf("storage adapter not initialized")
	}

	// Get votes from storage adapter
	storageVotes, err := storageAdapter.GetUserCommunityNoteVotes(ctx, userID, noteIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get user votes: %w", err)
	}

	// Convert to notes type
	votes := make(map[string]Vote)
	for noteID, storageVote := range storageVotes {
		vote := convertFromStorageVote(storageVote)
		votes[noteID] = vote
	}

	return votes, nil
}

// UpdateNoteScore updates a note's score and visibility using the storage adapter
func UpdateNoteScore(ctx context.Context, noteID string, score float64, status VisibilityStatus) error {
	if storageAdapter == nil {
		return fmt.Errorf("storage adapter not initialized")
	}

	// Update score using storage adapter
	err := storageAdapter.UpdateCommunityNoteScore(ctx, noteID, score, string(status))
	if err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	return nil
}

// UpdateNoteAnalysis updates AI analysis results using the storage adapter
func UpdateNoteAnalysis(ctx context.Context, noteID string, analysis *Analysis, sourceQuality float64) error {
	if storageAdapter == nil {
		return fmt.Errorf("storage adapter not initialized")
	}

	// Update analysis using storage adapter
	err := storageAdapter.UpdateCommunityNoteAnalysis(ctx, noteID, analysis.Sentiment, analysis.Objectivity, sourceQuality)
	if err != nil {
		return fmt.Errorf("failed to update note analysis: %w", err)
	}

	return nil
}

// GetNotesByAuthor retrieves notes created by a specific author using the storage adapter
func GetNotesByAuthor(ctx context.Context, authorID string, limit int32) ([]CommunityNote, error) {
	if storageAdapter == nil {
		return nil, fmt.Errorf("storage adapter not initialized")
	}

	// Get notes by author from storage adapter
	storageNotes, _, err := storageAdapter.GetCommunityNotesByAuthor(ctx, authorID, int(limit), "")
	if err != nil {
		return nil, fmt.Errorf("failed to query author notes: %w", err)
	}

	// Convert to notes type
	notes := make([]CommunityNote, 0, len(storageNotes))
	for _, storageNote := range storageNotes {
		note := convertFromStorageNote(storageNote)
		notes = append(notes, *note)
	}

	return notes, nil
}

// CheckNoteRateLimit checks if a user can create more notes today using the storage adapter
func CheckNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int) {
	if storageAdapter == nil {
		// If storage not initialized, allow creation
		return true, limit
	}

	// Check rate limit using storage adapter
	canCreate, remaining, err := storageAdapter.CheckCommunityNoteRateLimit(ctx, userID, limit)
	if err != nil {
		// If error checking, allow creation
		return true, limit
	}

	return canCreate, remaining
}

// GenerateNoteID generates a unique ID for a note
func GenerateNoteID() string {
	return uuid.New().String()
}
