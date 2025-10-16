package media

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// CloudWatch Enhanced Streaming errors
var (
	ErrNoQualityMetrics    = errors.InsufficientHistoricalData(1)
	ErrNoGeographicMetrics = errors.InsufficientHistoricalData(1)
	ErrGetMetric           = errors.FailedToGet("metric", stdErrors.New("failed to get metric"))
	ErrNoDataPoints        = errors.InsufficientHistoricalData(1)
	ErrGetMetricWithDim    = errors.FailedToGet("metric with dimension", stdErrors.New("failed to get metric with dimension"))
	ErrNoDataPointsWithDim = errors.InsufficientHistoricalData(1)
)

// Analytics errors
var (
	ErrAWSConfigLoad            = errors.ConnectionFailed("AWS config", stdErrors.New("failed to load AWS config"))
	ErrLogFilesListing          = errors.FailedToList("log files", stdErrors.New("failed to list log files"))
	ErrLogFileRetrieval         = errors.FailedToGet("log file", stdErrors.New("failed to retrieve log file"))
	ErrMetricDataSubmission     = errors.ProcessingFailed("metric data submission", stdErrors.New("metric data submission failed"))
	ErrBandwidthUsageRetrieval  = errors.FailedToGet("bandwidth usage", stdErrors.New("failed to retrieve bandwidth usage"))
	ErrBandwidthUsageStorage    = errors.FailedToStore("bandwidth usage", stdErrors.New("failed to store bandwidth usage"))
	ErrRealtimeMetricSubmission = errors.ProcessingFailed("realtime metric submission", stdErrors.New("realtime metric submission failed"))
)

// Video metadata parsing errors
var (
	ErrMoovAtomParseFailed        = errors.NewValidationError("validation", "failed to parse moov atom")
	ErrAtomSizeTooLarge           = errors.NewValidationError("validation", "atom size too large")
	ErrInvalidAtomSize            = errors.NewValidationError("validation", "invalid atom size")
	ErrAtomExtendsFile            = errors.NewValidationError("validation", "atom extends beyond file")
	ErrMvhdAtomParseFailed        = errors.NewValidationError("validation", "failed to parse mvhd")
	ErrUnsupportedMvhdVersion     = errors.NewValidationError("validation", "unsupported mvhd version")
	ErrTkhdAtomParseFailed        = errors.NewValidationError("validation", "failed to parse tkhd")
	ErrUnsupportedTkhdVersion     = errors.NewValidationError("validation", "unsupported tkhd version")
	ErrMoovAtomNotFound           = errors.NewValidationError("validation", "moov atom not found")
	ErrExtendedSizeIncomplete     = errors.NewValidationError("validation", "incomplete extended size atom")
	ErrMvhdAtomTooSmall           = errors.NewValidationError("validation", "mvhd atom too small")
	ErrMvhdV0AtomIncomplete       = errors.NewValidationError("validation", "mvhd v0 atom incomplete")
	ErrMvhdV1AtomIncomplete       = errors.NewValidationError("validation", "mvhd v1 atom incomplete")
	ErrTkhdAtomTooSmall           = errors.NewValidationError("validation", "tkhd atom too small")
	ErrTkhdV0AtomIncomplete       = errors.NewValidationError("validation", "tkhd v0 atom incomplete")
	ErrTkhdV1AtomIncomplete       = errors.NewValidationError("validation", "tkhd v1 atom incomplete")
	ErrHdlrAtomTooSmall           = errors.NewValidationError("validation", "hdlr atom too small")
	ErrStsdAtomTooSmall           = errors.NewValidationError("validation", "stsd atom too small")
	ErrStsdEntryIncomplete        = errors.NewValidationError("validation", "stsd entry incomplete")
	ErrVideoMetadataParsingFailed = errors.NewValidationError("validation", "video metadata parsing failed, populated with fallback values")
)

// Streaming service errors
var (
	ErrSignURL                = errors.NewValidationError("validation", "failed to sign URL")
	ErrInvalidURL             = errors.NewValidationError("validation", "invalid URL")
	ErrCreateSignature        = errors.NewValidationError("validation", "failed to create signature")
	ErrRecordStreamingEvent   = errors.NewValidationError("validation", "failed to record streaming event")
	ErrCreateSession          = errors.NewValidationError("validation", "failed to create session")
	ErrGetSession             = errors.NewValidationError("validation", "failed to get session")
	ErrUpdateSession          = errors.NewValidationError("validation", "failed to update session")
	ErrEndSession             = errors.NewValidationError("validation", "failed to end session")
	ErrQueryUserSessions      = errors.NewValidationError("validation", "failed to query user sessions")
	ErrScanMediaSessions      = errors.NewValidationError("validation", "failed to scan media sessions")
	ErrCleanupExpiredSessions = errors.NewValidationError("validation", "failed to cleanup expired sessions")
)

// Blurhash errors
var (
	ErrBlurhashEncode = errors.NewValidationError("validation", "failed to encode blurhash")
	ErrImageDecode    = errors.NewValidationError("validation", "failed to decode image")
	ErrBlurhashDecode = errors.NewValidationError("validation", "failed to decode blurhash")
)

// Image processing errors
var (
	ErrImageDecodeProcess = errors.NewValidationError("validation", "failed to decode image")
	ErrImageEncode        = errors.NewValidationError("validation", "failed to encode image")
)
