package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func canonicalDigest(hexBody string) string {
	body := strings.TrimSpace(hexBody)
	if len(body) < 64 {
		body = strings.Repeat("0", 64-len(body)) + body
	}
	return "sha256:" + body[:64]
}

func testPromoPackage() *PromoPackage {
	return &PromoPackage{
		PackageID:  "pkg-1",
		OwnerID:    "alice",
		ArticleID:  "https://example.com/articles/hello",
		PostText:   "Read our launch article",
		Visibility: PromoPackageVisibilityPublic,
		Assets: []PromoPackageAsset{
			{MediaID: "media-1", ContentHash: canonicalDigest("a"), PublishedURL: "https://cdn.example/published/a.png"},
			{MediaID: "media-2", ContentHash: canonicalDigest("b"), PublishedURL: "https://cdn.example/published/b.png"},
		},
		Status:    PromoPackageStatusDraft,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestPromoPackageUpdateKeys(t *testing.T) {
	p := testPromoPackage()
	require.NoError(t, p.UpdateKeys())
	require.Equal(t, "USER#alice#PROMO#PACKAGE", p.PK)
	require.Equal(t, "PACKAGE#pkg-1", p.SK)
	require.Equal(t, "USER#alice#PROMO#PACKAGE", p.GetPK())
	require.Equal(t, "PACKAGE#pkg-1", p.GetSK())

	missing := &PromoPackage{PackageID: "pkg-2"}
	require.ErrorContains(t, missing.UpdateKeys(), "ownerID")
	missing = &PromoPackage{OwnerID: "bob"}
	require.ErrorContains(t, missing.UpdateKeys(), "packageID")
}

func TestPromoPackageIsReleased(t *testing.T) {
	p := testPromoPackage()
	require.False(t, p.IsReleased())
	now := time.Now().UTC()
	p.Status = PromoPackageStatusReleased
	p.ReleasedStatusID = "status-1"
	p.ReleasedAt = &now
	require.True(t, p.IsReleased())
	p.ReleasedStatusID = ""
	require.False(t, p.IsReleased(), "missing released status id never counts as released")
}

func TestNormalizePromoPackageVisibility(t *testing.T) {
	got, err := NormalizePromoPackageVisibility("  PUBLIC ")
	require.NoError(t, err)
	require.Equal(t, PromoPackageVisibilityPublic, got)
	got, err = NormalizePromoPackageVisibility("unlisted")
	require.NoError(t, err)
	require.Equal(t, PromoPackageVisibilityUnlisted, got)
	for _, rejected := range []string{"private", "direct", "", "followers"} {
		_, err = NormalizePromoPackageVisibility(rejected)
		require.ErrorContains(t, err, "public or unlisted", "visibility %q must be rejected", rejected)
	}
}

func TestNormalizePromoPackageAssets(t *testing.T) {
	valid := []PromoPackageAsset{
		{MediaID: "m1", ContentHash: canonicalDigest("1"), PublishedURL: "https://cdn.example/p/1.png"},
		{MediaID: "m2", ContentHash: canonicalDigest("2")},
	}
	got, err := NormalizePromoPackageAssets(valid)
	require.NoError(t, err)
	require.Equal(t, "m1", got[0].MediaID)
	require.Equal(t, canonicalDigest("1"), got[0].ContentHash)

	// duplicates rejected
	dup := append([]PromoPackageAsset(nil), valid...)
	dup = append(dup, PromoPackageAsset{MediaID: "m1", ContentHash: canonicalDigest("9")})
	_, err = NormalizePromoPackageAssets(dup)
	require.ErrorContains(t, err, "bound more than once")

	// missing media id
	_, err = NormalizePromoPackageAssets([]PromoPackageAsset{{ContentHash: canonicalDigest("1")}})
	require.ErrorContains(t, err, "media ID is required")

	// non-canonical digest rejected
	_, err = NormalizePromoPackageAssets([]PromoPackageAsset{{MediaID: "m1", ContentHash: "md5:abc"}})
	require.ErrorContains(t, err, "canonical sha256 digest")

	// malformed URL rejected
	_, err = NormalizePromoPackageAssets([]PromoPackageAsset{{MediaID: "m1", ContentHash: canonicalDigest("1"), PublishedURL: "ftp://cdn/x"}})
	require.ErrorContains(t, err, "http(s) URL")

	// limit
	var tooMany []PromoPackageAsset
	for i := 0; i <= maxPromoPackageAssets; i++ {
		tooMany = append(tooMany, PromoPackageAsset{MediaID: "m" + string(rune('0'+i)), ContentHash: canonicalDigest(string(rune('0' + i)))})
	}
	_, err = NormalizePromoPackageAssets(tooMany)
	require.ErrorContains(t, err, "asset count exceeds")
}

func TestIsCanonicalSHA256Digest(t *testing.T) {
	require.True(t, IsCanonicalSHA256Digest("sha256:"+strings.Repeat("a", 64)))
	require.False(t, IsCanonicalSHA256Digest(""))
	require.False(t, IsCanonicalSHA256Digest("sha256:"+strings.Repeat("a", 63)))
	require.False(t, IsCanonicalSHA256Digest("sha256:"+strings.Repeat("A", 64)), "uppercase hex is not canonical")
	require.False(t, IsCanonicalSHA256Digest("md5:"+strings.Repeat("a", 64)))
	require.False(t, IsCanonicalSHA256Digest("sha256:"+strings.Repeat("g", 64)), "non-hex rejected")
}

func TestPromoPackageContentHashDeterministicAndFieldSensitive(t *testing.T) {
	base := testPromoPackage()
	hash := PromoPackageContentHash(base)
	require.NotEmpty(t, hash)
	require.Equal(t, hash, PromoPackageContentHash(testPromoPackage()), "same content hashes identically")

	// Every content field change re-hashes (stales approvals).
	mutate := func(f func(*PromoPackage)) string {
		p := testPromoPackage()
		f(p)
		return PromoPackageContentHash(p)
	}
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.PostText = "Read our launch article!" }), "post text change re-hashes")
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.Visibility = PromoPackageVisibilityUnlisted }), "visibility change re-hashes")
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.ArticleID = "https://example.com/articles/other" }), "article change re-hashes")
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.Assets[0].ContentHash = canonicalDigest("z") }), "asset digest change re-hashes")
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.Assets[0].MediaID = "media-9" }), "asset identity change re-hashes")

	// Order is meaningful: swapping the two assets re-hashes because the swap
	// changes the attachment order on the outbound post.
	swapped := testPromoPackage()
	swapped.Assets[0], swapped.Assets[1] = swapped.Assets[1], swapped.Assets[0]
	require.NotEqual(t, hash, PromoPackageContentHash(swapped), "asset reorder re-hashes")

	// Adding or removing an asset re-hashes.
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) { p.Assets = p.Assets[:1] }))
	require.NotEqual(t, hash, mutate(func(p *PromoPackage) {
		p.Assets = append(p.Assets, PromoPackageAsset{MediaID: "media-3", ContentHash: canonicalDigest("c")})
	}))
}

func TestPromoPackageContentHashLengthPrefixedBoundaries(t *testing.T) {
	// ["a","bc"] must not hash like ["ab","c"]: length-prefixing keeps field
	// boundaries unambiguous.
	ab := testPromoPackage()
	ab.Assets = []PromoPackageAsset{
		{MediaID: "a", ContentHash: canonicalDigest("a")},
		{MediaID: "bc", ContentHash: canonicalDigest("b")},
	}
	cd := testPromoPackage()
	cd.Assets = []PromoPackageAsset{
		{MediaID: "ab", ContentHash: canonicalDigest("a")},
		{MediaID: "c", ContentHash: canonicalDigest("b")},
	}
	require.NotEqual(t, PromoPackageContentHash(ab), PromoPackageContentHash(cd))
}

func TestPromoReviewGrantUpdateKeysAndActivity(t *testing.T) {
	now := time.Now().UTC()
	g := &PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now}
	require.NoError(t, g.UpdateKeys())
	require.Equal(t, "USER#alice#PROMO#REVIEW", g.PK)
	require.Equal(t, "GRANT#pkg-1#REVIEWER#reviewer", g.SK)
	require.Equal(t, "PROMO#REVIEWER#reviewer", g.GSI2PK)
	require.Contains(t, g.GSI2SK, "OWNER#alice#PACKAGE#pkg-1")

	// Fail-closed: missing expiry is treated as expired.
	require.False(t, g.IsActive(now), "grant without expiry fails closed")

	expires := now.Add(time.Hour)
	g.ExpiresAt = &expires
	require.True(t, g.IsActive(now))

	revoked := now.Add(time.Minute)
	g.RevokedAt = &revoked
	require.False(t, g.IsActive(now), "revoked grant is inactive")

	revokedAt := now.Add(-time.Minute)
	g.RevokedAt = &revokedAt
	g.ExpiresAt = &expires
	require.False(t, g.IsActive(now), "revocation dominates expiry")

	expired := now.Add(-time.Minute)
	g2 := &PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "r", GrantedAt: now, ExpiresAt: &expired}
	require.False(t, g2.IsActive(now), "expired grant fails closed")
	require.True(t, g2.Expired(now))
}

func TestPromoReviewGrantRevokedClearsQueueKeys(t *testing.T) {
	now := time.Now().UTC()
	revoked := now.Add(time.Minute)
	g := &PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "r", GrantedAt: now, RevokedAt: &revoked}
	require.NoError(t, g.UpdateKeys())
	require.Empty(t, g.GSI2PK)
	require.Empty(t, g.GSI2SK)
}

func TestPromoReviewVerdictUpdateKeys(t *testing.T) {
	recorded := time.Now().UTC()
	v := &PromoReviewVerdict{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", Verdict: PromoPackageReviewApproved, ContentHash: canonicalDigest("a"), RecordedAt: recorded}
	require.NoError(t, v.UpdateKeys())
	require.Equal(t, "USER#alice#PROMO#REVIEW", v.PK)
	require.Contains(t, v.SK, "VERDICT#pkg-1#TIME#")
	require.Contains(t, v.SK, "#REVIEWER#reviewer")

	require.ErrorContains(t, (&PromoReviewVerdict{PackageID: "pkg-1", Reviewer: "r", Verdict: PromoPackageReviewApproved}).UpdateKeys(), "ownerID")
}
