// Package common provides shared utilities for key operations.
package common //nolint:revive // Standard utility package name

import "strings"

// SplitKey splits a composite key into its parts
// For example, "user#123" becomes ["user", "123"]
func SplitKey(key string) []string {
	return strings.Split(key, "#")
}

// JoinKey joins parts into a composite key
// For example, JoinKey("user", "123") becomes "user#123"
func JoinKey(parts ...string) string {
	return strings.Join(parts, "#")
}
