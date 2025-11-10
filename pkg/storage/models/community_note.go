package models

import (
	"fmt"
	"time"
)

// CommunityNote represents a fact-checking note on an ActivityPub object
type CommunityNote struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK"` // NOTE#<id>
	SK string `dynamorm:"sk,attr:SK"` // METADATA

	// GSI fields for querying
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK"` // OBJECT#<object_id>#NOTES
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK"` // SCORE#<score>#<id>
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK"` // NOTES#<visibility_status>
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK"` // <created_at>#<id>
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsI3PK"` // AUTHOR#<author_id>#NOTES
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsI3SK"` // <created_at>#<id>

	// Core fields matching storage.CommunityNote
	ID               string   `dynamorm:"attr:id" json:"id"`
	ObjectID         string   `dynamorm:"attr:objectID" json:"object_id"`
	ObjectType       string   `dynamorm:"attr:objectType" json:"object_type"`
	AuthorID         string   `dynamorm:"attr:authorID" json:"author_id"`
	Content          string   `dynamorm:"attr:content" json:"content"`
	Language         string   `dynamorm:"attr:language" json:"language"`
	Sources          []string `dynamorm:"attr:sources" json:"sources"`
	HelpfulVotes     int      `dynamorm:"attr:helpfulVotes" json:"helpful_votes"`
	NotHelpfulVotes  int      `dynamorm:"attr:notHelpfulVotes" json:"not_helpful_votes"`
	Score            float64  `dynamorm:"attr:score" json:"score"`
	VisibilityStatus string   `dynamorm:"attr:visibilityStatus" json:"visibility_status"`

	// AI Analysis fields
	Sentiment     float64 `dynamorm:"attr:sentiment" json:"sentiment"`
	Objectivity   float64 `dynamorm:"attr:objectivity" json:"objectivity"`
	SourceQuality float64 `dynamorm:"attr:sourceQuality" json:"source_quality"`

	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL field
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing CommunityNote.
func (CommunityNote) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on the current field values
func (n *CommunityNote) UpdateKeys() error {
	n.PK = fmt.Sprintf(KeyPatternNote, n.ID)
	n.SK = SKMetadata

	// GSI1: Query by object ID, sorted by score
	n.GSI1PK = fmt.Sprintf(KeyPatternObjectNotes, n.ObjectID)
	n.GSI1SK = fmt.Sprintf("SCORE#%010.6f#%s", n.Score, n.ID)

	// GSI2: Query by visibility status, sorted by creation time
	n.GSI2PK = fmt.Sprintf("NOTES#%s", n.VisibilityStatus)
	n.GSI2SK = fmt.Sprintf("%s#%s", n.CreatedAt.Format(time.RFC3339), n.ID)

	// GSI3: Query by author, sorted by creation time
	n.GSI3PK = fmt.Sprintf("AUTHOR#%s#NOTES", n.AuthorID)
	n.GSI3SK = fmt.Sprintf("%s#%s", n.CreatedAt.Format(time.RFC3339), n.ID)

	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (n *CommunityNote) GetPK() string {
	return n.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (n *CommunityNote) GetSK() string {
	return n.SK
}

// CommunityNoteVote represents a vote on a community note
type CommunityNoteVote struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK"` // NOTE#<note_id>
	SK string `dynamorm:"sk,attr:SK"` // VOTE#<voter_id>

	// Core fields matching storage.CommunityNoteVote
	NoteID    string    `dynamorm:"attr:noteID" json:"note_id"`
	VoterID   string    `dynamorm:"attr:voterID" json:"voter_id"`
	VoteType  string    `dynamorm:"attr:voteType" json:"vote_type"` // helpful, not_helpful, neutral
	Helpful   bool      `dynamorm:"attr:helpful" json:"helpful"`    // For simplified access
	Weight    float64   `dynamorm:"attr:weight" json:"weight"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`

	// TTL field
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing CommunityNoteVote.
func (CommunityNoteVote) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys based on the current field values
func (v *CommunityNoteVote) UpdateKeys() error {
	v.PK = fmt.Sprintf(KeyPatternNote, v.NoteID)
	v.SK = fmt.Sprintf("VOTE#%s", v.VoterID)
	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (v *CommunityNoteVote) GetPK() string {
	return v.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (v *CommunityNoteVote) GetSK() string {
	return v.SK
}
