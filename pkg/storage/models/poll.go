package models

import (
	"fmt"
	"time"
)

// Poll represents a poll entity in DynamoDB
type Poll struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // POLL#{pollId}
	SK string `theorydb:"sk,attr:SK" json:"-"` // METADATA

	// GSI keys for querying by status
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"` // STATUS#{statusId}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"` // POLL

	// Business fields
	ID          string           `theorydb:"attr:id" json:"id"`
	StatusID    string           `theorydb:"attr:statusId" json:"statusId"`
	CreatedBy   string           `theorydb:"attr:createdBy" json:"createdBy"`
	Options     []string         `theorydb:"attr:options" json:"options"`
	Multiple    bool             `theorydb:"attr:multiple" json:"multiple"`
	HideTotals  bool             `theorydb:"attr:hideTotals" json:"hideTotals"`
	ExpiresAt   time.Time        `theorydb:"attr:expiresAt" json:"expiresAt"`
	CreatedAt   time.Time        `theorydb:"attr:createdAt" json:"createdAt"`
	UpdatedAt   time.Time        `theorydb:"attr:updatedAt" json:"updatedAt"`
	VotesCount  int              `theorydb:"attr:votesCount" json:"votesCount"`
	VotersCount int              `theorydb:"attr:votersCount" json:"votersCount"`
	Votes       map[string][]int `theorydb:"attr:votes" json:"votes"` // Map of voter ID to option indices

	// TTL for expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name for Poll records
func (Poll) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys based on the business fields
func (p *Poll) UpdateKeys() error {
	p.PK = fmt.Sprintf("POLL#%s", p.ID)
	p.SK = SKMetadata
	p.GSI1PK = fmt.Sprintf(KeyPatternStatus, p.StatusID)
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // POLL#{pollId}
	SK string `theorydb:"sk,attr:SK" json:"-"` // VOTE#{voterId}

	// Business fields
	VoterID string    `theorydb:"attr:voterId" json:"voterId"`
	Choices []int     `theorydb:"attr:choices" json:"choices"`
	VotedAt time.Time `theorydb:"attr:votedAt" json:"votedAt"`
}

// TableName returns the DynamoDB table name for PollVote records
func (PollVote) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys based on the business fields
func (v *PollVote) UpdateKeys() error {
	// PollVote keys are set when a pollID is provided during voting
	// This method is required by BaseModel interface but doesn't need pollID param
	// since the PK should already be set during vote creation
	return nil
}

// SetPollID sets the poll ID and updates keys (used during vote creation)
func (v *PollVote) SetPollID(pollID string) {
	v.PK = fmt.Sprintf("POLL#%s", pollID)
	v.SK = fmt.Sprintf("VOTE#%s", v.VoterID)
}

// GetPK returns the partition key (required by BaseModel)
func (v *PollVote) GetPK() string {
	return v.PK
}

// GetSK returns the sort key (required by BaseModel)
func (v *PollVote) GetSK() string {
	return v.SK
}
