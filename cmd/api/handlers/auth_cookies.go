package handlers

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/common"
)

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
