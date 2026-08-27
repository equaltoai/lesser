package cost

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// scanForbiddingCostDB mirrors the storage-repository wave helper: any read
// that issues a DynamoDB Scan fails the test. Batch E (umbrella #1469) rerouted
// ListInstanceConfigs to the GSI3 global listing key
// (INSTANCE_CONFIGS#ALL / INSTANCE#<domain>) on FederationInstanceConfigTracking.
type scanForbiddingCostDB struct {
	*fakedb.Fake
	scanCalls int
}

func (s *scanForbiddingCostDB) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	return nil, errors.New("full-table scan forbidden on event path")
}

func TestScanFreeWave_Event_ListInstanceConfigs(t *testing.T) {
	ctx := context.Background()
	f := &scanForbiddingCostDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, f)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.FederationInstanceConfigTracking{}))

	cfg := &models.FederationInstanceConfigTracking{Domain: "remote.example", Tier: models.FederationTierFree}
	cfg.UpdateKeys() // sets GSI3 INSTANCE_CONFIGS#ALL
	require.NoError(t, db.WithContext(ctx).Model(cfg).Create())

	adapter := NewRepositoryAdapter(db, zap.NewNop(), nil)
	got, err := adapter.ListInstanceConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "remote.example", got[0].Domain)
	require.Zero(t, f.scanCalls, "ListInstanceConfigs must not scan")
}

var _ core.DB // pin core import

// SaveInstanceConfig — the single FederationInstanceConfigTracking writer —
// maintains the GSI3 global listing key on every save.
func TestScanFreeWave_Event_SaveInstanceConfigMaintainsGSI3(t *testing.T) {
	ctx := context.Background()
	f := &scanForbiddingCostDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, f)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.FederationInstanceConfigTracking{}))

	adapter := NewRepositoryAdapter(db, zap.NewNop(), nil)
	require.NoError(t, adapter.SaveInstanceConfig(ctx, &InstanceConfig{Domain: "remote.example", Tier: TierStandard}))

	var got models.FederationInstanceConfigTracking
	require.NoError(t, db.WithContext(ctx).Model(&got).
		Where("PK", "=", "INSTANCE#remote.example").
		Where("SK", "=", "CONFIG").
		First(&got))
	require.Equal(t, "INSTANCE_CONFIGS#ALL", got.GSI3PK)
	require.Equal(t, "INSTANCE#remote.example", got.GSI3SK)
	require.Zero(t, f.scanCalls, "SaveInstanceConfig must not scan")
}
