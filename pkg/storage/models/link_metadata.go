package models

import (
	"net/url"
	"strings"
	"time"
)

// LinkMetadata represents metadata about a link
type LinkMetadata struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // LINK#{url}
	SK string `dynamorm:"sk" json:"-"` // METADATA

	// Attributes from interface
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Domain      string `json:"domain"`

	// Additional metadata
	FetchedAt    time.Time  `json:"fetched_at"`
	LastAccessed time.Time  `json:"last_accessed"`
	AccessCount  int64      `json:"access_count"`
	ContentType  string     `json:"content_type,omitempty"`       // MIME type
	Language     string     `json:"language,omitempty"`           // Detected language
	Author       string     `json:"author,omitempty"`             // Article author if available
	PublishedAt  *time.Time `json:"published_at,omitempty"`       // Publication date if available
	Keywords     []string   `json:"keywords,omitempty"`           // SEO keywords
	TTL          int64      `json:"ttl,omitempty" dynamorm:"ttl"` // 30 days cache
}

// UpdateKeys updates the partition and sort keys
func (l *LinkMetadata) UpdateKeys() {
	l.PK = "LINK#" + l.URL
	l.SK = SKMetadata

	// Extract domain from URL if not set
	if l.Domain == "" && l.URL != "" {
		if u, err := url.Parse(l.URL); err == nil {
			l.Domain = strings.ToLower(u.Hostname())
		}
	}

	// Set TTL to 30 days from fetch time
	l.TTL = l.FetchedAt.AddDate(0, 0, 30).Unix()
}

// NewLinkMetadata creates a new link metadata entry
func NewLinkMetadata(linkURL string) *LinkMetadata {
	metadata := &LinkMetadata{
		URL:          linkURL,
		FetchedAt:    time.Now().UTC(),
		LastAccessed: time.Now().UTC(),
		AccessCount:  1,
		Keywords:     []string{},
	}
	metadata.UpdateKeys()
	return metadata
}

// GetLinkMetadataKey returns the key for retrieving link metadata
func GetLinkMetadataKey(linkURL string) (pk, sk string) {
	return "LINK#" + linkURL, SKMetadata
}

// RecordAccess updates access tracking information
func (l *LinkMetadata) RecordAccess() {
	l.LastAccessed = time.Now().UTC()
	l.AccessCount++
}

// IsStale checks if the metadata should be refreshed
func (l *LinkMetadata) IsStale(maxAge time.Duration) bool {
	return time.Since(l.FetchedAt) > maxAge
}

// HasImage returns true if the link has an associated image
func (l *LinkMetadata) HasImage() bool {
	return l.Image != ""
}

// GetDisplayDomain returns a cleaned domain name for display
func (l *LinkMetadata) GetDisplayDomain() string {
	domain := l.Domain

	// Remove www. prefix
	domain = strings.TrimPrefix(domain, "www.")

	// Remove common TLDs for cleaner display
	if strings.Count(domain, ".") > 1 {
		// Keep only the main domain for well-known sites
		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	return domain
}

// SetFromOpenGraph sets metadata from OpenGraph tags
func (l *LinkMetadata) SetFromOpenGraph(ogTitle, ogDescription, ogImage string) {
	if ogTitle != "" {
		l.Title = ogTitle
	}
	if ogDescription != "" {
		l.Description = ogDescription
	}
	if ogImage != "" {
		l.Image = ogImage
	}
}

// SetFromTwitterCard sets metadata from Twitter Card tags
func (l *LinkMetadata) SetFromTwitterCard(twitterTitle, twitterDescription, twitterImage string) {
	// Only override if OpenGraph didn't provide values
	if l.Title == "" && twitterTitle != "" {
		l.Title = twitterTitle
	}
	if l.Description == "" && twitterDescription != "" {
		l.Description = twitterDescription
	}
	if l.Image == "" && twitterImage != "" {
		l.Image = twitterImage
	}
}

// TruncateDescription ensures description doesn't exceed a reasonable length
func (l *LinkMetadata) TruncateDescription(maxLength int) {
	if len(l.Description) > maxLength {
		// Find last complete word before limit
		truncated := l.Description[:maxLength]
		lastSpace := strings.LastIndex(truncated, " ")
		if lastSpace > 0 {
			l.Description = truncated[:lastSpace] + "..."
		} else {
			l.Description = truncated + "..."
		}
	}
}

// SanitizeMetadata cleans up the metadata fields
func (l *LinkMetadata) SanitizeMetadata() {
	// Trim whitespace
	l.Title = strings.TrimSpace(l.Title)
	l.Description = strings.TrimSpace(l.Description)
	l.Author = strings.TrimSpace(l.Author)

	// Remove duplicate keywords
	seen := make(map[string]bool)
	uniqueKeywords := []string{}
	for _, keyword := range l.Keywords {
		k := strings.ToLower(strings.TrimSpace(keyword))
		if k != "" && !seen[k] {
			seen[k] = true
			uniqueKeywords = append(uniqueKeywords, k)
		}
	}
	l.Keywords = uniqueKeywords

	// Ensure description is reasonable length
	l.TruncateDescription(500)
}
