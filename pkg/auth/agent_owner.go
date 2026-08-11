package auth

import "strings"

// AgentOwnerMatchesLocalPrincipal reports whether a stored AgentOwner identifies
// the local principal by username or by the principal's local actor URL.
func AgentOwnerMatchesLocalPrincipal(owner, principalUsername string, actorURL func(string) string) bool {
	principalUsername = strings.TrimSpace(principalUsername)
	owner = strings.TrimSpace(owner)
	if principalUsername == "" || owner == "" {
		return false
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
