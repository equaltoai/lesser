package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
)

func TestHandler_extractUsernameFromURL(t *testing.T) {
	// Create a handler instance for testing
	handler := &Handler{}

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Full ActivityPub URL",
			url:      "https://example.com/users/alice",
			expected: "@alice",
		},
		{
			name:     "URL with path",
			url:      "https://example.com/users/bob/profile",
			expected: "@profile",
		},
		{
			name:     "just username",
			url:      "charlie",
			expected: "@charlie",
		},
		{
			name:     "empty url",
			url:      "",
			expected: "@",
		},
		{
			name:     "URL with trailing slash",
			url:      "https://example.com/users/dave/",
			expected: "@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractUsernameFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandler_generateObjectHTML(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		obj      map[string]any
		contains []string
	}{
		{
			name: "Note object with basic content",
			obj: map[string]any{
				"id":           "https://example.com/notes/123",
				"type":         "Note",
				"content":      "Hello World!",
				"attributedTo": "https://example.com/users/alice",
				"published":    "2024-01-01T00:00:00Z",
			},
			contains: []string{
				"Hello World!",
				"@alice",
				"Note",
			},
		},
		{
			name: "Article object with HTML content",
			obj: map[string]any{
				"id":           "https://example.com/articles/456",
				"type":         "Article",
				"content":      "<p>This is an <strong>article</strong></p>",
				"attributedTo": "https://example.com/users/bob",
				"published":    "2024-01-01T00:00:00Z",
			},
			contains: []string{
				"article",
				"@bob",
				"Article",
			},
		},
		{
			name: "Object with missing actor",
			obj: map[string]any{
				"id":        "https://example.com/notes/789",
				"type":      "Note",
				"content":   "Content without actor",
				"published": "2024-01-01T00:00:00Z",
			},
			contains: []string{
				"Content without actor",
				"Note",
			},
		},
		{
			name: "Object with attachments",
			obj: map[string]any{
				"id":           "https://example.com/notes/999",
				"type":         "Note",
				"content":      "Note with attachment",
				"attributedTo": "https://example.com/users/charlie",
				"published":    "2024-01-01T00:00:00Z",
				"attachment": []any{
					map[string]any{
						"type":      "Image",
						"url":       "https://example.com/image.jpg",
						"mediaType": "image/jpeg",
					},
				},
			},
			contains: []string{
				"Note with attachment",
				"@charlie",
				"Image Attachment",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.generateObjectHTML(tt.obj)

			// Check that all expected strings are contained in the result
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected, "Expected HTML to contain: %s", expected)
			}

			// Ensure it's valid HTML structure
			assert.Contains(t, result, "<!DOCTYPE html>")
			assert.Contains(t, result, "<html")
			assert.Contains(t, result, "</html>")
		})
	}
}

func TestHandler_extractObjectData(t *testing.T) {
	handler := &Handler{}

	t.Run("Extract from ActivityPub Note", func(t *testing.T) {
		// Test with the actual fields that exist in activitypub.Note
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				Context:   "https://www.w3.org/ns/activitystreams",
				ID:        "https://example.com/notes/1",
				Type:      "Note",
				InReplyTo: "",
			},
			Content:      "Test note content",
			AttributedTo: "https://example.com/users/alice",
		}

		data := handler.extractObjectData(note)

		assert.Equal(t, "Note", data.objectType)
		assert.Equal(t, "Test note content", data.content)
		assert.Equal(t, "@alice", data.attributedTo)
		assert.Equal(t, "https://example.com/notes/1", data.id)
		// Sensitive field doesn't exist in the Note struct, so we can't test it
	})

	t.Run("Extract from map object", func(t *testing.T) {
		objMap := map[string]any{
			"id":           "https://example.com/articles/2",
			"type":         "Article",
			"content":      "Article content",
			"name":         "Article Title",
			"attributedTo": "https://example.com/users/bob",
			"published":    "2024-01-02T00:00:00Z",
			"summary":      "Article summary",
			"sensitive":    true,
		}

		data := handler.extractObjectData(objMap)

		assert.Equal(t, "Article", data.objectType)
		assert.Equal(t, "Article content", data.content)
		assert.Equal(t, "Article Title", data.name)
		assert.Equal(t, "@bob", data.attributedTo)
		assert.Equal(t, "https://example.com/articles/2", data.id)
		assert.Equal(t, "Article summary", data.summary)
		assert.True(t, data.sensitive)
	})

	t.Run("Extract from unknown object type", func(t *testing.T) {
		unknownObj := struct{ Unknown string }{"test"}

		data := handler.extractObjectData(unknownObj)

		assert.Equal(t, "Object", data.objectType)
		assert.Equal(t, "Unknown object type", data.content)
		assert.Equal(t, "unknown", data.id)
	})
}
