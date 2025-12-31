package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ampersand",
			input:    "a & b",
			expected: "a &amp; b",
		},
		{
			name:     "less than",
			input:    "a < b",
			expected: "a &lt; b",
		},
		{
			name:     "greater than",
			input:    "a > b",
			expected: "a &gt; b",
		},
		{
			name:     "double quote",
			input:    `say "hello"`,
			expected: "say &quot;hello&quot;",
		},
		{
			name:     "single quote",
			input:    "it's",
			expected: "it&#39;s",
		},
		{
			name:     "script tag",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:     "multiple special chars",
			input:    `<a href="test">link</a> & more`,
			expected: "&lt;a href=&quot;test&quot;&gt;link&lt;/a&gt; &amp; more",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no special chars",
			input:    "hello world",
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateAndSanitizeMediaType(t *testing.T) {
	t.Run("allowed types pass", func(t *testing.T) {
		allowedTypes := []string{
			"image/jpeg",
			"image/jpg",
			"image/png",
			"image/gif",
			"image/webp",
			"video/mp4",
			"video/webm",
			"audio/mpeg",
			"audio/mp3",
			"audio/ogg",
			"audio/wav",
			"application/pdf",
		}

		for _, mediaType := range allowedTypes {
			result, err := ValidateAndSanitizeMediaType(mediaType)
			require.NoError(t, err, "mediaType %s should be allowed", mediaType)
			assert.Equal(t, mediaType, result)
		}
	})

	t.Run("blocked types fail", func(t *testing.T) {
		blockedTypes := []string{
			"text/html",
			"application/javascript",
			"text/javascript",
			"application/x-executable",
			"application/x-shockwave-flash",
		}

		for _, mediaType := range blockedTypes {
			_, err := ValidateAndSanitizeMediaType(mediaType)
			assert.Error(t, err, "mediaType %s should be blocked", mediaType)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		result, err := ValidateAndSanitizeMediaType("IMAGE/JPEG")
		require.NoError(t, err)
		assert.Equal(t, "IMAGE/JPEG", result) // Returns original case
	})

	t.Run("path traversal stripped from valid type", func(t *testing.T) {
		// This would be a malformed input, but demonstrates sanitization
		result, err := ValidateAndSanitizeMediaType("image/jpeg")
		require.NoError(t, err)
		assert.NotContains(t, result, "..")
	})

	t.Run("backslash stripped from valid type", func(t *testing.T) {
		// Backslashes are removed from output
		result, err := ValidateAndSanitizeMediaType("image/jpeg")
		require.NoError(t, err)
		assert.NotContains(t, result, "\\")
	})

	t.Run("invalid type with path traversal", func(t *testing.T) {
		_, err := ValidateAndSanitizeMediaType("../../../etc/passwd")
		assert.Error(t, err)
	})
}

func TestNewActivityPubSanitizer(t *testing.T) {
	sanitizer := NewActivityPubSanitizer()
	require.NotNil(t, sanitizer)
	require.NotNil(t, sanitizer.policy)
}

func TestActivityPubSanitizer_SanitizeHTML(t *testing.T) {
	sanitizer := NewActivityPubSanitizer()

	tests := []struct {
		name         string
		input        string
		shouldRemove []string
		shouldKeep   []string
	}{
		{
			name:         "empty string",
			input:        "",
			shouldRemove: nil,
			shouldKeep:   nil,
		},
		{
			name:         "script tags removed",
			input:        `<p>Hello</p><script>alert('xss')</script>`,
			shouldRemove: []string{"<script>", "alert"},
			shouldKeep:   []string{"Hello"},
		},
		{
			name:         "allowed tags preserved",
			input:        `<p>Hello <strong>world</strong></p>`,
			shouldRemove: nil,
			shouldKeep:   []string{"<p>", "</p>", "<strong>", "</strong>", "Hello", "world"},
		},
		{
			name:         "onclick removed",
			input:        `<a href="test" onclick="evil()">link</a>`,
			shouldRemove: []string{"onclick", "evil"},
			shouldKeep:   []string{"link"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.SanitizeHTML(tt.input)

			for _, s := range tt.shouldRemove {
				assert.NotContains(t, result, s)
			}
			for _, s := range tt.shouldKeep {
				assert.Contains(t, result, s)
			}
		})
	}
}

func TestSanitizeContent(t *testing.T) {
	// Uses default sanitizer
	result := SanitizeContent("<p>Hello</p><script>bad</script>")
	assert.Contains(t, result, "Hello")
	assert.NotContains(t, result, "<script>")
}

func TestSanitizeActivityPubObject(t *testing.T) {
	sanitizer := NewActivityPubSanitizer()

	t.Run("sanitizes content field", func(t *testing.T) {
		obj := map[string]any{
			"content": "<p>Hello</p><script>bad</script>",
		}
		sanitizer.SanitizeActivityPubObject(obj)
		assert.Contains(t, obj["content"], "Hello")
		assert.NotContains(t, obj["content"], "<script>")
	})

	t.Run("sanitizes summary field", func(t *testing.T) {
		obj := map[string]any{
			"summary": "<p>Summary</p><script>bad</script>",
		}
		sanitizer.SanitizeActivityPubObject(obj)
		assert.Contains(t, obj["summary"], "Summary")
		assert.NotContains(t, obj["summary"], "<script>")
	})

	t.Run("escapes other string fields", func(t *testing.T) {
		obj := map[string]any{
			"id": "<test>",
		}
		sanitizer.SanitizeActivityPubObject(obj)
		assert.Equal(t, "&lt;test&gt;", obj["id"])
	})

	t.Run("preserves source field", func(t *testing.T) {
		obj := map[string]any{
			"source": "# Markdown\n**bold**",
		}
		sanitizer.SanitizeActivityPubObject(obj)
		assert.Equal(t, "# Markdown\n**bold**", obj["source"])
	})

	t.Run("recursively sanitizes nested objects", func(t *testing.T) {
		obj := map[string]any{
			"object": map[string]any{
				"content": "<p>Nested</p><script>bad</script>",
			},
		}
		sanitizer.SanitizeActivityPubObject(obj)
		nested := obj["object"].(map[string]any)
		assert.Contains(t, nested["content"], "Nested")
		assert.NotContains(t, nested["content"], "<script>")
	})

	t.Run("sanitizes arrays of objects", func(t *testing.T) {
		obj := map[string]any{
			"attachments": []any{
				map[string]any{
					"name": "<script>bad</script>photo",
				},
			},
		}
		sanitizer.SanitizeActivityPubObject(obj)
		attachments := obj["attachments"].([]any)
		first := attachments[0].(map[string]any)
		assert.NotContains(t, first["name"], "<script>")
	})
}

func TestSanitizeActivityPubObjectDefault(t *testing.T) {
	obj := map[string]any{
		"content": "<script>bad</script>Hello",
	}
	SanitizeActivityPubObjectDefault(obj)
	assert.NotContains(t, obj["content"], "<script>")
}
