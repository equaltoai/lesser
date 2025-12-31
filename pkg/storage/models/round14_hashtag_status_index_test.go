package models

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashtagIndexes_UpdateKeysAndHashes(t *testing.T) {
	t.Run("HashtagStatusIndex UpdateKeys normalizes hashtag and builds descending timestamp keys", func(t *testing.T) {
		published := time.Unix(1700000000, 0).UTC()
		hsi := &HashtagStatusIndex{
			StatusID:    "s1",
			HashtagName: "#GoLang",
			Visibility:  "public",
			Published:   published,
		}

		require.NoError(t, hsi.UpdateKeys())

		tagLower := "golang"
		timestampDesc := fmt.Sprintf("%019d", math.MaxInt64-published.Unix())

		assert.Equal(t, "HASHTAG_TIMELINE#"+tagLower, hsi.PK)
		assert.Equal(t, "STATUS#"+timestampDesc+"#s1", hsi.SK)
		assert.Equal(t, "STATUS_HASHTAGS#s1", hsi.GSI1PK)
		assert.Equal(t, "HASHTAG#"+tagLower, hsi.GSI1SK)
		assert.Equal(t, "HASHTAG_VIS#"+tagLower+"#public", hsi.GSI2PK)
		assert.Equal(t, "TIMELINE#"+timestampDesc, hsi.GSI2SK)

		assert.Equal(t, MainTableName, hsi.TableName())
		assert.Equal(t, hsi.PK, hsi.GetPK())
		assert.Equal(t, hsi.SK, hsi.GetSK())
	})

	t.Run("HashtagTrendingData UpdateKeys pads score and builds GSIs", func(t *testing.T) {
		period := time.Unix(1700000000, 0).UTC()
		htd := &HashtagTrendingData{
			HashtagName: "#GoLang",
			TrendScore:  12.345,
			Period:      period,
			TimeWindow:  "1h",
		}
		htd.UpdateKeys()

		assert.Equal(t, "TRENDING_HASHTAG#"+period.Format("2006-01-02-15"), htd.PK)
		assert.Contains(t, htd.SK, "HASHTAG#")
		assert.Equal(t, "HASHTAG_TREND#golang", htd.GSI1PK)
		assert.Contains(t, htd.GSI1SK, "TIME#")
		assert.Equal(t, "TRENDING_PERIOD#1h", htd.GSI2PK)
		assert.Contains(t, htd.GSI2SK, "SCORE#")
		assert.Equal(t, MainTableName, htd.TableName())
	})

	t.Run("HashtagSearchCache UpdateKeys hashes query and parameters", func(t *testing.T) {
		hsc := &HashtagSearchCache{
			Query:      "GoLang",
			Parameters: map[string]interface{}{"limit": 10},
			CreatedAt:  time.Unix(1700000000, 0).UTC(),
		}
		hsc.UpdateKeys()

		assert.True(t, strings.HasPrefix(hsc.PK, "HASHTAG_SEARCH_CACHE#"))
		assert.True(t, strings.HasPrefix(hsc.SK, "CACHE#"))
		assert.Equal(t, "SEARCH_CACHE", hsc.GSI1PK)
		assert.Contains(t, hsc.GSI1SK, "CREATED#")
		assert.Equal(t, MainTableName, hsc.TableName())

		// Hash helpers are deterministic for stable inputs.
		assert.Len(t, hsc.hashQuery("x"), 16)
		assert.Len(t, hsc.hashParameters(map[string]interface{}{"k": "v"}), 16)
	})
}
