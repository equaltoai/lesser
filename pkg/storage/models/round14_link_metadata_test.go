package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLinkMetadata_KeysAndSanitization(t *testing.T) {
	t.Run("UpdateKeys sets keys, domain, and TTL", func(t *testing.T) {
		fetched := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		lm := &LinkMetadata{
			URL:       "https://Example.com/path",
			FetchedAt: fetched,
		}
		lm.UpdateKeys()
		assert.Equal(t, "LINK#https://Example.com/path", lm.PK)
		assert.Equal(t, SKMetadata, lm.SK)
		assert.Equal(t, "example.com", lm.Domain)
		assert.Equal(t, fetched.AddDate(0, 0, 30).Unix(), lm.TTL)
		assert.Equal(t, MainTableName, lm.TableName())
	})

	t.Run("NewLinkMetadata initializes access fields and keys", func(t *testing.T) {
		lm := NewLinkMetadata("https://example.com")
		assert.Equal(t, "https://example.com", lm.URL)
		assert.Equal(t, int64(1), lm.AccessCount)
		assert.NotZero(t, lm.FetchedAt)
		assert.NotZero(t, lm.LastAccessed)
		assert.Equal(t, lm.PK, "LINK#https://example.com")
		assert.Equal(t, SKMetadata, lm.SK)
	})

	t.Run("GetLinkMetadataKey", func(t *testing.T) {
		pk, sk := GetLinkMetadataKey("https://example.com")
		assert.Equal(t, "LINK#https://example.com", pk)
		assert.Equal(t, SKMetadata, sk)
	})

	t.Run("RecordAccess increments count", func(t *testing.T) {
		lm := &LinkMetadata{AccessCount: 1, LastAccessed: time.Unix(0, 0).UTC()}
		lm.RecordAccess()
		assert.Equal(t, int64(2), lm.AccessCount)
		assert.True(t, lm.LastAccessed.After(time.Unix(0, 0).UTC()))
	})

	t.Run("IsStale and HasImage", func(t *testing.T) {
		lm := &LinkMetadata{FetchedAt: time.Now().Add(-2 * time.Hour)}
		assert.True(t, lm.IsStale(time.Hour))
		assert.False(t, lm.HasImage())

		lm.Image = "img"
		assert.True(t, lm.HasImage())
	})

	t.Run("GetDisplayDomain strips www and condenses multi-part domains", func(t *testing.T) {
		lm := &LinkMetadata{Domain: "www.sub.example.com"}
		assert.Equal(t, "example", lm.GetDisplayDomain())

		lm.Domain = "www.example.com"
		assert.Equal(t, "example.com", lm.GetDisplayDomain())
	})

	t.Run("OpenGraph and TwitterCard setters", func(t *testing.T) {
		lm := &LinkMetadata{Title: "OG", Description: "OGD", Image: "OGI"}
		lm.SetFromOpenGraph("New", "", "")
		assert.Equal(t, "New", lm.Title)

		lm = &LinkMetadata{}
		lm.SetFromTwitterCard("T", "D", "I")
		assert.Equal(t, "T", lm.Title)
		assert.Equal(t, "D", lm.Description)
		assert.Equal(t, "I", lm.Image)

		lm.SetFromOpenGraph("OG", "OGD", "OGI")
		lm.SetFromTwitterCard("T2", "D2", "I2")
		assert.Equal(t, "OG", lm.Title)
		assert.Equal(t, "OGD", lm.Description)
		assert.Equal(t, "OGI", lm.Image)
	})

	t.Run("TruncateDescription truncates at word boundary when possible", func(t *testing.T) {
		lm := &LinkMetadata{Description: "hello world here"}
		lm.TruncateDescription(8)
		assert.Equal(t, "hello...", lm.Description)

		lm = &LinkMetadata{Description: "longword"}
		lm.TruncateDescription(4)
		assert.Equal(t, "long...", lm.Description)
	})

	t.Run("SanitizeMetadata trims and de-dupes keywords", func(t *testing.T) {
		lm := &LinkMetadata{
			Title:       "  t ",
			Description: "  d ",
			Author:      "  a ",
			Keywords:    []string{"Go", "go ", " ", "Test"},
		}
		lm.SanitizeMetadata()
		assert.Equal(t, "t", lm.Title)
		assert.Equal(t, "d", lm.Description)
		assert.Equal(t, "a", lm.Author)
		assert.ElementsMatch(t, []string{"go", "test"}, lm.Keywords)
	})
}
