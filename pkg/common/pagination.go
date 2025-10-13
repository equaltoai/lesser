// Package common provides centralized pagination functionality for consistent
// pagination handling across all Lesser APIs and services.
package common

import (
	"strconv"
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
)

// PaginationParams represents standardized pagination parameters used across the application
type PaginationParams struct {
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	Page    int    `json:"page"`
	MaxID   string `json:"max_id"`
	MinID   string `json:"min_id"`
	SinceID string `json:"since_id"`
	Cursor  string `json:"cursor"`
}

// PaginationDefaults holds the default values for pagination
const (
	DefaultPaginationLimit = 20
	MaxPaginationLimit     = 100
	MinPaginationLimit     = 1
	DefaultPaginationPage  = 1
)

// GetPaginationParams extracts and validates pagination parameters from a Lift context
// This consolidates the pagination logic found across 50+ handlers in the codebase
func GetPaginationParams(ctx *lift.Context) PaginationParams {
	params := PaginationParams{
		Limit:   DefaultPaginationLimit,
		Offset:  0,
		Page:    DefaultPaginationPage,
		MaxID:   ctx.Query("max_id"),
		MinID:   ctx.Query("min_id"),
		SinceID: ctx.Query("since_id"),
		Cursor:  ctx.Query("cursor"),
	}

	// Parse limit with bounds checking
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = ValidatePaginationLimit(limit)
		}
	}

	// Parse offset
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			params.Offset = offset
		}
	}

	// Parse page and convert to offset if provided
	if pageStr := ctx.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page >= 1 {
			params.Page = page
			// If no explicit offset was provided, calculate from page
			if ctx.Query("offset") == "" {
				params.Offset = (page - 1) * params.Limit
			}
		}
	}

	return params
}

// GetPaginationParamsFromRequest extracts pagination from standard HTTP request
// This provides compatibility with non-Lift request handling
func GetPaginationParamsFromRequest(queryParams map[string][]string) PaginationParams {
	params := PaginationParams{
		Limit:  DefaultPaginationLimit,
		Offset: 0,
		Page:   DefaultPaginationPage,
	}

	// Helper to get first query param value
	getParam := func(key string) string {
		if values, exists := queryParams[key]; exists && len(values) > 0 {
			return values[0]
		}
		return ""
	}

	params.MaxID = getParam("max_id")
	params.MinID = getParam("min_id")
	params.SinceID = getParam("since_id")
	params.Cursor = getParam("cursor")

	// Parse limit with bounds checking
	if limitStr := getParam("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = ValidatePaginationLimit(limit)
		}
	}

	// Parse offset
	if offsetStr := getParam("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			params.Offset = offset
		}
	}

	// Parse page and convert to offset if provided
	if pageStr := getParam("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page >= 1 {
			params.Page = page
			// If no explicit offset was provided, calculate from page
			if getParam("offset") == "" {
				params.Offset = (page - 1) * params.Limit
			}
		}
	}

	return params
}

// ValidatePaginationLimit ensures the limit is within acceptable bounds
func ValidatePaginationLimit(limit int) int {
	if limit < MinPaginationLimit {
		return MinPaginationLimit
	}
	if limit > MaxPaginationLimit {
		return MaxPaginationLimit
	}
	return limit
}

// PaginationResult holds pagination response data
type PaginationResult struct {
	Items      interface{} `json:"items"`
	TotalCount int         `json:"total_count,omitempty"`
	HasNext    bool        `json:"has_next,omitempty"`
	HasPrev    bool        `json:"has_prev,omitempty"`
	NextCursor string      `json:"next_cursor,omitempty"`
	PrevCursor string      `json:"prev_cursor,omitempty"`
}

// PaginationLinks holds Link header data for Mastodon API compatibility
type PaginationLinks struct {
	Next string `json:"next,omitempty"`
	Prev string `json:"prev,omitempty"`
}

// BuildLinkHeader creates a Link header for pagination following RFC 5988
// This consolidates the link header building logic found across multiple handlers
func BuildLinkHeader(baseURL string, params PaginationParams, hasNext, hasPrev bool, nextCursor, prevCursor string) string {
	var links []string

	if hasNext {
		nextURL := buildPaginationURL(baseURL, params, "next", nextCursor)
		links = append(links, `<`+nextURL+`>; rel="next"`)
	}

	if hasPrev {
		prevURL := buildPaginationURL(baseURL, params, "prev", prevCursor)
		links = append(links, `<`+prevURL+`>; rel="prev"`)
	}

	return strings.Join(links, ", ")
}

// buildPaginationURL constructs a paginated URL with proper parameters
func buildPaginationURL(baseURL string, params PaginationParams, direction, cursor string) string {
	// Start with base URL
	url := baseURL
	if !strings.Contains(url, "?") {
		url += "?"
	} else {
		url += "&"
	}

	// Add cursor-based pagination for Mastodon compatibility
	switch direction {
	case "next":
		if cursor != "" {
			url += "max_id=" + cursor
		} else if params.MaxID != "" {
			url += "max_id=" + params.MaxID
		}
	case "prev":
		if cursor != "" {
			url += "min_id=" + cursor
		} else if params.MinID != "" {
			url += "min_id=" + params.MinID
		}
	}

	// Add limit if different from default
	if params.Limit != DefaultPaginationLimit {
		if strings.Contains(url, "max_id=") || strings.Contains(url, "min_id=") {
			url += "&"
		}
		url += "limit=" + strconv.Itoa(params.Limit)
	}

	return url
}

// SetPaginationHeaders sets standard pagination headers on a Lift context response
// This consolidates the header setting patterns found across handlers
func SetPaginationHeaders(ctx *lift.Context, baseURL string, params PaginationParams, hasNext, hasPrev bool, nextCursor, prevCursor string) {
	if hasNext || hasPrev {
		linkHeader := BuildLinkHeader(baseURL, params, hasNext, hasPrev, nextCursor, prevCursor)
		ctx.Response.Header("Link", linkHeader)
	}

	// Add pagination metadata headers
	ctx.Response.Header("X-Pagination-Limit", strconv.Itoa(params.Limit))
	ctx.Response.Header("X-Pagination-Offset", strconv.Itoa(params.Offset))
	if params.Page > 0 {
		ctx.Response.Header("X-Pagination-Page", strconv.Itoa(params.Page))
	}
}

// TimelinePaginationParams extends PaginationParams for timeline-specific needs
type TimelinePaginationParams struct {
	PaginationParams
	Local          bool `json:"local"`
	OnlyMedia      bool `json:"only_media"`
	RemoteOnly     bool `json:"remote_only"`
	IncludeReplies bool `json:"include_replies"`
}

// GetTimelinePaginationParams extracts timeline-specific pagination parameters
func GetTimelinePaginationParams(ctx *lift.Context) TimelinePaginationParams {
	params := TimelinePaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	// Parse boolean parameters with proper defaults
	params.Local = ctx.Query("local") == StringTrue || ctx.Query("local") == "1"
	params.OnlyMedia = ctx.Query("only_media") == StringTrue || ctx.Query("only_media") == "1"
	params.RemoteOnly = ctx.Query("remote") == StringTrue || ctx.Query("remote") == "1"
	params.IncludeReplies = ctx.Query("replies") != "false" // Default true, disable with "false"

	return params
}

// SearchPaginationParams extends PaginationParams for search-specific needs
type SearchPaginationParams struct {
	PaginationParams
	Type              string `json:"type"`               // account, hashtag, status
	Resolve           bool   `json:"resolve"`            // Resolve remote resources
	Following         bool   `json:"following"`          // Only search followed accounts
	AccountID         string `json:"account_id"`         // Limit search to specific account
	MaxResults        int    `json:"max_results"`        // Override limit for search APIs
	ExcludeUnreviewed bool   `json:"exclude_unreviewed"` // Exclude unreviewed content
}

// GetSearchPaginationParams extracts search-specific pagination parameters
func GetSearchPaginationParams(ctx *lift.Context) SearchPaginationParams {
	params := SearchPaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	params.Type = ctx.Query("type")
	params.Resolve = ctx.Query("resolve") == StringTrue || ctx.Query("resolve") == "1"
	params.Following = ctx.Query("following") == StringTrue || ctx.Query("following") == "1"
	params.AccountID = ctx.Query("account_id")
	params.ExcludeUnreviewed = ctx.Query("exclude_unreviewed") == StringTrue || ctx.Query("exclude_unreviewed") == "1"

	// Parse max_results for search APIs
	if maxStr := ctx.Query("max_results"); maxStr != "" {
		if maxResults, err := strconv.Atoi(maxStr); err == nil && maxResults > 0 {
			params.MaxResults = ValidatePaginationLimit(maxResults)
		}
	}

	return params
}

// AdminPaginationParams extends PaginationParams for admin-specific needs
type AdminPaginationParams struct {
	PaginationParams
	Origin       string `json:"origin"`        // local, remote
	Status       string `json:"status"`        // active, pending, disabled, etc.
	Permissions  string `json:"permissions"`   // Filter by permission level
	IP           string `json:"ip"`            // Filter by IP address
	Email        string `json:"email"`         // Filter by email pattern
	Username     string `json:"username"`      // Filter by username pattern
	ByDomain     string `json:"by_domain"`     // Filter by domain
	InviteFilter bool   `json:"invite_filter"` // Show only invited accounts
}

// GetAdminPaginationParams extracts admin-specific pagination parameters
func GetAdminPaginationParams(ctx *lift.Context) AdminPaginationParams {
	params := AdminPaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	params.Origin = ctx.Query("origin")
	params.Status = ctx.Query("status")
	params.Permissions = ctx.Query("permissions")
	params.IP = ctx.Query("ip")
	params.Email = ctx.Query("email")
	params.Username = ctx.Query("username")
	params.ByDomain = ctx.Query("by_domain")
	params.InviteFilter = ctx.Query("invited") == StringTrue || ctx.Query("invited") == "1"

	return params
}

// CalculateHasMore determines if there are more results based on the returned items count
// This helps with pagination logic across different storage backends
func CalculateHasMore(itemCount, requestedLimit int) bool {
	return itemCount >= requestedLimit
}

// CalculateOffset calculates the offset from page number and limit
func CalculateOffset(page, limit int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * limit
}

// CalculatePage calculates the page number from offset and limit
func CalculatePage(offset, limit int) int {
	if limit <= 0 {
		limit = DefaultPaginationLimit
	}
	return (offset / limit) + 1
}
