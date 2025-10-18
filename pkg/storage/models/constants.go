package models

// Common SK (Sort Key) constants used across multiple models
const (
	// SKMetadata is used for metadata records
	SKMetadata = "METADATA"

	// SKConfig is used for configuration records
	SKConfig = "CONFIG"

	// SKToken is used for token records
	SKToken = "TOKEN"

	// SKStats is used for statistics records
	SKStats = "STATS"

	// SKState is used for state records
	SKState = "STATE"

	// SKProfile is used for profile records
	SKProfile = "PROFILE"

	// SKInfo is used for info records
	SKInfo = "INFO"

	// SKEntry is used for entry records
	SKEntry = "ENTRY"

	// SKCurrent is used for current/active records
	SKCurrent = "CURRENT"

	// SKCode is used for code records
	SKCode = "CODE"
)

// Common key pattern prefixes used for PK and GSI keys
const (
	// User-related key patterns
	KeyPatternUser          = "USER#%s"
	KeyPatternUserPins      = "USER#%s#PINS"
	KeyPatternUserScheduled = "USER#%s#SCHEDULED"

	// Actor-related key patterns
	KeyPatternActor       = "ACTOR#%s"
	KeyPatternActorBlocks = "ACTOR#%s#BLOCKS"

	// Status-related key patterns
	KeyPatternStatus     = "STATUS#%s"
	KeyPatternStatusMeta = "STATUS_META#%s"

	// Object-related key patterns
	KeyPatternObject      = "OBJECT#%s"
	KeyPatternObjectNotes = "OBJECT#%s#NOTES"

	// Note-related key patterns
	KeyPatternNote = "NOTE#%s"

	// Device-related key patterns
	KeyPatternDevice = "DEVICE#%s"

	// Connection-related key patterns
	KeyPatternConnection = "CONNECTION#%s"

	// Session-related key patterns
	KeyPatternSession = "SESSION#%s"

	// Conversation-related key patterns
	KeyPatternConversation = "CONVERSATION#%s"

	// List-related key patterns
	KeyPatternList = "LIST#%s"

	// Hashtag-related key patterns
	KeyPatternHashtag       = "HASHTAG#%s"
	KeyPatternHashtagSearch = "HASHTAG_SEARCH#%s"

	// Media-related key patterns
	KeyPatternMedia            = "MEDIA#%s"
	KeyPatternMediaJob         = "JOB#%s"
	KeyPatternMediaSpending    = "MEDIA_SPENDING#%s"
	KeyPatternSpendingTxn      = "SPENDING_TXN#%s"
	KeyPatternTranscodingJob   = "TRANSCODING_JOB#%s"
	KeyPatternMediaIdempotency = "MEDIA_IDEM#%s"
	KeyPatternMediaBudget      = "MEDIA_BUDGET#%s"
)

// Table names
const (
	// MainTableName is the primary DynamoDB table name
	MainTableName = "lesser-main"

	// DefaultTableName is a placeholder table name for test/example code
	DefaultTableName = "lesser-table"
)

// Cost tracking key patterns
const (
	// CostDriverPK is the partition key for cost drivers
	CostDriverPK = "COST#DRIVER"

	// CostProjectionPK is the partition key for cost projections
	CostProjectionPK = "COST#PROJECTION"

	// FederationStatsPK is the partition key for federation stats
	FederationStatsPK = "FEDERATION#STATS"

	// TrendIncreasing indicates an increasing trend
	TrendIncreasing = "increasing"
	// TrendDecreasing indicates a decreasing trend
	TrendDecreasing = "decreasing"
	// TrendStable indicates a stable trend
	TrendStable = "stable"
)

// Resource category constants
const (
	ResourceStorage    = "storage"
	ResourceCompute    = "compute"
	ResourceBandwidth  = "bandwidth"
	ResourceLambda     = "lambda"
	ResourceProcessing = "processing"
)

// Period constants
const (
	PeriodRaw     = "raw"
	Period5Min    = "5min"
	PeriodHour    = "hour"
	PeriodDay     = "day"
	PeriodWeek    = "week"
	PeriodMonth   = "month"
	PeriodMonthly = "monthly"
	PeriodHourly  = "hourly"
	PeriodDaily   = "daily"
	PeriodWeekly  = "weekly"
)

// Status constants
const (
	StatusActive     = "active"
	StatusPending    = "pending"
	StatusHealthy    = "healthy"
	StatusWarning    = "warning"
	StatusCritical   = "critical"
	StatusFailed     = "failed"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusComplete   = "complete"
	StatusSuccess    = "success"
	StatusDisabled   = "disabled"
	StatusTimeout    = "timeout"
	StatusError      = "error"
	StatusCancelled  = "cancelled"
	StatusReady      = "ready"
	StatusRetry      = "retry"
)

// Media processing status constants
const (
	MediaStatusPending    = "pending"
	MediaStatusProcessing = "processing"
	MediaStatusCompleted  = "completed"
	MediaStatusFailed     = "failed"
	MediaStatusCancelled  = "cancelled"
	MediaStatusReady      = "ready"
)

// Media processing cost categories
const (
	MediaCostUpload     = "upload"
	MediaCostStorage    = "storage"
	MediaCostTranscode  = "transcode"
	MediaCostThumbnail  = "thumbnail"
	MediaCostModeration = "moderation"
	MediaCostAnalysis   = "analysis"
	MediaCostDelivery   = "delivery"
)

// Media processing task types
const (
	MediaTaskUpload     = "upload"
	MediaTaskResize     = "resize"
	MediaTaskThumbnail  = "thumbnail"
	MediaTaskBlurhash   = "blurhash"
	MediaTaskDimensions = "dimensions"
	MediaTaskDuration   = "duration"
	MediaTaskTranscode  = "transcode"
	MediaTaskWaveform   = "waveform"
	MediaTaskMetadata   = "metadata"
	MediaTaskEXIF       = "exif"
	MediaTaskModeration = "moderation"
)

// Visibility constants for activity permissions
const (
	VisibilityDisabled = "disabled"
)

// Cost tier constants
const (
	CostTierLow     = "low"
	CostTierMedium  = "medium"
	CostTierHigh    = "high"
	CostTierPremium = "premium"
)

// Period time constants
const (
	PeriodTimeHour = "hour"
)

// Other constants
const (
	TrusteeConfigPK     = "TRUSTEE#CONFIG"
	ModerationTypeEvent = "EVENT"
	TypeFollowed        = "followed"
	TypeEmail           = "email"
	TimelinePublic      = "PUBLIC"
)

// Entity type constants
const (
	EntityTypeUser            = "user"
	EntityTypeScheduledStatus = "scheduled_status"
	EntityTypeStatus          = "status"
	EntityTypeAccount         = "account"
)
