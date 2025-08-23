package storage

import "errors"

// Common storage errors
var (
	// ErrNotFound is returned when a requested item doesn't exist
	ErrNotFound = errors.New("item not found")

	// ErrAlreadyExists is returned when trying to create an item that already exists
	ErrAlreadyExists = errors.New("item already exists")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized is returned when an operation is not authorized
	ErrUnauthorized = errors.New("unauthorized")

	// ErrRateLimited is returned when rate limit is exceeded
	ErrRateLimited = errors.New("rate limit exceeded")
	
	// ErrInvalidRefreshToken is returned when a refresh token is invalid
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	
	// ErrExpiredRefreshToken is returned when a refresh token has expired
	ErrExpiredRefreshToken = errors.New("refresh token expired")
	
	// ErrTokenReuse is returned when refresh token reuse is detected (security breach)
	ErrTokenReuse = errors.New("refresh token reuse detected")

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

	// Enhanced Pattern Repository specific errors
	ErrPatternNotFound = errors.New("enhanced moderation pattern not found")
	ErrPatternSaveFailed = errors.New("failed to save enhanced moderation pattern")
	ErrPatternCreateFailed = errors.New("failed to create enhanced moderation pattern")
	ErrPatternUpdateFailed = errors.New("failed to update enhanced moderation pattern")
	ErrPatternDeleteFailed = errors.New("failed to delete enhanced moderation pattern")
	ErrPatternQueryFailed = errors.New("failed to query enhanced moderation patterns")
	ErrPatternCacheNotFound = errors.New("pattern cache entry not found")
	ErrPatternCacheCreateFailed = errors.New("failed to create pattern cache entry")
	ErrPatternCacheUpdateFailed = errors.New("failed to update pattern cache entry")
	ErrPatternMetricsCreateFailed = errors.New("failed to create pattern performance metrics")
	ErrPatternMetricsUpdateFailed = errors.New("failed to update pattern performance metrics")
	ErrPatternTestResultCreateFailed = errors.New("failed to create pattern test result")
	ErrPatternTestResultQueryFailed = errors.New("failed to query pattern test results")
	ErrPatternTestResultNotFound = errors.New("pattern test result not found")
	ErrPatternMetricsQueryFailed = errors.New("failed to query pattern performance metrics")
	ErrPatternAnalysisFailed = errors.New("pattern analysis failed")
	ErrPatternValidationFailed = errors.New("pattern validation failed")
	ErrDatabaseConnectionFailed = errors.New("database connection failed")
	ErrNilPattern = errors.New("pattern cannot be nil")
	ErrNilPatternCache = errors.New("pattern cache cannot be nil")
	ErrNilPatternMetric = errors.New("pattern metric cannot be nil")
	ErrNilPatternTestResult = errors.New("pattern test result cannot be nil")
)
