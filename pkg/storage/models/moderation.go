package models

import (
	"fmt"
	"time"
)

// ModerationAction represents the type of moderation action taken
type ModerationAction string

const (
	ModerationActionRemove  ModerationAction = "remove"
	ModerationActionSuspend ModerationAction = "suspend"
	ModerationActionSilence ModerationAction = "silence"
	ModerationActionWarning ModerationAction = "warning"
	ModerationActionRestore ModerationAction = "restore"
	ModerationActionDismiss ModerationAction = "dismiss"
)

// ModerationStatus represents the current status of a moderation case
type ModerationStatus string

const (
	ModerationStatusPending   ModerationStatus = "pending"
	ModerationStatusReviewing ModerationStatus = "reviewing"
	ModerationStatusActioned  ModerationStatus = "actioned"
	ModerationStatusAppealed  ModerationStatus = "appealed"
	ModerationStatusResolved  ModerationStatus = "resolved"
	ModerationStatusDismissed ModerationStatus = "dismissed"
)

// ModerationContentType represents the type of content being moderated
type ModerationContentType string

const (
	ModerationContentTypeStatus ModerationContentType = "status"
	ModerationContentTypeUser   ModerationContentType = "user"
	ModerationContentTypeMedia  ModerationContentType = "media"
	ModerationContentTypeReport ModerationContentType = "report"
)

// ModerationReason represents why moderation was triggered
type ModerationReason string

const (
	ModerationReasonSpam            ModerationReason = "spam"
	ModerationReasonHateSpeech      ModerationReason = "hate_speech"
	ModerationReasonHarassment      ModerationReason = "harassment"
	ModerationReasonMisinformation  ModerationReason = "misinformation"
	ModerationReasonProhibitedWords ModerationReason = "prohibited_words"
	ModerationReasonRateLimiting    ModerationReason = "rate_limiting"
	ModerationReasonCopyright       ModerationReason = "copyright"
	ModerationReasonOther           ModerationReason = "other"
)

// Moderation represents a moderation case stored in DynamoDB using DynamORM
type Moderation struct {
	// Primary key - using moderation ID as the primary identifier
	PK string `dynamorm:"pk" json:"pk"` // Format: "moderation#{moderation_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "moderation#{moderation_id}"

	// GSI1 - Content lookup (find all moderations for a specific piece of content)
	GSI1PK string `dynamorm:"index:content-moderation-index,pk" json:"gsi1_pk,omitempty"` // Format: "{content_type}#{content_id}"
	GSI1SK string `dynamorm:"index:content-moderation-index,sk" json:"gsi1_sk,omitempty"` // Format: "{created_at}#{moderation_id}"

	// GSI2 - Status queries (find all moderations with a specific status)
	GSI2PK string `dynamorm:"index:moderation-status-index,pk" json:"gsi2_pk"` // Format: "STATUS#{status}"
	GSI2SK string `dynamorm:"index:moderation-status-index,sk" json:"gsi2_sk"` // Format: "{created_at}#{moderation_id}"

	// GSI3 - Moderator queries (find all moderations by a specific moderator)
	GSI3PK string `dynamorm:"index:moderator-index,pk" json:"gsi3_pk,omitempty"` // Format: "MODERATOR#{moderator_id}"
	GSI3SK string `dynamorm:"index:moderator-index,sk" json:"gsi3_sk,omitempty"` // Format: "{created_at}#{moderation_id}"

	// GSI4 - User queries (find all moderations affecting a specific user)
	GSI4PK string `dynamorm:"index:user-moderation-index,pk" json:"gsi4_pk,omitempty"` // Format: "USER_MOD#{user_id}"
	GSI4SK string `dynamorm:"index:user-moderation-index,sk" json:"gsi4_sk,omitempty"` // Format: "{created_at}#{moderation_id}"

	// Core moderation data
	ModerationID  string                `json:"moderation_id"`
	ContentID     string                `json:"content_id"`
	ContentType   ModerationContentType `json:"content_type"`
	UserID        string                `json:"user_id"`        // User who created the content or is being moderated
	Action        ModerationAction      `json:"action"`         // Action taken
	Status        ModerationStatus      `json:"status"`         // Current status
	Reason        ModerationReason      `json:"reason"`         // Primary reason
	Evidence      ModerationEvidence    `json:"evidence"`       // Detailed evidence
	ModeratorID   string                `json:"moderator_id"`   // Human or "system" for automated
	ModeratorType string                `json:"moderator_type"` // "human", "automated", "system"
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	ActionedAt    *time.Time            `json:"actioned_at,omitempty"`
	ResolvedAt    *time.Time            `json:"resolved_at,omitempty"`

	// Appeal information
	AppealStatus   string     `json:"appeal_status,omitempty"`   // "none", "requested", "reviewing", "approved", "denied"
	AppealReason   string     `json:"appeal_reason,omitempty"`   // User's appeal text
	AppealedAt     *time.Time `json:"appealed_at,omitempty"`     // When appeal was submitted
	AppealReviewer string     `json:"appeal_reviewer,omitempty"` // Who reviewed the appeal
	AppealDecision string     `json:"appeal_decision,omitempty"` // Decision explanation

	// Audit trail
	History []ModerationHistoryEntry `json:"history"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// ModerationEvidence contains detailed evidence for the moderation decision
type ModerationEvidence struct {
	// Spam detection
	SpamScore        float64  `json:"spam_score,omitempty"`
	SpamIndicators   []string `json:"spam_indicators,omitempty"`
	DuplicateCount   int      `json:"duplicate_count,omitempty"`
	LinkCount        int      `json:"link_count,omitempty"`
	MentionCount     int      `json:"mention_count,omitempty"`
	HashtagCount     int      `json:"hashtag_count,omitempty"`
	SuspiciousLinks  []string `json:"suspicious_links,omitempty"`
	IPAddress        string   `json:"ip_address,omitempty"`
	UserAgent        string   `json:"user_agent,omitempty"`
	RegistrationAge  string   `json:"registration_age,omitempty"`
	PostFrequency    float64  `json:"post_frequency,omitempty"`  // posts per hour
	FollowerRatio    float64  `json:"follower_ratio,omitempty"`  // followers/following
	NewAccountPosts  int      `json:"new_account_posts,omitempty"` // posts in first 24h

	// Content analysis
	ProhibitedWords  []string `json:"prohibited_words,omitempty"`
	MatchedPatterns  []string `json:"matched_patterns,omitempty"`
	ContentHash      string   `json:"content_hash,omitempty"`
	MediaFingerprint string   `json:"media_fingerprint,omitempty"`
	TextContent      string   `json:"text_content,omitempty"` // Original content for review
	ContentLength    int      `json:"content_length,omitempty"`
	Language         string   `json:"language,omitempty"`
	Sentiment        float64  `json:"sentiment,omitempty"` // -1.0 to 1.0
	Toxicity         float64  `json:"toxicity,omitempty"`  // 0.0 to 1.0

	// Rate limiting
	RequestCount     int     `json:"request_count,omitempty"`
	RequestPeriod    string  `json:"request_period,omitempty"`
	BurstSize        int     `json:"burst_size,omitempty"`
	AverageInterval  float64 `json:"average_interval,omitempty"` // seconds between actions
	LastViolationAt  string  `json:"last_violation_at,omitempty"`
	ViolationCount   int     `json:"violation_count,omitempty"`

	// External signals
	ReportCount      int      `json:"report_count,omitempty"`
	ReporterIDs      []string `json:"reporter_ids,omitempty"`
	ExternalScore    float64  `json:"external_score,omitempty"`
	ExternalProvider string   `json:"external_provider,omitempty"`

	// Confidence metrics
	ConfidenceScore  float64 `json:"confidence_score,omitempty"` // 0.0 to 1.0
	FalsePositiveRisk float64 `json:"false_positive_risk,omitempty"`
	RequiresReview   bool    `json:"requires_review,omitempty"`
}

// ModerationHistoryEntry represents a single entry in the moderation audit trail
type ModerationHistoryEntry struct {
	Timestamp   time.Time        `json:"timestamp"`
	ActorID     string           `json:"actor_id"`
	ActorType   string           `json:"actor_type"` // "user", "moderator", "system"
	Action      ModerationAction `json:"action"`
	FromStatus  ModerationStatus `json:"from_status"`
	ToStatus    ModerationStatus `json:"to_status"`
	Note        string           `json:"note,omitempty"`
	ChangedData map[string]interface{} `json:"changed_data,omitempty"`
}

// TableName returns the DynamoDB table name for the Moderation model
func (Moderation) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (m *Moderation) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	// Generate moderation ID if not set
	if m.ModerationID == "" {
		m.ModerationID = fmt.Sprintf("mod_%d_%s", now.UnixNano(), generateRandomString(8))
	}

	// Set default status
	if m.Status == "" {
		m.Status = ModerationStatusPending
	}

	// Set default moderator type
	if m.ModeratorType == "" {
		if m.ModeratorID == "system" {
			m.ModeratorType = "automated"
		} else if m.ModeratorID != "" {
			m.ModeratorType = "human"
		}
	}

	// Initialize history if empty
	if m.History == nil {
		m.History = []ModerationHistoryEntry{}
	}

	// Add creation history entry
	m.History = append(m.History, ModerationHistoryEntry{
		Timestamp:  now,
		ActorID:    m.ModeratorID,
		ActorType:  m.ModeratorType,
		Action:     ModerationActionWarning, // Initial creation
		FromStatus: "",
		ToStatus:   m.Status,
		Note:       "Moderation case created",
	})

	// Set up primary key
	m.PK = "moderation#" + m.ModerationID
	m.SK = "moderation#" + m.ModerationID

	// Set up GSI keys
	m.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (m *Moderation) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Update GSI keys in case indexed fields changed
	m.setupGSIKeys()

	return nil
}

// setupGSIKeys configures all GSI partition and sort keys
func (m *Moderation) setupGSIKeys() {
	// GSI1 - Content lookup
	m.GSI1PK = fmt.Sprintf("%s#%s", m.ContentType, m.ContentID)
	m.GSI1SK = fmt.Sprintf("%s#%s", m.CreatedAt.Format(time.RFC3339), m.ModerationID)

	// GSI2 - Status queries
	m.GSI2PK = "STATUS#" + string(m.Status)
	m.GSI2SK = fmt.Sprintf("%s#%s", m.CreatedAt.Format(time.RFC3339), m.ModerationID)

	// GSI3 - Moderator queries (only if moderator is set)
	if m.ModeratorID != "" {
		m.GSI3PK = "MODERATOR#" + m.ModeratorID
		m.GSI3SK = fmt.Sprintf("%s#%s", m.CreatedAt.Format(time.RFC3339), m.ModerationID)
	} else {
		m.GSI3PK = ""
		m.GSI3SK = ""
	}

	// GSI4 - User queries (only if user is set)
	if m.UserID != "" {
		m.GSI4PK = "USER_MOD#" + m.UserID
		m.GSI4SK = fmt.Sprintf("%s#%s", m.CreatedAt.Format(time.RFC3339), m.ModerationID)
	} else {
		m.GSI4PK = ""
		m.GSI4SK = ""
	}
}

// AddHistoryEntry adds a new entry to the moderation history
func (m *Moderation) AddHistoryEntry(actorID, actorType string, action ModerationAction, fromStatus, toStatus ModerationStatus, note string) {
	entry := ModerationHistoryEntry{
		Timestamp:  time.Now(),
		ActorID:    actorID,
		ActorType:  actorType,
		Action:     action,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Note:       note,
	}
	m.History = append(m.History, entry)
}

// CanBeAppealed returns true if the moderation can be appealed
func (m *Moderation) CanBeAppealed() bool {
	// Can appeal if actioned and not already appealed
	return m.Status == ModerationStatusActioned && m.AppealStatus != "requested" && m.AppealStatus != "reviewing"
}

// IsAutomated returns true if this is an automated moderation
func (m *Moderation) IsAutomated() bool {
	return m.ModeratorType == "automated" || m.ModeratorType == "system"
}

// RequiresHumanReview returns true if this moderation needs human review
func (m *Moderation) RequiresHumanReview() bool {
	// Always require review for severe actions
	if m.Action == ModerationActionSuspend || m.Action == ModerationActionRemove {
		return true
	}
	// Check evidence confidence
	if m.Evidence.RequiresReview {
		return true
	}
	// Low confidence automated decisions need review
	if m.IsAutomated() && m.Evidence.ConfidenceScore < 0.8 {
		return true
	}
	// High false positive risk needs review
	if m.Evidence.FalsePositiveRisk > 0.3 {
		return true
	}
	return false
}

// GetSeverity returns the severity level of the moderation (1-5)
func (m *Moderation) GetSeverity() int {
	switch m.Action {
	case ModerationActionSuspend:
		return 5
	case ModerationActionRemove:
		return 4
	case ModerationActionSilence:
		return 3
	case ModerationActionWarning:
		return 2
	case ModerationActionDismiss, ModerationActionRestore:
		return 1
	default:
		return 1
	}
}

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// IsActive returns true if the moderation is still active (not resolved or dismissed)
func (m *Moderation) IsActive() bool {
	return m.Status != ModerationStatusResolved && m.Status != ModerationStatusDismissed
}

// GetPrimaryProhibitedWord returns the first prohibited word found, if any
func (m *Moderation) GetPrimaryProhibitedWord() string {
	if len(m.Evidence.ProhibitedWords) > 0 {
		return m.Evidence.ProhibitedWords[0]
	}
	return ""
}

// GetTotalReports returns the total number of reports for this content
func (m *Moderation) GetTotalReports() int {
	return m.Evidence.ReportCount
}

// HasHighSpamScore returns true if the spam score indicates likely spam
func (m *Moderation) HasHighSpamScore() bool {
	return m.Evidence.SpamScore > 0.7
}

// GetRateLimitViolationSeverity returns how severe the rate limit violation is
func (m *Moderation) GetRateLimitViolationSeverity() string {
	if m.Reason != ModerationReasonRateLimiting {
		return "none"
	}
	
	if m.Evidence.ViolationCount > 10 {
		return "severe"
	} else if m.Evidence.ViolationCount > 5 {
		return "moderate"
	} else if m.Evidence.ViolationCount > 2 {
		return "mild"
	}
	return "minor"
}