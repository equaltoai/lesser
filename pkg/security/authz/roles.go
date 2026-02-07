package authz

import "strings"

const (
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
	RoleUser      = "user"
)

func NormalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func IsAdmin(role string) bool {
	return NormalizeRole(role) == RoleAdmin
}

func IsModeratorOrAdmin(role string) bool {
	normalized := NormalizeRole(role)
	return normalized == RoleAdmin || normalized == RoleModerator
}
