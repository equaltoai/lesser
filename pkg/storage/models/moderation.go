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

// ModerationEvent represents a moderation event stored in DynamoDB
type ModerationEvent struct {
	// Primary key - Events are stored by object being moderated
	PK string `dynamorm:"pk" json:"pk"` // Format: "EVENT#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TIME#{RFC3339}#{event_id}"

	// GSI1 - Actor queries (find events by who created the content)
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "ACTOR#{actor_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Type/Category/Severity queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "TYPE#{event_type}#{category}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "SEVERITY#{severity}#{RFC3339}"

	// GSI3 - Event ID lookups
	GSI3PK string `dynamorm:"index:gsi3,pk" json:"gsi3_pk,omitempty"` // Format: "EVENTID#{event_id}"
	GSI3SK string `dynamorm:"index:gsi3,sk" json:"gsi3_sk,omitempty"` // Format: "EVENTID#{event_id}"

	// Type marker
	Type string `json:"type"` // "EVENT"

	// ModerationEvent fields (copied to avoid circular import)
	ID              string    `json:"id"`
	EventType       string    `json:"event_type"`
	ObjectID        string    `json:"object_id"`   // ID of content being moderated
	ObjectType      string    `json:"object_type"` // status, account, media
	ActorID         string    `json:"actor_id"`    // Who triggered this event
	Category        string    `json:"category"`
	Severity        string    `json:"severity"`
	ConfidenceScore float64   `json:"confidence_score"` // 0.0-1.0
	Evidence        []any     `json:"evidence"`
	Reason          string    `json:"reason,omitempty"` // Human-provided reason
	Created         time.Time `json:"created"`
	Updated         time.Time `json:"updated"`

	// DynamoDB TTL
	TTL       int64     `dynamorm:"ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name
func (ModerationEvent) TableName() string {
	return "lesser-main"
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationEvent) UpdateKeys() {
	// Primary key - events by object
	m.PK = fmt.Sprintf("EVENT#%s", m.ObjectID)
	m.SK = fmt.Sprintf("TIME#%s#%s", m.Created.Format(time.RFC3339), m.ID)

	// GSI1 - Actor queries
	m.GSI1PK = fmt.Sprintf("ACTOR#%s", m.ActorID)
	m.GSI1SK = fmt.Sprintf("TIME#%s", m.Created.Format(time.RFC3339))

	// GSI2 - Type/Category queries
	m.GSI2PK = fmt.Sprintf("TYPE#%s#%s", m.EventType, m.Category)
	m.GSI2SK = fmt.Sprintf("SEVERITY#%s#%s", m.Severity, m.Created.Format(time.RFC3339))

	// GSI3 - Event ID lookup
	m.GSI3PK = fmt.Sprintf("EVENTID#%s", m.ID)
	m.GSI3SK = fmt.Sprintf("EVENTID#%s", m.ID)

	// Set type marker
	m.Type = "EVENT"

	// Set TTL if not already set (30 days default)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
}

// ModerationReview represents a review by a moderator
type ModerationReview struct {
	// Primary key - reviews by event
	PK string `dynamorm:"pk" json:"pk"` // Format: "REVIEW#{event_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "REVIEWER#{reviewer_id}"

	// Type marker
	Type string `json:"type"` // "REVIEW"

	// Review fields (copied to avoid circular import)
	ID          string    `json:"id"`
	EventID     string    `json:"event_id"`
	ReviewerID  string    `json:"reviewer_id"`
	ReviewerRep float64   `json:"reviewer_rep,omitempty"`
	Action      string    `json:"action"` // none, remove, silence, suspend, warning
	Severity    string    `json:"severity"` // low, medium, high, critical
	Note        string    `json:"note,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Confidence  float64   `json:"confidence"` // 0.0-1.0
	Created     time.Time `json:"created"`

	// DynamoDB TTL
	TTL       int64     `dynamorm:"ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name
func (ModerationReview) TableName() string {
	return "lesser-main"
}

// UpdateKeys updates the keys based on current field values
func (r *ModerationReview) UpdateKeys() {
	r.PK = fmt.Sprintf("REVIEW#%s", r.EventID)
	r.SK = fmt.Sprintf("REVIEWER#%s", r.ReviewerID)
	r.Type = "REVIEW"
	r.CreatedAt = r.Created

	// Set TTL (30 days)
	if r.TTL == 0 {
		r.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
}

// ModerationDecision represents a consensus decision
type ModerationDecision struct {
	// Primary key - decisions by object
	PK string `dynamorm:"pk" json:"pk"` // Format: "DECISION#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TIME#{RFC3339}"

	// GSI1 - Active decisions lookup
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // "ACTIVE_DECISIONS"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // "OBJECT#{object_id}"

	// Type marker
	Type string `json:"type"` // "DECISION"

	// ModerationDecision fields (copied to avoid circular import)
	ID               string                 `json:"id"`
	EventID          string                 `json:"event_id"`
	ObjectID         string                 `json:"object_id"`
	Action           string                 `json:"action"` // none, remove, silence, suspend, warning
	ConsensusScore   float64                `json:"consensus_score"` // 0.0-1.0
	ReviewerCount    int                    `json:"reviewer_count"`
	TrustWeightTotal float64                `json:"trust_weight_total"`
	Reviews          []interface{}          `json:"reviews,omitempty"` // Array of Review objects
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Decided          time.Time              `json:"decided"`
	Expires          *time.Time             `json:"expires,omitempty"`

	// DynamoDB TTL
	TTL       int64     `dynamorm:"ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name
func (ModerationDecision) TableName() string {
	return "lesser-main"
}

// UpdateKeys updates the keys based on current field values
func (d *ModerationDecision) UpdateKeys() {
	d.PK = fmt.Sprintf("DECISION#%s", d.ObjectID)
	d.SK = fmt.Sprintf("TIME#%s", d.Decided.Format(time.RFC3339))
	d.GSI1PK = "ACTIVE_DECISIONS"
	d.GSI1SK = fmt.Sprintf("OBJECT#%s", d.ObjectID)
	d.Type = "DECISION"
	d.CreatedAt = d.Decided

	// Set TTL (90 days)
	if d.TTL == 0 {
		d.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
}

// ModerationPattern represents a moderation pattern
type ModerationPattern struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "MODERATION_PATTERN#{pattern_id}"
	SK string `dynamorm:"sk" json:"sk"` // "PATTERN"

	// GSI1 - Active pattern queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // "MODERATION_PATTERNS#ACTIVE" (when active)
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // "{severity}#{type}#{pattern_id}"

	// GSI2 - Severity-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // "MODERATION_PATTERNS#{severity}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // "{updated_at}#{pattern_id}"

	// Pattern data
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "regex", "keyword", "ai"
	Pattern     string    `json:"pattern"`
	Severity    string    `json:"severity"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastMatch   time.Time `json:"last_match,omitempty"`

	// TTL for auto-cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationPattern) TableName() string {
	return "lesser-main"
}

// UpdateKeys updates the keys based on current field values
func (p *ModerationPattern) UpdateKeys() {
	p.PK = fmt.Sprintf("MODERATION_PATTERN#%s", p.ID)
	p.SK = "PATTERN"

	// GSI1 - Active patterns
	if p.Active {
		p.GSI1PK = "MODERATION_PATTERNS#ACTIVE"
		p.GSI1SK = fmt.Sprintf("%s#%s#%s", p.Severity, p.Type, p.ID)
	} else {
		p.GSI1PK = ""
		p.GSI1SK = ""
	}

	// GSI2 - Severity queries
	p.GSI2PK = fmt.Sprintf("MODERATION_PATTERNS#%s", p.Severity)
	p.GSI2SK = fmt.Sprintf("%s#%s", p.UpdatedAt.Format(time.RFC3339), p.ID)

	// Set TTL (90 days)
	if p.TTL == 0 {
		p.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
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

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// Moderation represents an active moderation case being processed
type Moderation struct {
	ModerationID  string                    `json:"moderation_id"`
	ContentID     string                    `json:"content_id"`
	ContentType   ModerationContentType     `json:"content_type"`
	UserID        string                    `json:"user_id"`
	Status        ModerationStatus          `json:"status"`
	Action        ModerationAction          `json:"action"`
	Reason        ModerationReason          `json:"reason"`
	Evidence      ModerationEvidence        `json:"evidence"`
	ModeratorID   string                    `json:"moderator_id"`
	ModeratorType string                    `json:"moderator_type"` // "user", "automated", "admin"
	Metadata      map[string]interface{}    `json:"metadata,omitempty"`
	History       []ModerationHistoryEntry  `json:"history,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	ActionedAt    *time.Time                `json:"actioned_at,omitempty"`
	ResolvedAt    *time.Time                `json:"resolved_at,omitempty"`
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

// GetRateLimitViolationSeverity determines the severity of rate limit violations
func (m *Moderation) GetRateLimitViolationSeverity() string {
	if m.Evidence.ViolationCount > 10 || m.Evidence.RequestCount > 1000 {
		return "severe"
	}
	if m.Evidence.ViolationCount > 5 || m.Evidence.RequestCount > 500 {
		return "moderate"
	}
	return "minor"
}

// GetPrimaryProhibitedWord returns the first prohibited word found, or empty string
func (m *Moderation) GetPrimaryProhibitedWord() string {
	if len(m.Evidence.ProhibitedWords) > 0 {
		return m.Evidence.ProhibitedWords[0]
	}
	return ""
}