package common

import (
	"fmt"
	"net/url"
	"strings"
)

// CanonicalActorID validates and canonicalizes an ActivityPub actor URI.
func CanonicalActorID(actorID string) (string, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "", fmt.Errorf("actor ID is required")
	}
	if actorIDContainsControlRune(actorID) || strings.ContainsAny(actorID, " \t\r\n") {
		return "", fmt.Errorf("actor ID contains invalid characters")
	}
	if err := ValidateActivityPubURL(actorID, "actor_id"); err != nil {
		return "", err
	}

	parsed, err := url.Parse(actorID)
	if err != nil || parsed == nil {
		return "", fmt.Errorf("actor ID must be a valid URL")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("actor ID must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("actor ID must not contain query or fragment")
	}
	if parsed.Opaque != "" || strings.TrimSpace(parsed.Hostname()) == "" || strings.TrimSpace(parsed.Path) == "" || parsed.Path == "/" {
		return "", fmt.Errorf("actor ID must include host and path")
	}
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		if segment == "" {
			continue
		}
		unescaped, err := url.PathUnescape(segment)
		if err != nil {
			return "", fmt.Errorf("actor ID path contains invalid escaping")
		}
		if unescaped == "." || unescaped == ".." || actorIDContainsControlRune(unescaped) {
			return "", fmt.Errorf("actor ID path contains unsafe segment")
		}
	}

	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "/users" || path == "/@" {
		return "", fmt.Errorf("actor ID must include host and path")
	}
	if _, _, err := activityPubUsernameFromActorPath(path); err != nil {
		return "", err
	}

	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + path, nil
}

// ActorUsernameFromID extracts a local ActivityPub username from an actor
// identifier. Lesser-owned actor routes are intentionally strict: actor URLs of
// the form /users/<username> and /@<username> must contain exactly one username
// segment. Multi-segment paths such as /users/admin/mallory are not a username
// for one component and a storage key for another; they are invalid.
func ActorUsernameFromID(actorID string) (string, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "", fmt.Errorf("actor ID is required")
	}

	if !strings.Contains(actorID, "://") {
		username := strings.TrimPrefix(actorID, "@")
		if err := ValidateActivityPubUsername(username); err != nil {
			return "", err
		}
		return username, nil
	}

	canonical, err := CanonicalActorID(actorID)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(canonical)
	if err != nil || parsed == nil {
		return "", fmt.Errorf("actor ID must be a valid URL")
	}
	username, ok, err := activityPubUsernameFromActorPath(parsed.EscapedPath())
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("actor ID path is not a supported local actor route")
	}
	return username, nil
}

// SameCanonicalActorID reports whether two ActivityPub actor URI strings
// canonicalize to the same actor ID.
func SameCanonicalActorID(left, right string) bool {
	leftCanonical, leftErr := CanonicalActorID(left)
	rightCanonical, rightErr := CanonicalActorID(right)
	return leftErr == nil && rightErr == nil && leftCanonical == rightCanonical
}

// CachedRemoteActorIDMatchesLookup reports whether a cached remote actor ID may
// satisfy a cache lookup. Handle/acct lookups have no canonical actor URL to
// compare, so they are allowed; HTTP(S) URL lookups require exact canonical ID
// equality, and malformed actor URLs do not match cached actors.
func CachedRemoteActorIDMatchesLookup(cachedActorID, lookup string) bool {
	lookup = strings.TrimSpace(lookup)
	lowerLookup := strings.ToLower(lookup)
	if !strings.HasPrefix(lowerLookup, "http://") && !strings.HasPrefix(lowerLookup, "https://") {
		return true
	}
	return SameCanonicalActorID(cachedActorID, lookup)
}

func actorIDContainsControlRune(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func activityPubUsernameFromActorPath(path string) (string, bool, error) {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	switch {
	case strings.HasPrefix(path, "/users/"):
		return activityPubUsernameFromSingleSegment(strings.TrimPrefix(path, "/users/"), true)
	case strings.HasPrefix(path, "/@"):
		return activityPubUsernameFromSingleSegment(strings.TrimPrefix(path, "/@"), true)
	case path == "/users" || path == "/@":
		return "", true, fmt.Errorf("actor ID must include a concrete actor username")
	default:
		return "", false, nil
	}
}

func activityPubUsernameFromSingleSegment(escapedUsername string, knownActorPath bool) (string, bool, error) {
	if escapedUsername == "" || strings.Contains(escapedUsername, "/") {
		return "", knownActorPath, fmt.Errorf("actor ID must use exactly one actor username segment")
	}
	username, err := url.PathUnescape(escapedUsername)
	if err != nil {
		return "", knownActorPath, fmt.Errorf("actor ID username contains invalid escaping")
	}
	if err := ValidateActivityPubUsername(username); err != nil {
		return "", knownActorPath, err
	}
	return username, knownActorPath, nil
}
