package models

import (
	"fmt"
	"time"
)

// SocialRecoveryRequest represents a request for social account recovery
type SocialRecoveryRequest struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI fields for querying by status
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"gsi1sk,omitempty"`

	// GSI fields for querying by username
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsI2PK" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsI2SK" json:"gsi2sk,omitempty"`

	// Business fields
	ID            string          `dynamorm:"attr:id" json:"id"`
	Username      string          `dynamorm:"attr:username" json:"username"`
	InitiatedAt   time.Time       `dynamorm:"attr:initiatedAt" json:"initiated_at"`
	ExpiresAt     time.Time       `dynamorm:"attr:expiresAt" json:"expires_at"`
	RequiredVotes int             `dynamorm:"attr:requiredVotes" json:"required_votes"`
	ReceivedVotes map[string]bool `dynamorm:"attr:receivedVotes" json:"received_votes"` // trustee_id -> voted
	RecoveryToken string          `dynamorm:"attr:recoveryToken" json:"recovery_token"`
	Status        string          `dynamorm:"attr:status" json:"status"` // pending, approved, expired, cancelled
	TTL           int64           `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing SocialRecoveryRequest.
func (SocialRecoveryRequest) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on the recovery request
func (s *SocialRecoveryRequest) UpdateKeys() {
	// Primary key: RECOVERY#userID
	s.PK = fmt.Sprintf("RECOVERY#%s", s.Username)
	s.SK = fmt.Sprintf("REQUEST#%s", s.ID)

	// GSI1: For querying by status
	if s.Status != "" {
		s.GSI1PK = fmt.Sprintf("RECOVERY_STATUS#%s", s.Status)
		s.GSI1SK = fmt.Sprintf("%s#%s", s.ExpiresAt.Format(time.RFC3339), s.ID)
	}

	// GSI2: For querying pending requests by username
	if s.Status == StatusPending {
		s.GSI2PK = fmt.Sprintf(KeyPatternUser, s.Username)
		s.GSI2SK = fmt.Sprintf("RECOVERY#%s", s.InitiatedAt.Format(time.RFC3339))
	} else {
		// Clear GSI2 keys when not pending
		s.GSI2PK = ""
		s.GSI2SK = ""
	}

	// Set TTL to expiration time
	s.TTL = s.ExpiresAt.Unix()
}

// AddVote adds a vote from a trustee
func (s *SocialRecoveryRequest) AddVote(trusteeID string) bool {
	if s.ReceivedVotes == nil {
		s.ReceivedVotes = make(map[string]bool)
	}

	// Check if already voted
	if s.ReceivedVotes[trusteeID] {
		return false
	}

	s.ReceivedVotes[trusteeID] = true

	// Check if we have enough votes
	if s.GetVoteCount() >= s.RequiredVotes {
		s.Status = "approved"
		s.UpdateKeys()
	}

	return true
}

// GetVoteCount returns the number of votes received
func (s *SocialRecoveryRequest) GetVoteCount() int {
	count := 0
	for _, voted := range s.ReceivedVotes {
		if voted {
			count++
		}
	}
	return count
}

// IsExpired checks if the recovery request has expired
func (s *SocialRecoveryRequest) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsActive checks if the recovery request is still active
func (s *SocialRecoveryRequest) IsActive() bool {
	return s.Status == StatusPending && !s.IsExpired()
}

// Cancel cancels the recovery request
func (s *SocialRecoveryRequest) Cancel() {
	s.Status = "cancelled"
	s.UpdateKeys()
}

// MarkExpired marks the request as expired
func (s *SocialRecoveryRequest) MarkExpired() {
	s.Status = "expired"
	s.UpdateKeys()
}

// GetProgress returns the recovery progress as a percentage
func (s *SocialRecoveryRequest) GetProgress() float64 {
	if s.RequiredVotes == 0 {
		return 0
	}
	return float64(s.GetVoteCount()) / float64(s.RequiredVotes) * 100
}
