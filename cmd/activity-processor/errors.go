// Package main implements activity processor error handlers.
package main

import "github.com/equaltoai/lesser/pkg/errors"

// Stream processing errors - now using centralized Lambda domain functions
//nolint:unused
func batchRetryableErrors(errorCount int) *errors.AppError {
	return errors.BatchHasRetryableErrors(errorCount)
}

//nolint:unused
func missingNewImage(recordID string) *errors.AppError {
	return errors.StreamNewImageMissing(recordID)
}

//nolint:unused
func missingOldImage(recordID string) *errors.AppError {
	return errors.StreamOldImageMissing(recordID)
}

// Activity processing errors - using Federation domain functions
//nolint:unused
func missingAnnounceObjectID() *errors.AppError {
	return errors.ObjectMissingField("object")
}

// Object validation errors - using Federation domain functions
//nolint:unused
func objectNotMap() *errors.AppError {
	return errors.ObjectInvalidField("type", "object is not a map[string]any")
}

//nolint:unused
func missingObjectID() *errors.AppError {
	return errors.ObjectMissingField("id")
}

//nolint:unused
func objectIDMismatch() *errors.AppError {
	return errors.ObjectInvalidField("id", "object ID mismatch")
}

//nolint:unused
func missingObjectType() *errors.AppError {
	return errors.ObjectMissingField("type")
}

//nolint:unused
func missingAttributedTo() *errors.AppError {
	return errors.ObjectMissingField("attributedTo")
}

//nolint:unused
func missingMediaURL() *errors.AppError {
	return errors.ObjectMissingField("url")
}

//nolint:unused
func missingEventStartTime() *errors.AppError {
	return errors.ObjectMissingField("startTime")
}

// Timeline processing errors - using Lambda domain functions
//nolint:unused
func timelineRemovalFailed(actorID string, err error) *errors.AppError {
	return errors.TimelineRemovalFailed(actorID, err)
}

// Create activity errors - using Lambda domain functions
//nolint:unused
func timelineEntriesCreationFailed(err error) *errors.AppError {
	return errors.TimelineEntriesCreationFailed(err)
}

// Stream record processing errors - using Lambda and Federation domain functions
//nolint:unused
func streamRecordUnmarshalNewFailed(recordID string, err error) *errors.AppError {
	return errors.StreamUnmarshalFailed(recordID, "new", err)
}

//nolint:unused
func streamRecordUnmarshalOldFailed(recordID string, err error) *errors.AppError {
	return errors.StreamUnmarshalFailed(recordID, "old", err)
}

//nolint:unused
func activityParsingFailedDetailed(activityType string, err error) *errors.AppError {
	return errors.ActivityParsingFailed(activityType, err)
}

//nolint:unused
func activityObjectProcessingFailed(activityType string, err error) *errors.AppError {
	return errors.ActivityObjectProcessingFailed(activityType, err)
}

//nolint:unused
func noteMarshalingFailed(err error) *errors.AppError {
	return errors.ObjectMarshalingFailed("Note", err)
}

//nolint:unused
func noteUnmarshalingFailed(err error) *errors.AppError {
	return errors.ObjectUnmarshalingFailed("Note", err)
}

//nolint:unused
func timelineEntriesWriteFailed(err error) *errors.AppError {
	return errors.TimelineEntriesWriteFailed(err)
}

//nolint:unused
func actorRetrievalFailed(actorID string, err error) *errors.AppError {
	return errors.ActorFetchFailed(actorID, err)
}

//nolint:unused
func followersQueryingFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("followers querying", err)
}

//nolint:unused
func remoteObjectFetchFailed(url string, err error) *errors.AppError {
	return errors.RemoteFetchFailed(url, err)
}

//nolint:unused
func objectMarshalingFailed(objectType string, err error) *errors.AppError {
	return errors.ObjectMarshalingFailed(objectType, err)
}

//nolint:unused
func objectUnmarshalingToNoteFailed(err error) *errors.AppError {
	return errors.ObjectUnmarshalingFailed("Note", err)
}

//nolint:unused
func dlqRecordCreationFailed(messageID string, err error) *errors.AppError {
	return errors.DLQMessageSendFailed(messageID, err)
}

//nolint:unused
func tombstoneCreationFailedStream(err error) *errors.AppError {
	return errors.WorkflowStepFailed("tombstone creation", err)
}

// Activity workflow errors - using Lambda domain functions
//nolint:unused
func entityTypeExtractionFailed() *errors.AppError {
	return errors.EntityTypeExtractionFailed(nil)
}

//nolint:unused
func activityRecordUnmarshalingFailed(err error) *errors.AppError {
	return errors.ActivityRecordUnmarshalFailed(err)
}

//nolint:unused
func activityParsingFailed(activityType string, err error) *errors.AppError {
	return errors.ActivityParsingFailed(activityType, err)
}

//nolint:unused
func unknownActivityDirection(direction string) *errors.AppError {
	return errors.ActivityDirectionUnknown(direction)
}

// Follow activity errors - using Lambda domain functions
//nolint:unused
func followRelationshipCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("follow relationship creation", err)
}

// Accept/Reject activity errors - using Lambda domain functions
//nolint:unused
func relationshipStatusUpdateFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("relationship status update", err)
}

//nolint:unused
func rejectedRelationshipDeletionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("rejected relationship deletion", err)
}

// Create activity errors - using Lambda domain functions
//nolint:unused
func noteExtractionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("note extraction", err)
}

//nolint:unused
func statusCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("status creation", err)
}

//nolint:unused
func objectStorageFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("object storage", err)
}

//nolint:unused
func unsupportedObjectType(objectType string) *errors.AppError {
	return errors.ActivityTypeUnsupported(objectType)
}

// Delete activity errors - using Lambda domain functions
//nolint:unused
func tombstoneCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("tombstone creation", err)
}

// Like activity errors - using Lambda domain functions
//nolint:unused
func likeRecordCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("like record creation", err)
}

// Announce activity errors - using Lambda and Federation domain functions
//nolint:unused
func announceRecordCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("announce record creation", err)
}

//nolint:unused
func originalActivityFetchFailed(url string, err error) *errors.AppError {
	return errors.RemoteFetchFailed(url, err)
}

// Undo activity errors - using Lambda and Federation domain functions
//nolint:unused
func activityNotFoundLocally(activityID string) *errors.AppError {
	return errors.UndoObjectNotFound(activityID)
}

//nolint:unused
func followRelationshipDeletionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("follow relationship deletion", err)
}

//nolint:unused
func createdObjectDeletionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("created object deletion", err)
}

//nolint:unused
func objectHistoryRetrievalFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("object history retrieval", err)
}

//nolint:unused
func noHistoryFound(objectID string) *errors.AppError {
	return errors.WorkflowStepFailed("no history found for object", nil).WithMetadata("object_id", objectID)
}

//nolint:unused
func previousStateNotAvailable(objectID string) *errors.AppError {
	return errors.WorkflowStepFailed("previous state not available", nil).WithMetadata("object_id", objectID)
}

//nolint:unused
func objectReversionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("object reversion", err)
}

//nolint:unused
func tombstoneStatusCheckFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("tombstone status check", err)
}

//nolint:unused
func objectNotDeleted(objectID string) *errors.AppError {
	return errors.WorkflowStepFailed("object not deleted", nil).WithMetadata("object_id", objectID)
}

//nolint:unused
func tombstoneRetrievalFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("tombstone retrieval", err)
}

//nolint:unused
func objectHistoryRestorationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("object history restoration", err)
}

//nolint:unused
func objectRestorationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("object restoration", err)
}

//nolint:unused
func flagsRetrievalFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("flags retrieval", err)
}

//nolint:unused
func flagRecordDeletionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("flag record deletion", err)
}

//nolint:unused
func usernameExtractionFromActorURIFailed(uri string) *errors.AppError {
	return errors.ActorURIInvalid(uri)
}

//nolint:unused
func movedToFieldClearingFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("movedTo field clearing", err)
}

// Undo generic activity errors - using Federation domain functions
//nolint:unused
func objectIDExtractionFromActivityFailed() *errors.AppError {
	return errors.ActivityMissingField("object")
}

//nolint:unused
func undoActivityMissingActor() *errors.AppError {
	return errors.ActivityMissingField("actor")
}

// Add/Remove activity errors - using Lambda and Federation domain functions
//nolint:unused
func targetListRetrievalFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("target list retrieval", err)
}

//nolint:unused
func listOperationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("list operation", err)
}

//nolint:unused
func activityMissingTargetCollection() *errors.AppError {
	return errors.ActivityMissingField("target")
}

//nolint:unused
func objectExtractionFromActivityFailed() *errors.AppError {
	return errors.ActivityMissingField("object")
}

//nolint:unused
func noObjectsFoundInActivity() *errors.AppError {
	return errors.ActivityInvalidField("object", "no objects found in activity")
}

// Block activity errors - using Lambda domain functions
//nolint:unused
func blockRelationshipCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("block relationship creation", err)
}

// Flag activity errors - using Lambda domain functions
//nolint:unused
func flagRecordCreationFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("flag record creation", err)
}

// Move activity errors - using Federation and Lambda domain functions
//nolint:unused
func usernameExtractionFromOldActorURIFailed(uri string) *errors.AppError {
	return errors.ActorURIInvalid(uri)
}

//nolint:unused
func movedToFieldUpdateFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("movedTo field update", err)
}

// Undo activity-specific record deletion errors - using Federation and Lambda domain functions
//nolint:unused
func targetIDExtractionFromActivityFailed() *errors.AppError {
	return errors.ActivityMissingField("target")
}

//nolint:unused
func activityRecordDeletionFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("activity record deletion", err)
}

// Timeline processing errors - using Federation and Lambda domain functions
//nolint:unused
func usernameExtractionFromActorIDFailed(actorID string) *errors.AppError {
	return errors.ActorURIInvalid(actorID)
}

//nolint:unused
func followersRetrievalFailed(err error) *errors.AppError {
	return errors.WorkflowStepFailed("followers retrieval", err)
}

// Object processing errors - using Lambda domain functions
//nolint:unused
func objectValidationFailed(objectType, reason string) *errors.AppError {
	return errors.ObjectValidationFailed(objectType, reason)
}