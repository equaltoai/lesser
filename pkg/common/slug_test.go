package common

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic string",
			input:    "Hello World",
			expected: "hello-world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "numbers",
			input:    "Item 123",
			expected: "item-123",
		},
		{
			name:     "special characters",
			input:    "Hello @ World!",
			expected: "hello-world",
		},
		{
			name:     "multiple spaces",
			input:    "Hello    World",
			expected: "hello-world",
		},
		{
			name:     "existing dashes",
			input:    "Hello-World",
			expected: "hello-world",
		},
		{
			name:     "multiple dashes",
			input:    "Hello--World",
			expected: "hello-world",
		},
		{
			name:     "mixed separators",
			input:    "Hello_World",
			expected: "hello-world",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  Hello World  ",
			expected: "hello-world",
		},
		{
			name:     "leading and trailing dashes",
			input:    "-Hello World-",
			expected: "hello-world",
		},
		{
			name:     "unicode characters handled as non-ascii",
			input:    "Héllo Wörld",
			expected: "h-llo-w-rld", // Based on current implementation logic
		},
		{
			name:     "URL characters",
			input:    "https://example.com/foo",
			expected: "https-example-com-foo",
		},
		{
			name:     "mixed case",
			input:    "HeLLo WoRLd",
			expected: "hello-world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Slugify(tt.input)
			if result != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
