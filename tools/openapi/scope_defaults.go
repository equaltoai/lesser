package main

import "strings"

func applyScopeDefaults(routes []routeDef) {
	for i := range routes {
		if len(routes[i].Scopes) > 0 {
			continue
		}
		if routes[i].Auth != authModeBearerRequired && routes[i].Auth != authModeBearerOptional {
			continue
		}
		routes[i].Scopes = defaultScopesForRoute(routes[i])
	}
}

func defaultScopesForRoute(route routeDef) []string {
	path := strings.TrimSpace(route.Path)
	if path == "" || path == pathGraphQL {
		return nil
	}

	if scopes := scopesForKnownPrefixes(route.Method, path); len(scopes) > 0 {
		return scopes
	}

	if isStatusesPath(path) {
		return scopesByMethod(route.Method, "read:statuses", "write:statuses")
	}

	if isAccountsPath(path) {
		return scopesByMethod(route.Method, "read:accounts", "write:accounts")
	}

	return scopesByMethod(route.Method, "read", "write")
}

func scopesForKnownPrefixes(method, path string) []string {
	switch {
	case method == methodGET && (path == "/api/v1/souls/mine" || path == "/api/v1/souls/bound/me"):
		return []string{"read", "write"}
	case strings.HasPrefix(path, "/api/v1/admin/"):
		return scopesByMethod(method, "admin:read", "admin:write")
	case strings.HasPrefix(path, "/api/v1/follow_requests"):
		return scopesByMethod(method, "read:follows", "write:follows")
	case strings.HasPrefix(path, "/api/v1/blocks"), strings.HasPrefix(path, "/api/v1/domain_blocks"):
		return scopesByMethod(method, "read:blocks", "write:blocks")
	case strings.HasPrefix(path, "/api/v1/push/"):
		return []string{"push"}
	case strings.HasPrefix(path, "/api/v1/notifications"):
		return scopesByMethod(method, "read:notifications", "write:notifications")
	case strings.HasPrefix(path, "/api/v1/filters"), strings.HasPrefix(path, "/api/v2/filters"):
		return scopesByMethod(method, "read:filters", "write:filters")
	case strings.HasPrefix(path, "/api/v1/media"):
		return scopesByMethod(method, "read", "write:media")
	default:
		return nil
	}
}

func scopesByMethod(method, readScope, writeScope string) []string {
	if method == methodGET {
		return []string{readScope}
	}
	return []string{writeScope}
}

func isStatusesPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/statuses") || strings.Contains(path, "/statuses")
}

func isAccountsPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/accounts") || strings.Contains(path, "/accounts")
}
