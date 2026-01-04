package ssrf

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// ErrInvalidScheme indicates a URL used a non-http(s) scheme.
	ErrInvalidScheme = errors.New("invalid URL scheme")
	// ErrEmptyHostname indicates a URL is missing a hostname.
	ErrEmptyHostname = errors.New("empty hostname")
	// ErrBlockedHostname indicates a URL hostname is blocked by SSRF policy.
	ErrBlockedHostname = errors.New("hostname is blocked")
)

// ValidateURL validates that a URL is safe to resolve and dial from an SSRF perspective.
//
// It enforces:
// - http/https schemes only
// - non-empty hostname
// - hostname is not blocked by IsBlockedHostname (including IP literals)
func ValidateURL(u *url.URL) error {
	if u == nil {
		return errors.New("nil URL")
	}

	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: %s", ErrInvalidScheme, scheme)
	}

	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ErrEmptyHostname
	}

	if IsBlockedHostname(host) {
		return fmt.Errorf("%w: %s", ErrBlockedHostname, host)
	}

	return nil
}

// ValidateURLString parses and validates a URL string using ValidateURL.
func ValidateURLString(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	if err := ValidateURL(parsed); err != nil {
		return parsed, err
	}

	return parsed, nil
}
