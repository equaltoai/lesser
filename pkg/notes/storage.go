package notes

import (
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
)

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
	for i, url := range storageNote.Sources {
		sources[i] = Source{
			URL:         url,
			Title:       "",        // Title not stored in storage format
			Domain:      "",        // Domain not stored in storage format
			Reliability: 0.5,       // Default reliability
		}
	}

	// Convert visibility status
	status := VisibilityPending // Default
	switch storageNote.VisibilityStatus {
	case "visible":
		status = VisibilityVisible
	case "hidden":
		status = VisibilityHidden
	case "disputed":
		status = VisibilityDisputed
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
		VisibilityStatus: status,
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
		Weight:    vote.Weight,
		CreatedAt: vote.CreatedAt,
	}
}

// convertFromStorageVote converts storage.CommunityNoteVote to notes.Vote
func convertFromStorageVote(storageVote *storage.CommunityNoteVote) Vote {
	voteType := VoteHelpful // Default
	if storageVote.VoteType == "not_helpful" {
		voteType = VoteNotHelpful
	}

	return Vote{
		NoteID:    storageVote.NoteID,
		VoterID:   storageVote.VoterID,
		VoteType:  voteType,
		Reason:    "", // Not stored in storage layer  
		Weight:    storageVote.Weight,
		CreatedAt: storageVote.CreatedAt,
	}
}

// GenerateNoteID generates a unique ID for a note
func GenerateNoteID() string {
	return "CN-" + uuid.New().String()
}