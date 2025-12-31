// Package main implements error handlers for the inbox Lambda function.
package main

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Database and initialization errors - using Lambda domain functions

// dynamoDBInterfaceError creates an error when DynamoDB client does not implement core.DB interface.
func dynamoDBInterfaceError() *errors.AppError {
	return errors.ServiceInitializationFailed("DynamoDB client interface validation", nil)
}

// dynamORMInitError creates an error when DynamORM initialization fails.
func dynamORMInitError() *errors.AppError {
	return errors.ServiceInitializationFailed("DynamORM", nil)
}

// repositoryFactoryInitError creates an error when repository factory initialization fails.
func repositoryFactoryInitError() *errors.AppError {
	return errors.ServiceInitializationFailed("repository factory", nil)
}

// Direct message validation errors - using Validation domain functions

// dmToAddressingError creates an error for invalid 'to' addressing in direct message.
func dmToAddressingError() *errors.AppError {
	return errors.InvalidFormat("to", "ActivityPub addressing format")
}

// dmCcAddressingError creates an error for invalid 'cc' addressing in direct message.
func dmCcAddressingError() *errors.AppError {
	return errors.InvalidFormat("cc", "ActivityPub addressing format")
}

// dmBtoAddressingError creates an error for invalid 'bto' addressing in direct message.
func dmBtoAddressingError() *errors.AppError {
	return errors.InvalidFormat("bto", "ActivityPub addressing format")
}

// dmBccAddressingError creates an error for invalid 'bcc' addressing in direct message.
func dmBccAddressingError() *errors.AppError {
	return errors.InvalidFormat("bcc", "ActivityPub addressing format")
}

// dmPublicAddressError creates an error when direct messages include public addressing.
func dmPublicAddressError() *errors.AppError {
	return errors.InvalidValue("addressing", []string{"private recipients only"}, "public addressing")
}

// dmRecipientURLError creates an error for invalid recipient URL in direct message.
func dmRecipientURLError() *errors.AppError {
	return errors.URLInvalid("recipient URL")
}

// dmNoRecipientsError creates an error when direct messages have no specific actor recipients.
func dmNoRecipientsError() *errors.AppError {
	return errors.RequiredFieldMissing("recipients")
}

// Actor and authentication errors - using Auth domain functions

// Activity processing errors - using Federation domain functions

// marshalFollowError creates an error when marshaling embedded follow fails.
func marshalFollowError() *errors.AppError {
	return errors.MarshalingFailed("embedded follow", nil)
}

// parseFollowError creates an error when parsing embedded follow fails.
func parseFollowError() *errors.AppError {
	return errors.ParsingFailed("embedded follow", nil)
}

// invalidNoteError creates an error for invalid note object.
func invalidNoteError() *errors.AppError {
	return errors.ObjectInvalidField("type", "must be Note")
}

// createLikeError creates an error when like creation fails.
func createLikeError() *errors.AppError {
	return errors.FailedToCreate("like", nil)
}

// createAnnounceError creates an error when announce creation fails.
func createAnnounceError() *errors.AppError {
	return errors.FailedToCreate("announce", nil)
}

// createBlockError creates an error when block creation fails.
func createBlockError() *errors.AppError {
	return errors.FailedToCreate("block", nil)
}

// deleteBlockError creates an error when block deletion fails.
func deleteBlockError() *errors.AppError {
	return errors.FailedToDelete("block", nil)
}

// Block authorization errors - using Auth domain functions

// unauthorizedBlockUndoError creates an error when only the original blocker can undo their block.
func unauthorizedBlockUndoError() *errors.AppError {
	return errors.OperationNotAllowed("only the original blocker can undo their block")
}

// Collection management errors - using Validation and Auth domain functions

// addNoTargetError creates an error when add activity must specify a target collection.
func addNoTargetError() *errors.AppError {
	return errors.RequiredFieldMissing("target")
}

// addNoObjectError creates an error when add activity must specify an object to add.
func addNoObjectError() *errors.AppError {
	return errors.RequiredFieldMissing("object")
}

// addObjectNoIDError creates an error when add activity object must have an ID.
func addObjectNoIDError() *errors.AppError {
	return errors.RequiredFieldMissing("object.id")
}

// invalidCollectionTargetError creates an error for invalid collection target.
func invalidCollectionTargetError() *errors.AppError {
	return errors.URLInvalid("collection target")
}

// unauthorizedAddError creates an error for unauthorized add to collection.
func unauthorizedAddError() *errors.AppError {
	return errors.OperationNotAllowed("add to collection")
}

// addItemFailedError creates an error when adding item to collection fails.
func addItemFailedError() *errors.AppError {
	return errors.FailedToCreate("collection item", nil)
}

// removeNoTargetError creates an error when remove activity must specify a target collection.
func removeNoTargetError() *errors.AppError {
	return errors.RequiredFieldMissing("target")
}

// removeNoObjectError creates an error when remove activity must specify an object to remove.
func removeNoObjectError() *errors.AppError {
	return errors.RequiredFieldMissing("object")
}

// removeObjectNoIDError creates an error when remove activity object must have an ID.
func removeObjectNoIDError() *errors.AppError {
	return errors.RequiredFieldMissing("object.id")
}

// unauthorizedRemoveError creates an error for unauthorized remove from collection.
func unauthorizedRemoveError() *errors.AppError {
	return errors.OperationNotAllowed("remove from collection")
}

// removeItemFailedError creates an error when removing item from collection fails.
func removeItemFailedError() *errors.AppError {
	return errors.FailedToRemove("collection item", nil)
}

// URL processing errors - using Validation domain functions

// targetURLEmptyError creates an error when target URL is empty.
func targetURLEmptyError() *errors.AppError {
	return errors.RequiredFieldMissing("target URL")
}

// targetURLFormatError creates an error for invalid target URL format.
func targetURLFormatError() *errors.AppError {
	return errors.URLInvalid("target URL")
}

// Flag activity errors - using Federation and Validation domain functions

// invalidFlagError creates an error for invalid flag activity.
func invalidFlagError() *errors.AppError {
	return errors.ActivityInvalidField("flag", "invalid flag structure")
}

// flagNoObjectsError creates an error when flag activity must specify objects to flag.
func flagNoObjectsError() *errors.AppError {
	return errors.RequiredFieldMissing("objects")
}

// storeModerationFlagError creates an error when storing moderation flag fails.
func storeModerationFlagError() *errors.AppError {
	return errors.FailedToStore("moderation flag", nil)
}

// Move activity errors - using Federation and Auth domain functions

// moveNoTargetError creates an error when move activity must specify a target account.
func moveNoTargetError() *errors.AppError {
	return errors.RequiredFieldMissing("target")
}

// moveAuthorizationError creates an error when move authorization fails.
func moveAuthorizationError() *errors.AppError {
	return errors.OperationNotAllowed("move authorization failed")
}

// storeMigrationError creates an error when storing account migration fails.
func storeMigrationError() *errors.AppError {
	return errors.FailedToStore("account migration", nil)
}

// unsupportedFlagObjectError creates an error for unsupported object type in flag activity.
func unsupportedFlagObjectError() *errors.AppError {
	return errors.ActivityPubUnsupportedActivityType("flag object type")
}

// extractUsernameError creates an error when cannot extract username from new account ID.
func extractUsernameError() *errors.AppError {
	return errors.ParsingFailed("username from account ID", nil)
}

// verifyMoveAuthError creates an error when cannot verify move authorization via alsoKnownAs.
func verifyMoveAuthError() *errors.AppError {
	return errors.ProcessingFailed("move authorization verification", stdErrors.New("failed to verify move authorization"))
}

// moveNotAuthorizedError creates an error when move is not authorized.
func moveNotAuthorizedError() *errors.AppError {
	return errors.InsufficientPermissions("move not authorized: new account does not list old account in alsoKnownAs field")
}

// Collection ownership errors - using Auth and Validation domain functions

// determineCollectionOwnerError creates an error when cannot determine collection owner from target URL.
func determineCollectionOwnerError() *errors.AppError {
	return errors.ParsingFailed("collection owner from target URL", nil)
}

// extractCollectionOwnerError creates an error when cannot extract collection owner from target URL.
func extractCollectionOwnerError() *errors.AppError {
	return errors.ParsingFailed("collection owner from target URL", nil)
}

// unauthorizedCollectionError creates an error when only the actor can manage their own featured collection.
func unauthorizedCollectionError() *errors.AppError {
	return errors.InsufficientPermissions("only the actor can manage their own featured collection")
}

// unauthorizedCollectionModifyError creates an error when actor is not authorized to modify collection.
func unauthorizedCollectionModifyError() *errors.AppError {
	return errors.InsufficientPermissions("modify collection")
}

// Activity authorization errors - using Auth domain functions

// activityBlockedError creates an error when activity is blocked because actors have blocked each other.
func activityBlockedError() *errors.AppError {
	return errors.InsufficientPermissions("activity blocked: actors have blocked each other")
}

// determineObjectOwnerError creates an error when cannot determine object owner for authorization.
func determineObjectOwnerError() *errors.AppError {
	return errors.ParsingFailed("object owner for authorization", nil)
}

// unauthorizedUpdateError creates an error when actor cannot update object.
func unauthorizedUpdateError() *errors.AppError {
	return errors.InsufficientPermissions("update object")
}

// unauthorizedDeleteError creates an error when actor cannot delete object.
func unauthorizedDeleteError() *errors.AppError {
	return errors.InsufficientPermissions("delete object")
}

// Object processing errors - using Federation and Lambda domain functions

// serializeObjectError creates an error when serializing existing object fails.
func serializeObjectError() *errors.AppError {
	return errors.MarshalingFailed("existing object", nil)
}

// deserializeObjectError creates an error when deserializing existing object fails.
func deserializeObjectError() *errors.AppError {
	return errors.UnmarshalingFailed("existing object", nil)
}

// createUpdateHistoryError creates an error when creating update history fails.
func createUpdateHistoryError() *errors.AppError {
	return errors.FailedToCreate("update history", nil)
}

// unsupportedDeleteObjectError creates an error for unsupported object type in delete activity.
func unsupportedDeleteObjectError() *errors.AppError {
	return errors.ActivityPubUnsupportedActivityType("delete object type")
}

// getObjectLikesError creates an error when getting object likes fails.
func getObjectLikesError() *errors.AppError {
	return errors.FailedToGet("object likes", nil)
}

// getRepliesError creates an error when getting replies fails.
func getRepliesError() *errors.AppError {
	return errors.FailedToGet("replies", nil)
}

// createTombstoneError creates an error when creating tombstone fails.
func createTombstoneError() *errors.AppError {
	return errors.FailedToCreate("tombstone", nil)
}

// Recovery processing errors - using Lambda domain functions

// updateTrusteeConfirmationError creates an error when updating trustee confirmation fails.
func updateTrusteeConfirmationError() *errors.AppError {
	return errors.FailedToUpdate("trustee confirmation", nil)
}

// Request and conversion errors - using Validation domain functions

// requestConversionError creates an error when converting request fails.
func requestConversionError() *errors.AppError {
	return errors.ParsingFailed("request", nil)
}

// createRequestError creates an error when creating internal request structure fails.
func createRequestError() *errors.AppError {
	return errors.FailedToCreate("internal request", nil)
}

// Actor processing errors - using Federation domain functions

// fetchActorError creates an error when fetching actor fails.
func fetchActorError() *errors.AppError {
	return errors.FailedToGet("actor", nil)
}

// actorResponseError creates an error when actor response is invalid.
func actorResponseError() *errors.AppError {
	return errors.ExternalAPIError("actor response", 502, nil)
}

// parseActorError creates an error when parsing actor fails.
func parseActorError() *errors.AppError {
	return errors.ParsingFailed("actor", nil)
}

// noPublicKeyError creates an error when actor has no public key.
func noPublicKeyError() *errors.AppError {
	return errors.RequiredFieldMissing("public_key")
}

// parsePublicKeyError creates an error when parsing public key fails.
func parsePublicKeyError() *errors.AppError {
	return errors.ParsingFailed("public key", nil)
}
