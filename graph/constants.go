// Package graph implements the GraphQL API schema and resolvers.
package graph

// Content type constants
const (
	ContentTypeSpam    = "spam"
	ContentTypeNote    = "note"
	ContentType        = "content"
	ContentTypeArticle = "article"
	ContentTypeImage   = "image"
	ContentTypeEvent   = "event"
	ContentTypeVideo   = "video"
)

// Generic fallback constants
const unknownValue = "unknown"

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
	ServiceTypeTrust         = "trust"
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

// Admin account action constants (Mastodon admin compatibility)
const (
	AdminAccountActionSensitive   = "sensitive"
	AdminAccountActionUnsensitive = "unsensitive"
	AdminAccountActionSilence     = "silence"
	AdminAccountActionSuspend     = "suspend"
)

// Instance domain block severities
const (
	DomainBlockSeveritySilence = "silence"
	DomainBlockSeveritySuspend = "suspend"
)

// Query type constants
const (
	QueryTypeActor    = "actor"
	QueryTypeAll      = "all"
	QueryTypeAccounts = "accounts"
	QueryTypeStatuses = "statuses"
	QueryTypeHashtags = "hashtags"
)

// Pattern type constants
const (
	PatternTypeRegex   = "regex"
	PatternTypeKeyword = "keyword"
	PatternTypePhrase  = "phrase"
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
	VisibilityUnlisted      = "unlisted"
	VisibilityDisabled      = "disabled"
	TagTypeMention          = "Mention"
)

// Timeline and stream type constants
const (
	TimelineTypeHashtag = "hashtag"
	TimelineTypeList    = "list"
	TimelineTypeHome    = "home"
	TimelineTypeDirect  = "direct"
	TimelineTypeUser    = adminRoleUser
	TimelineTypeStatus  = "status"
)

// Object type constants
const (
	ObjectTypeStatus = "status"
)

// Alert level constants
const (
	AlertLevelLow      = "LOW"
	AlertLevelMedium   = "MEDIUM"
	AlertLevelHigh     = "HIGH"
	AlertLevelCritical = "CRITICAL"
)

// Quote type constants
const (
	QuoteType = "quote"
)

// Moderation action constants
const (
	ModerationActionHide   = "hide"
	ModerationActionRemove = "remove"
)

// Visibility and severity constants
const (
	VisibilityPrivate = "private"
	SeverityMedium    = "medium"
)

// None constant
const (
	NoneValue = "none"
)

// Vote type constants
const (
	VoteTypeHelpful    = "helpful"
	VoteTypeNotHelpful = "not_helpful"
)

// Job status constants
const (
	JobStatusCompleted  = "completed"
	JobStatusProcessing = "processing"
	JobStatusStatus     = "status"
)

// Event type constants
const (
	EventTypeFlagged = "flagged"
)

// Account type constants
const (
	AccountType = "account"
)

// Text type constants
const (
	TextType = "text"
)

// DynamoDB operation constants
const (
	DynamoOperationRead  = "read"
	DynamoOperationQuery = "query"
)

// dangerousURLSchemes lists URL schemes that must never appear in actor
// identifier or URL fields, even when the value is not expected to be a
// well-formed URL (e.g. email-address placeholders).
var dangerousURLSchemes = []string{
	"javascript",
	"data",
	"vbscript",
}
