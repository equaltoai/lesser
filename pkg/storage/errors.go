package storage

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// ErrNotFound is returned when a requested item doesn't exist
	ErrNotFound = errors.NotFound("item")

	// ErrAlreadyExists is returned when trying to create an item that already exists
	ErrAlreadyExists = errors.AlreadyExists("item")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.ValidationFailedWithField("invalid input")

	// ErrUnauthorized is returned when an operation is not authorized
	ErrUnauthorized = errors.AccessDenied("")

	// ErrRateLimited is returned when rate limit is exceeded
	ErrRateLimited = errors.RateLimitExceededGeneric("")

	// ErrInvalidRefreshToken is returned when a refresh token is invalid
	ErrInvalidRefreshToken = errors.RefreshTokenInvalid()

	// ErrExpiredRefreshToken is returned when a refresh token has expired
	ErrExpiredRefreshToken = errors.RefreshTokenExpired()

	// ErrTokenReuse is returned when refresh token reuse is detected (security breach)
	ErrTokenReuse = errors.TokenReuse()

	// User Media Config errors
	ErrInvalidPlanTier            = errors.InvalidPlanTier()
	ErrInvalidFileSize            = errors.InvalidFileSize()
	ErrFileSizeTooLarge           = errors.FileSizeExceedsLimit(0, 0)
	ErrVideoDurationInvalid       = errors.VideoDurationInvalid()
	ErrUploadLimitsInvalid        = errors.UploadLimitsInvalid()
	ErrBudgetLimitsInvalid        = errors.BudgetLimitsInvalid()
	ErrModerationThresholdInvalid = errors.ModerationThresholdInvalid()
	ErrInvalidQualitySetting      = errors.InvalidQualitySetting()
	ErrPlanUpgradeFailed          = errors.PlanUpgradeFailed(nil)
	ErrUserIDRequired             = errors.UserIDRequired()

	// Enhanced Pattern Repository specific errors
	ErrPatternNotFound               = errors.PatternNotFound()
	ErrPatternSaveFailed             = errors.PatternSaveFailed(nil)
	ErrPatternCreateFailed           = errors.PatternCreateFailed(nil)
	ErrPatternUpdateFailed           = errors.PatternUpdateFailed(nil)
	ErrPatternDeleteFailed           = errors.PatternDeleteFailed(nil)
	ErrPatternQueryFailed            = errors.PatternQueryFailed(nil)
	ErrPatternCacheNotFound          = errors.PatternCacheNotFound()
	ErrPatternCacheCreateFailed      = errors.PatternCacheCreateFailed(nil)
	ErrPatternCacheUpdateFailed      = errors.PatternCacheUpdateFailed(nil)
	ErrPatternMetricsCreateFailed    = errors.PatternMetricsCreateFailed(nil)
	ErrPatternMetricsUpdateFailed    = errors.PatternMetricsUpdateFailed(nil)
	ErrPatternTestResultCreateFailed = errors.PatternTestResultCreateFailed(nil)
	ErrPatternTestResultQueryFailed  = errors.PatternTestResultQueryFailed(nil)
	ErrPatternTestResultNotFound     = errors.PatternTestResultNotFound()
	ErrPatternMetricsQueryFailed     = errors.PatternMetricsQueryFailed(nil)
	ErrPatternAnalysisFailed         = errors.PatternAnalysisFailed(nil)
	ErrPatternValidationFailed       = errors.PatternValidationFailed("")
	ErrDatabaseConnectionFailed      = errors.DatabaseConnectionFailed(nil)
	ErrNilPattern                    = errors.NilPattern()
	ErrNilPatternCache               = errors.NilPatternCache()
	ErrNilPatternMetric              = errors.NilPatternMetric()
	ErrNilPatternTestResult          = errors.NilPatternTestResult()
)
