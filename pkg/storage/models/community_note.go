package models

import (
	"fmt"
	"time"
)

// CommunityNote represents a fact-checking note on an ActivityPub object
type CommunityNote struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK"` // NOTE#<id>
	SK string `theorydb:"sk,attr:SK"` // METADATA

	// GSI fields for querying
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"` // OBJECT#<object_id>#NOTES
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"` // SCORE#<score>#<id>
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK"` // NOTES#<visibility_status>
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK"` // <created_at>#<id>
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK"` // AUTHOR#<author_id>#NOTES
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK"` // <created_at>#<id>

	// Core fields matching storage.CommunityNote
	ID               string   `theorydb:"attr:id" json:"id"`
	ObjectID         string   `theorydb:"attr:objectID" json:"object_id"`
	ObjectType       string   `theorydb:"attr:objectType" json:"object_type"`
	AuthorID         string   `theorydb:"attr:authorID" json:"author_id"`
	Content          string   `theorydb:"attr:content" json:"content"`
	Language         string   `theorydb:"attr:language" json:"language"`
	Sources          []string `theorydb:"attr:sources" json:"sources"`
	HelpfulVotes     int      `theorydb:"attr:helpfulVotes" json:"helpful_votes"`
	NotHelpfulVotes  int      `theorydb:"attr:notHelpfulVotes" json:"not_helpful_votes"`
	Score            float64  `theorydb:"attr:score" json:"score"`
	VisibilityStatus string   `theorydb:"attr:visibilityStatus" json:"visibility_status"`

	// AI Analysis fields
	Sentiment     float64 `theorydb:"attr:sentiment" json:"sentiment"`
	Objectivity   float64 `theorydb:"attr:objectivity" json:"objectivity"`
	SourceQuality float64 `theorydb:"attr:sourceQuality" json:"source_quality"`

	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL field
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK"` // NOTE#<note_id>
	SK string `theorydb:"sk,attr:SK"` // VOTE#<voter_id>

	// Core fields matching storage.CommunityNoteVote
	NoteID    string    `theorydb:"attr:noteID" json:"note_id"`
	VoterID   string    `theorydb:"attr:voterID" json:"voter_id"`
	VoteType  string    `theorydb:"attr:voteType" json:"vote_type"` // helpful, not_helpful, neutral
	Helpful   bool      `theorydb:"attr:helpful" json:"helpful"`    // For simplified access
	Weight    float64   `theorydb:"attr:weight" json:"weight"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`

	// TTL field
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
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
