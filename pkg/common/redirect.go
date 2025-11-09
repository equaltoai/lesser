package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

// Whitelist of allowed redirect hosts
var allowedRedirectHosts = map[string]bool{
	"lesser.example.com":      true,
	"auth.lesser.example.com": true,
}

// ValidateRedirectURL validates that a redirect URL is safe and allowed and returns the sanitized value.
func ValidateRedirectURL(redirectURL string, currentHost string) (string, error) {
	redirectURL = strings.TrimSpace(redirectURL)
	if redirectURL == "" {
		return "", ErrRedirectURLEmpty
	}

	// Parse the URL
	u, err := url.Parse(redirectURL)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)

	// Relative URLs are safe (same origin)
	if u.Host == "" {
		// But check for protocol-relative URLs
		if strings.HasPrefix(redirectURL, "//") {
			return "", ErrProtocolRelativeURLsNotAllowed
		}
		// Check for javascript: or data: URLs
		if scheme == "javascript" || scheme == "data" {
			return "", ErrJavascriptDataURLsNotAllowed
		}
		return u.String(), nil
	}

	if scheme != "" && scheme != SchemeHTTP && scheme != SchemeHTTPS {
		return "", fmt.Errorf("redirect scheme %q is not allowed", scheme)
	}

	host := strings.ToLower(u.Host)

	// Check against whitelist
	if allowedRedirectHosts[host] {
		return u.String(), nil
	}

	// Allow same host
	if strings.EqualFold(u.Host, currentHost) {
		return u.String(), nil
	}

	return "", fmt.Errorf("redirect to external host not allowed: %s", u.Host)
}

// SafeRedirect performs a safe redirect with validation
func SafeRedirect(w http.ResponseWriter, r *http.Request, defaultPath string) {
	redirectTo := r.URL.Query().Get("redirect_uri")
	if redirectTo == "" {
		redirectTo = r.URL.Query().Get("return_to")
	}
	if redirectTo == "" {
		redirectTo = r.URL.Query().Get("next")
	}

	target := defaultPath
	if redirectTo != "" {
		sanitized, err := ValidateRedirectURL(redirectTo, r.Host)
		if err != nil {
			Logger().Warn("Invalid redirect attempt",
				zap.String("url", redirectTo),
				zap.Error(err))
		} else {
			target = sanitized
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// SafeRedirectOrDefault redirects to the given URL if safe, otherwise to default
func SafeRedirectOrDefault(w http.ResponseWriter, r *http.Request, redirectURL, defaultPath string) {
	target := defaultPath
	if redirectURL != "" {
		sanitized, err := ValidateRedirectURL(redirectURL, r.Host)
		if err != nil {
			Logger().Warn("Invalid redirect, using default",
				zap.String("requested_url", redirectURL),
				zap.String("default_url", defaultPath),
				zap.Error(err))
		} else {
			target = sanitized
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// ConfigureAllowedRedirectHosts updates the allowed redirect hosts
func ConfigureAllowedRedirectHosts(hosts []string) {
	newAllowed := make(map[string]bool)
	for _, host := range hosts {
		newAllowed[strings.ToLower(host)] = true
	}
	allowedRedirectHosts = newAllowed
}

// GetSafeRedirectURL returns a validated redirect URL or the default
func GetSafeRedirectURL(redirectURL, currentHost, defaultPath string) string {
	if redirectURL == "" {
		return defaultPath
	}
	sanitized, err := ValidateRedirectURL(redirectURL, currentHost)
	if err != nil {
		return defaultPath
	}
	return sanitized
}
