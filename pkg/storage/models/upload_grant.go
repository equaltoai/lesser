package models

import (
	"fmt"
	"strings"
	"time"
)

// Upload grant lifecycle states. EXPIRED is derived at read time from the
// bounded expiry (mirroring the review-grant fail-closed discipline); the
// stored states are MINTED, USED, and FAILED_DIGEST.
const (
	// UploadGrantStatusMinted is the initial state: the presigned PUT is valid
	// and a finalize may still consume the grant.
	UploadGrantStatusMinted = "MINTED"
	// UploadGrantStatusUsed records a successful finalize: the grant was
	// consumed exactly once and the verified bytes became an editorial asset.
	UploadGrantStatusUsed = "USED"
	// UploadGrantStatusFailedDigest records a finalize whose uploaded bytes did
	// not match the declared hash (or violated the declared size/type bounds).
	// The grant is consumed and the unverified object is deleted.
	UploadGrantStatusFailedDigest = "FAILED_DIGEST"
)

// UploadGrant authorizes one actor to PUT the exact declared bytes to one
// presigned S3 object and finalize them into an internal editorial media
// record. The grant is actor-scoped (owner partition), one-time (version-
// conditioned atomic consume), hash-bound (declared sha256 of the intended
// bytes), size-capped, and short-lived (DynamoDB TTL + fail-closed expiry).
type UploadGrant struct {
	PK    string `theorydb:"pk,attr:PK"`
	SK    string `theorydb:"sk,attr:SK"`
	Owner string `theorydb:"attr:owner"`
	// GrantID is an unguessable opaque identifier returned to the owner at mint.
	GrantID string `theorydb:"attr:grantID"`
	// ContentType is the declared media type; the presigned PUT signs it so S3
	// rejects a request carrying a different Content-Type header.
	ContentType string `theorydb:"attr:contentType"`
	// MaxSizeBytes is the declared size cap; finalize fails closed when the
	// stored object exceeds it.
	MaxSizeBytes int64 `theorydb:"attr:maxSizeBytes"`
	// ContentSHA256 is the hex-encoded sha256 of the exact bytes the caller
	// will PUT. Finalize computes the stored object's digest and admits the
	// asset only when it matches.
	ContentSHA256 string `theorydb:"attr:contentSHA256"`
	// S3Bucket/S3Key locate the presigned PUT target. The key embeds the minted
	// MediaID, is unguessable, and is scoped to exactly this grant.
	S3Bucket string `theorydb:"attr:s3Bucket"`
	S3Key    string `theorydb:"attr:s3Key"`
	// MediaID is minted with the grant so the object key and the final media
	// record share one identity; finalize creates the record under this ID.
	MediaID string `theorydb:"attr:mediaID,omitempty"`
	// FileName is the record-level filename derived from the declared type.
	FileName string `theorydb:"attr:fileName"`
	// Status is UploadGrantStatusMinted/Used/FailedDigest.
	Status string `theorydb:"attr:status"`
	// GrantedAt and ExpiresAt bound the grant. A grant past ExpiresAt fails
	// closed everywhere; ExpiresAtTTL lets DynamoDB self-clean the row.
	GrantedAt    time.Time `theorydb:"attr:grantedAt"`
	ExpiresAt    time.Time `theorydb:"attr:expiresAt"`
	ExpiresAtTTL int64     `theorydb:"ttl,attr:ttl" json:"-"`
	// UsedAt records the successful consume; FailedAt/FailureReason record a
	// digest-failure consume so the owner can inspect why an upload was not
	// admitted.
	UsedAt        *time.Time `theorydb:"attr:usedAt,omitempty"`
	FailedAt      *time.Time `theorydb:"attr:failedAt,omitempty"`
	FailureReason string     `theorydb:"attr:failureReason,omitempty"`
	// Version guards the single-use consume: finalize transitions MINTED under
	// the observed version, so a concurrent finalize cannot double-consume.
	Version int `theorydb:"version,attr:version"`
}

// TableName returns the table backing an upload grant.
func (UploadGrant) TableName() string { return MainTableName }

// UpdateKeys derives grant primary keys and the DynamoDB TTL attribute. A zero
// expiry is rejected: an upload grant without a bounded expiry must never
// authorize a PUT or finalize indefinitely (fail-closed, mirroring the review
// grant).
func (g *UploadGrant) UpdateKeys() error {
	if strings.TrimSpace(g.Owner) == "" || strings.TrimSpace(g.GrantID) == "" {
		return fmt.Errorf("owner and grantID are required")
	}
	if g.ExpiresAt.IsZero() {
		return fmt.Errorf("upload grant expiry is required")
	}
	g.PK = fmt.Sprintf("USER#%s#UPLOAD", strings.TrimSpace(g.Owner))
	g.SK = fmt.Sprintf("GRANT#%s", strings.TrimSpace(g.GrantID))
	g.ExpiresAtTTL = g.ExpiresAt.UTC().Unix()
	return nil
}

// IsMinted reports whether the grant is in its initial consumable state.
func (g *UploadGrant) IsMinted() bool {
	return g != nil && g.Status == UploadGrantStatusMinted
}

// IsUsed reports whether a finalize already consumed the grant successfully.
func (g *UploadGrant) IsUsed() bool {
	return g != nil && g.Status == UploadGrantStatusUsed
}

// IsFailedDigest reports whether a finalize consumed the grant because the
// uploaded bytes did not match the declared hash.
func (g *UploadGrant) IsFailedDigest() bool {
	return g != nil && g.Status == UploadGrantStatusFailedDigest
}

// Expired classifies a still-minted grant that has passed its bounded expiry.
// Consumed grants are never reported expired; the terminal state dominates.
func (g *UploadGrant) Expired(now time.Time) bool {
	if g == nil || !g.IsMinted() {
		return false
	}
	return !g.ExpiresAt.After(now)
}

// StatusClassification returns the inspectable grant status for a query time:
// the stored terminal states plus the derived EXPIRED classification.
func (g *UploadGrant) StatusClassification(now time.Time) string {
	if g == nil {
		return ""
	}
	switch {
	case g.IsUsed():
		return UploadGrantStatusUsed
	case g.IsFailedDigest():
		return UploadGrantStatusFailedDigest
	case g.Expired(now):
		return "EXPIRED"
	default:
		return UploadGrantStatusMinted
	}
}

// GetPK returns the partition key for this upload grant.
func (g *UploadGrant) GetPK() string {
	return g.PK
}

// GetSK returns the sort key for this upload grant.
func (g *UploadGrant) GetSK() string {
	return g.SK
}
