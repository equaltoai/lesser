package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestExtractMediaIDFromURL_Blurhash tests the URL parsing for blurhash resolver
func TestExtractMediaIDFromURL_Blurhash(t *testing.T) {
	resolver := &attachmentResolver{
		&Resolver{
			Logger: zap.NewNop(),
		},
	}

	tests := []struct {
		name            string
		url             string
		expectedMediaID string
		expectError     bool
	}{
		{
			name:            "Valid CDN URL with UUID",
			url:             "https://cdn.example.com/media/testuser/123e4567-e89b-12d3-a456-426614174000/original.jpg",
			expectedMediaID: "123e4567-e89b-12d3-a456-426614174000",
			expectError:     false,
		},
		{
			name:            "Valid S3 URL with UUID",
			url:             "https://bucket.s3.amazonaws.com/media/testuser/456e7890-e89b-12d3-a456-426614174001/small.jpg",
			expectedMediaID: "456e7890-e89b-12d3-a456-426614174001",
			expectError:     false,
		},
		{
			name:            "Valid URL with different filename",
			url:             "https://cdn.example.com/media/username123/789e0123-e89b-12d3-a456-426614174002/medium.webp",
			expectedMediaID: "789e0123-e89b-12d3-a456-426614174002",
			expectError:     false,
		},
		{
			name:        "Invalid URL - missing media prefix",
			url:         "https://cdn.example.com/images/testuser/123e4567-e89b-12d3-a456-426614174000/original.jpg",
			expectError: true,
		},
		{
			name:        "Invalid URL - not enough path segments",
			url:         "https://cdn.example.com/media/testuser/",
			expectError: true,
		},
		{
			name:        "Empty URL",
			url:         "",
			expectError: true,
		},
		{
			name:        "Malformed URL",
			url:         "not-a-valid-url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaID, err := resolver.extractMediaIDFromURL(tt.url)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, mediaID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMediaID, mediaID)
			}
		})
	}
}
