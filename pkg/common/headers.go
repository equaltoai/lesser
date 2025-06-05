package common

import (
	"fmt"
	"net/url"
)

// GetCORSHeaders returns standard CORS headers for API responses
func GetCORSHeaders() map[string]string {
	return map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
	}
}

// GetAPIHeaders returns standard headers for API responses including CORS
func GetAPIHeaders() map[string]string {
	headers := GetCORSHeaders()
	headers["Content-Type"] = "application/json"
	return headers
}

// AddLinkHeader adds a Link header for pagination
func AddLinkHeader(headers map[string]string, baseURL string, endpoint string, cursor string, params map[string]string) {
	if cursor == "" {
		return
	}

	// Build the next URL
	nextURL := fmt.Sprintf("%s%s", baseURL, endpoint)

	// Add query parameters
	queryParams := url.Values{}
	queryParams.Set("max_id", cursor)

	for key, value := range params {
		if value != "" && value != "0" && value != "false" {
			queryParams.Set(key, value)
		}
	}

	if len(queryParams) > 0 {
		nextURL += "?" + queryParams.Encode()
	}

	headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
}

// AddPaginationHeaders adds both Link and custom pagination headers
func AddPaginationHeaders(headers map[string]string, baseURL string, endpoint string, cursor string, params map[string]string, totalCount int) {
	// Add Link header for standard Mastodon pagination
	AddLinkHeader(headers, baseURL, endpoint, cursor, params)

	// Optionally add custom headers some clients might use
	if totalCount > 0 {
		headers["X-Total-Count"] = fmt.Sprintf("%d", totalCount)
	}
}
