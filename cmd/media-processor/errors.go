package main

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Configuration error functions

// MediaConvertRoleNotConfigured creates an error indicating MediaConvert role is not configured.
func MediaConvertRoleNotConfigured() *errors.AppError {
	return errors.ServiceInitializationFailed("MediaConvert", nil).WithMetadata("reason", "role_not_configured")
}

// AWS service error functions

// AWSConfigLoadFailed creates an error indicating AWS configuration loading failed.
func AWSConfigLoadFailed(err error) *errors.AppError {
	return errors.ServiceInitializationFailed("AWS", err)
}

// S3GetObjectFailed creates an error indicating S3 object retrieval failed.
func S3GetObjectFailed(err error) *errors.AppError {
	return errors.GetFailed("S3Object", err)
}

// S3ReadObjectFailed creates an error indicating S3 object reading failed.
func S3ReadObjectFailed(err error) *errors.AppError {
	return errors.GetFailed("S3ObjectData", err)
}

// S3UploadVideoFailed creates an error indicating video upload to S3 failed.
func S3UploadVideoFailed(err error) *errors.AppError {
	return errors.CreateFailed("S3Video", err)
}

// S3UploadOriginalFailed creates an error indicating original file upload to S3 failed.
func S3UploadOriginalFailed(err error) *errors.AppError {
	return errors.CreateFailed("S3Original", err)
}

// S3KeySanitizationFailed creates an error indicating S3 key sanitization failed.
func S3KeySanitizationFailed(err error) *errors.AppError {
	if err != nil {
		return errors.NewStorageInternalError(errors.CodeInvalidInput, "S3 key sanitization failed", err)
	}
	return errors.InvalidInput("s3_key", "sanitization failed")
}

// Job processing error functions

// JobGetFailed creates an error indicating job retrieval failed.
func JobGetFailed(err error) *errors.AppError {
	return errors.GetFailed("MediaJob", err)
}

// JobUpdateStatusFailed creates an error indicating job status update failed.
func JobUpdateStatusFailed(err error) *errors.AppError {
	return errors.UpdateFailed("MediaJob", err)
}

// JobUpdateWarningFailed creates an error indicating job update for budget warning failed.
func JobUpdateWarningFailed(err error) *errors.AppError {
	return errors.UpdateFailed("MediaJobWarning", err)
}

// Media processing error functions

// MediaDownloadFailed creates an error indicating media download failed.
func MediaDownloadFailed(err error) *errors.AppError {
	return errors.MediaProcessingFailed("download", err)
}

// MediaRecordUpdateFailed creates an error indicating media record update failed.
func MediaRecordUpdateFailed(err error) *errors.AppError {
	return errors.UpdateFailed("MediaRecord", err)
}

// ImageProcessingFailed creates an error indicating image processing failed.
func ImageProcessingFailed(err error) *errors.AppError {
	return errors.MediaProcessingFailed("image", err)
}

// VideoValidationFailed creates an error indicating video duration validation failed.
func VideoValidationFailed(err error) *errors.AppError {
	if err != nil {
		return errors.NewValidationErrorWithCode(errors.CodeInvalidFormat, "video_duration", "Video duration validation failed")
	}
	return errors.VideoInvalidFormat("duration validation failed")
}

// AudioMetadataReadFailed creates an error indicating audio metadata reading failed.
func AudioMetadataReadFailed(err error) *errors.AppError {
	if err != nil {
		return errors.NewValidationErrorWithCode(errors.CodeInvalidFormat, "audio_metadata", "Audio metadata read failed")
	}
	return errors.AudioInvalidFormat("metadata read failed")
}

// File validation error functions

// EmptyFileError creates an error indicating file data is empty.
func EmptyFileError() *errors.AppError {
	return errors.ContentEmpty("file")
}

// FileTypeNotAllowedError creates an error indicating file type is not allowed.
func FileTypeNotAllowedError(mimeType string) *errors.AppError {
	return errors.MediaInvalidMimeType(mimeType, []string{"image/*", "video/*", "audio/*"})
}

// UnsupportedMediaTypeError creates an error indicating unsupported media type.
func UnsupportedMediaTypeError(mediaType string) *errors.AppError {
	return errors.MediaInvalidMimeType(mediaType, []string{"image", "video", "audio"})
}

// UnknownFileTypeError creates an error indicating unknown file type.
func UnknownFileTypeError(fileType string) *errors.AppError {
	return errors.MediaInvalidMimeType(fileType, []string{"detectable media types"})
}

// FileTooLargeError creates an error indicating file is too large.
func FileTooLargeError(size, maxSize int64) *errors.AppError {
	return errors.MediaFileTooLarge(size, maxSize)
}

// VideoDurationExceededError creates an error indicating video duration exceeds user limit.
func VideoDurationExceededError(duration, maxDuration int) *errors.AppError {
	return errors.ValueOutOfRange("video_duration", 0, maxDuration, duration)
}

// UnsupportedMediaTypeForUserError creates an error indicating unsupported media type for user.
func UnsupportedMediaTypeForUserError(mediaType string) *errors.AppError {
	return errors.MediaInvalidMimeType(mediaType, []string{"user-allowed types"})
}

// FileSizeExceedsUserLimitError creates an error indicating file size exceeds user limit.
func FileSizeExceedsUserLimitError(size, userMaxSize int64) *errors.AppError {
	return errors.MediaFileTooLarge(size, userMaxSize)
}

// FileValidationFailedError creates an error indicating file validation failed.
func FileValidationFailedError(err error) *errors.AppError {
	if err != nil {
		return errors.NewValidationErrorWithCode(errors.CodeValidationFailed, "file", "File validation failed")
	}
	return errors.NewValidationError("file", "File validation failed")
}

// InvalidMimeTypeFormatError creates an error indicating invalid MIME type format.
func InvalidMimeTypeFormatError(mimeType string) *errors.AppError {
	return errors.InvalidFormat("mime_type", "type/subtype").WithMetadata("invalid_mime_type", mimeType)
}

// DetectedMimeTypeInvalidError creates an error indicating detected MIME type is invalid.
func DetectedMimeTypeInvalidError(mimeType string) *errors.AppError {
	return errors.MediaInvalidMimeType(mimeType, []string{"detectable types"})
}

// Audio processing error functions

// UnableToDetermineAudioDurationError creates an error indicating unable to determine audio duration.
func UnableToDetermineAudioDurationError(err error) *errors.AppError {
	if err != nil {
		return errors.NewValidationErrorWithCode(errors.CodeInvalidFormat, "audio_duration", "Unable to determine audio duration")
	}
	return errors.AudioInvalidFormat("duration extraction failed")
}

// S3 key validation error functions

// InvalidUsernameForS3KeyError creates an error indicating invalid username for S3 key.
func InvalidUsernameForS3KeyError(username string) *errors.AppError {
	return errors.InvalidCharacters("username", "alphanumeric and underscore only").WithMetadata("invalid_username", username)
}

// InvalidMediaIDForS3KeyError creates an error indicating invalid media ID for S3 key.
func InvalidMediaIDForS3KeyError(mediaID string) *errors.AppError {
	return errors.InvalidCharacters("media_id", "alphanumeric and hyphens only").WithMetadata("invalid_media_id", mediaID)
}

// InvalidFilenameForS3KeyError creates an error indicating invalid filename for S3 key.
func InvalidFilenameForS3KeyError(filename string) *errors.AppError {
	return errors.InvalidCharacters("filename", "safe filename characters only").WithMetadata("invalid_filename", filename)
}

// MIME type validation error functions

// MimeTypeMismatchError creates an error indicating claimed MIME type does not match detected type.
func MimeTypeMismatchError(claimed, detected string) *errors.AppError {
	return errors.InvalidValue("mime_type", []string{detected}, claimed)
}

// Budget and cost error functions

// BudgetExceededError creates an error indicating budget was exceeded.
func BudgetExceededError(cost, budget int64) *errors.AppError {
	return errors.CostLimitExceeded("media_processing", float64(cost)/1000000.0).WithMetadata("budget_micros", budget)
}

// Additional specific media processing error functions

// UnsupportedMediaTypeProcessingError creates an error indicating unsupported media type for processing.
func UnsupportedMediaTypeProcessingError(mediaType string) *errors.AppError {
	return errors.MediaProcessingFailed(mediaType, stdErrors.New("unsupported media type for processing"))
}

// FileTooLargeForTypeError creates an error indicating file is too large for its type.
func FileTooLargeForTypeError(size, maxSize int64, fileType string) *errors.AppError {
	return errors.MediaFileTooLarge(size, maxSize).WithMetadata("file_type", fileType)
}

// MimeTypeMismatchDetailedError creates an error indicating claimed MIME type does not match detected type with details.
func MimeTypeMismatchDetailedError(claimed, detected string) *errors.AppError {
	return errors.InvalidValue("mime_type", []string{detected}, claimed).WithMetadata("detailed", true)
}

// UnknownFileTypeForProcessingError creates an error indicating unknown file type for processing.
func UnknownFileTypeForProcessingError(fileType string) *errors.AppError {
	return errors.MediaProcessingFailed(fileType, stdErrors.New("unknown file type for processing")).WithMetadata("reason", "unknown_type")
}

// FileSizeExceedsLimitError creates an error indicating file size exceeds limit.
func FileSizeExceedsLimitError(size, limit int64) *errors.AppError {
	return errors.MediaFileTooLarge(size, limit)
}

// VideoDurationTooLongError creates an error indicating video duration is too long.
func VideoDurationTooLongError(duration, maxDuration int) *errors.AppError {
	return errors.ValueOutOfRange("video_duration", 0, maxDuration, duration)
}

// UnsupportedForUserError creates an error indicating operation is unsupported for user.
func UnsupportedForUserError(operation string) *errors.AppError {
	return errors.OperationNotAllowed(operation).WithMetadata("reason", "user_restrictions")
}

// BudgetExceededForJobError creates an error indicating budget was exceeded for job.
func BudgetExceededForJobError(jobID string, cost, budget int64) *errors.AppError {
	return errors.CostLimitExceeded("media_job", float64(cost)/1000000.0).WithMetadata("job_id", jobID).WithMetadata("budget_micros", budget)
}

// Transcoding helper error functions

// EnhancedMediaConvertJobCreationFailed creates an error indicating enhanced MediaConvert job creation failed.
func EnhancedMediaConvertJobCreationFailed(err error) *errors.AppError {
	return errors.ExternalServiceUnavailable("MediaConvert", err)
}

// S3KeySanitizationAudioFailed creates an error indicating S3 key sanitization for audio failed.
func S3KeySanitizationAudioFailed(err error) *errors.AppError {
	if err != nil {
		return errors.NewStorageInternalError(errors.CodeInvalidInput, "S3 key sanitization for audio failed", err)
	}
	return errors.InvalidInput("s3_key", "audio sanitization failed")
}

// AudioUploadFailed creates an error indicating audio upload failed.
func AudioUploadFailed(err error) *errors.AppError {
	return errors.CreateFailed("S3Audio", err)
}
