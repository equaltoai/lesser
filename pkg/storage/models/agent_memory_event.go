package models

import (
	"fmt"
	"strings"
	"time"
)

// Memory event types for agent timeline-as-memory indexing.
const (
	MemoryEventCreate     = "create"
	MemoryEventCorrection = "correction"
	MemoryEventRetraction = "retraction"
	MemoryEventTombstone  = "tombstone"
)

// AgentMemoryEvent captures the event-sourced "timeline-as-memory" semantics for agent accounts.
//
// Events are immutable and are keyed by the "original" status ID so retrieval can resolve the
// latest truth (e.g., corrections/retractions) without requiring silent edits.
type AgentMemoryEvent struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"` // MEMORY#{original_status_id}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // EVENT#{timestamp}#{event_id}

	// GSI1 supports querying events by agent (auditing/debugging) without scanning.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"` // AGENT#{agent_username}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"` // EVENT#{timestamp}#{event_id}

	EventID       string    `theorydb:"attr:eventID" json:"event_id"`
	EventType     string    `theorydb:"attr:eventType" json:"event_type"`
	StatusID      string    `theorydb:"attr:statusID" json:"status_id"`
	OriginalID    string    `theorydb:"attr:originalID" json:"original_id"`
	AgentUsername string    `theorydb:"attr:agentUsername" json:"agent_username"`
	Reason        string    `theorydb:"attr:reason" json:"reason,omitempty"`
	Timestamp     time.Time `theorydb:"attr:timestamp" json:"timestamp"`
	CreatedAt     time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing AgentMemoryEvent.
func (AgentMemoryEvent) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the event for persistence and derives all index keys.
func (e *AgentMemoryEvent) BeforeCreate() error {
	now := time.Now().UTC()
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}

	e.EventID = strings.TrimSpace(e.EventID)
	e.EventType = strings.ToLower(strings.TrimSpace(e.EventType))
	e.StatusID = strings.TrimSpace(e.StatusID)
	e.OriginalID = strings.TrimSpace(e.OriginalID)
	e.AgentUsername = strings.TrimSpace(e.AgentUsername)

	if e.EventID == "" || e.EventType == "" || e.StatusID == "" || e.OriginalID == "" || e.AgentUsername == "" {
		return fmt.Errorf("missing required fields")
	}

	e.PK = fmt.Sprintf("MEMORY#%s", e.OriginalID)
	e.SK = fmt.Sprintf("EVENT#%s#%s", e.Timestamp.Format(time.RFC3339Nano), e.EventID)

	e.GSI1PK = fmt.Sprintf("AGENT#%s", e.AgentUsername)
	e.GSI1SK = e.SK

	return nil
}
