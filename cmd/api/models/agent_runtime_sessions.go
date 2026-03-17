package models

import "time"

// AgentRuntimeSession represents a long-lived bearer + refresh runtime session for an agent.
type AgentRuntimeSession struct {
	SessionID         string     `json:"session_id"`
	ClientID          string     `json:"client_id"`
	DeviceLabel       string     `json:"device_label"`
	Scope             string     `json:"scope"`
	CreatedAt         time.Time  `json:"created_at"`
	LastUsedAt        time.Time  `json:"last_used_at"`
	IdleExpiresAt     time.Time  `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time  `json:"absolute_expires_at"`
	Revoked           bool       `json:"revoked"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	RevokedReason     string     `json:"revoked_reason,omitempty"`
}

// RevokeAgentRuntimeSessionRequest revokes one runtime session without affecting unrelated sessions.
type RevokeAgentRuntimeSessionRequest struct {
	Reason string `json:"reason,omitempty"`
}
