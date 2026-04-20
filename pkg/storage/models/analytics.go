package models

import (
	"fmt"
	"time"
)

// StatusEngagement tracks engagement on statuses (like, boost, reply)
type StatusEngagement struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK             string    `theorydb:"pk,attr:PK"`                                 // STATUS_ENGAGEMENT#statusID
	SK             string    `theorydb:"sk,attr:SK"`                                 // engagementType#timestamp#userID
	StatusID       string    `theorydb:"attr:statusID" json:"status_id"`             // Status being engaged with
	EngagementType string    `theorydb:"attr:engagementType" json:"engagement_type"` // like, boost, reply
	UserID         string    `theorydb:"attr:userID" json:"user_id"`                 // User performing engagement
	EngagedAt      time.Time `theorydb:"attr:engagedAt" json:"engaged_at"`           // When engagement occurred
	TTL            int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`          // 7 day TTL
}

// TableName returns the DynamoDB table backing StatusEngagement.
func (StatusEngagement) TableName() string {
	return MainTableName
}

// UpdateKeys updates GSI keys for StatusEngagement - no GSIs needed for this model
func (s *StatusEngagement) UpdateKeys() error {
	// Set primary keys (required for DynamoDB operations)
	s.PK = fmt.Sprintf("STATUS_ENGAGEMENT#%s", s.StatusID)
	s.SK = fmt.Sprintf("%s#%s#%s", s.EngagementType, s.EngagedAt.Format(time.RFC3339), s.UserID)
	// No GSIs for this model
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (s *StatusEngagement) GetPK() string {
	return s.PK
}

// GetSK returns the sort key for BaseModel interface
func (s *StatusEngagement) GetSK() string {
	return s.SK
}

// LinkShare tracks when links are shared in statuses
type LinkShare struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK       string    `theorydb:"pk,attr:PK"`                        // LINK_SHARE#url
	SK       string    `theorydb:"sk,attr:SK"`                        // STATUS#statusID
	URL      string    `theorydb:"attr:url" json:"url"`               // The shared URL
	StatusID string    `theorydb:"attr:statusID" json:"status_id"`    // Status containing the link
	AuthorID string    `theorydb:"attr:authorID" json:"author_id"`    // User who shared the link
	SharedAt time.Time `theorydb:"attr:sharedAt" json:"shared_at"`    // When the link was shared
	TTL      int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"` // 7 day TTL
}

// UpdateKeys updates GSI keys for LinkShare - no GSIs needed for this model
func (l *LinkShare) UpdateKeys() error {
	// Set primary keys (required for DynamoDB operations)
	l.PK = fmt.Sprintf("LINK_SHARE#%s", l.URL)
	l.SK = fmt.Sprintf("STATUS#%s", l.StatusID)
	// No GSIs for this model
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (l *LinkShare) GetPK() string {
	return l.PK
}

// GetSK returns the sort key for BaseModel interface
func (l *LinkShare) GetSK() string {
	return l.SK
}

// TableName returns the DynamoDB table backing LinkShare.
func (LinkShare) TableName() string {
	return MainTableName
}

// EngagementMetrics tracks engagement metrics for platform usage analysis
type EngagementMetrics struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Key fields - EXACT pattern from legacy: PK=`METRICS#type#date`, SK=`target#targetID`
	PK     string `theorydb:"pk,attr:PK"`                          // METRICS#type#date or STATUS#statusID or ENGAGEMENT#bucket
	SK     string `theorydb:"sk,attr:SK"`                          // target#targetID or ENGAGEMENT#METRICS or STATUS#timestamp#statusID
	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK,omitempty"` // For date range queries
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK,omitempty"` // For date range queries

	// Business fields from legacy
	MetricType  string    `theorydb:"attr:metricType" json:"metric_type,omitempty"`
	TargetID    string    `theorydb:"attr:targetID" json:"target_id,omitempty"` // user/post/hashtag ID
	Date        string    `theorydb:"attr:date" json:"date,omitempty"`          // YYYY-MM-DD format
	Views       int64     `theorydb:"attr:views" json:"views,omitempty"`
	Likes       int64     `theorydb:"attr:likes" json:"likes,omitempty"`
	Shares      int64     `theorydb:"attr:shares" json:"shares,omitempty"`
	Replies     int64     `theorydb:"attr:replies" json:"replies,omitempty"`
	UniqueUsers int64     `theorydb:"attr:uniqueUsers" json:"unique_users,omitempty"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at,omitempty"`

	// Additional fields for status metrics
	StatusID         string  `theorydb:"attr:statusID" json:"status_id,omitempty"`
	LikeCount        int64   `theorydb:"attr:likeCount" json:"like_count,omitempty"`
	BoostCount       int64   `theorydb:"attr:boostCount" json:"boost_count,omitempty"`
	ReplyCount       int64   `theorydb:"attr:replyCount" json:"reply_count,omitempty"`
	Score            float64 `theorydb:"attr:score" json:"score,omitempty"`
	EngagementBucket string  `theorydb:"attr:engagementBucket" json:"engagement_bucket,omitempty"`

	// TTL field
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates GSI keys for EngagementMetrics
func (e *EngagementMetrics) UpdateKeys() error {
	// Note: PK and SK must be set by the caller/repository based on the specific use case
	// This model has multiple key patterns: METRICS#type#date, STATUS#statusID, ENGAGEMENT#bucket
	// We cannot reconstruct them here without knowing the context

	// GSI8 is used for date range queries
	e.GSI8PK = e.PK
	e.GSI8SK = e.SK
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (e *EngagementMetrics) GetPK() string {
	return e.PK
}

// GetSK returns the sort key for BaseModel interface
func (e *EngagementMetrics) GetSK() string {
	return e.SK
}

// TableName returns the DynamoDB table backing EngagementMetrics.
func (e *EngagementMetrics) TableName() string {
	return MainTableName
}
