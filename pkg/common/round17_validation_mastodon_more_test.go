package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationMastodon_MoreDeterministicBranches(t *testing.T) {
	t.Run("ValidateAccountParams covers optional media fields", func(t *testing.T) {
		err := ValidateAccountParams(map[string]interface{}{
			"display_name": "Alice",
			"avatar":       "data",
			"header":       "data",
		})
		require.NoError(t, err)
	})

	t.Run("ValidateMediaIDs type and element errors", func(t *testing.T) {
		assert.Error(t, ValidateMediaIDs("not-an-array"))
		assert.Error(t, ValidateMediaIDs([]interface{}{123}))
	})

	t.Run("ValidatePollParams covers expiration type branches", func(t *testing.T) {
		base := map[string]interface{}{
			"options": []interface{}{"a", "b"},
		}

		tooLongU := map[string]interface{}{}
		for k, v := range base {
			tooLongU[k] = v
		}
		tooLongU["expires_in"] = uint(MaxPollDuration + 1)
		assert.Error(t, ValidatePollParams(tooLongU))

		tooLongU64 := map[string]interface{}{}
		for k, v := range base {
			tooLongU64[k] = v
		}
		tooLongU64["expires_in"] = uint64(MaxPollDuration + 1)
		assert.Error(t, ValidatePollParams(tooLongU64))

		badString := map[string]interface{}{}
		for k, v := range base {
			badString[k] = v
		}
		badString["expires_in"] = "not-a-number"
		assert.Error(t, ValidatePollParams(badString))

		goodString := map[string]interface{}{}
		for k, v := range base {
			goodString[k] = v
		}
		goodString["expires_in"] = "3600"
		assert.NoError(t, ValidatePollParams(goodString))
	})

	t.Run("ValidateFilterExpiration bounds and types", func(t *testing.T) {
		assert.Error(t, ValidateFilterExpiration(float64(-1)))
		assert.Error(t, ValidateFilterExpiration(float64(366*24*60*60)))
		assert.Error(t, ValidateFilterExpiration("1"))
	})

	t.Run("ValidateFilterKeywords keyword/whole_word types", func(t *testing.T) {
		assert.Error(t, ValidateFilterKeywords("not-an-array"))

		err := ValidateFilterKeywords([]interface{}{
			"not-an-object",
		})
		assert.Error(t, err)

		err = ValidateFilterKeywords([]interface{}{
			map[string]interface{}{
				"keyword":     123,
				"whole_word":  "true",
				"irrelevant":  "ok",
				"whole_words": true,
			},
		})
		assert.Error(t, err)
	})

	t.Run("ValidateReportStatusIDs type errors", func(t *testing.T) {
		assert.Error(t, ValidateReportStatusIDs("not-an-array"))
		assert.Error(t, ValidateReportStatusIDs([]interface{}{123}))
	})

	t.Run("ValidateMastodonTimeline boolean and id validation errors", func(t *testing.T) {
		assert.Error(t, ValidateMastodonTimeline(map[string]interface{}{
			"local": "true",
		}))

		assert.Error(t, ValidateMastodonTimeline(map[string]interface{}{
			"max_id": "bad!",
		}))
	})
}
