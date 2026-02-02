package models

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricRecord_UpdateKeys_TTL_Buckets_AndBuilders(t *testing.T) {
	ts := time.Date(2024, 6, 15, 10, 7, 45, 0, time.UTC)

	t.Run("UpdateKeys populates PK/SK and GSIs for raw aggregation", func(t *testing.T) {
		rec := &MetricRecord{
			MetricType:       "request",
			ServiceName:      "api",
			Timestamp:        ts,
			AggregationLevel: "raw",
			Dimensions:       map[string]string{},
		}

		before := time.Now()
		require.NoError(t, rec.UpdateKeys())

		assert.Equal(t, "METRICS#request#2024-06-15T10:07", rec.PK)
		assert.Equal(t, ts.Format(time.RFC3339), rec.SK)

		assert.Equal(t, "SERVICE#api", rec.GSI1PK)
		assert.Equal(t, "TIMESTAMP#"+ts.Format(time.RFC3339), rec.GSI1SK)
		assert.Equal(t, "METRIC_TYPE#request", rec.GSI2PK)
		assert.Equal(t, "DATE#"+ts.Format(common.DateFormat), rec.GSI3PK)
		assert.Equal(t, "AGGREGATION#raw", rec.GSI4PK)

		assert.Equal(t, rec.PK, rec.GetPK())
		assert.Equal(t, rec.SK, rec.GetSK())
		assert.Equal(t, MainTableName, rec.TableName())

		assert.WithinDuration(t, before.Add(30*24*time.Hour), time.Unix(rec.TTL, 0), 2*time.Second)
	})

	t.Run("5min buckets truncate timestamp and keep longer TTL", func(t *testing.T) {
		rec := &MetricRecord{
			MetricType:       "latency",
			ServiceName:      "api",
			Timestamp:        ts,
			AggregationLevel: "5min",
		}
		before := time.Now()
		require.NoError(t, rec.UpdateKeys())
		assert.Equal(t, "METRICS#latency#2024-06-15T10:05", rec.PK)
		assert.WithinDuration(t, before.Add(90*24*time.Hour), time.Unix(rec.TTL, 0), 2*time.Second)
	})

	t.Run("hourly and daily aggregation produce coarser buckets and long TTL", func(t *testing.T) {
		hourly := &MetricRecord{
			MetricType:       "latency",
			ServiceName:      "api",
			Timestamp:        ts,
			AggregationLevel: PeriodHourly,
		}
		daily := &MetricRecord{
			MetricType:       "latency",
			ServiceName:      "api",
			Timestamp:        ts,
			AggregationLevel: PeriodDaily,
		}
		before := time.Now()
		require.NoError(t, hourly.UpdateKeys())
		require.NoError(t, daily.UpdateKeys())

		assert.Equal(t, "METRICS#latency#2024-06-15T10", hourly.PK)
		assert.Equal(t, "METRICS#latency#2024-06-15", daily.PK)
		assert.WithinDuration(t, before.Add(365*24*time.Hour), time.Unix(hourly.TTL, 0), 2*time.Second)
		assert.WithinDuration(t, before.Add(365*24*time.Hour), time.Unix(daily.TTL, 0), 2*time.Second)
	})

	t.Run("Validate enforces timestamp and aggregation level", func(t *testing.T) {
		rec := &MetricRecord{AggregationLevel: "raw"}
		assert.ErrorIs(t, rec.Validate(), ErrTimestampRequired)

		rec = &MetricRecord{Timestamp: ts, AggregationLevel: "unknown"}
		assert.ErrorIs(t, rec.Validate(), ErrInvalidAggregationLevel)
	})

	t.Run("BeforeCreate sets timestamps and validates", func(t *testing.T) {
		rec := &MetricRecord{
			MetricType:       "request",
			ServiceName:      "api",
			AggregationLevel: "raw",
		}
		require.NoError(t, rec.BeforeCreate())
		assert.False(t, rec.Timestamp.IsZero())
		assert.False(t, rec.CreatedAt.IsZero())
		assert.True(t, rec.CreatedAt.Equal(rec.UpdatedAt))
	})

	t.Run("AddDimension initializes map", func(t *testing.T) {
		rec := &MetricRecord{}
		rec.AddDimension("region", "us-east-1")
		assert.Equal(t, "us-east-1", rec.Dimensions["region"])
	})

	t.Run("MetricRecordBuilder wires fields", func(t *testing.T) {
		rec := NewMetricRecordBuilder().
			ForService("api").
			OfType("latency").
			WithAggregationLevel("raw").
			WithTimestamp(ts).
			WithStats(2, 10, 3, 7, 5, 6, 7).
			WithUnit("ms").
			WithDimension("region", "us-east-1").
			Build()

		assert.Equal(t, "api", rec.ServiceName)
		assert.Equal(t, "latency", rec.MetricType)
		assert.Equal(t, "raw", rec.AggregationLevel)
		assert.Equal(t, ts, rec.Timestamp)
		assert.Equal(t, "ms", rec.Unit)
		assert.Equal(t, "us-east-1", rec.Dimensions["region"])
		assert.Equal(t, MainTableName, (MetricRecordBuilder{}).TableName())
	})
}
