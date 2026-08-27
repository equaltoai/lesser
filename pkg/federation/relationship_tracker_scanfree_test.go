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
