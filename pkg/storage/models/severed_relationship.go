package models

import (
	"fmt"
	"time"
)

// SeveranceReason represents why a federation relationship was severed
type SeveranceReason string

const (
	// SeveranceReasonDomainBlock represents a blocked severance reason
	SeveranceReasonDomainBlock SeveranceReason = "DOMAIN_BLOCK"
	// SeveranceReasonInstanceDown represents an unavailable severance reason
	SeveranceReasonInstanceDown SeveranceReason = "INSTANCE_DOWN"
	// SeveranceReasonDefederation represents a defederated severance reason
	SeveranceReasonDefederation SeveranceReason = "DEFEDERATION"
	// SeveranceReasonPolicyViolation represents a policy violation severance reason
	SeveranceReasonPolicyViolation SeveranceReason = "POLICY_VIOLATION"
	// SeveranceReasonOther represents other severance reasons
	SeveranceReasonOther SeveranceReason = "OTHER"
)

// SeveranceStatus represents the status of a severed relationship
type SeveranceStatus string

const (
	// SeveranceStatusActive represents an active severance
	SeveranceStatusActive SeveranceStatus = "ACTIVE"
	// SeveranceStatusAcknowledged represents an acknowledged severance
	SeveranceStatusAcknowledged SeveranceStatus = "ACKNOWLEDGED"
	// SeveranceStatusRestored represents a restored relationship
	SeveranceStatusRestored SeveranceStatus = "RESTORED"
)

// SeveredRelationship represents a broken federation relationship
type SeveredRelationship struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // SEVERED#{localInstance}
	SK string `dynamorm:"sk" json:"-"` // INSTANCE#{remoteInstance}#{timestamp}

	// GSI1 for filtering by status
	GSI1PK string `dynamorm:"gsi1pk" json:"-"` // STATUS#{status}
	GSI1SK string `dynamorm:"gsi1sk" json:"-"` // TIMESTAMP#{timestamp}

	// Fields
	ID                  string          `json:"id"`
	LocalInstance       string          `json:"local_instance"`
	RemoteInstance      string          `json:"remote_instance"`
	Reason              SeveranceReason `json:"reason"`
	Status              SeveranceStatus `json:"status"`
	Severity            string          `json:"severity"` // "low", "medium", "high"
	AffectedFollowers   int             `json:"affected_followers"`
	AffectedFollowing   int             `json:"affected_following"`
	DetectedAt          time.Time       `json:"detected_at"`
	AcknowledgedAt      *time.Time      `json:"acknowledged_at,omitempty"`
	Reversible          bool            `json:"reversible"`
	Details             string          `json:"details,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	AutoDetected        bool            `json:"auto_detected"`
	AdminNotes          string          `json:"admin_notes,omitempty"`
	ReconnectionAttempt bool            `json:"reconnection_attempt"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`

	// TTL for auto-cleanup (180 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing SeveredRelationship.
func (SeveredRelationship) TableName() string {
	return MainTableName
}

// AffectedRelationship represents a single affected follow/follower relationship
type AffectedRelationship struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // SEVERED#{severanceID}
	SK string `dynamorm:"sk" json:"-"` // AFFECTED#{actorID}

	// GSI1 for reverse lookup by actor
	GSI1PK string `dynamorm:"gsi1pk" json:"-"` // ACTOR#{actorID}
	GSI1SK string `dynamorm:"gsi1sk" json:"-"` // SEVERED#{severanceID}

	// Fields
	SeveranceID      string     `json:"severance_id"`
	ActorID          string     `json:"actor_id"`
	ActorHandle      string     `json:"actor_handle"`
	ActorDomain      string     `json:"actor_domain"`
	RelationshipType string     `json:"relationship_type"` // "follower" or "following"
	EstablishedAt    time.Time  `json:"established_at"`
	LastInteraction  *time.Time `json:"last_interaction,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`

	// TTL for auto-cleanup (180 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing AffectedRelationship.
func (AffectedRelationship) TableName() string {
	return MainTableName
}

// SeveranceReconnectionAttempt represents an attempt to restore severed relationships
type SeveranceReconnectionAttempt struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // SEVERED#{severanceID}
	SK string `dynamorm:"sk" json:"-"` // RECONNECT#{attemptID}

	// Fields
	ID            string     `json:"id"`
	SeveranceID   string     `json:"severance_id"`
	InitiatedBy   string     `json:"initiated_by"` // User ID who initiated
	Status        string     `json:"status"`       // "pending", "in_progress", "completed", "failed"
	AttemptedAt   time.Time  `json:"attempted_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	SuccessCount  int        `json:"success_count"`
	FailureCount  int        `json:"failure_count"`
	Notes         string     `json:"notes,omitempty"`
	ErrorMessages []string   `json:"error_messages,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// TTL for auto-cleanup (90 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing SeveranceReconnectionAttempt.
func (SeveranceReconnectionAttempt) TableName() string {
	return MainTableName
}

// NewSeveredRelationship creates a new severed relationship record
func NewSeveredRelationship(localInstance, remoteInstance string, reason SeveranceReason) *SeveredRelationship {
	now := time.Now()
	id := fmt.Sprintf("%s_%s_%d", localInstance, remoteInstance, now.Unix())

	return &SeveredRelationship{
		ID:                  id,
		LocalInstance:       localInstance,
		RemoteInstance:      remoteInstance,
		Reason:              reason,
		Status:              SeveranceStatusActive,
		Severity:            "medium",
		AffectedFollowers:   0,
		AffectedFollowing:   0,
		DetectedAt:          now,
		Reversible:          true,
		AutoDetected:        false,
		ReconnectionAttempt: false,
		CreatedAt:           now,
		UpdatedAt:           now,
		TTL:                 now.Add(180 * 24 * time.Hour).Unix(),
	}
}

// NewAffectedRelationship creates a new affected relationship record
func NewAffectedRelationship(severanceID, actorID, actorHandle, actorDomain, relationshipType string, establishedAt time.Time) *AffectedRelationship {
	now := time.Now()

	return &AffectedRelationship{
		SeveranceID:      severanceID,
		ActorID:          actorID,
		ActorHandle:      actorHandle,
		ActorDomain:      actorDomain,
		RelationshipType: relationshipType,
		EstablishedAt:    establishedAt,
		CreatedAt:        now,
		TTL:              now.Add(180 * 24 * time.Hour).Unix(),
	}
}

// NewSeveranceReconnectionAttempt creates a new reconnection attempt record
func NewSeveranceReconnectionAttempt(severanceID, initiatedBy string) *SeveranceReconnectionAttempt {
	now := time.Now()
	id := fmt.Sprintf("%s_%d", severanceID, now.Unix())

	return &SeveranceReconnectionAttempt{
		ID:            id,
		SeveranceID:   severanceID,
		InitiatedBy:   initiatedBy,
		Status:        "pending",
		AttemptedAt:   now,
		SuccessCount:  0,
		FailureCount:  0,
		ErrorMessages: []string{},
		CreatedAt:     now,
		UpdatedAt:     now,
		TTL:           now.Add(90 * 24 * time.Hour).Unix(),
	}
}

// UpdateKeys updates the DynamoDB keys for the severed relationship
func (s *SeveredRelationship) UpdateKeys() error {
	s.PK = fmt.Sprintf("SEVERED#%s", s.LocalInstance)
	s.SK = fmt.Sprintf("INSTANCE#%s#%d", s.RemoteInstance, s.DetectedAt.Unix())
	s.GSI1PK = fmt.Sprintf("STATUS#%s", s.Status)
	s.GSI1SK = fmt.Sprintf("TIMESTAMP#%d", s.DetectedAt.Unix())
	return nil
}

// GetPK returns the partition key
func (s *SeveredRelationship) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *SeveredRelationship) GetSK() string {
	return s.SK
}

// Acknowledge marks the severance as acknowledged
func (s *SeveredRelationship) Acknowledge() {
	now := time.Now()
	s.Status = SeveranceStatusAcknowledged
	s.AcknowledgedAt = &now
	s.UpdatedAt = now
}

// MarkReconnectionAttempt marks that a reconnection attempt has been made
func (s *SeveredRelationship) MarkReconnectionAttempt() {
	s.ReconnectionAttempt = true
	s.UpdatedAt = time.Now()
}

// UpdateKeys updates the DynamoDB keys for the affected relationship
func (a *AffectedRelationship) UpdateKeys() error {
	a.PK = fmt.Sprintf("SEVERED#%s", a.SeveranceID)
	a.SK = fmt.Sprintf("AFFECTED#%s", a.ActorID)
	a.GSI1PK = fmt.Sprintf("ACTOR#%s", a.ActorID)
	a.GSI1SK = fmt.Sprintf("SEVERED#%s", a.SeveranceID)
	return nil
}

// GetPK returns the partition key
func (a *AffectedRelationship) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *AffectedRelationship) GetSK() string {
	return a.SK
}

// UpdateKeys updates the DynamoDB keys for the reconnection attempt
func (r *SeveranceReconnectionAttempt) UpdateKeys() error {
	r.PK = fmt.Sprintf("SEVERED#%s", r.SeveranceID)
	r.SK = fmt.Sprintf("RECONNECT#%s", r.ID)
	return nil
}

// GetPK returns the partition key
func (r *SeveranceReconnectionAttempt) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *SeveranceReconnectionAttempt) GetSK() string {
	return r.SK
}

// MarkInProgress marks the reconnection attempt as in progress
func (r *SeveranceReconnectionAttempt) MarkInProgress() {
	r.Status = "in_progress"
	r.UpdatedAt = time.Now()
}

// MarkCompleted marks the reconnection attempt as completed
func (r *SeveranceReconnectionAttempt) MarkCompleted(successCount, failureCount int) {
	now := time.Now()
	r.Status = "completed"
	r.SuccessCount = successCount
	r.FailureCount = failureCount
	r.CompletedAt = &now
	r.UpdatedAt = now
}

// MarkFailed marks the reconnection attempt as failed
func (r *SeveranceReconnectionAttempt) MarkFailed(errorMessage string) {
	now := time.Now()
	r.Status = "failed"
	r.ErrorMessages = append(r.ErrorMessages, errorMessage)
	r.CompletedAt = &now
	r.UpdatedAt = now
}
