package media

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Media service specific errors
var (
	// ErrMediaNotFound is returned when media is not found
	ErrMediaNotFound = errors.NewAppError(errors.CodeNotFound, errors.CategoryMedia, "media not found")

	// ErrMediaCreateFailed is returned when media creation fails
	ErrMediaCreateFailed = errors.FailedToCreate("media", stdErrors.New("failed to create media"))

	// ErrMediaUpdateFailed is returned when media update fails
	ErrMediaUpdateFailed = errors.FailedToUpdate("media", stdErrors.New("failed to update media"))

	// ErrMediaDeleteFailed is returned when media deletion fails
	ErrMediaDeleteFailed = errors.FailedToDelete("media", stdErrors.New("failed to delete media"))

	// ErrMediaAccessDenied is returned when media access is denied
	ErrMediaAccessDenied = errors.AccessDeniedForResource("media", "unknown")

	// ErrMediaProcessingFailed is returned when media processing fails
	ErrMediaProcessingFailed = errors.ProcessingFailed("media processing", stdErrors.New("media processing failed"))

	// ErrDatabaseOperation is returned when database operations fail
	ErrDatabaseOperation = errors.NewStorageError(errors.CodeInternal, "database error")

	// ErrMediaStorageFailed is returned when media storage fails
	ErrMediaStorageFailed = errors.FailedToStore("media record", stdErrors.New("failed to store media record"))

	// ErrMediaRetrievalFailed is returned when media retrieval fails
	ErrMediaRetrievalFailed = errors.FailedToGet("media", stdErrors.New("failed to get media"))

	// ErrMediaFileDataRequired is returned when file data is required but missing
	ErrMediaFileDataRequired = errors.NewValidationError("file_data", "required")

	// ErrMediaFileTooLarge is returned when file size exceeds maximum limit
	ErrMediaFileTooLarge = errors.NewValidationError("file_size", "too large")

	// ErrMediaUnsupportedType is returned when content type is not supported
	ErrMediaUnsupportedType = errors.ContentTypeNotAllowed("unknown")

	// ErrMediaFileExtensionMismatch is returned when file extension doesn't match content type
	ErrMediaFileExtensionMismatch = errors.NewValidationError("file_extension", "does not match content type")

	// ErrMediaUnsafeSVG is returned when SVG content contains active or external content.
	ErrMediaUnsafeSVG = errors.NewValidationError("svg", "unsafe svg content")

	// ErrMediaNotReady is returned when media is not ready for viewing
	ErrMediaNotReady = errors.MediaAttachmentNotReady("unknown")

	// ErrMediaProcessingQueueFailed is returned when media processing queue operation fails
	ErrMediaProcessingQueueFailed = errors.ProcessingFailed("media processing queue", stdErrors.New("media processing queue failed"))

	// ErrMediaNotReadyForStreaming is returned when media is not ready for streaming
	ErrMediaNotReadyForStreaming = errors.NewValidationError("media_streaming", "not ready for streaming")

	// ErrMediaValidationFailed is returned when media validation fails
	ErrMediaValidationFailed = errors.MediaAttachmentValidationFailed("unknown reason")

	// ErrMediaUnauthorizedAccess is returned when user is not authorized to access/modify media
	ErrMediaUnauthorizedAccess = errors.InsufficientPermissions("media access")
)
