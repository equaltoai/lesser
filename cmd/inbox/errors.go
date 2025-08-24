package main

import "errors"

// Error constants for the inbox Lambda function
var (
	// Database and initialization errors
	ErrDynamoDBInterface     = errors.New("DynamoDB client does not implement core.DB interface")
	ErrDynamORMInit          = errors.New("failed to initialize DynamORM")
	ErrRepositoryFactoryInit = errors.New("failed to initialize repository factory")

	// Direct message validation errors
	ErrDMToAddressing  = errors.New("invalid 'to' addressing in direct message")
	ErrDMCcAddressing  = errors.New("invalid 'cc' addressing in direct message")
	ErrDMBtoAddressing = errors.New("invalid 'bto' addressing in direct message")
	ErrDMBccAddressing = errors.New("invalid 'bcc' addressing in direct message")
	ErrDMPublicAddress = errors.New("direct messages cannot include public addressing")
	ErrDMRecipientURL  = errors.New("invalid recipient URL in direct message")
	ErrDMNoRecipients  = errors.New("direct messages must have specific actor recipients")

	// Actor and authentication errors
	ErrRequestConversion = errors.New("failed to convert request")
	ErrCreateRequest     = errors.New("failed to create request")
	ErrFetchActor        = errors.New("failed to fetch actor")
	ErrActorResponse     = errors.New("failed to fetch actor: invalid status")
	ErrParseActor        = errors.New("failed to parse actor")
	ErrNoPublicKey       = errors.New("actor has no public key")
	ErrParsePublicKey    = errors.New("failed to parse public key")

	// Activity processing errors
	ErrMarshalFollow  = errors.New("failed to marshal embedded follow")
	ErrParseFollow    = errors.New("failed to parse embedded follow")
	ErrInvalidNote    = errors.New("invalid note object")
	ErrCreateLike     = errors.New("failed to create like")
	ErrCreateAnnounce = errors.New("failed to create announce")
	ErrCreateBlock    = errors.New("failed to create block")
	ErrDeleteBlock    = errors.New("failed to delete block")

	// Block authorization errors
	ErrUnauthorizedBlockUndo = errors.New("unauthorized: only the original blocker can undo their block")

	// Collection management errors
	ErrAddNoTarget             = errors.New("add activity must specify a target collection")
	ErrAddNoObject             = errors.New("add activity must specify an object to add")
	ErrAddObjectNoID           = errors.New("add activity object must have an ID")
	ErrInvalidCollectionTarget = errors.New("invalid collection target")
	ErrUnauthorizedAdd         = errors.New("unauthorized add to collection")
	ErrAddItemFailed           = errors.New("failed to add item to collection")

	ErrRemoveNoTarget     = errors.New("remove activity must specify a target collection")
	ErrRemoveNoObject     = errors.New("remove activity must specify an object to remove")
	ErrRemoveObjectNoID   = errors.New("remove activity object must have an ID")
	ErrUnauthorizedRemove = errors.New("unauthorized remove from collection")
	ErrRemoveItemFailed   = errors.New("failed to remove item from collection")

	// URL processing errors
	ErrTargetURLEmpty  = errors.New("target URL is empty")
	ErrTargetURLFormat = errors.New("invalid target URL format")

	// Flag activity errors
	ErrInvalidFlag         = errors.New("invalid flag activity")
	ErrFlagNoObjects       = errors.New("flag activity must specify objects to flag")
	ErrStoreModerationFlag = errors.New("failed to store moderation flag")

	// Move activity errors
	ErrMoveNoTarget          = errors.New("move activity must specify a target account")
	ErrMoveAuthorization     = errors.New("move authorization failed")
	ErrStoreMigration        = errors.New("failed to store account migration")
	ErrUnsupportedFlagObject = errors.New("unsupported object type in flag activity")
	ErrExtractUsername       = errors.New("cannot extract username from new account ID")
	ErrVerifyMoveAuth        = errors.New("cannot verify move authorization via alsoKnownAs")
	ErrMoveNotAuthorized     = errors.New("move not authorized: new account does not list old account in alsoKnownAs field")

	// Collection ownership errors
	ErrDetermineCollectionOwner     = errors.New("cannot determine collection owner from target URL")
	ErrExtractCollectionOwner       = errors.New("cannot extract collection owner from target URL")
	ErrUnauthorizedCollection       = errors.New("only the actor can manage their own featured collection")
	ErrUnauthorizedCollectionModify = errors.New("actor is not authorized to modify collection")

	// Activity authorization errors
	ErrActivityBlocked      = errors.New("activity blocked: actors have blocked each other")
	ErrDetermineObjectOwner = errors.New("cannot determine object owner for authorization")
	ErrUnauthorizedUpdate   = errors.New("unauthorized update: actor cannot update object")
	ErrUnauthorizedDelete   = errors.New("unauthorized delete: actor cannot delete object")

	// Object processing errors
	ErrSerializeObject         = errors.New("failed to serialize existing object")
	ErrDeserializeObject       = errors.New("failed to deserialize existing object")
	ErrCreateUpdateHistory     = errors.New("failed to create update history")
	ErrUnsupportedDeleteObject = errors.New("unsupported object type in delete activity")
	ErrGetObjectLikes          = errors.New("failed to get object likes")
	ErrGetReplies              = errors.New("failed to get replies")
	ErrCreateTombstone         = errors.New("failed to create tombstone")

	// Recovery processing errors
	ErrUpdateTrusteeConfirmation = errors.New("failed to update trustee confirmation")
)
