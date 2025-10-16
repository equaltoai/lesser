package streaming

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Media metadata errors
var (
	ErrMediaMetadataNotFound    = errors.NewAppError(errors.CodeNotFound, errors.CategoryMedia, "media metadata not found")
	ErrGetMetadataFromDynamoDB  = errors.FailedToGet("metadata from DynamoDB", stdErrors.New("failed to get metadata from DynamoDB"))
	ErrCreateMetadataInDynamoDB = errors.FailedToCreate("metadata in DynamoDB", stdErrors.New("failed to create metadata in DynamoDB"))
	ErrUpdateMetadataInDynamoDB = errors.FailedToUpdate("metadata in DynamoDB", stdErrors.New("failed to update metadata in DynamoDB"))
)

// S3 operation errors
var (
	ErrCheckManifestExists        = errors.ProcessingFailed("check manifest exists", stdErrors.New("failed to check manifest existence"))
	ErrSaveManifestToS3           = errors.ProcessingFailed("save manifest to S3", stdErrors.New("failed to save manifest to S3"))
	ErrGetSegmentInfo             = errors.FailedToGet("segment info", stdErrors.New("failed to get segment info"))
	ErrListSegments               = errors.FailedToList("segments", stdErrors.New("failed to list segments"))
	ErrCreateIndexForQuality      = errors.ProcessingFailed("create index for quality", stdErrors.New("failed to create index for quality"))
	ErrCreateSegmentsDirectory    = errors.ProcessingFailed("create segments directory for quality", stdErrors.New("failed to create segments directory for quality"))
	ErrCreateMasterIndex          = errors.ProcessingFailed("create master index", stdErrors.New("failed to create master index"))
	ErrGeneratePresignedUploadURL = errors.ProcessingFailed("generate presigned upload URL", stdErrors.New("failed to generate presigned upload URL"))
)

// CloudFront configuration errors
var (
	ErrCloudFrontNotConfigured              = errors.ConfigurationMissing("cloudfront_config")
	ErrInvalidCloudFrontPrivateKeyPath      = errors.ConfigurationInvalid("cloudfront_private_key_path", "invalid path")
	ErrFailedToReadCloudFrontPrivateKeyFile = errors.ProcessingFailed("CloudFront private key file read", stdErrors.New("failed to read CloudFront private key file"))
	ErrCloudFrontPrivateKeyNotProvided      = errors.ConfigurationMissing("cloudfront_private_key")
	ErrInvalidRSAPrivateKeyPEM              = errors.InvalidPrivateKeyType()
	ErrFailedToParseRSAPrivateKey           = errors.ProcessingFailed("RSA private key parsing", stdErrors.New("failed to parse RSA private key"))
)

// Keyframe data errors
var (
	ErrFailedToGetKeyframeData    = errors.FailedToGet("keyframe data", stdErrors.New("failed to get keyframe data"))
	ErrFailedToReadIFramePlaylist = errors.ProcessingFailed("I-frame playlist read", stdErrors.New("failed to read I-frame playlist"))
	ErrFailedToReadKeyframeData   = errors.ProcessingFailed("keyframe data read", stdErrors.New("failed to read keyframe data"))
)

// Session management errors
var (
	ErrCreateSession          = errors.FailedToCreate("session", stdErrors.New("failed to create session"))
	ErrGetSession             = errors.FailedToGet("session", stdErrors.New("failed to get session"))
	ErrUpdateSession          = errors.FailedToUpdate("session", stdErrors.New("failed to update session"))
	ErrEndSession             = errors.ProcessingFailed("end session", stdErrors.New("failed to end session"))
	ErrQueryUserSessions      = errors.FailedToQuery("user sessions", stdErrors.New("failed to query user sessions"))
	ErrScanMediaSessions      = errors.FailedToQuery("media sessions", stdErrors.New("failed to scan media sessions"))
	ErrCleanupExpiredSessions = errors.ProcessingFailed("cleanup expired sessions", stdErrors.New("failed to cleanup expired sessions"))
)
