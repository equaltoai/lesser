// Package graph implements the GraphQL API schema and resolvers.
package graph

// Content type constants
const (
	ContentTypeSpam = "spam"
	ContentTypeNote = "note"
)

// Health status constants
const (
	HealthStatusHealthy  = "healthy"
	HealthStatusWarning  = "warning"
	HealthStatusCritical = "critical"
	HealthStatusError    = "error"
	HealthStatusUnknown  = "unknown"
)

// Service type constants
const (
	ServiceTypeTimeline      = "timeline"
	ServiceTypeSearch        = "search"
	ServiceTypeNotifications = "notifications"
)

// Metric type constants
const (
	MetricTypeServiceStatus     = "service_status"
	MetricTypePerformanceMetric = "performance_metric"
)

// Federation software constants
const (
	SoftwareMastodon = "mastodon"
	SoftwarePleroma  = "pleroma"
	SoftwareLemmy    = "lemmy"
	SoftwarePeertube = "peertube"
	SoftwarePixelfed = "pixelfed"
)

// User status constants
const (
	UserStatusSuspended = "suspended"
	UserStatusActive    = "active"
)

// Relationship constants
const (
	RelationshipFollowing = "following"
	RelationshipFollower  = "follower"
)

// Public key constant
const (
	MockPublicKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A...
-----END PUBLIC KEY-----`
)

// Activity type constants
const (
	ActivityTypeMention      = "Mention"
	ActivityTypeQuoteCreated = "quote_created"
)

// Domain constants
const (
	DomainLocalhost = "localhost"
)

// RSA key markers
const (
	RSAPublicKeyEnd = "-----END PUBLIC KEY-----"
)

// Moderation action constants
const (
	ModerationActionFlag    = "flag"
	ModerationActionReview  = "review"
	ModerationActionReject  = "reject"
	ModerationActionApprove = "approve"
)

// Query type constants
const (
	QueryTypeActor = "actor"
	QueryTypeAll   = "all"
)

// Pattern type constants
const (
	PatternTypeRegex   = "regex"
	PatternTypeKeyword = "keyword"
)

// Severity constants
const (
	SeverityHigh = "high"
	SeverityLow  = "low"
)

// Subscription and event type constants
const (
	SubscriptionTypeMetrics = "metrics"
	EventTypeFollowers      = "followers"
	EventTypeCost           = "cost"
	VisibilityPublic        = "public"
	VisibilityDisabled      = "disabled"
	TagTypeMention          = "Mention"
)
