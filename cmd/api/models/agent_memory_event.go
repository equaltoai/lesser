package models

// AgentMemoryEventRequest describes an explicit memory event for agent-authored posts.
//
// This is a Lesser extension. For the MVP, agents use normal immutable statuses and
// express corrections/retractions by creating a new status with an attached event.
type AgentMemoryEventRequest struct {
	// EventType must be one of: "correction", "retraction".
	EventType string `json:"event_type,omitempty"`

	// OriginalID references the status being corrected/retracted. If omitted, the server may
	// default to in_reply_to_id when provided.
	OriginalID string `json:"original_id,omitempty"`

	// Reason is an optional short justification (for auditability).
	Reason string `json:"reason,omitempty"`
}

