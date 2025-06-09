package common

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

func ValidateRedirectURL(redirectURL string, currentHost string) error {
	if redirectURL == "" {
		return fmt.Errorf("redirect URL cannot be empty")
	}

	// Parse the URL
	u, err := url.Parse(redirectURL)
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %w", err)
	}

	// Relative URLs are safe (same origin)
	if u.Host == "" {
		// But check for protocol-relative URLs
		if strings.HasPrefix(redirectURL, "//") {
			return fmt.Errorf("protocol-relative URLs not allowed")
		}
		// Check for javascript: or data: URLs
		if u.Scheme == "javascript" || u.Scheme == "data" {
			return fmt.Errorf("javascript: and data: URLs not allowed")
		}
		return nil
	}

	// Check against whitelist
	if allowedRedirectHosts[u.Host] {
		return nil
	}

	// Allow same host
	if u.Host == currentHost {
		return nil
	}

	return fmt.Errorf("redirect to external host not allowed: %s", u.Host)
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

	// Validate the redirect URL
	err := ValidateRedirectURL(redirectTo, r.Host)
	if err != nil {
		Logger().Warn("Invalid redirect attempt",
			zap.String("url", redirectTo),
			zap.Error(err))
		redirectTo = defaultPath
	}

	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// SafeRedirectOrDefault redirects to the given URL if safe, otherwise to default
func SafeRedirectOrDefault(w http.ResponseWriter, r *http.Request, redirectURL, defaultPath string) {
	err := ValidateRedirectURL(redirectURL, r.Host)
	if err != nil {
		Logger().Warn("Invalid redirect, using default",
			zap.String("requested_url", redirectURL),
			zap.String("default_url", defaultPath),
			zap.Error(err))
		redirectURL = defaultPath
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ConfigureAllowedRedirectHosts updates the allowed redirect hosts
func ConfigureAllowedRedirectHosts(hosts []string) {
	newAllowed := make(map[string]bool)
	for _, host := range hosts {
		newAllowed[host] = true
	}
	allowedRedirectHosts = newAllowed
}

// GetSafeRedirectURL returns a validated redirect URL or the default
func GetSafeRedirectURL(redirectURL, currentHost, defaultPath string) string {
	err := ValidateRedirectURL(redirectURL, currentHost)
	if err != nil {
		return defaultPath
	}
	return redirectURL
}
