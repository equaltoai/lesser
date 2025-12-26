package common

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	slugWhitespace = regexp.MustCompile(`\s+`)
	slugInvalid    = regexp.MustCompile(`[^a-z0-9-]+`)
	slugDashes     = regexp.MustCompile(`-+`)
)

// Slugify converts a human-friendly string into a lowercase, URL-safe slug.
func Slugify(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.ToLower(value)
	value = slugWhitespace.ReplaceAllString(value, "-")
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return r
		}
		return '-'
	}, value)

	value = slugInvalid.ReplaceAllString(value, "-")
	value = slugDashes.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")

	return value
}
