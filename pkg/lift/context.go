package lift

import (
	"strconv"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
)

// Context utilities and helpers for Lift applications

// Pagination represents pagination parameters
type Pagination struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Total  int64  `json:"total,omitempty"`
	Next   string `json:"next,omitempty"`
	Prev   string `json:"prev,omitempty"`
}

// PaginationResponse wraps data with pagination information
type PaginationResponse struct {
	Data       any        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// GetPaginationParams extracts pagination parameters from query string
// Uses ctx.GetRequestID() for request tracking as specified in requirements
func GetPaginationParams(ctx *lift.Context) Pagination {
	// Default values
	limit := 20
	offset := 0

	// Extract limit
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	// Extract offset
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Extract page-based pagination (convert to offset)
	if pageStr := ctx.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			offset = (page - 1) * limit
		}
	}

	return Pagination{
		Limit:  limit,
		Offset: offset,
	}
}

// RespondWithPagination sends a paginated response
func RespondWithPagination(ctx *lift.Context, data any, pagination Pagination) error {
	response := PaginationResponse{
		Data:       data,
		Pagination: pagination,
	}
	return ctx.JSON(response)
}

// Response helper functions for consistent API responses

// SuccessResponse represents a successful API response
type SuccessResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse represents an error API response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
}

// RespondWithSuccess sends a successful response
func RespondWithSuccess(ctx *lift.Context, data any, message ...string) error {
	response := SuccessResponse{
		Success: true,
		Data:    data,
	}

	if len(message) > 0 {
		response.Message = message[0]
	}

	return ctx.JSON(response)
}

// RespondWithError sends an error response
func RespondWithError(ctx *lift.Context, statusCode int, message string, code ...string) error {
	response := ErrorResponse{
		Success: false,
		Error:   message,
	}

	if len(code) > 0 {
		response.Code = code[0]
	}

	return ctx.Status(statusCode).JSON(response)
}

// RespondWithData sends data directly (for Mastodon API compatibility)
func RespondWithData(ctx *lift.Context, data any) error {
	return ctx.JSON(data)
}

// Type-safe context access functions using existing auth.Claims struct

// GetRequestID retrieves the request ID using ctx.GetRequestID() as specified
func GetRequestID(ctx *lift.Context) string {
	return ctx.GetRequestID()
}

// GetUserAgent retrieves the User-Agent header
func GetUserAgent(ctx *lift.Context) string {
	return ctx.Header("User-Agent")
}

// GetClientIP retrieves the client IP address
func GetClientIP(ctx *lift.Context) string {
	// Try X-Forwarded-For first (common in load balancers)
	if ip := ctx.Header("X-Forwarded-For"); ip != "" {
		return ip
	}

	// Try X-Real-IP
	if ip := ctx.Header("X-Real-IP"); ip != "" {
		return ip
	}

	// Fallback to remote address (may not be available in Lambda)
	return ctx.Header("X-Forwarded-For")
}

// GetContentType retrieves the Content-Type header
func GetContentType(ctx *lift.Context) string {
	return ctx.Header("Content-Type")
}

// GetAcceptHeader retrieves the Accept header
func GetAcceptHeader(ctx *lift.Context) string {
	return ctx.Header("Accept")
}

// Query parameter helpers

// GetQueryParam retrieves a query parameter with optional default value
func GetQueryParam(ctx *lift.Context, key string, defaultValue ...string) string {
	value := ctx.Query(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// GetQueryParamInt retrieves a query parameter as integer with optional default
func GetQueryParamInt(ctx *lift.Context, key string, defaultValue ...int) int {
	value := ctx.Query(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// GetQueryParamBool retrieves a query parameter as boolean
func GetQueryParamBool(ctx *lift.Context, key string) bool {
	value := ctx.Query(key)
	return value == "true" || value == "1" || value == "yes"
}

// Path parameter helpers

// GetPathParam retrieves a path parameter
func GetPathParam(ctx *lift.Context, key string) string {
	return ctx.PathParam(key)
}

// GetPathParamInt retrieves a path parameter as integer
func GetPathParamInt(ctx *lift.Context, key string) (int, error) {
	value := ctx.PathParam(key)
	if value == "" {
		return 0, ctx.BadRequest("Missing path parameter: "+key, nil)
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, ctx.BadRequest("Invalid path parameter: "+key, err)
	}

	return intValue, nil
}

// Header helpers

// SetCacheHeaders sets appropriate cache headers
func SetCacheHeaders(ctx *lift.Context, maxAge int) {
	ctx.Response.Header("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
}

// SetNoCacheHeaders sets no-cache headers
func SetNoCacheHeaders(ctx *lift.Context) {
	ctx.Response.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	ctx.Response.Header("Pragma", "no-cache")
	ctx.Response.Header("Expires", "0")
}

// SetSecurityHeaders sets common security headers
func SetSecurityHeaders(ctx *lift.Context) {
	ctx.Response.Header("X-Content-Type-Options", "nosniff")
	ctx.Response.Header("X-Frame-Options", "DENY")
	ctx.Response.Header("X-XSS-Protection", "1; mode=block")
	ctx.Response.Header("Referrer-Policy", "strict-origin-when-cross-origin")
}

// Validation helpers

// ValidateRequired checks if required fields are present
func ValidateRequired(ctx *lift.Context, fields map[string]string) error {
	for field, value := range fields {
		if value == "" {
			return ctx.BadRequest("Missing required field: "+field, nil)
		}
	}
	return nil
}

// ValidateEmail performs basic email validation
func ValidateEmail(email string) bool {
	// Basic email validation - in production, use a proper email validation library
	return len(email) > 3 &&
		len(email) <= 254 &&
		email[0] != '@' &&
		email[len(email)-1] != '@' &&
		containsChar(email, '@') &&
		containsChar(email, '.')
}

// containsChar checks if a string contains a specific character
func containsChar(s string, c rune) bool {
	for _, char := range s {
		if char == c {
			return true
		}
	}
	return false
}

// Authentication context helpers (re-exported from auth.go for convenience)

// GetAuthenticatedUser retrieves the authenticated user's claims
func GetAuthenticatedUser(ctx *lift.Context) (*auth.EnhancedClaims, error) {
	return GetClaims(ctx)
}

// GetAuthenticatedUsername retrieves the authenticated user's username
func GetAuthenticatedUsername(ctx *lift.Context) (string, error) {
	return GetUsername(ctx)
}

// GetOptionalAuthenticatedUsername retrieves username if authenticated, empty string otherwise
func GetOptionalAuthenticatedUsername(ctx *lift.Context) string {
	return GetOptionalUsername(ctx)
}

// IsUserAuthenticated checks if the current request is authenticated
func IsUserAuthenticated(ctx *lift.Context) bool {
	return IsAuthenticated(ctx)
}

// CheckUserScope checks if the authenticated user has a specific scope
func CheckUserScope(ctx *lift.Context, scope string) bool {
	return HasScope(ctx, scope)
}

// GetCurrentTenant retrieves the current tenant ID
func GetCurrentTenant(ctx *lift.Context) (string, error) {
	return GetTenantID(ctx)
}

// GetCurrentSession retrieves the current session ID
func GetCurrentSession(ctx *lift.Context) (string, error) {
	return GetSessionID(ctx)
}
