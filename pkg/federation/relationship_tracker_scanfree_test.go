package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// scanForbiddingFederationDB mirrors the storage-repository wave helper: any
// read that issues a DynamoDB Scan fails the test. Batch E (umbrella #1469)
// converted all four relationship_tracker.go `.Scan` sites (all keyed) to
// `.All`; this pins the two exported read surfaces.
type scanForbiddingFederationDB struct {
	*fakedb.Fake
	scanCalls int
}

func (s *scanForbiddingFederationDB) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	return nil, errors.New("full-table scan forbidden on event path")
}

func newScanFreeFederationTracker(t *testing.T, modelTypes ...any) (*RelationshipTracker, *scanForbiddingFederationDB) {
	t.Helper()
	f := &scanForbiddingFederationDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, f)
	require.NoError(t, err)
	for _, m := range modelTypes {
		require.NoError(t, db.CreateTable(m))
	}
	return NewRelationshipTracker(nil, db, zap.NewNop()), f
}

// 1) GetRelationshipsByState — keyed gsi1 (FEDERATION_STATE#<state>).
func TestScanFreeWave_Event_GetRelationshipsByState(t *testing.T) {
	ctx := context.Background()
	rt, s := newScanFreeFederationTracker(t, &models.FederationRelationship{})

	now := time.Now()
	for _, state := range []models.RelationshipState{models.StateActive, models.StateIdle} {
		rel := &models.FederationRelationship{
			ID:               string(state),
			UserID:           "user-1",
			TargetInstance:   "remote.example",
			RelationshipType: "follow",
			State:            state,
			LastActivity:     now,
			FirstSeen:        now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		rel.UpdateKeys() // sets GSI1 FEDERATION_STATE#<state>
		require.NoError(t, rt.db.WithContext(ctx).Model(rel).Create())
	}

	got, err := rt.GetRelationshipsByState(ctx, models.StateActive, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, models.StateActive, got[0].State)
	require.Zero(t, s.scanCalls, "GetRelationshipsByState must not scan")
}

// 2) GetUserRelationships — keyed PK (USER#<userID>#FEDERATION).
func TestScanFreeWave_Event_GetUserRelationships(t *testing.T) {
	ctx := context.Background()
	rt, s := newScanFreeFederationTracker(t, &models.FederationRelationship{})

	now := time.Now()
	rel := &models.FederationRelationship{
		ID:               "rel-1",
		UserID:           "user-1",
		TargetInstance:   "remote.example",
		RelationshipType: "follow",
		State:            models.StateActive,
		LastActivity:     now,
		FirstSeen:        now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	rel.UpdateKeys() // sets PK USER#user-1#FEDERATION
	require.NoError(t, rt.db.WithContext(ctx).Model(rel).Create())

	got, err := rt.GetUserRelationships(ctx, "user-1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "rel-1", got[0].ID)
	require.Zero(t, s.scanCalls, "GetUserRelationships must not scan")
}

// 3) processStateTransitions (daemon tick) — keyed gsi1
// (FEDERATION_STATE#<state>, bounded per-tick batches).
func TestScanFreeWave_Event_ProcessStateTransitions(t *testing.T) {
	ctx := context.Background()
	rt, s := newScanFreeFederationTracker(t, &models.FederationRelationship{})

	now := time.Now()
	seed := func(id string, lastActivity time.Time) {
		rel := &models.FederationRelationship{
			ID:               id,
			UserID:           "user-" + id,
			TargetInstance:   "remote.example",
			RelationshipType: "follow",
			State:            models.StateActive,
			LastActivity:     lastActivity,
			FirstSeen:        now.Add(-200 * 24 * time.Hour),
			CreatedAt:        now.Add(-200 * 24 * time.Hour),
			UpdatedAt:        now,
		}
		rel.UpdateKeys() // sets GSI1 FEDERATION_STATE#active
		require.NoError(t, rt.db.WithContext(ctx).Model(rel).Create())
	}
	seed("rel-idle", now.Add(-8*24*time.Hour)) // stale (>7d) → should transition to idle
	seed("rel-fresh", now.Add(-1*time.Hour))   // fresh → stays active

	require.NoError(t, rt.processStateTransitions(ctx))
	require.Zero(t, s.scanCalls, "processStateTransitions must not scan")

	idle, err := rt.GetRelationshipsByState(ctx, models.StateIdle, 10)
	require.NoError(t, err)
	require.Len(t, idle, 1, "stale active relationship transitioned to idle")
	require.Equal(t, "rel-idle", idle[0].ID)

	active, err := rt.GetRelationshipsByState(ctx, models.StateActive, 10)
	require.NoError(t, err)
	require.Len(t, active, 1, "fresh relationship stayed active")
	require.Equal(t, "rel-fresh", active[0].ID)
}

// 4) archiveDormantRelationships (daemon tick) — keyed gsi1
// (FEDERATION_STATE#dormant with a gsi1SK < cutoff range).
func TestScanFreeWave_Event_ArchiveDormantRelationships(t *testing.T) {
	ctx := context.Background()
	rt, s := newScanFreeFederationTracker(t, &models.FederationRelationship{}, &models.FederationRelationshipIndex{})

	now := time.Now()
	dormant := &models.FederationRelationship{
		ID:               "rel-dormant",
		UserID:           "user-1",
		TargetInstance:   "remote.example",
		RelationshipType: "follow",
		State:            models.StateDormant,
		LastActivity:     now.Add(-120 * 24 * time.Hour), // older than archiveAfter (90d)
		FirstSeen:        now.Add(-200 * 24 * time.Hour),
		CreatedAt:        now.Add(-200 * 24 * time.Hour),
		UpdatedAt:        now,
	}
	dormant.UpdateKeys() // sets GSI1 FEDERATION_STATE#dormant / <unix>#target#id
	require.NoError(t, rt.db.WithContext(ctx).Model(dormant).Create())

	fresh := &models.FederationRelationship{
		ID:               "rel-fresh",
		UserID:           "user-2",
		TargetInstance:   "remote.example",
		RelationshipType: "follow",
		State:            models.StateDormant,
		LastActivity:     now.Add(-1 * time.Hour), // within the cutoff → must survive
		FirstSeen:        now.Add(-200 * 24 * time.Hour),
		CreatedAt:        now.Add(-200 * 24 * time.Hour),
		UpdatedAt:        now,
	}
	fresh.UpdateKeys()
	require.NoError(t, rt.db.WithContext(ctx).Model(fresh).Create())

	require.NoError(t, rt.archiveDormantRelationships(ctx))
	require.Zero(t, s.scanCalls, "archiveDormantRelationships must not scan")

	// The stale dormant relationship is archived (index row) and deleted.
	var archived models.FederationRelationship
	require.Error(t, rt.db.WithContext(ctx).Model(&archived).
		Where("PK", "=", "USER#user-1#FEDERATION").
		Where("SK", "=", dormant.SK).
		First(&archived), "archived relationship full record must be deleted")

	var index models.FederationRelationshipIndex
	require.NoError(t, rt.db.WithContext(ctx).Model(&index).
		Where("PK", "=", "FEDERATION_REL_INDEX#rel-dormant").
		Where("SK", "=", "INDEX").
		First(&index))
	require.Equal(t, models.StateArchived, index.State)

	// The fresh dormant relationship survives untouched.
	var kept models.FederationRelationship
	require.NoError(t, rt.db.WithContext(ctx).Model(&kept).
		Where("PK", "=", "USER#user-2#FEDERATION").
		Where("SK", "=", fresh.SK).
		First(&kept))
	require.Equal(t, "rel-fresh", kept.ID)
}
