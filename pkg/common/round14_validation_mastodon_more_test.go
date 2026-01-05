package common

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationMastodon_MoreCoverage(t *testing.T) {
	t.Run("ValidateAccountParams handles booleans and fields", func(t *testing.T) {
		err := ValidateAccountParams(map[string]interface{}{
			"display_name": "Alice",
			"note":         "Hello",
			"locked":       true,
			"fields_attributes": []interface{}{
				map[string]interface{}{"name": "n", "value": "v"},
			},
		})
		require.NoError(t, err)

		err = ValidateAccountParams(map[string]interface{}{
			"locked": "true",
		})
		require.Error(t, err)
	})

	t.Run("ValidateFilterParams required fields and types", func(t *testing.T) {
		err := ValidateFilterParams(map[string]interface{}{})
		require.Error(t, err)

		err = ValidateFilterParams(map[string]interface{}{
			"title":   "t",
			"context": []interface{}{"home"},
		})
		require.NoError(t, err)

		err = ValidateFilterParams(map[string]interface{}{
			"title":   "t",
			"context": []interface{}{"nope"},
		})
		require.Error(t, err)
	})

	t.Run("ValidateMediaParams required file and optional fields", func(t *testing.T) {
		err := ValidateMediaParams(map[string]interface{}{})
		require.Error(t, err)

		err = ValidateMediaParams(map[string]interface{}{
			"file":        "data",
			"description": strings.Repeat("x", MaxMediaDescLength+1),
		})
		require.Error(t, err)

		err = ValidateMediaParams(map[string]interface{}{
			"file":  "data",
			"focus": "2,0",
		})
		require.Error(t, err)

		err = ValidateMediaParams(map[string]interface{}{
			"file":  "data",
			"focus": "0.5,0.5",
		})
		require.NoError(t, err)
	})

	t.Run("ValidateReportParams required fields and booleans", func(t *testing.T) {
		err := ValidateReportParams(map[string]interface{}{})
		require.Error(t, err)

		err = ValidateReportParams(map[string]interface{}{
			"account_id": "acct",
			"forward":    "true",
		})
		require.Error(t, err)

		err = ValidateReportParams(map[string]interface{}{
			"account_id": "acct",
			"status_ids": []interface{}{"status123"},
			"category":   "spam",
			"forward":    true,
		})
		require.NoError(t, err)
	})

	t.Run("ValidateListParams required title and replies policy", func(t *testing.T) {
		err := ValidateListParams(map[string]interface{}{})
		require.Error(t, err)

		err = ValidateListParams(map[string]interface{}{
			"title":          "t",
			"replies_policy": "nope",
		})
		require.Error(t, err)
	})

	t.Run("ValidateApplicationParams required fields and website", func(t *testing.T) {
		err := ValidateApplicationParams(map[string]interface{}{})
		require.Error(t, err)

		err = ValidateApplicationParams(map[string]interface{}{
			"client_name":   "app",
			"redirect_uris": "https://example.com/callback",
			"scopes":        "read write:statuses",
			"website":       "not-a-url",
		})
		require.Error(t, err)
	})

	t.Run("ValidateScheduledTime branches", func(t *testing.T) {
		assert.NoError(t, ValidateScheduledTime(""))
		assert.Error(t, ValidateScheduledTime("not-a-time"))
		assert.Error(t, ValidateScheduledTime(time.Now().Add(-time.Minute).Format(time.RFC3339)))
		assert.Error(t, ValidateScheduledTime(time.Now().Add(366*24*time.Hour).Format(time.RFC3339)))
		assert.NoError(t, ValidateScheduledTime(time.Now().Add(time.Minute).Format(time.RFC3339)))
	})

	t.Run("ValidateMastodonMimeType and hashtag", func(t *testing.T) {
		assert.Error(t, ValidateMastodonMimeType(""))
		assert.Error(t, ValidateMastodonMimeType("not-a-mime"))
		assert.Error(t, ValidateMastodonMimeType("application/octet-stream"))
		assert.NoError(t, ValidateMastodonMimeType("image/png"))

		assert.Error(t, ValidateHashtag(""))
		assert.Error(t, ValidateHashtag("#bad!"))
		assert.NoError(t, ValidateHashtag("#ok_1"))
	})

	t.Run("ValidateMastodonTimeline limit and ids", func(t *testing.T) {
		err := ValidateMastodonTimeline(map[string]interface{}{
			"limit": "bad",
		})
		require.Error(t, err)

		err = ValidateMastodonTimeline(map[string]interface{}{
			"limit":  float64(10),
			"max_id": "status123",
			"local":  true,
		})
		require.NoError(t, err)
	})
}

