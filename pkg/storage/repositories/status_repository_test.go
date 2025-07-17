package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStatusRepository(t *testing.T) {
	repo := NewStatusRepository(nil)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
}

func TestExtractStatusIDFromURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Standard ActivityPub URL",
			url:      "https://example.com/users/alice/statuses/123",
			expected: "123",
		},
		{
			name:     "Simple ID",
			url:      "123",
			expected: "123",
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractStatusIDFromURL(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractStatusIDFromURL_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with query params",
			url:      "https://example.com/statuses/456?param=value",
			expected: "456?param=value",
		},
		{
			name:     "URL ending with slash",
			url:      "https://example.com/statuses/789/",
			expected: "",
		},
		{
			name:     "Single slash",
			url:      "/",
			expected: "",
		},
		{
			name:     "Multiple slashes",
			url:      "//",
			expected: "",
		},
		{
			name:     "Path with multiple segments",
			url:      "first/second/third/last",
			expected: "last",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractStatusIDFromURL(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}
