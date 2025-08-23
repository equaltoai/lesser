package services

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

	// AWS Queue service specific errors
	// ErrImportExportQueueURLNotConfigured is returned when ImportExportQueueURL is not configured
	ErrImportExportQueueURLNotConfigured = errors.New("ImportExportQueueURL not configured")

	// ErrSQSConnectFailed is returned when SQS queue connection fails
	ErrSQSConnectFailed = errors.New("failed to connect to SQS queue")

	// ErrQueueMessageMarshalFailed is returned when queue message marshaling fails
	ErrQueueMessageMarshalFailed = errors.New("failed to marshal queue message")

	// ErrSQSMessageSendFailed is returned when SQS message sending fails
	ErrSQSMessageSendFailed = errors.New("failed to send message to SQS")

	// Notes service errors
	// ErrNotesValidationFailed is returned when notes validation fails
	ErrNotesValidationFailed = errors.New("validation failed")

	// ErrGetAuthorAccount is returned when getting author account fails
	ErrGetAuthorAccount = errors.New("failed to get author account")

	// ErrCreateStatus is returned when status creation fails (reused from above)
	// ErrCreateStatus = errors.New("failed to create status") // Already defined above

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

	// Account service errors
	// ErrValidationFailed is returned when input validation fails
	ErrValidationFailed = errors.New("validation failed")

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = errors.New("failed to get account")

	// ErrUpdateProfile is returned when profile update fails
	ErrUpdateProfile = errors.New("failed to update profile")

	// ErrStoreAccount is returned when account storage fails
	ErrStoreAccount = errors.New("failed to store account")

	// ErrGetPreferences is returned when preferences retrieval fails
	ErrGetPreferences = errors.New("failed to get preferences")

	// ErrUpdatePreferences is returned when preferences update fails
	ErrUpdatePreferences = errors.New("failed to update preferences")

	// ErrAccountNotFound is returned when account is not found
	ErrAccountNotFound = errors.New("account not found")

	// ErrSearchAccounts is returned when account search fails
	ErrSearchAccounts = errors.New("failed to search accounts")

	// ErrEmptySearchQuery is returned when search query is empty
	ErrEmptySearchQuery = errors.New("search query cannot be empty")

	// ErrUsernameRequired is returned when username is missing
	ErrUsernameRequired = errors.New("username is required")

	// ErrUpdaterIDRequired is returned when updater ID is missing
	ErrUpdaterIDRequired = errors.New("updater_id is required")

	// ErrProfileFieldNameEmpty is returned when profile field name is empty
	ErrProfileFieldNameEmpty = errors.New("profile field name cannot be empty")

	// ErrProfileFieldNameTooLong is returned when profile field name is too long
	ErrProfileFieldNameTooLong = errors.New("profile field name too long (max 255 characters)")

	// ErrProfileFieldValueTooLong is returned when profile field value is too long
	ErrProfileFieldValueTooLong = errors.New("profile field value too long (max 255 characters)")

	// ErrInvalidExpandMediaSetting is returned when expand media setting is invalid
	ErrInvalidExpandMediaSetting = errors.New("invalid expand media setting")

	// ErrInvalidTimelineOrder is returned when timeline order is invalid
	ErrInvalidTimelineOrder = errors.New("invalid timeline order")

	// ErrAccountNoActivityPubActor is returned when account has no ActivityPub actor
	ErrAccountNoActivityPubActor = errors.New("account has no ActivityPub actor")

	// ErrRelationshipRepositoryNotAvailable is returned when relationship repository is not available
	ErrRelationshipRepositoryNotAvailable = errors.New("relationship repository not available")

	// ErrActorRepositoryNotAvailable is returned when actor repository is not available
	ErrActorRepositoryNotAvailable = errors.New("actor repository not available")

	// ErrGetActor is returned when actor retrieval fails
	ErrGetActor = errors.New("failed to get actor")

	// ErrGetFollowersAccounts is returned when followers retrieval fails
	ErrGetFollowersAccounts = errors.New("failed to get followers")

	// ErrGetFollowingList is returned when following list retrieval fails
	ErrGetFollowingList = errors.New("failed to get following list")

	// ErrGetViewerActor is returned when viewer actor retrieval fails
	ErrGetViewerActor = errors.New("failed to get viewer actor")

	// ErrGetViewerFollowing is returned when viewer following retrieval fails
	ErrGetViewerFollowing = errors.New("failed to get viewer following")

	// ErrTargetAccountNotFound is returned when target account is not found
	ErrTargetAccountNotFound = errors.New("target account not found")

	// ErrAccountAlreadyPinned is returned when account is already pinned
	ErrAccountAlreadyPinned = errors.New("account already pinned")

	// ErrPinAccount is returned when account pinning fails
	ErrPinAccount = errors.New("failed to pin account")

	// ErrUnpinAccount is returned when account unpinning fails
	ErrUnpinAccount = errors.New("failed to unpin account")

	// ErrGetAccountPins is returned when account pins retrieval fails
	ErrGetAccountPins = errors.New("failed to get account pins")

	// ErrSetAccountNote is returned when account note setting fails
	ErrSetAccountNote = errors.New("failed to set account note")

	// ErrRemoveFollower is returned when follower removal fails
	ErrRemoveFollower = errors.New("failed to remove follower")

	// ErrUsernameAlreadyTaken is returned when username is already taken
	ErrUsernameAlreadyTaken = errors.New("username already taken")

	// ErrGenerateKeypair is returned when keypair generation fails
	ErrGenerateKeypair = errors.New("failed to generate keypair")

	// ErrEncodePublicKey is returned when public key encoding fails
	ErrEncodePublicKey = errors.New("failed to encode public key")

	// ErrHashPassword is returned when password hashing fails
	ErrHashPassword = errors.New("failed to hash password")

	// ErrCreateAccount is returned when account creation fails
	ErrCreateAccount = errors.New("failed to create account")

	// ErrEmailRequired is returned when email is missing
	ErrEmailRequired = errors.New("email is required")

	// ErrMustAgreeToTerms is returned when user doesn't agree to terms
	ErrMustAgreeToTerms = errors.New("must agree to terms of service")

	// ErrCryptoServiceNotConfigured is returned when crypto service is not configured
	ErrCryptoServiceNotConfigured = errors.New("crypto service not configured")

	// ErrAuthServiceNotConfigured is returned when auth service is not configured
	ErrAuthServiceNotConfigured = errors.New("auth service not configured")

	// ErrStorageNotAvailable is returned when storage is not available
	ErrStorageNotAvailable = errors.New("storage not available")

	// ErrUserRepositoryNotAvailable is returned when user repository is not available
	ErrUserRepositoryNotAvailable = errors.New("user repository not available")

	// ErrCheckAccountPinned is returned when checking if account is pinned fails
	ErrCheckAccountPinned = errors.New("failed to check if account is pinned")

	// ErrGetUser is returned when user retrieval fails
	ErrGetUser = errors.New("failed to get user")

	// ErrGetUserPreferences is returned when user preferences retrieval fails
	ErrGetUserPreferences = errors.New("failed to get user preferences")

	// ErrDomainBlockRepositoryNotAvailable is returned when domain block repository is not available
	ErrDomainBlockRepositoryNotAvailable = errors.New("domain block repository not available")

	// ErrCheckDomainBlockedByUser is returned when checking if domain is blocked by user fails
	ErrCheckDomainBlockedByUser = errors.New("failed to check if domain is blocked by user")

	// ErrAccountRepositoryNotAvailable is returned when account repository is not available
	ErrAccountRepositoryNotAvailable = errors.New("account repository not available")

	// ErrGetFieldVerification is returned when field verification retrieval fails
	ErrGetFieldVerification = errors.New("failed to get field verification")

	// ErrGetAccountNote is returned when account note retrieval fails
	ErrGetAccountNote = errors.New("failed to get account note")

	// Business logic service errors
	// ErrStoreScheduledStatus is returned when storing a scheduled status fails
	ErrStoreScheduledStatus = errors.New("failed to store scheduled status")

	// ErrEmojiRepositoryNotAvailable is returned when emoji repository is not available
	ErrEmojiRepositoryNotAvailable = errors.New("emoji repository not available")

	// Adapter service errors
	// ErrUnsupportedKeyType is returned when a key type is not supported for cryptographic operations
	ErrUnsupportedKeyType = errors.New("unsupported key type")

	// ErrInvalidPrivateKeyType is returned when an invalid private key type is provided
	ErrInvalidPrivateKeyType = errors.New("expected *rsa.PrivateKey")

	// Storage adapter errors
	// ErrInvalidNotificationType is returned when notification type conversion fails
	ErrInvalidNotificationType = errors.New("invalid notification type")

	// ErrGetDomainHealthScore is returned when domain health score retrieval fails
	ErrGetDomainHealthScore = errors.New("failed to get domain health score")

	// ErrUnsupportedStorageType is returned when an unsupported storage type is provided
	ErrUnsupportedStorageType = errors.New("unsupported storage type - only core.RepositoryStorage is supported")

	// Registry service errors
	// ErrApplyRegistryOption is returned when applying a registry option fails
	ErrApplyRegistryOption = errors.New("failed to apply registry option")

	// ErrRegistryValidation is returned when registry validation fails
	ErrRegistryValidation = errors.New("registry validation failed")

	// ErrStorageCannotBeNil is returned when storage dependency is nil
	ErrStorageCannotBeNil = errors.New("storage cannot be nil")

	// ErrPublisherCannotBeNil is returned when publisher dependency is nil
	ErrPublisherCannotBeNil = errors.New("publisher cannot be nil")

	// ErrLoggerCannotBeNil is returned when logger dependency is nil
	ErrLoggerCannotBeNil = errors.New("logger cannot be nil")

	// ErrConfigCannotBeNil is returned when config dependency is nil
	ErrConfigCannotBeNil = errors.New("config cannot be nil")

	// ErrStorageRequired is returned when storage is required but not provided
	ErrStorageRequired = errors.New("storage is required - use WithStorage()")

	// ErrEventBusNotInitialized is returned when internal event bus is not initialized
	ErrEventBusNotInitialized = errors.New("internal event bus not initialized")

	// ErrEventBusSubscription is returned when event bus subscription fails
	ErrEventBusSubscription = errors.New("failed to subscribe to internal event bus")

	// File validation service errors
	// ErrLoadAWSConfig is returned when AWS config loading fails in file validation
	ErrLoadAWSConfig = errors.New("failed to load AWS config")

	// ErrFileSizeExceedsLimit is returned when file size exceeds configured limit
	ErrFileSizeExceedsLimit = errors.New("file size exceeds limit")

	// ErrContentTypeNotAllowed is returned when file content type is not in allowed list
	ErrContentTypeNotAllowed = errors.New("content type is not allowed")

	// ErrFormatNotSupported is returned when file format is not supported
	ErrFormatNotSupported = errors.New("format is not supported")

	// ErrInvalidJSONFormat is returned when JSON format validation fails
	ErrInvalidJSONFormat = errors.New("invalid JSON format")

	// ErrCSVNoContent is returned when CSV file has no content
	ErrCSVNoContent = errors.New("CSV file has no content")

	// ErrCSVNoColumns is returned when CSV file has no columns
	ErrCSVNoColumns = errors.New("CSV file has no columns")

	// ErrCSVTooManyInconsistentRows is returned when CSV has too many inconsistent rows
	ErrCSVTooManyInconsistentRows = errors.New("CSV file has too many inconsistent rows")

	// ErrFileTooMuchBinaryContent is returned when file contains excessive binary content
	ErrFileTooMuchBinaryContent = errors.New("file contains too much binary content")

	// ErrJSONStructureTooDeep is returned when JSON structure is too deeply nested
	ErrJSONStructureTooDeep = errors.New("JSON structure is too deeply nested")

	// ErrCSVRowInconsistentColumns is returned when CSV row has inconsistent column count
	ErrCSVRowInconsistentColumns = errors.New("CSV row has inconsistent column count")

	// ErrSuspiciousContentDetected is returned when potentially suspicious content is detected
	ErrSuspiciousContentDetected = errors.New("potentially suspicious content detected")

	// ErrLineTooLong is returned when a line is excessively long
	ErrLineTooLong = errors.New("line is excessively long")

	// ErrFileEmpty is returned when file is empty
	ErrFileEmpty = errors.New("file is empty")

	// File validation warning constants
	// ErrJSONObjectEmpty is returned when JSON object is empty
	ErrJSONObjectEmpty = errors.New("JSON object is empty")

	// ErrJSONArrayEmpty is returned when JSON array is empty
	ErrJSONArrayEmpty = errors.New("JSON array is empty")

	// ErrJSONNotObjectOrArray is returned when JSON is not an object or array
	ErrJSONNotObjectOrArray = errors.New("JSON is not an object or array")

	// ErrCSVNoDataRows is returned when CSV file has no data rows
	ErrCSVNoDataRows = errors.New("CSV file has no data rows")

	// ErrCSVHeaderMissingImportFields is returned when CSV header does not contain expected import fields
	ErrCSVHeaderMissingImportFields = errors.New("CSV header does not contain expected fields for import")

	// Quote service errors
	// ErrInvalidQuoteRequest is returned when quote request validation fails
	ErrInvalidQuoteRequest = errors.New("invalid quote request")

	// ErrGetTargetStatus is returned when getting target status fails
	ErrGetTargetStatus = errors.New("failed to get target status")

	// ErrTargetStatusNotFound is returned when target status is not found
	ErrTargetStatusNotFound = errors.New("target status not found")

	// ErrTargetStatusNotQuotable is returned when target status is not quotable
	ErrTargetStatusNotQuotable = errors.New("target status is not quotable")

	// ErrCheckQuotePermissions is returned when checking quote permissions fails
	ErrCheckQuotePermissions = errors.New("failed to check quote permissions")

	// ErrNotAuthorizedToQuote is returned when user is not authorized to quote status
	ErrNotAuthorizedToQuote = errors.New("not authorized to quote this status")

	// ErrCreateQuoteStatus is returned when creating quote status fails
	ErrCreateQuoteStatus = errors.New("failed to create quote status")

	// ErrCreateQuoteRelationship is returned when creating quote relationship fails
	ErrCreateQuoteRelationship = errors.New("failed to create quote relationship")

	// ErrGetQuoteRelationships is returned when getting quote relationships fails
	ErrGetQuoteRelationships = errors.New("failed to get quote relationships")

	// ErrGetQuoteRelationship is returned when getting quote relationship fails
	ErrGetQuoteRelationship = errors.New("failed to get quote relationship")

	// ErrQuoteRelationshipNotFound is returned when quote relationship is not found
	ErrQuoteRelationshipNotFound = errors.New("quote relationship not found")

	// ErrNotAuthorizedToDeleteQuote is returned when user is not authorized to delete quote
	ErrNotAuthorizedToDeleteQuote = errors.New("not authorized to delete this quote")

	// ErrWithdrawQuoteRelationship is returned when withdrawing quote relationship fails
	ErrWithdrawQuoteRelationship = errors.New("failed to withdraw quote relationship")

	// ErrGetQuotePermissions is returned when getting quote permissions fails
	ErrGetQuotePermissions = errors.New("failed to get quote permissions")

	// ErrCheckExistingPermissions is returned when checking existing permissions fails
	ErrCheckExistingPermissions = errors.New("failed to check existing permissions")

	// ErrSaveQuotePermissions is returned when saving quote permissions fails
	ErrSaveQuotePermissions = errors.New("failed to save quote permissions")

	// ErrQuoteContentTooLong is returned when quote content is too long
	ErrQuoteContentTooLong = errors.New("quote content too long")

	// Relationship service specific errors
	// ErrCannotFollowSelf is returned when a user tries to follow themselves
	ErrCannotFollowSelf = errors.New("users cannot follow themselves")

	// ErrCannotBlockSelf is returned when a user tries to block themselves
	ErrCannotBlockSelf = errors.New("users cannot block themselves")

	// ErrCannotMuteSelf is returned when a user tries to mute themselves
	ErrCannotMuteSelf = errors.New("users cannot mute themselves")

	// ErrFollowWhileBlocked is returned when trying to follow a user who has blocked you
	ErrFollowWhileBlocked = errors.New("cannot follow user: you are blocked")

	// ErrFailedToCreateFollowRequest is returned when creating follow request fails
	ErrFailedToCreateFollowRequest = errors.New("failed to create follow request")

	// ErrFailedToAcceptFollowRequest is returned when accepting follow request fails
	ErrFailedToAcceptFollowRequest = errors.New("failed to accept follow request")

	// ErrFailedToRejectFollowRequest is returned when rejecting follow request fails
	ErrFailedToRejectFollowRequest = errors.New("failed to reject follow request")

	// ErrFailedToGetExistingRelationship is returned when getting existing relationship fails
	ErrFailedToGetExistingRelationship = errors.New("failed to get existing relationship")

	// ErrFailedToGetUpdatedRelationship is returned when getting updated relationship fails
	ErrFailedToGetUpdatedRelationship = errors.New("failed to get updated relationship")

	// ErrFailedToCheckBlockStatus is returned when checking block status fails
	ErrFailedToCheckBlockStatus = errors.New("failed to check block status")

	// ErrFailedToCheckMuteStatus is returned when checking mute status fails
	ErrFailedToCheckMuteStatus = errors.New("failed to check mute status")

	// ErrFailedToUnfollow is returned when unfollow operation fails
	ErrFailedToUnfollow = errors.New("failed to unfollow")

	// ErrFailedToBlockUser is returned when blocking user fails
	ErrFailedToBlockUser = errors.New("failed to block user")

	// ErrFailedToUnblockUser is returned when unblocking user fails
	ErrFailedToUnblockUser = errors.New("failed to unblock user")

	// ErrFailedToMuteUser is returned when muting user fails
	ErrFailedToMuteUser = errors.New("failed to mute user")

	// ErrFailedToUnmuteUser is returned when unmuting user fails
	ErrFailedToUnmuteUser = errors.New("failed to unmute user")

	// ErrFailedToBuildRelationshipData is returned when building relationship data fails
	ErrFailedToBuildRelationshipData = errors.New("failed to build relationship data")

	// ErrFailedToCountFollowers is returned when counting followers fails
	ErrFailedToCountFollowers = errors.New("failed to count followers")

	// ErrFailedToCountFollowing is returned when counting following fails
	ErrFailedToCountFollowing = errors.New("failed to count following")

	// ErrFollowRequestNotFound is returned when a follow request is not found
	ErrFollowRequestNotFound = errors.New("follow request not found")

	// ErrFailedToAddDomainBlock is returned when adding domain block fails
	ErrFailedToAddDomainBlock = errors.New("failed to add domain block")

	// ErrFailedToRemoveDomainBlock is returned when removing domain block fails
	ErrFailedToRemoveDomainBlock = errors.New("failed to remove domain block")

	// ErrFailedToGetDomainBlocks is returned when getting domain blocks fails
	ErrFailedToGetDomainBlocks = errors.New("failed to get domain blocks")

	// ErrFailedToGetMutedUsers is returned when getting muted users fails
	ErrFailedToGetMutedUsers = errors.New("failed to get muted users")

	// ErrFailedToGetBlockedUsers is returned when getting blocked users fails
	ErrFailedToGetBlockedUsers = errors.New("failed to get blocked users")

	// ErrFailedToGetRelationshipUsers is returned when getting relationship users fails
	ErrFailedToGetRelationshipUsers = errors.New("failed to get relationship users")

	// ErrFailedToGetPendingFollowRequests is returned when getting pending follow requests fails
	ErrFailedToGetPendingFollowRequests = errors.New("failed to get pending follow requests")

	// ErrUnsupportedRelationType is returned when an unsupported relation type is requested
	ErrUnsupportedRelationType = errors.New("unsupported relation type")

	// ErrNoRepositoryOrStorage is returned when neither repository nor storage is available
	ErrNoRepositoryOrStorage = errors.New("no repository or storage available")

	// ErrCannotAcceptOwnFollowRequest is returned when trying to accept your own follow request
	ErrCannotAcceptOwnFollowRequest = errors.New("cannot accept follow request from self")

	// ErrCannotRejectOwnFollowRequest is returned when trying to reject your own follow request
	ErrCannotRejectOwnFollowRequest = errors.New("cannot reject follow request from self")

	// ErrTargetIDsEmpty is returned when target IDs list is empty
	ErrTargetIDsEmpty = errors.New("target_ids cannot be empty")

	// ErrTooManyTargetIDs is returned when too many target IDs are provided
	ErrTooManyTargetIDs = errors.New("too many target_ids (max 40)")

	// Account operation permission errors
	// ErrCannotUpdateProfileForOtherUser is returned when trying to update profile for another user
	ErrCannotUpdateProfileForOtherUser = errors.New("cannot update profile for another user")

	// ErrCannotUpdatePreferencesForOtherUser is returned when trying to update preferences for another user
	ErrCannotUpdatePreferencesForOtherUser = errors.New("cannot update preferences for another user")

	// ErrCannotPinAccountForOtherUser is returned when trying to pin account for another user
	ErrCannotPinAccountForOtherUser = errors.New("cannot pin account for another user")

	// ErrCannotUnpinAccountForOtherUser is returned when trying to unpin account for another user
	ErrCannotUnpinAccountForOtherUser = errors.New("cannot unpin account for another user")

	// ErrCannotSetNoteForOtherUser is returned when trying to set note for another user
	ErrCannotSetNoteForOtherUser = errors.New("cannot set note for another user")

	// ErrCannotRemoveFollowerForOtherUser is returned when trying to remove follower for another user
	ErrCannotRemoveFollowerForOtherUser = errors.New("cannot remove follower for another user")

	// Essential media service errors that are not yet standardized
	// ErrMediaNotFound is returned when media is not found
	ErrMediaNotFound = errors.New("media not found")

	// ErrMediaStorageFailed is returned when media storage fails
	ErrMediaStorageFailed = errors.New("failed to store media record")

	// ErrMediaRetrievalFailed is returned when media retrieval fails
	ErrMediaRetrievalFailed = errors.New("failed to get media")

	// ErrMediaUpdateFailed is returned when media update fails
	ErrMediaUpdateFailed = errors.New("failed to update media")

	// ErrMediaFileDataRequired is returned when file data is required but missing
	ErrMediaFileDataRequired = errors.New("file data is required")

	// ErrMediaFileTooLarge is returned when file size exceeds maximum limit
	ErrMediaFileTooLarge = errors.New("file size too large")

	// ErrMediaUnsupportedType is returned when content type is not supported
	ErrMediaUnsupportedType = errors.New("unsupported content type")

	// ErrMediaFileExtensionMismatch is returned when file extension doesn't match content type
	ErrMediaFileExtensionMismatch = errors.New("file extension does not match content type")

	// ErrMediaNotReady is returned when media is not ready for viewing
	ErrMediaNotReady = errors.New("media not ready for viewing")

	// ErrMediaProcessingQueueFailed is returned when media processing queue operation fails
	ErrMediaProcessingQueueFailed = errors.New("media processing queue failed")

	// ErrMediaNotReadyForStreaming is returned when media is not ready for streaming
	ErrMediaNotReadyForStreaming = errors.New("media not ready for streaming")

	// ErrMediaValidationFailed is returned when media validation fails
	ErrMediaValidationFailed = errors.New("media validation failed")

	// ErrMediaUnauthorizedAccess is returned when unauthorized access to media is attempted
	ErrMediaUnauthorizedAccess = errors.New("unauthorized access to media")

	// List Management errors
	// List CRUD Operations
	// ErrCreateList is returned when list creation fails
	ErrCreateList = errors.New("failed to create list")

	// ErrDeleteList is returned when list deletion fails
	ErrDeleteList = errors.New("failed to delete list")

	// ErrUpdateList is returned when list update fails
	ErrUpdateList = errors.New("failed to update list")

	// ErrGetList is returned when list retrieval fails
	ErrGetList = errors.New("failed to get list")

	// ErrListNotFound is returned when list is not found
	ErrListNotFound = errors.New("list not found")

	// ErrListAlreadyExists is returned when list already exists
	ErrListAlreadyExists = errors.New("list already exists")

	// List Membership Management
	// ErrAddListMember is returned when adding list member fails
	ErrAddListMember = errors.New("failed to add list member")

	// ErrRemoveListMember is returned when removing list member fails
	ErrRemoveListMember = errors.New("failed to remove list member")

	// ErrGetListMembers is returned when getting list members fails
	ErrGetListMembers = errors.New("failed to get list members")

	// ErrMemberNotInList is returned when member is not in list
	ErrMemberNotInList = errors.New("member not in list")

	// ErrMemberAlreadyInList is returned when member is already in list
	ErrMemberAlreadyInList = errors.New("member already in list")

	// List Permissions & Validation
	// ErrInsufficientListPermission is returned when user lacks permission for list operation
	ErrInsufficientListPermission = errors.New("insufficient permission for list operation")

	// ErrCannotModifyList is returned when list cannot be modified
	ErrCannotModifyList = errors.New("cannot modify list")

	// ErrListOwnershipRequired is returned when list ownership is required
	ErrListOwnershipRequired = errors.New("list ownership required")

	// ErrInvalidListName is returned when list name is invalid
	ErrInvalidListName = errors.New("invalid list name")

	// ErrListNameTooLong is returned when list name is too long
	ErrListNameTooLong = errors.New("list name too long")

	// ErrEmptyListName is returned when list name is empty
	ErrEmptyListName = errors.New("list name cannot be empty")

	// ErrInvalidListOperation is returned when list operation is invalid
	ErrInvalidListOperation = errors.New("invalid list operation")

	// Enhanced Media Processing errors
	// Media Upload & Storage
	// ErrUploadMedia is returned when media upload fails
	ErrUploadMedia = errors.New("failed to upload media")

	// ErrProcessMedia is returned when media processing fails
	ErrProcessMedia = errors.New("failed to process media")

	// ErrStoreMedia is returned when media storage fails
	ErrStoreMedia = errors.New("failed to store media")

	// ErrRetrieveMedia is returned when media retrieval fails
	ErrRetrieveMedia = errors.New("failed to retrieve media")

	// Media Validation
	// ErrInvalidMediaType is returned when media type is invalid
	ErrInvalidMediaType = errors.New("invalid media type")

	// ErrMediaTooLarge is returned when media file is too large
	ErrMediaTooLarge = errors.New("media file too large")

	// ErrCorruptedMedia is returned when media file is corrupted
	ErrCorruptedMedia = errors.New("corrupted media file")

	// ErrUnsupportedFormat is returned when media format is unsupported
	ErrUnsupportedFormat = errors.New("unsupported media format")

	// Media Processing
	// ErrTranscodeMedia is returned when media transcoding fails
	ErrTranscodeMedia = errors.New("failed to transcode media")

	// ErrGenerateThumbnail is returned when thumbnail generation fails
	ErrGenerateThumbnail = errors.New("failed to generate thumbnail")

	// ErrExtractMetadata is returned when metadata extraction fails
	ErrExtractMetadata = errors.New("failed to extract metadata")

	// ErrCompressionFailed is returned when media compression fails
	ErrCompressionFailed = errors.New("media compression failed")

	// Enhanced Job Queue Management errors
	// Job Submission
	// ErrSubmitJob is returned when job submission fails
	ErrSubmitJob = errors.New("failed to submit job")

	// ErrQueueJob is returned when job queuing fails
	ErrQueueJob = errors.New("failed to queue job")

	// ErrScheduleJob is returned when job scheduling fails
	ErrScheduleJob = errors.New("failed to schedule job")

	// ErrCancelJob is returned when job cancellation fails
	ErrCancelJob = errors.New("failed to cancel job")

	// Job Processing
	// ErrProcessJob is returned when job processing fails
	ErrProcessJob = errors.New("failed to process job")

	// ErrExecuteJob is returned when job execution fails
	ErrExecuteJob = errors.New("failed to execute job")

	// ErrCompleteJob is returned when job completion fails
	ErrCompleteJob = errors.New("failed to complete job")

	// ErrJobTimeout is returned when job times out
	ErrJobTimeout = errors.New("job execution timeout")

	// Job Validation
	// ErrInvalidJobType is returned when job type is invalid
	ErrInvalidJobType = errors.New("invalid job type")

	// ErrInvalidJobPayload is returned when job payload is invalid
	ErrInvalidJobPayload = errors.New("invalid job payload")

	// ErrJobNotFound is returned when job is not found
	ErrJobNotFound = errors.New("job not found")

	// ErrJobAlreadyProcessed is returned when job is already processed
	ErrJobAlreadyProcessed = errors.New("job already processed")

	// Queue Management
	// ErrQueueFull is returned when queue is full
	ErrQueueFull = errors.New("queue is full")

	// ErrQueueUnavailable is returned when queue is unavailable
	ErrQueueUnavailable = errors.New("queue unavailable")

	// ErrWorkerUnavailable is returned when worker is unavailable
	ErrWorkerUnavailable = errors.New("worker unavailable")

	// ErrRetryLimitExceeded is returned when retry limit is exceeded
	ErrRetryLimitExceeded = errors.New("retry limit exceeded")

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

	// Conversation service errors
	// ErrConversationValidationFailed is returned when conversation validation fails
	ErrConversationValidationFailed = errors.New("validation failed")

	// ErrGetSenderAccount is returned when getting sender account fails
	ErrGetSenderAccount = errors.New("failed to get sender account")

	// ErrInvalidRecipient is returned when recipient validation fails
	ErrInvalidRecipient = errors.New("invalid recipient")

	// ErrLookupExistingConversation is returned when looking up existing conversation fails
	ErrLookupExistingConversation = errors.New("failed to lookup existing conversation")

	// ErrCreateConversation is returned when conversation creation fails
	ErrCreateConversation = errors.New("failed to create conversation")

	// ErrCreateDirectMessage is returned when direct message creation fails
	ErrCreateDirectMessage = errors.New("failed to create direct message")

	// ErrGetConversation is returned when conversation retrieval fails
	ErrGetConversation = errors.New("failed to get conversation")

	// ErrNotConversationParticipant is returned when user is not a participant in conversation
	ErrNotConversationParticipant = errors.New("user is not a participant in this conversation")

	// ErrMarkConversationRead is returned when marking conversation as read fails
	ErrMarkConversationRead = errors.New("failed to mark conversation as read")

	// ErrGetUserConversations is returned when getting user conversations fails
	ErrGetUserConversations = errors.New("failed to get user conversations")

	// ErrGetConversationMessages is returned when getting conversation messages fails
	ErrGetConversationMessages = errors.New("failed to get conversation messages")

	// ErrRecipientsRequired is returned when recipients list is required but empty
	ErrRecipientsRequired = errors.New("recipients is required")

	// ErrContentTooLongConversation is returned when conversation content is too long
	ErrContentTooLongConversation = errors.New("content too long (max 5000 characters)")

	// ErrInvalidInReplyToIDConversation is returned when in_reply_to_id is invalid for conversation
	ErrInvalidInReplyToIDConversation = errors.New("invalid in_reply_to_id")

	// ErrCanOnlyReplyToDirectMessages is returned when attempting to reply to non-direct message
	ErrCanOnlyReplyToDirectMessages = errors.New("can only reply to direct messages")

	// ErrConversationNotFound is returned when conversation is not found
	ErrConversationNotFound = errors.New("conversation not found")

	// ErrDeleteConversation is returned when conversation deletion fails
	ErrDeleteConversation = errors.New("failed to delete conversation")

	// Cost realtime aggregation service errors
	// ErrStreamProcessingErrors is returned when stream processing completes with errors
	ErrStreamProcessingErrors = errors.New("stream processing completed with errors")

	// ErrRecordProcessingFailed is returned when processing records fails
	ErrRecordProcessingFailed = errors.New("failed to process records")

	// ErrProcessAICostRecord is returned when processing AI cost record fails
	ErrProcessAICostRecord = errors.New("failed to process AI cost record")

	// ErrProcessWebSocketCostRecord is returned when processing WebSocket cost record fails
	ErrProcessWebSocketCostRecord = errors.New("failed to process WebSocket cost record")

	// ErrProcessFederationCostRecord is returned when processing federation cost record fails
	ErrProcessFederationCostRecord = errors.New("failed to process federation cost record")

	// ErrUnmarshalAICostRecord is returned when unmarshaling AI cost record fails
	ErrUnmarshalAICostRecord = errors.New("failed to unmarshal AI cost record")

	// ErrUnmarshalWebSocketCostRecord is returned when unmarshaling WebSocket cost record fails
	ErrUnmarshalWebSocketCostRecord = errors.New("failed to unmarshal WebSocket cost record")

	// ErrUnmarshalFederationCostRecord is returned when unmarshaling federation cost record fails
	ErrUnmarshalFederationCostRecord = errors.New("failed to unmarshal federation cost record")

	// ErrUnsupportedEventType is returned when DynamoDB event type is not supported
	ErrUnsupportedEventType = errors.New("unsupported event type")

	// ErrMarshalToJSON is returned when marshaling to JSON fails
	ErrMarshalToJSON = errors.New("failed to marshal to JSON")

	// ErrUnmarshalToTarget is returned when unmarshaling to target fails
	ErrUnmarshalToTarget = errors.New("failed to unmarshal to target")

	// ErrCreateHourlyAggregation is returned when creating hourly aggregation fails
	ErrCreateHourlyAggregation = errors.New("failed to create hourly aggregation")

	// ErrCreateDailyAggregation is returned when creating daily aggregation fails
	ErrCreateDailyAggregation = errors.New("failed to create daily aggregation")

	// AWS Storage Client errors
	// ErrS3BucketConfigRequired is returned when S3 bucket configuration is missing
	ErrS3BucketConfigRequired = errors.New("S3_MEDIA_BUCKET configuration is required")

	// ErrAWSConfigLoadFailed is returned when AWS config loading fails
	ErrAWSConfigLoadFailed = errors.New("failed to load AWS config")

	// ErrS3BucketAccessFailed is returned when S3 bucket access fails
	ErrS3BucketAccessFailed = errors.New("failed to access S3 bucket")

	// ErrPresignedURLCreationFailed is returned when presigned URL creation fails
	ErrPresignedURLCreationFailed = errors.New("failed to create presigned URL")

	// ErrCannotUploadEmptyData is returned when attempting to upload empty data
	ErrCannotUploadEmptyData = errors.New("cannot upload empty data")

	// ErrS3UploadFailed is returned when S3 file upload fails
	ErrS3UploadFailed = errors.New("failed to upload file to S3")

	// ErrS3DownloadFailed is returned when S3 file download fails
	ErrS3DownloadFailed = errors.New("failed to download file from S3")
)