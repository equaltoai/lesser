package auth

import "strings"

// AgentOwnerMatchesLocalPrincipal reports whether a stored AgentOwner identifies
// the local principal by username or by the principal's local actor URL. When the
// principal is itself an actor URL (an owner stored in URL form and carried through
// DelegatedBy), the two URLs are compared directly rather than re-wrapped through
// actorURL, which would never match.
func AgentOwnerMatchesLocalPrincipal(owner, principalUsername string, actorURL func(string) string) bool {
	principalUsername = strings.TrimSpace(principalUsername)
	owner = strings.TrimSpace(owner)
	if principalUsername == "" || owner == "" {
		return false
	}

	lowerPrincipal := strings.ToLower(principalUsername)
	if strings.HasPrefix(lowerPrincipal, "http://") || strings.HasPrefix(lowerPrincipal, "https://") {
		return strings.EqualFold(owner, principalUsername)
	}

	lowerOwner := strings.ToLower(owner)
	if strings.HasPrefix(lowerOwner, "http://") || strings.HasPrefix(lowerOwner, "https://") {
		return actorURL != nil && strings.EqualFold(owner, actorURL(principalUsername))
	}

	owner = strings.TrimPrefix(owner, "@")
	if strings.Contains(owner, "/") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(owner), principalUsername)
}
