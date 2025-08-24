// Package main defines error functions for the import-processor Lambda function.
package main

import "github.com/equaltoai/lesser/pkg/errors"

// AWS-related errors

// ErrAWSConfigLoad returns an error indicating AWS configuration failed to load.
func ErrAWSConfigLoad(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to load AWS config", err)
}

// ErrS3ObjectGet returns an error indicating S3 object retrieval failed.
func ErrS3ObjectGet(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get object from S3", err)
}

// Import processing errors

// ErrRollbackFailed returns an error indicating rollback failed with multiple errors.
func ErrRollbackFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeTransactionFailed, "rollback failed with multiple errors", err)
}

// ErrOperationFailed returns an error indicating operation failed.
func ErrOperationFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "operation failed", err)
}

// ErrUnsupportedImportFormat returns an error indicating unsupported import format.
func ErrUnsupportedImportFormat() error {
	return errors.NewAppError(errors.CodeInvalidFormat, errors.CategoryValidation, "unsupported import format")
}

// ErrCSVImportNotSupportedForType returns an error indicating CSV import not supported for type.
func ErrCSVImportNotSupportedForType(importType string) error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "CSV import not supported for type").
		WithMetadata("import_type", importType)
}

// ErrJSONImportNotSupportedForType returns an error indicating JSON import not supported for type.
func ErrJSONImportNotSupportedForType(importType string) error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "JSON import not supported for type").
		WithMetadata("import_type", importType)
}

// ErrActivityPubImportOnlySupportsArchive returns an error indicating ActivityPub import only supports archive type.
func ErrActivityPubImportOnlySupportsArchive() error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "ActivityPub import only supported for archive type")
}

// ErrNoItemsFoundInActivityPubCollection returns an error indicating no items found in ActivityPub collection.
func ErrNoItemsFoundInActivityPubCollection() error {
	return errors.NewAppError(errors.CodeNotFound, errors.CategoryValidation, "no items found in ActivityPub collection")
}

// ErrItemNotValidActivityPubObject returns an error indicating item is not a valid ActivityPub object.
func ErrItemNotValidActivityPubObject() error {
	return errors.NewAppError(errors.CodeActivityParsingFailed, errors.CategoryFederation, "item is not a valid ActivityPub object")
}

// ErrItemMissingTypeField returns an error indicating item missing type field.
func ErrItemMissingTypeField() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "item missing type field")
}

// ErrCreateActivityMissingObject returns an error indicating create activity missing object.
func ErrCreateActivityMissingObject() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "create activity missing object")
}

// ErrCreateActivityObjectNotValid returns an error indicating create activity object is not valid.
func ErrCreateActivityObjectNotValid() error {
	return errors.NewAppError(errors.CodeActivityParsingFailed, errors.CategoryFederation, "create activity object is not a valid object")
}

// ErrFollowActivityMissingTargetObject returns an error indicating follow activity missing target object.
func ErrFollowActivityMissingTargetObject() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "follow activity missing target object")
}

// ErrLikeActivityMissingObjectID returns an error indicating like activity missing object ID.
func ErrLikeActivityMissingObjectID() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "like activity missing object ID")
}

// ErrAnnounceActivityMissingObjectID returns an error indicating announce activity missing object ID.
func ErrAnnounceActivityMissingObjectID() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "announce activity missing object ID")
}

// ErrObjectMissingID returns an error indicating object missing ID.
func ErrObjectMissingID() error {
	return errors.NewAppError(errors.CodeRequiredFieldMissing, errors.CategoryValidation, "object missing ID")
}

// ErrImportDownloadFailed returns an error indicating import file download failed.
func ErrImportDownloadFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to download import file", err)
}

// ErrImportProcessFailed returns an error indicating import processing failed.
func ErrImportProcessFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to process import", err)
}

// ErrImportStatusUpdateFailed returns an error indicating import status update failed.
func ErrImportStatusUpdateFailed(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to update import status", err)
}

// ErrCSVHeaderRead returns an error indicating CSV header read failed.
func ErrCSVHeaderRead(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to read CSV header")
}

// ErrJSONParseFailed returns an error indicating JSON parsing failed.
func ErrJSONParseFailed(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to parse lists JSON")
}

// ErrActivityPubCollectionParseFailed returns an error indicating ActivityPub collection parsing failed.
func ErrActivityPubCollectionParseFailed(err error) error {
	return errors.WrapError(err, errors.CodeActivityParsingFailed, errors.CategoryFederation, "failed to parse ActivityPub collection")
}

// Model preparation errors

// ErrAnnouncePrepFailed returns an error indicating announce preparation failed.
func ErrAnnouncePrepFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to prepare announce", err)
}

// ErrBlockPrepFailed returns an error indicating block preparation failed.
func ErrBlockPrepFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to prepare block", err)
}

// ErrMutePrepFailed returns an error indicating mute preparation failed.
func ErrMutePrepFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to prepare mute", err)
}

// ErrListPrepFailed returns an error indicating list preparation failed.
func ErrListPrepFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to prepare list", err)
}

// ErrListMemberPrepFailed returns an error indicating list member preparation failed.
func ErrListMemberPrepFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to prepare list member", err)
}

// Storage operation errors

// ErrFollowRelationshipStore returns an error indicating follow relationship storage failed.
func ErrFollowRelationshipStore(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to store follow relationship", err)
}

// ErrFollowerActorGet returns an error indicating follower actor retrieval failed.
func ErrFollowerActorGet(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get follower actor", err)
}

// ErrBookmarkCreate returns an error indicating bookmark creation failed.
func ErrBookmarkCreate(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to create bookmark", err)
}

// ErrListCreate returns an error indicating list creation failed.
func ErrListCreate(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to create list", err)
}
