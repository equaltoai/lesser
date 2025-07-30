package models

import (
	"fmt"
	"time"
)

// QuoteRelationship represents a quote relationship between notes
type QuoteRelationship struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI fields for querying by quoted status
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1sk,omitempty"`

	// GSI fields for querying by author
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2sk,omitempty"`

	// Business fields
	ID             string     `json:"id"`
	QuoterNoteID   string     `json:"quoter_note_id"`
	TargetNoteID   string     `json:"target_note_id"`
	QuoterID       string     `json:"quoter_id"`
	TargetAuthorID string     `json:"target_author_id,omitempty"`
	Timestamp      time.Time  `json:"timestamp"`
	Withdrawn      bool       `json:"withdrawn"`
	WithdrawnAt    *time.Time `json:"withdrawn_at,omitempty"`
}

// UpdateKeys updates the composite keys based on the quote relationship
func (q *QuoteRelationship) UpdateKeys() {
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
}

// Withdraw marks the quote relationship as withdrawn
func (q *QuoteRelationship) Withdraw() {
	q.Withdrawn = true
	now := time.Now()
	q.WithdrawnAt = &now
	q.UpdateKeys() // This will clear GSI keys
}

// IsActive returns whether the quote relationship is active (not withdrawn)
func (q *QuoteRelationship) IsActive() bool {
	return !q.Withdrawn
}

// GenerateID generates a unique ID for the quote relationship
func (q *QuoteRelationship) GenerateID() {
	q.ID = fmt.Sprintf("%s:%s", q.QuoterNoteID, q.TargetNoteID)
}