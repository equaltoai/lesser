package handlers

import (
	"net/http"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
)

// Example of setting secure session cookie in Lambda response
func setSessionCookie(response *events.APIGatewayV2HTTPResponse, sessionToken string) {
	// Create secure cookie string
	cookie := &http.Cookie{
		Name:     "lesser_session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   3600,                 // 1 hour
		Secure:   true,                 // HTTPS only
		HttpOnly: true,                 // No JavaScript access
		SameSite: http.SameSiteLaxMode, // CSRF protection, Lax for OAuth flows
	}

	// Add Set-Cookie header to response
	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}
	response.Headers["Set-Cookie"] = cookie.String()
}

// Example of setting multiple cookies (session + refresh)
func setAuthCookies(response *events.APIGatewayV2HTTPResponse, sessionToken, refreshToken string) {
	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}

	// Session cookie (short-lived)
	sessionCookie := &http.Cookie{
		Name:     "lesser_session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   3600, // 1 hour
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	// Refresh cookie (long-lived)
	refreshCookie := &http.Cookie{
		Name:     "lesser_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth/refresh", // Restrict to refresh endpoint
		MaxAge:   86400 * 30,             // 30 days
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode, // Strict for refresh tokens
	}

	// API Gateway doesn't support multiple Set-Cookie headers directly
	// You would need to use multiValueHeaders in the response
	response.Headers["Set-Cookie"] = sessionCookie.String()

	// For multiple cookies in API Gateway v2, use MultiValueHeaders
	if response.MultiValueHeaders == nil {
		response.MultiValueHeaders = make(map[string][]string)
	}
	response.MultiValueHeaders["Set-Cookie"] = []string{
		sessionCookie.String(),
		refreshCookie.String(),
	}
}

// Example of deleting cookies on logout
func clearAuthCookies(response *events.APIGatewayV2HTTPResponse) {
	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}

	// Delete session cookie
	sessionCookie := &http.Cookie{
		Name:     "lesser_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete cookie
		Expires:  time.Unix(0, 0),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	// Delete refresh cookie
	refreshCookie := &http.Cookie{
		Name:     "lesser_refresh",
		Value:    "",
		Path:     "/api/v1/auth/refresh",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	// Use MultiValueHeaders for multiple cookies
	if response.MultiValueHeaders == nil {
		response.MultiValueHeaders = make(map[string][]string)
	}
	response.MultiValueHeaders["Set-Cookie"] = []string{
		sessionCookie.String(),
		refreshCookie.String(),
	}
}

// Example usage in a login handler
func exampleLoginSuccessResponse(username, sessionToken, refreshToken string) *events.APIGatewayV2HTTPResponse {
	response := &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: `{"status": "success", "username": "` + username + `"}`,
	}

	// Set secure auth cookies
	setAuthCookies(response, sessionToken, refreshToken)

	// Apply security headers
	response.Headers["X-Content-Type-Options"] = "nosniff"
	response.Headers["X-Frame-Options"] = "DENY"
	response.Headers["X-XSS-Protection"] = "1; mode=block"
	response.Headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains"

	return response
}

// Configuration for different cookie types
var (
	// SessionCookieConfig for short-lived session tokens
	SessionCookieConfig = common.CookieConfig{
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // Lax for OAuth compatibility
	}

	// RefreshCookieConfig for long-lived refresh tokens
	RefreshCookieConfig = common.CookieConfig{
		Path:     "/api/v1/auth/refresh",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode, // Strict for better security
	}

	// CSRFCookieConfig for CSRF tokens (readable by JavaScript)
	CSRFCookieConfig = common.CookieConfig{
		Path:     "/",
		Secure:   true,
		HttpOnly: false, // JavaScript needs to read this
		SameSite: http.SameSiteStrictMode,
	}
)
