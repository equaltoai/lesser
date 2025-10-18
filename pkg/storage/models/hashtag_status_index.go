package models

import (
	"fmt"
	"strings"
	"time"
)

// HashtagStatusIndex represents an efficient index for hashtag-to-status mapping
// This enables fast hashtag timeline queries without scanning all statuses
type HashtagStatusIndex struct {
	// Primary key - hashtag timeline
	PK string `dynamorm:"pk" json:"pk"` // Format: "HASHTAG_TIMELINE#{hashtag_name}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "STATUS#{timestamp_desc}#{status_id}"

	// GSI1 - Status-to-hashtag reverse index for cleanup
	GSI1PK string `dynamorm:"index:status-hashtag-index,pk" json:"gsi1_pk"` // Format: "STATUS_HASHTAGS#{status_id}"
	GSI1SK string `dynamorm:"index:status-hashtag-index,sk" json:"gsi1_sk"` // Format: "HASHTAG#{hashtag_name}"

	// GSI2 - Timeline by visibility for filtering
	GSI2PK string `dynamorm:"index:hashtag-visibility-index,pk" json:"gsi2_pk"` // Format: "HASHTAG_VIS#{hashtag_name}#{visibility}"
	GSI2SK string `dynamorm:"index:hashtag-visibility-index,sk" json:"gsi2_sk"` // Format: "TIMELINE#{timestamp_desc}"

	// Status data
	StatusID     string    `json:"status_id"`
	AuthorID     string    `json:"author_id"`
	AuthorHandle string    `json:"author_handle"`
	StatusURL    string    `json:"status_url,omitempty"`
	Content      string    `json:"content,omitempty"`     // Excerpt for search results
	MediaCount   int       `json:"media_count,omitempty"` // Number of media attachments
	Language     string    `json:"language,omitempty"`    // Content language
	Visibility   string    `json:"visibility"`            // public, unlisted, private, direct
	Published    time.Time `json:"published"`             // When the status was published
	HashtagName  string    `json:"hashtag_name"`          // The hashtag (for reverse index)

	// TTL for automatic cleanup (90 days for efficient hashtag timelines)
	TTL int64 `dynamorm:"ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// UpdateKeys updates all the GSI keys when the index data changes
func (hsi *HashtagStatusIndex) UpdateKeys() error {
	tagLower := strings.ToLower(strings.TrimPrefix(hsi.HashtagName, "#"))

	// Create timestamp-descending format for SK (latest first)
	// Use unix timestamp with reversal for DESC ordering
	timestampDesc := fmt.Sprintf("%019d", (1<<63-1)-hsi.Published.Unix()) // Max int64 - timestamp for DESC order

	// Primary key for hashtag timeline
	hsi.PK = fmt.Sprintf("HASHTAG_TIMELINE#%s", tagLower)
	hsi.SK = fmt.Sprintf("STATUS#%s#%s", timestampDesc, hsi.StatusID)

	// GSI1 for reverse lookup (status -> hashtags)
	hsi.GSI1PK = fmt.Sprintf("STATUS_HASHTAGS#%s", hsi.StatusID)
	hsi.GSI1SK = fmt.Sprintf("HASHTAG#%s", tagLower)

	// GSI2 for visibility filtering
	hsi.GSI2PK = fmt.Sprintf("HASHTAG_VIS#%s#%s", tagLower, hsi.Visibility)
	hsi.GSI2SK = fmt.Sprintf("TIMELINE#%s", timestampDesc)

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (hsi *HashtagStatusIndex) GetPK() string {
	return hsi.PK
}

// GetSK returns the sort key for BaseModel interface
func (hsi *HashtagStatusIndex) GetSK() string {
	return hsi.SK
}

// HashtagTrendingData represents trending data for hashtags with time-windowed metrics
type HashtagTrendingData struct {
	// Primary key - trending hashtag data
	PK string `dynamorm:"pk" json:"pk"` // Format: "TRENDING_HASHTAG#{date}#{hour}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "HASHTAG#{score_padded}#{hashtag_name}"

	// GSI1 - Query by hashtag across time periods
	GSI1PK string `dynamorm:"index:hashtag-trending-history,pk" json:"gsi1_pk"` // Format: "HASHTAG_TREND#{hashtag_name}"
	GSI1SK string `dynamorm:"index:hashtag-trending-history,sk" json:"gsi1_sk"` // Format: "TIME#{timestamp}"

	// GSI2 - Query trending for time period
	GSI2PK string `dynamorm:"index:trending-by-period,pk" json:"gsi2_pk"` // Format: "TRENDING_PERIOD#{time_window}"
	GSI2SK string `dynamorm:"index:trending-by-period,sk" json:"gsi2_sk"` // Format: "SCORE#{score_padded}#{hashtag_name}"

	// Hashtag info
	HashtagName string    `json:"hashtag_name"`
	URL         string    `json:"url"`
	Period      time.Time `json:"period"`      // Start of the time period
	TimeWindow  string    `json:"time_window"` // "1h", "6h", "24h", "7d"

	// Trending metrics
	TrendScore     float64 `json:"trend_score"`     // Overall trending score
	UsageCount     int64   `json:"usage_count"`     // Usage count in period
	UniqueUsers    int64   `json:"unique_users"`    // Unique users in period
	Growth         float64 `json:"growth"`          // Growth rate vs previous period
	Velocity       float64 `json:"velocity"`        // Usage per hour
	MomentumScore  float64 `json:"momentum_score"`  // Acceleration indicator
	TrustScore     float64 `json:"trust_score"`     // Trust-weighted score
	EngagementRate float64 `json:"engagement_rate"` // Engagement per usage
	DiversityScore float64 `json:"diversity_score"` // User diversity score

	// Component scores for analysis
	ComponentScores map[string]float64 `json:"component_scores,omitempty"`

	// TTL for automatic cleanup (30 days)
	TTL int64 `dynamorm:"ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateKeys updates all the GSI keys when the trending data changes
func (htd *HashtagTrendingData) UpdateKeys() {
	tagLower := strings.ToLower(strings.TrimPrefix(htd.HashtagName, "#"))

	// Pad score for lexicographic sorting (10-digit precision with 3 decimal places)
	scorePadded := fmt.Sprintf("%010.3f", htd.TrendScore)

	// Primary key for trending data by time period
	dateHour := htd.Period.Format("2006-01-02-15")
	htd.PK = fmt.Sprintf("TRENDING_HASHTAG#%s", dateHour)
	htd.SK = fmt.Sprintf("HASHTAG#%s#%s", scorePadded, tagLower)

	// GSI1 for hashtag trending history
	htd.GSI1PK = fmt.Sprintf("HASHTAG_TREND#%s", tagLower)
	htd.GSI1SK = fmt.Sprintf("TIME#%d", htd.Period.Unix())

	// GSI2 for trending by time window
	htd.GSI2PK = fmt.Sprintf("TRENDING_PERIOD#%s", htd.TimeWindow)
	htd.GSI2SK = fmt.Sprintf("SCORE#%s#%s", scorePadded, tagLower)
}

// HashtagSearchCache represents a cache for hashtag search results to improve performance
type HashtagSearchCache struct {
	// Primary key - search cache
	PK string `dynamorm:"pk" json:"pk"` // Format: "HASHTAG_SEARCH_CACHE#{query_hash}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "CACHE#{params_hash}"

	// GSI1 - Cleanup by creation time
	GSI1PK string `dynamorm:"index:search-cache-cleanup,pk" json:"gsi1_pk"` // Format: "SEARCH_CACHE"
	GSI1SK string `dynamorm:"index:search-cache-cleanup,sk" json:"gsi1_sk"` // Format: "CREATED#{timestamp}"

	// Cache data
	Query        string                 `json:"query"`
	Parameters   map[string]interface{} `json:"parameters"`
	Results      []string               `json:"results"`       // Hashtag names
	TotalResults int                    `json:"total_results"` // Total available
	NextCursor   string                 `json:"next_cursor,omitempty"`
	HitCount     int64                  `json:"hit_count"` // Number of times used
	LastAccessed time.Time              `json:"last_accessed"`

	// TTL for automatic cleanup (2 hours for hashtag search cache)
	TTL int64 `dynamorm:"ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// UpdateKeys updates all the GSI keys when the cache data changes
func (hsc *HashtagSearchCache) UpdateKeys() {
	hsc.PK = fmt.Sprintf("HASHTAG_SEARCH_CACHE#%s", hsc.hashQuery(hsc.Query))
	hsc.SK = fmt.Sprintf("CACHE#%s", hsc.hashParameters(hsc.Parameters))

	// GSI1 for cleanup
	hsc.GSI1PK = "SEARCH_CACHE"
	hsc.GSI1SK = fmt.Sprintf("CREATED#%d", hsc.CreatedAt.Unix())
}

// hashQuery creates a simple hash of the query string
func (hsc *HashtagSearchCache) hashQuery(query string) string {
	// Simple hash implementation - in production use SHA-256
	hash := int64(0)
	for i, r := range query {
		hash = hash*31 + int64(r) + int64(i)
	}
	if hash < 0 {
		hash = -hash
	}
	return fmt.Sprintf("%016x", hash)
}

// hashParameters creates a hash of the parameters map
func (hsc *HashtagSearchCache) hashParameters(params map[string]interface{}) string {
	// Simple deterministic hash of parameters
	hash := int64(1)
	for k, v := range params {
		keyHash := int64(0)
		for i, r := range k {
			keyHash = keyHash*31 + int64(r) + int64(i)
		}
		valueHash := int64(0)
		valueStr := fmt.Sprintf("%v", v)
		for i, r := range valueStr {
			valueHash = valueHash*31 + int64(r) + int64(i)
		}
		hash = hash*37 + keyHash + valueHash
	}
	if hash < 0 {
		hash = -hash
	}
	return fmt.Sprintf("%016x", hash)
}
