package models

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// SKAgentGovernance is the sort key for typed agent-governance rows.
const SKAgentGovernance = "AGENT_GOVERNANCE"

// AgentGovernanceState stores typed agent governance state outside the core user row.
type AgentGovernanceState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	Username string `theorydb:"attr:username" json:"username"`

	QuarantineStatus     string     `theorydb:"attr:quarantineStatus,omitempty" json:"quarantine_status,omitempty"`
	QuarantineStart      *time.Time `theorydb:"attr:quarantineStart" json:"quarantine_start,omitempty"`
	QuarantineEnd        *time.Time `theorydb:"attr:quarantineEnd" json:"quarantine_end,omitempty"`
	QuarantineApprovedBy string     `theorydb:"attr:quarantineApprovedBy,omitempty" json:"quarantine_approved_by,omitempty"`
	QuarantineApprovedAt *time.Time `theorydb:"attr:quarantineApprovedAt" json:"quarantine_approved_at,omitempty"`

	DelegatedScopes []string `theorydb:"attr:delegatedScopes" json:"delegated_scopes,omitempty"`
	SelfScopes      []string `theorydb:"attr:selfScopes" json:"self_scopes,omitempty"`
	SelfSovereign   bool     `theorydb:"attr:selfSovereign" json:"self_sovereign,omitempty"`

	Verified         bool       `theorydb:"attr:verified" json:"verified"`
	VerifiedAt       *time.Time `theorydb:"attr:verifiedAt" json:"verified_at,omitempty"`
	VerifiedBy       string     `theorydb:"attr:verifiedBy,omitempty" json:"verified_by,omitempty"`
	VerifiedReason   string     `theorydb:"attr:verifiedReason,omitempty" json:"verified_reason,omitempty"`
	UnverifiedAt     *time.Time `theorydb:"attr:unverifiedAt" json:"unverified_at,omitempty"`
	UnverifiedBy     string     `theorydb:"attr:unverifiedBy,omitempty" json:"unverified_by,omitempty"`
	UnverifiedReason string     `theorydb:"attr:unverifiedReason,omitempty" json:"unverified_reason,omitempty"`

	KeyRotatedAt *time.Time `theorydb:"attr:keyRotatedAt" json:"key_rotated_at,omitempty"`

	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the backing DynamoDB table for the model.
func (AgentGovernanceState) TableName() string {
	return MainTableName
}

// BeforeCreate normalizes the model before an insert.
func (s *AgentGovernanceState) BeforeCreate() error {
	if err := s.prepareForWrite(true); err != nil {
		return err
	}
	return s.UpdateKeys()
}

// BeforeUpdate normalizes the model before an update.
func (s *AgentGovernanceState) BeforeUpdate() error {
	if err := s.prepareForWrite(false); err != nil {
		return err
	}
	return s.UpdateKeys()
}

// UpdateKeys refreshes the primary key fields from the username.
func (s *AgentGovernanceState) UpdateKeys() error {
	if s == nil {
		return fmt.Errorf("agent governance state is required")
	}
	s.Username = strings.ToLower(strings.TrimSpace(s.Username))
	if err := common.ValidateRequiredParam("Username", s.Username); err != nil {
		return err
	}
	s.PK = fmt.Sprintf(KeyPatternUser, s.Username)
	s.SK = SKAgentGovernance
	return nil
}

// GetPK returns the model partition key.
func (s *AgentGovernanceState) GetPK() string {
	if s == nil {
		return ""
	}
	return s.PK
}

// GetSK returns the model sort key.
func (s *AgentGovernanceState) GetSK() string {
	if s == nil {
		return ""
	}
	return s.SK
}

func (s *AgentGovernanceState) prepareForWrite(isCreate bool) error {
	if s == nil {
		return fmt.Errorf("agent governance state is required")
	}
	s.Username = strings.ToLower(strings.TrimSpace(s.Username))
	if err := common.ValidateRequiredParam("Username", s.Username); err != nil {
		return err
	}

	now := time.Now().UTC()
	if isCreate && s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	s.CreatedAt = s.CreatedAt.UTC()
	s.UpdatedAt = s.UpdatedAt.UTC()

	s.QuarantineStart = normalizeAgentGovernanceTime(s.QuarantineStart)
	s.QuarantineEnd = normalizeAgentGovernanceTime(s.QuarantineEnd)
	s.QuarantineApprovedAt = normalizeAgentGovernanceTime(s.QuarantineApprovedAt)
	s.VerifiedAt = normalizeAgentGovernanceTime(s.VerifiedAt)
	s.UnverifiedAt = normalizeAgentGovernanceTime(s.UnverifiedAt)
	s.KeyRotatedAt = normalizeAgentGovernanceTime(s.KeyRotatedAt)

	s.DelegatedScopes = normalizeAgentGovernanceScopes(s.DelegatedScopes)
	s.SelfScopes = normalizeAgentGovernanceScopes(s.SelfScopes)
	s.QuarantineStatus = strings.TrimSpace(s.QuarantineStatus)
	s.QuarantineApprovedBy = strings.TrimSpace(s.QuarantineApprovedBy)
	s.VerifiedBy = strings.TrimSpace(s.VerifiedBy)
	s.VerifiedReason = strings.TrimSpace(s.VerifiedReason)
	s.UnverifiedBy = strings.TrimSpace(s.UnverifiedBy)
	s.UnverifiedReason = strings.TrimSpace(s.UnverifiedReason)

	return nil
}

func normalizeAgentGovernanceTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	t := value.UTC()
	return &t
}

func normalizeAgentGovernanceScopes(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}
