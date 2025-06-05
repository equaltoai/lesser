package mastodon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractHashtags(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "simple hashtag",
			content:  "Hello #world",
			expected: []string{"world"},
		},
		{
			name:     "multiple hashtags",
			content:  "This is a #test with #multiple #hashtags",
			expected: []string{"test", "multiple", "hashtags"},
		},
		{
			name:     "hashtags with HTML",
			content:  "<p>Check out #golang and #activitypub!</p>",
			expected: []string{"golang", "activitypub"},
		},
		{
			name:     "case insensitive deduplication",
			content:  "#GoLang is great, I love #golang",
			expected: []string{"golang"},
		},
		{
			name:     "hashtag at start of line",
			content:  "#start of the line",
			expected: []string{"start"},
		},
		{
			name:     "hashtag at end of line",
			content:  "End with a #hashtag",
			expected: []string{"hashtag"},
		},
		{
			name:     "unicode hashtags",
			content:  "Unicode #테스트 and #日本語",
			expected: []string{"테스트", "日本語"},
		},
		{
			name:     "hashtag with underscore",
			content:  "Underscore #test_tag works",
			expected: []string{"test_tag"},
		},
		{
			name:     "no hashtags",
			content:  "This has no hashtags",
			expected: []string{},
		},
		{
			name:     "number only hashtag ignored",
			content:  "Number #123 should be ignored",
			expected: []string{},
		},
		{
			name:     "hashtag with numbers",
			content:  "Valid #test123 hashtag",
			expected: []string{"test123"},
		},
		{
			name:     "hashtag in URL should not match",
			content:  "URL: https://example.com/#anchor should not match",
			expected: []string{},
		},
		{
			name:     "HTML entities",
			content:  "HTML entities &amp; #test &lt;tag&gt;",
			expected: []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractHashtags(tt.content)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestExtractHashtagsWithCase(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "preserves case",
			content:  "Hello #World and #GoLang",
			expected: []string{"World", "GoLang"},
		},
		{
			name:     "first occurrence wins",
			content:  "#GoLang is great, I love #golang too",
			expected: []string{"GoLang"},
		},
		{
			name:     "mixed case hashtags",
			content:  "#CamelCase #UPPERCASE #lowercase",
			expected: []string{"CamelCase", "UPPERCASE", "lowercase"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractHashtagsWithCase(tt.content)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestNormalizeHashtag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase conversion",
			input:    "GoLang",
			expected: "golang",
		},
		{
			name:     "removes hash prefix",
			input:    "#test",
			expected: "test",
		},
		{
			name:     "already normalized",
			input:    "test",
			expected: "test",
		},
		{
			name:     "unicode normalization",
			input:    "#테스트",
			expected: "테스트",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeHashtag(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAllNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "all numbers",
			input:    "123",
			expected: true,
		},
		{
			name:     "mixed content",
			input:    "test123",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single digit",
			input:    "0",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAllNumbers(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
