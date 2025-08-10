package models

import (
	"fmt"
	"time"
)

// ModerationAction represents the type of moderation action taken
type ModerationAction string

const (
	// ModerationActionRemove represents a remove moderation action
	ModerationActionRemove ModerationAction = "remove"
	// ModerationActionSuspend represents a suspend moderation action
	ModerationActionSuspend ModerationAction = "suspend"
	// ModerationActionSilence represents a silence moderation action
	ModerationActionSilence ModerationAction = "silence"
	// ModerationActionWarning represents a warning moderation action
	ModerationActionWarning ModerationAction = "warning"
	// ModerationActionRestore represents a restore moderation action
	ModerationActionRestore ModerationAction = "restore"
	// ModerationActionDismiss represents a dismiss moderation action
	ModerationActionDismiss ModerationAction = "dismiss"
)

// ModerationStatus represents the current status of a moderation case
type ModerationStatus string

const (
	// ModerationStatusPending represents a pending moderation status
	ModerationStatusPending ModerationStatus = "pending"
	// ModerationStatusReviewing represents a reviewing moderation status
	ModerationStatusReviewing ModerationStatus = "reviewing"
	// ModerationStatusActioned represents an actioned moderation status
	ModerationStatusActioned ModerationStatus = "actioned"
	// ModerationStatusAppealed represents an appealed moderation status
	ModerationStatusAppealed ModerationStatus = "appealed"
	// ModerationStatusResolved represents a resolved moderation status
	ModerationStatusResolved ModerationStatus = "resolved"
	// ModerationStatusDismissed represents a dismissed moderation status
	ModerationStatusDismissed ModerationStatus = "dismissed"
)

// ModerationContentType represents the type of content being moderated
type ModerationContentType string

const (
	// ModerationContentTypeStatus represents status content type
	ModerationContentTypeStatus ModerationContentType = "status"
	// ModerationContentTypeUser represents user content type
	ModerationContentTypeUser ModerationContentType = "user"
	// ModerationContentTypeMedia represents media content type
	ModerationContentTypeMedia ModerationContentType = "media"
	// ModerationContentTypeReport represents report content type
	ModerationContentTypeReport ModerationContentType = "report"
)

// ModerationReason represents why moderation was triggered
type ModerationReason string

const (
	// ModerationReasonSpam represents spam moderation reason
	ModerationReasonSpam ModerationReason = "spam"
	// ModerationReasonHateSpeech represents hate speech moderation reason
	ModerationReasonHateSpeech ModerationReason = "hate_speech"
	// ModerationReasonHarassment represents harassment moderation reason
	ModerationReasonHarassment ModerationReason = "harassment"
	// ModerationReasonMisinformation represents misinformation moderation reason
	ModerationReasonMisinformation ModerationReason = "misinformation"
	// ModerationReasonProhibitedWords represents prohibited words moderation reason
	ModerationReasonProhibitedWords ModerationReason = "prohibited_words"
	// ModerationReasonRateLimiting represents rate limiting moderation reason
	ModerationReasonRateLimiting ModerationReason = "rate_limiting"
	// ModerationReasonCopyright represents copyright moderation reason
	ModerationReasonCopyright ModerationReason = "copyright"
	// ModerationReasonOther represents other/miscellaneous moderation reason
	ModerationReasonOther ModerationReason = "other"
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
	return MainTableName
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationEvent) UpdateKeys() {
	// Primary key - events by object
	m.PK = fmt.Sprintf("EVENT#%s", m.ObjectID)
	m.SK = fmt.Sprintf("TIME#%s#%s", m.Created.Format(time.RFC3339), m.ID)

	// GSI1 - Actor queries
	m.GSI1PK = fmt.Sprintf(KeyPatternActor, m.ActorID)
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
	ID          string                 `json:"id"`
	EventID     string                 `json:"event_id"`
	ReviewerID  string                 `json:"reviewer_id"`
	ReviewerRep float64                `json:"reviewer_rep,omitempty"`
	Action      string                 `json:"action"`   // none, remove, silence, suspend, warning
	Severity    string                 `json:"severity"` // low, medium, high, critical
	Note        string                 `json:"note,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Confidence  float64                `json:"confidence"` // 0.0-1.0
	Created     time.Time              `json:"created"`

	// DynamoDB TTL
	TTL       int64     `dynamorm:"ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table name
func (ModerationReview) TableName() string {
	return MainTableName
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
	Action           string                 `json:"action"`          // none, remove, silence, suspend, warning
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
	return MainTableName
}

// UpdateKeys updates the keys based on current field values
func (d *ModerationDecision) UpdateKeys() {
	d.PK = fmt.Sprintf("DECISION#%s", d.ObjectID)
	d.SK = fmt.Sprintf("TIME#%s", d.Decided.Format(time.RFC3339))
	d.GSI1PK = "ACTIVE_DECISIONS"
	d.GSI1SK = fmt.Sprintf(KeyPatternObject, d.ObjectID)
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
	PK string `dynamorm:"pk" json:"pk"` // Format: "PATTERN#{pattern_id}"
	SK string `dynamorm:"sk" json:"sk"` // "METADATA"

	// GSI1 - Active pattern queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // "MODERATION_PATTERNS#ACTIVE" (when active)
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // "{severity}#{type}#{pattern_id}"

	// GSI2 - Severity-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // "MODERATION_PATTERNS#{severity}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // "{updated_at}#{pattern_id}"

	// Pattern data
	PatternID   string    `json:"pattern_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`     // "regex", "keyword", "phrase"
	Pattern     string    `json:"pattern"`
	Category    string    `json:"category"` // "toxicity", "spam", "violence", etc.
	Severity    float64   `json:"severity"` // 0.0 to 1.0
	Active      bool      `json:"active"`
	Flags       []string  `json:"flags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HitCount    int64     `json:"hit_count"`
	LastHit     time.Time `json:"last_hit,omitempty"`

	// TTL for auto-cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationPattern) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys based on current field values
func (p *ModerationPattern) UpdateKeys() {
	p.PK = fmt.Sprintf("PATTERN#%s", p.PatternID)
	p.SK = "METADATA"

	// GSI1 - Active patterns
	if p.Active {
		p.GSI1PK = "MODERATION_PATTERNS#ACTIVE"
		p.GSI1SK = fmt.Sprintf("%.2f#%s#%s", p.Severity, p.Type, p.PatternID)
	} else {
		p.GSI1PK = ""
		p.GSI1SK = ""
	}

	// GSI2 - Severity queries
	p.GSI2PK = fmt.Sprintf("MODERATION_PATTERNS#%.2f", p.Severity)
	p.GSI2SK = fmt.Sprintf("%s#%s", p.UpdatedAt.Format(time.RFC3339), p.PatternID)

	// Set TTL (90 days)
	if p.TTL == 0 {
		p.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
}

// ModerationEvidence contains detailed evidence for the moderation decision
type ModerationEvidence struct {
	// Spam detection
	SpamScore       float64  `json:"spam_score,omitempty"`
	SpamIndicators  []string `json:"spam_indicators,omitempty"`
	DuplicateCount  int      `json:"duplicate_count,omitempty"`
	LinkCount       int      `json:"link_count,omitempty"`
	MentionCount    int      `json:"mention_count,omitempty"`
	HashtagCount    int      `json:"hashtag_count,omitempty"`
	SuspiciousLinks []string `json:"suspicious_links,omitempty"`
	IPAddress       string   `json:"ip_address,omitempty"`
	UserAgent       string   `json:"user_agent,omitempty"`
	RegistrationAge string   `json:"registration_age,omitempty"`
	PostFrequency   float64  `json:"post_frequency,omitempty"`    // posts per hour
	FollowerRatio   float64  `json:"follower_ratio,omitempty"`    // followers/following
	NewAccountPosts int      `json:"new_account_posts,omitempty"` // posts in first 24h

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
	RequestCount    int     `json:"request_count,omitempty"`
	RequestPeriod   string  `json:"request_period,omitempty"`
	BurstSize       int     `json:"burst_size,omitempty"`
	AverageInterval float64 `json:"average_interval,omitempty"` // seconds between actions
	LastViolationAt string  `json:"last_violation_at,omitempty"`
	ViolationCount  int     `json:"violation_count,omitempty"`

	// External signals
	ReportCount      int      `json:"report_count,omitempty"`
	ReporterIDs      []string `json:"reporter_ids,omitempty"`
	ExternalScore    float64  `json:"external_score,omitempty"`
	ExternalProvider string   `json:"external_provider,omitempty"`

	// Confidence metrics
	ConfidenceScore   float64 `json:"confidence_score,omitempty"` // 0.0 to 1.0
	FalsePositiveRisk float64 `json:"false_positive_risk,omitempty"`
	RequiresReview    bool    `json:"requires_review,omitempty"`
}

// ModerationHistoryEntry represents a single entry in the moderation audit trail
type ModerationHistoryEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	ActorID     string                 `json:"actor_id"`
	ActorType   string                 `json:"actor_type"` // "user", "moderator", "system"
	Action      ModerationAction       `json:"action"`
	FromStatus  ModerationStatus       `json:"from_status"`
	ToStatus    ModerationStatus       `json:"to_status"`
	Note        string                 `json:"note,omitempty"`
	ChangedData map[string]interface{} `json:"changed_data,omitempty"`
}

// Moderation represents an active moderation case being processed
type Moderation struct {
	ModerationID  string                   `json:"moderation_id"`
	ContentID     string                   `json:"content_id"`
	ContentType   ModerationContentType    `json:"content_type"`
	UserID        string                   `json:"user_id"`
	Status        ModerationStatus         `json:"status"`
	Action        ModerationAction         `json:"action"`
	Reason        ModerationReason         `json:"reason"`
	Evidence      ModerationEvidence       `json:"evidence"`
	ModeratorID   string                   `json:"moderator_id"`
	ModeratorType string                   `json:"moderator_type"` // "user", "automated", "admin"
	Metadata      map[string]interface{}   `json:"metadata,omitempty"`
	History       []ModerationHistoryEntry `json:"history,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	ActionedAt    *time.Time               `json:"actioned_at,omitempty"`
	ResolvedAt    *time.Time               `json:"resolved_at,omitempty"`
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

// ModerationAnalysisResult stores detailed analysis results for audit/appeals
type ModerationAnalysisResult struct {
	// Primary key - analysis results by content
	PK string `dynamorm:"pk" json:"pk"` // Format: "ANALYSIS#{content_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "RESULT#{timestamp}#{analysis_id}"

	// GSI1 - Author queries (find analyses by content author)
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "AUTHOR#{author_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Analysis type queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "ANALYSIS_TYPE#{type}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "CONFIDENCE#{confidence}#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "ANALYSIS_RESULT"

	// Analysis result data
	ID              string                 `json:"id"`
	ContentID       string                 `json:"content_id"`
	ContentType     string                 `json:"content_type"` // text, image, video
	AuthorID        string                 `json:"author_id"`
	AnalysisType    string                 `json:"analysis_type"` // text, image, video, combined
	Confidence      float64                `json:"confidence"`
	Results         map[string]interface{} `json:"results"` // Full analysis results
	PatternMatches  []interface{}          `json:"pattern_matches,omitempty"`
	ThreatMatches   []interface{}          `json:"threat_matches,omitempty"`
	ReputationScore interface{}            `json:"reputation_score,omitempty"`
	ProcessingTime  int64                  `json:"processing_time"` // milliseconds
	AnalyzedAt      time.Time              `json:"analyzed_at"`
	CreatedAt       time.Time              `json:"created_at"`

	// DynamoDB TTL (90 days for analysis results)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationAnalysisResult) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationAnalysisResult) UpdateKeys() {
	// Primary key - analysis results by content
	m.PK = fmt.Sprintf("ANALYSIS#%s", m.ContentID)
	m.SK = fmt.Sprintf("RESULT#%s#%s", m.AnalyzedAt.Format(time.RFC3339), m.ID)

	// GSI1 - Author queries
	m.GSI1PK = fmt.Sprintf(KeyPatternActor, m.AuthorID)
	m.GSI1SK = fmt.Sprintf("TIME#%s", m.AnalyzedAt.Format(time.RFC3339))

	// GSI2 - Analysis type queries
	confidenceStr := fmt.Sprintf("%03d", int(m.Confidence*100))
	m.GSI2PK = fmt.Sprintf("ANALYSIS_TYPE#%s", m.AnalysisType)
	m.GSI2SK = fmt.Sprintf("CONFIDENCE#%s#%s", confidenceStr, m.AnalyzedAt.Format(time.RFC3339))

	// Set type marker
	m.Type = "ANALYSIS_RESULT"
	m.CreatedAt = m.AnalyzedAt

	// Set TTL (90 days default)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
}

// ModerationDecisionResult stores enhanced decision results with enforcement tracking
type ModerationDecisionResult struct {
	// Primary key - decisions by content
	PK string `dynamorm:"pk" json:"pk"` // Format: "DECISION_RESULT#{content_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TIME#{RFC3339}#{decision_id}"

	// GSI1 - Active decisions lookup
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // "ACTIVE_DECISIONS"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // "CONTENT#{content_id}"

	// GSI2 - Action type queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "ACTION#{action}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "CONFIDENCE#{confidence}#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "DECISION_RESULT"

	// Decision data
	ID                string                 `json:"id"`
	ContentID         string                 `json:"content_id"`
	AuthorID          string                 `json:"author_id"`
	Action            string                 `json:"action"` // allow, flag, quarantine, remove, shadow_ban
	Confidence        float64                `json:"confidence"`
	Reasons           []interface{}          `json:"reasons"`
	RequiresReview    bool                   `json:"requires_review"`
	ReviewPriority    int                    `json:"review_priority"`
	Recommendations   []string               `json:"recommendations,omitempty"`
	ExpiresAt         *time.Time             `json:"expires_at,omitempty"`
	DecidedAt         time.Time              `json:"decided_at"`
	EnforcementStatus string                 `json:"enforcement_status"` // pending, applied, failed, expired
	EnforcedAt        *time.Time             `json:"enforced_at,omitempty"`
	EnforcementError  string                 `json:"enforcement_error,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`

	// DynamoDB TTL (90 days for decisions)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationDecisionResult) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationDecisionResult) UpdateKeys() {
	// Primary key - decision results by content
	m.PK = fmt.Sprintf("DECISION_RESULT#%s", m.ContentID)
	m.SK = fmt.Sprintf("TIME#%s#%s", m.DecidedAt.Format(time.RFC3339), m.ID)

	// GSI1 - Active decisions lookup
	if m.EnforcementStatus == "pending" || m.EnforcementStatus == "applied" {
		m.GSI1PK = "ACTIVE_DECISIONS"
		m.GSI1SK = fmt.Sprintf("CONTENT#%s", m.ContentID)
	} else {
		m.GSI1PK = ""
		m.GSI1SK = ""
	}

	// GSI2 - Action type queries
	confidenceStr := fmt.Sprintf("%03d", int(m.Confidence*100))
	m.GSI2PK = fmt.Sprintf("ACTION#%s", m.Action)
	m.GSI2SK = fmt.Sprintf("CONFIDENCE#%s#%s", confidenceStr, m.DecidedAt.Format(time.RFC3339))

	// Set type marker
	m.Type = "DECISION_RESULT"
	m.CreatedAt = m.DecidedAt

	// Set TTL (90 days default)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
}

// ModerationReviewQueue represents items in the review queue
type ModerationReviewQueue struct {
	// Primary key - queue items by status and priority
	PK string `dynamorm:"pk" json:"pk"` // Format: "REVIEW_QUEUE#{status}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "PRIORITY#{priority}#{RFC3339}#{item_id}"

	// GSI1 - Content lookups
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "QUEUE_CONTENT#{content_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "STATUS#{status}"

	// GSI2 - Assignee queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "ASSIGNEE#{assignee_id}" (when assigned)
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "PRIORITY#{priority}#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "REVIEW_QUEUE"

	// Queue item data
	ID             string                 `json:"id"`
	ContentID      string                 `json:"content_id"`
	AuthorID       string                 `json:"author_id"`
	Status         string                 `json:"status"`   // pending, assigned, reviewing, completed, dismissed
	Priority       int                    `json:"priority"` // 1-10, higher is more urgent
	AssignedTo     string                 `json:"assigned_to,omitempty"`
	AssignedAt     *time.Time             `json:"assigned_at,omitempty"`
	Category       string                 `json:"category"`
	Severity       string                 `json:"severity"`
	Reason         string                 `json:"reason"`
	Evidence       map[string]interface{} `json:"evidence,omitempty"`
	Deadline       *time.Time             `json:"deadline,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	ReviewCount    int                    `json:"review_count"`
	LastReviewedAt *time.Time             `json:"last_reviewed_at,omitempty"`

	// DynamoDB TTL (30 days for queue items)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (ModerationReviewQueue) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationReviewQueue) UpdateKeys() {
	// Primary key - queue items by status and priority
	priorityStr := fmt.Sprintf("%02d", m.Priority)
	m.PK = fmt.Sprintf("REVIEW_QUEUE#%s", m.Status)
	m.SK = fmt.Sprintf("PRIORITY#%s#%s#%s", priorityStr, m.CreatedAt.Format(time.RFC3339), m.ID)

	// GSI1 - Content lookups
	m.GSI1PK = fmt.Sprintf("QUEUE_CONTENT#%s", m.ContentID)
	m.GSI1SK = fmt.Sprintf("STATUS#%s", m.Status)

	// GSI2 - Assignee queries (only when assigned)
	if m.AssignedTo != "" {
		m.GSI2PK = fmt.Sprintf("ASSIGNEE#%s", m.AssignedTo)
		m.GSI2SK = fmt.Sprintf("PRIORITY#%s#%s", priorityStr, m.CreatedAt.Format(time.RFC3339))
	} else {
		m.GSI2PK = ""
		m.GSI2SK = ""
	}

	// Set type marker
	m.Type = "REVIEW_QUEUE"

	// Set TTL (30 days default)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
}

// AuditLog represents an audit trail entry for admin actions
type AuditLog struct {
	// Primary key - audit logs by timestamp
	PK string `dynamorm:"pk" json:"pk"` // Format: "AUDIT_LOG"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TIME#{RFC3339}#{log_id}"

	// GSI1 - Actor queries (find actions by who performed them)
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "ADMIN#{admin_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Target queries (find actions on specific targets)
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "TARGET#{target_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "ACTION#{action}#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "AUDIT_LOG"

	// Audit data
	ID          string    `json:"id"`
	AdminID     string    `json:"admin_id"`         // Who performed the action
	AdminRole   string    `json:"admin_role"`       // admin or moderator
	Action      string    `json:"action"`           // suspend, silence, resolve_report, etc.
	TargetType  string    `json:"target_type"`      // account, status, report, domain
	TargetID    string    `json:"target_id"`        // ID of the target
	Reason      string    `json:"reason,omitempty"` // Reason for action
	Details     any       `json:"details,omitempty"`// Additional details
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	CreatedAt   time.Time `json:"created_at"`

	// DynamoDB TTL - audit logs expire after 2 years
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (AuditLog) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on current field values
func (a *AuditLog) UpdateKeys() {
	// Primary key - audit logs by time
	a.PK = "AUDIT_LOG"
	a.SK = fmt.Sprintf("TIME#%s#%s", a.Timestamp.Format(time.RFC3339), a.ID)

	// GSI1 - Admin queries
	a.GSI1PK = fmt.Sprintf("ADMIN#%s", a.AdminID)
	a.GSI1SK = fmt.Sprintf("TIME#%s", a.Timestamp.Format(time.RFC3339))

	// GSI2 - Target queries
	a.GSI2PK = fmt.Sprintf("TARGET#%s", a.TargetID)
	a.GSI2SK = fmt.Sprintf("ACTION#%s#%s", a.Action, a.Timestamp.Format(time.RFC3339))

	// Set type marker
	a.Type = "AUDIT_LOG"
	a.CreatedAt = a.Timestamp

	// Set TTL (2 years)
	if a.TTL == 0 {
		a.TTL = time.Now().Add(2 * 365 * 24 * time.Hour).Unix()
	}
}
