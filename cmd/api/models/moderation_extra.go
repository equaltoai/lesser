package models

import "github.com/equaltoai/lesser/pkg/storage"

// ModerationReviewResponse represents the response from POST /api/v1/moderation/review.
type ModerationReviewResponse struct {
	ReviewID   string `json:"review_id"`
	EventID    string `json:"event_id"`
	Action     string `json:"action"`
	ReviewedAt string `json:"reviewed_at"`
}

// ModerationHistoryTimelineEntry represents a timeline entry in GET /api/v1/moderation/history/{object_id}.
type ModerationHistoryTimelineEntry struct {
	Timestamp string                      `json:"timestamp"`
	Type      string                      `json:"type"`
	Event     *storage.ModerationEvent    `json:"event,omitempty"`
	Decision  *storage.ModerationDecision `json:"decision,omitempty"`
}

// ModerationHistoryResponse represents the response from GET /api/v1/moderation/history/{object_id}.
type ModerationHistoryResponse struct {
	ObjectID      string                           `json:"object_id"`
	Events        []storage.ModerationEvent        `json:"events"`
	Decisions     []storage.ModerationDecision     `json:"decisions"`
	Timeline      []ModerationHistoryTimelineEntry `json:"timeline"`
	CurrentStatus string                           `json:"current_status"`
	LastUpdated   string                           `json:"last_updated"`
}
