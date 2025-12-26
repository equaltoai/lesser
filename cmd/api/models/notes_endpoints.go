package models

import (
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
)

// CommunityNoteSource represents a note source entry (request-side).
type CommunityNoteSource struct {
	URL string `json:"url"`
}

// CreateCommunityNoteRequest represents POST /api/v1/notes.
type CreateCommunityNoteRequest struct {
	ObjectID   string              `json:"object_id"`
	ObjectType string              `json:"object_type"`
	Content    string              `json:"content"`
	Language   string              `json:"language"`
	Sources    []CommunityNoteSource `json:"sources,omitempty"`
}

// VoteCommunityNoteRequest represents POST /api/v1/notes/{id}/vote.
type VoteCommunityNoteRequest struct {
	VoteType string `json:"vote_type"`
	Reason   string `json:"reason,omitempty"`
}

// CommunityNoteRateLimit represents rate limit details for note creation.
type CommunityNoteRateLimit struct {
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	Reset     string `json:"reset"`
}

// CreateCommunityNoteResponse represents the response from POST /api/v1/notes.
type CreateCommunityNoteResponse struct {
	Note      *storage.CommunityNote   `json:"note"`
	RateLimit CommunityNoteRateLimit `json:"rate_limit"`
}

// VoteCommunityNoteResponse represents the response from POST /api/v1/notes/{id}/vote.
type VoteCommunityNoteResponse struct {
	Vote   *storage.CommunityNoteVote `json:"vote"`
	NoteID string                     `json:"note_id"`
}

// CommunityNoteSummary represents a minimal note payload returned by GET /api/v1/notes/{object_id}.
type CommunityNoteSummary struct {
	ID               string    `json:"id"`
	ObjectID         string    `json:"object_id"`
	AuthorID         string    `json:"author_id"`
	Content          string    `json:"content"`
	Language         string    `json:"language"`
	Sources          []string  `json:"sources"`
	HelpfulVotes     int       `json:"helpful_votes"`
	NotHelpfulVotes  int       `json:"not_helpful_votes"`
	Score            float64   `json:"score"`
	VisibilityStatus string    `json:"visibility_status"`
	CreatedAt        time.Time `json:"created_at"`
}

// CommunityNoteStats represents aggregate stats for a note listing.
type CommunityNoteStats struct {
	Total          int     `json:"total"`
	Visible        int     `json:"visible"`
	AverageScore   float64 `json:"average_score"`
	AverageHelpful float64 `json:"average_helpful"`
}

// CommunityNotesResponse represents the response from GET /api/v1/notes/{object_id}.
type CommunityNotesResponse struct {
	Notes []CommunityNoteSummary `json:"notes"`
	Stats CommunityNoteStats     `json:"stats"`
}

// UserNoteAccount represents a minimal account payload for user note statuses.
type UserNoteAccount struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
}

// UserNoteCard represents note metadata embedded in the user notes response.
type UserNoteCard struct {
	Type       string   `json:"type"`
	ObjectID   string   `json:"object_id"`
	ObjectType string   `json:"object_type"`
	Score      float64  `json:"score"`
	Sources    []string `json:"sources,omitempty"`
}

// UserNoteStatus represents a status-like response entry for GET /api/v1/accounts/{id}/notes.
type UserNoteStatus struct {
	ID               string         `json:"id"`
	Content          string         `json:"content"`
	CreatedAt        string         `json:"created_at"`
	Account          UserNoteAccount `json:"account"`
	Visibility       string         `json:"visibility"`
	Sensitive        bool           `json:"sensitive"`
	SpoilerText      string         `json:"spoiler_text"`
	MediaAttachments []any          `json:"media_attachments"`
	Mentions         []any          `json:"mentions"`
	Tags             []any          `json:"tags"`
	Emojis           []any          `json:"emojis"`
	ReblogsCount     int            `json:"reblogs_count"`
	FavouritesCount  int            `json:"favourites_count"`
	RepliesCount     int            `json:"replies_count"`
	URL              string         `json:"url"`
	Card             UserNoteCard   `json:"card"`
}

