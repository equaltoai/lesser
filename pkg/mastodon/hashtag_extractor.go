package mastodon

import (
	"html"
	"regexp"
	"strings"
)

// HashtagRegex matches hashtags in text
// Supports Unicode characters, numbers, and underscores
// Uses spaces as padding to ensure proper boundary matching
var HashtagRegex = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])#([\p{L}\p{N}_]+)`)

// URLRegex matches URLs to remove them before hashtag extraction
var URLRegex = regexp.MustCompile(`https?://[^\s<]+`)

// ExtractHashtags extracts hashtags from HTML or plain text content
// Returns a slice of unique hashtag names (without the # prefix)
func ExtractHashtags(content string) []string {
	// First, unescape HTML entities
	unescaped := html.UnescapeString(content)

	// Strip HTML tags to get plain text
	// Simple regex for basic HTML tag removal
	htmlTagRegex := regexp.MustCompile(`<[^>]+>`)
	plainText := htmlTagRegex.ReplaceAllString(unescaped, " ")

	// Remove URLs to avoid matching hashtags within them
	plainText = URLRegex.ReplaceAllString(plainText, " ")

	// Add space padding to ensure proper boundary detection
	plainText = " " + plainText + " "

	// Find all hashtag matches
	matches := HashtagRegex.FindAllStringSubmatch(plainText, -1)

	// Use a map to ensure uniqueness
	tagMap := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			// Convert to lowercase for consistency
			tag := strings.ToLower(match[1])
			// Skip if it's just numbers
			if !isAllNumbers(tag) {
				tagMap[tag] = true
			}
		}
	}

	// Convert map to slice
	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}

	return tags
}

// ExtractHashtagsWithCase extracts hashtags preserving their original case
// Useful for display purposes
func ExtractHashtagsWithCase(content string) []string {
	// First, unescape HTML entities
	unescaped := html.UnescapeString(content)

	// Strip HTML tags to get plain text
	htmlTagRegex := regexp.MustCompile(`<[^>]+>`)
	plainText := htmlTagRegex.ReplaceAllString(unescaped, " ")

	// Remove URLs to avoid matching hashtags within them
	plainText = URLRegex.ReplaceAllString(plainText, " ")

	// Add space padding to ensure proper boundary detection
	plainText = " " + plainText + " "

	// Find all hashtag matches
	matches := HashtagRegex.FindAllStringSubmatch(plainText, -1)

	// Use a map to ensure uniqueness (case-insensitive key, case-sensitive value)
	tagMap := make(map[string]string)
	for _, match := range matches {
		if len(match) > 1 {
			tag := match[1]
			lowerTag := strings.ToLower(tag)
			// Skip if it's just numbers
			if !isAllNumbers(tag) {
				// Keep the first occurrence's case
				if _, exists := tagMap[lowerTag]; !exists {
					tagMap[lowerTag] = tag
				}
			}
		}
	}

	// Convert map to slice
	tags := make([]string, 0, len(tagMap))
	for _, tag := range tagMap {
		tags = append(tags, tag)
	}

	return tags
}

// isAllNumbers checks if a string contains only numbers
func isAllNumbers(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// NormalizeHashtag normalizes a hashtag for storage/lookup
// Converts to lowercase and removes the # prefix if present
func NormalizeHashtag(tag string) string {
	tag = strings.TrimPrefix(tag, "#")
	return strings.ToLower(tag)
}
