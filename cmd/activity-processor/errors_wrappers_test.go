package main

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestActivityProcessor_ErrorWrappers(t *testing.T) {
	baseErr := stderrors.New("boom")

	cases := []struct {
		name string
		fn   func() *apperrors.AppError
	}{
		{"batchRetryableErrors", func() *apperrors.AppError { return batchRetryableErrors(2) }},
		{"missingNewImage", func() *apperrors.AppError { return missingNewImage("rec-1") }},
		{"missingOldImage", func() *apperrors.AppError { return missingOldImage("rec-2") }},
		{"missingAnnounceObjectID", missingAnnounceObjectID},
		{"objectNotMap", objectNotMap},
		{"missingObjectID", missingObjectID},
		{"objectIDMismatch", objectIDMismatch},
		{"missingObjectType", missingObjectType},
		{"missingAttributedTo", missingAttributedTo},
		{"missingMediaURL", missingMediaURL},
		{"missingEventStartTime", missingEventStartTime},
		{"timelineRemovalFailed", func() *apperrors.AppError { return timelineRemovalFailed("actor", baseErr) }},
		{"timelineEntriesCreationFailed", func() *apperrors.AppError { return timelineEntriesCreationFailed(baseErr) }},
		{"streamRecordUnmarshalNewFailed", func() *apperrors.AppError { return streamRecordUnmarshalNewFailed("rec-3", baseErr) }},
		{"streamRecordUnmarshalOldFailed", func() *apperrors.AppError { return streamRecordUnmarshalOldFailed("rec-4", baseErr) }},
		{"activityParsingFailedDetailed", func() *apperrors.AppError { return activityParsingFailedDetailed("Create", baseErr) }},
		{"activityObjectProcessingFailed", func() *apperrors.AppError { return activityObjectProcessingFailed("Create", baseErr) }},
		{"noteMarshalingFailed", func() *apperrors.AppError { return noteMarshalingFailed(baseErr) }},
		{"noteUnmarshalingFailed", func() *apperrors.AppError { return noteUnmarshalingFailed(baseErr) }},
		{"timelineEntriesWriteFailed", func() *apperrors.AppError { return timelineEntriesWriteFailed(baseErr) }},
		{"actorRetrievalFailed", func() *apperrors.AppError { return actorRetrievalFailed("actor", baseErr) }},
		{"followersQueryingFailed", func() *apperrors.AppError { return followersQueryingFailed(baseErr) }},
		{"remoteObjectFetchFailed", func() *apperrors.AppError {
			return remoteObjectFetchFailed("https://remote.example/objects/1", baseErr)
		}},
		{"objectMarshalingFailed", func() *apperrors.AppError { return objectMarshalingFailed("Object", baseErr) }},
		{"objectUnmarshalingToNoteFailed", func() *apperrors.AppError { return objectUnmarshalingToNoteFailed(baseErr) }},
		{"dlqRecordCreationFailed", func() *apperrors.AppError { return dlqRecordCreationFailed("msg-1", baseErr) }},
		{"tombstoneCreationFailedStream", func() *apperrors.AppError { return tombstoneCreationFailedStream(baseErr) }},
		{"entityTypeExtractionFailed", entityTypeExtractionFailed},
		{"activityRecordUnmarshalingFailed", func() *apperrors.AppError { return activityRecordUnmarshalingFailed(baseErr) }},
		{"activityParsingFailed", func() *apperrors.AppError { return activityParsingFailed("Create", baseErr) }},
		{"unknownActivityDirection", func() *apperrors.AppError { return unknownActivityDirection("sideways") }},
		{"followRelationshipCreationFailed", func() *apperrors.AppError { return followRelationshipCreationFailed(baseErr) }},
		{"relationshipStatusUpdateFailed", func() *apperrors.AppError { return relationshipStatusUpdateFailed(baseErr) }},
		{"rejectedRelationshipDeletionFailed", func() *apperrors.AppError { return rejectedRelationshipDeletionFailed(baseErr) }},
		{"noteExtractionFailed", func() *apperrors.AppError { return noteExtractionFailed(baseErr) }},
		{"statusCreationFailed", func() *apperrors.AppError { return statusCreationFailed(baseErr) }},
		{"objectStorageFailed", func() *apperrors.AppError { return objectStorageFailed(baseErr) }},
		{"unsupportedObjectType", func() *apperrors.AppError { return unsupportedObjectType("Foo") }},
		{"tombstoneCreationFailed", func() *apperrors.AppError { return tombstoneCreationFailed(baseErr) }},
		{"likeRecordCreationFailed", func() *apperrors.AppError { return likeRecordCreationFailed(baseErr) }},
		{"announceRecordCreationFailed", func() *apperrors.AppError { return announceRecordCreationFailed(baseErr) }},
		{"originalActivityFetchFailed", func() *apperrors.AppError {
			return originalActivityFetchFailed("https://example.com/activities/1", baseErr)
		}},
		{"activityNotFoundLocally", func() *apperrors.AppError { return activityNotFoundLocally("https://example.com/activities/1") }},
		{"followRelationshipDeletionFailed", func() *apperrors.AppError { return followRelationshipDeletionFailed(baseErr) }},
		{"createdObjectDeletionFailed", func() *apperrors.AppError { return createdObjectDeletionFailed(baseErr) }},
		{"objectHistoryRetrievalFailed", func() *apperrors.AppError { return objectHistoryRetrievalFailed(baseErr) }},
		{"noHistoryFound", func() *apperrors.AppError { return noHistoryFound("obj") }},
		{"previousStateNotAvailable", func() *apperrors.AppError { return previousStateNotAvailable("obj") }},
		{"objectReversionFailed", func() *apperrors.AppError { return objectReversionFailed(baseErr) }},
		{"tombstoneStatusCheckFailed", func() *apperrors.AppError { return tombstoneStatusCheckFailed(baseErr) }},
		{"objectNotDeleted", func() *apperrors.AppError { return objectNotDeleted("obj") }},
		{"tombstoneRetrievalFailed", func() *apperrors.AppError { return tombstoneRetrievalFailed(baseErr) }},
		{"objectHistoryRestorationFailed", func() *apperrors.AppError { return objectHistoryRestorationFailed(baseErr) }},
		{"objectRestorationFailed", func() *apperrors.AppError { return objectRestorationFailed(baseErr) }},
		{"flagsRetrievalFailed", func() *apperrors.AppError { return flagsRetrievalFailed(baseErr) }},
		{"flagRecordDeletionFailed", func() *apperrors.AppError { return flagRecordDeletionFailed(baseErr) }},
		{"usernameExtractionFromActorURIFailed", func() *apperrors.AppError { return usernameExtractionFromActorURIFailed("not-a-url") }},
		{"movedToFieldClearingFailed", func() *apperrors.AppError { return movedToFieldClearingFailed(baseErr) }},
		{"objectIDExtractionFromActivityFailed", objectIDExtractionFromActivityFailed},
		{"undoActivityMissingActor", undoActivityMissingActor},
		{"targetListRetrievalFailed", func() *apperrors.AppError { return targetListRetrievalFailed(baseErr) }},
		{"listOperationFailed", func() *apperrors.AppError { return listOperationFailed(baseErr) }},
		{"activityMissingTargetCollection", activityMissingTargetCollection},
		{"objectExtractionFromActivityFailed", objectExtractionFromActivityFailed},
		{"noObjectsFoundInActivity", noObjectsFoundInActivity},
		{"blockRelationshipCreationFailed", func() *apperrors.AppError { return blockRelationshipCreationFailed(baseErr) }},
		{"flagRecordCreationFailed", func() *apperrors.AppError { return flagRecordCreationFailed(baseErr) }},
		{"usernameExtractionFromOldActorURIFailed", func() *apperrors.AppError { return usernameExtractionFromOldActorURIFailed("not-a-url") }},
		{"movedToFieldUpdateFailed", func() *apperrors.AppError { return movedToFieldUpdateFailed(baseErr) }},
		{"targetIDExtractionFromActivityFailed", targetIDExtractionFromActivityFailed},
		{"activityRecordDeletionFailed", func() *apperrors.AppError { return activityRecordDeletionFailed(baseErr) }},
		{"usernameExtractionFromActorIDFailed", func() *apperrors.AppError { return usernameExtractionFromActorIDFailed("not-a-url") }},
		{"followersRetrievalFailed", func() *apperrors.AppError { return followersRetrievalFailed(baseErr) }},
		{"objectValidationFailed", func() *apperrors.AppError { return objectValidationFailed("Note", "bad") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn()
			require.NotNil(t, got)
			require.NotEmpty(t, got.Error())
		})
	}
}
