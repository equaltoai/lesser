package main

import "errors"

// Stream processing errors
var (
	ErrBatchRetryableErrors = errors.New("batch has retryable errors")
	ErrMissingNewImage      = errors.New("no new image in record")
	ErrMissingOldImage      = errors.New("no old image in remove record")
)

// Activity processing errors
var (
	ErrMissingAnnounceObjectID = errors.New("no object ID in Announce activity")
)

// Object validation errors
var (
	ErrObjectNotMap          = errors.New("object is not a map[string]any")
	ErrMissingObjectID       = errors.New("object missing or invalid 'id' field")
	ErrObjectIDMismatch      = errors.New("object ID mismatch")
	ErrMissingObjectType     = errors.New("object missing or invalid 'type' field")
	ErrMissingAttributedTo   = errors.New("object missing 'attributedTo' field")
	ErrMissingMediaURL       = errors.New("media object missing 'url' field")
	ErrMissingEventStartTime = errors.New("event object missing 'startTime' field")
)

// Timeline processing errors
var (
	ErrTimelineRemovalFailed = errors.New("failed to remove from timeline types")
)

// Activity processing workflow errors
var (
	ErrEntityTypeExtraction        = errors.New("entity type extraction failed")
	ErrActivityRecordUnmarshaling  = errors.New("activity record unmarshaling failed") 
	ErrActivityParsing             = errors.New("activity parsing failed")
	ErrUnknownActivityDirection    = errors.New("unknown activity direction")
)

// Follow activity errors
var (
	ErrFollowRelationshipCreation = errors.New("follow relationship creation failed")
)

// Accept/Reject activity errors  
var (
	ErrRelationshipStatusUpdate     = errors.New("relationship status update failed")
	ErrRejectedRelationshipDeletion = errors.New("rejected relationship deletion failed")
)

// Create activity errors
var (
	ErrNoteExtraction         = errors.New("note extraction failed")
	ErrStatusCreation         = errors.New("status creation failed")
	ErrObjectStorage          = errors.New("object storage failed")
	ErrUnsupportedObjectType  = errors.New("unsupported object type")
	ErrTimelineEntriesCreation = errors.New("timeline entries creation failed")
)

// Delete activity errors
var (
	ErrTombstoneCreation = errors.New("tombstone creation failed")
)

// Like activity errors
var (
	ErrLikeRecordCreation = errors.New("like record creation failed")
)

// Announce activity errors
var (
	ErrAnnounceRecordCreation = errors.New("announce record creation failed")
	ErrOriginalActivityFetch  = errors.New("original activity fetch failed")
)

// Undo activity errors
var (
	ErrActivityNotFoundLocally           = errors.New("activity not found locally")
	ErrFollowRelationshipDeletion        = errors.New("follow relationship deletion failed")
	ErrCreatedObjectDeletion             = errors.New("created object deletion failed")
	ErrObjectHistoryRetrieval            = errors.New("object history retrieval failed")
	ErrNoHistoryFound                    = errors.New("no history found")
	ErrPreviousStateNotAvailable         = errors.New("previous state not available")
	ErrObjectReversion                   = errors.New("object reversion failed")
	ErrTombstoneStatusCheck              = errors.New("tombstone status check failed")
	ErrObjectNotDeleted                  = errors.New("object not deleted")
	ErrTombstoneRetrieval                = errors.New("tombstone retrieval failed")
	ErrObjectHistoryRestoration          = errors.New("object history restoration failed")
	ErrObjectRestoration                 = errors.New("object restoration failed")
	ErrFlagsRetrieval                    = errors.New("flags retrieval failed")
	ErrFlagRecordDeletion                = errors.New("flag record deletion failed")
	ErrUsernameExtractionFromActorURI    = errors.New("username extraction from actor URI failed")
	ErrMovedToFieldClearing              = errors.New("movedTo field clearing failed")
)

// Undo generic activity errors
var (
	ErrObjectIDExtractionFromActivity = errors.New("object ID extraction from activity failed")
	ErrUndoActivityMissingActor       = errors.New("undo activity missing actor")
)

// Add/Remove activity errors  
var (
	ErrTargetListRetrieval                = errors.New("target list retrieval failed")
	ErrListOperation                      = errors.New("list operation failed")
	ErrActivityMissingTargetCollection    = errors.New("activity missing target collection")
	ErrObjectExtractionFromActivity       = errors.New("object extraction from activity failed")
	ErrNoObjectsFoundInActivity           = errors.New("no objects found in activity")
)

// Block activity errors
var (
	ErrBlockRelationshipCreation = errors.New("block relationship creation failed")
)

// Flag activity errors
var (
	ErrFlagRecordCreation = errors.New("flag record creation failed")
)

// Move activity errors
var (
	ErrUsernameExtractionFromOldActorURI = errors.New("username extraction from old actor URI failed")
	ErrMovedToFieldUpdate                = errors.New("movedTo field update failed")
)

// Undo activity-specific record deletion errors
var (
	ErrTargetIDExtractionFromActivity = errors.New("target ID extraction from activity failed")
	ErrActivityRecordDeletion          = errors.New("activity record deletion failed")
)

// Timeline processing errors
var (
	ErrUsernameExtractionFromActorID = errors.New("username extraction from actor ID failed")
	ErrFollowersRetrieval            = errors.New("followers retrieval failed")
)

// Stream record processing errors
var (
	ErrStreamRecordUnmarshalNew     = errors.New("failed to unmarshal new image")
	ErrStreamRecordUnmarshalOld     = errors.New("failed to unmarshal old image")
	ErrActivityParsingFailed        = errors.New("failed to parse activity")
	ErrActivityObjectProcessing     = errors.New("failed to process activity object")
	ErrNoteMarshaling              = errors.New("failed to marshal note")
	ErrNoteUnmarshaling            = errors.New("failed to unmarshal note")
	ErrTimelineEntriesWrite        = errors.New("failed to write timeline entries")
	ErrActorRetrieval              = errors.New("failed to get actor")
	ErrFollowersQuerying           = errors.New("failed to query followers")
	ErrObjectValidation            = errors.New("validation failed")
	ErrRemoteObjectFetch           = errors.New("failed after attempts")
	ErrObjectMarshaling            = errors.New("failed to marshal object")
	ErrObjectUnmarshalingToNote    = errors.New("failed to unmarshal to Note")
	ErrDLQRecordCreation           = errors.New("failed to create DLQ record")
	ErrTombstoneCreationFailed     = errors.New("failed to create tombstone")
)
