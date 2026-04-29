package models

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestFederationModels_UpdateKeys(t *testing.T) {
	t.Run("FederationInstance UpdateKeys", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fi := &FederationInstance{Domain: "example.com", LastSeen: ts}
		fi.UpdateKeys()
		assert.Equal(t, "INSTANCE#example.com", fi.PK)
		assert.Equal(t, "INSTANCE#example.com", fi.SK)
		assert.Equal(t, "FEDERATION_ACTIVE", fi.GSI1PK)
		assert.Equal(t, ts.Format(time.RFC3339), fi.GSI1SK)
		assert.Equal(t, MainTableName, fi.TableName())
	})

	t.Run("FederationCostActivity UpdateKeys uses timestamp and sets TTL", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fca := &FederationCostActivity{
			ID:        "id",
			Domain:    "example.com",
			Timestamp: ts,
		}
		require.NoError(t, fca.UpdateKeys())
		assert.Equal(t, "FEDERATION#example.com#"+ts.Format(common.MonthFormat), fca.PK)
		assert.Contains(t, fca.SK, "ACTIVITY#")
		assert.Equal(t, "FEDERATION_DAILY#"+ts.Format(common.DateFormat), fca.GSI1PK)
		assert.Equal(t, "DOMAIN#example.com#id", fca.GSI1SK)
		assert.Equal(t, ts.Add(90*24*time.Hour).Unix(), fca.TTL)
		assert.Equal(t, MainTableName, fca.TableName())
		assert.Equal(t, fca.PK, fca.GetPK())
		assert.Equal(t, fca.SK, fca.GetSK())
	})

	t.Run("FederationCost UpdateKeys sets cost aggregation keys", func(t *testing.T) {
		fc := &FederationCost{Domain: "example.com"}
		fc.UpdateKeys()
		assert.Contains(t, fc.PK, "FEDERATION_COSTS#")
		assert.Equal(t, "DOMAIN#example.com", fc.SK)
		assert.Equal(t, MainTableName, fc.TableName())
	})

	t.Run("FederationNode UpdateKeys and domain index", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fn := &FederationNode{Domain: "example.com", LastSeen: ts}
		fn.UpdateKeys()
		assert.Equal(t, "FEDERATION_NODE#example.com", fn.PK)
		assert.Equal(t, "NODE", fn.SK)
		assert.Equal(t, "FEDERATION_ACTIVE", fn.GSI1PK)
		assert.Contains(t, fn.GSI1SK, "example.com")
		assert.Equal(t, "DOMAIN#example.com", fn.GSI3PK)
		assert.Equal(t, "FEDERATION_NODE", fn.GSI3SK)
		assert.Equal(t, MainTableName, fn.TableName())
	})

	t.Run("FederationEdge UpdateKeys", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fe := &FederationEdge{
			SourceDomain:   "a",
			TargetDomain:   "b",
			ConnectionType: "follows",
			LastActivity:   ts,
		}
		fe.UpdateKeys()
		assert.Equal(t, "FEDERATION_EDGE#a", fe.PK)
		assert.Equal(t, "b", fe.SK)
		assert.Equal(t, "INSTANCE#a#CONNECTIONS#follows", fe.GSI2PK)
		assert.Contains(t, fe.GSI2SK, "#b")
		assert.Equal(t, MainTableName, fe.TableName())
	})

	t.Run("InstanceMetadata UpdateKeys", func(t *testing.T) {
		im := &InstanceMetadata{Domain: "example.com"}
		im.UpdateKeys()
		assert.Equal(t, "INSTANCE_META#example.com", im.PK)
		assert.Equal(t, SKMetadata, im.SK)
		assert.Equal(t, MainTableName, im.TableName())
	})

	t.Run("InstanceCluster UpdateKeys pads size", func(t *testing.T) {
		ic := &InstanceCluster{ClusterID: "c1", Size: 12}
		ic.UpdateKeys()
		assert.Equal(t, "FEDERATION_CLUSTER#CLUSTERS", ic.PK)
		assert.Equal(t, "c1", ic.SK)
		assert.Equal(t, "CLUSTERS_BY_SIZE", ic.GSI1PK)
		assert.Equal(t, "00012#c1", ic.GSI1SK)
		assert.Equal(t, MainTableName, ic.TableName())
	})

	t.Run("InstanceConnection UpdateKeys uses connection patterns", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		ic := &InstanceConnection{
			Domain:         "example.com",
			TargetDomain:   "remote",
			ConnectionType: "mentions",
			LastActivity:   ts,
		}
		ic.UpdateKeys()
		assert.Equal(t, "CONNECTION#example.com", ic.PK)
		assert.Equal(t, "mentions#remote", ic.SK)
		assert.Equal(t, "INSTANCE#example.com#CONNECTIONS#mentions", ic.GSI2PK)
		assert.Contains(t, ic.GSI2SK, "#remote")
		assert.Equal(t, MainTableName, ic.TableName())
	})

	t.Run("timeseries window records", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fw := NewFederationTimeseriesWindow(ts, ts)
		assert.Equal(t, "TIMESERIES#FEDERATION", fw.PK)
		assert.Equal(t, "WINDOW#2023-11-14T22:13:20Z", fw.SK)
		assert.Equal(t, "FederationTimeseries", fw.Type)
		assert.Equal(t, MainTableName, fw.TableName())

		iw := NewInstanceTimeseriesWindow("example.com", ts, ts)
		assert.Equal(t, "TIMESERIES#INSTANCE#example.com", iw.PK)
		assert.Equal(t, "WINDOW#2023-11-14T22:13:20Z", iw.SK)
		assert.Equal(t, "InstanceTimeseries", iw.Type)
		assert.Equal(t, MainTableName, iw.TableName())
	})

	t.Run("FederationHealthReport is a computed type", func(t *testing.T) {
		assert.Equal(t, MainTableName, (FederationHealthReport{}).TableName())
	})
}

func TestFederationTimeseriesModelsRegisterWithTableTheory(t *testing.T) {
	db, err := tabletheory.New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	ts := time.Unix(1700000000, 0).UTC()

	federationWindow := NewFederationTimeseriesWindow(ts, ts)
	err = db.Model(federationWindow).UpdateBuilder().Set("UpdatedAt", federationWindow.UpdatedAt).Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "partition key PK is required for update")
	require.NotContains(t, err.Error(), "failed to register model")

	instanceWindow := NewInstanceTimeseriesWindow("example.com", ts, ts)
	err = db.Model(instanceWindow).UpdateBuilder().Set("UpdatedAt", instanceWindow.UpdatedAt).Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "partition key PK is required for update")
	require.NotContains(t, err.Error(), "failed to register model")
}
