package models

import "time"

// AdminModerationOverviewResponse represents moderation system overview information.
type AdminModerationOverviewResponse struct {
	PendingReviews   int                       `json:"pending_reviews"`
	OpenReports      int                       `json:"open_reports"`
	ActiveModerators int                       `json:"active_moderators"`
	RecentDecisions  []AdminModerationDecision `json:"recent_decisions"`
	TrustGraphHealth AdminTrustGraphHealth     `json:"trust_graph_health"`
}

// AdminModerationDecision represents a summary of a recent moderation decision.
type AdminModerationDecision struct {
	ID         string    `json:"id"`
	EventType  string    `json:"event_type"`
	ActorID    string    `json:"actor_id"`
	Severity   string    `json:"severity"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminTrustGraphHealth represents trust graph health metrics.
type AdminTrustGraphHealth struct {
	TotalRelationships int     `json:"total_relationships"`
	AverageTrustScore  float64 `json:"average_trust_score"`
	IsolatedUsers      int     `json:"isolated_users"`
}

// AdminModerationEvent represents an event returned by moderation event listings.
type AdminModerationEvent struct {
	ID              string    `json:"id"`
	EventType       string    `json:"event_type"`
	ActorID         string    `json:"actor_id"`
	ObjectID        string    `json:"object_id"`
	ObjectType      string    `json:"object_type"`
	Category        string    `json:"category"`
	Severity        string    `json:"severity"`
	Reason          string    `json:"reason"`
	Evidence        []any     `json:"evidence,omitempty"`
	ConfidenceScore float64   `json:"confidence_score"`
	CreatedAt       time.Time `json:"created_at"`
}

// AdminModerationEventOverrideRequest represents a request to override a moderation event decision.
type AdminModerationEventOverrideRequest struct {
	Decision string `json:"decision"` // "approve" or "reject"
	Reason   string `json:"reason"`
}

// AdminModerationEventOverrideResponse represents the result of overriding a moderation event.
type AdminModerationEventOverrideResponse struct {
	EventID  string `json:"event_id"`
	Decision string `json:"decision"`
	Action   string `json:"action"`
	Override bool   `json:"override"`
	Admin    string `json:"admin"`
	Reason   string `json:"reason"`
}

// AdminTrustGraphNode represents a node in the trust graph response.
type AdminTrustGraphNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// AdminTrustGraphEdge represents an edge in the trust graph response.
type AdminTrustGraphEdge struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Trust     float64   `json:"trust"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AdminTrustGraphStats represents summary statistics for a trust graph response.
type AdminTrustGraphStats struct {
	TotalNodes int `json:"total_nodes"`
	TotalEdges int `json:"total_edges"`
}

// AdminTrustGraphResponse represents GET /api/v1/admin/moderation/trust/graph.
type AdminTrustGraphResponse struct {
	Nodes []AdminTrustGraphNode `json:"nodes"`
	Edges []AdminTrustGraphEdge `json:"edges"`
	Stats AdminTrustGraphStats  `json:"stats"`
}

// AdminUpdateTrustRequest represents a request to update a trust relationship.
type AdminUpdateTrustRequest struct {
	Trust    float64 `json:"trust"`
	Category string  `json:"category,omitempty"`
	Reason   string  `json:"reason"`
}

// AdminUpdateTrustResponse represents the result of updating a trust relationship.
type AdminUpdateTrustResponse struct {
	FromActorID string    `json:"from_actor_id"`
	ToActorID   string    `json:"to_actor_id"`
	Trust       float64   `json:"trust"`
	Category    string    `json:"category"`
	UpdatedBy   string    `json:"updated_by"`
	Reason      string    `json:"reason"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AdminReviewer represents a reviewer/moderator user returned by reviewer listings.
type AdminReviewer struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Role            string    `json:"role"`
	TotalReviews    int       `json:"total_reviews"`
	AccurateReviews int       `json:"accurate_reviews"`
	AccuracyRate    float64   `json:"accuracy_rate"`
	LastReviewAt    time.Time `json:"last_review_at"`
}

// AdminModerationReviewersResponse represents GET /api/v1/admin/moderation/reviewers.
type AdminModerationReviewersResponse struct {
	Reviewers []AdminReviewer `json:"reviewers"`
	Total     int             `json:"total"`
}

// AdminPromoteModeratorResponse represents the response when promoting a user to moderator.
type AdminPromoteModeratorResponse struct {
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	NewRole    string `json:"new_role"`
	PromotedBy string `json:"promoted_by"`
}

// AdminDemoteModeratorResponse represents the response when demoting a moderator to user.
type AdminDemoteModeratorResponse struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	NewRole   string `json:"new_role"`
	DemotedBy string `json:"demoted_by"`
}
