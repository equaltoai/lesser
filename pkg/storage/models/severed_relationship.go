package models

import (
	"fmt"
	"time"
)

// SeveranceReason represents why a federation relationship was severed
type SeveranceReason string

const (
	// SeveranceReasonBlocked represents a blocked severance reason
	SeveranceReasonBlocked SeveranceReason = "blocked"
	// SeveranceReasonUnavailable represents an unavailable severance reason
	SeveranceReasonUnavailable SeveranceReason = "unavailable"
	// SeveranceReasonSuspended represents a suspended severance reason
	SeveranceReasonSuspended SeveranceReason = "suspended"
	// SeveranceReasonDefederated represents a defederated severance reason
	SeveranceReasonDefederated SeveranceReason = "defederated"
	// SeveranceReasonLimited represents a limited severance reason
	SeveranceReasonLimited SeveranceReason = "limited"
	// SeveranceReasonRestored represents a restored severance reason
	SeveranceReasonRestored SeveranceReason = "restored" // Added for restored relationships
)

// SeveredRelationship represents a broken federation relationship (matches legacy structure)
type SeveredRelationship struct {
	PK string `dynamorm:"pk"` // SEVERED#localInstance#remoteInstance
	SK string `dynamorm:"sk"` // TIMESTAMP#timestamp

	ID              string           `json:"id"`
	LocalInstance   string           `json:"local_instance"`
	RemoteInstance  string           `json:"remote_instance"`
	Reason          SeveranceReason  `json:"reason"`
	AffectedFollows []AffectedFollow `json:"affected_follows"`
	Timestamp       time.Time        `json:"timestamp"`
	Reversible      bool             `json:"reversible"`
	Details         string           `json:"details,omitempty"`
	EstimatedImpact int              `json:"estimated_impact"` // Number of affected relationships
}

// AffectedFollow represents a follow relationship affected by severance
type AffectedFollow struct {
	LocalUser    string    `json:"local_user"`
	RemoteUser   string    `json:"remote_user"`
	Direction    string    `json:"direction"` // "following", "follower", "mutual"
	LastActivity time.Time `json:"last_activity"`
}

// UpdateKeys updates the DynamoDB keys for the severed relationship
func (s *SeveredRelationship) UpdateKeys() {
	s.PK = fmt.Sprintf("SEVERED#%s#%s", s.LocalInstance, s.RemoteInstance)
	s.SK = fmt.Sprintf("TIMESTAMP#%d", s.Timestamp.Unix())
}
