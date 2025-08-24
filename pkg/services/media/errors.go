package media

import "errors"

// Media service specific errors
var (
	// ErrMediaNotFound is returned when media is not found
	ErrMediaNotFound = errors.New("media not found")

	// ErrMediaCreateFailed is returned when media creation fails
	ErrMediaCreateFailed = errors.New("failed to create media")

	// ErrMediaUpdateFailed is returned when media update fails
	ErrMediaUpdateFailed = errors.New("failed to update media")

	// ErrMediaDeleteFailed is returned when media deletion fails
	ErrMediaDeleteFailed = errors.New("failed to delete media")

	// ErrMediaAccessDenied is returned when media access is denied
	ErrMediaAccessDenied = errors.New("media access denied")

	// ErrMediaProcessingFailed is returned when media processing fails
	ErrMediaProcessingFailed = errors.New("media processing failed")

	// ErrDatabaseOperation is returned when database operations fail
	ErrDatabaseOperation = errors.New("database error")

	// ErrMediaStorageFailed is returned when media storage fails
	ErrMediaStorageFailed = errors.New("failed to store media record")

	// ErrMediaRetrievalFailed is returned when media retrieval fails
	ErrMediaRetrievalFailed = errors.New("failed to get media")

	// ErrMediaFileDataRequired is returned when file data is required but missing
	ErrMediaFileDataRequired = errors.New("file data is required")

	// ErrMediaFileTooLarge is returned when file size exceeds maximum limit
	ErrMediaFileTooLarge = errors.New("file size too large")

	// ErrMediaUnsupportedType is returned when content type is not supported
	ErrMediaUnsupportedType = errors.New("unsupported content type")

	// ErrMediaFileExtensionMismatch is returned when file extension doesn't match content type
	ErrMediaFileExtensionMismatch = errors.New("file extension does not match content type")

	// ErrMediaNotReady is returned when media is not ready for viewing
	ErrMediaNotReady = errors.New("media not ready for viewing")

	// ErrMediaProcessingQueueFailed is returned when media processing queue operation fails
	ErrMediaProcessingQueueFailed = errors.New("media processing queue failed")

	// ErrMediaNotReadyForStreaming is returned when media is not ready for streaming
	ErrMediaNotReadyForStreaming = errors.New("media not ready for streaming")

	// ErrMediaValidationFailed is returned when media validation fails
	ErrMediaValidationFailed = errors.New("media validation failed")

	// ErrMediaUnauthorizedAccess is returned when user is not authorized to access/modify media
	ErrMediaUnauthorizedAccess = errors.New("unauthorized media access")
)
