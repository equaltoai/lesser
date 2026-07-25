// Package common provides additional standardized response functions that extend
// the existing error_responses.go with more convenience functions and Mastodon-specific patterns
package common //nolint:revive // shared package name by design

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/errors"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func errorCodeForHTTPStatus(status int) string {
	switch status {
	case 400:
		return string(errors.CodeBadRequest)
	case 401:
		return string(errors.CodeUnauthorized)
	case 403:
		return string(errors.CodeForbidden)
	case 404:
		return string(errors.CodeNotFound)
	case 405:
		return string(errors.CodeMethodNotAllowed)
	case 409:
		return string(errors.CodeConflict)
	case 410:
		return string(errors.CodeGone)
	case 408:
		return string(errors.CodeTimeout)
	case 413:
		return string(errors.CodeContentTooLarge)
	case 422:
		return string(errors.CodeUnprocessableEntity)
	case 429:
		return string(errors.CodeRateLimited)
	case 503:
		return string(errors.CodeExternalServiceUnavailable)
	default:
		if status >= 400 && status < 500 {
			return string(errors.CodeBadRequest)
		}
		return string(errors.CodeInternal)
	}
}

// SendError is a convenience function for sending standardized error responses with Lift
// This consolidates the pattern: return apptheory.JSON(code, map[string]string{"error": message})
func SendError(_ *apptheory.Context, code int, message string) (*apptheory.Response, error) {
	return apptheory.JSON(code, StandardErrorResponse{
		Error: message,
		Code:  errorCodeForHTTPStatus(code),
	})
}

// SendJSON is a convenience function for sending successful JSON responses with Lift
// This consolidates the pattern: return apptheory.JSON(code, data)
func SendJSON(_ *apptheory.Context, code int, data interface{}) (*apptheory.Response, error) {
	return apptheory.JSON(code, data)
}

// SendMastodonError sends Mastodon API-compatible error responses
// Mastodon clients expect a specific error format for proper error handling
func SendMastodonError(_ *apptheory.Context, code int, errorMsg string) (*apptheory.Response, error) {
	mastodonError := map[string]interface{}{
		"error":      errorMsg,
		"error_code": errorCodeForHTTPStatus(code),
	}

	// Add additional fields for specific error codes
	switch code {
	case 422:
		mastodonError["error_description"] = "Validation failed"
	case 429:
		mastodonError["error_description"] = "Rate limit exceeded"
	}

	return apptheory.JSON(code, mastodonError)
}

// Success response helpers for Lift contexts

// SendOK sends a 200 OK response with data
func SendOK(ctx *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return SendJSON(ctx, 200, data)
}

// SendCreated sends a 201 Created response with data
func SendCreated(ctx *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return SendJSON(ctx, 201, data)
}

// SendAccepted sends a 202 Accepted response with data
func SendAccepted(ctx *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return SendJSON(ctx, 202, data)
}

// SendNoContent sends a 204 No Content response
func SendNoContent(ctx *apptheory.Context) (*apptheory.Response, error) {
	_ = ctx
	return &apptheory.Response{Status: 204}, nil
}

// Mastodon-specific response helpers

// MastodonAccount represents a basic Mastodon account structure for responses
type MastodonAccount struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Acct           string `json:"acct"`
	DisplayName    string `json:"display_name"`
	Note           string `json:"note"`
	URL            string `json:"url"`
	Avatar         string `json:"avatar"`
	Header         string `json:"header"`
	Locked         bool   `json:"locked"`
	CreatedAt      string `json:"created_at"`
	FollowersCount int    `json:"followers_count"`
	FollowingCount int    `json:"following_count"`
	StatusesCount  int    `json:"statuses_count"`
	Bot            bool   `json:"bot"`
	Discoverable   bool   `json:"discoverable"`
}

// MastodonStatus represents a basic Mastodon status structure for responses
type MastodonStatus struct {
	ID                 string          `json:"id"`
	CreatedAt          string          `json:"created_at"`
	InReplyToID        *string         `json:"in_reply_to_id"`
	InReplyToAccountID *string         `json:"in_reply_to_account_id"`
	Sensitive          bool            `json:"sensitive"`
	SpoilerText        string          `json:"spoiler_text"`
	Visibility         string          `json:"visibility"`
	Language           *string         `json:"language"`
	URI                string          `json:"uri"`
	URL                *string         `json:"url"`
	RepliesCount       int             `json:"replies_count"`
	ReblogsCount       int             `json:"reblogs_count"`
	FavouritesCount    int             `json:"favourites_count"`
	Content            string          `json:"content"`
	Reblog             *MastodonStatus `json:"reblog"`
	Account            MastodonAccount `json:"account"`
	Reblogged          bool            `json:"reblogged"`
	Favourited         bool            `json:"favourited"`
	Bookmarked         bool            `json:"bookmarked"`
	Pinned             bool            `json:"pinned"`
}

// SendMastodonAccount sends a Mastodon-formatted account response
func SendMastodonAccount(ctx *apptheory.Context, account MastodonAccount) (*apptheory.Response, error) {
	return SendOK(ctx, account)
}

// SendMastodonAccounts sends a Mastodon-formatted accounts array response
func SendMastodonAccounts(ctx *apptheory.Context, accounts []MastodonAccount) (*apptheory.Response, error) {
	return SendOK(ctx, accounts)
}

// SendMastodonStatus sends a Mastodon-formatted status response
func SendMastodonStatus(ctx *apptheory.Context, status MastodonStatus) (*apptheory.Response, error) {
	return SendOK(ctx, status)
}

// SendMastodonStatuses sends a Mastodon-formatted statuses array response
func SendMastodonStatuses(ctx *apptheory.Context, statuses []MastodonStatus) (*apptheory.Response, error) {
	return SendOK(ctx, statuses)
}

// Pagination response helpers

// PaginatedResponse wraps data with pagination metadata
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination contains pagination metadata
type Pagination struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	Page       int    `json:"page,omitempty"`
	TotalCount int    `json:"total_count,omitempty"`
	HasNext    bool   `json:"has_next"`
	HasPrev    bool   `json:"has_prev"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
}

// SendPaginatedResponse sends a response with pagination metadata
func SendPaginatedResponse(ctx *apptheory.Context, data interface{}, pagination *Pagination) (*apptheory.Response, error) {
	response := PaginatedResponse{
		Data:       data,
		Pagination: pagination,
	}
	return SendOK(ctx, response)
}

// SendPaginatedMastodonResponse sends Mastodon-compatible paginated response
// Mastodon uses Link headers for pagination instead of response body metadata
func SendPaginatedMastodonResponse(ctx *apptheory.Context, data interface{}, params PaginationParams, hasNext, hasPrev bool, nextCursor, prevCursor string) (*apptheory.Response, error) {
	resp, err := SendOK(ctx, data)
	if err != nil {
		return nil, err
	}

	// Mastodon uses Link headers for pagination instead of response body metadata.
	if hasNext || hasPrev {
		baseURL := GetBaseURL(ctx)
		linkHeader := BuildLinkHeader(baseURL, params, hasNext, hasPrev, nextCursor, prevCursor)
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}
		resp.Headers["link"] = []string{linkHeader}
	}

	return resp, nil
}

// GetBaseURL constructs the base URL for the current request
func GetBaseURL(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}

	scheme := "https"
	if proto := firstHeaderValue(ctx.Request.Headers, "x-forwarded-proto"); strings.EqualFold(proto, "http") {
		scheme = "http"
	}

	host := firstHeaderValue(ctx.Request.Headers, "host")
	if host == "" {
		host = firstHeaderValue(ctx.Request.Headers, "x-forwarded-host")
	}

	path := ctx.Request.Path

	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

// Enhanced error response helpers that maintain backward compatibility

// RespondWithJSON is an alias for SendJSON for consistency with existing patterns
func RespondWithJSON(ctx *apptheory.Context, code int, data interface{}) (*apptheory.Response, error) {
	return SendJSON(ctx, code, data)
}

// RespondWithError is an alias for SendError for consistency with existing patterns
func RespondWithError(ctx *apptheory.Context, code int, message string) (*apptheory.Response, error) {
	return SendError(ctx, code, message)
}

// Streaming response helpers

// StreamingMessage represents a Server-Sent Events message
type StreamingMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// SendStreamingMessage sends a Server-Sent Events formatted message
func SendStreamingMessage(ctx *apptheory.Context, event string, data interface{}) (*apptheory.Response, error) {
	_ = ctx

	// Format as SSE
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	message := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData))
	return &apptheory.Response{
		Status: 200,
		Headers: map[string][]string{
			"content-type":  {"text/event-stream; charset=utf-8"},
			"cache-control": {"no-cache"},
			"connection":    {"keep-alive"},
		},
		Body: []byte(message),
	}, nil
}

// WebSocket response helpers

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Stream  []string    `json:"stream,omitempty"`
	Event   string      `json:"event,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

// CreateWebSocketMessage creates a properly formatted WebSocket message
func CreateWebSocketMessage(msgType string, stream []string, event string, payload interface{}) WebSocketMessage {
	return WebSocketMessage{
		Type:    msgType,
		Stream:  stream,
		Event:   event,
		Payload: payload,
	}
}

// Response validation helpers

// ValidateResponseData ensures response data is not nil and properly formatted
func ValidateResponseData(data interface{}) interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	return data
}

// Response header helpers

// SetCORSHeaders sets comprehensive CORS headers for API responses.
func SetCORSHeaders(resp *apptheory.Response) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers["access-control-allow-origin"] = []string{"*"}
	resp.Headers["access-control-allow-methods"] = []string{"GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD"}
	resp.Headers["access-control-allow-headers"] = []string{"Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto"}
	resp.Headers["access-control-max-age"] = []string{"86400"}
}

// SetActivityPubHeaders sets headers for ActivityPub responses
func SetActivityPubHeaders(resp *apptheory.Response) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers["content-type"] = []string{"application/activity+json; charset=utf-8"}
	SetCORSHeaders(resp)
}

// SetJSONHeaders sets headers for JSON API responses
func SetJSONHeaders(resp *apptheory.Response) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers["content-type"] = []string{"application/json; charset=utf-8"}
	SetCORSHeaders(resp)
}

// SetSecurityHeaders sets security-related headers
func SetSecurityHeaders(resp *apptheory.Response) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers["x-frame-options"] = []string{"DENY"}
	resp.Headers["x-content-type-options"] = []string{"nosniff"}
	resp.Headers["x-xss-protection"] = []string{"1; mode=block"}
	resp.Headers["referrer-policy"] = []string{"strict-origin-when-cross-origin"}
}

// Cache control helpers

// SetCacheHeaders sets appropriate cache headers based on content type
func SetCacheHeaders(resp *apptheory.Response, maxAge int) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}

	if maxAge > 0 {
		resp.Headers["cache-control"] = []string{fmt.Sprintf("public, max-age=%d", maxAge)}
	} else {
		resp.Headers["cache-control"] = []string{"no-cache, no-store, must-revalidate"}
		resp.Headers["pragma"] = []string{"no-cache"}
		resp.Headers["expires"] = []string{"0"}
	}
}

// SetNoCache sets no-cache headers for sensitive responses
func SetNoCache(resp *apptheory.Response) {
	SetCacheHeaders(resp, 0)
}

// Utility response functions

// SendEmpty sends an empty array response (used in many Mastodon endpoints)
func SendEmpty(ctx *apptheory.Context) (*apptheory.Response, error) {
	return SendOK(ctx, []interface{}{})
}

// SendEmptyObject sends an empty object response
func SendEmptyObject(ctx *apptheory.Context) (*apptheory.Response, error) {
	return SendOK(ctx, map[string]interface{}{})
}

// SendBool sends a boolean response
func SendBool(ctx *apptheory.Context, value bool) (*apptheory.Response, error) {
	return SendOK(ctx, map[string]bool{"result": value})
}

// SendCount sends a count response
func SendCount(ctx *apptheory.Context, count int) (*apptheory.Response, error) {
	return SendOK(ctx, map[string]int{"count": count})
}

// SendID sends an ID response
func SendID(ctx *apptheory.Context, id string) (*apptheory.Response, error) {
	return SendOK(ctx, map[string]string{"id": id})
}

// Rate limiting response helpers

// SendRateLimitHeaders sets rate limit headers
func SendRateLimitHeaders(resp *apptheory.Response, limit, remaining int, resetTime int64) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers["x-ratelimit-limit"] = []string{strconv.Itoa(limit)}
	resp.Headers["x-ratelimit-remaining"] = []string{strconv.Itoa(remaining)}
	resp.Headers["x-ratelimit-reset"] = []string{strconv.FormatInt(resetTime, 10)}
}

// Health check response helpers

// HealthCheckResponse represents a health check response
type HealthCheckResponse struct {
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Version   string                 `json:"version,omitempty"`
	Checks    map[string]interface{} `json:"checks,omitempty"`
}

// SendHealthCheck sends a health check response
func SendHealthCheck(ctx *apptheory.Context, health HealthCheckResponse) (*apptheory.Response, error) {
	if health.Status == "ok" {
		return SendOK(ctx, health)
	}
	return SendError(ctx, 503, "Service Unavailable")
}

func firstHeaderValue(headers map[string][]string, key string) string {
	if headers == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}

	// Most AppTheory requests already canonicalize to lowercase keys.
	keyLower := strings.ToLower(key)
	if values := headers[keyLower]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	if values := headers[key]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
