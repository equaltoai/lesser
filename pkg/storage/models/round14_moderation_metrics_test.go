package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationMetricsModels_UpdateKeys(t *testing.T) {
	t.Run("ModerationMetricsEntry UpdateKeys sets keys, marker, and TTL", func(t *testing.T) {
		m := &ModerationMetricsEntry{
			MetricType: "decision:allow",
			Hour:       "01",
			Date:       "2024-01-01",
		}
		before := time.Now()
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "METRICS#2024-01-01", m.PK)
		assert.Equal(t, "STATS#01#decision:allow", m.SK)
		assert.Equal(t, "METRIC_TYPE#decision:allow", m.GSI1PK)
		assert.Equal(t, "DATE#2024-01-01#01", m.GSI1SK)
		assert.Equal(t, "METRIC_STATS", m.Type)
		assert.True(t, time.Unix(m.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("ModerationFalsePositive UpdateKeys sets keys, marker, and TTL", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fp := &ModerationFalsePositive{
			ContentID: "c1",
			Date:      "2024-01-01",
			Timestamp: ts,
		}
		require.NoError(t, fp.UpdateKeys())
		assert.Equal(t, "METRICS#2024-01-01", fp.PK)
		assert.Equal(t, "FP#c1", fp.SK)
		assert.Equal(t, "FALSE_POSITIVES", fp.GSI1PK)
		assert.Contains(t, fp.GSI1SK, "DATE#2024-01-01#")
		assert.Equal(t, "FALSE_POSITIVE", fp.Type)
		assert.Equal(t, MainTableName, fp.TableName())
		assert.Equal(t, fp.PK, fp.GetPK())
		assert.Equal(t, fp.SK, fp.GetSK())
	})

	t.Run("ModerationDecisionSample UpdateKeys sets keys, marker, and TTL", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		ds := &ModerationDecisionSample{
			ContentID: "c1",
			Decision:  "allow",
			Date:      "2024-01-01",
			Timestamp: ts,
		}
		before := time.Now()
		require.NoError(t, ds.UpdateKeys())
		assert.Equal(t, "SAMPLES#2024-01-01", ds.PK)
		assert.Contains(t, ds.SK, "#c1")
		assert.Equal(t, "DECISION#allow", ds.GSI1PK)
		assert.Contains(t, ds.GSI1SK, "DATE#2024-01-01#")
		assert.Equal(t, "DECISION_SAMPLE", ds.Type)
		assert.True(t, time.Unix(ds.TTL, 0).After(before.Add(29*24*time.Hour)))
		assert.Equal(t, MainTableName, ds.TableName())
		assert.Equal(t, ds.PK, ds.GetPK())
		assert.Equal(t, ds.SK, ds.GetSK())
	})

	t.Run("ModerationPatternStats UpdateKeys sets padded ranking and TTL", func(t *testing.T) {
		ps := &ModerationPatternStats{
			PatternID: "p1",
			HitCount:  12,
		}
		before := time.Now()
		require.NoError(t, ps.UpdateKeys())
		assert.Equal(t, "PATTERN_STATS#p1", ps.PK)
		assert.Equal(t, SKStats, ps.SK)
		assert.Equal(t, "PATTERN_HITS", ps.GSI1PK)
		assert.Equal(t, "00000000000000000012#p1", ps.GSI1SK)
		assert.Equal(t, "PATTERN_STATS", ps.Type)
		assert.True(t, time.Unix(ps.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.Equal(t, MainTableName, ps.TableName())
		assert.Equal(t, ps.PK, ps.GetPK())
		assert.Equal(t, ps.SK, ps.GetSK())
	})

	t.Run("Helper structs TableName methods", func(t *testing.T) {
		assert.Equal(t, MainTableName, (ModerationMetricsTimeRange{}).TableName())
		assert.Equal(t, MainTableName, (ModerationMetricsStats{}).TableName())
		assert.Equal(t, MainTableName, (RealtimeStats{}).TableName())
		assert.Equal(t, MainTableName, (PatternStats{}).TableName())
	})
}
