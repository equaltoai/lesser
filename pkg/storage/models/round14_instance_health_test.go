package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceHealth_KeysAndScoring(t *testing.T) {
	t.Run("UpdateKeys sets keys and TTL", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		h := &InstanceHealth{
			Domain:    "example.com",
			Timestamp: ts,
		}
		require.NoError(t, h.UpdateKeys())
		assert.Equal(t, "INSTANCE#example.com", h.PK)
		assert.Contains(t, h.SK, "HEALTH#")
		assert.Equal(t, ts.Add(7*24*time.Hour).Unix(), h.TTL)
		assert.Equal(t, MainTableName, h.TableName())
		assert.Equal(t, h.PK, h.GetPK())
		assert.Equal(t, h.SK, h.GetSK())
	})

	t.Run("NewInstanceHealth sets defaults and keys", func(t *testing.T) {
		h := NewInstanceHealth("example.com")
		require.NotNil(t, h)
		assert.Equal(t, "example.com", h.Domain)
		assert.False(t, h.Timestamp.IsZero())
		assert.Equal(t, "serverless-v1", h.CheckerVersion)
		assert.Contains(t, h.UserAgent, "Lesser/")
		assert.Contains(t, h.PK, "INSTANCE#example.com")
	})

	t.Run("IsHealthy and IsCritical", func(t *testing.T) {
		h := &InstanceHealth{Reachable: true, StatusCode: 200, ErrorRate: 0.05}
		assert.True(t, h.IsHealthy())
		assert.False(t, h.IsCritical())

		h = &InstanceHealth{Reachable: false, StatusCode: 200, ErrorRate: 0}
		assert.False(t, h.IsHealthy())
		assert.True(t, h.IsCritical())

		h = &InstanceHealth{Reachable: true, StatusCode: 503, ErrorRate: 0}
		assert.True(t, h.IsCritical())

		h = &InstanceHealth{Reachable: true, StatusCode: 200, ErrorRate: 0.6}
		assert.True(t, h.IsCritical())
	})

	t.Run("GetHealthScore applies penalties and clamps to [0,100]", func(t *testing.T) {
		h := &InstanceHealth{Reachable: false}
		assert.Equal(t, 0.0, h.GetHealthScore())

		h = &InstanceHealth{Reachable: true, StatusCode: 200, ResponseTime: 500 * time.Millisecond, ErrorRate: 0, InboxBacklog: 0}
		assert.Equal(t, 100.0, h.GetHealthScore())

		h = &InstanceHealth{
			Reachable:    true,
			StatusCode:   500,
			ResponseTime: 2 * time.Second,
			ErrorRate:    0.5,
			InboxBacklog: 2000,
		}
		assert.InDelta(t, 39.0, h.GetHealthScore(), 0.000001)

		h = &InstanceHealth{
			Reachable:    true,
			StatusCode:   500,
			ResponseTime: 100 * time.Second,
			ErrorRate:    10,
			InboxBacklog: 100000,
		}
		assert.Equal(t, 0.0, h.GetHealthScore())
	})
}

func TestInstanceHealthSummary_UpdateKeysAndBuilder(t *testing.T) {
	lastUpdated := time.Unix(1700000000, 0).UTC()

	t.Run("UpdateKeys window variants", func(t *testing.T) {
		cases := []struct {
			window time.Duration
			sk     string
		}{
			{time.Hour, "SUMMARY#1h"},
			{24 * time.Hour, "SUMMARY#24h"},
			{7 * 24 * time.Hour, "SUMMARY#7d"},
			{5 * time.Second, "SUMMARY#5s"},
		}

		for _, tc := range cases {
			s := &InstanceHealthSummary{
				Domain:      "example.com",
				Window:      tc.window,
				LastUpdated: lastUpdated,
			}
			require.NoError(t, s.UpdateKeys())
			assert.Equal(t, "INSTANCE#example.com", s.PK)
			assert.Equal(t, tc.sk, s.SK)
			assert.Equal(t, lastUpdated.Add(30*24*time.Hour).Unix(), s.TTL)
			assert.Equal(t, MainTableName, s.TableName())
			assert.Equal(t, s.PK, s.GetPK())
			assert.Equal(t, s.SK, s.GetSK())
		}
	})

	t.Run("NewInstanceHealthSummary initializes maps and keys", func(t *testing.T) {
		s := NewInstanceHealthSummary("example.com", time.Hour)
		require.NotNil(t, s)
		assert.Equal(t, "example.com", s.Domain)
		assert.Equal(t, time.Hour, s.Window)
		assert.NotNil(t, s.StatusCodeCounts)
		assert.Contains(t, s.SK, "SUMMARY#")
	})
}

