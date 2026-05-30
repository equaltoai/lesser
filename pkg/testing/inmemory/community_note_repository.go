// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// CommunityNoteRepository is a thread-safe in-memory implementation of interfaces.CommunityNoteRepository.
type CommunityNoteRepository struct {
	mu sync.RWMutex
	// notes stores notes keyed by noteID
	notes map[string]*storage.CommunityNote
	// notesByObject stores note IDs keyed by objectID
	notesByObject map[string][]string
	// notesByAuthor stores note IDs keyed by authorID
	notesByAuthor map[string][]string
	// votes stores votes keyed by "noteID:userID"
	votes map[string]*storage.CommunityNoteVote
	// votesByNote stores vote keys keyed by noteID
	votesByNote map[string][]string
	// votesByUser stores vote keys keyed by userID
	votesByUser map[string][]string
}

// NewCommunityNoteRepository creates a new in-memory community note repository
func NewCommunityNoteRepository() *CommunityNoteRepository {
	return &CommunityNoteRepository{
		notes:         make(map[string]*storage.CommunityNote),
		notesByObject: make(map[string][]string),
		notesByAuthor: make(map[string][]string),
		votes:         make(map[string]*storage.CommunityNoteVote),
		votesByNote:   make(map[string][]string),
		votesByUser:   make(map[string][]string),
	}
}

// CreateCommunityNote creates a new community note
func (r *CommunityNoteRepository) CreateCommunityNote(_ context.Context, note *storage.CommunityNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.notes[note.ID]; exists {
		return storage.ErrAlreadyExists
	}
	r.notes[note.ID] = note
	r.notesByObject[note.ObjectID] = append(r.notesByObject[note.ObjectID], note.ID)
	r.notesByAuthor[note.AuthorID] = append(r.notesByAuthor[note.AuthorID], note.ID)
	return nil
}

// GetCommunityNote retrieves a note by ID
func (r *CommunityNoteRepository) GetCommunityNote(_ context.Context, noteID string) (*storage.CommunityNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	note, exists := r.notes[noteID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return note, nil
}

// GetVisibleCommunityNotes retrieves visible notes for an object
func (r *CommunityNoteRepository) GetVisibleCommunityNotes(_ context.Context, objectID string) ([]*storage.CommunityNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	noteIDs := r.notesByObject[objectID]
	var results []*storage.CommunityNote
	for _, noteID := range noteIDs {
		if note, exists := r.notes[noteID]; exists {
			if note.Status == "visible" || note.Status == "" {
				results = append(results, note)
			}
		}
	}
	return results, nil
}

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
func (r *CommunityNoteRepository) GetCommunityNotesByAuthor(_ context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ordered := make([]*storage.CommunityNote, 0, len(r.notesByAuthor[authorID]))
	for _, noteID := range r.notesByAuthor[authorID] {
		if note, exists := r.notes[noteID]; exists && note != nil {
			ordered = append(ordered, note)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return inMemoryCommunityNoteAuthorCursor(ordered[i]) > inMemoryCommunityNoteAuthorCursor(ordered[j])
	})

	results := make([]*storage.CommunityNote, 0, len(ordered))
	for _, note := range ordered {
		noteCursor := inMemoryCommunityNoteAuthorCursor(note)
		if cursor != "" && noteCursor >= cursor {
			continue
		}
		results = append(results, note)
		if limit > 0 && len(results) == limit {
			break
		}
	}

	nextCursor := ""
	if limit > 0 && len(results) == limit {
		nextCursor = inMemoryCommunityNoteAuthorCursor(results[len(results)-1])
	}
	return results, nextCursor, nil
}

func inMemoryCommunityNoteAuthorCursor(note *storage.CommunityNote) string {
	if note == nil {
		return ""
	}
	return note.CreatedAt.Format(time.RFC3339) + "#" + note.ID
}

// UpdateCommunityNoteScore updates a note's score and visibility
func (r *CommunityNoteRepository) UpdateCommunityNoteScore(_ context.Context, noteID string, score float64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	note, exists := r.notes[noteID]
	if !exists {
		return storage.ErrNotFound
	}
	note.Score = score
	note.Status = status
	return nil
}

// UpdateCommunityNoteAnalysis updates AI analysis results for a note
func (r *CommunityNoteRepository) UpdateCommunityNoteAnalysis(_ context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	note, exists := r.notes[noteID]
	if !exists {
		return storage.ErrNotFound
	}
	note.Sentiment = sentiment
	note.Objectivity = objectivity
	note.SourceQuality = sourceQuality
	return nil
}

// CreateCommunityNoteVote creates a vote on a note
func (r *CommunityNoteRepository) CreateCommunityNoteVote(_ context.Context, vote *storage.CommunityNoteVote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := vote.NoteID + ":" + vote.VoterID
	if _, exists := r.votes[key]; exists {
		return storage.ErrAlreadyExists
	}
	r.votes[key] = vote
	r.votesByNote[vote.NoteID] = append(r.votesByNote[vote.NoteID], key)
	r.votesByUser[vote.VoterID] = append(r.votesByUser[vote.VoterID], key)
	return nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note
func (r *CommunityNoteRepository) GetCommunityNoteVotes(_ context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	voteKeys := r.votesByNote[noteID]
	var results []*storage.CommunityNoteVote
	for _, key := range voteKeys {
		if vote, exists := r.votes[key]; exists {
			results = append(results, vote)
		}
	}
	return results, nil
}

// GetUserCommunityNoteVotes retrieves a user's votes on specific notes
func (r *CommunityNoteRepository) GetUserCommunityNoteVotes(_ context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*storage.CommunityNoteVote)
	for _, noteID := range noteIDs {
		key := noteID + ":" + userID
		if vote, exists := r.votes[key]; exists {
			result[noteID] = vote
		}
	}
	return result, nil
}

// GetUserVotingHistory retrieves a user's voting history for reputation calculation
func (r *CommunityNoteRepository) GetUserVotingHistory(_ context.Context, userID string, limit int) ([]*storage.CommunityNoteVote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	voteKeys := r.votesByUser[userID]
	var results []*storage.CommunityNoteVote
	for _, key := range voteKeys {
		if vote, exists := r.votes[key]; exists {
			results = append(results, vote)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// Clear clears all data (test helper)
func (r *CommunityNoteRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes = make(map[string]*storage.CommunityNote)
	r.notesByObject = make(map[string][]string)
	r.notesByAuthor = make(map[string][]string)
	r.votes = make(map[string]*storage.CommunityNoteVote)
	r.votesByNote = make(map[string][]string)
	r.votesByUser = make(map[string][]string)
}

// Ensure CommunityNoteRepository implements interfaces.CommunityNoteRepository
var _ interfaces.CommunityNoteRepository = (*CommunityNoteRepository)(nil)
