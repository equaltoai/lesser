package repositories

// Account status constants
const (
	AccountStatusEmail     = "email"
	AccountStatusSuspended = "suspended"
)

// Status constants - these supplement models.Status* constants
const (
	StatusUnknown   = "unknown"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusError     = "error"
	StatusPending   = "pending"
	StatusHealthy   = "healthy"
	StatusCritical  = "critical"
	StatusWarning   = "warning"
	StatusLow       = "low"
	StatusHigh      = "high"
	StatusMedium    = "medium"
)

// Period constants - these supplement models.Period* constants
const (
	PeriodDaily   = "daily"
	PeriodWeekly  = "weekly"
	PeriodMonthly = "monthly"
)

// Type constants
const (
	TypeFollowed = "followed"
)

// Connection type constants
const (
	ConnectionTypeAll = "all"
)

// Replies policy constants
const (
	RepliesPolicyList     = "list"
	RepliesPolicyFollowed = "followed"
	RepliesPolicyNone     = "none"
)

// WebSocket event types
const (
	WSEventConnect    = "connect"
	WSEventDisconnect = "disconnect"
	WSEventMessageIn  = "message_in"
	WSEventMessageOut = "message_out"
	WSEventSubscribe  = "subscribe"
)

// Other repository constants
const (
	ActivityTypeNote    = "Note"
	ModerationTypeEvent = "EVENT"
	VisibilityPublic    = "public"
	VisibilityDisabled  = "disabled"
	TagTypeMention      = "Mention"
)

// GSI field name constants
const (
	gsi1SKField = "gsi1SK"
	gsi2SKField = "gsi2SK"
)

// User preference keys
const (
	PrefKeyLanguage                  = "language"
	PrefKeyDefaultPostingVisibility  = "default_posting_visibility"
	PrefKeyDefaultMediaSensitive     = "default_media_sensitive"
	PrefKeyExpandSpoilers            = "expand_spoilers"
	PrefKeyExpandMedia               = "expand_media"
	PrefKeyAutoplayGifs              = "autoplay_gifs"
	PrefKeyShowFollowCounts          = "show_follow_counts"
	PrefKeyPreferredTimelineOrder    = "preferred_timeline_order"
	PrefKeySearchSuggestionsEnabled  = "search_suggestions_enabled"
	PrefKeyPersonalizedSearchEnabled = "personalized_search_enabled"
	PrefKeyReblogFilters             = "reblog_filters"
	PrefKeyStreamingDefaultQuality   = "streaming_default_quality"
	PrefKeyStreamingAutoQuality      = "streaming_auto_quality"
	PrefKeyStreamingPreloadNext      = "streaming_preload_next"
	PrefKeyStreamingDataSaver        = "streaming_data_saver"
	PrefKeyDirect                    = "direct"
)
