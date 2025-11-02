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

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		if parsed, err := url.Parse(host); err == nil {
			host = parsed.Host
		}
	}
	if colon := strings.Index(host, ":"); colon != -1 {
		host = host[:colon]
	}
	return strings.ToLower(host)
}

// ValidateRedirectURL validates that a redirect URL is safe and allowed
func ValidateRedirectURL(redirectURL string, currentHost string) error {
	redirectURL = strings.TrimSpace(redirectURL)
	if redirectURL == "" {
		return ErrRedirectURLEmpty
	}
	if strings.HasPrefix(redirectURL, "//") {
		return ErrProtocolRelativeURLsNotAllowed
	}

	// Parse the URL
	u, err := url.Parse(redirectURL)
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "" && scheme != "http" && scheme != "https" {
		return ErrRedirectSchemeNotAllowed
	}

	// Relative URLs are safe (same origin)
	if u.Host == "" {
		// But check for disguised schemes
		if scheme != "" {
			return ErrRedirectSchemeNotAllowed
		}
		return nil
	}

	// Check against whitelist
	host := normalizeHost(u.Host)
	if host == "" {
		return ErrExternalHostNotAllowed
	}

	if allowedRedirectHosts[host] {
		return nil
	}

	current := normalizeHost(currentHost)
	if current != "" && host == current {
		return nil
	}

	return fmt.Errorf("redirect to external host not allowed: %s", host)
}

// SafeRedirect performs a safe redirect with validation
func SafeRedirect(w http.ResponseWriter, r *http.Request, defaultPath string) {
	raw := r.URL.Query().Get("redirect_uri")
	if raw == "" {
		raw = r.URL.Query().Get("return_to")
	}
	if raw == "" {
		raw = r.URL.Query().Get("next")
	}

	target := defaultPath
	if raw != "" {
		if sanitized, err := sanitizedRedirectTarget(raw, r.Host); err == nil {
			target = sanitized
		} else {
			Logger().Warn("Invalid redirect attempt",
				zap.String("url", raw),
				zap.Error(err))
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// SafeRedirectOrDefault redirects to the given URL if safe, otherwise to default
func SafeRedirectOrDefault(w http.ResponseWriter, r *http.Request, redirectURL, defaultPath string) {
	target := defaultPath
	if redirectURL != "" {
		if sanitized, err := sanitizedRedirectTarget(redirectURL, r.Host); err == nil {
			target = sanitized
		} else {
			Logger().Warn("Invalid redirect, using default",
				zap.String("requested_url", redirectURL),
				zap.String("default_url", defaultPath),
				zap.Error(err))
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// ConfigureAllowedRedirectHosts updates the allowed redirect hosts
func ConfigureAllowedRedirectHosts(hosts []string) {
	newAllowed := make(map[string]bool)
	for _, host := range hosts {
		normalized := normalizeHost(host)
		if normalized != "" {
			newAllowed[normalized] = true
		}
	}
	allowedRedirectHosts = newAllowed
}

// GetSafeRedirectURL returns a validated redirect URL or the default
func GetSafeRedirectURL(redirectURL, currentHost, defaultPath string) string {
	if sanitized, err := sanitizedRedirectTarget(redirectURL, currentHost); err == nil {
		return sanitized
	}
	return defaultPath
}

func sanitizedRedirectTarget(redirectURL, currentHost string) (string, error) {
	if err := ValidateRedirectURL(redirectURL, currentHost); err != nil {
		return "", err
	}

	u, err := url.Parse(redirectURL)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URL: %w", err)
	}

	if u.Host == "" {
		if u.Path == "" {
			return "/", nil
		}
		return u.String(), nil
	}

	// For allowed external hosts, redirect to the same path on the current origin to avoid open redirects.
	clean := &url.URL{
		Path:     u.Path,
		RawQuery: u.RawQuery,
		Fragment: u.Fragment,
	}
	if result := clean.String(); result != "" {
		if strings.HasPrefix(result, "/") {
			return result, nil
		}
		return "/" + result, nil
	}
	return "/", nil
}
