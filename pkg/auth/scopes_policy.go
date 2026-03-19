package auth

import "strings"

const (
	// ScopeFollow grants relationship-management capability on public OAuth surfaces.
	ScopeFollow = "follow"
	// ScopePush grants push-subscription capability on public OAuth surfaces.
	ScopePush = "push"

	scopeWriteFollowsAlias = "write:follows"
)

var canonicalOAuthScopes = []string{
	ScopeRead,
	ScopeWrite,
	ScopeFollow,
	ScopePush,
}

var defaultOAuthScopes = []string{
	ScopeRead,
	ScopeWrite,
}

// CanonicalOAuthScopes returns the externally-advertised Lesser OAuth scope catalog.
func CanonicalOAuthScopes() []string {
	return append([]string(nil), canonicalOAuthScopes...)
}

func normalizeScopeValue(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}

func scopeBase(scope string) string {
	scope = normalizeScopeValue(scope)
	if scope == "" {
		return ""
	}
	base, _, _ := strings.Cut(scope, ":")
	return base
}

func isCanonicalOAuthScope(scope string) bool {
	switch normalizeScopeValue(scope) {
	case ScopeRead, ScopeWrite, ScopeFollow, ScopePush:
		return true
	default:
		return false
	}
}

func isPublicOAuthCompatibilityScope(scope string) bool {
	scope = normalizeScopeValue(scope)
	if scope == "" {
		return false
	}

	switch scopeBase(scope) {
	case ScopeRead, ScopeWrite:
		return true
	default:
		return false
	}
}

func isRecognizedOAuthScope(scope string) bool {
	scope = normalizeScopeValue(scope)
	if scope == "" {
		return false
	}

	if isCanonicalOAuthScope(scope) || isPublicOAuthCompatibilityScope(scope) {
		return true
	}

	return scopeBase(scope) == ScopeAdmin
}

// ValidatePublicOAuthScopes validates externally requestable scopes.
// Canonical scopes are advertised publicly; read:* and write:* families remain accepted as
// compatibility aliases for legacy clients and stored records.
func ValidatePublicOAuthScopes(scopes []string) error {
	for _, scope := range scopes {
		if !isCanonicalOAuthScope(scope) && !isPublicOAuthCompatibilityScope(scope) {
			return ErrInvalidScope
		}
	}

	return nil
}

// ScopeGrantAllows reports whether one granted scope satisfies a requested scope, including
// broad-scope implications and the legacy write:follows compatibility alias.
func ScopeGrantAllows(granted, requested string) bool {
	granted = normalizeScopeValue(granted)
	requested = normalizeScopeValue(requested)
	if granted == "" || requested == "" {
		return false
	}

	if granted == requested {
		return true
	}

	switch requested {
	case ScopeFollow, scopeWriteFollowsAlias:
		return granted == ScopeWrite || granted == ScopeFollow || granted == scopeWriteFollowsAlias
	}

	switch scopeBase(requested) {
	case ScopeRead:
		return granted == ScopeRead
	case ScopeWrite:
		return granted == ScopeWrite
	case ScopeAdmin:
		return granted == ScopeAdmin || granted == requested
	default:
		return false
	}
}

// ScopeSetAllows reports whether every requested scope is satisfied by the granted scope set.
func ScopeSetAllows(grantedScopes, requestedScopes []string) bool {
	for _, requested := range requestedScopes {
		requested = normalizeScopeValue(requested)
		if requested == "" {
			continue
		}

		allowed := false
		for _, granted := range grantedScopes {
			if ScopeGrantAllows(granted, requested) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}
