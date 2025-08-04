package models

import (
	"fmt"
	"time"
)

// Poll represents a poll entity in DynamoDB
type Poll struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"-"` // POLL#{pollId}
	SK string `dynamorm:"sk" json:"-"` // METADATA

	// GSI keys for querying by status
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // STATUS#{statusId}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // POLL

	// Business fields
	ID          string           `json:"id"`
	StatusID    string           `json:"statusId"`
	CreatedBy   string           `json:"createdBy"`
	Options     []string         `json:"options"`
	Multiple    bool             `json:"multiple"`
	HideTotals  bool             `json:"hideTotals"`
	ExpiresAt   time.Time        `json:"expiresAt"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	VotesCount  int              `json:"votesCount"`
	VotersCount int              `json:"votersCount"`
	Votes       map[string][]int `json:"votes"` // Map of voter ID to option indices

	// TTL for expiration
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the DynamoDB keys based on the business fields
func (p *Poll) UpdateKeys() error {
	p.PK = fmt.Sprintf("POLL#%s", p.ID)
	p.SK = "METADATA"
	p.GSI1PK = fmt.Sprintf("STATUS#%s", p.StatusID)
	p.GSI1SK = "POLL"

	// Set TTL for poll expiration (add 1 day buffer)
	if !p.ExpiresAt.IsZero() {
		p.TTL = p.ExpiresAt.Add(24 * time.Hour).Unix()
	}
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (p *Poll) GetPK() string {
	return p.PK
}

// GetSK returns the sort key (required by BaseModel)
func (p *Poll) GetSK() string {
	return p.SK
}

// PollVote represents an individual vote on a poll
type PollVote struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"-"` // POLL#{pollId}
	SK string `dynamorm:"sk" json:"-"` // VOTE#{voterId}

	// Business fields
	VoterID string    `json:"voterId"`
	Choices []int     `json:"choices"`
	VotedAt time.Time `json:"votedAt"`
}

// UpdateKeys updates the DynamoDB keys based on the business fields
func (v *PollVote) UpdateKeys(pollID string) {
	v.PK = fmt.Sprintf("POLL#%s", pollID)
	v.SK = fmt.Sprintf("VOTE#%s", v.VoterID)
}