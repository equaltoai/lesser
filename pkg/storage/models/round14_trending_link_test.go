package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrendingLink_KeysScoringAndDisplayHelpers(t *testing.T) {
	t.Run("UpdateKeys sets PK/SK, extracts domain, and sets TTL from date", func(t *testing.T) {
		tl := &TrendingLink{
			URL:           "https://Example.com/path",
			Date:          "2024-01-01",
			LinkID:        "id-1",
			TrendingScore: 123,
		}
		tl.UpdateKeys()
		assert.Equal(t, "TRENDING#2024-01-01", tl.PK)
		assert.Contains(t, tl.SK, "LINK#")
		assert.Equal(t, "example.com", tl.Domain)
		assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 7).Unix(), tl.TTL)
		assert.Equal(t, MainTableName, tl.TableName())
	})

	t.Run("UpdateKeys leaves domain empty on parse error and TTL empty on invalid date", func(t *testing.T) {
		tl := &TrendingLink{
			URL:    "https://example.com/%zz",
			Date:   "not-a-date",
			LinkID: "id-1",
		}
		tl.UpdateKeys()
		assert.Empty(t, tl.Domain)
		assert.Equal(t, int64(0), tl.TTL)
	})

	t.Run("NewTrendingLink defaults type and computes score", func(t *testing.T) {
		base := &TrendingLink{
			URL:        "https://example.com/hello-world.html",
			ShareCount: 10,
		}
		tl := NewTrendingLink("2024-01-01", base)
		assert.Equal(t, "link", tl.Type)
		assert.NotEmpty(t, tl.LinkID)
		assert.Equal(t, "TRENDING#2024-01-01", tl.PK)
		assert.True(t, tl.TrendingScore > 0)
	})

	t.Run("CalculateTrendingScore applies type multiplier and time decay", func(t *testing.T) {
		now := time.Now()
		link := &TrendingLink{ShareCount: 10, Type: "link", CreatedAt: now.Add(-time.Hour)}
		video := &TrendingLink{ShareCount: 10, Type: "video", CreatedAt: now.Add(-time.Hour)}
		photo := &TrendingLink{ShareCount: 10, Type: "photo", CreatedAt: now.Add(-time.Hour)}

		link.CalculateTrendingScore()
		video.CalculateTrendingScore()
		photo.CalculateTrendingScore()

		assert.True(t, video.TrendingScore > link.TrendingScore)
		assert.True(t, photo.TrendingScore > link.TrendingScore)

		// Future CreatedAt skips time decay branch.
		future := &TrendingLink{ShareCount: 10, Type: "video", CreatedAt: now.Add(time.Hour)}
		future.CalculateTrendingScore()
		assert.InDelta(t, 15.0, future.TrendingScore, 0.0001)
	})

	t.Run("IsStillTrending and FormatTrendingSummary", func(t *testing.T) {
		tl := &TrendingLink{
			Title:         "Hello",
			Domain:        "example.com",
			ShareCount:    5,
			Rank:          2,
			TrendingScore: 100,
			CreatedAt:     time.Now().Add(-time.Minute),
		}
		assert.True(t, tl.IsStillTrending(50, time.Hour))
		assert.False(t, tl.IsStillTrending(200, time.Hour))

		summary := tl.FormatTrendingSummary()
		assert.Contains(t, summary, "Rank #2")
		assert.Contains(t, summary, "Hello")
		assert.Contains(t, summary, "example.com")
	})

	t.Run("GetDisplayTitle prefers title and falls back to URL parsing", func(t *testing.T) {
		tl := &TrendingLink{Title: "Explicit"}
		assert.Equal(t, "Explicit", tl.GetDisplayTitle())

		tl = &TrendingLink{URL: "https://example.com/hello-world.html"}
		assert.Equal(t, "Hello World", tl.GetDisplayTitle())

		tl = &TrendingLink{URL: "https://example.com/"}
		assert.Equal(t, "example.com", tl.GetDisplayTitle())

		tl = &TrendingLink{URL: "not a url \u007f"}
		assert.True(t, strings.Contains(tl.GetDisplayTitle(), "not a url"))
	})

	t.Run("Key helpers produce stable keys", func(t *testing.T) {
		pk, sk := GetTrendingLinkKey("2024-01-01", "id", 100)
		assert.Equal(t, "TRENDING#2024-01-01", pk)
		assert.Contains(t, sk, "LINK#")

		pk, prefix := GetTrendingLinksKeys("2024-01-01")
		assert.Equal(t, "TRENDING#2024-01-01", pk)
		assert.Equal(t, "LINK#", prefix)

		pk2, prefix2 := GetTrendingLinksByDomainKeys("2024-01-01", "example.com")
		assert.Equal(t, pk, pk2)
		assert.Equal(t, prefix, prefix2)
	})
}
