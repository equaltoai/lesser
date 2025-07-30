package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Actor represents an ActivityPub actor stored in DynamoDB using DynamORM
type Actor struct {
	// Primary key - using actor username as the primary identifier
	PK string `dynamorm:"pk" json:"pk"` // Format: "ACTOR#{username}" - MUST match legacy exactly
	SK string `dynamorm:"sk" json:"sk"` // Format: "PROFILE" - MUST match legacy exactly

	// GSI1 - Username search with prefix partitioning for efficient queries
	GSI1PK string `dynamorm:"index:username-search-index,pk" json:"gsi1_pk"` // Format: "USERNAME_SEARCH#{first_2_chars}"
	GSI1SK string `dynamorm:"index:username-search-index,sk" json:"gsi1_sk"` // Format: "{lowercase_username}"

	// GSI2 - Display name search
	GSI2PK string `dynamorm:"index:name-search-index,pk" json:"gsi2_pk,omitempty"` // Format: "NAME_SEARCH#{first_2_chars}"
	GSI2SK string `dynamorm:"index:name-search-index,sk" json:"gsi2_sk,omitempty"` // Format: "{lowercase_name}#{username}"

	// GSI3 - Domain-based queries for federation
	GSI3PK string `dynamorm:"index:domain-index,pk" json:"gsi3_pk"` // Format: "DOMAIN#{domain}"
	GSI3SK string `dynamorm:"index:domain-index,sk" json:"gsi3_sk"` // Format: "{username}"

	// GSI4 - Popularity ranking by follower count
	GSI4PK string `dynamorm:"index:popularity-index,pk" json:"gsi4_pk"` // Format: "ACTOR_RANK#{bucket}"
	GSI4SK string `dynamorm:"index:popularity-index,sk" json:"gsi4_sk"` // Format: "{padded_count}#{username}"

	// GSI5 - Recent activity tracking
	GSI5PK string `dynamorm:"index:activity-index,pk" json:"gsi5_pk"` // Format: "ACTIVE#{date}"
	GSI5SK string `dynamorm:"index:activity-index,sk" json:"gsi5_sk"` // Format: "{timestamp}#{username}"

	// Core actor data
	Actor      *activitypub.Actor `dynamorm:"json" json:"actor"`
	Username   string             `json:"username"`
	PrivateKey string             `json:"private_key,omitempty"` // Encrypted private key
	NumericID  string             `json:"numeric_id"`            // Mastodon-compatible numeric ID

	// Metadata
	CreatedAt    time.Time    `dynamorm:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `dynamorm:"updated_at" json:"updated_at"`
	LastStatusAt *time.Time   `json:"last_status_at,omitempty"`
	Fields       []ActorField `dynamorm:"json" json:"fields,omitempty"`

	// Counts for efficient access
	FollowerCount  int `json:"follower_count"`
	FollowingCount int `json:"following_count"`
	StatusCount    int `json:"status_count"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// ActorField represents a profile field (like bio fields in Mastodon)
type ActorField struct {
	Name       string     `json:"name"`
	Value      string     `json:"value"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

// ActorMetadata contains additional metadata about an actor
type ActorMetadata struct {
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	LastStatusAt *time.Time   `json:"last_status_at,omitempty"`
	Fields       []ActorField `json:"fields,omitempty"`
}

// TableName returns the DynamoDB table name for the Actor model
func (Actor) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (a *Actor) BeforeCreate() error {
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	// Set up primary key - matches legacy pattern exactly
	a.PK = "ACTOR#" + a.Username
	a.SK = "PROFILE"

	// Set up GSI keys
	a.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (a *Actor) BeforeUpdate() error {
	a.UpdatedAt = time.Now()

	// Update GSI keys in case display name changed
	a.setupGSIKeys()

	return nil
}

// setupGSIKeys configures all GSI partition and sort keys
func (a *Actor) setupGSIKeys() {
	username := a.Username
	usernameLower := strings.ToLower(username)

	// GSI1 - Username search with prefix partitioning
	if len(usernameLower) >= 2 {
		a.GSI1PK = "USERNAME_SEARCH#" + usernameLower[:2]
		a.GSI1SK = usernameLower
	}

	// GSI2 - Display name search (if display name exists)
	if a.Actor != nil && a.Actor.Name != "" {
		displayNameLower := strings.ToLower(a.Actor.Name)
		if len(displayNameLower) >= 2 {
			a.GSI2PK = "NAME_SEARCH#" + displayNameLower[:2]
			a.GSI2SK = displayNameLower + "#" + username
		}
	}

	// GSI3 - Domain search (for federation)
	// This would be set based on the domain configuration
	// For now, we'll leave it empty and set it in the service layer

	// GSI4 - Popularity ranking
	bucket := getFollowerCountBucket(a.FollowerCount)
	a.GSI4PK = "ACTOR_RANK#" + bucket
	a.GSI4SK = formatFollowerCountForGSI(a.FollowerCount, username)

	// GSI5 - Recent activity
	now := time.Now()
	a.GSI5PK = "ACTIVE#" + now.Format("2006-01-02")
	a.GSI5SK = fmt.Sprintf("%d#%s", now.Unix(), username)
}

// getFollowerCountBucket returns a bucket for follower count grouping
func getFollowerCountBucket(count int) string {
	switch {
	case count >= 10000:
		return "10K+"
	case count >= 1000:
		return "1K+"
	case count >= 100:
		return "100+"
	case count >= 10:
		return "10+"
	default:
		return "0-9"
	}
}

// formatFollowerCountForGSI formats follower count for GSI sorting
func formatFollowerCountForGSI(count int, username string) string {
	// Pad count to 10 digits for proper sorting, then add username for uniqueness
	return fmt.Sprintf("%010d#%s", count, username)
}

// UpdateKeys updates the GSI keys for this actor (required by DynamORM)
func (a *Actor) UpdateKeys() {
	a.setupGSIKeys()
}
