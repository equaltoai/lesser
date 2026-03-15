package models

import (
	"fmt"
	"strings"
	"time"
)

// RelationshipRecord represents a follow relationship between users
// This model preserves the EXACT key patterns from the legacy implementation
type RelationshipRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly (UPPERCASE prefixes!)
	PK string `theorydb:"pk,attr:PK" json:"PK"` // FOLLOW#{followerUsername}
	SK string `theorydb:"sk,attr:SK" json:"SK"` // FOLLOWING#{followingUsername}

	// GSI1 for reverse lookups (who follows me)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK"` // FOLLOW#{followedUsername}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK"` // FOLLOWER#{followerUsername}

	// GSI2 for follower domain queries (Phase 2.4 - severance detection)
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2PK,omitempty"` // FOLLOWER_DOMAIN#{domain}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2SK,omitempty"` // FOLLOWING#{username}

	// GSI3 for following domain queries (Phase 2.4 - severance detection)
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"gsi3PK,omitempty"` // FOLLOWING_DOMAIN#{domain}
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"gsi3SK,omitempty"` // FOLLOWER#{username}

	// Core fields from legacy
	ActivityID string    `theorydb:"attr:activityID" json:"ActivityID"`
	State      string    `theorydb:"attr:state" json:"State"` // pending, accepted, rejected
	CreatedAt  time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"UpdatedAt"`

	// Relationship preferences
	Notifying      bool     `theorydb:"attr:notifying" json:"Notifying"`           // Receive notifications for this user's posts
	ShowingReblogs bool     `theorydb:"attr:showingReblogs" json:"ShowingReblogs"` // Show reblogs from this user in timeline
	Languages      []string `theorydb:"attr:languages" json:"Languages,omitempty"` // Filter to specific languages (empty = all)
	Note           string   `theorydb:"attr:note" json:"Note,omitempty"`           // Private note about this relationship
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
	// Validate that PK and SK are set
	if r.PK == "" {
		return fmt.Errorf("PK is required (format: FOLLOW#{followerUsername})")
	}
	if r.SK == "" {
		return fmt.Errorf("SK is required (format: FOLLOWING#{followingUsername})")
	}

	// Note: Primary keys (PK, SK) should already be set by the caller/repository
	// We only validate and update GSI keys here

	// GSI1 keys should also be set by repository methods
	// Update domain-based GSI keys (GSI2, GSI3) for severance detection

	// Extract follower username from PK: FOLLOW#{username}
	followerUsername := strings.TrimPrefix(r.PK, "FOLLOW#")
	if followerDomain, ok := extractRelationshipDomain(followerUsername); ok {
		r.GSI2PK = fmt.Sprintf("FOLLOWER_DOMAIN#%s", followerDomain)
		r.GSI2SK = r.SK // FOLLOWING#{username}
	} else if strings.TrimSpace(r.GSI2PK) == "" || strings.TrimSpace(r.GSI2SK) == "" {
		r.GSI2PK = ""
		r.GSI2SK = ""
	}

	// Extract following username from SK: FOLLOWING#{username}
	followingUsername := strings.TrimPrefix(r.SK, "FOLLOWING#")
	if followingDomain, ok := extractRelationshipDomain(followingUsername); ok {
		r.GSI3PK = fmt.Sprintf("FOLLOWING_DOMAIN#%s", followingDomain)
		r.GSI3SK = r.GSI1SK // FOLLOWER#{username}
	} else if strings.TrimSpace(r.GSI3PK) == "" || strings.TrimSpace(r.GSI3SK) == "" {
		r.GSI3PK = ""
		r.GSI3SK = ""
	}

	return nil
}

// extractRelationshipDomain extracts the domain from a federated handle (username@domain)
// Specific to relationship records to avoid conflicts with other domain extractors
func extractRelationshipDomain(handle string) (string, bool) {
	parts := strings.Split(handle, "@")
	if len(parts) < 2 {
		// Local user, no domain
		return "", false
	}
	domain := strings.ToLower(parts[len(parts)-1])
	if domain == "" || domain == "localhost" {
		return "", false
	}
	return domain, true
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
