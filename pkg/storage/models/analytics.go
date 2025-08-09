package models

import (
	"time"
)

// StatusEngagement tracks engagement on statuses (like, boost, reply)
type StatusEngagement struct {
	PK             string    `dynamorm:"pk"`                       // STATUS_ENGAGEMENT#statusID
	SK             string    `dynamorm:"sk"`                       // engagementType#timestamp#userID
	StatusID       string    `json:"status_id"`                    // Status being engaged with
	EngagementType string    `json:"engagement_type"`              // like, boost, reply
	UserID         string    `json:"user_id"`                      // User performing engagement
	EngagedAt      time.Time `json:"engaged_at"`                   // When engagement occurred
	TTL            int64     `json:"ttl,omitempty" dynamorm:"ttl"` // 7 day TTL
}

// UpdateKeys updates GSI keys for StatusEngagement - no GSIs needed for this model
func (s *StatusEngagement) UpdateKeys() {
	// No GSIs for this model
}

// LinkShare tracks when links are shared in statuses
type LinkShare struct {
	PK       string    `dynamorm:"pk"`                       // LINK_SHARE#url
	SK       string    `dynamorm:"sk"`                       // STATUS#statusID
	URL      string    `json:"url"`                          // The shared URL
	StatusID string    `json:"status_id"`                    // Status containing the link
	AuthorID string    `json:"author_id"`                    // User who shared the link
	SharedAt time.Time `json:"shared_at"`                    // When the link was shared
	TTL      int64     `json:"ttl,omitempty" dynamorm:"ttl"` // 7 day TTL
}

// UpdateKeys updates GSI keys for LinkShare - no GSIs needed for this model
func (l *LinkShare) UpdateKeys() {
	// No GSIs for this model
}

// EngagementMetrics tracks engagement metrics for platform usage analysis
type EngagementMetrics struct {
	// Key fields - EXACT pattern from legacy: PK=`METRICS#type#date`, SK=`target#targetID`
	PK     string `dynamorm:"pk"`            // METRICS#type#date or STATUS#statusID or ENGAGEMENT#bucket
	SK     string `dynamorm:"sk"`            // target#targetID or ENGAGEMENT#METRICS or STATUS#timestamp#statusID
	GSI8PK string `dynamorm:"index:gsi8,pk"` // For date range queries
	GSI8SK string `dynamorm:"index:gsi8,sk"` // For date range queries

	// Business fields from legacy
	MetricType  string    `json:"metric_type,omitempty"`
	TargetID    string    `json:"target_id,omitempty"` // user/post/hashtag ID
	Date        string    `json:"date,omitempty"`      // YYYY-MM-DD format
	Views       int64     `json:"views,omitempty"`
	Likes       int64     `json:"likes,omitempty"`
	Shares      int64     `json:"shares,omitempty"`
	Replies     int64     `json:"replies,omitempty"`
	UniqueUsers int64     `json:"unique_users,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`

	// Additional fields for status metrics
	StatusID         string  `json:"status_id,omitempty"`
	LikeCount        int64   `json:"like_count,omitempty"`
	BoostCount       int64   `json:"boost_count,omitempty"`
	ReplyCount       int64   `json:"reply_count,omitempty"`
	Score            float64 `json:"score,omitempty"`
	EngagementBucket string  `json:"engagement_bucket,omitempty"`

	// TTL field
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates GSI keys for EngagementMetrics
func (e *EngagementMetrics) UpdateKeys() {
	// GSI8 is used for date range queries
	e.GSI8PK = e.PK
	e.GSI8SK = e.SK
}

// TableName returns the DynamoDB table name
func (e *EngagementMetrics) TableName() string {
	return DefaultTableName // Replace with actual table name
}
