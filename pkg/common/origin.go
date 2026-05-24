package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// OriginURLFromHeaders reconstructs the public request origin from forwarded headers.
// Lesser-specific forwarded headers win because CloudFront strips the public Host header
// before origin verification on API behaviors. Forwarded host values must be plain
// host[:port] authority values; malformed proxy input is ignored rather than trusted.
func OriginURLFromHeaders(headers map[string][]string) string {
	if origin := lesserForwardedOriginURL(headers); origin != "" {
		return origin
	}
	if origin := standardForwardedOriginURL(headers); origin != "" {
		return origin
	}
	if origin := xForwardedOriginURL(headers); origin != "" {
		return origin
	}
	return ""
}

// RequestURLFromHeaders reconstructs the request URL used for signature verification.
func RequestURLFromHeaders(headers map[string][]string, path string, query map[string][]string) *url.URL {
	origin := OriginURLFromHeaders(headers)
	if origin == "" {
		if host, ok := normalizeForwardedHost(headerMapValue(headers, HostHeader)); ok {
			origin = SchemeHTTPS + "://" + host
		} else {
			origin = SchemeHTTPS + "://"
		}
	}

	u, err := url.Parse(origin)
	if err != nil {
		u = &url.URL{Scheme: SchemeHTTPS}
	}
	if u.Scheme == "" {
		u.Scheme = SchemeHTTPS
	}
	if _, ok := normalizeForwardedHost(u.Host); !ok {
		u.Host = ""
	}

	u.Path = path

	values := u.Query()
	for key, entries := range query {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	u.RawQuery = values.Encode()

	return u
}

func standardForwardedOriginURL(headers map[string][]string) string {
	forwarded := firstCommaSeparated(headerMapValue(headers, "Forwarded"))
	if forwarded == "" {
		return ""
	}
	var host string
	proto := SchemeHTTPS
	for _, part := range strings.Split(forwarded, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "host":
			host = value
		case "proto":
			proto = strings.ToLower(value)
		}
	}
	normalizedHost, ok := normalizeForwardedHost(host)
	if !ok {
		return ""
	}
	switch proto {
	case SchemeHTTP, SchemeHTTPS:
	default:
		proto = SchemeHTTPS
	}
	return proto + "://" + normalizedHost
}

func xForwardedOriginURL(headers map[string][]string) string {
	host, ok := normalizeForwardedHost(firstCommaSeparated(headerMapValue(headers, "X-Forwarded-Host")))
	if !ok {
		return ""
	}
	proto := strings.ToLower(firstCommaSeparated(headerMapValue(headers, XForwardedProtoHeader)))
	switch proto {
	case SchemeHTTP, SchemeHTTPS:
	default:
		proto = SchemeHTTPS
	}
	return proto + "://" + host
}

func lesserForwardedOriginURL(headers map[string][]string) string {
	host, ok := normalizeForwardedHost(headerMapValue(headers, XLesserForwardedHost))
	if !ok {
		return ""
	}

	proto := strings.ToLower(headerMapValue(headers, XLesserForwardedProto))
	if proto == "" {
		proto = strings.ToLower(headerMapValue(headers, XForwardedProtoHeader))
	}
	switch proto {
	case SchemeHTTP, SchemeHTTPS:
	default:
		proto = SchemeHTTPS
	}

	return proto + "://" + host
}

func normalizeForwardedHost(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\t /\\?#@") {
		return "", false
	}
	u, err := url.Parse(SchemeHTTPS + "://" + raw)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" || strings.ContainsAny(host, "\r\n\t /\\?#@") {
		return "", false
	}
	port := u.Port()
	if port == "" {
		if strings.Contains(host, ":") && net.ParseIP(host) != nil {
			return "[" + host + "]", true
		}
		return host, true
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}

func firstCommaSeparated(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func headerMapValue(headers map[string][]string, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	for existingKey, values := range headers {
		if strings.ToLower(strings.TrimSpace(existingKey)) != key {
			continue
		}
		if len(values) == 0 {
			return ""
		}
		return strings.TrimSpace(values[0])
	}
	return ""
}

// ValidateRequestOriginDomain checks that the origin reconstructed from forwarded
// headers matches the expected instance domain. This prevents forwarded-host spoofing
// in federation-facing endpoints (actor, objects, inbox) where the reconstructed
// origin feeds into ActivityPub HTTP Signature verification.
//
// Returns nil when validation passes or when localDomain is empty (caller cannot
// bind without a known domain). Returns an error when the origin host differs from
// the expected local domain.
func ValidateRequestOriginDomain(headers map[string][]string, localDomain string) error {
	if strings.TrimSpace(localDomain) == "" {
		return nil
	}

	origin := OriginURLFromHeaders(headers)
	if origin == "" {
		return errors.New("origin domain validation failed: no origin from forwarded headers")
	}

	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("origin domain validation failed: invalid origin URL: %w", err)
	}

	originHost := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if originHost == "" {
		return errors.New("origin domain validation failed: missing origin host")
	}

	expectedHost := strings.ToLower(strings.TrimSpace(localDomain))
	if originHost != expectedHost {
		return fmt.Errorf("origin host %q does not match instance host %q", originHost, expectedHost)
	}

	return nil
}
