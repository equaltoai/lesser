package media

import "errors"

// CloudWatch Enhanced Streaming errors
var (
	ErrNoQualityMetrics    = errors.New("no quality metrics available")
	ErrNoGeographicMetrics = errors.New("no geographic metrics available")
	ErrGetMetric           = errors.New("failed to get metric")
	ErrNoDataPoints        = errors.New("no data points for metric")
	ErrGetMetricWithDim    = errors.New("failed to get metric with dimension")
	ErrNoDataPointsWithDim = errors.New("no data points for metric with dimension")
)

// Analytics errors
var (
	ErrAWSConfigLoad            = errors.New("failed to load AWS config")
	ErrLogFilesListing          = errors.New("failed to list log files")
	ErrLogFileRetrieval         = errors.New("failed to get log file")
	ErrMetricDataSubmission     = errors.New("failed to put metric data")
	ErrBandwidthUsageRetrieval  = errors.New("failed to get bandwidth usage")
	ErrBandwidthUsageStorage    = errors.New("failed to store bandwidth usage")
	ErrRealtimeMetricSubmission = errors.New("failed to send realtime metric")
)

// Video metadata parsing errors
var (
	ErrMoovAtomParseFailed        = errors.New("failed to parse moov atom")
	ErrAtomSizeTooLarge           = errors.New("atom size too large")
	ErrInvalidAtomSize            = errors.New("invalid atom size")
	ErrAtomExtendsFile            = errors.New("atom extends beyond file")
	ErrMvhdAtomParseFailed        = errors.New("failed to parse mvhd")
	ErrUnsupportedMvhdVersion     = errors.New("unsupported mvhd version")
	ErrTkhdAtomParseFailed        = errors.New("failed to parse tkhd")
	ErrUnsupportedTkhdVersion     = errors.New("unsupported tkhd version")
	ErrMoovAtomNotFound           = errors.New("moov atom not found")
	ErrExtendedSizeIncomplete     = errors.New("incomplete extended size atom")
	ErrMvhdAtomTooSmall           = errors.New("mvhd atom too small")
	ErrMvhdV0AtomIncomplete       = errors.New("mvhd v0 atom incomplete")
	ErrMvhdV1AtomIncomplete       = errors.New("mvhd v1 atom incomplete")
	ErrTkhdAtomTooSmall           = errors.New("tkhd atom too small")
	ErrTkhdV0AtomIncomplete       = errors.New("tkhd v0 atom incomplete")
	ErrTkhdV1AtomIncomplete       = errors.New("tkhd v1 atom incomplete")
	ErrHdlrAtomTooSmall           = errors.New("hdlr atom too small")
	ErrStsdAtomTooSmall           = errors.New("stsd atom too small")
	ErrStsdEntryIncomplete        = errors.New("stsd entry incomplete")
	ErrVideoMetadataParsingFailed = errors.New("video metadata parsing failed, populated with fallback values")
)

// Streaming service errors
var (
	ErrSignURL                = errors.New("failed to sign URL")
	ErrInvalidURL             = errors.New("invalid URL")
	ErrCreateSignature        = errors.New("failed to create signature")
	ErrRecordStreamingEvent   = errors.New("failed to record streaming event")
	ErrCreateSession          = errors.New("failed to create session")
	ErrGetSession             = errors.New("failed to get session")
	ErrUpdateSession          = errors.New("failed to update session")
	ErrEndSession             = errors.New("failed to end session")
	ErrQueryUserSessions      = errors.New("failed to query user sessions")
	ErrScanMediaSessions      = errors.New("failed to scan media sessions")
	ErrCleanupExpiredSessions = errors.New("failed to cleanup expired sessions")
)

// Blurhash errors
var (
	ErrBlurhashEncode = errors.New("failed to encode blurhash")
	ErrImageDecode    = errors.New("failed to decode image")
	ErrBlurhashDecode = errors.New("failed to decode blurhash")
)

// Image processing errors
var (
	ErrImageDecodeProcess = errors.New("failed to decode image")
	ErrImageEncode        = errors.New("failed to encode image")
)
