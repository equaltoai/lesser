package models

import (
	"fmt"
	"time"
)

// CommunityNote represents a fact-checking note on an ActivityPub object
type CommunityNote struct {
	// Keys
	PK string `dynamorm:"pk"` // NOTE#<id>
	SK string `dynamorm:"sk"` // METADATA

	// GSI fields for querying
	GSI1PK string `dynamorm:"index:gsi1,pk"` // OBJECT#<object_id>#NOTES
	GSI1SK string `dynamorm:"index:gsi1,sk"` // SCORE#<score>#<id>
	GSI2PK string `dynamorm:"index:gsi2,pk"` // NOTES#<visibility_status>
	GSI2SK string `dynamorm:"index:gsi2,sk"` // <created_at>#<id>
	GSI3PK string `dynamorm:"index:gsi3,pk"` // AUTHOR#<author_id>#NOTES
	GSI3SK string `dynamorm:"index:gsi3,sk"` // <created_at>#<id>

	// Core fields matching storage.CommunityNote
	ID               string    `json:"id"`
	ObjectID         string    `json:"object_id"`
	ObjectType       string    `json:"object_type"`
	AuthorID         string    `json:"author_id"`
	Content          string    `json:"content"`
	Language         string    `json:"language"`
	Sources          []string  `json:"sources"`
	HelpfulVotes     int       `json:"helpful_votes"`
	NotHelpfulVotes  int       `json:"not_helpful_votes"`
	Score            float64   `json:"score"`
	VisibilityStatus string    `json:"visibility_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// TTL field
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the GSI keys based on the current field values
func (n *CommunityNote) UpdateKeys() {
	n.PK = fmt.Sprintf("NOTE#%s", n.ID)
	n.SK = "METADATA"

	// GSI1: Query by object ID, sorted by score
	n.GSI1PK = fmt.Sprintf("OBJECT#%s#NOTES", n.ObjectID)
	n.GSI1SK = fmt.Sprintf("SCORE#%010.6f#%s", n.Score, n.ID)

	// GSI2: Query by visibility status, sorted by creation time
	n.GSI2PK = fmt.Sprintf("NOTES#%s", n.VisibilityStatus)
	n.GSI2SK = fmt.Sprintf("%s#%s", n.CreatedAt.Format(time.RFC3339), n.ID)

	// GSI3: Query by author, sorted by creation time
	n.GSI3PK = fmt.Sprintf("AUTHOR#%s#NOTES", n.AuthorID)
	n.GSI3SK = fmt.Sprintf("%s#%s", n.CreatedAt.Format(time.RFC3339), n.ID)
}

// CommunityNoteVote represents a vote on a community note
type CommunityNoteVote struct {
	// Keys
	PK string `dynamorm:"pk"` // NOTE#<note_id>
	SK string `dynamorm:"sk"` // VOTE#<voter_id>

	// Core fields matching storage.CommunityNoteVote
	NoteID    string    `json:"note_id"`
	VoterID   string    `json:"voter_id"`
	VoteType  string    `json:"vote_type"` // helpful, not_helpful, neutral
	Helpful   bool      `json:"helpful"`   // For simplified access
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"created_at"`

	// TTL field
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the keys based on the current field values
func (v *CommunityNoteVote) UpdateKeys() {
	v.PK = fmt.Sprintf("NOTE#%s", v.NoteID)
	v.SK = fmt.Sprintf("VOTE#%s", v.VoterID)
}