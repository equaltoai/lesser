package streaming

import "github.com/equaltoai/lesser/pkg/errors"

// Media metadata errors
var (
	ErrMediaMetadataNotFound    = errors.NewAppError(errors.CodeNotFound, errors.CategoryMedia, "media metadata not found")
	ErrGetMetadataFromDynamoDB  = errors.FailedToGet("metadata from DynamoDB", nil)
	ErrCreateMetadataInDynamoDB = errors.FailedToCreate("metadata in DynamoDB", nil)
	ErrUpdateMetadataInDynamoDB = errors.FailedToUpdate("metadata in DynamoDB", nil)
)

// S3 operation errors
var (
	ErrCheckManifestExists        = errors.ProcessingFailed("check manifest exists", nil)
	ErrSaveManifestToS3           = errors.ProcessingFailed("save manifest to S3", nil)
	ErrGetSegmentInfo             = errors.FailedToGet("segment info", nil)
	ErrListSegments               = errors.FailedToList("segments", nil)
	ErrCreateIndexForQuality      = errors.ProcessingFailed("create index for quality", nil)
	ErrCreateSegmentsDirectory    = errors.ProcessingFailed("create segments directory for quality", nil)
	ErrCreateMasterIndex          = errors.ProcessingFailed("create master index", nil)
	ErrGeneratePresignedUploadURL = errors.ProcessingFailed("generate presigned upload URL", nil)
)

// CloudFront configuration errors
var (
	ErrCloudFrontNotConfigured              = errors.ConfigurationMissing("cloudfront_config")
	ErrInvalidCloudFrontPrivateKeyPath      = errors.ConfigurationInvalid("cloudfront_private_key_path", "invalid path")
	ErrFailedToReadCloudFrontPrivateKeyFile = errors.ProcessingFailed("CloudFront private key file read", nil)
	ErrCloudFrontPrivateKeyNotProvided      = errors.ConfigurationMissing("cloudfront_private_key")
	ErrInvalidRSAPrivateKeyPEM              = errors.InvalidPrivateKeyType()
	ErrFailedToParseRSAPrivateKey           = errors.ProcessingFailed("RSA private key parsing", nil)
)

// Keyframe data errors
var (
	ErrFailedToGetKeyframeData    = errors.FailedToGet("keyframe data", nil)
	ErrFailedToReadIFramePlaylist = errors.ProcessingFailed("I-frame playlist read", nil)
	ErrFailedToReadKeyframeData   = errors.ProcessingFailed("keyframe data read", nil)
)

// Session management errors
var (
	ErrCreateSession          = errors.FailedToCreate("session", nil)
	ErrGetSession             = errors.FailedToGet("session", nil)
	ErrUpdateSession          = errors.FailedToUpdate("session", nil)
	ErrEndSession             = errors.ProcessingFailed("end session", nil)
	ErrQueryUserSessions      = errors.FailedToQuery("user sessions", nil)
	ErrScanMediaSessions      = errors.FailedToQuery("media sessions", nil)
	ErrCleanupExpiredSessions = errors.ProcessingFailed("cleanup expired sessions", nil)
)
