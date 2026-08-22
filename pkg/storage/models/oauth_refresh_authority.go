package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	oauthRefreshAuthorityPrefix = "OAUTH_REFRESH_AUTHORITY#"
	oauthRefreshFamilyPrefix    = "OAUTH_REFRESH_FAMILY#"
	oauthRefreshBudgetPrefix    = "OAUTH_REFRESH_WALK_BUDGET#"
	oauthRefreshCurrentSK       = "CURRENT"
	oauthRefreshSuccessorPrefix = "SUCCESSOR#"
)

// OAuthRefreshFamilySlot is one live refresh family in a tuple-scoped authority row.
// Slots are ordered least-recently-used to most-recently-used.
type OAuthRefreshFamilySlot struct {
	FamilyID      string    `json:"family_id"`
	HeadTokenHash string    `json:"head_token_hash"`
	Generation    int       `json:"generation"`
	ExpiresAt     time.Time `json:"expires_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OAuthRefreshAuthority is the strongly-consistent singleton authority for a
// (username, client, resource) tuple. Revision is the CAS boundary.
type OAuthRefreshAuthority struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Username  string                   `theorydb:"attr:username" json:"username"`
	ClientID  string                   `theorydb:"attr:clientID" json:"client_id"`
	Resource  string                   `theorydb:"attr:resource,omitempty" json:"resource,omitempty"`
	Slots     []OAuthRefreshFamilySlot `theorydb:"json,attr:slots" json:"slots"`
	UpdatedAt time.Time                `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL       int64                    `theorydb:"ttl,attr:ttl" json:"-"`
	Revision  int                      `theorydb:"version,attr:revision" json:"revision"`
}

// TableName returns the shared single-table DynamoDB name.
func (OAuthRefreshAuthority) TableName() string { return MainTableName }

// UpdateKeys derives the tuple-scoped authority primary key.
func (a *OAuthRefreshAuthority) UpdateKeys() error {
	if a == nil || strings.TrimSpace(a.Username) == "" || strings.TrimSpace(a.ClientID) == "" {
		return fmt.Errorf("refresh authority requires username and client id")
	}
	a.PK = OAuthRefreshAuthorityPK(a.Username, a.ClientID, a.Resource)
	a.SK = oauthRefreshCurrentSK
	return nil
}

// BeforeCreate prepares keys and creation defaults.
func (a *OAuthRefreshAuthority) BeforeCreate() error {
	if err := a.UpdateKeys(); err != nil {
		return err
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// OAuthRefreshAuthorityPK hashes a length-delimited tuple so attacker-controlled
// identifiers never become ambiguous or disclose identity in DynamoDB keys.
func OAuthRefreshAuthorityPK(username, clientID, resource string) string {
	material := fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(username), username, len(clientID), clientID, len(resource), resource)
	sum := sha256.Sum256([]byte(material))
	return oauthRefreshAuthorityPrefix + hex.EncodeToString(sum[:])
}

// OAuthRefreshSuccessorArtifact is one immutable edge in a refresh lineage.
// Only the successor credential is encrypted; all lookup and integrity fields are hashes.
type OAuthRefreshSuccessorArtifact struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	FamilyID            string    `theorydb:"attr:familyID" json:"family_id"`
	PredecessorHash     string    `theorydb:"attr:predecessorHash" json:"predecessor_hash"`
	SuccessorHash       string    `theorydb:"attr:successorHash" json:"successor_hash"`
	SuccessorToken      string    `theorydb:"encrypted,attr:successorToken" json:"successor_token"`
	SuccessorGeneration int       `theorydb:"attr:successorGeneration" json:"successor_generation"`
	CreatedAt           time.Time `theorydb:"attr:createdAt" json:"created_at"`
	TTL                 int64     `theorydb:"ttl,attr:ttl" json:"-"`
}

// TableName returns the shared single-table DynamoDB name.
func (OAuthRefreshSuccessorArtifact) TableName() string { return MainTableName }

// UpdateKeys derives the family and predecessor-scoped artifact key.
func (a *OAuthRefreshSuccessorArtifact) UpdateKeys() error {
	if a == nil || strings.TrimSpace(a.FamilyID) == "" || strings.TrimSpace(a.PredecessorHash) == "" {
		return fmt.Errorf("refresh successor artifact requires family and predecessor hash")
	}
	a.PK = oauthRefreshFamilyPrefix + a.FamilyID
	a.SK = oauthRefreshSuccessorPrefix + a.PredecessorHash
	return nil
}

// BeforeCreate validates artifact integrity fields and creation defaults.
func (a *OAuthRefreshSuccessorArtifact) BeforeCreate() error {
	if err := a.UpdateKeys(); err != nil {
		return err
	}
	if strings.TrimSpace(a.SuccessorHash) == "" || strings.TrimSpace(a.SuccessorToken) == "" || a.SuccessorGeneration < 1 {
		return fmt.Errorf("refresh successor artifact is incomplete")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return nil
}

// OAuthRefreshWalkBudget bounds replay-chain work per family and time window.
type OAuthRefreshWalkBudget struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	FamilyID  string    `theorydb:"attr:familyID" json:"family_id"`
	Window    string    `theorydb:"attr:window" json:"window"`
	Consumed  int       `theorydb:"attr:consumed" json:"consumed"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"-"`
	Version   int       `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the shared single-table DynamoDB name.
func (OAuthRefreshWalkBudget) TableName() string { return MainTableName }

// UpdateKeys derives the family and time-window-scoped budget key.
func (b *OAuthRefreshWalkBudget) UpdateKeys() error {
	if b == nil || strings.TrimSpace(b.FamilyID) == "" || strings.TrimSpace(b.Window) == "" {
		return fmt.Errorf("refresh walk budget requires family and window")
	}
	b.PK = oauthRefreshBudgetPrefix + b.FamilyID
	b.SK = "WINDOW#" + b.Window
	return nil
}

// BeforeCreate prepares keys and creation defaults.
func (b *OAuthRefreshWalkBudget) BeforeCreate() error {
	if err := b.UpdateKeys(); err != nil {
		return err
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = time.Now().UTC()
	}
	return nil
}
