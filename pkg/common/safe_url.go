package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"net/url"
	"strings"
	"unicode"
)

// SafeHTTPURL returns a canonical HTTP(S) URL only when the input has no raw
// control characters, uses an allowed scheme, and includes a host.
func SafeHTTPURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsFunc(raw, unicode.IsControl) {
		return "", false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != SchemeHTTP && scheme != SchemeHTTPS {
		return "", false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}

	return parsed.String(), true
}
