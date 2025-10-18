package services

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Activity processing errors
var (
	// ErrGetEntityType is returned when entity type extraction fails
	ErrGetEntityType = func() error { return errors.EntityTypeExtractionFailed(nil) }()

	// ErrUnmarshalActivityRecord is returned when activity record unmarshaling fails
	ErrUnmarshalActivityRecord = func() error { return errors.UnmarshalingFailed("activity record", nil) }()

	// ErrParseActivity is returned when activity parsing fails
	ErrParseActivity = func() error { return errors.ActivityParsingFailed("activity", nil) }()

	// ErrUnknownActivityDirection is returned when activity direction is not recognized
	ErrUnknownActivityDirection = func() error { return errors.ActivityDirectionUnknown("") }()

	// Follow activity errors
	// ErrFollowMissingObjectID is returned when follow activity object is missing id field
	ErrFollowMissingObjectID = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrFollowInvalidObjectType is returned when follow activity has invalid object type
	ErrFollowInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for follow activity") }()

	// ErrFollowMissingTargetUser is returned when follow activity is missing target user
	ErrFollowMissingTargetUser = func() error { return errors.ActivityMissingField("object") }()

	// ErrExtractUsernamesFromFollow is returned when username extraction from Follow activity fails
	ErrExtractUsernamesFromFollow = func() error { return errors.UsernameExtractionFailed("follow activity", nil) }()

	// ErrCreateFollowRelationship is returned when creating follow relationship fails
	ErrCreateFollowRelationship = func() error { return errors.FailedToCreate("follow relationship", nil) }()

	// Accept activity errors
	// ErrAcceptInvalidObjectType is returned when accept activity has invalid object type
	ErrAcceptInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for accept activity") }()

	// ErrExtractUsernamesFromAccept is returned when username extraction from Accept activity fails
	ErrExtractUsernamesFromAccept = func() error { return errors.UsernameExtractionFailed("accept activity", nil) }()

	// ErrUpdateRelationshipStatus is returned when updating relationship status fails
	ErrUpdateRelationshipStatus = func() error { return errors.FailedToUpdate("relationship status", nil) }()

	// Create activity errors
	// ErrExtractNote is returned when note extraction from create activity fails
	ErrExtractNote = func() error { return errors.ObjectParsingFailed("note", nil) }()

	// ErrCreateStatus is returned when status creation fails
	ErrCreateStatus = func() error { return errors.FailedToCreate("status", nil) }()

	// ErrStoreObject is returned when object storage fails
	ErrStoreObject = func() error { return errors.FailedToStore("object", nil) }()

	// ErrUnsupportedObjectType is returned when object type is not supported
	ErrUnsupportedObjectType = func() error { return errors.ObjectTypeUnsupported("") }()

	// ErrCreateTimelineEntries is returned when timeline entry creation fails
	ErrCreateTimelineEntries = func() error { return errors.FailedToCreate("timeline entries", nil) }()

	// Reject activity errors
	// ErrRejectInvalidObjectType is returned when reject activity has invalid object type
	ErrRejectInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for reject activity") }()

	// ErrExtractUsernamesFromReject is returned when username extraction from Reject activity fails
	ErrExtractUsernamesFromReject = func() error { return errors.UsernameExtractionFailed("reject activity", nil) }()

	// ErrDeleteRejectedRelationship is returned when deleting rejected relationship fails
	ErrDeleteRejectedRelationship = func() error { return errors.FailedToDelete("rejected relationship", nil) }()

	// Delete activity errors
	// ErrDeleteMissingObjectID is returned when delete activity object is missing id field
	ErrDeleteMissingObjectID = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrDeleteInvalidObjectType is returned when delete activity has invalid object type
	ErrDeleteInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for delete activity") }()

	// ErrDeleteMissingObjectID2 is returned when delete activity is missing object ID
	ErrDeleteMissingObjectID2 = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrActorNotAuthorizedDelete is returned when actor is not authorized to delete object
	ErrActorNotAuthorizedDelete = func() error { return errors.DeleteUnauthorized("", "") }()

	// ErrCreateTombstone is returned when tombstone creation fails
	ErrCreateTombstone = func() error { return errors.FailedToCreate("tombstone", nil) }()

	// Like activity errors
	// ErrLikeMissingObjectID is returned when like activity object is missing id field
	ErrLikeMissingObjectID = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrLikeInvalidObjectType is returned when like activity has invalid object type
	ErrLikeInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for like activity") }()

	// ErrLikeMissingObjectID2 is returned when like activity is missing object ID
	ErrLikeMissingObjectID2 = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrLikeMissingActor is returned when like activity is missing actor
	ErrLikeMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrCreateLikeRecord is returned when creating like record fails
	ErrCreateLikeRecord = func() error { return errors.FailedToCreate("like record", nil) }()

	// Announce activity errors
	// ErrAnnounceMissingObjectID is returned when announce activity object is missing id field
	ErrAnnounceMissingObjectID = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrAnnounceInvalidObjectType is returned when announce activity has invalid object type
	ErrAnnounceInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for announce activity") }()

	// ErrAnnounceMissingObjectID2 is returned when announce activity is missing object ID
	ErrAnnounceMissingObjectID2 = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrAnnounceMissingActor is returned when announce activity is missing actor
	ErrAnnounceMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrCreateAnnounceRecord is returned when creating announce record fails
	ErrCreateAnnounceRecord = func() error { return errors.FailedToCreate("announce record", nil) }()

	// Undo activity errors
	// ErrFetchOriginalActivity is returned when fetching original activity fails
	ErrFetchOriginalActivity = func() error { return errors.OriginalActivityFetchFailed(nil) }()

	// ErrUndoInvalidObjectType is returned when undo activity has invalid object type
	ErrUndoInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for undo activity") }()

	// ErrExtractActivityTypeFromUndo is returned when extracting activity type from undo target fails
	ErrExtractActivityTypeFromUndo = func() error { return errors.ActivityTypeExtractionFailed("undo target", nil) }()

	// ErrActorNotAuthorizedUndo is returned when actor is not authorized to undo activity
	ErrActorNotAuthorizedUndo = func() error { return errors.UndoUnauthorized("", "") }()

	// ErrActivityNotFoundLocally is returned when activity is not found locally
	ErrActivityNotFoundLocally = func() error { return errors.ActivityNotFoundLocally("") }()

	// ErrExtractTargetActorFromFollow is returned when extracting target actor from follow activity fails
	ErrExtractTargetActorFromFollow = func() error { return errors.ActorURIExtractionFailed("follow activity target", nil) }()

	// ErrDeleteFollowRelationship is returned when deleting follow relationship fails
	ErrDeleteFollowRelationship = func() error { return errors.FailedToDelete("follow relationship", nil) }()

	// ErrExtractObjectIDFromCreate is returned when extracting object ID from create activity fails
	ErrExtractObjectIDFromCreate = func() error { return errors.ObjectIDExtractionFailed("create activity", nil) }()

	// ErrUndoCreateMissingActor is returned when undo create activity is missing actor
	ErrUndoCreateMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrDeleteCreatedObject is returned when deleting created object fails
	ErrDeleteCreatedObject = func() error { return errors.FailedToDelete("created object", nil) }()

	// ErrExtractObjectIDFromUpdate is returned when extracting object ID from update activity fails
	ErrExtractObjectIDFromUpdate = func() error { return errors.ObjectIDExtractionFailed("update activity", nil) }()

	// ErrUndoUpdateMissingActor is returned when undo update activity is missing actor
	ErrUndoUpdateMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrGetObjectHistory is returned when getting object history fails
	ErrGetObjectHistory = func() error { return errors.FailedToGet("object history", nil) }()

	// ErrNoHistoryFound is returned when no history is found for object
	ErrNoHistoryFound = func() error { return errors.ObjectHistoryNotFound("") }()

	// ErrPreviousStateNotAvailable is returned when previous state is not available for object
	ErrPreviousStateNotAvailable = func() error { return errors.PreviousStateNotAvailable("") }()

	// ErrRevertObject is returned when reverting object fails
	ErrRevertObject = func() error { return errors.FailedToUpdate("object reversion", nil) }()

	// ErrExtractObjectIDFromDelete is returned when extracting object ID from delete activity fails
	ErrExtractObjectIDFromDelete = func() error { return errors.ObjectIDExtractionFailed("delete activity", nil) }()

	// ErrUndoDeleteMissingActor is returned when undo delete activity is missing actor
	ErrUndoDeleteMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrCheckTombstoneStatus is returned when checking tombstone status fails
	ErrCheckTombstoneStatus = func() error { return errors.TombstoneStatusCheckFailed("", nil) }()

	// ErrObjectNotDeleted is returned when object is not deleted
	ErrObjectNotDeleted = func() error { return errors.ObjectNotDeleted("") }()

	// ErrGetTombstone is returned when getting tombstone fails
	ErrGetTombstone = func() error { return errors.FailedToGet("tombstone", nil) }()

	// ErrGetObjectHistoryForRestoration is returned when getting object history for restoration fails
	ErrGetObjectHistoryForRestoration = func() error { return errors.FailedToGet("object history for restoration", nil) }()

	// ErrNoPreviousStateForRestoration is returned when no previous state is available for restoration
	ErrNoPreviousStateForRestoration = func() error { return errors.NoPreviousStateForRestoration("") }()

	// ErrRestoreObject is returned when restoring object fails
	ErrRestoreObject = func() error { return errors.FailedToUpdate("object restoration", nil) }()

	// ErrExtractOriginalActivityIDFromAccept is returned when extracting original activity ID from accept activity fails
	ErrExtractOriginalActivityIDFromAccept = func() error { return errors.ObjectIDExtractionFailed("accept activity", nil) }()

	// ErrUndoAcceptMissingActor is returned when undo accept activity is missing actor
	ErrUndoAcceptMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrExtractFlaggedObjectIDFromFlag is returned when extracting flagged object ID from flag activity fails
	ErrExtractFlaggedObjectIDFromFlag = func() error { return errors.ObjectIDExtractionFailed("flag activity", nil) }()

	// ErrUndoFlagMissingActor is returned when undo flag activity is missing actor
	ErrUndoFlagMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrRetrieveFlagsForObject is returned when retrieving flags for object fails
	ErrRetrieveFlagsForObject = func() error { return errors.FailedToGet("flags for object", nil) }()

	// ErrDeleteFlagRecord is returned when deleting flag record fails
	ErrDeleteFlagRecord = func() error { return errors.FailedToDelete("flag record", nil) }()

	// ErrExtractMovedToTargetFromMove is returned when extracting moved-to target from move activity fails
	ErrExtractMovedToTargetFromMove = func() error { return errors.ActorURIExtractionFailed("move activity target", nil) }()

	// ErrUndoMoveMissingActor is returned when undo move activity is missing actor
	ErrUndoMoveMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrExtractUsernameFromActorURI is returned when extracting username from actor URI fails
	ErrExtractUsernameFromActorURI = func() error { return errors.UsernameExtractionFailed("actor URI", nil) }()

	// ErrClearMovedToField is returned when clearing movedTo field fails
	ErrClearMovedToField = func() error { return errors.FailedToUpdate("movedTo field", nil) }()

	// ErrExtractObjectIDFromActivity is returned when extracting object ID from activity fails
	ErrExtractObjectIDFromActivity = func() error { return errors.ObjectIDExtractionFailed("activity", nil) }()

	// ErrUndoActivityMissingActor is returned when undo activity is missing actor
	ErrUndoActivityMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrExtractListIDFromTargetCollection is returned when extracting list ID from target collection fails
	ErrExtractListIDFromTargetCollection = func() error { return errors.ObjectIDExtractionFailed("target collection", nil) }()

	// ErrGetTargetList is returned when getting target list fails
	ErrGetTargetList = func() error { return errors.FailedToGet("target list", nil) }()

	// ErrActorNoPermissionModifyList is returned when actor does not have permission to modify list
	ErrActorNoPermissionModifyList = func() error { return errors.InsufficientPermissions("modify list") }()

	// ErrExtractUsernameFromObjectID is returned when extracting username from object ID fails
	ErrExtractUsernameFromObjectID = func() error { return errors.UsernameExtractionFailed("object ID", nil) }()

	// ErrPerformListOperation is returned when list operation fails
	ErrPerformListOperation = func() error { return errors.ProcessingFailed("list operation", stdErrors.New("list operation failed")) }()

	// Block activity errors
	// ErrBlockMissingObjectID is returned when block activity object is missing id field
	ErrBlockMissingObjectID = func() error { return errors.ActivityMissingField("object.id") }()

	// ErrBlockInvalidObjectType is returned when block activity has invalid object type
	ErrBlockInvalidObjectType = func() error { return errors.ActivityInvalidField("object.type", "invalid type for block activity") }()

	// ErrBlockMissingBlockedActor is returned when block activity is missing blocked actor
	ErrBlockMissingBlockedActor = func() error { return errors.ActivityMissingField("object") }()

	// ErrBlockMissingBlockerActor is returned when block activity is missing blocker actor
	ErrBlockMissingBlockerActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrCreateBlockRelationship is returned when creating block relationship fails
	ErrCreateBlockRelationship = func() error { return errors.FailedToCreate("block relationship", nil) }()

	// Flag activity errors
	// ErrExtractFlaggedObjectFromFlag is returned when extracting flagged object from Flag activity fails
	ErrExtractFlaggedObjectFromFlag = func() error { return errors.ObjectIDExtractionFailed("flag activity", nil) }()

	// ErrNoFlaggedObjectsFound is returned when no flagged objects are found in Flag activity
	ErrNoFlaggedObjectsFound = func() error { return errors.FlaggedObjectsNotFound() }()

	// ErrCreateFlagRecord is returned when creating flag record fails
	ErrCreateFlagRecord = func() error { return errors.FailedToCreate("flag record", nil) }()

	// Move activity errors
	// ErrMoveMustSpecifyTarget is returned when move activity must specify a target account
	ErrMoveMustSpecifyTarget = func() error { return errors.MoveTargetMustBeSpecified() }()

	// ErrExtractUsernameFromOldActorURI is returned when extracting username from old actor URI fails
	ErrExtractUsernameFromOldActorURI = func() error { return errors.UsernameExtractionFailed("old actor URI", nil) }()

	// ErrUpdateMovedToField is returned when updating movedTo field fails
	ErrUpdateMovedToField = func() error { return errors.FailedToUpdate("movedTo field", nil) }()

	// Collection activity errors (Add/Remove)
	// ErrActivityMissingTargetCollection is returned when activity is missing target collection
	ErrActivityMissingTargetCollection = func() error { return errors.TargetCollectionMissing() }()

	// ErrExtractObjectFromActivity is returned when extracting object from activity fails
	ErrExtractObjectFromActivity = func() error { return errors.ObjectIDExtractionFailed("activity object", nil) }()

	// ErrNoObjectsFoundInActivity is returned when no objects are found in activity
	ErrNoObjectsFoundInActivity = func() error { return errors.ObjectsNotFoundInActivity() }()

	// Target-specific activity errors
	// ErrExtractTargetIDFromActivity is returned when extracting target ID from activity fails
	ErrExtractTargetIDFromActivity = func() error { return errors.TargetIDExtractionFailed("activity", nil) }()

	// ErrDeleteActivityRecord is returned when deleting activity record fails
	ErrDeleteActivityRecord = func() error { return errors.FailedToDelete("activity record", nil) }()

	// Timeline processing errors
	// ErrExtractUsernameFromActorID is returned when extracting username from actor ID fails
	ErrExtractUsernameFromActorID = func() error { return errors.UsernameExtractionFailed("actor ID", nil) }()

	// ErrGetFollowers is returned when getting followers fails
	ErrGetFollowers = func() error { return errors.FailedToGet("followers", nil) }()

	// Federation service errors
	// ErrUnsupportedDatabaseType is returned when database type is not supported
	ErrUnsupportedDatabaseType = func() error { return errors.DatabaseTypeUnsupported("") }()

	// ErrNoDatabaseAvailable is returned when no database is available
	ErrNoDatabaseAvailable = func() error { return errors.NoDatabaseAvailable() }()

	// ErrActivityMissingActor is returned when activity is missing actor
	ErrActivityMissingActor = func() error { return errors.ActivityMissingField("actor") }()

	// ErrInvalidActorIDFormat is returned when actor ID format is invalid
	ErrInvalidActorIDFormat = func() error { return errors.ActorIDFormatInvalid("") }()

	// Job queue service errors
	// ErrAWSConfigLoad is returned when AWS config loading fails
	ErrAWSConfigLoad = func() error { return errors.ConnectionFailed("AWS config", nil) }()

	// ErrImportJobSerialization is returned when import job serialization fails
	ErrImportJobSerialization = func() error { return errors.MarshalingFailed("import job", nil) }()

	// ErrImportJobQueue is returned when import job queuing fails
	ErrImportJobQueue = func() error {
		return errors.ProcessingFailed("import job queue", stdErrors.New("import job queue processing failed"))
	}()

	// ErrExportJobSerialization is returned when export job serialization fails
	ErrExportJobSerialization = func() error { return errors.MarshalingFailed("export job", nil) }()

	// ErrExportJobQueue is returned when export job queuing fails
	ErrExportJobQueue = func() error {
		return errors.ProcessingFailed("export job queue", stdErrors.New("export job queue processing failed"))
	}()

	// ErrMediaJobSerialization is returned when media job serialization fails
	ErrMediaJobSerialization = func() error { return errors.MarshalingFailed("media job", nil) }()

	// ErrMediaJobQueue is returned when media job queuing fails
	ErrMediaJobQueue = func() error {
		return errors.ProcessingFailed("media job queue", stdErrors.New("media job queue processing failed"))
	}()

	// ErrScheduledJobSerialization is returned when scheduled job serialization fails
	ErrScheduledJobSerialization = func() error { return errors.MarshalingFailed("scheduled job", nil) }()

	// ErrScheduledJobQueue is returned when scheduled job queuing fails
	ErrScheduledJobQueue = func() error {
		return errors.ProcessingFailed("scheduled job queue", stdErrors.New("scheduled job queue processing failed"))
	}()

	// ErrActivityJobSerialization is returned when activity job serialization fails
	ErrActivityJobSerialization = func() error { return errors.MarshalingFailed("activity job", nil) }()

	// ErrActivityJobQueue is returned when activity job queuing fails
	ErrActivityJobQueue = func() error {
		return errors.ProcessingFailed("activity job queue", stdErrors.New("activity job queue processing failed"))
	}()

	// ErrQueueURLNotConfigured is returned when queue URL is not configured
	ErrQueueURLNotConfigured = func() error { return errors.QueueURLNotConfigured("") }()

	// ErrMessageSerialization is returned when message serialization fails
	ErrMessageSerialization = func() error { return errors.MessageMarshalingFailed("", nil) }()

	// ErrDelayedJobQueue is returned when delayed job queuing fails
	ErrDelayedJobQueue = func() error {
		return errors.ProcessingFailed("delayed job queue", stdErrors.New("delayed job queue processing failed"))
	}()

	// ErrBatchMessageSend is returned when batch message sending fails
	ErrBatchMessageSend = func() error {
		return errors.ProcessingFailed("batch message send", stdErrors.New("batch message send failed"))
	}()

	// ErrBatchOperation is returned when batch operation fails
	ErrBatchOperation = func() error {
		return errors.ProcessingFailed("batch operation", stdErrors.New("batch operation failed"))
	}()

	// ErrQueueAttributeQuery is returned when queue attribute query fails
	ErrQueueAttributeQuery = func() error { return errors.FailedToQuery("queue attributes", nil) }()

	// AWS Queue service specific errors
	// ErrImportExportQueueURLNotConfigured is returned when ImportExportQueueURL is not configured
	ErrImportExportQueueURLNotConfigured = func() error { return errors.QueueURLNotConfigured("ImportExportQueueURL") }()

	// ErrSQSConnectFailed is returned when SQS queue connection fails
	ErrSQSConnectFailed = func() error { return errors.SQSConnectionFailed(nil) }()

	// ErrQueueMessageMarshalFailed is returned when queue message marshaling fails
	ErrQueueMessageMarshalFailed = func() error { return errors.MessageMarshalingFailed("queue message", nil) }()

	// ErrSQSMessageSendFailed is returned when SQS message sending fails
	ErrSQSMessageSendFailed = func() error { return errors.SQSMessageSendFailed(nil) }()

	// Notes service errors
	// ErrNotesValidationFailed is returned when notes validation fails
	ErrNotesValidationFailed = func() error { return errors.ValidationFailedWithField("notes") }()

	// ErrGetAuthorAccount is returned when getting author account fails
	ErrGetAuthorAccount = func() error { return errors.FailedToGet("author account", nil) }()

	// ErrGetStatus is returned when status retrieval fails
	ErrGetStatus = func() error { return errors.FailedToGet("status", nil) }()

	// ErrUpdateStatus is returned when status update fails
	ErrUpdateStatus = func() error { return errors.FailedToUpdate("status", nil) }()

	// ErrDeleteStatus is returned when status deletion fails
	ErrDeleteStatus = func() error { return errors.FailedToDelete("status", nil) }()

	// ErrStatusNotFound is returned when status is not found
	ErrStatusNotFound = func() error { return errors.NotFound("status") }()

	// ErrStatusIDRequired is returned when status ID is required but missing
	ErrStatusIDRequired = func() error { return errors.RequiredFieldMissing("status_id") }()

	// ErrCheckViewPermissions is returned when checking view permissions fails
	ErrCheckViewPermissions = func() error {
		return errors.ProcessingFailed("view permissions check", stdErrors.New("view permissions check failed"))
	}()

	// ErrCheckFollowingRelationship is returned when checking following relationship fails
	ErrCheckFollowingRelationship = func() error {
		return errors.ProcessingFailed("following relationship check", stdErrors.New("following relationship check failed"))
	}()

	// ErrHomeTimelineRequiresViewerID is returned when home timeline requires viewer_id
	ErrHomeTimelineRequiresViewerID = func() error { return errors.TimelineRequiresField("home", "viewer_id") }()

	// ErrUserTimelineRequiresAuthorID is returned when user timeline requires author_id
	ErrUserTimelineRequiresAuthorID = func() error { return errors.TimelineRequiresField("user", "author_id") }()

	// ErrConversationsTimelineRequiresConversationID is returned when conversations timeline requires conversation_id
	ErrConversationsTimelineRequiresConversationID = func() error { return errors.TimelineRequiresField("conversations", "conversation_id") }()

	// ErrDirectTimelineRequiresViewerID is returned when direct timeline requires viewer_id
	ErrDirectTimelineRequiresViewerID = func() error { return errors.TimelineRequiresField("direct", "viewer_id") }()

	// ErrHashtagTimelineRequiresHashtag is returned when hashtag timeline requires hashtag
	ErrHashtagTimelineRequiresHashtag = func() error { return errors.TimelineRequiresField("hashtag", "hashtag") }()

	// ErrListTimelineRequiresListID is returned when list timeline requires list_id
	ErrListTimelineRequiresListID = func() error { return errors.TimelineRequiresField("list", "list_id") }()

	// ErrUnsupportedTimelineType is returned when timeline type is unsupported
	ErrUnsupportedTimelineType = func() error { return errors.UnsupportedTimelineType("") }()

	// ErrGetTimeline is returned when timeline retrieval fails
	ErrGetTimeline = func() error { return errors.FailedToGet("timeline", nil) }()

	// ErrStatusContentValidationFailed is returned when status content validation fails
	ErrStatusContentValidationFailed = func() error { return errors.ContentValidationFailed("status content", "validation failed") }()

	// ErrVisibilityValidationFailed is returned when visibility validation fails
	ErrVisibilityValidationFailed = func() error { return errors.ContentValidationFailed("visibility", "validation failed") }()

	// ErrSpoilerTextValidationFailed is returned when spoiler text validation fails
	ErrSpoilerTextValidationFailed = func() error { return errors.ContentValidationFailed("spoiler text", "validation failed") }()

	// ErrInvalidInReplyToID is returned when in_reply_to_id is invalid
	ErrInvalidInReplyToID = func() error { return errors.ContentValidationFailed("in_reply_to_id", "invalid format") }()

	// ErrContentCannotBeEmpty is returned when content cannot be empty
	ErrContentCannotBeEmpty = func() error { return errors.RequiredFieldMissing("content") }()

	// ErrContentTooLong is returned when content is too long
	ErrContentTooLong = func() error { return errors.ContentValidationFailed("content", "too long (max 5000 characters)") }()

	// ErrContentTooLongShort is returned when content is too long for short form
	ErrContentTooLongShort = func() error { return errors.ContentValidationFailed("content", "too long (max 500 characters)") }()

	// ErrBookmarkStatus is returned when bookmarking status fails
	ErrBookmarkStatus = func() error { return errors.FailedToCreate("status bookmark", nil) }()

	// ErrUnbookmarkStatus is returned when unbookmarking status fails
	ErrUnbookmarkStatus = func() error { return errors.FailedToDelete("status bookmark", nil) }()

	// ErrGetBookmarks is returned when getting bookmarks fails
	ErrGetBookmarks = func() error { return errors.FailedToGet("bookmarks", nil) }()

	// ErrGetLikers is returned when getting likers fails
	ErrGetLikers = func() error { return errors.FailedToGet("likers", nil) }()

	// ErrGetRebloggerAccount is returned when getting reblogger account fails
	ErrGetRebloggerAccount = func() error { return errors.FailedToGet("reblogger account", nil) }()

	// ErrReblogStatus is returned when reblogging status fails
	ErrReblogStatus = func() error { return errors.FailedToCreate("status reblog", nil) }()

	// ErrGetRebloggers is returned when getting rebloggers fails
	ErrGetRebloggers = func() error { return errors.FailedToGet("rebloggers", nil) }()

	// ErrMuteStatus is returned when muting status fails
	ErrMuteStatus = func() error { return errors.FailedToUpdate("status mute", nil) }()

	// ErrUnmuteStatus is returned when unmuting status fails
	ErrUnmuteStatus = func() error { return errors.FailedToUpdate("status unmute", nil) }()

	// ErrCreateLike is returned when creating like fails
	ErrCreateLike = func() error { return errors.FailedToCreate("like", nil) }()

	// ErrDeleteLike is returned when deleting like fails
	ErrDeleteLike = func() error { return errors.FailedToDelete("like", nil) }()

	// ErrGetLikes is returned when getting likes fails
	ErrGetLikes = func() error { return errors.FailedToGet("likes", nil) }()

	// ErrCreateReblog is returned when creating reblog fails
	ErrCreateReblog = func() error { return errors.FailedToCreate("reblog", nil) }()

	// ErrDeleteReblog is returned when deleting reblog fails
	ErrDeleteReblog = func() error { return errors.FailedToDelete("reblog", nil) }()

	// ErrGetAnnounces is returned when getting announces fails
	ErrGetAnnounces = func() error { return errors.FailedToGet("announces", nil) }()

	// ErrPinStatus is returned when pinning status fails
	ErrPinStatus = func() error { return errors.FailedToUpdate("status pin", nil) }()

	// ErrUnpinStatus is returned when unpinning status fails
	ErrUnpinStatus = func() error { return errors.FailedToUpdate("status unpin", nil) }()

	// ErrConversationServiceNotAvailable is returned when conversation service is not available
	ErrConversationServiceNotAvailable = func() error { return errors.ServiceNotAvailable("conversation service") }()

	// ErrMuteConversation is returned when muting conversation fails
	ErrMuteConversation = func() error { return errors.FailedToUpdate("conversation mute", nil) }()

	// ErrUnmuteConversation is returned when unmuting conversation fails
	ErrUnmuteConversation = func() error { return errors.FailedToUpdate("conversation unmute", nil) }()

	// ErrGetConversations is returned when getting conversations fails
	ErrGetConversations = func() error { return errors.FailedToGet("conversations", nil) }()

	// ErrViewerIDRequiredForFavoritedTimeline is returned when viewer_id is required for favorited timeline
	ErrViewerIDRequiredForFavoritedTimeline = func() error { return errors.TimelineRequiresField("favorited", "viewer_id") }()

	// ErrGetViewerAccount is returned when getting viewer account fails
	ErrGetViewerAccount = func() error { return errors.FailedToGet("viewer account", nil) }()

	// ErrLikeRepositoryNotAvailable is returned when like repository is not available
	ErrLikeRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("like repository") }()

	// ErrGetLikedObjects is returned when getting liked objects fails
	ErrGetLikedObjects = func() error { return errors.FailedToGet("liked objects", nil) }()

	// ErrGetStatuses is returned when getting statuses fails
	ErrGetStatuses = func() error { return errors.FailedToGet("statuses", nil) }()

	// ErrCreateScheduledStatus is returned when creating scheduled status fails
	ErrCreateScheduledStatus = func() error { return errors.FailedToCreate("scheduled status", nil) }()

	// ErrScheduledTimeInPast is returned when scheduled time must be in the future
	ErrScheduledTimeInPast = func() error { return errors.ScheduledTimeValidationFailed("scheduled time must be in the future") }()

	// ErrGetSearchSuggestions is returned when getting search suggestions fails
	ErrGetSearchSuggestions = func() error { return errors.FailedToGet("search suggestions", nil) }()

	// ErrCreateCommunityNote is returned when creating community note fails
	ErrCreateCommunityNote = func() error { return errors.FailedToCreate("community note", nil) }()

	// ErrGetVisibleCommunityNotes is returned when getting visible community notes fails
	ErrGetVisibleCommunityNotes = func() error { return errors.FailedToGet("visible community notes", nil) }()

	// ErrGetCommunityNote is returned when getting community note fails
	ErrGetCommunityNote = func() error { return errors.FailedToGet("community note", nil) }()

	// ErrCreateCommunityNoteVote is returned when creating community note vote fails
	ErrCreateCommunityNoteVote = func() error { return errors.FailedToCreate("community note vote", nil) }()

	// ErrGetCommunityNotesByAuthor is returned when getting community notes by author fails
	ErrGetCommunityNotesByAuthor = func() error { return errors.FailedToGet("community notes by author", nil) }()

	// ErrCountStatusesByAuthor is returned when counting statuses by author fails
	ErrCountStatusesByAuthor = func() error {
		return errors.ProcessingFailed("status count by author", stdErrors.New("failed to count statuses by author"))
	}()

	// ErrGetUserTimeline is returned when getting user timeline fails
	ErrGetUserTimeline = func() error { return errors.FailedToGet("user timeline", nil) }()

	// ErrCountReplies is returned when counting replies fails
	ErrCountReplies = func() error { return errors.ProcessingFailed("reply count", stdErrors.New("failed to count replies")) }()

	// ErrGetBoostCount is returned when getting boost count fails
	ErrGetBoostCount = func() error {
		return errors.ProcessingFailed("boost count", stdErrors.New("failed to get boost count"))
	}()

	// ErrGetLikeCount is returned when getting like count fails
	ErrGetLikeCount = func() error { return errors.ProcessingFailed("like count", stdErrors.New("failed to get like count")) }()

	// ErrCheckUserHasLiked is returned when checking if user has liked fails
	ErrCheckUserHasLiked = func() error {
		return errors.ProcessingFailed("user has liked check", stdErrors.New("failed to verify if user has liked"))
	}()

	// ErrCheckUserHasReblogged is returned when checking if user has reblogged fails
	ErrCheckUserHasReblogged = func() error {
		return errors.ProcessingFailed("user has reblogged check", stdErrors.New("failed to verify if user has reblogged"))
	}()

	// ErrCheckUserHasBookmarked is returned when checking if user has bookmarked fails
	ErrCheckUserHasBookmarked = func() error {
		return errors.ProcessingFailed("user has bookmarked check", stdErrors.New("failed to verify if user has bookmarked"))
	}()

	// Account service errors
	// ErrValidationFailed is returned when input validation fails
	ErrValidationFailed = func() error { return errors.ValidationFailedWithField("input") }()

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = func() error { return errors.FailedToGet("account", nil) }()

	// ErrUpdateProfile is returned when profile update fails
	ErrUpdateProfile = func() error { return errors.FailedToUpdate("profile", nil) }()

	// ErrStoreAccount is returned when account storage fails
	ErrStoreAccount = func() error { return errors.FailedToStore("account", nil) }()

	// ErrGetPreferences is returned when preferences retrieval fails
	ErrGetPreferences = func() error { return errors.FailedToGet("preferences", nil) }()

	// ErrUpdatePreferences is returned when preferences update fails
	ErrUpdatePreferences = func() error { return errors.FailedToUpdate("preferences", nil) }()

	// ErrAccountNotFound is returned when account is not found
	ErrAccountNotFound = func() error { return errors.NotFound("account") }()

	// ErrSearchAccounts is returned when account search fails
	ErrSearchAccounts = func() error { return errors.ProcessingFailed("account search", stdErrors.New("account search failed")) }()

	// ErrEmptySearchQuery is returned when search query is empty
	ErrEmptySearchQuery = func() error { return errors.RequiredFieldMissing("search query") }()

	// ErrUsernameRequired is returned when username is missing
	ErrUsernameRequired = func() error { return errors.RequiredFieldMissing("username") }()

	// ErrUpdaterIDRequired is returned when updater ID is missing
	ErrUpdaterIDRequired = func() error { return errors.RequiredFieldMissing("updater_id") }()

	// ErrProfileFieldNameEmpty is returned when profile field name is empty
	ErrProfileFieldNameEmpty = func() error { return errors.ContentValidationFailed("profile field name", "cannot be empty") }()

	// ErrProfileFieldNameTooLong is returned when profile field name is too long
	ErrProfileFieldNameTooLong = func() error {
		return errors.ContentValidationFailed("profile field name", "too long (max 255 characters)")
	}()

	// ErrProfileFieldValueTooLong is returned when profile field value is too long
	ErrProfileFieldValueTooLong = func() error {
		return errors.ContentValidationFailed("profile field value", "too long (max 255 characters)")
	}()

	// ErrInvalidExpandMediaSetting is returned when expand media setting is invalid
	ErrInvalidExpandMediaSetting = func() error { return errors.ExpandMediaSettingInvalid("") }()

	// ErrInvalidTimelineOrder is returned when timeline order is invalid
	ErrInvalidTimelineOrder = func() error { return errors.TimelineOrderInvalid("") }()

	// ErrAccountNoActivityPubActor is returned when account has no ActivityPub actor
	ErrAccountNoActivityPubActor = func() error { return errors.AccountNoActivityPubActor("") }()

	// ErrRelationshipRepositoryNotAvailable is returned when relationship repository is not available
	ErrRelationshipRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("relationship repository") }()

	// ErrActorRepositoryNotAvailable is returned when actor repository is not available
	ErrActorRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("actor repository") }()

	// ErrGetActor is returned when actor retrieval fails
	ErrGetActor = func() error { return errors.FailedToGet("actor", nil) }()

	// ErrGetFollowersAccounts is returned when followers retrieval fails
	ErrGetFollowersAccounts = func() error { return errors.FailedToGet("followers accounts", nil) }()

	// ErrGetFollowingList is returned when following list retrieval fails
	ErrGetFollowingList = func() error { return errors.FailedToGet("following list", nil) }()

	// ErrGetViewerActor is returned when viewer actor retrieval fails
	ErrGetViewerActor = func() error { return errors.FailedToGet("viewer actor", nil) }()

	// ErrGetViewerFollowing is returned when viewer following retrieval fails
	ErrGetViewerFollowing = func() error { return errors.FailedToGet("viewer following", nil) }()

	// ErrTargetAccountNotFound is returned when target account is not found
	ErrTargetAccountNotFound = func() error { return errors.NotFound("target account") }()

	// ErrAccountAlreadyPinned is returned when account is already pinned
	ErrAccountAlreadyPinned = func() error { return errors.AccountAlreadyPinned("") }()

	// ErrPinAccount is returned when account pinning fails
	ErrPinAccount = func() error { return errors.FailedToUpdate("account pin", nil) }()

	// ErrUnpinAccount is returned when account unpinning fails
	ErrUnpinAccount = func() error { return errors.FailedToUpdate("account unpin", nil) }()

	// ErrGetAccountPins is returned when account pins retrieval fails
	ErrGetAccountPins = func() error { return errors.FailedToGet("account pins", nil) }()

	// ErrSetAccountNote is returned when account note setting fails
	ErrSetAccountNote = func() error { return errors.FailedToUpdate("account note", nil) }()

	// ErrRemoveFollower is returned when follower removal fails
	ErrRemoveFollower = func() error { return errors.FailedToDelete("follower", nil) }()

	// ErrUsernameAlreadyTaken is returned when username is already taken
	ErrUsernameAlreadyTaken = func() error { return errors.UsernameTaken("") }()

	// ErrGenerateKeypair is returned when keypair generation fails
	ErrGenerateKeypair = func() error { return errors.KeypairGenerationFailed(nil) }()

	// ErrEncodePublicKey is returned when public key encoding fails
	ErrEncodePublicKey = func() error { return errors.PublicKeyEncodingFailed(nil) }()

	// ErrHashPassword is returned when password hashing fails
	ErrHashPassword = func() error { return errors.PasswordHashingFailed(nil) }()

	// ErrCreateAccount is returned when account creation fails
	ErrCreateAccount = func() error { return errors.FailedToCreate("account", nil) }()

	// ErrEmailRequired is returned when email is missing
	ErrEmailRequired = func() error { return errors.EmailRequired() }()

	// ErrMustAgreeToTerms is returned when user doesn't agree to terms
	ErrMustAgreeToTerms = func() error { return errors.MustAgreeToTerms() }()

	// ErrCryptoServiceNotConfigured is returned when crypto service is not configured
	ErrCryptoServiceNotConfigured = func() error { return errors.ServiceNotAvailable("crypto service") }()

	// ErrAuthServiceNotConfigured is returned when auth service is not configured
	ErrAuthServiceNotConfigured = func() error { return errors.ServiceNotAvailable("auth service") }()

	// ErrStorageNotAvailable is returned when storage is not available
	ErrStorageNotAvailable = func() error { return errors.ServiceNotAvailable("storage") }()

	// ErrUserRepositoryNotAvailable is returned when user repository is not available
	ErrUserRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("user repository") }()

	// ErrCheckAccountPinned is returned when checking if account is pinned fails
	ErrCheckAccountPinned = func() error {
		return errors.ProcessingFailed("account pin check", stdErrors.New("account pin check failed"))
	}()

	// ErrGetUser is returned when user retrieval fails
	ErrGetUser = func() error { return errors.FailedToGet("user", nil) }()

	// ErrGetUserPreferences is returned when user preferences retrieval fails
	ErrGetUserPreferences = func() error { return errors.FailedToGet("user preferences", nil) }()

	// ErrDomainBlockRepositoryNotAvailable is returned when domain block repository is not available
	ErrDomainBlockRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("domain block repository") }()

	// ErrCheckDomainBlockedByUser is returned when checking if domain is blocked by user fails
	ErrCheckDomainBlockedByUser = func() error {
		return errors.ProcessingFailed("domain block check", stdErrors.New("domain block check failed"))
	}()

	// ErrAccountRepositoryNotAvailable is returned when account repository is not available
	ErrAccountRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("account repository") }()

	// ErrGetFieldVerification is returned when field verification retrieval fails
	ErrGetFieldVerification = func() error { return errors.FailedToGet("field verification", nil) }()

	// ErrGetAccountNote is returned when account note retrieval fails
	ErrGetAccountNote = func() error { return errors.FailedToGet("account note", nil) }()

	// Business logic service errors
	// ErrStoreScheduledStatus is returned when storing a scheduled status fails
	ErrStoreScheduledStatus = func() error { return errors.FailedToStore("scheduled status", nil) }()

	// ErrEmojiRepositoryNotAvailable is returned when emoji repository is not available
	ErrEmojiRepositoryNotAvailable = func() error { return errors.RepositoryNotAvailable("emoji repository") }()

	// Adapter service errors
	// ErrUnsupportedKeyType is returned when a key type is not supported for cryptographic operations
	ErrUnsupportedKeyType = func() error { return errors.KeyTypeUnsupported("") }()

	// ErrInvalidPrivateKeyType is returned when an invalid private key type is provided
	ErrInvalidPrivateKeyType = func() error { return errors.InvalidPrivateKeyType() }()

	// Storage adapter errors
	// ErrInvalidNotificationType is returned when notification type conversion fails
	ErrInvalidNotificationType = func() error { return errors.NotificationTypeInvalid("") }()

	// ErrGetDomainHealthScore is returned when domain health score retrieval fails
	ErrGetDomainHealthScore = func() error { return errors.DomainHealthScoreRetrievalFailed("", nil) }()

	// ErrUnsupportedStorageType is returned when an unsupported storage type is provided
	ErrUnsupportedStorageType = func() error { return errors.StorageTypeUnsupported("core.RepositoryStorage") }()

	// Registry service errors
	// ErrApplyRegistryOption is returned when applying a registry option fails
	ErrApplyRegistryOption = func() error { return errors.RegistryOptionApplyFailed(nil) }()

	// ErrRegistryValidation is returned when registry validation fails
	ErrRegistryValidation = func() error { return errors.RegistryValidationFailed("") }()

	// ErrStorageCannotBeNil is returned when storage dependency is nil
	ErrStorageCannotBeNil = func() error { return errors.RequiredFieldMissing("storage") }()

	// ErrPublisherCannotBeNil is returned when publisher dependency is nil
	ErrPublisherCannotBeNil = func() error { return errors.RequiredFieldMissing("publisher") }()

	// ErrLoggerCannotBeNil is returned when logger dependency is nil
	ErrLoggerCannotBeNil = func() error { return errors.RequiredFieldMissing("logger") }()

	// ErrConfigCannotBeNil is returned when config dependency is nil
	ErrConfigCannotBeNil = func() error { return errors.RequiredFieldMissing("config") }()

	// ErrStorageRequired is returned when storage is required but not provided
	ErrStorageRequired = func() error { return errors.RequiredFieldMissing("storage") }()

	// ErrEventBusNotInitialized is returned when internal event bus is not initialized
	ErrEventBusNotInitialized = func() error { return errors.EventBusNotInitialized() }()

	// ErrEventBusSubscription is returned when event bus subscription fails
	ErrEventBusSubscription = func() error { return errors.EventBusSubscriptionFailed(nil) }()

	// File validation service errors
	// ErrLoadAWSConfig is returned when AWS config loading fails in file validation
	ErrLoadAWSConfig = func() error { return errors.ConnectionFailed("AWS config", nil) }()

	// ErrFileSizeExceedsLimit is returned when file size exceeds configured limit
	ErrFileSizeExceedsLimit = func() error { return errors.FileSizeExceedsLimit(0, 0) }()

	// ErrContentTypeNotAllowed is returned when file content type is not in allowed list
	ErrContentTypeNotAllowed = func() error { return errors.ContentTypeNotAllowed("") }()

	// ErrFormatNotSupported is returned when file format is not supported
	ErrFormatNotSupported = func() error { return errors.FormatNotSupported("") }()

	// ErrInvalidJSONFormat is returned when JSON format validation fails
	ErrInvalidJSONFormat = func() error { return errors.JSONFormatInvalid("") }()

	// ErrCSVNoContent is returned when CSV file has no content
	ErrCSVNoContent = func() error { return errors.CSVValidationFailed("CSV file has no content") }()

	// ErrCSVNoColumns is returned when CSV file has no columns
	ErrCSVNoColumns = func() error { return errors.CSVValidationFailed("CSV file has no columns") }()

	// ErrCSVTooManyInconsistentRows is returned when CSV has too many inconsistent rows
	ErrCSVTooManyInconsistentRows = func() error { return errors.CSVValidationFailed("CSV file has too many inconsistent rows") }()

	// ErrFileTooMuchBinaryContent is returned when file contains excessive binary content
	ErrFileTooMuchBinaryContent = func() error { return errors.FileValidationFailed("file contains too much binary content") }()

	// ErrJSONStructureTooDeep is returned when JSON structure is too deeply nested
	ErrJSONStructureTooDeep = func() error { return errors.JSONFormatInvalid("JSON structure is too deeply nested") }()

	// ErrCSVRowInconsistentColumns is returned when CSV row has inconsistent column count
	ErrCSVRowInconsistentColumns = func() error { return errors.CSVValidationFailed("CSV row has inconsistent column count") }()

	// ErrSuspiciousContentDetected is returned when potentially suspicious content is detected
	ErrSuspiciousContentDetected = func() error { return errors.FileValidationFailed("potentially suspicious content detected") }()

	// ErrLineTooLong is returned when a line is excessively long
	ErrLineTooLong = func() error { return errors.FileValidationFailed("line is excessively long") }()

	// ErrFileEmpty is returned when file is empty
	ErrFileEmpty = func() error { return errors.FileValidationFailed("file is empty") }()

	// File validation warning constants
	// ErrJSONObjectEmpty is returned when JSON object is empty
	ErrJSONObjectEmpty = func() error { return errors.JSONFormatInvalid("JSON object is empty") }()

	// ErrJSONArrayEmpty is returned when JSON array is empty
	ErrJSONArrayEmpty = func() error { return errors.JSONFormatInvalid("JSON array is empty") }()

	// ErrJSONNotObjectOrArray is returned when JSON is not an object or array
	ErrJSONNotObjectOrArray = func() error { return errors.JSONFormatInvalid("JSON is not an object or array") }()

	// ErrCSVNoDataRows is returned when CSV file has no data rows
	ErrCSVNoDataRows = func() error { return errors.CSVValidationFailed("CSV file has no data rows") }()

	// ErrCSVHeaderMissingImportFields is returned when CSV header does not contain expected import fields
	ErrCSVHeaderMissingImportFields = func() error {
		return errors.CSVValidationFailed("CSV header does not contain expected fields for import")
	}()

	// Quote service errors
	// ErrInvalidQuoteRequest is returned when quote request validation fails
	ErrInvalidQuoteRequest = func() error { return errors.ValidationFailedWithField("quote request") }()

	// ErrGetTargetStatus is returned when getting target status fails
	ErrGetTargetStatus = func() error { return errors.FailedToGet("target status", nil) }()

	// ErrTargetStatusNotFound is returned when target status is not found
	ErrTargetStatusNotFound = func() error { return errors.NotFound("target status") }()

	// ErrTargetStatusNotQuotable is returned when target status is not quotable
	ErrTargetStatusNotQuotable = func() error { return errors.BusinessRuleViolated("target status not quotable", nil) }()

	// ErrCheckQuotePermissions is returned when checking quote permissions fails
	ErrCheckQuotePermissions = func() error {
		return errors.ProcessingFailed("quote permissions check", stdErrors.New("quote permissions check failed"))
	}()

	// ErrNotAuthorizedToQuote is returned when user is not authorized to quote status
	ErrNotAuthorizedToQuote = func() error { return errors.InsufficientPermissions("quote status") }()

	// ErrCreateQuoteStatus is returned when creating quote status fails
	ErrCreateQuoteStatus = func() error { return errors.FailedToCreate("quote status", nil) }()

	// ErrCreateQuoteRelationship is returned when creating quote relationship fails
	ErrCreateQuoteRelationship = func() error { return errors.FailedToCreate("quote relationship", nil) }()

	// ErrGetQuoteRelationships is returned when getting quote relationships fails
	ErrGetQuoteRelationships = func() error { return errors.FailedToGet("quote relationships", nil) }()

	// ErrGetQuoteRelationship is returned when getting quote relationship fails
	ErrGetQuoteRelationship = func() error { return errors.FailedToGet("quote relationship", nil) }()

	// ErrQuoteRelationshipNotFound is returned when quote relationship is not found
	ErrQuoteRelationshipNotFound = func() error { return errors.NotFound("quote relationship") }()

	// ErrNotAuthorizedToDeleteQuote is returned when user is not authorized to delete quote
	ErrNotAuthorizedToDeleteQuote = func() error { return errors.InsufficientPermissions("delete quote") }()

	// ErrWithdrawQuoteRelationship is returned when withdrawing quote relationship fails
	ErrWithdrawQuoteRelationship = func() error { return errors.FailedToDelete("quote relationship", nil) }()

	// ErrGetQuotePermissions is returned when getting quote permissions fails
	ErrGetQuotePermissions = func() error { return errors.FailedToGet("quote permissions", nil) }()

	// ErrCheckExistingPermissions is returned when checking existing permissions fails
	ErrCheckExistingPermissions = func() error {
		return errors.ProcessingFailed("existing permissions check", stdErrors.New("existing permissions check failed"))
	}()

	// ErrSaveQuotePermissions is returned when saving quote permissions fails
	ErrSaveQuotePermissions = func() error { return errors.FailedToSave("quote permissions", nil) }()

	// ErrQuoteContentTooLong is returned when quote content is too long
	ErrQuoteContentTooLong = func() error { return errors.ContentValidationFailed("quote content", "too long") }()

	// Relationship service specific errors
	// ErrCannotFollowSelf is returned when a user tries to follow themselves
	ErrCannotFollowSelf = func() error { return errors.OperationNotAllowedOnSelf("follow") }()

	// ErrCannotBlockSelf is returned when a user tries to block themselves
	ErrCannotBlockSelf = func() error { return errors.OperationNotAllowedOnSelf("block") }()

	// ErrCannotMuteSelf is returned when a user tries to mute themselves
	ErrCannotMuteSelf = func() error { return errors.OperationNotAllowedOnSelf("mute") }()

	// ErrFollowWhileBlocked is returned when trying to follow a user who has blocked you
	ErrFollowWhileBlocked = func() error { return errors.BusinessRuleViolated("cannot follow user: you are blocked", nil) }()

	// ErrFailedToCreateFollowRequest is returned when creating follow request fails
	ErrFailedToCreateFollowRequest = func() error { return errors.FailedToCreate("follow request", nil) }()

	// ErrFailedToAcceptFollowRequest is returned when accepting follow request fails
	ErrFailedToAcceptFollowRequest = func() error { return errors.FailedToUpdate("follow request accept", nil) }()

	// ErrFailedToRejectFollowRequest is returned when rejecting follow request fails
	ErrFailedToRejectFollowRequest = func() error { return errors.FailedToUpdate("follow request reject", nil) }()

	// ErrFailedToGetExistingRelationship is returned when getting existing relationship fails
	ErrFailedToGetExistingRelationship = func() error { return errors.FailedToGet("existing relationship", nil) }()

	// ErrFailedToGetUpdatedRelationship is returned when getting updated relationship fails
	ErrFailedToGetUpdatedRelationship = func() error { return errors.FailedToGet("updated relationship", nil) }()

	// ErrFailedToCheckBlockStatus is returned when checking block status fails
	ErrFailedToCheckBlockStatus = func() error {
		return errors.ProcessingFailed("block status check", stdErrors.New("block status check failed"))
	}()

	// ErrFailedToCheckMuteStatus is returned when checking mute status fails
	ErrFailedToCheckMuteStatus = func() error {
		return errors.ProcessingFailed("mute status check", stdErrors.New("mute status check failed"))
	}()

	// ErrFailedToUnfollow is returned when unfollow operation fails
	ErrFailedToUnfollow = func() error { return errors.FailedToDelete("follow relationship", nil) }()

	// ErrFailedToBlockUser is returned when blocking user fails
	ErrFailedToBlockUser = func() error { return errors.FailedToCreate("user block", nil) }()

	// ErrFailedToUnblockUser is returned when unblocking user fails
	ErrFailedToUnblockUser = func() error { return errors.FailedToDelete("user block", nil) }()

	// ErrFailedToMuteUser is returned when muting user fails
	ErrFailedToMuteUser = func() error { return errors.FailedToCreate("user mute", nil) }()

	// ErrFailedToUnmuteUser is returned when unmuting user fails
	ErrFailedToUnmuteUser = func() error { return errors.FailedToDelete("user mute", nil) }()

	// ErrFailedToBuildRelationshipData is returned when building relationship data fails
	ErrFailedToBuildRelationshipData = func() error {
		return errors.ProcessingFailed("relationship data build", stdErrors.New("relationship data build failed"))
	}()

	// ErrFailedToCountFollowers is returned when counting followers fails
	ErrFailedToCountFollowers = func() error {
		return errors.ProcessingFailed("follower count", stdErrors.New("failed to count followers"))
	}()

	// ErrFailedToCountFollowing is returned when counting following fails
	ErrFailedToCountFollowing = func() error {
		return errors.ProcessingFailed("following count", stdErrors.New("failed to count following"))
	}()

	// ErrFollowRequestNotFound is returned when a follow request is not found
	ErrFollowRequestNotFound = func() error { return errors.NotFound("follow request") }()

	// ErrFailedToAddDomainBlock is returned when adding domain block fails
	ErrFailedToAddDomainBlock = func() error { return errors.FailedToCreate("domain block", nil) }()

	// ErrFailedToRemoveDomainBlock is returned when removing domain block fails
	ErrFailedToRemoveDomainBlock = func() error { return errors.FailedToDelete("domain block", nil) }()

	// ErrFailedToGetDomainBlocks is returned when getting domain blocks fails
	ErrFailedToGetDomainBlocks = func() error { return errors.FailedToGet("domain blocks", nil) }()

	// ErrFailedToGetMutedUsers is returned when getting muted users fails
	ErrFailedToGetMutedUsers = func() error { return errors.FailedToGet("muted users", nil) }()

	// ErrFailedToGetBlockedUsers is returned when getting blocked users fails
	ErrFailedToGetBlockedUsers = func() error { return errors.FailedToGet("blocked users", nil) }()

	// ErrFailedToGetRelationshipUsers is returned when getting relationship users fails
	ErrFailedToGetRelationshipUsers = func() error { return errors.FailedToGet("relationship users", nil) }()

	// ErrFailedToGetPendingFollowRequests is returned when getting pending follow requests fails
	ErrFailedToGetPendingFollowRequests = func() error { return errors.FailedToGet("pending follow requests", nil) }()

	// ErrUnsupportedRelationType is returned when an unsupported relation type is requested
	ErrUnsupportedRelationType = func() error { return errors.ValidationFailedWithField("unsupported relation type") }()

	// ErrNoRepositoryOrStorage is returned when neither repository nor storage is available
	ErrNoRepositoryOrStorage = func() error { return errors.ServiceNotAvailable("repository or storage") }()

	// ErrCannotAcceptOwnFollowRequest is returned when trying to accept your own follow request
	ErrCannotAcceptOwnFollowRequest = func() error { return errors.OperationNotAllowedOnSelf("accept follow request") }()

	// ErrCannotRejectOwnFollowRequest is returned when trying to reject your own follow request
	ErrCannotRejectOwnFollowRequest = func() error { return errors.OperationNotAllowedOnSelf("reject follow request") }()

	// ErrTargetIDsEmpty is returned when target IDs list is empty
	ErrTargetIDsEmpty = func() error { return errors.RequiredFieldMissing("target_ids") }()

	// ErrTooManyTargetIDs is returned when too many target IDs are provided
	ErrTooManyTargetIDs = func() error { return errors.TooManyRequests("target IDs (max 40)") }()

	// Account operation permission errors
	// ErrCannotUpdateProfileForOtherUser is returned when trying to update profile for another user
	ErrCannotUpdateProfileForOtherUser = func() error { return errors.InsufficientPermissions("update profile for another user") }()

	// ErrCannotUpdatePreferencesForOtherUser is returned when trying to update preferences for another user
	ErrCannotUpdatePreferencesForOtherUser = func() error { return errors.InsufficientPermissions("update preferences for another user") }()

	// ErrCannotPinAccountForOtherUser is returned when trying to pin account for another user
	ErrCannotPinAccountForOtherUser = func() error { return errors.InsufficientPermissions("pin account for another user") }()

	// ErrCannotUnpinAccountForOtherUser is returned when trying to unpin account for another user
	ErrCannotUnpinAccountForOtherUser = func() error { return errors.InsufficientPermissions("unpin account for another user") }()

	// ErrCannotSetNoteForOtherUser is returned when trying to set note for another user
	ErrCannotSetNoteForOtherUser = func() error { return errors.InsufficientPermissions("set note for another user") }()

	// ErrCannotRemoveFollowerForOtherUser is returned when trying to remove follower for another user
	ErrCannotRemoveFollowerForOtherUser = func() error { return errors.InsufficientPermissions("remove follower for another user") }()

	// Essential media service errors that are not yet standardized
	// ErrMediaNotFound is returned when media is not found
	ErrMediaNotFound = func() error { return errors.NotFound("media") }()

	// ErrMediaStorageFailed is returned when media storage fails
	ErrMediaStorageFailed = func() error { return errors.FailedToStore("media record", nil) }()

	// ErrMediaRetrievalFailed is returned when media retrieval fails
	ErrMediaRetrievalFailed = func() error { return errors.FailedToGet("media", nil) }()

	// ErrMediaUpdateFailed is returned when media update fails
	ErrMediaUpdateFailed = func() error { return errors.FailedToUpdate("media", nil) }()

	// ErrMediaFileDataRequired is returned when file data is required but missing
	ErrMediaFileDataRequired = func() error { return errors.RequiredFieldMissing("file data") }()

	// ErrMediaFileTooLarge is returned when file size exceeds maximum limit
	ErrMediaFileTooLarge = func() error { return errors.FileSizeExceedsLimit(0, 0) }()

	// ErrMediaUnsupportedType is returned when content type is not supported
	ErrMediaUnsupportedType = func() error { return errors.ContentTypeNotAllowed("") }()

	// ErrMediaFileExtensionMismatch is returned when file extension doesn't match content type
	ErrMediaFileExtensionMismatch = func() error {
		return errors.MediaAttachmentValidationFailed("file extension does not match content type")
	}()

	// ErrMediaNotReady is returned when media is not ready for viewing
	ErrMediaNotReady = func() error { return errors.MediaAttachmentNotReady("") }()

	// ErrMediaProcessingQueueFailed is returned when media processing queue operation fails
	ErrMediaProcessingQueueFailed = func() error {
		return errors.ProcessingFailed("media processing queue", stdErrors.New("media processing queue failed"))
	}()

	// ErrMediaNotReadyForStreaming is returned when media is not ready for streaming
	ErrMediaNotReadyForStreaming = func() error { return errors.MediaAttachmentNotReady("") }()

	// ErrMediaValidationFailed is returned when media validation fails
	ErrMediaValidationFailed = func() error { return errors.MediaAttachmentValidationFailed("validation failed") }()

	// ErrMediaUnauthorizedAccess is returned when unauthorized access to media is attempted
	ErrMediaUnauthorizedAccess = func() error { return errors.InsufficientPermissions("access media") }()

	// List Management errors
	// List CRUD Operations
	// ErrCreateList is returned when list creation fails
	ErrCreateList = func() error { return errors.FailedToCreate("list", nil) }()

	// ErrDeleteList is returned when list deletion fails
	ErrDeleteList = func() error { return errors.FailedToDelete("list", nil) }()

	// ErrUpdateList is returned when list update fails
	ErrUpdateList = func() error { return errors.FailedToUpdate("list", nil) }()

	// ErrGetList is returned when list retrieval fails
	ErrGetList = func() error { return errors.FailedToGet("list", nil) }()

	// ErrListNotFound is returned when list is not found
	ErrListNotFound = func() error { return errors.NotFound("list") }()

	// ErrListAlreadyExists is returned when list already exists
	ErrListAlreadyExists = func() error { return errors.AlreadyExists("list") }()

	// List Membership Management
	// ErrAddListMember is returned when adding list member fails
	ErrAddListMember = func() error { return errors.FailedToCreate("list member", nil) }()

	// ErrRemoveListMember is returned when removing list member fails
	ErrRemoveListMember = func() error { return errors.FailedToDelete("list member", nil) }()

	// ErrGetListMembers is returned when getting list members fails
	ErrGetListMembers = func() error { return errors.FailedToGet("list members", nil) }()

	// ErrMemberNotInList is returned when member is not in list
	ErrMemberNotInList = func() error { return errors.NotFound("member in list") }()

	// ErrMemberAlreadyInList is returned when member is already in list
	ErrMemberAlreadyInList = func() error { return errors.AlreadyExists("member in list") }()

	// List Permissions & Validation
	// ErrInsufficientListPermission is returned when user lacks permission for list operation
	ErrInsufficientListPermission = func() error { return errors.InsufficientPermissions("list operation") }()

	// ErrCannotModifyList is returned when list cannot be modified
	ErrCannotModifyList = func() error { return errors.InsufficientPermissions("modify list") }()

	// ErrListOwnershipRequired is returned when list ownership is required
	ErrListOwnershipRequired = func() error { return errors.InsufficientPermissions("list ownership required") }()

	// ErrInvalidListName is returned when list name is invalid
	ErrInvalidListName = func() error { return errors.ContentValidationFailed("list name", "invalid format") }()

	// ErrListNameTooLong is returned when list name is too long
	ErrListNameTooLong = func() error { return errors.ContentValidationFailed("list name", "too long") }()

	// ErrEmptyListName is returned when list name is empty
	ErrEmptyListName = func() error { return errors.RequiredFieldMissing("list name") }()

	// ErrInvalidListOperation is returned when list operation is invalid
	ErrInvalidListOperation = func() error { return errors.ValidationFailedWithField("list operation") }()

	// Enhanced Media Processing errors
	// Media Upload & Storage
	// ErrUploadMedia is returned when media upload fails
	ErrUploadMedia = func() error { return errors.ProcessingFailed("media upload", stdErrors.New("media upload failed")) }()

	// ErrProcessMedia is returned when media processing fails
	ErrProcessMedia = func() error {
		return errors.ProcessingFailed("media processing", stdErrors.New("media processing failed"))
	}()

	// ErrStoreMedia is returned when media storage fails
	ErrStoreMedia = func() error { return errors.FailedToStore("media", nil) }()

	// ErrRetrieveMedia is returned when media retrieval fails
	ErrRetrieveMedia = func() error { return errors.FailedToGet("media", nil) }()

	// Media Validation
	// ErrInvalidMediaType is returned when media type is invalid
	ErrInvalidMediaType = func() error { return errors.ContentTypeNotAllowed("") }()

	// ErrMediaTooLarge is returned when media file is too large
	ErrMediaTooLarge = func() error { return errors.FileSizeExceedsLimit(0, 0) }()

	// ErrCorruptedMedia is returned when media file is corrupted
	ErrCorruptedMedia = func() error { return errors.FileValidationFailed("corrupted media file") }()

	// ErrUnsupportedFormat is returned when media format is unsupported
	ErrUnsupportedFormat = func() error { return errors.FormatNotSupported("") }()

	// Media Processing
	// ErrTranscodeMedia is returned when media transcoding fails
	ErrTranscodeMedia = func() error {
		return errors.ProcessingFailed("media transcoding", stdErrors.New("media transcoding failed"))
	}()

	// ErrGenerateThumbnail is returned when thumbnail generation fails
	ErrGenerateThumbnail = func() error {
		return errors.ProcessingFailed("thumbnail generation", stdErrors.New("thumbnail generation failed"))
	}()

	// ErrExtractMetadata is returned when metadata extraction fails
	ErrExtractMetadata = func() error {
		return errors.ProcessingFailed("metadata extraction", stdErrors.New("metadata extraction failed"))
	}()

	// ErrCompressionFailed is returned when media compression fails
	ErrCompressionFailed = func() error {
		return errors.ProcessingFailed("media compression", stdErrors.New("media compression failed"))
	}()

	// Enhanced Job Queue Management errors
	// Job Submission
	// ErrSubmitJob is returned when job submission fails
	ErrSubmitJob = func() error { return errors.ProcessingFailed("job submission", stdErrors.New("job submission failed")) }()

	// ErrQueueJob is returned when job queuing fails
	ErrQueueJob = func() error { return errors.ProcessingFailed("job queuing", stdErrors.New("job queuing failed")) }()

	// ErrScheduleJob is returned when job scheduling fails
	ErrScheduleJob = func() error { return errors.ProcessingFailed("job scheduling", stdErrors.New("job scheduling failed")) }()

	// ErrCancelJob is returned when job cancellation fails
	ErrCancelJob = func() error {
		return errors.ProcessingFailed("job cancellation", stdErrors.New("job cancellation failed"))
	}()

	// Job Processing
	// ErrProcessJob is returned when job processing fails
	ErrProcessJob = func() error { return errors.ProcessingFailed("job processing", stdErrors.New("job processing failed")) }()

	// ErrExecuteJob is returned when job execution fails
	ErrExecuteJob = func() error { return errors.ProcessingFailed("job execution", stdErrors.New("job execution failed")) }()

	// ErrCompleteJob is returned when job completion fails
	ErrCompleteJob = func() error { return errors.ProcessingFailed("job completion", stdErrors.New("job completion failed")) }()

	// ErrJobTimeout is returned when job times out
	ErrJobTimeout = func() error { return errors.TimeoutError("job execution") }()

	// Job Validation
	// ErrInvalidJobType is returned when job type is invalid
	ErrInvalidJobType = func() error { return errors.ValidationFailedWithField("job type") }()

	// ErrInvalidJobPayload is returned when job payload is invalid
	ErrInvalidJobPayload = func() error { return errors.ValidationFailedWithField("job payload") }()

	// ErrJobNotFound is returned when job is not found
	ErrJobNotFound = func() error { return errors.NotFound("job") }()

	// ErrJobAlreadyProcessed is returned when job is already processed
	ErrJobAlreadyProcessed = func() error { return errors.AlreadyExists("job already processed") }()

	// Queue Management
	// ErrQueueFull is returned when queue is full
	ErrQueueFull = func() error { return errors.QuotaExceeded("queue", 0) }()

	// ErrQueueUnavailable is returned when queue is unavailable
	ErrQueueUnavailable = func() error { return errors.ServiceUnavailable("queue") }()

	// ErrWorkerUnavailable is returned when worker is unavailable
	ErrWorkerUnavailable = func() error { return errors.ServiceUnavailable("worker") }()

	// ErrRetryLimitExceeded is returned when retry limit is exceeded
	ErrRetryLimitExceeded = func() error { return errors.QuotaExceeded("retry limit", 0) }()

	// Emoji service errors
	// ErrEmojiNotFound is returned when emoji is not found
	ErrEmojiNotFound = func() error { return errors.NotFound("emoji") }()

	// ErrGetEmoji is returned when emoji retrieval fails
	ErrGetEmoji = func() error { return errors.FailedToGet("emoji", nil) }()

	// ErrListEmojis is returned when emoji listing fails
	ErrListEmojis = func() error { return errors.FailedToList("emojis", nil) }()

	// ErrEmojiAlreadyExists is returned when emoji with shortcode already exists
	ErrEmojiAlreadyExists = func() error { return errors.AlreadyExists("emoji") }()

	// ErrCreateEmoji is returned when emoji creation fails
	ErrCreateEmoji = func() error { return errors.FailedToCreate("emoji", nil) }()

	// ErrCannotUpdateRemoteEmoji is returned when attempting to update a remote emoji
	ErrCannotUpdateRemoteEmoji = func() error { return errors.BusinessRuleViolated("cannot update remote emoji", nil) }()

	// ErrUpdateEmoji is returned when emoji update fails
	ErrUpdateEmoji = func() error { return errors.FailedToUpdate("emoji", nil) }()

	// ErrCannotDeleteRemoteEmoji is returned when attempting to delete a remote emoji
	ErrCannotDeleteRemoteEmoji = func() error { return errors.BusinessRuleViolated("cannot delete remote emoji", nil) }()

	// ErrDeleteEmoji is returned when emoji deletion fails
	ErrDeleteEmoji = func() error { return errors.FailedToDelete("emoji", nil) }()

	// ErrRemoteEmojiNotFound is returned when remote emoji is not found
	ErrRemoteEmojiNotFound = func() error { return errors.NotFound("remote emoji") }()

	// ErrGetRemoteEmoji is returned when remote emoji retrieval fails
	ErrGetRemoteEmoji = func() error { return errors.FailedToGet("remote emoji", nil) }()

	// ErrCreateLocalEmojiCopy is returned when creating local emoji copy fails
	ErrCreateLocalEmojiCopy = func() error { return errors.FailedToCreate("local emoji copy", nil) }()

	// ErrSearchEmojis is returned when emoji search fails
	ErrSearchEmojis = func() error { return errors.ProcessingFailed("emoji search", stdErrors.New("emoji search failed")) }()

	// ErrGetPopularEmojis is returned when getting popular emojis fails
	ErrGetPopularEmojis = func() error { return errors.FailedToGet("popular emojis", nil) }()

	// ErrIncrementEmojiUsage is returned when incrementing emoji usage fails
	ErrIncrementEmojiUsage = func() error { return errors.FailedToUpdate("emoji usage", nil) }()

	// ErrInvalidShortcode is returned when shortcode validation fails
	ErrInvalidShortcode = func() error { return errors.ValidationFailedWithField("shortcode") }()

	// ErrReservedShortcode is returned when shortcode is reserved
	ErrReservedShortcode = func() error { return errors.BusinessRuleViolated("shortcode is reserved", nil) }()

	// Conversation service errors
	// ErrConversationValidationFailed is returned when conversation validation fails
	ErrConversationValidationFailed = func() error { return errors.ValidationFailedWithField("conversation") }()

	// ErrGetSenderAccount is returned when getting sender account fails
	ErrGetSenderAccount = func() error { return errors.FailedToGet("sender account", nil) }()

	// ErrInvalidRecipient is returned when recipient validation fails
	ErrInvalidRecipient = func() error { return errors.ValidationFailedWithField("recipient") }()

	// ErrLookupExistingConversation is returned when looking up existing conversation fails
	ErrLookupExistingConversation = func() error { return errors.FailedToGet("existing conversation", nil) }()

	// ErrCreateConversation is returned when conversation creation fails
	ErrCreateConversation = func() error { return errors.FailedToCreate("conversation", nil) }()

	// ErrCreateDirectMessage is returned when direct message creation fails
	ErrCreateDirectMessage = func() error { return errors.FailedToCreate("direct message", nil) }()

	// ErrGetConversation is returned when conversation retrieval fails
	ErrGetConversation = func() error { return errors.FailedToGet("conversation", nil) }()

	// ErrNotConversationParticipant is returned when user is not a participant in conversation
	ErrNotConversationParticipant = func() error { return errors.InsufficientPermissions("not a conversation participant") }()

	// ErrMarkConversationRead is returned when marking conversation as read fails
	ErrMarkConversationRead = func() error { return errors.FailedToUpdate("conversation read status", nil) }()

	// ErrGetUserConversations is returned when getting user conversations fails
	ErrGetUserConversations = func() error { return errors.FailedToGet("user conversations", nil) }()

	// ErrGetConversationMessages is returned when getting conversation messages fails
	ErrGetConversationMessages = func() error { return errors.FailedToGet("conversation messages", nil) }()

	// ErrRecipientsRequired is returned when recipients list is required but empty
	ErrRecipientsRequired = func() error { return errors.RequiredFieldMissing("recipients") }()

	// ErrContentTooLongConversation is returned when conversation content is too long
	ErrContentTooLongConversation = func() error {
		return errors.ContentValidationFailed("conversation content", "too long (max 5000 characters)")
	}()

	// ErrInvalidInReplyToIDConversation is returned when in_reply_to_id is invalid for conversation
	ErrInvalidInReplyToIDConversation = func() error { return errors.ContentValidationFailed("in_reply_to_id", "invalid for conversation") }()

	// ErrCanOnlyReplyToDirectMessages is returned when attempting to reply to non-direct message
	ErrCanOnlyReplyToDirectMessages = func() error { return errors.BusinessRuleViolated("can only reply to direct messages", nil) }()

	// ErrConversationNotFound is returned when conversation is not found
	ErrConversationNotFound = func() error { return errors.NotFound("conversation") }()

	// ErrDeleteConversation is returned when conversation deletion fails
	ErrDeleteConversation = func() error { return errors.FailedToDelete("conversation", nil) }()

	// Cost realtime aggregation service errors
	// ErrStreamProcessingErrors is returned when stream processing completes with errors
	ErrStreamProcessingErrors = func() error {
		return errors.ProcessingFailed("stream processing", stdErrors.New("stream processing failed"))
	}()

	// ErrRecordProcessingFailed is returned when processing records fails
	ErrRecordProcessingFailed = func() error {
		return errors.ProcessingFailed("record processing", stdErrors.New("record processing failed"))
	}()

	// ErrProcessAICostRecord is returned when processing AI cost record fails
	ErrProcessAICostRecord = func() error {
		return errors.ProcessingFailed("AI cost record processing", stdErrors.New("AI cost record processing failed"))
	}()

	// ErrProcessWebSocketCostRecord is returned when processing WebSocket cost record fails
	ErrProcessWebSocketCostRecord = func() error {
		return errors.ProcessingFailed("WebSocket cost record processing", stdErrors.New("WebSocket cost record processing failed"))
	}()

	// ErrProcessFederationCostRecord is returned when processing federation cost record fails
	ErrProcessFederationCostRecord = func() error {
		return errors.ProcessingFailed("federation cost record processing", stdErrors.New("federation cost record processing failed"))
	}()

	// ErrUnmarshalAICostRecord is returned when unmarshaling AI cost record fails
	ErrUnmarshalAICostRecord = func() error { return errors.UnmarshalingFailed("AI cost record", nil) }()

	// ErrUnmarshalWebSocketCostRecord is returned when unmarshaling WebSocket cost record fails
	ErrUnmarshalWebSocketCostRecord = func() error { return errors.UnmarshalingFailed("WebSocket cost record", nil) }()

	// ErrUnmarshalFederationCostRecord is returned when unmarshaling federation cost record fails
	ErrUnmarshalFederationCostRecord = func() error { return errors.UnmarshalingFailed("federation cost record", nil) }()

	// ErrUnsupportedEventType is returned when DynamoDB event type is not supported
	ErrUnsupportedEventType = func() error { return errors.ValidationFailedWithField("unsupported event type") }()

	// ErrMarshalToJSON is returned when marshaling to JSON fails
	ErrMarshalToJSON = func() error { return errors.MarshalingFailed("JSON", nil) }()

	// ErrUnmarshalToTarget is returned when unmarshaling to target fails
	ErrUnmarshalToTarget = func() error { return errors.UnmarshalingFailed("target", nil) }()

	// ErrCreateHourlyAggregation is returned when creating hourly aggregation fails
	ErrCreateHourlyAggregation = func() error { return errors.FailedToCreate("hourly aggregation", nil) }()

	// ErrCreateDailyAggregation is returned when creating daily aggregation fails
	ErrCreateDailyAggregation = func() error { return errors.FailedToCreate("daily aggregation", nil) }()

	// AWS Storage Client errors
	// ErrS3BucketConfigRequired is returned when S3 bucket configuration is missing
	ErrS3BucketConfigRequired = func() error { return errors.RequiredFieldMissing("S3_MEDIA_BUCKET configuration") }()

	// ErrAWSConfigLoadFailed is returned when AWS config loading fails
	ErrAWSConfigLoadFailed = func() error { return errors.ConnectionFailed("AWS config", nil) }()

	// ErrS3BucketAccessFailed is returned when S3 bucket access fails
	ErrS3BucketAccessFailed = func() error { return errors.ConnectionFailed("S3 bucket", nil) }()

	// ErrPresignedURLCreationFailed is returned when presigned URL creation fails
	ErrPresignedURLCreationFailed = func() error {
		return errors.ProcessingFailed("presigned URL creation", stdErrors.New("presigned URL creation failed"))
	}()

	// ErrCannotUploadEmptyData is returned when attempting to upload empty data
	ErrCannotUploadEmptyData = func() error { return errors.ValidationFailedWithField("cannot upload empty data") }()

	// ErrS3UploadFailed is returned when S3 file upload fails
	ErrS3UploadFailed = func() error { return errors.ProcessingFailed("S3 file upload", stdErrors.New("S3 file upload failed")) }()

	// ErrS3DownloadFailed is returned when S3 file download fails
	ErrS3DownloadFailed = func() error {
		return errors.ProcessingFailed("S3 file download", stdErrors.New("S3 file download failed"))
	}()
)
