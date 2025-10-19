package models

import (
	stdErrors "errors"

	centralErrors "github.com/equaltoai/lesser/pkg/errors"
)

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// User Media Config errors
	ErrInvalidPlanTier            = centralErrors.InvalidPlanTier()
	ErrInvalidFileSize            = centralErrors.InvalidFileSize()
	ErrFileSizeTooLarge           = centralErrors.FileSizeExceedsLimit(0, 0)
	ErrVideoDurationInvalid       = centralErrors.VideoDurationInvalid()
	ErrUploadLimitsInvalid        = centralErrors.UploadLimitsInvalid()
	ErrBudgetLimitsInvalid        = centralErrors.BudgetLimitsInvalid()
	ErrModerationThresholdInvalid = centralErrors.ModerationThresholdInvalid()
	ErrInvalidQualitySetting      = centralErrors.InvalidQualitySetting()
	ErrPlanUpgradeFailed          = centralErrors.PlanUpgradeFailed(stdErrors.New("plan upgrade failed"))
	ErrUserIDRequired             = centralErrors.UserIDRequired()

	// Relay Cost errors
	ErrRelayWindowStartRequired = centralErrors.ValidationFailedWithField("window start is required")
	ErrInvalidBudgetLimit       = centralErrors.ValidationFailedWithField("limit_micro_cents must be positive")

	// Transcoding Job errors
	ErrTranscodingJobIDRequired   = centralErrors.RequiredFieldMissing("transcoding job ID")
	ErrTranscodingMediaIDRequired = centralErrors.RequiredFieldMissing("transcoding media ID")
	ErrTranscodingUserIDRequired  = centralErrors.RequiredFieldMissing("transcoding user ID")
	ErrInvalidJobType             = centralErrors.NewValidationError("job_type", "Invalid job type")
	ErrInvalidJobStatus           = centralErrors.NewValidationError("job_status", "Invalid job status")
	ErrNegativeSize               = centralErrors.ValueOutOfRange("size", 0, int64(^uint64(0)>>1), -1)
	ErrNegativeCost               = centralErrors.ValueOutOfRange("cost", 0, int64(^uint64(0)>>1), -1)

	// Metrics validation errors
	ErrMetricIDRequired          = centralErrors.RequiredFieldMissing("ID")
	ErrMetricTypeRequired        = centralErrors.RequiredFieldMissing("type")
	ErrMetricServiceRequired     = centralErrors.RequiredFieldMissing("service")
	ErrInvalidMetricType         = centralErrors.NewValidationError("metric_type", "Invalid metric type")
	ErrInvalidPeriod             = centralErrors.NewValidationError("period", "Invalid period")
	ErrMetricWindowStartRequired = centralErrors.RequiredFieldMissing("WindowStart")
	ErrMetricWindowEndRequired   = centralErrors.RequiredFieldMissing("WindowEnd")
	ErrWindowEndBeforeStart      = centralErrors.NewValidationError("window_end", "WindowEnd must be after WindowStart")
	ErrTimestampRequired         = centralErrors.RequiredFieldMissing("timestamp")
	ErrAggregationLevelRequired  = centralErrors.RequiredFieldMissing("AggregationLevel")
	ErrInvalidAggregationLevel   = centralErrors.NewValidationError("aggregation_level", "Invalid aggregation level")
	ErrFailedToUpdateKeys        = centralErrors.FailedToUpdate("keys", stdErrors.New("failed to update keys"))

	// MetricRecord validation errors
	ErrMetricRecordTypeRequired    = centralErrors.RequiredFieldMissing("MetricType")
	ErrMetricRecordServiceRequired = centralErrors.RequiredFieldMissing("ServiceName")

	// WebSocket Cost Tracking errors
	ErrInvalidWebSocketOperationType = centralErrors.NewValidationError("operation_type", "Invalid operation type")
	ErrInvalidWebSocketPeriod        = centralErrors.NewValidationError("period", "Invalid period")
	ErrBudgetMicroCentsNegative      = centralErrors.ValueOutOfRange("BudgetMicroCents", 0, int64(^uint64(0)>>1), -1)
	ErrWebSocketWindowStartRequired  = centralErrors.RequiredFieldMissing("WindowStart")
	ErrWebSocketWindowEndRequired    = centralErrors.RequiredFieldMissing("WindowEnd")
	ErrWebSocketWindowEndBeforeStart = centralErrors.NewValidationError("window_end", "WindowEnd must be after WindowStart")

	// Scheduled Job Cost Tracking errors
	ErrInvalidScheduledJobStatus        = centralErrors.NewValidationError("status", "Invalid status")
	ErrInvalidScheduledJobSchedule      = centralErrors.NewValidationError("schedule", "Invalid schedule")
	ErrScheduledJobWindowStartRequired  = centralErrors.RequiredFieldMissing("WindowStart")
	ErrScheduledJobWindowEndRequired    = centralErrors.RequiredFieldMissing("WindowEnd")
	ErrScheduledJobWindowEndBeforeStart = centralErrors.NewValidationError("window_end", "WindowEnd must be after WindowStart")
	ErrInvalidScheduledJobPeriod        = centralErrors.NewValidationError("period", "Invalid period")

	// Media validation errors
	ErrFileSizeZero           = centralErrors.ValueOutOfRange("FileSize", 1, int64(^uint64(0)>>1), 0)
	ErrUnsupportedContentType = centralErrors.ContentTypeNotAllowed("unknown")
	ErrInvalidMediaStatus     = centralErrors.NewValidationError("media_status", "Invalid media status")
	ErrInvalidMediaCategory   = centralErrors.NewValidationError("media_category", "Invalid media category")
	ErrMediaIDRequired        = centralErrors.RequiredFieldMissing("MediaID")

	// Media Metadata validation errors
	ErrMediaMetadataIDRequired       = centralErrors.RequiredFieldMissing("MediaID")
	ErrMediaMetadataInvalidStatus    = centralErrors.NewValidationError("status", "Invalid status")
	ErrMediaMetadataWidthNegative    = centralErrors.ValueOutOfRange("width", 0, int64(^uint64(0)>>1), -1)
	ErrMediaMetadataHeightNegative   = centralErrors.ValueOutOfRange("height", 0, int64(^uint64(0)>>1), -1)
	ErrMediaMetadataDurationNegative = centralErrors.ValueOutOfRange("duration", 0, int64(^uint64(0)>>1), -1)
	ErrMediaMetadataFileSizeNegative = centralErrors.ValueOutOfRange("file_size", 0, int64(^uint64(0)>>1), -1)

	// DLQ Message validation errors
	ErrDLQIDRequired                = centralErrors.RequiredFieldMissing("ID")
	ErrDLQOriginalMessageIDRequired = centralErrors.RequiredFieldMissing("OriginalMessageID")
	ErrDLQServiceRequired           = centralErrors.RequiredFieldMissing("service")
	ErrDLQMessageBodyRequired       = centralErrors.RequiredFieldMissing("MessageBody")
	ErrDLQErrorTypeRequired         = centralErrors.RequiredFieldMissing("ErrorType")
	ErrDLQErrorMessageRequired      = centralErrors.RequiredFieldMissing("ErrorMessage")
	ErrDLQInvalidStatus             = centralErrors.NewValidationError("status", "Invalid status")
	ErrDLQInvalidPriority           = centralErrors.NewValidationError("priority", "Invalid priority")

	// Reputation validation errors
	ErrInvalidActorIDFormat          = centralErrors.InvalidFormat("actorID", "valid actor ID format")
	ErrReputationMarshalFailed       = centralErrors.MarshalingFailed("reputation", stdErrors.New("reputation marshaling failed"))
	ErrInvalidReputationJSON         = centralErrors.JSONFormatInvalid("invalid reputation structure")
	ErrReputationUnmarshalFailed     = centralErrors.UnmarshalingFailed("reputation to map", stdErrors.New("reputation unmarshaling failed"))
	ErrCalculatedAtFieldMissing      = centralErrors.RequiredFieldMissing("calculatedAt")
	ErrCalculatedAtParseFailed       = centralErrors.ParsingFailed("calculatedAt", stdErrors.New("calculatedAt parsing failed"))
	ErrInvalidReputationDataJSON     = centralErrors.JSONFormatInvalid("invalid reputation data structure")
	ErrReputationDataUnmarshalFailed = centralErrors.UnmarshalingFailed("reputation data", stdErrors.New("reputation data unmarshaling failed"))

	// CSRF Token validation errors
	ErrCSRFTokenRequired     = centralErrors.RequiredFieldMissing("token")
	ErrCSRFUserIDRequired    = centralErrors.RequiredFieldMissing("UserID")
	ErrCSRFExpiresAtRequired = centralErrors.RequiredFieldMissing("ExpiresAt")
	ErrCSRFCreatedAtRequired = centralErrors.RequiredFieldMissing("CreatedAt")
	ErrCSRFInvalidTimeRange  = centralErrors.NewValidationError("expires_at", "ExpiresAt must be after CreatedAt")

	// Notification Delivery validation errors
	ErrNotificationIDRequired = centralErrors.RequiredFieldMissing("notification ID")
	ErrDeliveryMethodRequired = centralErrors.RequiredFieldMissing("delivery method")
	ErrInvalidDeliveryMethod  = centralErrors.NewValidationError("delivery_method", "Invalid delivery method")
	ErrInvalidDeliveryStatus  = centralErrors.NewValidationError("delivery_status", "Invalid delivery status")

	// Session validation errors
	ErrSessionIDGenerationFailed   = centralErrors.SessionIDGenerationFailed(stdErrors.New("session ID generation failed"))
	ErrAccessTokenGenerationFailed = centralErrors.AccessTokenGenerationFailed(stdErrors.New("access token generation failed"))
	ErrExpiresAtRequired           = centralErrors.RequiredFieldMissing("ExpiresAt")

	// Cost tracking validation errors
	ErrInvalidOperationType     = centralErrors.NewValidationError("operation_type", "Invalid operation type")
	ErrInvalidCostPeriod        = centralErrors.NewValidationError("period", "Invalid period")
	ErrCostWindowStartRequired  = centralErrors.RequiredFieldMissing("WindowStart")
	ErrCostWindowEndRequired    = centralErrors.RequiredFieldMissing("WindowEnd")
	ErrCostWindowEndBeforeStart = centralErrors.NewValidationError("window_end", "WindowEnd must be after WindowStart")

	// OAuth Session validation errors
	ErrOAuthSessionIDGenerationFailed = centralErrors.SessionIDGenerationFailed(stdErrors.New("OAuth session ID generation failed"))
	ErrOAuthCSRFTokenGenerationFailed = centralErrors.CSRFTokenGenerationFailed(stdErrors.New("OAuth CSRF token generation failed"))

	// Provider Account validation errors
	ErrProviderIDRequired = centralErrors.RequiredFieldMissing("provider ID")
	ErrInvalidProvider    = centralErrors.NewValidationError("provider", "Invalid provider")
	ErrAccessTokenExpired = centralErrors.NewAppError(centralErrors.CodeTokenExpired, centralErrors.CategoryAuth, "Access token has expired")

	// Conversation validation errors
	ErrConversationIDRequired   = centralErrors.RequiredFieldMissing("conversation ID")
	ErrConversationDataRequired = centralErrors.RequiredFieldMissing("conversation data")

	// Conversation Status validation errors
	ErrConversationStatusIDRequired       = centralErrors.RequiredFieldMissing("conversation ID")
	ErrConversationStatusUserIDRequired   = centralErrors.RequiredFieldMissing("user ID")
	ErrConversationStatusStatusIDRequired = centralErrors.RequiredFieldMissing("status ID")

	// Media Attachment validation errors
	ErrMediaAttachmentIDRequired         = centralErrors.RequiredFieldMissing("MediaID")
	ErrMediaAttachmentEntityTypeRequired = centralErrors.RequiredFieldMissing("EntityType")
	ErrMediaAttachmentEntityIDRequired   = centralErrors.RequiredFieldMissing("EntityID")
	ErrMediaAttachmentOrderNegative      = centralErrors.ValueOutOfRange("order", 0, int64(^uint64(0)>>1), -1)
	ErrMediaAttachmentInvalidEntityType  = centralErrors.NewValidationError("entity_type", "Invalid entity type")
	ErrMediaAttachmentInvalidFocalPoint  = centralErrors.InvalidFormat("focal_point", "x,y format")

	// Media Spending validation errors
	ErrInvalidMonthlyPeriodFormat         = centralErrors.InvalidFormat("monthly_period", "YYYY-MM format")
	ErrInvalidDailyPeriodFormat           = centralErrors.InvalidFormat("daily_period", "YYYY-MM-DD format")
	ErrInvalidPeriodType                  = centralErrors.NewValidationError("period_type", "Invalid period type")
	ErrInvalidPeriodTypeValue             = centralErrors.NewValidationError("period_type", "PeriodType must be 'monthly' or 'daily'")
	ErrNegativeSpendingAmounts            = centralErrors.ValueOutOfRange("spending_amounts", 0, int64(^uint64(0)>>1), -1)
	ErrNegativeCostMicros                 = centralErrors.ValueOutOfRange("CostMicros", 0, int64(^uint64(0)>>1), -1)
	ErrInvalidSpendingCategory            = centralErrors.NewValidationError("category", "Invalid category")
	ErrMediaSpendingUserIDRequired        = centralErrors.RequiredFieldMissing("UserID")
	ErrMediaSpendingPeriodRequired        = centralErrors.RequiredFieldMissing("period")
	ErrMediaSpendingTransactionIDRequired = centralErrors.RequiredFieldMissing("TransactionID")

	// Alert validation errors
	ErrAlertIDRequired = centralErrors.RequiredFieldMissing("alert_id")

	// Webhook Delivery validation errors
	ErrDeliveryIDRequired = centralErrors.RequiredFieldMissing("delivery_id")
	ErrWebhookIDRequired  = centralErrors.RequiredFieldMissing("webhook_id")

	// Dead Letter Message validation errors
	ErrMessageIDRequired = centralErrors.RequiredFieldMissing("message_id")

	// Conversation Mute validation errors
	ErrConversationMuteUsernameRequired       = centralErrors.RequiredFieldMissing("username")
	ErrConversationMuteConversationIDRequired = centralErrors.RequiredFieldMissing("conversation ID")

	// Export validation errors
	ErrExportInvalidStartDate = centralErrors.InvalidFormat("start_date", "valid date format")
	ErrExportInvalidEndDate   = centralErrors.InvalidFormat("end_date", "valid date format")

	// Media Job validation errors
	ErrMediaJobIDRequired    = centralErrors.RequiredFieldMissing("JobID")
	ErrInvalidMediaJobStatus = centralErrors.NewValidationError("job_status", "Invalid job status")

	// Push Subscription validation errors
	ErrPushSubscriptionP256dhRequired = centralErrors.RequiredFieldMissing("p256dh public key")
	ErrPushSubscriptionAuthRequired   = centralErrors.RequiredFieldMissing("auth secret")

	// Block validation errors
	ErrBlockUpdateKeysFailed = centralErrors.FailedToUpdate("block keys", stdErrors.New("failed to update block keys"))

	// Mute validation errors
	ErrMuteUpdateKeysFailed = centralErrors.FailedToUpdate("mute keys", stdErrors.New("failed to update mute keys"))

	// CloudWatch Metrics validation errors
	ErrCloudWatchMetricServiceNameRequired = centralErrors.RequiredFieldMissing("ServiceName")

	// Notification validation errors
	ErrInvalidNotificationType = centralErrors.NewValidationError("notification_type", "Invalid notification type")
)
