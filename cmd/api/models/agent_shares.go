package models

import "time"

// AgentShareGrant is the REST representation of one per-agent share-list entry.
type AgentShareGrant struct {
	AgentUsername   string     `json:"agent_username"`
	GranteeUsername string     `json:"grantee_username"`
	GrantedBy       string     `json:"granted_by"`
	GrantedAt       time.Time  `json:"granted_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RevokedBy       string     `json:"revoked_by,omitempty"`
	Active          bool       `json:"active"`
}

// AgentShareGrantListResponse wraps owner and grantee discovery lists.
type AgentShareGrantListResponse struct {
	Grants []AgentShareGrant `json:"grants"`
}
