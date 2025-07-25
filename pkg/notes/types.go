package notes

import (
	"time"
)

// CommunityNote represents a fact-checking note on any ActivityPub object
type CommunityNote struct {
	// Identity
	ID         string `json:"id" dynamodbav:"ID"`
	ObjectID   string `json:"object_id" dynamodbav:"ObjectID"`
	ObjectType string `json:"object_type" dynamodbav:"ObjectType"`

	// Author
	AuthorID  string  `json:"author_id" dynamodbav:"AuthorID"`
	AuthorRep float64 `json:"author_reputation" dynamodbav:"AuthorRep"`

	// Content
	Content  string   `json:"content" dynamodbav:"Content"`
	Language string   `json:"language" dynamodbav:"Language"`
	Sources  []Source `json:"sources" dynamodbav:"Sources"`

	// Scoring
	HelpfulVotes     int              `json:"helpful_votes" dynamodbav:"HelpfulVotes"`
	NotHelpfulVotes  int              `json:"not_helpful_votes" dynamodbav:"NotHelpfulVotes"`
	Score            float64          `json:"score" dynamodbav:"Score"`
	VisibilityStatus VisibilityStatus `json:"visibility_status" dynamodbav:"VisibilityStatus"`

	// AI Analysis
	Sentiment     float64 `json:"sentiment" dynamodbav:"Sentiment"`
	Objectivity   float64 `json:"objectivity" dynamodbav:"Objectivity"`
	SourceQuality float64 `json:"source_quality" dynamodbav:"SourceQuality"`

	// Federation
	Federated   bool       `json:"federated" dynamodbav:"Federated"`
	FederatedAt *time.Time `json:"federated_at,omitempty" dynamodbav:"FederatedAt,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at" dynamodbav:"CreatedAt"`
	UpdatedAt time.Time `json:"updated_at" dynamodbav:"UpdatedAt"`
	TTL       int64     `json:"-" dynamodbav:"TTL,omitempty"`
}

// Source represents a reference supporting the note
type Source struct {
	URL         string  `json:"url" dynamodbav:"URL"`
	Title       string  `json:"title" dynamodbav:"Title"`
	Domain      string  `json:"domain" dynamodbav:"Domain"`
	Reliability float64 `json:"reliability" dynamodbav:"Reliability"`
}

// Vote represents a user's vote on a note
type Vote struct {
	NoteID    string    `json:"note_id" dynamodbav:"NoteID"`
	VoterID   string    `json:"voter_id" dynamodbav:"VoterID"`
	VoterRep  float64   `json:"voter_reputation" dynamodbav:"VoterRep"`
	VoteType  VoteType  `json:"vote_type" dynamodbav:"VoteType"`
	Reason    string    `json:"reason,omitempty" dynamodbav:"Reason,omitempty"`
	Weight    float64   `json:"weight" dynamodbav:"Weight"`
	CreatedAt time.Time `json:"created_at" dynamodbav:"CreatedAt"`
}

// VoteType represents the type of vote on a community note
type VoteType string

const (
	VoteHelpful    VoteType = "helpful"
	VoteNotHelpful VoteType = "not_helpful"
	VoteNeutral    VoteType = "neutral"
)

// VisibilityStatus represents the visibility state of a note
type VisibilityStatus string

const (
	VisibilityPending  VisibilityStatus = "pending"
	VisibilityVisible  VisibilityStatus = "visible"
	VisibilityHidden   VisibilityStatus = "hidden"
	VisibilityDisputed VisibilityStatus = "disputed"
)

// Request/Response types
type CreateNoteRequest struct {
	ObjectID   string   `json:"object_id" validate:"required"`
	ObjectType string   `json:"object_type" validate:"required"`
	Content    string   `json:"content" validate:"required,min=10,max=500"`
	Language   string   `json:"language" validate:"required,len=2"`
	Sources    []Source `json:"sources" validate:"max=5"`
}

type VoteRequest struct {
	VoteType VoteType `json:"vote_type" validate:"required,oneof=helpful not_helpful neutral"`
	Reason   string   `json:"reason" validate:"max=200"`
}

type NotesResponse struct {
	Notes []CommunityNote `json:"notes"`
	Stats map[string]any  `json:"stats"`
}

// Constants for thresholds and limits
const (
	// Reputation requirements
	MinReputationToCreateNotes = 100.0
	MinReputationToVote        = 10.0

	// Visibility thresholds
	VisibilityThreshold = 0.5  // Minimum score to be visible
	ProminentThreshold  = 0.75 // Score for prominent display
	DisputeThreshold    = 0.3  // Below this, mark as disputed
	FederationThreshold = 0.7  // Minimum score to federate

	// Federation requirements
	FederationMinRep = 500.0 // Minimum reputation to accept federated notes

	// Rate limits (notes per day based on reputation)
	BaseNoteLimit = 1
	MaxNoteLimit  = 10

	// Content limits
	MaxNoteLength   = 500
	MinNoteLength   = 10
	MaxSources      = 5
	MaxReasonLength = 200

	// TTL
	NoteTTLDays = 90
)

// Analysis represents AI analysis results
type Analysis struct {
	Sentiment   float64  `json:"sentiment"`
	Objectivity float64  `json:"objectivity"`
	HasPII      bool     `json:"has_pii"`
	Language    string   `json:"language"`
	Keywords    []string `json:"keywords"`
}
