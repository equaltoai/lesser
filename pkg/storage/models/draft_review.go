package models

import (
	"fmt"
	"strings"
	"time"
)

// DraftReviewGrant authorizes one local account to review an owner's draft.
type DraftReviewGrant struct {
	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	GSI2PK    string    `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK    string    `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`
	OwnerID   string    `theorydb:"attr:ownerID"`
	DraftID   string    `theorydb:"attr:draftID"`
	Reviewer  string    `theorydb:"attr:reviewer"`
	GrantedAt time.Time `theorydb:"attr:grantedAt"`
	// ExpiresAt bounds the grant. Grants are cheap and explicitly re-shared, so
	// an unbounded grant would let a stale reviewer assignment authorize reads,
	// URL minting, and approval forever. Expired grants fail closed everywhere.
	ExpiresAt *time.Time `theorydb:"attr:expiresAt,omitempty"`
	RevokedAt *time.Time `theorydb:"attr:revokedAt,omitempty"`
	Version   int        `theorydb:"version,attr:version"`
}

// TableName returns the table for a review grant.
func (DraftReviewGrant) TableName() string { return MainTableName }

// IsActive reports whether the grant is currently usable: not revoked and not
// past its bounded expiry. A grant without an expiry is deliberately treated as
// expired (fail-closed) so rows created before the M2 expiry surface cannot
// authorize reads or approval indefinitely; re-sharing refreshes the grant.
func (g *DraftReviewGrant) IsActive(now time.Time) bool {
	if g == nil || g.RevokedAt != nil {
		return false
	}
	if g.ExpiresAt == nil {
		return false
	}
	return g.ExpiresAt.After(now)
}

// Expired reports whether a bounded grant has passed its expiry. Revocation is
// reported separately by the active checks; this predicate only classifies
// un-revoked grants that can no longer authorize anything.
func (g *DraftReviewGrant) Expired(now time.Time) bool {
	if g == nil || g.RevokedAt != nil {
		return false
	}
	if g.ExpiresAt == nil {
		return true
	}
	return !g.ExpiresAt.After(now)
}

// UpdateKeys derives grant primary and sparse reviewer-queue keys.
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

// DraftReviewVerdict is an immutable review decision audit record.
type DraftReviewVerdict struct {
	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	OwnerID     string    `theorydb:"attr:ownerID"`
	DraftID     string    `theorydb:"attr:draftID"`
	Reviewer    string    `theorydb:"attr:reviewer"`
	Verdict     string    `theorydb:"attr:verdict"`
	Notes       string    `theorydb:"attr:notes,omitempty"`
	ContentHash string    `theorydb:"attr:contentHash,omitempty" json:"content_hash,omitempty"`
	RecordedAt  time.Time `theorydb:"attr:recordedAt"`
	Version     int       `theorydb:"version,attr:version"`
}

// TableName returns the table for a review verdict.
func (DraftReviewVerdict) TableName() string { return MainTableName }

// UpdateKeys derives verdict primary keys.
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
