package models

import "errors"

// Model validation and business logic errors
var (
	// User Media Config errors
	ErrInvalidPlanTier = errors.New("invalid plan tier")
	ErrInvalidFileSize = errors.New("invalid file size configuration")
	ErrFileSizeTooLarge = errors.New("file size exceeds limits")
	ErrVideoDurationInvalid = errors.New("invalid video duration")
	ErrUploadLimitsInvalid = errors.New("invalid upload limits")
	ErrBudgetLimitsInvalid = errors.New("invalid budget limits")
	ErrModerationThresholdInvalid = errors.New("invalid moderation threshold")
	ErrInvalidQualitySetting = errors.New("invalid quality setting")
	ErrPlanUpgradeFailed = errors.New("plan upgrade failed")
	ErrUserIDRequired = errors.New("user ID is required")

	// Relay Cost errors
	ErrRelayWindowStartRequired = errors.New("window start is required")
	ErrInvalidBudgetLimit = errors.New("limit_micro_cents must be positive")

	// Transcoding Job errors
	ErrTranscodingJobIDRequired = errors.New("transcoding job ID is required")
	ErrTranscodingMediaIDRequired = errors.New("transcoding media ID is required")
	ErrTranscodingUserIDRequired = errors.New("transcoding user ID is required")
	ErrInvalidJobType = errors.New("invalid job type")
	ErrInvalidJobStatus = errors.New("invalid job status")
	ErrNegativeSize = errors.New("sizes cannot be negative")
	ErrNegativeCost = errors.New("costs cannot be negative")

	// Metrics validation errors
	ErrMetricIDRequired = errors.New("ID is required")
	ErrMetricTypeRequired = errors.New("type is required")
	ErrMetricServiceRequired = errors.New("service is required")
	ErrInvalidMetricType = errors.New("invalid metric type")
	ErrInvalidPeriod = errors.New("invalid period")
	ErrMetricWindowStartRequired = errors.New("WindowStart is required")
	ErrMetricWindowEndRequired = errors.New("WindowEnd is required")
	ErrWindowEndBeforeStart = errors.New("WindowEnd must be after WindowStart")
	ErrTimestampRequired = errors.New("timestamp is required")
	ErrAggregationLevelRequired = errors.New("AggregationLevel is required")
	ErrInvalidAggregationLevel = errors.New("invalid aggregation level")
	ErrFailedToUpdateKeys = errors.New("failed to update keys")

	// MetricRecord validation errors  
	ErrMetricRecordTypeRequired = errors.New("MetricType is required")
	ErrMetricRecordServiceRequired = errors.New("ServiceName is required")

	// WebSocket Cost Tracking errors
	ErrInvalidWebSocketOperationType = errors.New("invalid operation type")
	ErrInvalidWebSocketPeriod = errors.New("invalid period")
	ErrBudgetMicroCentsNegative = errors.New("BudgetMicroCents cannot be negative")
	ErrWebSocketWindowStartRequired = errors.New("WindowStart is required")
	ErrWebSocketWindowEndRequired = errors.New("WindowEnd is required")
	ErrWebSocketWindowEndBeforeStart = errors.New("WindowEnd must be after WindowStart")

	// Scheduled Job Cost Tracking errors
	ErrInvalidScheduledJobStatus = errors.New("invalid status")
	ErrInvalidScheduledJobSchedule = errors.New("invalid schedule")
	ErrScheduledJobWindowStartRequired = errors.New("WindowStart is required")
	ErrScheduledJobWindowEndRequired = errors.New("WindowEnd is required")
	ErrScheduledJobWindowEndBeforeStart = errors.New("WindowEnd must be after WindowStart")
	ErrInvalidScheduledJobPeriod = errors.New("invalid period")

	// Media validation errors
	ErrFileSizeZero = errors.New("FileSize must be greater than 0")
	ErrUnsupportedContentType = errors.New("unsupported content type")
	ErrInvalidMediaStatus = errors.New("invalid media status")
	ErrMediaIDRequired = errors.New("MediaID is required")

	// Media Metadata validation errors
	ErrMediaMetadataIDRequired = errors.New("MediaID is required")
	ErrMediaMetadataInvalidStatus = errors.New("invalid status")
	ErrMediaMetadataWidthNegative = errors.New("width must be non-negative")
	ErrMediaMetadataHeightNegative = errors.New("height must be non-negative")
	ErrMediaMetadataDurationNegative = errors.New("duration must be non-negative")
	ErrMediaMetadataFileSizeNegative = errors.New("file size must be non-negative")

	// DLQ Message validation errors
	ErrDLQIDRequired = errors.New("ID is required")
	ErrDLQOriginalMessageIDRequired = errors.New("OriginalMessageID is required")
	ErrDLQServiceRequired = errors.New("service is required")
	ErrDLQMessageBodyRequired = errors.New("MessageBody is required")
	ErrDLQErrorTypeRequired = errors.New("ErrorType is required")
	ErrDLQErrorMessageRequired = errors.New("ErrorMessage is required")
	ErrDLQInvalidStatus = errors.New("invalid status")
	ErrDLQInvalidPriority = errors.New("invalid priority")

	// Reputation validation errors
	ErrInvalidActorIDFormat = errors.New("invalid actorID format")
	ErrReputationMarshalFailed = errors.New("failed to marshal reputation")
	ErrInvalidReputationJSON = errors.New("invalid reputation JSON")
	ErrReputationUnmarshalFailed = errors.New("failed to unmarshal reputation to map")
	ErrCalculatedAtFieldMissing = errors.New("calculatedAt field not found or not a string")
	ErrCalculatedAtParseFailed = errors.New("failed to parse calculatedAt")
	ErrInvalidReputationDataJSON = errors.New("invalid reputation data JSON")
	ErrReputationDataUnmarshalFailed = errors.New("failed to unmarshal reputation data")

	// CSRF Token validation errors
	ErrCSRFTokenRequired = errors.New("token is required")
	ErrCSRFUserIDRequired = errors.New("UserID is required")
	ErrCSRFExpiresAtRequired = errors.New("ExpiresAt must be set")
	ErrCSRFCreatedAtRequired = errors.New("CreatedAt must be set")
	ErrCSRFInvalidTimeRange = errors.New("ExpiresAt must be after CreatedAt")

	// Notification Delivery validation errors
	ErrNotificationIDRequired = errors.New("notification ID is required")
	ErrDeliveryMethodRequired = errors.New("delivery method is required")
	ErrInvalidDeliveryMethod = errors.New("invalid delivery method")
	ErrInvalidDeliveryStatus = errors.New("invalid delivery status")

	// Session validation errors
	ErrSessionIDGenerationFailed = errors.New("failed to generate session ID")
	ErrAccessTokenGenerationFailed = errors.New("failed to generate access token")
	ErrExpiresAtRequired = errors.New("ExpiresAt must be set")

	// Cost tracking validation errors
	ErrInvalidOperationType = errors.New("invalid operation type")
	ErrInvalidCostPeriod = errors.New("invalid period")
	ErrCostWindowStartRequired = errors.New("WindowStart is required")
	ErrCostWindowEndRequired = errors.New("WindowEnd is required")
	ErrCostWindowEndBeforeStart = errors.New("WindowEnd must be after WindowStart")

	// OAuth Session validation errors
	ErrOAuthSessionIDGenerationFailed = errors.New("failed to generate session ID")
	ErrOAuthCSRFTokenGenerationFailed = errors.New("failed to generate CSRF token")

	// Provider Account validation errors
	ErrProviderIDRequired = errors.New("provider ID is required")
	ErrInvalidProvider = errors.New("invalid provider")
	ErrAccessTokenExpired = errors.New("access token has expired")

	// Conversation validation errors
	ErrConversationIDRequired = errors.New("conversation ID is required")
	ErrConversationDataRequired = errors.New("conversation data is required")

	// Conversation Status validation errors
	ErrConversationStatusIDRequired = errors.New("conversation ID is required")
	ErrConversationStatusUserIDRequired = errors.New("user ID is required")
	ErrConversationStatusStatusIDRequired = errors.New("status ID is required")

	// Media Attachment validation errors
	ErrMediaAttachmentIDRequired = errors.New("MediaID is required")
	ErrMediaAttachmentEntityTypeRequired = errors.New("EntityType is required")
	ErrMediaAttachmentEntityIDRequired = errors.New("EntityID is required")
	ErrMediaAttachmentOrderNegative = errors.New("order must be non-negative")
	ErrMediaAttachmentInvalidEntityType = errors.New("invalid entity type")
	ErrMediaAttachmentInvalidFocalPoint = errors.New("focal point must be in x,y format")

	// Media Spending validation errors
	ErrInvalidMonthlyPeriodFormat = errors.New("invalid monthly period format")
	ErrInvalidDailyPeriodFormat = errors.New("invalid daily period format")
	ErrInvalidPeriodType = errors.New("invalid period type")
	ErrInvalidPeriodTypeValue = errors.New("PeriodType must be 'monthly' or 'daily'")
	ErrNegativeSpendingAmounts = errors.New("spending amounts cannot be negative")
	ErrNegativeCostMicros = errors.New("CostMicros cannot be negative")
	ErrInvalidSpendingCategory = errors.New("invalid category")
	ErrMediaSpendingUserIDRequired = errors.New("UserID is required")
	ErrMediaSpendingPeriodRequired = errors.New("Period is required")
	ErrMediaSpendingTransactionIDRequired = errors.New("TransactionID is required")

	// Alert validation errors
	ErrAlertIDRequired = errors.New("alert_id is required")

	// Webhook Delivery validation errors
	ErrDeliveryIDRequired = errors.New("delivery_id is required")
	ErrWebhookIDRequired = errors.New("webhook_id is required")

	// Dead Letter Message validation errors
	ErrMessageIDRequired = errors.New("message_id is required")

	// Conversation Mute validation errors
	ErrConversationMuteUsernameRequired = errors.New("username is required")
	ErrConversationMuteConversationIDRequired = errors.New("conversation ID is required")

	// Export validation errors
	ErrExportInvalidStartDate = errors.New("invalid start date")
	ErrExportInvalidEndDate = errors.New("invalid end date")

	// Media Job validation errors
	ErrMediaJobIDRequired = errors.New("JobID is required")
	ErrInvalidMediaJobStatus = errors.New("invalid job status")

	// Push Subscription validation errors
	ErrPushSubscriptionP256dhRequired = errors.New("p256dh public key is required")
	ErrPushSubscriptionAuthRequired = errors.New("auth secret is required")

	// Block validation errors
	ErrBlockUpdateKeysFailed = errors.New("failed to update block keys")

	// Mute validation errors
	ErrMuteUpdateKeysFailed = errors.New("failed to update mute keys")

	// CloudWatch Metrics validation errors
	ErrCloudWatchMetricServiceNameRequired = errors.New("ServiceName is required")

	// Notification validation errors
	ErrInvalidNotificationType = errors.New("invalid notification type")
)