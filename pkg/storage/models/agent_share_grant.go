package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	// AgentShareGrantSKPrefix identifies per-agent share-list rows.
	AgentShareGrantSKPrefix = "AGENT_SHARE#GRANTEE#"
	// AgentShareGrantGranteeGSI2Prefix identifies the sparse active-grant index.
	AgentShareGrantGranteeGSI2Prefix = "AGENT_SHARE#GRANTEE#"
)

// AgentShareGrant authorizes one local Lesser account to access an agent through
// actor-scoped surfaces that independently enforce this storage contract.
type AgentShareGrant struct {
	PK              string     `theorydb:"pk,attr:PK" json:"-"`
	SK              string     `theorydb:"sk,attr:SK" json:"-"`
	GSI2PK          string     `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK          string     `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`
	AgentUsername   string     `theorydb:"attr:agentUsername" json:"agent_username"`
	GranteeUsername string     `theorydb:"attr:granteeUsername" json:"grantee_username"`
	GrantedBy       string     `theorydb:"attr:grantedBy" json:"granted_by"`
	GrantedAt       time.Time  `theorydb:"attr:grantedAt" json:"granted_at"`
	RevokedAt       *time.Time `theorydb:"attr:revokedAt,omitempty" json:"revoked_at,omitempty"`
	RevokedBy       string     `theorydb:"attr:revokedBy,omitempty" json:"revoked_by,omitempty"`
	Version         int        `theorydb:"version,attr:version" json:"-"`
}

// TableName returns the single Lesser table backing agent share grants.
func (AgentShareGrant) TableName() string { return MainTableName }

// AgentShareGrantPK returns the direct-read partition key for an agent.
func AgentShareGrantPK(agentUsername string) string {
	return fmt.Sprintf("USER#%s", canonicalAgentShareUsername(agentUsername))
}

// AgentShareGrantSK returns the direct-read sort key for one grantee.
func AgentShareGrantSK(granteeUsername string) string {
	return AgentShareGrantSKPrefix + canonicalAgentShareUsername(granteeUsername)
}

// AgentShareGrantGranteeGSI2PK returns the active-list partition for one grantee.
func AgentShareGrantGranteeGSI2PK(granteeUsername string) string {
	return AgentShareGrantGranteeGSI2Prefix + canonicalAgentShareUsername(granteeUsername)
}

func canonicalAgentShareUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// UpdateKeys derives the primary direct-read keys and sparse active-grant index.
func (g *AgentShareGrant) UpdateKeys() error {
	if g == nil {
		return fmt.Errorf("agent share grant is required")
	}
	g.AgentUsername = canonicalAgentShareUsername(g.AgentUsername)
	g.GranteeUsername = canonicalAgentShareUsername(g.GranteeUsername)
	g.GrantedBy = canonicalAgentShareUsername(g.GrantedBy)
	g.RevokedBy = canonicalAgentShareUsername(g.RevokedBy)
	if g.AgentUsername == "" || g.GranteeUsername == "" || g.GrantedBy == "" {
		return fmt.Errorf("agent_username, grantee_username, and granted_by are required")
	}
	if g.GrantedAt.IsZero() {
		g.GrantedAt = time.Now().UTC()
	} else {
		g.GrantedAt = g.GrantedAt.UTC()
	}
	if g.RevokedAt != nil {
		revokedAt := g.RevokedAt.UTC()
		g.RevokedAt = &revokedAt
		if g.RevokedBy == "" {
			return fmt.Errorf("revoked_by is required for a revoked grant")
		}
	}

	g.PK = AgentShareGrantPK(g.AgentUsername)
	g.SK = AgentShareGrantSK(g.GranteeUsername)
	if g.RevokedAt == nil {
		g.GSI2PK = AgentShareGrantGranteeGSI2PK(g.GranteeUsername)
		g.GSI2SK = fmt.Sprintf("AGENT#%s", g.AgentUsername)
	} else {
		g.GSI2PK = ""
		g.GSI2SK = ""
	}
	return nil
}
