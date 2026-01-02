package common

import (
	"testing"

	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/stretchr/testify/assert"
)

func TestPaginationParamsExtractionAndHelpers(t *testing.T) {
	t.Run("GetPaginationParams defaults and parsing", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		p := GetPaginationParams(ctx)
		assert.Equal(t, DefaultPaginationLimit, p.Limit)
		assert.Equal(t, 0, p.Offset)
		assert.Equal(t, DefaultPaginationPage, p.Page)

		ctx2 := liftTesting.MockLiftContext("GET", "/test", liftTesting.WithQueryParams(map[string]string{
			"limit":  "200",
			"page":   "2",
			"max_id": "m",
		}))
		p2 := GetPaginationParams(ctx2)
		assert.Equal(t, MaxPaginationLimit, p2.Limit)
		assert.Equal(t, 100, p2.Offset) // page 2, limit clamped to 100
		assert.Equal(t, 2, p2.Page)
		assert.Equal(t, "m", p2.MaxID)

		// Explicit offset overrides page-derived offset.
		ctx3 := liftTesting.MockLiftContext("GET", "/test", liftTesting.WithQueryParams(map[string]string{
			"limit":  "10",
			"page":   "3",
			"offset": "7",
		}))
		p3 := GetPaginationParams(ctx3)
		assert.Equal(t, 10, p3.Limit)
		assert.Equal(t, 7, p3.Offset)
		assert.Equal(t, 3, p3.Page)
	})

	t.Run("GetPaginationParamsFromRequest mirrors Lift version", func(t *testing.T) {
		p := GetPaginationParamsFromRequest(map[string][]string{
			"limit":  {"5"},
			"page":   {"2"},
			"max_id": {"m"},
		})
		assert.Equal(t, 5, p.Limit)
		assert.Equal(t, 5, p.Offset) // (2-1)*5
		assert.Equal(t, 2, p.Page)
		assert.Equal(t, "m", p.MaxID)
	})

	t.Run("BuildLinkHeader and SetPaginationHeaders", func(t *testing.T) {
		baseURL := "https://example.com/api/v1/timeline"
		params := PaginationParams{Limit: 40}
		link := BuildLinkHeader(baseURL, params, true, true, "next", "prev")
		assert.Contains(t, link, `rel="next"`)
		assert.Contains(t, link, `rel="prev"`)
		assert.Contains(t, link, "limit=40")

		ctx := liftTesting.MockLiftContext("GET", "/test")
		SetPaginationHeaders(ctx, baseURL, params, true, false, "next", "")
		assert.NotEmpty(t, ctx.Response.Headers["Link"])
		assert.Equal(t, "40", ctx.Response.Headers["X-Pagination-Limit"])
	})

	t.Run("GetTimelinePaginationParams parses booleans", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test", liftTesting.WithQueryParams(map[string]string{
			"local":      "true",
			"only_media": "1",
			"remote":     "true",
			"replies":    "false",
		}))
		p := GetTimelinePaginationParams(ctx)
		assert.True(t, p.Local)
		assert.True(t, p.OnlyMedia)
		assert.True(t, p.RemoteOnly)
		assert.False(t, p.IncludeReplies)
	})

	t.Run("GetSearchPaginationParams parses booleans and max_results", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test", liftTesting.WithQueryParams(map[string]string{
			"type":               "account",
			"resolve":            "1",
			"following":          "true",
			"exclude_unreviewed": "1",
			"max_results":        "200",
		}))
		p := GetSearchPaginationParams(ctx)
		assert.Equal(t, "account", p.Type)
		assert.True(t, p.Resolve)
		assert.True(t, p.Following)
		assert.True(t, p.ExcludeUnreviewed)
		assert.Equal(t, MaxPaginationLimit, p.MaxResults)
	})

	t.Run("GetAdminPaginationParams parses invited", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test", liftTesting.WithQueryParams(map[string]string{
			"origin":   "local",
			"status":   "active",
			"invited":  "true",
			"username": "alice",
		}))
		p := GetAdminPaginationParams(ctx)
		assert.Equal(t, "local", p.Origin)
		assert.Equal(t, "active", p.Status)
		assert.True(t, p.InviteFilter)
		assert.Equal(t, "alice", p.Username)
	})
}
