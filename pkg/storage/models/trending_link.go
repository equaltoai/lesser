package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TrendingLink represents a trending link
type TrendingLink struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // TRENDING#{date}
	SK string `dynamorm:"sk" json:"-"` // LINK#{score}#{linkID}

	// Attributes from interface
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // link, photo, video
	AuthorName  string    `json:"author_name"`
	Image       string    `json:"image"`
	ImageURL    string    `json:"image_url"` // Additional field for image URL
	ShareCount  int64     `json:"share_count"`
	UserID      string    `json:"user_id"`    // User who shared the link
	CreatedAt   time.Time `json:"created_at"` // When this share was recorded

	// Additional fields for trending
	Date          string  `json:"date"`                         // Date for trending (YYYY-MM-DD)
	LinkID        string  `json:"link_id"`                      // Unique ID for this trending entry
	TrendingScore float64 `json:"trending_score"`               // Calculated trending score
	Domain        string  `json:"domain"`                       // Extracted domain from URL
	Rank          int     `json:"rank"`                         // Position in trending list
	TTL           int64   `json:"ttl,omitempty" dynamorm:"ttl"` // 7 days retention
}

// UpdateKeys updates the partition and sort keys based on date and score
func (t *TrendingLink) UpdateKeys() {
	t.PK = fmt.Sprintf("TRENDING#%s", t.Date)
	// Format score with leading zeros for proper sorting (higher scores first)
	t.SK = fmt.Sprintf("LINK#%010.0f#%s", 10000000000-t.TrendingScore, t.LinkID)

	// Extract domain from URL
	if u, err := url.Parse(t.URL); err == nil {
		t.Domain = strings.ToLower(u.Hostname())
	}

	// Set TTL to 7 days from the trending date
	if t.Date != "" {
		if date, err := time.Parse(common.DateFormat, t.Date); err == nil {
			t.TTL = date.AddDate(0, 0, 7).Unix()
		}
	}
}

// NewTrendingLink creates a new trending link
func NewTrendingLink(date string, link *TrendingLink) *TrendingLink {
	trending := &TrendingLink{
		URL:         link.URL,
		Title:       link.Title,
		Description: link.Description,
		Type:        link.Type,
		AuthorName:  link.AuthorName,
		Image:       link.Image,
		ImageURL:    link.ImageURL,
		ShareCount:  link.ShareCount,
		UserID:      link.UserID,
		CreatedAt:   time.Now().UTC(),
		Date:        date,
		LinkID:      generateLinkID(link.URL),
	}

	// Default type if not specified
	if err := common.ValidateRequiredParam("trending.Type", trending.Type); err != nil {
		trending.Type = "link"
	}

	trending.CalculateTrendingScore()
	trending.UpdateKeys()
	return trending
}

// generateLinkID creates a unique ID for a link based on its URL
func generateLinkID(linkURL string) string {
	// Simple hash of URL for uniqueness
	h := 0
	for _, r := range linkURL {
		h = (h << 5) - h + int(r)
	}
	if h < 0 {
		h = -h
	}
	return fmt.Sprintf("%d", h)
}

// GetTrendingLinkKey returns the key for retrieving a specific trending link
func GetTrendingLinkKey(date, linkID string, score float64) (pk, sk string) {
	pk = fmt.Sprintf("TRENDING#%s", date)
	sk = fmt.Sprintf("LINK#%010.0f#%s", 10000000000-score, linkID)
	return
}

// GetTrendingLinksKeys returns keys for querying trending links for a date
func GetTrendingLinksKeys(date string) (pk, skPrefix string) {
	return fmt.Sprintf("TRENDING#%s", date), "LINK#"
}

// GetTrendingLinksByDomainKeys returns keys for querying trending links from a specific domain
func GetTrendingLinksByDomainKeys(date, _ string) (pk, skPrefix string) {
	// Note: This would require a GSI to efficiently query by domain
	// For now, return standard trending links keys
	return GetTrendingLinksKeys(date)
}

// CalculateTrendingScore calculates the trending score based on share count and recency
func (t *TrendingLink) CalculateTrendingScore() {
	// Base score is share count
	baseScore := float64(t.ShareCount)

	// Apply time decay - links lose 50% of their score every 12 hours
	hoursSinceCreated := time.Since(t.CreatedAt).Hours()
	decay := 1.0
	if hoursSinceCreated > 0 {
		halfLife := 12.0 // hours (links decay faster than statuses)
		decay = pow(0.5, hoursSinceCreated/halfLife)
	}

	// Boost certain types of content
	typeMultiplier := 1.0
	switch t.Type {
	case "video":
		typeMultiplier = 1.5 // Videos get a boost
	case "photo":
		typeMultiplier = 1.2 // Photos get a small boost
	}

	t.TrendingScore = baseScore * decay * typeMultiplier
}

// IsStillTrending checks if the link should still be considered trending
func (t *TrendingLink) IsStillTrending(minScore float64, maxAge time.Duration) bool {
	return t.TrendingScore >= minScore && time.Since(t.CreatedAt) <= maxAge
}

// FormatTrendingSummary returns a human-readable summary
func (t *TrendingLink) FormatTrendingSummary() string {
	return fmt.Sprintf("Rank #%d: %s (%s) - %d shares (score: %.0f)",
		t.Rank, t.Title, t.Domain, t.ShareCount, t.TrendingScore)
}

// GetDisplayTitle returns a title for display, using URL if title is empty
func (t *TrendingLink) GetDisplayTitle() string {
	if t.Title != "" {
		return t.Title
	}

	// Try to extract a meaningful title from URL
	if u, err := url.Parse(t.URL); err == nil {
		path := strings.TrimPrefix(u.Path, "/")
		if path != "" {
			// Convert path to title-like format
			parts := strings.Split(path, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				// Remove file extension
				if idx := strings.LastIndex(lastPart, "."); idx > 0 {
					lastPart = lastPart[:idx]
				}
				// Replace hyphens/underscores with spaces
				lastPart = strings.ReplaceAll(lastPart, "-", " ")
				lastPart = strings.ReplaceAll(lastPart, "_", " ")
				caser := cases.Title(language.English)
				return caser.String(lastPart)
			}
		}
		return u.Hostname()
	}

	return t.URL
}

// TableName returns the DynamoDB table backing TrendingLink.
func (TrendingLink) TableName() string {
	return MainTableName
}
