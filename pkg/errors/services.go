// Package errors defines common error types and constants used across the Lesser ActivityPub service layer.
// It provides centralized error definitions for activity processing, validation, federation operations,
// and various service-specific error conditions.
package errors // nolint:revive // Legacy package name; import with an alias when also using stdlib errors.

import "errors"

// Activity processing errors
var (
	// ErrGetEntityType is returned when entity type extraction fails
	ErrGetEntityType = errors.New("failed to get entity type")

	// ErrUnmarshalActivityRecord is returned when activity record unmarshaling fails
	ErrUnmarshalActivityRecord = errors.New("failed to unmarshal activity record")

	// ErrParseActivity is returned when activity parsing fails
	ErrParseActivity = errors.New("failed to parse activity")

	// ErrUnknownActivityDirection is returned when activity direction is not recognized
	ErrUnknownActivityDirection = errors.New("unknown activity direction")

	// Follow activity errors
	// ErrFollowMissingObjectID is returned when follow activity object is missing id field
	ErrFollowMissingObjectID = errors.New("follow activity object missing id field")

	// ErrFollowInvalidObjectType is returned when follow activity has invalid object type
	ErrFollowInvalidObjectType = errors.New("follow activity has invalid object type")

	// ErrFollowMissingTargetUser is returned when follow activity is missing target user
	ErrFollowMissingTargetUser = errors.New("follow activity missing target user")

	// ErrExtractUsernamesFromFollow is returned when username extraction from Follow activity fails
	ErrExtractUsernamesFromFollow = errors.New("failed to extract usernames from Follow activity")

	// ErrCreateFollowRelationship is returned when creating follow relationship fails
	ErrCreateFollowRelationship = errors.New("failed to create follow relationship")

	// Accept activity errors
	// ErrAcceptInvalidObjectType is returned when accept activity has invalid object type
	ErrAcceptInvalidObjectType = errors.New("accept activity has invalid object type")

	// ErrExtractUsernamesFromAccept is returned when username extraction from Accept activity fails
	ErrExtractUsernamesFromAccept = errors.New("failed to extract usernames from Accept activity")

	// ErrUpdateRelationshipStatus is returned when updating relationship status fails
	ErrUpdateRelationshipStatus = errors.New("failed to update relationship status")

	// Create activity errors
	// ErrExtractNote is returned when note extraction from create activity fails
	ErrExtractNote = errors.New("failed to extract Note")

	// ErrCreateStatus is returned when status creation fails
	ErrCreateStatus = errors.New("failed to create status")

	// ErrStoreObject is returned when object storage fails
	ErrStoreObject = errors.New("failed to store object")

	// ErrUnsupportedObjectType is returned when object type is not supported
	ErrUnsupportedObjectType = errors.New("unsupported object type")

	// ErrCreateTimelineEntries is returned when timeline entry creation fails
	ErrCreateTimelineEntries = errors.New("failed to create timeline entries")

	// Reject activity errors
	// ErrRejectInvalidObjectType is returned when reject activity has invalid object type
	ErrRejectInvalidObjectType = errors.New("reject activity has invalid object type")

	// ErrExtractUsernamesFromReject is returned when username extraction from Reject activity fails
	ErrExtractUsernamesFromReject = errors.New("failed to extract usernames from Reject activity")

	// ErrDeleteRejectedRelationship is returned when deleting rejected relationship fails
	ErrDeleteRejectedRelationship = errors.New("failed to delete rejected relationship")

	// Delete activity errors
	// ErrDeleteMissingObjectID is returned when delete activity object is missing id field
	ErrDeleteMissingObjectID = errors.New("delete activity object missing id field")

	// ErrDeleteInvalidObjectType is returned when delete activity has invalid object type
	ErrDeleteInvalidObjectType = errors.New("delete activity has invalid object type")

	// ErrDeleteMissingObjectID2 is returned when delete activity is missing object ID
	ErrDeleteMissingObjectID2 = errors.New("delete activity missing object ID")

	// ErrActorNotAuthorizedDelete is returned when actor is not authorized to delete object
	ErrActorNotAuthorizedDelete = errors.New("actor not authorized to delete object")

	// ErrCreateTombstone is returned when tombstone creation fails
	ErrCreateTombstone = errors.New("failed to create tombstone")

	// Like activity errors
	// ErrLikeMissingObjectID is returned when like activity object is missing id field
	ErrLikeMissingObjectID = errors.New("like activity object missing id field")

	// ErrLikeInvalidObjectType is returned when like activity has invalid object type
	ErrLikeInvalidObjectType = errors.New("like activity has invalid object type")

	// ErrLikeMissingObjectID2 is returned when like activity is missing object ID
	ErrLikeMissingObjectID2 = errors.New("like activity missing object ID")

	// ErrLikeMissingActor is returned when like activity is missing actor
	ErrLikeMissingActor = errors.New("like activity missing actor")

	// ErrCreateLikeRecord is returned when creating like record fails
	ErrCreateLikeRecord = errors.New("failed to create like record")

	// Announce activity errors
	// ErrAnnounceMissingObjectID is returned when announce activity object is missing id field
	ErrAnnounceMissingObjectID = errors.New("announce activity object missing id field")

	// ErrAnnounceInvalidObjectType is returned when announce activity has invalid object type
	ErrAnnounceInvalidObjectType = errors.New("announce activity has invalid object type")

	// ErrAnnounceMissingObjectID2 is returned when announce activity is missing object ID
	ErrAnnounceMissingObjectID2 = errors.New("announce activity missing object ID")

	// ErrAnnounceMissingActor is returned when announce activity is missing actor
	ErrAnnounceMissingActor = errors.New("announce activity missing actor")

	// ErrCreateAnnounceRecord is returned when creating announce record fails
	ErrCreateAnnounceRecord = errors.New("failed to create announce record")

	// Undo activity errors
	// ErrFetchOriginalActivity is returned when fetching original activity fails
	ErrFetchOriginalActivity = errors.New("failed to fetch original activity")

	// ErrUndoInvalidObjectType is returned when undo activity has invalid object type
	ErrUndoInvalidObjectType = errors.New("undo activity has invalid object type")

	// ErrExtractActivityTypeFromUndo is returned when extracting activity type from undo target fails
	ErrExtractActivityTypeFromUndo = errors.New("failed to extract activity type from undo target")

	// ErrActorNotAuthorizedUndo is returned when actor is not authorized to undo activity
	ErrActorNotAuthorizedUndo = errors.New("actor not authorized to undo activity")

	// ErrActivityNotFoundLocally is returned when activity is not found locally
	ErrActivityNotFoundLocally = errors.New("activity not found locally")

	// ErrExtractTargetActorFromFollow is returned when extracting target actor from follow activity fails
	ErrExtractTargetActorFromFollow = errors.New("unable to extract target actor from follow activity")

	// ErrDeleteFollowRelationship is returned when deleting follow relationship fails
	ErrDeleteFollowRelationship = errors.New("failed to delete follow relationship")

	// ErrExtractObjectIDFromCreate is returned when extracting object ID from create activity fails
	ErrExtractObjectIDFromCreate = errors.New("unable to extract object ID from create activity")

	// ErrUndoCreateMissingActor is returned when undo create activity is missing actor
	ErrUndoCreateMissingActor = errors.New("undo create activity missing actor")

	// ErrDeleteCreatedObject is returned when deleting created object fails
	ErrDeleteCreatedObject = errors.New("failed to delete created object")

	// ErrExtractObjectIDFromUpdate is returned when extracting object ID from update activity fails
	ErrExtractObjectIDFromUpdate = errors.New("unable to extract object ID from update activity")

	// ErrUndoUpdateMissingActor is returned when undo update activity is missing actor
	ErrUndoUpdateMissingActor = errors.New("undo update activity missing actor")

	// ErrGetObjectHistory is returned when getting object history fails
	ErrGetObjectHistory = errors.New("failed to get object history")

	// ErrNoHistoryFound is returned when no history is found for object
	ErrNoHistoryFound = errors.New("no history found for object")

	// ErrPreviousStateNotAvailable is returned when previous state is not available for object
	ErrPreviousStateNotAvailable = errors.New("previous state not available for object")

	// ErrRevertObject is returned when reverting object fails
	ErrRevertObject = errors.New("failed to revert object")

	// ErrExtractObjectIDFromDelete is returned when extracting object ID from delete activity fails
	ErrExtractObjectIDFromDelete = errors.New("unable to extract object ID from delete activity")

	// ErrUndoDeleteMissingActor is returned when undo delete activity is missing actor
	ErrUndoDeleteMissingActor = errors.New("undo delete activity missing actor")

	// ErrCheckTombstoneStatus is returned when checking tombstone status fails
	ErrCheckTombstoneStatus = errors.New("failed to check tombstone status")

	// ErrObjectNotDeleted is returned when object is not deleted
	ErrObjectNotDeleted = errors.New("object is not deleted")

	// ErrGetTombstone is returned when getting tombstone fails
	ErrGetTombstone = errors.New("failed to get tombstone")

	// ErrGetObjectHistoryForRestoration is returned when getting object history for restoration fails
	ErrGetObjectHistoryForRestoration = errors.New("failed to get object history for restoration")

	// ErrNoPreviousStateForRestoration is returned when no previous state is available for restoration
	ErrNoPreviousStateForRestoration = errors.New("no previous state available for restoration")

	// ErrRestoreObject is returned when restoring object fails
	ErrRestoreObject = errors.New("failed to restore object")

	// ErrExtractOriginalActivityIDFromAccept is returned when extracting original activity ID from accept activity fails
	ErrExtractOriginalActivityIDFromAccept = errors.New("unable to extract original activity ID from accept activity")

	// ErrUndoAcceptMissingActor is returned when undo accept activity is missing actor
	ErrUndoAcceptMissingActor = errors.New("undo accept activity missing actor")

	// ErrExtractFlaggedObjectIDFromFlag is returned when extracting flagged object ID from flag activity fails
	ErrExtractFlaggedObjectIDFromFlag = errors.New("unable to extract flagged object ID from flag activity")

	// ErrUndoFlagMissingActor is returned when undo flag activity is missing actor
	ErrUndoFlagMissingActor = errors.New("undo flag activity missing actor")

	// ErrRetrieveFlagsForObject is returned when retrieving flags for object fails
	ErrRetrieveFlagsForObject = errors.New("failed to retrieve flags for object")

	// ErrDeleteFlagRecord is returned when deleting flag record fails
	ErrDeleteFlagRecord = errors.New("failed to delete flag record")

	// ErrExtractMovedToTargetFromMove is returned when extracting moved-to target from move activity fails
	ErrExtractMovedToTargetFromMove = errors.New("unable to extract moved-to target from move activity")

	// ErrUndoMoveMissingActor is returned when undo move activity is missing actor
	ErrUndoMoveMissingActor = errors.New("undo move activity missing actor")

	// ErrExtractUsernameFromActorURI is returned when extracting username from actor URI fails
	ErrExtractUsernameFromActorURI = errors.New("unable to extract username from actor URI")

	// ErrClearMovedToField is returned when clearing movedTo field fails
	ErrClearMovedToField = errors.New("failed to clear movedTo field")

	// ErrExtractObjectIDFromActivity is returned when extracting object ID from activity fails
	ErrExtractObjectIDFromActivity = errors.New("unable to extract object ID from activity")

	// ErrUndoActivityMissingActor is returned when undo activity is missing actor
	ErrUndoActivityMissingActor = errors.New("undo activity missing actor")

	// ErrExtractListIDFromTargetCollection is returned when extracting list ID from target collection fails
	ErrExtractListIDFromTargetCollection = errors.New("unable to extract list ID from target collection")

	// ErrGetTargetList is returned when getting target list fails
	ErrGetTargetList = errors.New("failed to get target list")

	// ErrActorNoPermissionModifyList is returned when actor does not have permission to modify list
	ErrActorNoPermissionModifyList = errors.New("actor does not have permission to modify list")

	// ErrExtractUsernameFromObjectID is returned when extracting username from object ID fails
	ErrExtractUsernameFromObjectID = errors.New("unable to extract username from object ID")

	// ErrPerformListOperation is returned when list operation fails
	ErrPerformListOperation = errors.New("failed to perform list operation")

	// Block activity errors
	// ErrBlockMissingObjectID is returned when block activity object is missing id field
	ErrBlockMissingObjectID = errors.New("block activity object missing id field")

	// ErrBlockInvalidObjectType is returned when block activity has invalid object type
	ErrBlockInvalidObjectType = errors.New("block activity has invalid object type")

	// ErrBlockMissingBlockedActor is returned when block activity is missing blocked actor
	ErrBlockMissingBlockedActor = errors.New("block activity missing blocked actor")

	// ErrBlockMissingBlockerActor is returned when block activity is missing blocker actor
	ErrBlockMissingBlockerActor = errors.New("block activity missing blocker actor")

	// ErrCreateBlockRelationship is returned when creating block relationship fails
	ErrCreateBlockRelationship = errors.New("failed to create block relationship")

	// Flag activity errors
	// ErrExtractFlaggedObjectFromFlag is returned when extracting flagged object from Flag activity fails
	ErrExtractFlaggedObjectFromFlag = errors.New("unable to extract flagged object from Flag activity")

	// ErrNoFlaggedObjectsFound is returned when no flagged objects are found in Flag activity
	ErrNoFlaggedObjectsFound = errors.New("no flagged objects found in Flag activity")

	// ErrCreateFlagRecord is returned when creating flag record fails
	ErrCreateFlagRecord = errors.New("failed to create flag record")

	// Move activity errors
	// ErrMoveMustSpecifyTarget is returned when move activity must specify a target account
	ErrMoveMustSpecifyTarget = errors.New("move activity must specify a target account")

	// ErrExtractUsernameFromOldActorURI is returned when extracting username from old actor URI fails
	ErrExtractUsernameFromOldActorURI = errors.New("unable to extract username from old actor URI")

	// ErrUpdateMovedToField is returned when updating movedTo field fails
	ErrUpdateMovedToField = errors.New("failed to update movedTo field")

	// Collection activity errors (Add/Remove)
	// ErrActivityMissingTargetCollection is returned when activity is missing target collection
	ErrActivityMissingTargetCollection = errors.New("activity missing target collection")

	// ErrExtractObjectFromActivity is returned when extracting object from activity fails
	ErrExtractObjectFromActivity = errors.New("unable to extract object from activity")

	// ErrNoObjectsFoundInActivity is returned when no objects are found in activity
	ErrNoObjectsFoundInActivity = errors.New("no objects found in activity")

	// Target-specific activity errors
	// ErrExtractTargetIDFromActivity is returned when extracting target ID from activity fails
	ErrExtractTargetIDFromActivity = errors.New("unable to extract target ID from activity")

	// ErrDeleteActivityRecord is returned when deleting activity record fails
	ErrDeleteActivityRecord = errors.New("failed to delete activity record")

	// Timeline processing errors
	// ErrExtractUsernameFromActorID is returned when extracting username from actor ID fails
	ErrExtractUsernameFromActorID = errors.New("failed to extract username from actor ID")

	// ErrGetFollowers is returned when getting followers fails
	ErrGetFollowers = errors.New("failed to get followers")

	// Federation service errors
	// ErrUnsupportedDatabaseType is returned when database type is not supported
	ErrUnsupportedDatabaseType = errors.New("unsupported database type")

	// ErrNoDatabaseAvailable is returned when no database is available
	ErrNoDatabaseAvailable = errors.New("no database available")

	// ErrActivityMissingActor is returned when activity is missing actor
	ErrActivityMissingActor = errors.New("activity missing actor")

	// ErrInvalidActorIDFormat is returned when actor ID format is invalid
	ErrInvalidActorIDFormat = errors.New("invalid actor ID format")

	// Job queue service errors
	// ErrAWSConfigLoad is returned when AWS config loading fails
	ErrAWSConfigLoad = errors.New("failed to load AWS config")

	// ErrImportJobSerialization is returned when import job serialization fails
	ErrImportJobSerialization = errors.New("failed to serialize import job")

	// ErrImportJobQueue is returned when import job queuing fails
	ErrImportJobQueue = errors.New("failed to queue import job")

	// ErrExportJobSerialization is returned when export job serialization fails
	ErrExportJobSerialization = errors.New("failed to serialize export job")

	// ErrExportJobQueue is returned when export job queuing fails
	ErrExportJobQueue = errors.New("failed to queue export job")

	// ErrMediaJobSerialization is returned when media job serialization fails
	ErrMediaJobSerialization = errors.New("failed to serialize media job")

	// ErrMediaJobQueue is returned when media job queuing fails
	ErrMediaJobQueue = errors.New("failed to queue media job")

	// ErrScheduledJobSerialization is returned when scheduled job serialization fails
	ErrScheduledJobSerialization = errors.New("failed to serialize scheduled job")

	// ErrScheduledJobQueue is returned when scheduled job queuing fails
	ErrScheduledJobQueue = errors.New("failed to queue scheduled job")

	// ErrActivityJobSerialization is returned when activity job serialization fails
	ErrActivityJobSerialization = errors.New("failed to serialize activity job")

	// ErrActivityJobQueue is returned when activity job queuing fails
	ErrActivityJobQueue = errors.New("failed to queue activity job")

	// ErrQueueURLNotConfigured is returned when queue URL is not configured
	ErrQueueURLNotConfigured = errors.New("queue URL not configured")

	// ErrMessageSerialization is returned when message serialization fails
	ErrMessageSerialization = errors.New("failed to serialize message")

	// ErrDelayedJobQueue is returned when delayed job queuing fails
	ErrDelayedJobQueue = errors.New("failed to queue delayed job")

	// ErrBatchMessageSend is returned when batch message sending fails
	ErrBatchMessageSend = errors.New("failed to send batch messages")

	// ErrBatchOperation is returned when batch operation fails
	ErrBatchOperation = errors.New("batch operation failed")

	// ErrQueueAttributeQuery is returned when queue attribute query fails
	ErrQueueAttributeQuery = errors.New("failed to query queue attributes")

	// Lists service errors
	// ErrListValidationFailed is returned when list validation fails
	ErrListValidationFailed = errors.New("validation failed")

	// ErrListCreateFailed is returned when list creation fails
	ErrListCreateFailed = errors.New("failed to create list")

	// ErrListGetFailed is returned when list retrieval fails
	ErrListGetFailed = errors.New("failed to get list")

	// ErrListUpdateFailed is returned when list update fails
	ErrListUpdateFailed = errors.New("failed to update list")

	// ErrListDeleteFailed is returned when list deletion fails
	ErrListDeleteFailed = errors.New("failed to delete list")

	// ErrListMembershipCheckFailed is returned when list membership check fails
	ErrListMembershipCheckFailed = errors.New("failed to check membership")

	// ErrListMemberAddFailed is returned when adding member to list fails
	ErrListMemberAddFailed = errors.New("failed to add member to list")

	// ErrListMemberRemoveFailed is returned when removing member from list fails
	ErrListMemberRemoveFailed = errors.New("failed to remove member from list")

	// ErrListNotFound is returned when list is not found
	ErrListNotFound = errors.New("list not found")

	// ErrGetUserLists is returned when getting user lists fails
	ErrGetUserLists = errors.New("failed to get user lists")

	// ErrGetListTimeline is returned when getting list timeline fails
	ErrGetListTimeline = errors.New("failed to get list timeline")

	// ErrGetListMembers is returned when getting list members fails
	ErrGetListMembers = errors.New("failed to get list members")

	// List validation errors
	// ErrListUsernameRequired is returned when username is missing
	ErrListUsernameRequired = errors.New("username is required")

	// ErrListCreatorIDRequired is returned when creator ID is missing
	ErrListCreatorIDRequired = errors.New("creator_id is required")

	// ErrListTitleRequired is returned when title is missing
	ErrListTitleRequired = errors.New("title is required")

	// ErrListTitleEmpty is returned when title is empty
	ErrListTitleEmpty = errors.New("title cannot be empty")

	// ErrListIDRequired is returned when list ID is missing
	ErrListIDRequired = errors.New("list_id is required")

	// ErrListUpdaterIDRequired is returned when updater ID is missing
	ErrListUpdaterIDRequired = errors.New("updater_id is required")

	// ErrListDeleterIDRequired is returned when deleter ID is missing
	ErrListDeleterIDRequired = errors.New("deleter_id is required")

	// ErrListMemberUsernameRequired is returned when member username is missing
	ErrListMemberUsernameRequired = errors.New("member_username is required")

	// ErrListAdderIDRequired is returned when adder ID is missing
	ErrListAdderIDRequired = errors.New("adder_id is required")

	// ErrListRemoverIDRequired is returned when remover ID is missing
	ErrListRemoverIDRequired = errors.New("remover_id is required")

	// Notes service errors
	// ErrNotesValidationFailed is returned when notes validation fails
	ErrNotesValidationFailed = errors.New("validation failed")

	// ErrGetAuthorAccount is returned when getting author account fails
	ErrGetAuthorAccount = errors.New("failed to get author account")

	// ErrGetStatus is returned when status retrieval fails
	ErrGetStatus = errors.New("failed to get status")

	// ErrUpdateStatus is returned when status update fails
	ErrUpdateStatus = errors.New("failed to update status")

	// ErrDeleteStatus is returned when status deletion fails
	ErrDeleteStatus = errors.New("failed to delete status")

	// ErrStatusNotFound is returned when status is not found
	ErrStatusNotFound = errors.New("status not found")

	// ErrStatusIDRequired is returned when status ID is required but missing
	ErrStatusIDRequired = errors.New("status_id is required")

	// ErrCheckViewPermissions is returned when checking view permissions fails
	ErrCheckViewPermissions = errors.New("failed to check view permissions")

	// ErrCheckFollowingRelationship is returned when checking following relationship fails
	ErrCheckFollowingRelationship = errors.New("failed to check following relationship")

	// ErrHomeTimelineRequiresViewerID is returned when home timeline requires viewer_id
	ErrHomeTimelineRequiresViewerID = errors.New("home timeline requires viewer_id")

	// ErrUserTimelineRequiresAuthorID is returned when user timeline requires author_id
	ErrUserTimelineRequiresAuthorID = errors.New("user timeline requires author_id")

	// ErrConversationsTimelineRequiresConversationID is returned when conversations timeline requires conversation_id
	ErrConversationsTimelineRequiresConversationID = errors.New("conversations timeline requires conversation_id")

	// ErrDirectTimelineRequiresViewerID is returned when direct timeline requires viewer_id
	ErrDirectTimelineRequiresViewerID = errors.New("direct timeline requires viewer_id")

	// ErrHashtagTimelineRequiresHashtag is returned when hashtag timeline requires hashtag
	ErrHashtagTimelineRequiresHashtag = errors.New("hashtag timeline requires hashtag")

	// ErrListTimelineRequiresListID is returned when list timeline requires list_id
	ErrListTimelineRequiresListID = errors.New("list timeline requires list_id")

	// ErrUnsupportedTimelineType is returned when timeline type is unsupported
	ErrUnsupportedTimelineType = errors.New("unsupported timeline type")

	// ErrGetTimeline is returned when timeline retrieval fails
	ErrGetTimeline = errors.New("failed to get timeline")

	// ErrStatusContentValidationFailed is returned when status content validation fails
	ErrStatusContentValidationFailed = errors.New("status content validation failed")

	// ErrVisibilityValidationFailed is returned when visibility validation fails
	ErrVisibilityValidationFailed = errors.New("visibility validation failed")

	// ErrSpoilerTextValidationFailed is returned when spoiler text validation fails
	ErrSpoilerTextValidationFailed = errors.New("spoiler text validation failed")

	// ErrInvalidInReplyToID is returned when in_reply_to_id is invalid
	ErrInvalidInReplyToID = errors.New("invalid in_reply_to_id")

	// ErrContentCannotBeEmpty is returned when content cannot be empty
	ErrContentCannotBeEmpty = errors.New("content cannot be empty")

	// ErrContentTooLong is returned when content is too long
	ErrContentTooLong = errors.New("content too long (max 5000 characters)")

	// ErrContentTooLongShort is returned when content is too long for short form
	ErrContentTooLongShort = errors.New("content too long (max 500 characters)")

	// ErrBookmarkStatus is returned when bookmarking status fails
	ErrBookmarkStatus = errors.New("failed to bookmark status")

	// ErrUnbookmarkStatus is returned when unbookmarking status fails
	ErrUnbookmarkStatus = errors.New("failed to unbookmark status")

	// ErrGetBookmarks is returned when getting bookmarks fails
	ErrGetBookmarks = errors.New("failed to get bookmarks")

	// ErrGetLikers is returned when getting likers fails
	ErrGetLikers = errors.New("failed to get likers")

	// ErrGetRebloggerAccount is returned when getting reblogger account fails
	ErrGetRebloggerAccount = errors.New("failed to get reblogger account")

	// ErrReblogStatus is returned when reblogging status fails
	ErrReblogStatus = errors.New("failed to reblog status")

	// ErrGetRebloggers is returned when getting rebloggers fails
	ErrGetRebloggers = errors.New("failed to get rebloggers")

	// ErrMuteStatus is returned when muting status fails
	ErrMuteStatus = errors.New("failed to mute status")

	// ErrUnmuteStatus is returned when unmuting status fails
	ErrUnmuteStatus = errors.New("failed to unmute status")

	// ErrCreateLike is returned when creating like fails
	ErrCreateLike = errors.New("failed to create like")

	// ErrDeleteLike is returned when deleting like fails
	ErrDeleteLike = errors.New("failed to delete like")

	// ErrGetLikes is returned when getting likes fails
	ErrGetLikes = errors.New("failed to get likes")

	// ErrCreateReblog is returned when creating reblog fails
	ErrCreateReblog = errors.New("failed to create reblog")

	// ErrDeleteReblog is returned when deleting reblog fails
	ErrDeleteReblog = errors.New("failed to delete reblog")

	// ErrGetAnnounces is returned when getting announces fails
	ErrGetAnnounces = errors.New("failed to get announces")

	// ErrPinStatus is returned when pinning status fails
	ErrPinStatus = errors.New("failed to pin status")

	// ErrUnpinStatus is returned when unpinning status fails
	ErrUnpinStatus = errors.New("failed to unpin status")

	// ErrConversationServiceNotAvailable is returned when conversation service is not available
	ErrConversationServiceNotAvailable = errors.New("conversation service not available")

	// ErrMuteConversation is returned when muting conversation fails
	ErrMuteConversation = errors.New("failed to mute conversation")

	// ErrUnmuteConversation is returned when unmuting conversation fails
	ErrUnmuteConversation = errors.New("failed to unmute conversation")

	// ErrGetConversations is returned when getting conversations fails
	ErrGetConversations = errors.New("failed to get conversations")

	// ErrViewerIDRequiredForFavoritedTimeline is returned when viewer_id is required for favorited timeline
	ErrViewerIDRequiredForFavoritedTimeline = errors.New("viewer_id is required for favorited timeline")

	// ErrGetViewerAccount is returned when getting viewer account fails
	ErrGetViewerAccount = errors.New("failed to get viewer account")

	// ErrLikeRepositoryNotAvailable is returned when like repository is not available
	ErrLikeRepositoryNotAvailable = errors.New("like repository not available")

	// ErrGetLikedObjects is returned when getting liked objects fails
	ErrGetLikedObjects = errors.New("failed to get liked objects")

	// ErrGetStatuses is returned when getting statuses fails
	ErrGetStatuses = errors.New("failed to get statuses")

	// ErrCreateScheduledStatus is returned when creating scheduled status fails
	ErrCreateScheduledStatus = errors.New("failed to create scheduled status")

	// ErrScheduledTimeInPast is returned when scheduled time must be in the future
	ErrScheduledTimeInPast = errors.New("scheduled time must be in the future")

	// ErrMediaAttachmentWithID is returned when media attachment operation fails with specific ID
	ErrMediaAttachmentWithID = errors.New("media attachment operation failed")

	// ErrGetSearchSuggestions is returned when getting search suggestions fails
	ErrGetSearchSuggestions = errors.New("failed to get search suggestions")

	// ErrCreateCommunityNote is returned when creating community note fails
	ErrCreateCommunityNote = errors.New("failed to create community note")

	// ErrGetVisibleCommunityNotes is returned when getting visible community notes fails
	ErrGetVisibleCommunityNotes = errors.New("failed to get visible community notes")

	// ErrGetCommunityNote is returned when getting community note fails
	ErrGetCommunityNote = errors.New("failed to get community note")

	// ErrCreateCommunityNoteVote is returned when creating community note vote fails
	ErrCreateCommunityNoteVote = errors.New("failed to create community note vote")

	// ErrGetCommunityNotesByAuthor is returned when getting community notes by author fails
	ErrGetCommunityNotesByAuthor = errors.New("failed to get community notes by author")

	// ErrCountStatusesByAuthor is returned when counting statuses by author fails
	ErrCountStatusesByAuthor = errors.New("failed to count statuses by author")

	// ErrGetUserTimeline is returned when getting user timeline fails
	ErrGetUserTimeline = errors.New("failed to get user timeline")

	// ErrCountReplies is returned when counting replies fails
	ErrCountReplies = errors.New("failed to count replies")

	// ErrGetBoostCount is returned when getting boost count fails
	ErrGetBoostCount = errors.New("failed to get boost count")

	// ErrGetLikeCount is returned when getting like count fails
	ErrGetLikeCount = errors.New("failed to get like count")

	// ErrCheckUserHasLiked is returned when checking if user has liked fails
	ErrCheckUserHasLiked = errors.New("failed to check if user has liked")

	// ErrCheckUserHasReblogged is returned when checking if user has reblogged fails
	ErrCheckUserHasReblogged = errors.New("failed to check if user has reblogged")

	// ErrCheckUserHasBookmarked is returned when checking if user has bookmarked fails
	ErrCheckUserHasBookmarked = errors.New("failed to check if user has bookmarked")

	// List permission errors
	// ErrListUnauthorizedCreate is returned when user cannot create list for another user
	ErrListUnauthorizedCreate = errors.New("cannot create list for another user")

	// ErrListUnauthorizedUpdate is returned when user cannot update list owned by another user
	ErrListUnauthorizedUpdate = errors.New("cannot update list owned by another user")

	// ErrListUnauthorizedDelete is returned when user cannot delete list owned by another user
	ErrListUnauthorizedDelete = errors.New("cannot delete list owned by another user")

	// ErrListUnauthorizedAddMember is returned when user cannot add members to list owned by another user
	ErrListUnauthorizedAddMember = errors.New("cannot add members to list owned by another user")

	// ErrListUnauthorizedRemoveMember is returned when user cannot remove members from list owned by another user
	ErrListUnauthorizedRemoveMember = errors.New("cannot remove members from list owned by another user")

	// ErrListUnauthorizedView is returned when user cannot view list owned by another user
	ErrListUnauthorizedView = errors.New("cannot view list owned by another user")

	// ErrListUnauthorizedViewLists is returned when user cannot view lists for another user
	ErrListUnauthorizedViewLists = errors.New("cannot view lists for another user")

	// ErrListUnauthorizedViewTimeline is returned when user cannot view timeline for list owned by another user
	ErrListUnauthorizedViewTimeline = errors.New("cannot view timeline for list owned by another user")

	// ErrListUnauthorizedViewMembers is returned when user cannot view members of list owned by another user
	ErrListUnauthorizedViewMembers = errors.New("cannot view members of list owned by another user")

	// Emoji service errors
	// ErrEmojiNotFound is returned when emoji is not found
	ErrEmojiNotFound = errors.New("emoji not found")

	// ErrGetEmoji is returned when emoji retrieval fails
	ErrGetEmoji = errors.New("failed to get emoji")

	// ErrListEmojis is returned when emoji listing fails
	ErrListEmojis = errors.New("failed to list emojis")

	// ErrEmojiAlreadyExists is returned when emoji with shortcode already exists
	ErrEmojiAlreadyExists = errors.New("emoji already exists")

	// ErrCreateEmoji is returned when emoji creation fails
	ErrCreateEmoji = errors.New("failed to create emoji")

	// ErrCannotUpdateRemoteEmoji is returned when attempting to update a remote emoji
	ErrCannotUpdateRemoteEmoji = errors.New("cannot update remote emoji")

	// ErrUpdateEmoji is returned when emoji update fails
	ErrUpdateEmoji = errors.New("failed to update emoji")

	// ErrCannotDeleteRemoteEmoji is returned when attempting to delete a remote emoji
	ErrCannotDeleteRemoteEmoji = errors.New("cannot delete remote emoji")

	// ErrDeleteEmoji is returned when emoji deletion fails
	ErrDeleteEmoji = errors.New("failed to delete emoji")

	// ErrRemoteEmojiNotFound is returned when remote emoji is not found
	ErrRemoteEmojiNotFound = errors.New("remote emoji not found")

	// ErrGetRemoteEmoji is returned when remote emoji retrieval fails
	ErrGetRemoteEmoji = errors.New("failed to get remote emoji")

	// ErrCreateLocalEmojiCopy is returned when creating local emoji copy fails
	ErrCreateLocalEmojiCopy = errors.New("failed to create local emoji copy")

	// ErrSearchEmojis is returned when emoji search fails
	ErrSearchEmojis = errors.New("failed to search emojis")

	// ErrGetPopularEmojis is returned when getting popular emojis fails
	ErrGetPopularEmojis = errors.New("failed to get popular emojis")

	// ErrIncrementEmojiUsage is returned when incrementing emoji usage fails
	ErrIncrementEmojiUsage = errors.New("failed to increment emoji usage")

	// ErrInvalidShortcode is returned when shortcode validation fails
	ErrInvalidShortcode = errors.New("invalid shortcode")

	// ErrReservedShortcode is returned when shortcode is reserved
	ErrReservedShortcode = errors.New("shortcode is reserved")

	// Scheduled status media attachment errors
	// ErrMediaAttachmentNotFound is returned when media attachment is not found or inaccessible
	ErrMediaAttachmentNotFound = errors.New("media attachment not found or inaccessible")

	// ErrMediaAttachmentNotReady is returned when media attachment is not ready
	ErrMediaAttachmentNotReady = errors.New("media attachment not ready")

	// ErrMediaAttachmentExpired is returned when media attachment has expired
	ErrMediaAttachmentExpired = errors.New("media attachment has expired")

	// ErrRetrieveMediaAttachment is returned when retrieving media attachment fails
	ErrRetrieveMediaAttachment = errors.New("failed to retrieve media attachment")

	// ErrValidationFailed is returned when validation fails
	ErrValidationFailed = errors.New("validation failed")

	// Import/Export service errors
	// Export operation errors
	// ErrExportValidationFailed is returned when export validation fails
	ErrExportValidationFailed = errors.New("export validation failed")

	// ErrCreateExport is returned when export creation fails
	ErrCreateExport = errors.New("failed to create export")

	// ErrQueueExport is returned when export queueing fails
	ErrQueueExport = errors.New("failed to queue export")

	// ErrGetExport is returned when export retrieval fails
	ErrGetExport = errors.New("failed to get export")

	// ErrExportAccessForbidden is returned when user cannot access export
	ErrExportAccessForbidden = errors.New("user cannot access export")

	// ErrListExports is returned when export listing fails
	ErrListExports = errors.New("failed to list exports")

	// ErrUpdateExportProgress is returned when export progress update fails
	ErrUpdateExportProgress = errors.New("failed to update export progress")

	// ErrUpdateExportStatus is returned when export status update fails
	ErrUpdateExportStatus = errors.New("failed to update export status")

	// ErrExportNotFound is returned when export is not found
	ErrExportNotFound = errors.New("export not found")

	// ErrNotAuthorizedCancelExport is returned when user is not authorized to cancel export
	ErrNotAuthorizedCancelExport = errors.New("not authorized to cancel this export")

	// ErrCannotCancelCompletedExport is returned when trying to cancel completed export
	ErrCannotCancelCompletedExport = errors.New("cannot cancel completed export")

	// ErrExportAlreadyCancelled is returned when export is already cancelled
	ErrExportAlreadyCancelled = errors.New("export is already cancelled")

	// ErrCannotCancelFailedExport is returned when trying to cancel failed export
	ErrCannotCancelFailedExport = errors.New("cannot cancel failed export")

	// ErrCancelExport is returned when export cancellation fails
	ErrCancelExport = errors.New("failed to cancel export")

	// ErrGetCancelledExport is returned when getting cancelled export fails
	ErrGetCancelledExport = errors.New("failed to get cancelled export")

	// Import operation errors
	// ErrCreateImport is returned when import creation fails
	ErrCreateImport = errors.New("failed to create import")

	// ErrImportNotFound is returned when import is not found
	ErrImportNotFound = errors.New("import not found")

	// ErrNotAuthorizedAccessImport is returned when user is not authorized to access import
	ErrNotAuthorizedAccessImport = errors.New("not authorized to access this import")

	// ErrListImports is returned when import listing fails
	ErrListImports = errors.New("failed to list imports")

	// Export validation errors
	// ErrUserNotFound is returned when user is not found during validation
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidDateRangeOrder is returned when start date is after end date
	ErrInvalidDateRangeOrder = errors.New("invalid date range: start date after end date")

	// ErrInvalidDateRangeFuture is returned when end date is in the future
	ErrInvalidDateRangeFuture = errors.New("invalid date range: end date in the future")

	// Search service errors
	// ErrSearchAccounts is returned when account search operation fails
	ErrSearchAccounts = errors.New("failed to search accounts")

	// ErrSearchHashtags is returned when hashtag search operation fails
	ErrSearchHashtags = errors.New("failed to search hashtags")

	// ErrSearchStatuses is returned when status search operation fails
	ErrSearchStatuses = errors.New("failed to search statuses")

	// ErrGetDirectory is returned when directory retrieval fails
	ErrGetDirectory = errors.New("failed to get directory")

	// ErrGetSuggestions is returned when suggestions retrieval fails
	ErrGetSuggestions = errors.New("failed to get suggestions")

	// ErrRemoveSuggestion is returned when suggestion removal fails
	ErrRemoveSuggestion = errors.New("failed to remove suggestion")

	// Cost analytics service errors
	// ErrGetAICostData is returned when AI cost data retrieval fails
	ErrGetAICostData = errors.New("failed to get AI cost data")

	// ErrUnsupportedMetric is returned when an unsupported metric type is requested
	ErrUnsupportedMetric = errors.New("unsupported metric")

	// ErrInsufficientHistoricalData is returned when insufficient historical data for prediction
	ErrInsufficientHistoricalData = errors.New("insufficient historical data for prediction (need at least 7 points)")

	// Bulk operations service errors
	// ErrBulkOperationNotFound is returned when bulk operation is not found
	ErrBulkOperationNotFound = errors.New("operation not found")

	// ErrBulkOperationInvalidData is returned when bulk operation data is invalid
	ErrBulkOperationInvalidData = errors.New("invalid operation data")

	// ErrBulkOperationUnauthorizedAccess is returned when user cannot access bulk operation
	ErrBulkOperationUnauthorizedAccess = errors.New("unauthorized access to bulk operation")

	// ErrBulkContentNotFound is returned when bulk content is not found
	ErrBulkContentNotFound = errors.New("bulk content not found")

	// ErrBulkContentUnauthorizedDelete is returned when user is not authorized to delete content
	ErrBulkContentUnauthorizedDelete = errors.New("not authorized to delete content")
)
