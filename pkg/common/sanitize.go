package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// ActivityPubSanitizer provides HTML sanitization for ActivityPub content
type ActivityPubSanitizer struct {
	policy *bluemonday.Policy
}

// Global sanitizer instance
var defaultSanitizer *ActivityPubSanitizer

func init() {
	defaultSanitizer = NewActivityPubSanitizer()
}

// NewActivityPubSanitizer creates a new sanitizer configured for ActivityPub content
func NewActivityPubSanitizer() *ActivityPubSanitizer {
	policy := bluemonday.NewPolicy()

	// Allow basic formatting tags
	policy.AllowElements("p", "br", "span", "del", "pre", "code")
	policy.AllowElements("em", "strong", "b", "i", "u", "s", "strike")
	policy.AllowElements("blockquote", "ul", "ol", "li")

	// Allow links with specific attributes
	policy.AllowAttrs("href", "title", "rel").OnElements("a")
	policy.AllowAttrs("class").OnElements("span", "a")

	// Require that links have rel="nofollow noopener noreferrer" for security
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	// Allow mentions and hashtags classes (Mastodon-style)
	policy.AllowAttrs("class").Matching(regexp.MustCompile(`^(h-card|mention|hashtag|invisible|ellipsis)$`)).OnElements("span", "a")

	// Allow data attributes for mentions
	policy.AllowAttrs("data-user").OnElements("span", "a")

	// Prevent any JavaScript
	policy.RequireParseableURLs(true)

	return &ActivityPubSanitizer{
		policy: policy,
	}
}

// SanitizeHTML sanitizes HTML content for safe display
func (s *ActivityPubSanitizer) SanitizeHTML(content string) string {
	if content == "" {
		return ""
	}

	// First pass: sanitize with bluemonday
	sanitized := s.policy.Sanitize(content)

	// Second pass: ensure mentions and hashtags are preserved
	sanitized = s.preserveMentionsAndHashtags(sanitized)

	return sanitized
}

// SanitizeActivityPubObject sanitizes all string fields in an ActivityPub object
func (s *ActivityPubSanitizer) SanitizeActivityPubObject(obj map[string]any) {
	for key, value := range obj {
		switch v := value.(type) {
		case string:
			// Sanitize HTML content fields
			if key == "content" || key == "summary" || key == "name" {
				obj[key] = s.SanitizeHTML(v)
			} else if key == "source" {
				// Don't sanitize source content (markdown/plaintext)
				continue
			} else {
				// For other string fields, escape HTML
				obj[key] = EscapeHTML(v)
			}
		case map[string]any:
			// Recursively sanitize nested objects
			s.SanitizeActivityPubObject(v)
		case []any:
			// Sanitize arrays of objects
			for _, item := range v {
				if itemMap, ok := item.(map[string]any); ok {
					s.SanitizeActivityPubObject(itemMap)
				}
			}
		}
	}
}

// preserveMentionsAndHashtags ensures mentions and hashtags are properly formatted
func (s *ActivityPubSanitizer) preserveMentionsAndHashtags(content string) string {
	// This is a simplified version - in production, you'd want to parse the HTML
	// and only process text nodes to avoid breaking existing HTML
	return content
}

// EscapeHTML escapes HTML special characters
func EscapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// SanitizeContent is a convenience function using the default sanitizer
func SanitizeContent(content string) string {
	return defaultSanitizer.SanitizeHTML(content)
}

// SanitizeActivityPubObjectDefault sanitizes an object using the default sanitizer
func SanitizeActivityPubObjectDefault(obj map[string]any) {
	defaultSanitizer.SanitizeActivityPubObject(obj)
}

// ValidateAndSanitizeMediaType validates and sanitizes media MIME types
func ValidateAndSanitizeMediaType(mediaType string) (string, error) {
	// Whitelist of allowed media types
	allowedTypes := map[string]bool{
		"image/jpeg":      true,
		"image/jpg":       true,
		"image/png":       true,
		"image/gif":       true,
		"image/webp":      true,
		"video/mp4":       true,
		"video/webm":      true,
		"audio/mpeg":      true,
		"audio/mp3":       true,
		"audio/ogg":       true,
		"audio/wav":       true,
		"application/pdf": true,
	}

	// Normalize to lowercase for comparison
	normalizedType := strings.ToLower(mediaType)

	// Check against whitelist first (before any sanitization)
	if !allowedTypes[normalizedType] {
		return "", ValidationError{Field: "mediaType", Message: "unsupported media type"}
	}

	// Remove any path traversal attempts from the original input
	sanitized := strings.ReplaceAll(mediaType, "..", "")
	sanitized = strings.ReplaceAll(sanitized, "\\", "")

	return sanitized, nil
}
