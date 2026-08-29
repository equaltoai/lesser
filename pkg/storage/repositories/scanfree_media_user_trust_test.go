package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// TestMediaRepository_KeyedReads_NeverScan pins that the media list reads are
// keyed queries (partition-key equality on gsiNPK), never a Scan, and that a
// limit <= 0 is clamped instead of reading an unbounded partition.
// Media-job / spending / transcoding reads are covered by the mock-based
// tests, which fail on any Scan call: those models carry a pointer-typed ttl
// tag that tabletheory v3.0.6 rejects at model registration, so they cannot be
// hosted by the fakedb seam.
func TestMediaRepository_KeyedReads_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Media{})

	const userID = "alice1234"
	seedMediaRows(t, ctx, db, userID, 3)

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)

	media, err := repo.GetMediaByUser(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, media, 3)

	// limit 0 is clamped to the default, still keyed.
	media, err = repo.GetMediaByUser(ctx, userID, 0)
	require.NoError(t, err)
	require.Len(t, media, 3)

	media, err = repo.GetMediaByStatus(ctx, models.StatusPending, 0)
	require.NoError(t, err)
	require.Len(t, media, 3)

	// Content type queries match the normalized gsi3 partition prefix.
	media, err = repo.GetMediaByContentType(ctx, "image", 10)
	require.NoError(t, err)
	require.Len(t, media, 3)

	require.Zero(t, s.scanCalls, "no scan may run on a keyed media read")
}

// TestMediaRepository_UnmarkAllMediaAsSensitive_PaginatesWithoutScan pins that
// unmarking a user's media walks every page via cursor (100+ rows => >1 page)
// and issues zero scans.
func TestMediaRepository_UnmarkAllMediaAsSensitive_PaginatesWithoutScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Media{})

	const userID = "alice1234"
	ids := seedMediaRows(t, ctx, db, userID, 120)

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)

	require.NoError(t, repo.UnmarkAllMediaAsSensitive(ctx, userID))
	require.Zero(t, s.scanCalls, "no scan may run while unmarking all media")

	// Every seeded row was visited and unmarked.
	for _, id := range ids {
		m, err := repo.GetMedia(ctx, id)
		require.NoError(t, err)
		require.False(t, m.IsNSFW, "media %s must be unmarked", id)
	}
}

// TestUserRepository_VouchReads_NeverScan pins GetVouch, the GSI vouch list
// helpers, and the monthly count to keyed queries.
func TestUserRepository_VouchReads_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.Vouch{})

	seedVouch(t, ctx, db, "vouch-1", "alice", "bob", time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC))
	seedVouch(t, ctx, db, "vouch-2", "alice", "carol", time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC))
	seedVouch(t, ctx, db, "vouch-3", "dave", "bob", time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC))

	repo := NewUserRepository(db, "test-table", zap.NewNop())

	vouch, err := repo.GetVouch(ctx, "vouch-1")
	require.NoError(t, err)
	require.NotNil(t, vouch)
	require.Equal(t, "alice", vouch.From)

	// Point lookup on a missing key keeps the NotFound contract.
	missing, err := repo.GetVouch(ctx, "vouch-missing")
	require.Error(t, err)
	require.Nil(t, missing)

	byActor, err := repo.GetVouchesByActor(ctx, "alice", false)
	require.NoError(t, err)
	require.Len(t, byActor, 2)

	forActor, err := repo.GetVouchesForActor(ctx, "bob", false)
	require.NoError(t, err)
	require.Len(t, forActor, 2)

	// The model layer stamps CreatedAt with the write time, so the count lands
	// in the current UTC month; a historical month is never counted.
	count, err := repo.GetMonthlyVouchCount(ctx, "alice", 2020, time.January)
	require.NoError(t, err)
	require.Zero(t, count)

	now := time.Now().UTC()
	count, err = repo.GetMonthlyVouchCount(ctx, "alice", now.Year(), now.Month())
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.Zero(t, s.scanCalls, "no scan may run on a vouch read")
}

// TestUserRepository_TrustReads_NeverScan pins the rewritten per-category
// trust list reads (PK / gsi1PK equality) with SK cursors and zero scans.
func TestUserRepository_TrustReads_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.TrustRelationship{})

	seedTrustRelationship(t, ctx, db, "truster1", "trustee1", models.TrustCategoryGeneral)
	seedTrustRelationship(t, ctx, db, "truster1", "trustee2", models.TrustCategoryGeneral)
	seedTrustRelationship(t, ctx, db, "truster1", "trustee3", models.TrustCategoryContent)
	seedTrustRelationship(t, ctx, db, "truster2", "trustee1", models.TrustCategoryGeneral)

	repo := NewUserRepository(db, "test-table", zap.NewNop())

	rels, nextCursor, err := repo.GetTrustRelationships(ctx, "truster1", 10, "")
	require.NoError(t, err)
	require.Len(t, rels, 3)
	require.Empty(t, nextCursor)
	for _, rel := range rels {
		require.Equal(t, "truster1", rel.TrusterID)
	}

	// Page 1 of 2 with the fixed category order: content rows first, then
	// general rows sorted by SK (TRUSTEE#trustee1 before TRUSTEE#trustee2).
	page1, cursor, err := repo.GetTrustRelationships(ctx, "truster1", 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, cursor)

	page2, cursor, err := repo.GetTrustRelationships(ctx, "truster1", 2, cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, cursor)
	require.Equal(t, "trustee2", page2[0].TrusteeID)

	// Invalid cursor surfaces an error, not a silent full read.
	_, _, err = repo.GetTrustRelationships(ctx, "truster1", 10, "%%not-a-cursor%%")
	require.Error(t, err)

	trustedBy, nextCursor, err := repo.GetTrustedByRelationships(ctx, "trustee1", 10, "")
	require.NoError(t, err)
	require.Len(t, trustedBy, 2)
	require.Empty(t, nextCursor)

	_, _, err = repo.GetTrustedByRelationships(ctx, "trustee1", 10, "%%not-a-cursor%%")
	require.Error(t, err)

	require.Zero(t, s.scanCalls, "no scan may run on a trust relationship read")
}

// TestTrustRepository_GetAllTrustRelationships_NeverScan pins the admin
// visualization listing to the gsi3 global key. Rows written before the gsi3
// key existed (legacy, pending backfill) must not appear until backfilled.
func TestTrustRepository_GetAllTrustRelationships_NeverScan(t *testing.T) {
	ctx := context.Background()
	db, s := newScanForbiddingTestDB(t, &models.TrustRelationship{})

	seedTrustRelationship(t, ctx, db, "truster1", "trustee1", models.TrustCategoryGeneral)
	seedTrustRelationship(t, ctx, db, "truster2", "trustee2", models.TrustCategoryContent)
	seedTrustRelationship(t, ctx, db, "truster3", "trustee3", models.TrustCategoryBehavior)

	// Legacy row without the gsi3 global-listing key (pre-backfill shape).
	legacy := &models.TrustRelationship{
		TrusterID: "legacy-t", TrusteeID: "legacy-e", Category: models.TrustCategoryTechnical,
		Score: 0.1, Confidence: 0.1,
	}
	require.NoError(t, legacy.UpdateKeys())
	legacy.GSI3PK = ""
	legacy.GSI3SK = ""
	require.NoError(t, db.WithContext(ctx).Model(legacy).Create())

	repo := NewTrustRepository(db, "test-table", zap.NewNop(), nil)

	rels, err := repo.GetAllTrustRelationships(ctx, 10)
	require.NoError(t, err)
	require.Len(t, rels, 3)
	for _, rel := range rels {
		require.NotEqual(t, "legacy-t", rel.TrusterID, "legacy rows must be excluded until backfilled")
	}

	require.Zero(t, s.scanCalls, "no scan may run on the trust relationship listing")
}

// ===== seeding helpers (fakedb.Create does not run model hooks) =====

// seedMediaRows writes n media rows for one user with distinct gsi1 sort keys
// so cursor pagination exercises more than one page, and returns their IDs.
func seedMediaRows(t *testing.T, ctx context.Context, db core.DB, userID string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('0'+i%10)) + string(rune('0'+(i/10)%10))
		if i >= 676 {
			id = string(rune('0'+i/676)) + id
		}
		media := &models.Media{
			MediaID:     id,
			UserID:      userID,
			ContentType: "image/png",
			FileSize:    1,
			Status:      models.StatusPending,
			Version:     "original",
			IsNSFW:      true,
		}
		require.NoError(t, media.BeforeCreate())
		require.NoError(t, db.WithContext(ctx).Model(media).Create())
		ids = append(ids, id)
	}
	return ids
}

func seedVouch(t *testing.T, ctx context.Context, db core.DB, vouchID, from, to string, createdAt time.Time) {
	t.Helper()
	payload, err := json.Marshal(storage.Vouch{
		ID:        vouchID,
		From:      from,
		To:        to,
		Active:    true,
		CreatedAt: createdAt,
	})
	require.NoError(t, err)

	vouch := &models.Vouch{VouchData: string(payload)}
	vouch.UpdateKeys(vouchID, from, to, true, createdAt, createdAt.Add(24*time.Hour))
	require.NoError(t, db.WithContext(ctx).Model(vouch).Create())
}

func seedTrustRelationship(t *testing.T, ctx context.Context, db core.DB, trusterID, trusteeID string, category models.TrustCategory) {
	t.Helper()
	rel := &models.TrustRelationship{
		ID:         "trust_" + trusterID + "_" + trusteeID,
		TrusterID:  trusterID,
		TrusteeID:  trusteeID,
		Category:   category,
		Score:      0.5,
		Confidence: 0.8,
		Type:       "RELATIONSHIP",
	}
	require.NoError(t, rel.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(rel).Create())
}
