package auth

import (
	"net"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
)

// RedirectURIsMatch applies the registered redirect policy for an OAuth client.
// Public native clients may vary only the port of an otherwise identical
// loopback redirect URI, as required by RFC 8252. All other clients and URI
// components retain exact-match behavior.
func RedirectURIsMatch(client *storage.OAuthClient, registeredURI, presentedURI string) bool {
	if registeredURI == presentedURI {
		return true
	}
	if !allowsLoopbackRedirectPortVariance(client) {
		return false
	}

	registered, err := url.Parse(registeredURI)
	if err != nil {
		return false
	}
	presented, err := url.Parse(presentedURI)
	if err != nil {
		return false
	}

	return loopbackRedirectURIsEqualExceptPort(registered, presented)
}

func allowsLoopbackRedirectPortVariance(client *storage.OAuthClient) bool {
	return client != nil &&
		!client.Confidential &&
		strings.EqualFold(strings.TrimSpace(client.ClientClass), ClientClassCLI)
}

func loopbackRedirectURIsEqualExceptPort(registered, presented *url.URL) bool {
	if registered == nil || presented == nil ||
		registered.Scheme != "http" || registered.Scheme != presented.Scheme {
		return false
	}

	// RFC 8252 keeps every component except the port exact. In particular,
	// localhost does not match an IP literal, and loopback IPs are not a host
	// wildcard for one another.
	registeredHost := registered.Hostname()
	presentedHost := presented.Hostname()
	if registeredHost == "" || registeredHost != presentedHost ||
		!isLoopbackRedirectHost(registeredHost) ||
		!isLoopbackRedirectHost(presentedHost) {
		return false
	}

	// Flexible matching is limited to the port. Keep every other parsed URI
	// component exact, including escaped path spelling, query, and fragment.
	return registered.Opaque == presented.Opaque &&
		registered.User == nil && presented.User == nil &&
		registered.EscapedPath() == presented.EscapedPath() &&
		registered.RawQuery == presented.RawQuery &&
		registered.ForceQuery == presented.ForceQuery &&
		registered.EscapedFragment() == presented.EscapedFragment() &&
		registered.OmitHost == presented.OmitHost
}

func isLoopbackRedirectHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
