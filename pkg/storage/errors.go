package storage

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

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
	ErrPlanUpgradeFailed          = errors.PlanUpgradeFailed(stdErrors.New("plan upgrade failed"))
	ErrUserIDRequired             = errors.UserIDRequired()

	// Enhanced Pattern Repository specific errors
	ErrPatternNotFound               = errors.PatternNotFound()
	ErrPatternSaveFailed             = errors.PatternSaveFailed(stdErrors.New("pattern save failed"))
	ErrPatternCreateFailed           = errors.PatternCreateFailed(stdErrors.New("pattern create failed"))
	ErrPatternUpdateFailed           = errors.PatternUpdateFailed(stdErrors.New("pattern update failed"))
	ErrPatternDeleteFailed           = errors.PatternDeleteFailed(stdErrors.New("pattern delete failed"))
	ErrPatternQueryFailed            = errors.PatternQueryFailed(stdErrors.New("pattern query failed"))
	ErrPatternCacheNotFound          = errors.PatternCacheNotFound()
	ErrPatternCacheCreateFailed      = errors.PatternCacheCreateFailed(stdErrors.New("pattern cache create failed"))
	ErrPatternCacheUpdateFailed      = errors.PatternCacheUpdateFailed(stdErrors.New("pattern cache update failed"))
	ErrPatternMetricsCreateFailed    = errors.PatternMetricsCreateFailed(stdErrors.New("pattern metrics create failed"))
	ErrPatternMetricsUpdateFailed    = errors.PatternMetricsUpdateFailed(stdErrors.New("pattern metrics update failed"))
	ErrPatternTestResultCreateFailed = errors.PatternTestResultCreateFailed(stdErrors.New("pattern test result create failed"))
	ErrPatternTestResultQueryFailed  = errors.PatternTestResultQueryFailed(stdErrors.New("pattern test result query failed"))
	ErrPatternTestResultNotFound     = errors.PatternTestResultNotFound()
	ErrPatternMetricsQueryFailed     = errors.PatternMetricsQueryFailed(stdErrors.New("pattern metrics query failed"))
	ErrPatternAnalysisFailed         = errors.PatternAnalysisFailed(stdErrors.New("pattern analysis failed"))
	ErrPatternValidationFailed       = errors.PatternValidationFailed("")
	ErrDatabaseConnectionFailed      = errors.DatabaseConnectionFailed(stdErrors.New("database connection failed"))
	ErrNilPattern                    = errors.NilPattern()
	ErrNilPatternCache               = errors.NilPatternCache()
	ErrNilPatternMetric              = errors.NilPatternMetric()
	ErrNilPatternTestResult          = errors.NilPatternTestResult()
)
