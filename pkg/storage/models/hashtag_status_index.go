package models

import (
	"fmt"
	"strings"
	"time"
)

// HashtagStatusIndex represents an efficient index for hashtag-to-status mapping
// This enables fast hashtag timeline queries without scanning all statuses
type HashtagStatusIndex struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - hashtag timeline
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "HASHTAG_TIMELINE#{hashtag_name}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "STATUS#{timestamp_desc}#{status_id}"

	// GSI1 - Status-to-hashtag reverse index for cleanup
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"` // Format: "STATUS_HASHTAGS#{status_id}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"` // Format: "HASHTAG#{hashtag_name}"

	// GSI2 - Timeline by visibility for filtering
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk"` // Format: "HASHTAG_VIS#{hashtag_name}#{visibility}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk"` // Format: "TIMELINE#{timestamp_desc}"

	// Status data
	StatusID     string    `theorydb:"attr:statusID" json:"status_id"`
	AuthorID     string    `theorydb:"attr:authorID" json:"author_id"`
	AuthorHandle string    `theorydb:"attr:authorHandle" json:"author_handle"`
	StatusURL    string    `theorydb:"attr:statusURL" json:"status_url,omitempty"`
	Content      string    `theorydb:"attr:content" json:"content,omitempty"`        // Excerpt for search results
	MediaCount   int       `theorydb:"attr:mediaCount" json:"media_count,omitempty"` // Number of media attachments
	Language     string    `theorydb:"attr:language" json:"language,omitempty"`      // Content language
	Visibility   string    `theorydb:"attr:visibility" json:"visibility"`            // public, unlisted, private, direct
	Published    time.Time `theorydb:"attr:published" json:"published"`              // When the status was published
	HashtagName  string    `theorydb:"attr:hashtagName" json:"hashtag_name"`         // The hashtag (for reverse index)

	// TTL for automatic cleanup (90 days for efficient hashtag timelines)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName ensures HashtagStatusIndex records persist in the shared Dynamo table.
func (HashtagStatusIndex) TableName() string {
	return MainTableName
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - trending hashtag data
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "TRENDING_HASHTAG#{date}#{hour}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "HASHTAG#{score_padded}#{hashtag_name}"

	// GSI1 - Query by hashtag across time periods
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"` // Format: "HASHTAG_TREND#{hashtag_name}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"` // Format: "TIME#{timestamp}"

	// GSI2 - Query trending for time period
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk"` // Format: "TRENDING_PERIOD#{time_window}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk"` // Format: "SCORE#{score_padded}#{hashtag_name}"

	// Hashtag info
	HashtagName string    `theorydb:"attr:hashtagName" json:"hashtag_name"`
	URL         string    `theorydb:"attr:url" json:"url"`
	Period      time.Time `theorydb:"attr:period" json:"period"`          // Start of the time period
	TimeWindow  string    `theorydb:"attr:timeWindow" json:"time_window"` // "1h", "6h", "24h", "7d"

	// Trending metrics
	TrendScore     float64 `theorydb:"attr:trendScore" json:"trend_score"`         // Overall trending score
	UsageCount     int64   `theorydb:"attr:usageCount" json:"usage_count"`         // Usage count in period
	UniqueUsers    int64   `theorydb:"attr:uniqueUsers" json:"unique_users"`       // Unique users in period
	Growth         float64 `theorydb:"attr:growth" json:"growth"`                  // Growth rate vs previous period
	Velocity       float64 `theorydb:"attr:velocity" json:"velocity"`              // Usage per hour
	MomentumScore  float64 `theorydb:"attr:momentumScore" json:"momentum_score"`   // Acceleration indicator
	TrustScore     float64 `theorydb:"attr:trustScore" json:"trust_score"`         // Trust-weighted score
	EngagementRate float64 `theorydb:"attr:engagementRate" json:"engagement_rate"` // Engagement per usage
	DiversityScore float64 `theorydb:"attr:diversityScore" json:"diversity_score"` // User diversity score

	// Component scores for analysis
	ComponentScores map[string]float64 `theorydb:"attr:componentScores" json:"component_scores,omitempty"`

	// TTL for automatic cleanup (30 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName ensures trending data records use the shared single table.
func (HashtagTrendingData) TableName() string {
	return MainTableName
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - search cache
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "HASHTAG_SEARCH_CACHE#{query_hash}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "CACHE#{params_hash}"

	// GSI1 - Cleanup by creation time
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"` // Format: "SEARCH_CACHE"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"` // Format: "CREATED#{timestamp}"

	// Cache data
	Query        string                 `theorydb:"attr:query" json:"query"`
	Parameters   map[string]interface{} `theorydb:"attr:parameters" json:"parameters"`
	Results      []string               `theorydb:"attr:results" json:"results"`            // Hashtag names
	TotalResults int                    `theorydb:"attr:totalResults" json:"total_results"` // Total available
	NextCursor   string                 `theorydb:"attr:nextCursor" json:"next_cursor,omitempty"`
	HitCount     int64                  `theorydb:"attr:hitCount" json:"hit_count"` // Number of times used
	LastAccessed time.Time              `theorydb:"attr:lastAccessed" json:"last_accessed"`

	// TTL for automatic cleanup (2 hours for hashtag search cache)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName ensures hashtag search cache items share the main table.
func (HashtagSearchCache) TableName() string {
	return MainTableName
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
