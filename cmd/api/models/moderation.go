package models

// FlagRequest represents a content flagging request
type FlagRequest struct {
	ObjectID        string  `json:"object_id"`
	ObjectType      string  `json:"object_type"` // "status", "account", "media"
	Category        string  `json:"category"`    // spam, hate_speech, harassment, etc.
	Severity        int     `json:"severity"`    // 1-4
	ConfidenceScore float64 `json:"confidence_score"`
	Reason          string  `json:"reason"`
}

// ModerationEventResponse represents a moderation event in API responses
type ModerationEventResponse struct {
	ID              string  `json:"id"`
	EventType       string  `json:"event_type"`
	ObjectID        string  `json:"object_id"`
	ObjectType      string  `json:"object_type"`
	Category        string  `json:"category"`
	Severity        int     `json:"severity"`
	ConfidenceScore float64 `json:"confidence_score"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
}

// ReviewQueueItem represents an item in the moderation review queue
type ReviewQueueItem struct {
	ID              string  `json:"id"`
	ObjectID        string  `json:"object_id"`
	ObjectType      string  `json:"object_type"`
	ObjectPreview   string  `json:"object_preview,omitempty"`
	AuthorID        string  `json:"author_id,omitempty"`
	Category        string  `json:"category"`
	Severity        int     `json:"severity"`
	ConfidenceScore float64 `json:"confidence_score"`
	PriorityScore   float64 `json:"priority_score"`
	ReportCount     int     `json:"report_count"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
}

// ConsensusReview represents a single review in consensus visualization
type ConsensusReview struct {
	ReviewerID     string  `json:"reviewer_id"`
	ReviewerDomain string  `json:"reviewer_domain,omitempty"`
	Action         string  `json:"action"`
	Confidence     float64 `json:"confidence"`
	TrustWeight    float64 `json:"trust_weight"`
	ReviewedAt     string  `json:"reviewed_at"`
}

// ConsensusVisualization represents the consensus state for a moderation event
type ConsensusVisualization struct {
	EventID         string             `json:"event_id"`
	ObjectID        string             `json:"object_id"`
	Category        string             `json:"category"`
	Severity        int                `json:"severity"`
	ConfidenceScore float64            `json:"confidence_score"`
	Reviews         []*ConsensusReview `json:"reviews"`
	ReviewerCount   int                `json:"reviewer_count"`
	ConsensusScore  float64            `json:"consensus_score,omitempty"`
	Decision        string             `json:"decision,omitempty"`
	DecidedAt       string             `json:"decided_at,omitempty"`
}

// ReviewRequest represents a moderation review submission
type ReviewRequest struct {
	EventID    string  `json:"event_id"`
	Action     string  `json:"action"`     // none, warning, silence, suspend, remove, etc.
	Category   string  `json:"category"`   // spam, hate_speech, harassment, etc.
	Severity   int     `json:"severity"`   // 1-4
	Confidence float64 `json:"confidence"` // 0.0-1.0
	Notes      string  `json:"notes,omitempty"`
}

// TrustRelationshipResponse represents a trust relationship in API responses
type TrustRelationshipResponse struct {
	ID            string  `json:"id"`
	TrusteeID     string  `json:"trustee_id"`
	TrusteeDomain string  `json:"trustee_domain,omitempty"`
	Category      string  `json:"category"`
	Score         float64 `json:"score"`
	Confidence    float64 `json:"confidence"`
	UpdatedAt     string  `json:"updated_at"`
}

// UpdateTrustRequest represents a trust relationship update request
type UpdateTrustRequest struct {
	TrusteeID     string  `json:"trustee_id"`
	TrusteeDomain string  `json:"trustee_domain,omitempty"`
	Category      string  `json:"category"`   // content, behavior, technical, general
	Score         float64 `json:"score"`      // -1.0 to 1.0
	Confidence    float64 `json:"confidence"` // 0.0 to 1.0
}

// TrustScoreResponse represents an actor's trust score in API responses
type TrustScoreResponse struct {
	ActorID      string             `json:"actor_id"`
	ActorDomain  string             `json:"actor_domain,omitempty"`
	OverallScore float64            `json:"overall_score"`
	Scores       map[string]float64 `json:"scores"`
	TrusterCount int                `json:"truster_count"`
	CalculatedAt string             `json:"calculated_at"`
}
