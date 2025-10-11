// Package common provides additional standardized response functions that extend
// the existing error_responses.go with more convenience functions and Mastodon-specific patterns
package common

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pay-theory/lift/pkg/lift"
)

// SendError is a convenience function for sending standardized error responses with Lift
// This consolidates the pattern: return ctx.Status(code).JSON(map[string]string{"error": message})
func SendError(ctx *lift.Context, code int, message string) error {
	return ctx.Status(code).JSON(StandardErrorResponse{Error: message})
}

// SendJSON is a convenience function for sending successful JSON responses with Lift
// This consolidates the pattern: return ctx.Status(code).JSON(data)
func SendJSON(ctx *lift.Context, code int, data interface{}) error {
	return ctx.Status(code).JSON(data)
}

// SendMastodonError sends Mastodon API-compatible error responses
// Mastodon clients expect a specific error format for proper error handling
func SendMastodonError(ctx *lift.Context, code int, error string) error {
	mastodonError := map[string]interface{}{
		"error": error,
	}

	// Add additional fields for specific error codes
	switch code {
	case 422:
		mastodonError["error_description"] = "Validation failed"
	case 429:
		mastodonError["error_description"] = "Rate limit exceeded"
	}

	return ctx.Status(code).JSON(mastodonError)
}

// Success response helpers for Lift contexts

// SendOK sends a 200 OK response with data
func SendOK(ctx *lift.Context, data interface{}) error {
	return SendJSON(ctx, 200, data)
}

// SendCreated sends a 201 Created response with data
func SendCreated(ctx *lift.Context, data interface{}) error {
	return SendJSON(ctx, 201, data)
}

// SendAccepted sends a 202 Accepted response with data
func SendAccepted(ctx *lift.Context, data interface{}) error {
	return SendJSON(ctx, 202, data)
}

// SendNoContent sends a 204 No Content response
func SendNoContent(ctx *lift.Context) error {
	return ctx.Status(204).JSON(nil)
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
func SendMastodonAccount(ctx *lift.Context, account MastodonAccount) error {
	return SendOK(ctx, account)
}

// SendMastodonAccounts sends a Mastodon-formatted accounts array response
func SendMastodonAccounts(ctx *lift.Context, accounts []MastodonAccount) error {
	return SendOK(ctx, accounts)
}

// SendMastodonStatus sends a Mastodon-formatted status response
func SendMastodonStatus(ctx *lift.Context, status MastodonStatus) error {
	return SendOK(ctx, status)
}

// SendMastodonStatuses sends a Mastodon-formatted statuses array response
func SendMastodonStatuses(ctx *lift.Context, statuses []MastodonStatus) error {
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
func SendPaginatedResponse(ctx *lift.Context, data interface{}, pagination *Pagination) error {
	response := PaginatedResponse{
		Data:       data,
		Pagination: pagination,
	}
	return SendOK(ctx, response)
}

// SendPaginatedMastodonResponse sends Mastodon-compatible paginated response
// Mastodon uses Link headers for pagination instead of response body metadata
func SendPaginatedMastodonResponse(ctx *lift.Context, data interface{}, params PaginationParams, hasNext, hasPrev bool, nextCursor, prevCursor string) error {
	// Set pagination headers
	if hasNext || hasPrev {
		baseURL := GetBaseURL(ctx)
		linkHeader := BuildLinkHeader(baseURL, params, hasNext, hasPrev, nextCursor, prevCursor)
		ctx.Response.Header("Link", linkHeader)
	}

	// Send data without pagination metadata in body (Mastodon style)
	return SendOK(ctx, data)
}

// GetBaseURL constructs the base URL for the current request
func GetBaseURL(ctx *lift.Context) string {
	scheme := "https"
	if ctx.Request != nil && ctx.Request.Request != nil {
		if ctx.Request.Request.Headers["X-Forwarded-Proto"] == "http" {
			scheme = "http"
		}
	}

	host := ctx.Request.Headers["Host"]
	if host == "" && ctx.Request.Request != nil {
		host = ctx.Request.Request.Headers["Host"]
	}

	path := ctx.Request.Path

	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

// Enhanced error response helpers that maintain backward compatibility

// RespondWithJSON is an alias for SendJSON for consistency with existing patterns
func RespondWithJSON(ctx *lift.Context, code int, data interface{}) error {
	return SendJSON(ctx, code, data)
}

// RespondWithError is an alias for SendError for consistency with existing patterns
func RespondWithError(ctx *lift.Context, code int, message string) error {
	return SendError(ctx, code, message)
}

// Streaming response helpers

// StreamingMessage represents a Server-Sent Events message
type StreamingMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// SendStreamingMessage sends a Server-Sent Events formatted message
func SendStreamingMessage(ctx *lift.Context, event string, data interface{}) error {
	ctx.Response.Header("Content-Type", "text/plain")
	ctx.Response.Header("Cache-Control", "no-cache")
	ctx.Response.Header("Connection", "keep-alive")

	// Format as SSE
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	message := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(jsonData))
	return ctx.Status(200).JSON(map[string]string{"sse": message})
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

// SetCORSHeaders sets comprehensive CORS headers for API responses
func SetCORSHeaders(ctx *lift.Context) {
	ctx.Response.Header("Access-Control-Allow-Origin", "*")
	ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD")
	ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto")
	ctx.Response.Header("Access-Control-Max-Age", "86400")
}

// SetActivityPubHeaders sets headers for ActivityPub responses
func SetActivityPubHeaders(ctx *lift.Context) {
	ctx.Response.Header("Content-Type", "application/activity+json; charset=utf-8")
	SetCORSHeaders(ctx)
}

// SetJSONHeaders sets headers for JSON API responses
func SetJSONHeaders(ctx *lift.Context) {
	ctx.Response.Header("Content-Type", "application/json; charset=utf-8")
	SetCORSHeaders(ctx)
}

// SetSecurityHeaders sets security-related headers
func SetSecurityHeaders(ctx *lift.Context) {
	ctx.Response.Header("X-Frame-Options", "DENY")
	ctx.Response.Header("X-Content-Type-Options", "nosniff")
	ctx.Response.Header("X-XSS-Protection", "1; mode=block")
	ctx.Response.Header("Referrer-Policy", "strict-origin-when-cross-origin")
}

// Cache control helpers

// SetCacheHeaders sets appropriate cache headers based on content type
func SetCacheHeaders(ctx *lift.Context, maxAge int) {
	if maxAge > 0 {
		ctx.Response.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
	} else {
		ctx.Response.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		ctx.Response.Header("Pragma", "no-cache")
		ctx.Response.Header("Expires", "0")
	}
}

// SetNoCache sets no-cache headers for sensitive responses
func SetNoCache(ctx *lift.Context) {
	SetCacheHeaders(ctx, 0)
}

// Utility response functions

// SendEmpty sends an empty array response (used in many Mastodon endpoints)
func SendEmpty(ctx *lift.Context) error {
	return SendOK(ctx, []interface{}{})
}

// SendEmptyObject sends an empty object response
func SendEmptyObject(ctx *lift.Context) error {
	return SendOK(ctx, map[string]interface{}{})
}

// SendBool sends a boolean response
func SendBool(ctx *lift.Context, value bool) error {
	return SendOK(ctx, map[string]bool{"result": value})
}

// SendCount sends a count response
func SendCount(ctx *lift.Context, count int) error {
	return SendOK(ctx, map[string]int{"count": count})
}

// SendID sends an ID response
func SendID(ctx *lift.Context, id string) error {
	return SendOK(ctx, map[string]string{"id": id})
}

// Rate limiting response helpers

// SendRateLimitHeaders sets rate limit headers
func SendRateLimitHeaders(ctx *lift.Context, limit, remaining int, resetTime int64) {
	ctx.Response.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	ctx.Response.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	ctx.Response.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
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
func SendHealthCheck(ctx *lift.Context, health HealthCheckResponse) error {
	if health.Status == "ok" {
		return SendOK(ctx, health)
	}
	return SendError(ctx, 503, "Service Unavailable")
}
