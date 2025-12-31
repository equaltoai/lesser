package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrends_UpdateKeysAndTTL(t *testing.T) {
	ts := time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

	t.Run("HashtagTrend UpdateKeys sets keys and TTL", func(t *testing.T) {
		h := &HashtagTrend{
			Name:      "golang",
			TrendScore: 1.234,
			UpdatedAt:  ts,
		}
		require.NoError(t, h.UpdateKeys())
		assert.Contains(t, h.PK, "TREND_TYPE#HASHTAG#2024-06-15")
		assert.Contains(t, h.SK, "SCORE#")
		assert.Equal(t, h.PK, h.GSI8PK)
		assert.Equal(t, h.SK, h.GSI8SK)
		assert.Equal(t, ts.Add(7*24*time.Hour).Unix(), h.TTL)
		assert.Equal(t, MainTableName, h.TableName())
		assert.Equal(t, h.PK, h.GetPK())
		assert.Equal(t, h.SK, h.GetSK())
	})

	t.Run("StatusTrend UpdateKeys sets keys and preserves pre-set TTL", func(t *testing.T) {
		s := &StatusTrend{
			ID:        "st1",
			TrendScore: 2.0,
			UpdatedAt:  ts,
			TTL:        123,
		}
		require.NoError(t, s.UpdateKeys())
		assert.Contains(t, s.PK, "TREND_TYPE#STATUS#2024-06-15")
		assert.Contains(t, s.SK, "SCORE#")
		assert.Equal(t, int64(123), s.TTL)
		assert.Equal(t, MainTableName, s.TableName())
	})

	t.Run("LinkTrend UpdateKeys sets keys and TTL", func(t *testing.T) {
		l := &LinkTrend{
			URL:       "https://example.com",
			TrendScore: 0.5,
			UpdatedAt:  ts,
		}
		require.NoError(t, l.UpdateKeys())
		assert.Contains(t, l.PK, "TREND_TYPE#LINK#2024-06-15")
		assert.Contains(t, l.SK, "SCORE#")
		assert.Equal(t, ts.Add(7*24*time.Hour).Unix(), l.TTL)
		assert.Equal(t, MainTableName, l.TableName())
	})

	t.Run("SearchQuery UpdateKeys tracks user query history", func(t *testing.T) {
		q := &SearchQuery{
			UserID:     "alice",
			Query:      "cats",
			SearchedAt: ts,
		}
		require.NoError(t, q.UpdateKeys())
		assert.Equal(t, "USER#alice", q.PK)
		assert.Contains(t, q.SK, "SEARCH#")
		assert.Equal(t, ts.Add(30*24*time.Hour).Unix(), q.TTL)
		assert.Equal(t, MainTableName, q.TableName())
		assert.Equal(t, q.PK, q.GetPK())
		assert.Equal(t, q.SK, q.GetSK())
	})

	t.Run("PopularQueryCounter UpdateKeys sets keys, padded counts, and TTL by bucket", func(t *testing.T) {
		p := &PopularQueryCounter{
			QueryHash:  "h1",
			TimeBucket: "daily",
			Date:       "2024-06-15",
			Count:      12,
			UpdatedAt:  ts,
		}
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, "POPULAR_QUERY#h1", p.PK)
		assert.Equal(t, "COUNTER#daily", p.SK)
		assert.Equal(t, "POPULAR#daily#2024-06-15", p.GSI8PK)
		assert.Equal(t, "COUNT#0000000012#h1", p.GSI8SK)
		assert.Equal(t, ts.Add(30*24*time.Hour).Unix(), p.TTL)
		assert.Equal(t, MainTableName, p.TableName())
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())

		p = &PopularQueryCounter{
			QueryHash:  "h2",
			TimeBucket: "other",
			Date:       "2024-06-15",
			Count:      1,
			UpdatedAt:  ts,
			TTL:        999,
		}
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, int64(999), p.TTL)
	})
}

