package storage

import "github.com/equaltoai/lesser/pkg/config"

// DynamoDB Key Prefixes and Patterns
const (
	// User-related keys
	UserKeyPrefix    = "USER#%s"
	UserPinsKey      = "USER#%s#PINS"
	UserScheduledKey = "USER#%s#SCHEDULED"

	// Actor-related keys
	ActorKeyPrefix = "ACTOR#%s"
	ActorBlocksKey = "ACTOR#%s#BLOCKS"

	// Instance-related keys
	InstanceConfigKey      = "INSTANCE#CONFIG"
	InstanceConnectionsKey = "INSTANCE#%s#CONNECTIONS"

	// Delivery keys
	DeliveryKeyPrefix = "DELIVERY#%s"

	// WebSocket-related keys
	WSConnKey       = "WS_CONN#%s"
	WSUserKey       = "WS_USER#%s"
	WSCostKey       = "WS_COST#%s"
	WSBudgetKey     = "WS_BUDGET#%s#%s"
	WSAggKey        = "WS_AGG#%s#%s"
	WSUserBudgetKey = "WS_USER_BUDGET#%s"
	WSUserAggKey    = "WS_USER_AGG#%s#%s"

	// Wallet-related keys
	WalletKeyPrefix    = "WALLET#%s"
	WalletFullKey      = "WALLET#%s#%s"
	WalletBaseKey      = "WALLET#"
	WalletChallengeKey = "WALLET_CHALLENGE#%s"

	// Vouch/reputation keys
	VouchKeyPrefix = "VOUCH#%s"
	VoucheeKey     = "VOUCHEE#%s"
	VoucherKey     = "VOUCHER#%s"

	// Vote keys
	VoteKeyPrefix = "VOTE#%s"
	VoteBaseKey   = "VOTE#"

	// Search keys
	UsernameSearchKey = "USERNAME_SEARCH#%s"

	// Window keys
	WindowKeyString = "WINDOW#%s"
	WindowKeyNumber = "WINDOW#%d"
)

// Common field names used across models
const (
	FieldObjectID         = "objectID"
	FieldTargetID         = "targetID"
	FieldUsername         = "username"
	FieldCreatedAt        = "created_at"
	FieldUpdatedAt        = "updated_at"
	FieldUserID           = "user_id"
	FieldStatus           = "status"
	FieldType             = "type"
	FieldDomain           = "domain"
	FieldName             = "name"
	FieldRequestID        = "request_id"
	FieldCollectionType   = "collection_type"
	FieldExpiresAt        = "ExpiresAt"
	FieldRetryIndex       = "retry-index"
	FieldS3Key            = "s3_key"
	FieldAuthor           = "author"
	FieldTags             = "tags"
	FieldWarningThreshold = "warning_threshold"
	FieldTotalDuration    = "total_duration"
	FieldSubscriberID     = "subscriber_id"
	FieldOAuth            = "oauth"
)

// Table names
var (
	// MainTableName is the primary DynamoDB table name
	MainTableName = config.GetMainTableName()
)

// Cost tracking constants
const (
	// CostPKDriver is the partition key for cost drivers
	CostPKDriver = "COST#DRIVER"

	// CostPKProjection is the partition key for cost projections
	CostPKProjection = "COST#PROJECTION"
)

// Trend constants
const (
	// TrendIncreasing indicates an increasing trend
	TrendIncreasing = "increasing"

	// TrendDecreasing indicates a decreasing trend
	TrendDecreasing = "decreasing"

	// TrendStable indicates a stable trend
	TrendStable = "stable"
)

// Period constants
const (
	// PeriodHourly represents hourly aggregation
	PeriodHourly = "hourly"

	// PeriodDaily represents daily aggregation
	PeriodDaily = "daily"

	// PeriodWeekly represents weekly aggregation
	PeriodWeekly = "weekly"

	// PeriodMonthly represents monthly aggregation
	PeriodMonthly = "month"
)

// Priority constants
const (
	// PriorityLow represents low priority
	PriorityLow = "low"

	// PriorityMedium represents medium priority
	PriorityMedium = "medium"

	// PriorityHigh represents high priority
	PriorityHigh = "high"

	// PriorityCritical represents critical priority
	PriorityCritical = "critical"
)

// Status constants
const (
	// StatusPending represents pending status
	StatusPending = "pending"

	// StatusProcessing represents processing status
	StatusProcessing = "processing"

	// StatusCompleted represents completed status
	StatusCompleted = "completed"

	// StatusFailed represents failed status
	StatusFailed = "failed"

	// StatusRetrying represents retrying status
	StatusRetrying = "retrying"
)
