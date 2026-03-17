// Package common provides centralized constants to comply with goconst linting requirements.
// This consolidates frequently used string literals found throughout the codebase.
package common

// Status constants - used across multiple services for consistent status tracking
const (
	// Entity status states
	StatusActive     = "active"
	StatusInactive   = "inactive"
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusExpired    = "expired"
	StatusSuspended  = "suspended"
	StatusDeleted    = "deleted"

	// Account/Actor states
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"
	AccountStatusPending   = "pending"
	AccountStatusDeleted   = "deleted"
)

// Operation type constants - used in ActivityPub and audit logging
const (
	// CRUD operations
	OperationCreate = "create"
	OperationRead   = "read"
	OperationUpdate = "update"
	OperationDelete = "delete"

	// ActivityPub activities
	ActivityCreate   = "Create"
	ActivityUpdate   = "Update"
	ActivityDelete   = "Delete"
	ActivityFollow   = "Follow"
	ActivityAccept   = "Accept"
	ActivityReject   = "Reject"
	ActivityAnnounce = "Announce"
	ActivityLike     = "Like"
	ActivityUndo     = "Undo"
	ActivityBlock    = "Block"
	ActivityFlag     = "Flag"
	ActivityMove     = "Move"
	ActivityAdd      = "Add"
	ActivityRemove   = "Remove"
)

// Visibility constants - used for post visibility settings (additional to business.go)
const (
	VisibilityLocal = "local" // Additional visibility level not in business.go
)

// Content type constants - additional to http_constants.go
const (
	ContentTypeActivityPub = "application/activity+json"
	ContentTypePlain       = "text/plain"
	ContentTypeForm        = "application/x-www-form-urlencoded"
	ContentTypeMultipart   = "multipart/form-data"
)

// Specific MIME type constants (complementary to business_mastodon.go MediaType)
const (
	// Specific image MIME types
	MediaTypeJPEG     = "image/jpeg"
	MediaTypePNG      = "image/png"
	MediaTypeWebP     = "image/webp"
	MediaTypeGIFImage = "image/gif"

	// Specific video MIME types
	MediaTypeMP4  = "video/mp4"
	MediaTypeWebM = "video/webm"
	MediaTypeMOV  = "video/quicktime"

	// Specific audio MIME types
	MediaTypeMP3  = "audio/mpeg"
	MediaTypeOGG  = "audio/ogg"
	MediaTypeWAV  = "audio/wav"
	MediaTypeFLAC = "audio/flac"
)

// Federation constants
const (
	// Actor types
	ActorTypePerson  = "Person"
	ActorTypeService = "Service"
	ActorTypeGroup   = "Group"

	// Object types
	ObjectTypeNote      = "Note"
	ObjectTypeArticle   = "Article"
	ObjectTypeImage     = "Image"
	ObjectTypeVideo     = "Video"
	ObjectTypeAudio     = "Audio"
	ObjectTypeDocument  = "Document"
	ObjectTypePage      = "Page"
	ObjectTypeEvent     = "Event"
	ObjectTypePlace     = "Place"
	ObjectTypeProfile   = "Profile"
	ObjectTypeTombstone = "Tombstone"

	// Collection types
	CollectionTypeOrderedCollection     = "OrderedCollection"
	CollectionTypeOrderedCollectionPage = "OrderedCollectionPage"
	CollectionTypeCollection            = "Collection"
	CollectionTypeCollectionPage        = "CollectionPage"
)

// Timeline and stream constants
const (
	TimelineHome      = "home"
	TimelinePublic    = "public"
	TimelineLocal     = "local"
	TimelineFederated = "federated"
	TimelineUser      = "user"
	TimelineHashtag   = "hashtag"
	TimelineList      = "list"

	// Stream event types
	StreamEventUpdate       = "update"
	StreamEventDelete       = "delete"
	StreamEventNotification = "notification"
	StreamEventFilters      = "filters_changed"
	StreamEventConversation = "conversation"
	StreamEventAnnouncement = "announcement"
	StreamEventStatusUpdate = "status.update"
)

// Notification types
const (
	NotificationTypeMention       = "mention"
	NotificationTypeReply         = "reply"
	NotificationTypeReblog        = "reblog"
	NotificationTypeFavourite     = "favourite"
	NotificationTypeFollow        = "follow"
	NotificationTypeFollowRequest = "follow_request"
	NotificationTypePoll          = "poll"
	NotificationTypeStatus        = "status"
	NotificationTypeUpdate        = "update"
	NotificationTypeAdminSignUp   = "admin.sign_up"
	NotificationTypeAdminReport   = "admin.report"
)

// Filter contexts are defined in business_mastodon.go as FilterContext type
// Additional filter contexts (if any) can be added here

// Job processing constants
const (
	// Job types
	JobTypeImport       = "import"
	JobTypeExport       = "export"
	JobTypeMedia        = "media"
	JobTypeFederation   = "federation"
	JobTypeModeration   = "moderation"
	JobTypeNotification = "notification"
	JobTypeSearch       = "search"
	JobTypeAnalytics    = "analytics"

	// Job priorities
	JobPriorityLow      = "low"
	JobPriorityNormal   = "normal"
	JobPriorityHigh     = "high"
	JobPriorityCritical = "critical"

	// Queue names
	QueueDefault      = "default"
	QueueImport       = "import"
	QueueExport       = "export"
	QueueMedia        = "media"
	QueueFederation   = "federation"
	QueueModeration   = "moderation"
	QueueNotification = "notification"
	QueueSearch       = "search"
	QueueAnalytics    = "analytics"
	QueueDLQ          = "dlq" // Dead letter queue
)

// Relationship constants
const (
	RelationshipFollowing     = "following"
	RelationshipFollowed      = "followed_by"
	RelationshipBlocked       = "blocked"
	RelationshipBlocking      = "blocking"
	RelationshipMuted         = "muted"
	RelationshipMuting        = "muting"
	RelationshipRequested     = "requested"
	RelationshipDomainBlocked = "domain_blocked"
	RelationshipEndorsed      = "endorsed"
)

// Report constants
const (
	ReportCategorySpam      = "spam"
	ReportCategoryViolation = "violation"
	ReportCategoryOther     = "other"
	ReportCategoryLegal     = "legal"

	ReportStatusOpen     = "open"
	ReportStatusResolved = "resolved"
	ReportStatusRejected = "rejected"
)

// Moderation constants
const (
	ModerationActionNone      = "none"
	ModerationActionWarn      = "warn"
	ModerationActionSilence   = "silence"
	ModerationActionSuspend   = "suspend"
	ModerationActionSensitive = "mark_as_sensitive"
	ModerationActionDelete    = "delete"

	ModerationSeverityInfo     = "info"
	ModerationSeverityWarning  = "warning"
	ModerationSeverityError    = "error"
	ModerationSeverityCritical = "critical"
)

// Search constants
const (
	SearchTypeAccount = "accounts"
	SearchTypeStatus  = "statuses"
	SearchTypeHashtag = "hashtags"
	SearchTypeAll     = "all"

	SearchScopeLocal  = "local"
	SearchScopeRemote = "remote"
	SearchScopeAll    = "all"
)

// File processing constants
const (
	ProcessingStatusQueued     = "queued"
	ProcessingStatusProcessing = "processing"
	ProcessingStatusCompleted  = "completed"
	ProcessingStatusFailed     = "failed"

	// File categories
	FileCategoryAvatar     = "avatar"
	FileCategoryHeader     = "header"
	FileCategoryAttachment = "attachment"
	FileCategoryEmoji      = "emoji"
	FileCategoryPreview    = "preview"
)

// Cache constants
const (
	CacheKeyUser         = "user:"
	CacheKeyActor        = "actor:"
	CacheKeyStatus       = "status:"
	CacheKeyAccount      = "account:"
	CacheKeyTimeline     = "timeline:"
	CacheKeyNotification = "notification:"
	CacheKeySearch       = "search:"
	CacheKeyMedia        = "media:"
	CacheKeyFederation   = "federation:"

	// Cache durations (used as string constants for configuration)
	CacheDurationShort    = "5m"
	CacheDurationMedium   = "30m"
	CacheDurationLong     = "2h"
	CacheDurationExtended = "24h"
)

// Database operation constants
const (
	DBOperationCreate = "create"
	DBOperationRead   = "read"
	DBOperationUpdate = "update"
	DBOperationDelete = "delete"
	DBOperationScan   = "scan"
	DBOperationQuery  = "query"
	DBOperationBatch  = "batch"

	// DynamoDB specific operations
	DynamoOperationPutItem            = "PutItem"
	DynamoOperationGetItem            = "GetItem"
	DynamoOperationUpdateItem         = "UpdateItem"
	DynamoOperationDeleteItem         = "DeleteItem"
	DynamoOperationQuery              = "Query"
	DynamoOperationScan               = "Scan"
	DynamoOperationBatchGetItem       = "BatchGetItem"
	DynamoOperationBatchWriteItem     = "BatchWriteItem"
	DynamoOperationTransactWriteItems = "TransactWriteItems"
	DynamoOperationTransactGetItems   = "TransactGetItems"
)

// Logging constants
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
	LogLevelFatal = "fatal"

	LogFieldComponent = "component"
	LogFieldOperation = "operation"
	LogFieldUserID    = "user_id"
	LogFieldStatusID  = "status_id"
	LogFieldDomain    = "domain"
	LogFieldError     = "error"
	LogFieldDuration  = "duration"
	LogFieldSize      = "size"
)

// HTTP header constants
const (
	HeaderAuthorization = "Authorization"
	HeaderContentType   = "Content-Type"
	HeaderAccept        = "Accept"
	HeaderUserAgent     = "User-Agent"
	HeaderSignature     = "Signature"
	HeaderDigest        = "Digest"
	HeaderDate          = "Date"
	HeaderHost          = "Host"
	HeaderActivityPub   = "activity+json"

	// CORS headers
	HeaderCORSOrigin      = "Access-Control-Allow-Origin"
	HeaderCORSMethods     = "Access-Control-Allow-Methods"
	HeaderCORSHeaders     = "Access-Control-Allow-Headers"
	HeaderCORSCredentials = "Access-Control-Allow-Credentials"
	HeaderCORSMaxAge      = "Access-Control-Max-Age"

	// Security headers
	HeaderXFrameOptions           = "X-Frame-Options"
	HeaderXContentTypeOptions     = "X-Content-Type-Options"
	HeaderXXSSProtection          = "X-XSS-Protection"
	HeaderStrictTransportSecurity = "Strict-Transport-Security"
	HeaderReferrerPolicy          = "Referrer-Policy"
)

// Environment constants
const (
	EnvProduction  = "production"
	EnvStaging     = "staging"
	EnvDevelopment = "development"
	EnvTesting     = "testing"
	EnvLocal       = "local"
)

// Configuration keys (used in configuration files and environment variables)
const (
	ConfigKeyDBHost     = "db.host"
	ConfigKeyDBPort     = "db.port"
	ConfigKeyDBName     = "db.name"
	ConfigKeyDBUser     = "db.user"
	ConfigKeyDBPassword = "db.password"

	ConfigKeyRedisHost     = "redis.host"
	ConfigKeyRedisPort     = "redis.port"
	ConfigKeyRedisPassword = "redis.password"

	ConfigKeyS3Bucket    = "s3.bucket"
	ConfigKeyS3Region    = "s3.region"
	ConfigKeyS3AccessKey = "s3.access_key"
	ConfigKeyS3SecretKey = "s3.secret_key" // #nosec G101 -- configuration key name, not a credential

	ConfigKeyDomainName = "domain.name"
	ConfigKeyBaseURL    = "base.url"
	ConfigKeyAPIVersion = "api.version"

	ConfigKeyJWTSecret     = "jwt.secret"
	ConfigKeyOAuthSecret   = "oauth.secret"
	ConfigKeyEncryptionKey = "encryption.key"

	ConfigKeyLogLevel  = "log.level"
	ConfigKeyLogFormat = "log.format"
	ConfigKeyLogOutput = "log.output"

	ConfigKeyRateLimitEnabled  = "ratelimit.enabled"
	ConfigKeyRateLimitRequests = "ratelimit.requests"
	ConfigKeyRateLimitWindow   = "ratelimit.window"

	ConfigKeyCacheEnabled = "cache.enabled"
	ConfigKeyCacheTTL     = "cache.ttl"
	ConfigKeyCacheSize    = "cache.size"
)

// Default values as constants (excluding DefaultPaginationLimit which exists in pagination.go)
const (
	DefaultMaxFileSize = 10 * 1024 * 1024 // 10MB
	DefaultTimeout     = 30               // seconds
	DefaultRetryCount  = 3
	DefaultBatchSize   = 100

	// Default TTL values (in seconds)
	DefaultCacheTTL   = 300    // 5 minutes
	DefaultSessionTTL = 3600   // 1 hour
	DefaultTokenTTL   = 900    // 15 minutes
	DefaultRefreshTTL = 604800 // 7 days
)

// Regular expression patterns as constants
const (
	PatternUsername = `^[a-zA-Z0-9_]{1,30}$`
	PatternEmail    = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	PatternURL      = `^https?://[^\s/$.?#].[^\s]*$`
	PatternHashtag  = `#[a-zA-Z0-9_]+`
	PatternMention  = `@[a-zA-Z0-9_]+(@[a-zA-Z0-9.-]+)?`
	PatternUUID     = `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
)

// Special character constants
const (
	CharSpace     = " "
	CharNewline   = "\n"
	CharTab       = "\t"
	CharEmpty     = ""
	CharDot       = "."
	CharComma     = ","
	CharSemicolon = ";"
	CharColon     = ":"
	CharSlash     = "/"
	CharBackslash = "\\"
	CharHash      = "#"
	CharAt        = "@"
	CharPlus      = "+"
	CharMinus     = "-"
	CharEquals    = "="
	CharQuestion  = "?"
	CharAmpersand = "&"
)
