// Package cors provides shared browser CORS origin normalization helpers.
package cors

import (
	"net/url"
	"strings"
)

// DenyAllAllowlist is a syntactically invalid origin-list sentinel used when an
// operator supplied only invalid origins. Runtime parsers ignore it and deploy
// templates never match it, preserving fail-closed behavior without falling back
// to the instance-origin default.
const DenyAllAllowlist = "https://invalid.invalid/lesser-cors-deny-all"

// NormalizeOrigin returns a normalized origin string and parsed URL when raw is
// exactly an origin. Paths other than '/', queries, fragments, opaque URLs, and
// userinfo are rejected.
func NormalizeOrigin(raw string) (string, *url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, false
	}

	if parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil {
		return "", nil, false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", nil, false
	}
	if path := strings.TrimSpace(parsed.Path); path != "" && path != "/" {
		return "", nil, false
	}

	normalized := (&url.URL{
		Scheme: strings.ToLower(parsed.Scheme),
		Host:   strings.ToLower(parsed.Host),
	}).String()
	return normalized, parsed, true
}

// ParseAllowedOrigins parses a comma-separated CORS allowlist for runtime use.
// Invalid entries are ignored fail-closed. A literal '*' is preserved only when
// explicitly supplied.
func ParseAllowedOrigins(raw string) []string {
	seen := map[string]struct{}{}
	allowedOrigins := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return []string{"*"}
		}

		normalized, _, ok := NormalizeOrigin(origin)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		allowedOrigins = append(allowedOrigins, normalized)
	}
	return allowedOrigins
}

// NormalizeAllowedOriginsForDeploy normalizes a comma-separated allowlist before
// it is embedded in deploy-time infrastructure. An empty raw value means
// "use the instance-origin default". A non-empty raw value with no valid entries
// returns DenyAllAllowlist so deploy-time preflights fail closed instead of
// accidentally reverting to the default origin.
func NormalizeAllowedOriginsForDeploy(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	allowedOrigins := ParseAllowedOrigins(raw)
	if len(allowedOrigins) == 0 {
		return DenyAllAllowlist
	}
	return strings.Join(allowedOrigins, ",")
}
