package models

import (
	"fmt"
	"strings"
	"time"
)

// DraftReviewGrant authorizes one local account to review an owner's draft.
type DraftReviewGrant struct {
	PK        string     `theorydb:"pk,attr:PK"`
	SK        string     `theorydb:"sk,attr:SK"`
	GSI2PK    string     `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK    string     `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`
	OwnerID   string     `theorydb:"attr:ownerID"`
	DraftID   string     `theorydb:"attr:draftID"`
	Reviewer  string     `theorydb:"attr:reviewer"`
	GrantedAt time.Time  `theorydb:"attr:grantedAt"`
	RevokedAt *time.Time `theorydb:"attr:revokedAt,omitempty"`
	Version   int        `theorydb:"version,attr:version"`
}

func (DraftReviewGrant) TableName() string { return MainTableName }
func (g *DraftReviewGrant) UpdateKeys() error {
	if strings.TrimSpace(g.OwnerID) == "" || strings.TrimSpace(g.DraftID) == "" || strings.TrimSpace(g.Reviewer) == "" {
		return fmt.Errorf("ownerID, draftID, and reviewer are required")
	}
	if g.GrantedAt.IsZero() {
		g.GrantedAt = time.Now().UTC()
	}
	g.PK = fmt.Sprintf("USER#%s#DRAFT#REVIEW", g.OwnerID)
	g.SK = fmt.Sprintf("GRANT#%s#REVIEWER#%s", g.DraftID, g.Reviewer)
	if g.RevokedAt == nil {
		g.GSI2PK = fmt.Sprintf("DRAFT#REVIEWER#%s", g.Reviewer)
		g.GSI2SK = fmt.Sprintf("TIME#%s#OWNER#%s#DRAFT#%s", g.GrantedAt.UTC().Format(time.RFC3339Nano), g.OwnerID, g.DraftID)
	} else {
		g.GSI2PK = ""
		g.GSI2SK = ""
	}
	return nil
}

type DraftReviewVerdict struct {
	PK         string    `theorydb:"pk,attr:PK"`
	SK         string    `theorydb:"sk,attr:SK"`
	OwnerID    string    `theorydb:"attr:ownerID"`
	DraftID    string    `theorydb:"attr:draftID"`
	Reviewer   string    `theorydb:"attr:reviewer"`
	Verdict    string    `theorydb:"attr:verdict"`
	Notes      string    `theorydb:"attr:notes,omitempty"`
	RecordedAt time.Time `theorydb:"attr:recordedAt"`
	Version    int       `theorydb:"version,attr:version"`
}

func (DraftReviewVerdict) TableName() string { return MainTableName }
func (v *DraftReviewVerdict) UpdateKeys() error {
	if strings.TrimSpace(v.OwnerID) == "" || strings.TrimSpace(v.DraftID) == "" || strings.TrimSpace(v.Reviewer) == "" || strings.TrimSpace(v.Verdict) == "" {
		return fmt.Errorf("ownerID, draftID, reviewer, and verdict are required")
	}
	if v.RecordedAt.IsZero() {
		v.RecordedAt = time.Now().UTC()
	}
	v.PK = fmt.Sprintf("USER#%s#DRAFT#REVIEW", v.OwnerID)
	v.SK = fmt.Sprintf("VERDICT#%s#TIME#%s#REVIEWER#%s", v.DraftID, v.RecordedAt.UTC().Format(time.RFC3339Nano), v.Reviewer)
	return nil
}
