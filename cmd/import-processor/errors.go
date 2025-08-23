// Package main defines error constants for the import-processor Lambda function.
package main

import "errors"

// AWS-related errors
var (
	ErrAWSConfigLoad = errors.New("failed to load AWS config")
	ErrS3ObjectGet   = errors.New("failed to get object from S3")
)

// Import processing errors
var (
	ErrRollbackFailed                       = errors.New("rollback failed with multiple errors")
	ErrOperationFailed                      = errors.New("operation failed")
	ErrUnsupportedImportFormat              = errors.New("unsupported import format")
	ErrCSVImportNotSupportedForType         = errors.New("CSV import not supported for type")
	ErrJSONImportNotSupportedForType        = errors.New("JSON import not supported for type")
	ErrActivityPubImportOnlySupportsArchive = errors.New("ActivityPub import only supported for archive type")
	ErrNoItemsFoundInActivityPubCollection  = errors.New("no items found in ActivityPub collection")
	ErrItemNotValidActivityPubObject        = errors.New("item is not a valid ActivityPub object")
	ErrItemMissingTypeField                 = errors.New("item missing type field")
	ErrCreateActivityMissingObject          = errors.New("create activity missing object")
	ErrCreateActivityObjectNotValid         = errors.New("create activity object is not a valid object")
	ErrFollowActivityMissingTargetObject    = errors.New("follow activity missing target object")
	ErrLikeActivityMissingObjectID          = errors.New("like activity missing object ID")
	ErrAnnounceActivityMissingObjectID      = errors.New("announce activity missing object ID")
	ErrObjectMissingID                      = errors.New("object missing ID")
	ErrImportDownloadFailed                 = errors.New("failed to download import file")
	ErrImportProcessFailed                  = errors.New("failed to process import")
	ErrImportStatusUpdateFailed             = errors.New("failed to update import status")
	ErrCSVHeaderRead                        = errors.New("failed to read CSV header")
	ErrJSONParseFailed                      = errors.New("failed to parse lists JSON")
	ErrActivityPubCollectionParseFailed     = errors.New("failed to parse ActivityPub collection")
)

// Model preparation errors
var (
	ErrAnnouncePrepFailed   = errors.New("failed to prepare announce")
	ErrBlockPrepFailed      = errors.New("failed to prepare block")
	ErrMutePrepFailed       = errors.New("failed to prepare mute")
	ErrListPrepFailed       = errors.New("failed to prepare list")
	ErrListMemberPrepFailed = errors.New("failed to prepare list member")
)

// Storage operation errors
var (
	ErrFollowRelationshipStore = errors.New("failed to store follow relationship")
	ErrFollowerActorGet        = errors.New("failed to get follower actor")
	ErrBookmarkCreate          = errors.New("failed to create bookmark")
	ErrListCreate              = errors.New("failed to create list")
)
