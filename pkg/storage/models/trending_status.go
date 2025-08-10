package models

import (
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"time"
)

// TrendingStatus represents a trending status/post
type TrendingStatus struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // TRENDING#{date}
	SK string `dynamorm:"sk" json:"-"` // STATUS#{score}#{statusID}

	// Attributes from interface
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	AuthorID    string    `json:"author_id"`
	Content     string    `json:"content"`
	Engagements int64     `json:"engagements"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"` // When this trend record was created
	Likes       int       `json:"likes"`      // Number of likes
	Boosts      int       `json:"boosts"`     // Number of boosts
	Replies     int       `json:"replies"`    // Number of replies

	// Additional fields for trending
	Date          string  `json:"date"`                         // Date for trending (YYYY-MM-DD)
	TrendingScore float64 `json:"trending_score"`               // Calculated trending score
	Rank          int     `json:"rank"`                         // Position in trending list
	TTL           int64   `json:"ttl,omitempty" dynamorm:"ttl"` // 7 days retention
}

// UpdateKeys updates the partition and sort keys based on date and score
func (t *TrendingStatus) UpdateKeys() {
	t.PK = fmt.Sprintf("TRENDING#%s", t.Date)
	// Format score with leading zeros for proper sorting (higher scores first)
	t.SK = fmt.Sprintf("STATUS#%010.0f#%s", 10000000000-t.TrendingScore, t.ID)

	// Set TTL to 7 days from the trending date
	if t.Date != "" {
		if date, err := time.Parse(common.DateFormat, t.Date); err == nil {
			t.TTL = date.AddDate(0, 0, 7).Unix()
		}
	}
}

// NewTrendingStatus creates a new trending status
func NewTrendingStatus(date string, status *TrendingStatus) *TrendingStatus {
	trending := &TrendingStatus{
		ID:          status.ID,
		URL:         status.URL,
		AuthorID:    status.AuthorID,
		Content:     status.Content,
		Engagements: status.Engagements,
		PublishedAt: status.PublishedAt,
		CreatedAt:   time.Now().UTC(),
		Likes:       status.Likes,
		Boosts:      status.Boosts,
		Replies:     status.Replies,
		Date:        date,
	}
	trending.CalculateTrendingScore()
	trending.UpdateKeys()
	return trending
}

// GetTrendingStatusKey returns the key for retrieving a specific trending status
func GetTrendingStatusKey(date, statusID string, score float64) (pk, sk string) {
	pk = fmt.Sprintf("TRENDING#%s", date)
	sk = fmt.Sprintf("STATUS#%010.0f#%s", 10000000000-score, statusID)
	return
}

// GetTrendingStatusesKeys returns keys for querying trending statuses for a date
func GetTrendingStatusesKeys(date string) (pk, skPrefix string) {
	return fmt.Sprintf("TRENDING#%s", date), "STATUS#"
}

// GetTrendingStatusRangeKeys returns keys for querying top N trending statuses
func GetTrendingStatusRangeKeys(date string, minScore float64) (pk, skStart, skEnd string) {
	pk = fmt.Sprintf("TRENDING#%s", date)
	skStart = "STATUS#"
	skEnd = fmt.Sprintf("STATUS#%010.0f", 10000000000-minScore)
	return
}

// CalculateTrendingScore calculates the trending score based on engagement metrics
func (t *TrendingStatus) CalculateTrendingScore() {
	// Weight different types of engagement
	likeWeight := 1.0
	boostWeight := 2.0
	replyWeight := 3.0

	// Calculate base score
	baseScore := float64(t.Likes)*likeWeight +
		float64(t.Boosts)*boostWeight +
		float64(t.Replies)*replyWeight

	// Apply time decay - content loses 50% of its score every 24 hours
	hoursSincePublished := time.Since(t.PublishedAt).Hours()
	decay := 1.0
	if hoursSincePublished > 0 {
		halfLife := 24.0 // hours
		decay = pow(0.5, hoursSincePublished/halfLife)
	}

	t.TrendingScore = baseScore * decay
	t.Engagements = int64(t.Likes + t.Boosts + t.Replies)
}

// pow is a simple power function for float64
func pow(base, exp float64) float64 {
	if exp == 0 {
		return 1
	}
	result := base
	for i := 1; i < int(exp); i++ {
		result *= base
	}
	return result
}

// IsStillTrending checks if the status should still be considered trending
func (t *TrendingStatus) IsStillTrending(minScore float64, maxAge time.Duration) bool {
	return t.TrendingScore >= minScore && time.Since(t.PublishedAt) <= maxAge
}

// FormatTrendingSummary returns a human-readable summary
func (t *TrendingStatus) FormatTrendingSummary() string {
	return fmt.Sprintf("Rank #%d: %d likes, %d boosts, %d replies (score: %.0f)",
		t.Rank, t.Likes, t.Boosts, t.Replies, t.TrendingScore)
}
