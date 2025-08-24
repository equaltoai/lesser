package main

import "errors"

// Error constants for media processor
var (
	// Configuration errors
	ErrMediaConvertRoleNotConfigured = errors.New("MediaConvert role not configured")

	// AWS service errors
	ErrAWSConfigLoad     = errors.New("failed to load AWS config")
	ErrS3GetObject       = errors.New("failed to get object from S3")
	ErrS3ReadObject      = errors.New("failed to read S3 object")
	ErrS3UploadVideo     = errors.New("failed to upload video")
	ErrS3UploadOriginal  = errors.New("failed to upload original")
	ErrS3KeySanitization = errors.New("failed to sanitize S3 key")

	// Job processing errors
	ErrJobGet           = errors.New("failed to get job")
	ErrJobUpdateStatus  = errors.New("failed to update job status")
	ErrJobUpdateWarning = errors.New("failed to get job for budget warning")

	// Media processing errors
	ErrMediaDownload     = errors.New("failed to download original")
	ErrMediaRecordUpdate = errors.New("failed to update media record")
	ErrImageProcessing   = errors.New("failed to process image")
	ErrVideoValidation   = errors.New("cannot validate video duration")
	ErrAudioMetadataRead = errors.New("failed to read audio metadata")

	// File validation errors
	ErrEmptyFile                   = errors.New("file data is empty")
	ErrFileTypeNotAllowed          = errors.New("file type not allowed")
	ErrUnsupportedMediaType        = errors.New("unsupported media type")
	ErrUnknownFileType             = errors.New("unknown file type")
	ErrFileTooLarge                = errors.New("file too large")
	ErrVideoDurationExceeded       = errors.New("video duration exceeds user limit")
	ErrUnsupportedMediaTypeForUser = errors.New("unsupported media type")
	ErrFileSizeExceedsUserLimit    = errors.New("file size exceeds user limit")
	ErrFileValidationFailed        = errors.New("file validation failed")
	ErrInvalidMimeTypeFormat       = errors.New("invalid MIME type format")
	ErrDetectedMimeTypeInvalid     = errors.New("detected MIME type is invalid")

	// Audio processing errors
	ErrUnableToDetermineAudioDuration = errors.New("unable to determine audio duration")

	// S3 key validation errors
	ErrInvalidUsernameForS3Key = errors.New("invalid username for S3 key")
	ErrInvalidMediaIDForS3Key  = errors.New("invalid media ID for S3 key")
	ErrInvalidFilenameForS3Key = errors.New("invalid filename for S3 key")

	// MIME type validation errors
	ErrMimeTypeMismatch = errors.New("claimed MIME type does not match detected type")

	// Budget and cost errors
	ErrBudgetExceeded = errors.New("budget exceeded")

	// Additional specific media processing errors
	ErrUnsupportedMediaTypeProcessing = errors.New("unsupported media type for processing")
	ErrFileTooLargeForType            = errors.New("file too large for type")
	ErrMimeTypeMismatchDetailed       = errors.New("claimed MIME type does not match detected type")
	ErrUnknownFileTypeForProcessing   = errors.New("unknown file type for processing")
	ErrFileSizeExceedsLimit           = errors.New("file size exceeds limit")
	ErrVideoDurationTooLong           = errors.New("video duration too long")
	ErrUnsupportedForUser             = errors.New("unsupported for user")
	ErrBudgetExceededForJob           = errors.New("budget exceeded for job")

	// Transcoding helper errors
	ErrEnhancedMediaConvertJobCreation = errors.New("failed to create enhanced MediaConvert job")
	ErrS3KeySanitizationAudio          = errors.New("failed to sanitize S3 key")
	ErrAudioUpload                     = errors.New("failed to upload audio")
)
