package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"net/url"
	"strings"

	apptheory "github.com/theory-cloud/apptheory/runtime"
)

// OriginURLFromHeaders reconstructs the public request origin from forwarded headers.
// Lesser-specific forwarded headers win because CloudFront strips the public Host header
// before origin verification on API behaviors.
func OriginURLFromHeaders(headers map[string][]string) string {
	if origin := lesserForwardedOriginURL(headers); origin != "" {
		return origin
	}
	return apptheory.OriginURL(headers)
}

// RequestURLFromHeaders reconstructs the request URL used for signature verification.
func RequestURLFromHeaders(headers map[string][]string, path string, query map[string][]string) *url.URL {
	origin := OriginURLFromHeaders(headers)
	if origin == "" {
		if host := headerMapValue(headers, HostHeader); host != "" {
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

func lesserForwardedOriginURL(headers map[string][]string) string {
	host := headerMapValue(headers, XLesserForwardedHost)
	if host == "" {
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
