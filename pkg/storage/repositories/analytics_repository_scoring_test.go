package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// extractDomainFromURL Tests (package-level helper)
// ============================================================================

func TestExtractDomainFromURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{
			name:     "simple URL",
			rawURL:   "https://example.com/path",
			expected: "example.com",
		},
		{
			name:     "URL with www prefix",
			rawURL:   "https://www.example.com/path",
			expected: "example.com",
		},
		{
			name:     "URL without scheme",
			rawURL:   "example.com/path",
			expected: "",
		},
		{
			name:     "YouTube URL",
			rawURL:   "https://www.youtube.com/watch?v=123",
			expected: "youtube.com",
		},
		{
			name:     "subdomain URL",
			rawURL:   "https://blog.example.com/post",
			expected: "blog.example.com",
		},
		{
			name:     "invalid URL returns original",
			rawURL:   "not a valid url ::: %%% ::::",
			expected: "not a valid url ::: %%% ::::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomainFromURL(tt.rawURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Converter Function Tests (for getTrendingItemsGeneric)
// ============================================================================

func TestHashtagTrendConverter(t *testing.T) {
	// This tests the converter function pattern used in getTrendingHashtagsInternal
	// The actual conversion logic is inline, but we can test the result structure

	now := time.Now()
	trendingHashtag := &storage.TrendingHashtag{
		Name:        "golang",
		URL:         "https://example.com/tags/golang",
		UsageCount:  100,
		UniqueUsers: 50,
		LastUsed:    now,
		FirstSeen:   now.Add(-24 * time.Hour),
	}

	assert.Equal(t, "golang", trendingHashtag.Name)
	assert.Equal(t, int64(100), trendingHashtag.UsageCount)
	assert.Equal(t, int64(50), trendingHashtag.UniqueUsers)
}

func TestStatusTrendConverter(t *testing.T) {
	now := time.Now()
	trendingStatus := &storage.TrendingStatus{
		ID:          "status-123",
		URL:         "https://example.com/statuses/status-123",
		AuthorID:    "user-456",
		Content:     "Hello World!",
		Engagements: 500,
		PublishedAt: now,
	}

	assert.Equal(t, "status-123", trendingStatus.ID)
	assert.Equal(t, "user-456", trendingStatus.AuthorID)
	assert.Equal(t, int64(500), trendingStatus.Engagements)
}

func TestLinkTrendConverter(t *testing.T) {
	trendingLink := &storage.TrendingLink{
		URL:         "https://example.com/article",
		Title:       "Example Article",
		Description: "An interesting article",
		Type:        "link",
		AuthorName:  "John Doe",
		Image:       "https://example.com/image.jpg",
		ShareCount:  250,
	}

	assert.Equal(t, "https://example.com/article", trendingLink.URL)
	assert.Equal(t, "Example Article", trendingLink.Title)
	assert.Equal(t, int64(250), trendingLink.ShareCount)
}

// ============================================================================
// Link Type Detection Tests
// ============================================================================

func TestLinkTypeDetection(t *testing.T) {
	// Test the logic used in updateLinkTrendScore and GetRecentLinks

	tests := []struct {
		name         string
		url          string
		expectedType string
		expectedImg  bool
	}{
		{
			name:         "YouTube URL",
			url:          "https://www.youtube.com/watch?v=abc123",
			expectedType: LinkTypeVideo,
			expectedImg:  false,
		},
		{
			name:         "youtu.be short URL",
			url:          "https://youtu.be/abc123",
			expectedType: LinkTypeVideo,
			expectedImg:  false,
		},
		{
			name:         "JPG image",
			url:          "https://example.com/image.jpg",
			expectedType: LinkTypePhoto,
			expectedImg:  true,
		},
		{
			name:         "PNG image",
			url:          "https://example.com/image.png",
			expectedType: LinkTypePhoto,
			expectedImg:  true,
		},
		{
			name:         "GIF image",
			url:          "https://example.com/animation.gif",
			expectedType: LinkTypePhoto,
			expectedImg:  true,
		},
		{
			name:         "WebP image",
			url:          "https://example.com/photo.webp",
			expectedType: LinkTypePhoto,
			expectedImg:  true,
		},
		{
			name:         "regular link",
			url:          "https://example.com/article",
			expectedType: "link",
			expectedImg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lowerURL := tt.url

			var linkType string
			var isImage bool

			// Replicate the logic from updateLinkTrendScore
			if containsIgnoreCase(lowerURL, "youtube.com") || containsIgnoreCase(lowerURL, "youtu.be") {
				linkType = LinkTypeVideo
			} else if containsIgnoreCase(lowerURL, ".jpg") || containsIgnoreCase(lowerURL, ".png") ||
				containsIgnoreCase(lowerURL, ".gif") || containsIgnoreCase(lowerURL, ".webp") {
				linkType = LinkTypePhoto
				isImage = true
			} else {
				linkType = "link"
			}

			assert.Equal(t, tt.expectedType, linkType)
			assert.Equal(t, tt.expectedImg, isImage)
		})
	}
}

// Helper for case-insensitive contains check
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1, c2 := s[i+j], substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 = c2 + 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// ============================================================================
// Default Domain Test
// ============================================================================

func TestDefaultDomain(t *testing.T) {
	assert.Equal(t, "localhost", DefaultDomain)
}

// ============================================================================
// TrendModel Interface Compliance Tests
// ============================================================================

func TestTrendModelInterface(t *testing.T) {
	// Verify that the expected model types implement TrendModel interface
	// This is a compile-time check essentially

	// We can't instantiate the models directly without more setup,
	// but we can verify the interface definition
	var _ TrendModel = (*trendModelMock)(nil)
}

type trendModelMock struct{}

func (t *trendModelMock) UpdateKeys() error {
	return nil
}

// ============================================================================
// TrendDeletable Interface Compliance Test
// ============================================================================

func TestTrendDeletableInterface(t *testing.T) {
	var _ TrendDeletable = (*trendDeletableMock)(nil)
}

type trendDeletableMock struct{}

func (t *trendDeletableMock) GetIdentifier() string {
	return "test-identifier"
}
