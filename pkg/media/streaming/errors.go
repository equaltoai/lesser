package streaming

import "errors"

// Media metadata errors
var (
	ErrMediaMetadataNotFound    = errors.New("media metadata not found")
	ErrGetMetadataFromDynamoDB  = errors.New("get metadata from DynamoDB")
	ErrCreateMetadataInDynamoDB = errors.New("create metadata in DynamoDB")
	ErrUpdateMetadataInDynamoDB = errors.New("update metadata in DynamoDB")
)

// S3 operation errors
var (
	ErrCheckManifestExists        = errors.New("check manifest exists")
	ErrSaveManifestToS3           = errors.New("save manifest to S3")
	ErrGetSegmentInfo             = errors.New("get segment info")
	ErrListSegments               = errors.New("list segments")
	ErrCreateIndexForQuality      = errors.New("create index for quality")
	ErrCreateSegmentsDirectory    = errors.New("create segments directory for quality")
	ErrCreateMasterIndex          = errors.New("create master index")
	ErrGeneratePresignedUploadURL = errors.New("generate presigned upload URL")
)

// CloudFront configuration errors
var (
	ErrCloudFrontNotConfigured              = errors.New("CloudFront not configured: missing domain or key pair ID")
	ErrInvalidCloudFrontPrivateKeyPath      = errors.New("invalid CloudFront private key path")
	ErrFailedToReadCloudFrontPrivateKeyFile = errors.New("failed to read CloudFront private key file")
	ErrCloudFrontPrivateKeyNotProvided      = errors.New("CloudFront private key not provided")
	ErrInvalidRSAPrivateKeyPEM              = errors.New("invalid RSA private key PEM")
	ErrFailedToParseRSAPrivateKey           = errors.New("failed to parse RSA private key")
)

// Keyframe data errors
var (
	ErrFailedToGetKeyframeData    = errors.New("failed to get keyframe data")
	ErrFailedToReadIFramePlaylist = errors.New("failed to read I-frame playlist")
	ErrFailedToReadKeyframeData   = errors.New("failed to read keyframe data")
)

// Session management errors
var (
	ErrCreateSession          = errors.New("create session")
	ErrGetSession             = errors.New("get session")
	ErrUpdateSession          = errors.New("update session")
	ErrEndSession             = errors.New("end session")
	ErrQueryUserSessions      = errors.New("query user sessions")
	ErrScanMediaSessions      = errors.New("scan media sessions")
	ErrCleanupExpiredSessions = errors.New("cleanup expired sessions")
)
