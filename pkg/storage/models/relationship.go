package models

import (
	"fmt"
	"time"
)

// RelationshipRecord represents a follow relationship between users
// This model preserves the EXACT key patterns from the legacy implementation
type RelationshipRecord struct {
	// Primary keys - MUST match legacy exactly (UPPERCASE prefixes!)
	PK string `dynamorm:"pk" json:"PK"` // FOLLOW#{followerUsername}
	SK string `dynamorm:"sk" json:"SK"` // FOLLOWING#{followingUsername}

	// GSI1 for reverse lookups (who follows me)
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK"` // FOLLOW#{followedUsername}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK"` // FOLLOWER#{followerUsername}

	// Core fields from legacy
	ActivityID string    `json:"ActivityID"`
	State      string    `json:"State"` // pending, accepted, rejected
	CreatedAt  time.Time `json:"CreatedAt"`
	UpdatedAt  time.Time `json:"UpdatedAt"`

	// Relationship preferences
	Notifying      bool     `json:"Notifying"`           // Receive notifications for this user's posts
	ShowingReblogs bool     `json:"ShowingReblogs"`      // Show reblogs from this user in timeline
	Languages      []string `json:"Languages,omitempty"` // Filter to specific languages (empty = all)
	Note           string   `json:"Note,omitempty"`      // Private note about this relationship
}

// Follow relationship state constants (from legacy)
const (
	RelationshipPending  = "pending"
	RelationshipAccepted = "accepted"
	RelationshipRejected = "rejected"
)

// TableName returns the DynamoDB table name
func (RelationshipRecord) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the record before creation
func (r *RelationshipRecord) BeforeCreate() error {
	// Keys are set by the repository methods
	r.CreatedAt = time.Now()
	r.UpdatedAt = time.Now()
	if err := r.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// BeforeUpdate updates timestamps
func (r *RelationshipRecord) BeforeUpdate() error {
	r.UpdatedAt = time.Now()
	if err := r.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// UpdateKeys updates GSI keys based on primary keys
func (r *RelationshipRecord) UpdateKeys() error {
	// GSI keys are already set by repository methods
	// This method ensures they stay in sync if needed
	return nil
}

// GetPK returns the partition key
func (r *RelationshipRecord) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *RelationshipRecord) GetSK() string {
	return r.SK
}

// NewRelationshipRecord creates a new relationship record with proper keys
func NewRelationshipRecord(followerUsername, followingUsername, activityID string) *RelationshipRecord {
	now := time.Now()
	return &RelationshipRecord{
		PK:         fmt.Sprintf("FOLLOW#%s", followerUsername),
		SK:         fmt.Sprintf("FOLLOWING#%s", followingUsername),
		GSI1PK:     fmt.Sprintf("FOLLOW#%s", followingUsername),
		GSI1SK:     fmt.Sprintf("FOLLOWER#%s", followerUsername),
		ActivityID: activityID,
		State:      RelationshipPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Accept updates the relationship to accepted state
func (r *RelationshipRecord) Accept() {
	r.State = RelationshipAccepted
	r.UpdatedAt = time.Now()
}

// Reject updates the relationship to rejected state
func (r *RelationshipRecord) Reject() {
	r.State = RelationshipRejected
	r.UpdatedAt = time.Now()
}

// ExtractFollowerUsername extracts the follower username from PK
func (r *RelationshipRecord) ExtractFollowerUsername() string {
	prefix := "FOLLOW#"
	if len(r.PK) > len(prefix) {
		return r.PK[len(prefix):]
	}
	return ""
}

// ExtractFollowingUsername extracts the following username from SK
func (r *RelationshipRecord) ExtractFollowingUsername() string {
	prefix := "FOLLOWING#"
	if len(r.SK) > len(prefix) {
		return r.SK[len(prefix):]
	}
	return ""
}

// ExtractFollowerFromGSI extracts follower username from GSI1SK
func (r *RelationshipRecord) ExtractFollowerFromGSI() string {
	prefix := "FOLLOWER#"
	if len(r.GSI1SK) > len(prefix) {
		return r.GSI1SK[len(prefix):]
	}
	return ""
}
