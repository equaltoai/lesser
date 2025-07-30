package storage

import "time"

// EngagementData represents engagement metrics data
type EngagementData struct {
	Views       int64 `json:"views"`
	Likes       int64 `json:"likes"`
	Shares      int64 `json:"shares"`
	Replies     int64 `json:"replies"`
	UniqueUsers int64 `json:"unique_users"`
}

// EngagementMetricsSummary represents a summary of engagement metrics
type EngagementMetricsSummary struct {
	Date        string `json:"date"`
	MetricType  string `json:"metric_type"`
	TargetID    string `json:"target_id"`
	TotalViews  int64  `json:"total_views"`
	TotalLikes  int64  `json:"total_likes"`
	TotalShares int64  `json:"total_shares"`
	UniqueUsers int64  `json:"unique_users"`
}

// EngagementRanking represents ranked engagement data
type EngagementRanking struct {
	TargetID    string  `json:"target_id"`
	Score       float64 `json:"score"`
	Views       int64   `json:"views"`
	Likes       int64   `json:"likes"`
	Shares      int64   `json:"shares"`
	Replies     int64   `json:"replies"`
	UniqueUsers int64   `json:"unique_users"`
}

// AggregatedEngagement represents aggregated engagement metrics
type AggregatedEngagement struct {
	MetricType       string          `json:"metric_type"`
	DateRange        string          `json:"date_range"`
	TotalViews       int64           `json:"total_views"`
	TotalLikes       int64           `json:"total_likes"`
	TotalShares      int64           `json:"total_shares"`
	TotalUniqueUsers int64           `json:"total_unique_users"`
	UniqueUsers      map[string]bool `json:"-"` // Used during aggregation
}

// TrendingHashtagData represents trending hashtag information
type TrendingHashtagData struct {
	Hashtag   string    `json:"hashtag"`
	Score     float64   `json:"score"`
	UseCount  int64     `json:"use_count"`
	UserCount int64     `json:"user_count"`
	History   []float64 `json:"history"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HashtagTrendHistory represents the trend history for a hashtag
type HashtagTrendHistory struct {
	Hashtag string       `json:"hashtag"`
	Days    []DailyTrend `json:"days"`
}

// DailyTrend represents a daily trend data point
type DailyTrend struct {
	Date      string  `json:"date"`
	Score     float64 `json:"score"`
	UseCount  int64   `json:"use_count"`
	UserCount int64   `json:"user_count"`
}

// InstanceMetricData represents instance-wide metric data
type InstanceMetricData struct {
	Date       string    `json:"date"`
	MetricType string    `json:"metric_type"`
	Value      int64     `json:"value"`
	Delta      int64     `json:"delta"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// MetricHistoryPoint represents a point in metric history
type MetricHistoryPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
	Delta int64  `json:"delta"`
}

// GrowthRate represents growth rate calculation results
type GrowthRate struct {
	MetricType     string  `json:"metric_type"`
	StartDate      string  `json:"start_date"`
	EndDate        string  `json:"end_date"`
	StartValue     int64   `json:"start_value"`
	EndValue       int64   `json:"end_value"`
	GrowthRate     float64 `json:"growth_rate"`
	AbsoluteChange int64   `json:"absolute_change"`
}

// ModerationAction represents a moderation action for analytics
type ModerationAction struct {
	ModeratorID    string  `json:"moderator_id"`
	Resolved       bool    `json:"resolved"`
	ResolutionTime float64 `json:"resolution_time"` // in hours
}

// ModerationAnalyticsData represents moderation analytics data
type ModerationAnalyticsData struct {
	Date                  string           `json:"date"`
	ReportType            string           `json:"report_type"`
	Count                 int64            `json:"count"`
	ResolvedCount         int64            `json:"resolved_count"`
	AverageResolutionTime float64          `json:"average_resolution_time"`
	ModeratorActions      map[string]int64 `json:"moderator_actions"`
	UpdatedAt             time.Time        `json:"updated_at"`
}

// ModeratorStatistics represents statistics for a moderator
type ModeratorStatistics struct {
	ModeratorID          string                    `json:"moderator_id"`
	TotalActions         int64                     `json:"total_actions"`
	ActionsByType        map[string]int64          `json:"actions_by_type"`
	AverageActionsPerDay float64                   `json:"average_actions_per_day"`
	DailyActions         []DailyModeratorAction    `json:"daily_actions"`
}

// DailyModeratorAction represents daily action count for a moderator
type DailyModeratorAction struct {
	Date    string `json:"date"`
	Actions int64  `json:"actions"`
}

// ReportTrend represents trend data for a report type
type ReportTrend struct {
	ReportType     string              `json:"report_type"`
	TotalCount     int64               `json:"total_count"`
	TotalResolved  int64               `json:"total_resolved"`
	ResolutionRate float64             `json:"resolution_rate"`
	Daily          []DailyReportCount  `json:"daily"`
}

// DailyReportCount represents daily report counts
type DailyReportCount struct {
	Date          string `json:"date"`
	Count         int64  `json:"count"`
	ResolvedCount int64  `json:"resolved_count"`
}