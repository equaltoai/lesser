package models

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Promo package status values. A package is composed in DRAFT and moves to
// RELEASED exactly once when the release transition stamps the created outbound
// Status ID; re-release of a released package is refused. RELEASING is the
// transient pre-stamp state held between the version-conditioned release
// reservation and the final released stamp; a crash left in RELEASING has no
// outbound post and requires operator reconciliation (release is refused while
// it is held).
const (
	PromoPackageStatusDraft     = "draft"
	PromoPackageStatusReleasing = "releasing"
	PromoPackageStatusReleased  = "released"
)

// Promo package visibility values. Issue #1446 scopes promo attachment to
// public and unlisted posts only; private and direct posts are structurally
// rejected at compose and re-checked at release.
const (
	PromoPackageVisibilityPublic   = "public"
	PromoPackageVisibilityUnlisted = "unlisted"
)

// Promo review verdict values mirror the draft-review vocabulary.
const (
	PromoPackageReviewApproved         = "APPROVED"
	PromoPackageReviewChangesRequested = "CHANGES_REQUESTED"

	// PromoPackageGrantLifetime bounds every promo review grant, matching the
	// M2 draft-review posture: grants are cheap, ephemeral assignments refreshed
	// on re-share, and an unbounded grant would let a stale reviewer assignment
	// authorize package reads and approval forever.
	PromoPackageGrantLifetime = 7 * 24 * time.Hour

	// maxPromoPackageAssets mirrors the outbound Status attachment limit
	// enforced by the notes service, so every reviewed package can actually be
	// released without truncation or re-composition.
	maxPromoPackageAssets = 4
)

// PromoPackageAsset is one asset bound into a promo package. The binding
// carries the media identity plus the canonical sha256:<hex> digest as bound at
// review time and the durable published serving minted by the M2 publish
// transition. The digest snapshot is what review verdicts hash over; release
// re-verifies the live media record still carries the same digest and is still
// in the PUBLISHED durable state, so the exact reviewed bytes are what attach
// to the outbound post.
type PromoPackageAsset struct {
	MediaID     string `json:"media_id"`
	ContentHash string `json:"content_hash"`
	// PublishedURL snapshots the M2 durable serving at compose time. It is a
	// presentation copy for the review surface; release re-reads the media
	// record and requires digest equality before attaching the record's current
	// PublishedURL, so a stale snapshot can never substitute for the reviewed
	// bytes.
	PublishedURL string `json:"published_url,omitempty"`
}

// PromoPackage is a reviewed promotional package: outbound post text and an
// exact, ordered set of PUBLISHED media assets attached to a public/unlisted
// post, promoting a published article. Pre-release the record is visible only
// to its owner and holders of an active PromoReviewGrant.
type PromoPackage struct {
	// Primary keys: USER#{ownerID}#PROMO#PACKAGE / PACKAGE#{packageID}
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	PackageID string `theorydb:"attr:packageID"`
	OwnerID   string `theorydb:"attr:ownerID"`

	// ArticleID is the published article's object ID that the post promotes.
	ArticleID string `theorydb:"attr:articleID"`

	// PostText is the outbound post content; the package can append nothing at
	// release beyond what review bound.
	PostText string `theorydb:"attr:postText"`

	// Visibility is public or unlisted only (issue #1446 scope).
	Visibility string `theorydb:"attr:visibility"`

	// Assets is the ordered asset set attached to the post. Order is
	// semantically meaningful (attachment order), so the content hash covers the
	// binding order and reordering re-requires review.
	Assets []PromoPackageAsset `theorydb:"attr:assets,omitempty"`

	// ContentHash is the canonical package digest over post text, visibility,
	// article reference, and ordered asset digests. Review verdicts bind to it;
	// any content change re-hashes and stales prior approvals.
	ContentHash string `theorydb:"attr:contentHash,omitempty"`

	// Status is draft or released.
	Status string `theorydb:"attr:status"`

	// ReleasedStatusID is the outbound Status created by the release
	// transition; stamped exactly once, and its presence blocks re-release.
	ReleasedStatusID string     `theorydb:"attr:releasedStatusID,omitempty"`
	ReleasedAt       *time.Time `theorydb:"attr:releasedAt,omitempty"`

	CreatedAt time.Time `theorydb:"attr:createdAt"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt"`

	// ModelVersion provides optimistic concurrency for the field-scoped content
	// and release writes (CAS), mirroring the M2 UpdateMediaPublishedState
	// precedent.
	ModelVersion int `theorydb:"version,attr:modelVersion"`
}

// TableName returns the DynamoDB table backing PromoPackage.
func (PromoPackage) TableName() string {
	return MainTableName
}

// UpdateKeys derives the primary keys for a promo package.
func (p *PromoPackage) UpdateKeys() error {
	if strings.TrimSpace(p.OwnerID) == "" {
		return fmt.Errorf("ownerID is required")
	}
	if strings.TrimSpace(p.PackageID) == "" {
		return fmt.Errorf("packageID is required")
	}
	p.PK = fmt.Sprintf("USER#%s#PROMO#PACKAGE", strings.TrimSpace(p.OwnerID))
	p.SK = fmt.Sprintf("PACKAGE#%s", strings.TrimSpace(p.PackageID))
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// GetPK returns the partition key.
func (p *PromoPackage) GetPK() string { return p.PK }

// GetSK returns the sort key.
func (p *PromoPackage) GetSK() string { return p.SK }

// IsReleased reports whether the release transition already stamped an outbound
// Status for this package.
func (p *PromoPackage) IsReleased() bool {
	return p != nil && strings.TrimSpace(p.ReleasedStatusID) != "" &&
		strings.EqualFold(strings.TrimSpace(p.Status), PromoPackageStatusReleased)
}

// IsReleasing reports whether the package is mid-release: the version-
// conditioned release reservation was won but the outbound Status has not been
// stamped yet (or the process crashed between reservation and stamp). Release
// and composition are refused while this transient state is held.
func (p *PromoPackage) IsReleasing() bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.Status), PromoPackageStatusReleasing)
}

// NormalizePromoPackageVisibility canonicalizes the intended outbound
// visibility. Issue #1446 scopes promo attachment to public/unlisted posts;
// private and direct are structurally rejected here and re-checked at release.
func NormalizePromoPackageVisibility(visibility string) (string, error) {
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	switch visibility {
	case PromoPackageVisibilityPublic, PromoPackageVisibilityUnlisted:
		return visibility, nil
	default:
		return "", fmt.Errorf("promo package visibility must be public or unlisted, got %q", visibility)
	}
}

// NormalizePromoPackageAssets validates and canonicalizes the ordered asset set.
// Each asset must name a media ID and carry the canonical sha256:<hex> digest of
// the bytes bound at review time; the same media cannot be bound twice; the set
// is capped at the outbound Status attachment limit. PublishedURL, when
// present, must be an http(s) URL.
func NormalizePromoPackageAssets(assets []PromoPackageAsset) ([]PromoPackageAsset, error) {
	if len(assets) > maxPromoPackageAssets {
		return nil, fmt.Errorf("promo package asset count exceeds %d", maxPromoPackageAssets)
	}
	out := make([]PromoPackageAsset, 0, len(assets))
	seen := make(map[string]struct{}, len(assets))
	for _, source := range assets {
		asset := source
		asset.MediaID = strings.TrimSpace(asset.MediaID)
		asset.ContentHash = strings.TrimSpace(asset.ContentHash)
		asset.PublishedURL = strings.TrimSpace(asset.PublishedURL)
		if asset.MediaID == "" {
			return nil, errors.New("promo package asset media ID is required")
		}
		if _, exists := seen[asset.MediaID]; exists {
			return nil, fmt.Errorf("promo package media %q is bound more than once", asset.MediaID)
		}
		seen[asset.MediaID] = struct{}{}
		if !IsCanonicalSHA256Digest(asset.ContentHash) {
			return nil, fmt.Errorf("promo package asset %q must carry the canonical sha256 digest bound at review time", asset.MediaID)
		}
		if asset.PublishedURL != "" &&
			!strings.HasPrefix(asset.PublishedURL, "https://") &&
			!strings.HasPrefix(asset.PublishedURL, "http://") {
			return nil, fmt.Errorf("promo package asset %q published URL must be an http(s) URL", asset.MediaID)
		}
		out = append(out, asset)
	}
	return out, nil
}

// IsCanonicalSHA256Digest reports whether value is a canonical sha256:<hex>
// identifier with exactly 64 lowercase hex characters, matching the M0 pipeline
// digest, the M3 upload-grant admission (64-lowercase-hex sha256), and the
// MediaProvenance content-integrity surface.
func IsCanonicalSHA256Digest(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	hexPart := strings.TrimPrefix(value, "sha256:")
	if hexPart != strings.ToLower(hexPart) {
		return false
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return false
	}
	return true
}

// PromoPackageContentHash binds review verdicts and the release gate to the
// exact reviewed package content: the outbound post text, the intended
// visibility, the promoted article reference, and the ordered asset set
// (identity plus the digest bound at review time). The canonicalization follows
// the M2 draft-review pattern — every field is length-prefixed so boundaries
// stay unambiguous even when values contain control characters. Order is
// meaningful: assets hash in binding order because the order is the attachment
// order on the outbound post. Any text/visibility/article/asset change
// re-hashes and makes prior verdicts and principal authorization stale through
// the verdict-vs-hash comparison, so the exact reviewed package is what
// releases.
func PromoPackageContentHash(p *PromoPackage) string {
	if p == nil {
		return ""
	}
	h := sha256.New()
	var length [8]byte
	write := func(value string) {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(value))
	}
	write(p.PostText)
	write(p.Visibility)
	write(p.ArticleID)
	for _, asset := range p.Assets {
		write(asset.MediaID)
		write(asset.ContentHash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// PromoReviewGrant authorizes one local account to review an owner's promo
// package before release. Semantics mirror the M2 draft-review grant exactly:
// owner-scoped, expiry-bounded (7 days, refreshed on re-share), and fail-closed
// — a revoked or expired grant authorizes neither package reads nor approval.
type PromoReviewGrant struct {
	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	GSI2PK    string    `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"`
	GSI2SK    string    `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"`
	OwnerID   string    `theorydb:"attr:ownerID"`
	PackageID string    `theorydb:"attr:packageID"`
	Reviewer  string    `theorydb:"attr:reviewer"`
	GrantedAt time.Time `theorydb:"attr:grantedAt"`
	// ExpiresAt bounds the grant. Grants recorded without an expiry are treated
	// as expired so pre-M4 rows cannot authorize reads or approval indefinitely.
	ExpiresAt *time.Time `theorydb:"attr:expiresAt,omitempty"`
	RevokedAt *time.Time `theorydb:"attr:revokedAt,omitempty"`
	Version   int        `theorydb:"version,attr:version"`
}

// TableName returns the table for a promo review grant.
func (PromoReviewGrant) TableName() string { return MainTableName }

// UpdateKeys derives grant primary and sparse reviewer-queue keys.
//
//nolint:dupl // the review-grant key derivation mirrors its sibling review surface (M4 issue #1446)
func (g *PromoReviewGrant) UpdateKeys() error {
	if strings.TrimSpace(g.OwnerID) == "" || strings.TrimSpace(g.PackageID) == "" || strings.TrimSpace(g.Reviewer) == "" {
		return fmt.Errorf("ownerID, packageID, and reviewer are required")
	}
	if g.GrantedAt.IsZero() {
		g.GrantedAt = time.Now().UTC()
	}
	g.PK = fmt.Sprintf("USER#%s#PROMO#REVIEW", g.OwnerID)
	g.SK = fmt.Sprintf("GRANT#%s#REVIEWER#%s", g.PackageID, g.Reviewer)
	if g.RevokedAt == nil {
		g.GSI2PK = fmt.Sprintf("PROMO#REVIEWER#%s", g.Reviewer)
		g.GSI2SK = fmt.Sprintf("TIME#%s#OWNER#%s#PACKAGE#%s", g.GrantedAt.UTC().Format(time.RFC3339Nano), g.OwnerID, g.PackageID)
	} else {
		g.GSI2PK = ""
		g.GSI2SK = ""
	}
	return nil
}

// IsActive reports whether the grant is currently usable: not revoked and not
// past its bounded expiry. A grant without an expiry is deliberately treated as
// expired (fail-closed).
func (g *PromoReviewGrant) IsActive(now time.Time) bool {
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
func (g *PromoReviewGrant) Expired(now time.Time) bool {
	if g == nil || g.RevokedAt != nil {
		return false
	}
	if g.ExpiresAt == nil {
		return true
	}
	return !g.ExpiresAt.After(now)
}

// PromoReviewVerdict is an immutable review decision audit record bound to the
// exact package content hash. Any package content change re-hashes and the
// verdict-vs-hash comparison stales the approval, matching M2.
//
//nolint:dupl // promo review verdicts mirror the draft-review verdict record shape (M4 issue #1446)
type PromoReviewVerdict struct {
	PK          string    `theorydb:"pk,attr:PK"`
	SK          string    `theorydb:"sk,attr:SK"`
	OwnerID     string    `theorydb:"attr:ownerID"`
	PackageID   string    `theorydb:"attr:packageID"`
	Reviewer    string    `theorydb:"attr:reviewer"`
	Verdict     string    `theorydb:"attr:verdict"`
	Notes       string    `theorydb:"attr:notes,omitempty"`
	ContentHash string    `theorydb:"attr:contentHash,omitempty"`
	RecordedAt  time.Time `theorydb:"attr:recordedAt"`
	Version     int       `theorydb:"version,attr:version"`
}

// TableName returns the table for a promo review verdict.
func (PromoReviewVerdict) TableName() string { return MainTableName }

// UpdateKeys derives verdict primary keys.
//
//nolint:dupl // promo review verdicts mirror the draft-review verdict record shape (M4 issue #1446)
func (v *PromoReviewVerdict) UpdateKeys() error {
	if strings.TrimSpace(v.OwnerID) == "" || strings.TrimSpace(v.PackageID) == "" || strings.TrimSpace(v.Reviewer) == "" || strings.TrimSpace(v.Verdict) == "" {
		return fmt.Errorf("ownerID, packageID, reviewer, and verdict are required")
	}
	if v.RecordedAt.IsZero() {
		v.RecordedAt = time.Now().UTC()
	}
	v.PK = fmt.Sprintf("USER#%s#PROMO#REVIEW", v.OwnerID)
	v.SK = fmt.Sprintf("VERDICT#%s#TIME#%s#REVIEWER#%s", v.PackageID, v.RecordedAt.UTC().Format(time.RFC3339Nano), v.Reviewer)
	return nil
}
