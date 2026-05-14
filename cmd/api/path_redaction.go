package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const privateConversationLogHashPrefix = "sha256:"

// sanitizeLogPath normalizes private route path segments before generic
// request/access logs and derived metrics persist them. It intentionally keeps
// the route class visible while replacing the private conversation identifier
// with a stable short hash for correlation.
func sanitizeLogPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	path, _ := splitLogPathSuffix(trimmed)
	if path == "" {
		return trimmed
	}
	if sanitized, ok := sanitizeLesserSelfScopeMintConversationPath(path); ok {
		return sanitized
	}
	if sanitized, ok := sanitizeLesserSelfScopeMintConversationListPath(path); ok {
		return sanitized
	}
	return trimmed
}

func splitLogPathSuffix(raw string) (path string, suffix string) {
	for i, r := range raw {
		if r == '?' || r == '#' {
			return raw[:i], raw[i:]
		}
	}
	return raw, ""
}

func sanitizeLesserSelfScopeMintConversationPath(path string) (string, bool) {
	segments, leadingSlash := splitLogPathSegments(path)
	if len(segments) != 7 ||
		segments[0] != "api" ||
		segments[1] != "v1" ||
		segments[2] != "souls" ||
		segments[3] != "bound" ||
		segments[4] != "me" ||
		segments[5] != "mint-conversations" ||
		strings.TrimSpace(segments[6]) == "" {
		return "", false
	}

	segments[6] = "conversation-" + shortLogHash(segments[6])
	return joinLogPathSegments(segments, leadingSlash), true
}

func sanitizeLesserSelfScopeMintConversationListPath(path string) (string, bool) {
	segments, leadingSlash := splitLogPathSegments(path)
	if len(segments) != 6 ||
		segments[0] != "api" ||
		segments[1] != "v1" ||
		segments[2] != "souls" ||
		segments[3] != "bound" ||
		segments[4] != "me" ||
		segments[5] != "mint-conversations" {
		return "", false
	}

	return joinLogPathSegments(segments, leadingSlash), true
}

func splitLogPathSegments(path string) ([]string, bool) {
	leadingSlash := strings.HasPrefix(path, "/")
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil, leadingSlash
	}
	return strings.Split(trimmed, "/"), leadingSlash
}

func joinLogPathSegments(segments []string, leadingSlash bool) string {
	joined := strings.Join(segments, "/")
	if leadingSlash {
		return "/" + joined
	}
	return joined
}

func shortLogHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return privateConversationLogHashPrefix + hex.EncodeToString(sum[:])[:16]
}
