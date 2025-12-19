package models

import (
	"fmt"
	"time"
)

// QuoteRelationship represents a quote relationship between notes
type QuoteRelationship struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI fields for querying by quoted status
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk,omitempty"`

	// GSI fields for querying by author
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2sk,omitempty"`

	// Business fields
	ID             string     `dynamorm:"attr:id" json:"id"`
	QuoterNoteID   string     `dynamorm:"attr:quoterNoteID" json:"quoter_note_id"`
	TargetNoteID   string     `dynamorm:"attr:targetNoteID" json:"target_note_id"`
	QuoterID       string     `dynamorm:"attr:quoterID" json:"quoter_id"`
	TargetAuthorID string     `dynamorm:"attr:targetAuthorID" json:"target_author_id,omitempty"`
	Timestamp      time.Time  `dynamorm:"attr:timestamp" json:"timestamp"`
	Withdrawn      bool       `dynamorm:"attr:withdrawn" json:"withdrawn"`
	WithdrawnAt    *time.Time `dynamorm:"attr:withdrawnAt" json:"withdrawn_at,omitempty"`
}

// UpdateKeys updates the composite keys based on the quote relationship
func (q *QuoteRelationship) UpdateKeys() error {
	// Primary key: QUOTE#quotingStatusID
	q.PK = fmt.Sprintf("QUOTE#%s", q.QuoterNoteID)
	q.SK = fmt.Sprintf("QUOTED#%s", q.TargetNoteID)

	// GSI1: For finding all quotes of a specific status
	q.GSI1PK = fmt.Sprintf("QUOTED#%s", q.TargetNoteID)
	q.GSI1SK = fmt.Sprintf("%s#%s", q.Timestamp.Format(time.RFC3339), q.QuoterNoteID)

	// GSI2: For finding all quotes by a specific author
	q.GSI2PK = fmt.Sprintf("QUOTER#%s", q.QuoterID)
	q.GSI2SK = fmt.Sprintf("%s#%s", q.Timestamp.Format(time.RFC3339), q.QuoterNoteID)

	// Clear GSI keys if withdrawn
	if q.Withdrawn {
		q.GSI1PK = ""
		q.GSI1SK = ""
		q.GSI2PK = ""
		q.GSI2SK = ""
	}
	return nil
}

// GetPK returns the partition key
func (q *QuoteRelationship) GetPK() string {
	return q.PK
}

// GetSK returns the sort key
func (q *QuoteRelationship) GetSK() string {
	return q.SK
}

// Withdraw marks the quote relationship as withdrawn
func (q *QuoteRelationship) Withdraw() {
	q.Withdrawn = true
	now := time.Now()
	q.WithdrawnAt = &now
	_ = q.UpdateKeys() // This will clear GSI keys
}

// IsActive returns whether the quote relationship is active (not withdrawn)
func (q *QuoteRelationship) IsActive() bool {
	return !q.Withdrawn
}

// GenerateID generates a unique ID for the quote relationship
func (q *QuoteRelationship) GenerateID() {
	q.ID = fmt.Sprintf("%s:%s", q.QuoterNoteID, q.TargetNoteID)
}

// TableName returns the DynamoDB table backing QuoteRelationship.
func (QuoteRelationship) TableName() string {
	return MainTableName
}
