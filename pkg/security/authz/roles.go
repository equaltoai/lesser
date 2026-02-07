// Package authz provides role-based authorization helpers.
package authz

import "strings"

// Roles define normalized role names used for authorization checks.
const (
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
	RoleUser      = "user"
)

// NormalizeRole normalizes role strings for comparisons.
func NormalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

// IsAdmin reports whether role is an administrator role.
func IsAdmin(role string) bool {
	return NormalizeRole(role) == RoleAdmin
}

// IsModeratorOrAdmin reports whether role is a moderator or administrator role.
func IsModeratorOrAdmin(role string) bool {
	normalized := NormalizeRole(role)
	return normalized == RoleAdmin || normalized == RoleModerator
}
