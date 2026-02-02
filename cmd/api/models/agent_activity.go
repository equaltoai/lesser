package models

import "time"

// AgentActivityLogEntry represents a single audited agent action.
type AgentActivityLogEntry struct {
	AgentUsername string    `json:"agent_username"`
	Action        string    `json:"action"`
	TargetID      string    `json:"target_id,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Metadata      any       `json:"metadata,omitempty"`
}

// AgentActivityLogList is the response payload for GET /api/v1/agents/{username}/activity.
type AgentActivityLogList []AgentActivityLogEntry
