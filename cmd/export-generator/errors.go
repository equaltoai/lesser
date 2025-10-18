// Package main provides error functions for the export-generator Lambda function.
package main

import "github.com/equaltoai/lesser/pkg/errors"

// Export generation error functions

// Format and validation errors

// ErrUnsupportedExportFormat returns an error indicating unsupported export format.
func ErrUnsupportedExportFormat() error {
	return errors.NewAppError(errors.CodeInvalidFormat, errors.CategoryValidation, "unsupported export format")
}

// ErrCSVExportNotSupported returns an error indicating CSV export not supported for type.
func ErrCSVExportNotSupported() error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "CSV export not supported for type")
}

// AWS-related errors

// ErrAWSConfigLoad returns an error indicating AWS config failed to load.
func ErrAWSConfigLoad(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to load AWS config", err)
}

// ErrS3Upload returns an error indicating S3 upload failed.
func ErrS3Upload(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to upload export", err)
}

// ErrS3PresignedURL returns an error indicating S3 presigned URL generation failed.
func ErrS3PresignedURL(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to generate pre-signed URL", err)
}

// Processing errors

// ErrGenerateExport returns an error indicating export generation failed.
func ErrGenerateExport(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to generate export", err)
}

// ErrCSVWriter returns an error indicating CSV writer error.
func ErrCSVWriter(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "CSV writer error", err)
}

// ErrZipWriter returns an error indicating ZIP writer failed.
func ErrZipWriter(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to close zip writer", err)
}

// ErrUpdateStatus returns an error indicating export status update failed.
func ErrUpdateStatus(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to update export status", err)
}

// Data retrieval errors

// ErrGetActor returns an error indicating failed to get actor.
func ErrGetActor(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get actor", err)
}

// ErrGetFollowers returns an error indicating failed to get followers.
func ErrGetFollowers(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get followers", err)
}

// ErrGetFollowing returns an error indicating failed to get following.
func ErrGetFollowing(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get following", err)
}

// ErrGetBlocks returns an error indicating failed to get blocked actors.
func ErrGetBlocks(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get blocked actors", err)
}

// ErrGetMutes returns an error indicating failed to get muted actors.
func ErrGetMutes(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get muted actors", err)
}

// ErrGetLists returns an error indicating failed to get lists.
func ErrGetLists(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get lists", err)
}

// ErrGetBookmarks returns an error indicating failed to get bookmarks.
func ErrGetBookmarks(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get bookmarks", err)
}

// ErrGetOutbox returns an error indicating failed to get outbox activities.
func ErrGetOutbox(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get outbox activities", err)
}

// ErrGetFollowingActors returns an error indicating failed to get following actors.
func ErrGetFollowingActors(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get following actors", err)
}

// ErrGetFollowerActors returns an error indicating failed to get follower actors.
func ErrGetFollowerActors(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get follower actors", err)
}

// ErrGetActorLikes returns an error indicating failed to get actor likes.
func ErrGetActorLikes(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get actor likes", err)
}

// ErrGetDomainBlocks returns an error indicating failed to get domain blocks.
func ErrGetDomainBlocks(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get domain blocks", err)
}

// ErrGetUserMedia returns an error indicating failed to get user media.
func ErrGetUserMedia(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "get user media", err)
}

// ZIP creation errors

// ErrZipEntryCreate returns an error indicating failed to create ZIP entry.
func ErrZipEntryCreate(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "create ZIP entry", err)
}

// ErrZipCopy returns an error indicating failed to copy to ZIP.
func ErrZipCopy(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "copy to ZIP", err)
}
