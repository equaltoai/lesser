// Package common provides centralized pagination functionality for consistent
// pagination handling across all Lesser APIs and services.
package common //nolint:revive // shared package name by design

import (
	"strconv"
	"strings"

	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
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

// GetPaginationParams extracts and validates pagination parameters from an AppTheory context.
func GetPaginationParams(ctx *apptheory.Context) PaginationParams {
	if ctx == nil {
		return PaginationParams{
			Limit:  DefaultPaginationLimit,
			Offset: 0,
			Page:   DefaultPaginationPage,
		}
	}
	return GetPaginationParamsFromRequest(ctx.Request.Query)
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

// SetPaginationHeaders sets standard pagination headers on an AppTheory response.
func SetPaginationHeaders(resp *apptheory.Response, baseURL string, params PaginationParams, hasNext, hasPrev bool, nextCursor, prevCursor string) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}

	if hasNext || hasPrev {
		linkHeader := BuildLinkHeader(baseURL, params, hasNext, hasPrev, nextCursor, prevCursor)
		resp.Headers["link"] = []string{linkHeader}
	}

	// Add pagination metadata headers
	resp.Headers["x-pagination-limit"] = []string{strconv.Itoa(params.Limit)}
	resp.Headers["x-pagination-offset"] = []string{strconv.Itoa(params.Offset)}
	if params.Page > 0 {
		resp.Headers["x-pagination-page"] = []string{strconv.Itoa(params.Page)}
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

// GetTimelinePaginationParams extracts timeline-specific pagination parameters.
func GetTimelinePaginationParams(ctx *apptheory.Context) TimelinePaginationParams {
	params := TimelinePaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	// Parse boolean parameters with proper defaults
	var q map[string][]string
	if ctx != nil {
		q = ctx.Request.Query
	}
	params.Local = firstQueryValue(q, "local") == StringTrue || firstQueryValue(q, "local") == "1"
	params.OnlyMedia = firstQueryValue(q, "only_media") == StringTrue || firstQueryValue(q, "only_media") == "1"
	params.RemoteOnly = firstQueryValue(q, "remote") == StringTrue || firstQueryValue(q, "remote") == "1"
	params.IncludeReplies = firstQueryValue(q, "replies") != "false" // Default true, disable with "false"

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

// GetSearchPaginationParams extracts search-specific pagination parameters.
func GetSearchPaginationParams(ctx *apptheory.Context) SearchPaginationParams {
	params := SearchPaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	var q map[string][]string
	if ctx != nil {
		q = ctx.Request.Query
	}
	params.Type = firstQueryValue(q, "type")
	params.Resolve = firstQueryValue(q, "resolve") == StringTrue || firstQueryValue(q, "resolve") == "1"
	params.Following = firstQueryValue(q, "following") == StringTrue || firstQueryValue(q, "following") == "1"
	params.AccountID = firstQueryValue(q, "account_id")
	params.ExcludeUnreviewed = firstQueryValue(q, "exclude_unreviewed") == StringTrue || firstQueryValue(q, "exclude_unreviewed") == "1"

	// Parse max_results for search APIs
	if maxStr := firstQueryValue(q, "max_results"); maxStr != "" {
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

// GetAdminPaginationParams extracts admin-specific pagination parameters.
func GetAdminPaginationParams(ctx *apptheory.Context) AdminPaginationParams {
	params := AdminPaginationParams{
		PaginationParams: GetPaginationParams(ctx),
	}

	var q map[string][]string
	if ctx != nil {
		q = ctx.Request.Query
	}
	params.Origin = firstQueryValue(q, "origin")
	params.Status = firstQueryValue(q, "status")
	params.Permissions = firstQueryValue(q, "permissions")
	params.IP = firstQueryValue(q, "ip")
	params.Email = firstQueryValue(q, "email")
	params.Username = firstQueryValue(q, "username")
	params.ByDomain = firstQueryValue(q, "by_domain")
	params.InviteFilter = firstQueryValue(q, "invited") == StringTrue || firstQueryValue(q, "invited") == "1"

	return params
}

func firstQueryValue(query map[string][]string, key string) string {
	if query == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	values := query[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
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
