package storage

import (
	"strings"
	"time"
)

const (
	// AgentQuarantineStatusQuarantined marks an agent as currently quarantined.
	AgentQuarantineStatusQuarantined = "quarantined"
	// AgentQuarantineStatusApproved marks an agent as approved to operate.
	AgentQuarantineStatusApproved = "approved"
)

// AgentGovernanceState stores typed governance state for an agent account.
// It is intentionally separate from User.Metadata so core account hydration
// does not depend on extension-state decoding.
type AgentGovernanceState struct {
	Username string `json:"username"`

	QuarantineStatus     string     `json:"quarantine_status,omitempty"`
	QuarantineStart      *time.Time `json:"quarantine_start,omitempty"`
	QuarantineEnd        *time.Time `json:"quarantine_end,omitempty"`
	QuarantineApprovedBy string     `json:"quarantine_approved_by,omitempty"`
	QuarantineApprovedAt *time.Time `json:"quarantine_approved_at,omitempty"`

	DelegatedScopes []string `json:"delegated_scopes,omitempty"`
	SelfScopes      []string `json:"self_scopes,omitempty"`
	SelfSovereign   bool     `json:"self_sovereign,omitempty"`

	Verified         bool       `json:"verified"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	VerifiedBy       string     `json:"verified_by,omitempty"`
	VerifiedReason   string     `json:"verified_reason,omitempty"`
	UnverifiedAt     *time.Time `json:"unverified_at,omitempty"`
	UnverifiedBy     string     `json:"unverified_by,omitempty"`
	UnverifiedReason string     `json:"unverified_reason,omitempty"`

	KeyRotatedAt *time.Time `json:"key_rotated_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

// Clone returns a deep copy safe for caller mutation.
func (s *AgentGovernanceState) Clone() *AgentGovernanceState {
	if s == nil {
		return nil
	}

	clone := *s
	clone.DelegatedScopes = append([]string(nil), s.DelegatedScopes...)
	clone.SelfScopes = append([]string(nil), s.SelfScopes...)
	clone.QuarantineStart = cloneAgentGovernanceTime(s.QuarantineStart)
	clone.QuarantineEnd = cloneAgentGovernanceTime(s.QuarantineEnd)
	clone.QuarantineApprovedAt = cloneAgentGovernanceTime(s.QuarantineApprovedAt)
	clone.VerifiedAt = cloneAgentGovernanceTime(s.VerifiedAt)
	clone.UnverifiedAt = cloneAgentGovernanceTime(s.UnverifiedAt)
	clone.KeyRotatedAt = cloneAgentGovernanceTime(s.KeyRotatedAt)
	return &clone
}

// DelegatedScopesCopy returns a caller-safe copy of delegated scopes.
func (s *AgentGovernanceState) DelegatedScopesCopy() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.DelegatedScopes...)
}

// SelfScopesCopy returns a caller-safe copy of self-sovereign scopes.
func (s *AgentGovernanceState) SelfScopesCopy() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.SelfScopes...)
}

// QuarantineActiveAt reports whether the agent remains quarantined at now.
func (s *AgentGovernanceState) QuarantineActiveAt(now time.Time) (bool, time.Time) {
	if s == nil {
		return false, time.Time{}
	}
	if strings.EqualFold(strings.TrimSpace(s.QuarantineStatus), AgentQuarantineStatusApproved) {
		return false, time.Time{}
	}
	if s.QuarantineEnd == nil || s.QuarantineEnd.IsZero() {
		return false, time.Time{}
	}
	end := s.QuarantineEnd.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if now.Before(end) {
		return true, end
	}
	return false, end
}

func cloneAgentGovernanceTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
