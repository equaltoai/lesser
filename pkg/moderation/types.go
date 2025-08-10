package moderation

import (
	"time"
)

// EventType represents the type of moderation event
type EventType string

const (
	// EventTypeFlagged represents a flagged content event
	EventTypeFlagged EventType = "flagged"
	// EventTypeReviewed represents a reviewed content event
	EventTypeReviewed EventType = "reviewed"
	// EventTypeAppealed represents an appealed content event
	EventTypeAppealed EventType = "appealed"
	// EventTypeExpired represents an expired content event
	EventTypeExpired EventType = "expired"
)

// Category represents the category of moderation
type Category string

const (
	// CategorySpam represents spam content category
	CategorySpam Category = "spam"
	// CategoryHateSpeech represents hate speech content category
	CategoryHateSpeech Category = "hate_speech"
	// CategoryHarassment represents harassment content category
	CategoryHarassment Category = "harassment"
	// CategoryMisinformation represents misinformation content category
	CategoryMisinformation Category = "misinformation"
	// CategoryNSFW represents NSFW content category
	CategoryNSFW Category = "nsfw"
	// CategoryViolence represents violence-related content
	CategoryViolence Category = "violence"
	// CategoryOther represents other content categories
	CategoryOther Category = "other"
)

// Severity represents the severity level
type Severity int

const (
	// SeverityLow represents low severity level
	SeverityLow Severity = 1
	// SeverityMedium represents medium severity level
	SeverityMedium Severity = 2
	// SeverityHigh represents high severity level
	SeverityHigh Severity = 3
	// SeverityCritical represents critical severity level
	SeverityCritical Severity = 4
)

// ActionType represents the action to take
type ActionType string

const (
	// ActionTypeNone represents no action taken
	ActionTypeNone ActionType = "none"
	// ActionTypeWarning represents a warning action
	ActionTypeWarning ActionType = "warning"
	// ActionTypeSilence represents a silence action
	ActionTypeSilence ActionType = "silence"
	// ActionTypeSuspend represents a suspend action
	ActionTypeSuspend ActionType = "suspend"
	// ActionTypeRemove represents a remove action
	ActionTypeRemove ActionType = "remove"
)

// Evidence represents supporting evidence for a moderation event
type Evidence struct {
	Type        string         `json:"type"`        // ai_detection, user_report, pattern_match, etc.
	Score       float64        `json:"score"`       // Confidence score 0.0-1.0
	Description string         `json:"description"` // Human-readable description
	Metadata    map[string]any `json:"metadata,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

// ModerationEvent represents a moderation event in the system
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific event
type ModerationEvent struct {
	ID              string     `json:"id"`
	EventType       EventType  `json:"event_type"`
	ObjectID        string     `json:"object_id"`   // ID of content being moderated
	ObjectType      string     `json:"object_type"` // status, account, media
	ActorID         string     `json:"actor_id"`    // Who triggered this event
	Category        Category   `json:"category"`
	Severity        Severity   `json:"severity"`
	ConfidenceScore float64    `json:"confidence_score"` // 0.0-1.0
	Evidence        []Evidence `json:"evidence"`
	Reason          string     `json:"reason,omitempty"` // Human-provided reason
	Created         time.Time  `json:"created"`
	Updated         time.Time  `json:"updated"`
	TTL             int64      `json:"ttl,omitempty"` // Unix timestamp for expiration
}

// Review represents a moderation review by a reviewer
type Review struct {
	ID         string     `json:"id"`
	EventID    string     `json:"event_id"`
	ReviewerID string     `json:"reviewer_id"`
	Action     ActionType `json:"action"`
	Category   Category   `json:"category"`
	Severity   Severity   `json:"severity"`
	Confidence float64    `json:"confidence"` // 0.0-1.0
	Notes      string     `json:"notes,omitempty"`
	Weight     float64    `json:"weight"` // Trust-weighted value
	Created    time.Time  `json:"created"`
}

// ModerationDecision represents the consensus decision
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific decision
type ModerationDecision struct {
	ID               string     `json:"id"`
	EventID          string     `json:"event_id"`
	ObjectID         string     `json:"object_id"`
	Action           ActionType `json:"action"`
	Reason           string     `json:"reason,omitempty"`   // Reason for the decision
	ConsensusScore   float64    `json:"consensus_score"`    // Agreement percentage
	ReviewerCount    int        `json:"reviewer_count"`     // Number of reviewers
	TrustWeightTotal float64    `json:"trust_weight_total"` // Total trust weight
	Reviews          []*Review  `json:"reviews"`
	Decided          time.Time  `json:"decided"`
	AppliedAt        *time.Time `json:"applied_at,omitempty"`
	AppealedAt       *time.Time `json:"appealed_at,omitempty"`
}

// QueueItem represents an item in the moderation queue
type QueueItem struct {
	Event          *ModerationEvent `json:"event"`
	Priority       float64          `json:"priority"` // Calculated based on severity and confidence
	ReviewCount    int              `json:"review_count"`
	LastReviewedAt *time.Time       `json:"last_reviewed_at,omitempty"`
}

// ModerationHistory represents the complete history for an object
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific history
type ModerationHistory struct {
	ObjectID      string               `json:"object_id"`
	Events        []ModerationEvent    `json:"events"`
	Decisions     []ModerationDecision `json:"decisions"`
	CurrentStatus string               `json:"current_status"`
	Timeline      []TimelineEntry      `json:"timeline"`
}

// TimelineEntry represents an entry in the moderation timeline
type TimelineEntry struct {
	Timestamp   time.Time      `json:"timestamp"`
	Type        string         `json:"type"` // event, review, decision, appeal
	ActorID     string         `json:"actor_id"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ConsensusConfig represents the configuration for consensus calculation
type ConsensusConfig struct {
	MinReviewers        int     `json:"min_reviewers"`        // Minimum number of reviewers
	MinTrustWeight      float64 `json:"min_trust_weight"`     // Minimum total trust weight
	ConsensusThreshold  float64 `json:"consensus_threshold"`  // Percentage agreement required
	CriticalThreshold   float64 `json:"critical_threshold"`   // Higher threshold for critical actions
	EscalationThreshold float64 `json:"escalation_threshold"` // When to escalate for review
	ReviewTimeoutHours  int     `json:"review_timeout_hours"` // Hours before auto-decision
}

// DefaultConsensusConfig returns the default consensus configuration
func DefaultConsensusConfig() *ConsensusConfig {
	return &ConsensusConfig{
		MinReviewers:        3,
		MinTrustWeight:      0.5,
		ConsensusThreshold:  0.7,
		CriticalThreshold:   0.9,
		EscalationThreshold: 0.8,
		ReviewTimeoutHours:  24,
	}
}
