// Package main provides error constants for the export-generator Lambda function.
package main

import "errors"

// Export generation error constants
var (
	// Format and validation errors
	ErrUnsupportedExportFormat = errors.New("unsupported export format")
	ErrCSVExportNotSupported   = errors.New("CSV export not supported for type")

	// AWS-related errors
	ErrAWSConfigLoad  = errors.New("failed to load AWS config")
	ErrS3Upload       = errors.New("failed to upload export")
	ErrS3PresignedURL = errors.New("failed to generate pre-signed URL")

	// Processing errors
	ErrGenerateExport = errors.New("failed to generate export")
	ErrCSVWriter      = errors.New("CSV writer error")
	ErrZipWriter      = errors.New("failed to close zip writer")
	ErrUpdateStatus   = errors.New("failed to update export status")

	// Data retrieval errors
	ErrGetActor           = errors.New("failed to get actor")
	ErrGetFollowers       = errors.New("get followers")
	ErrGetFollowing       = errors.New("get following")
	ErrGetBlocks          = errors.New("get blocked actors")
	ErrGetMutes           = errors.New("get muted actors")
	ErrGetLists           = errors.New("get lists")
	ErrGetBookmarks       = errors.New("get bookmarks")
	ErrGetOutbox          = errors.New("get outbox activities")
	ErrGetFollowingActors = errors.New("get following actors")
	ErrGetFollowerActors  = errors.New("get follower actors")
	ErrGetActorLikes      = errors.New("get actor likes")
	ErrGetDomainBlocks    = errors.New("get domain blocks")
	ErrGetUserMedia       = errors.New("get user media")

	// ZIP creation errors
	ErrZipEntryCreate = errors.New("create ZIP entry")
	ErrZipCopy        = errors.New("copy to ZIP")
)
