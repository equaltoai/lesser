package common

import (
	"fmt"
	"net/url"
	"strings"
)

// IsLocalActorID returns true when the actor ID is a URL whose host matches
// localDomain (case-insensitive). It replaces ad-hoc strings.Contains checks
// that were vulnerable to substring false matches (e.g. "example.com" matching
// "evil-example.com").
//
// Non-URL actor IDs (bare usernames, user@domain handles) always return false.
func IsLocalActorID(actorID, localDomain string) bool {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(localDomain) == "" {
		return false
	}
	host := ExtractDomainFromActorID(actorID)
	if host == "" {
		return false
	}
	return strings.EqualFold(host, strings.TrimSpace(localDomain))
}

// IsHTTPActorID reports whether actorID is a parseable HTTP(S) actor URL with a host.
// Scheme and host case follow net/url semantics rather than raw prefix matching.
func IsHTTPActorID(actorID string) bool {
	u, ok := parseHTTPActorID(actorID)
	return ok && strings.TrimSpace(u.Host) != ""
}

// ExtractDomainFromActorID returns the host portion of an actor URL.
// Returns empty string when actorID is not a valid HTTP/HTTPS URL.
func ExtractDomainFromActorID(actorID string) string {
	u, ok := parseHTTPActorID(actorID)
	if !ok {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		host = u.Host
	}
	return strings.ToLower(strings.TrimSpace(host))
}

func parseHTTPActorID(actorID string) (*url.URL, bool) {
	trimmed := strings.TrimSpace(actorID)
	if trimmed == "" {
		return nil, false
	}
	u, err := url.Parse(trimmed)
	if err != nil || u == nil || u.Host == "" {
		return nil, false
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return nil, false
	}
	return u, true
}

// ValidateActorDomainConsistency verifies that a fetched actor document's
// declared actor ID belongs to the same domain as the URL it was fetched from.
// This prevents spoofing: a malicious server at evil.example should not be able
// to return an actor document claiming actor.ID = "https://victim.example/users/bob".
//
// Returns an error when the domains don't match, or when either argument is
// not a valid HTTP/HTTPS URL.
func ValidateActorDomainConsistency(fetchURL, declaredID string) error {
	fetchHost := ExtractDomainFromActorID(fetchURL)
	if fetchHost == "" {
		return fmt.Errorf("invalid fetch URL %q", fetchURL)
	}
	declaredHost := ExtractDomainFromActorID(declaredID)
	if declaredHost == "" {
		return fmt.Errorf("invalid declared actor ID %q", declaredID)
	}
	if !strings.EqualFold(fetchHost, declaredHost) {
		return fmt.Errorf("actor domain mismatch: fetched from %q but declared ID has domain %q", fetchHost, declaredHost)
	}
	return nil
}
