package errors

import "fmt"

// Federation domain errors
// Consolidates ActivityPub federation errors from multiple files

// NewFederationError creates a new federation error with the specified error code and message.
func NewFederationError(code ErrorCode, message string) *AppError {
	return NewAppError(code, CategoryFederation, message)
}

// NewFederationInternalError creates a federation error with internal details wrapped from an underlying error.
func NewFederationInternalError(code ErrorCode, message string, internal error) *AppError {
	return WrapError(internal, code, CategoryFederation, message)
}

// ActivityParsingFailed creates an error indicating ActivityPub activity parsing failed.
func ActivityParsingFailed(activityType string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to parse ActivityPub activity", err).
		WithMetadata("activity_type", activityType)
}

// ActivityTypeUnsupported creates an error indicating an unsupported ActivityPub activity type.
func ActivityTypeUnsupported(activityType string) *AppError {
	return NewFederationError(CodeUnsupportedActivityType, "Unsupported ActivityPub activity type").
		WithMetadata("activity_type", activityType)
}

// ActivityMissingField creates an error indicating an ActivityPub activity is missing a required field.
func ActivityMissingField(field string) *AppError {
	return NewFederationError(CodeValidationFailed, "ActivityPub activity missing required field").
		WithMetadata("field", field)
}

// ActivityInvalidField creates an error indicating an ActivityPub activity has an invalid field.
func ActivityInvalidField(field, reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "ActivityPub activity has invalid field").
		WithMetadata("field", field).
		WithMetadata("reason", reason)
}

// ObjectParsingFailed creates an error indicating ActivityPub object parsing failed.
func ObjectParsingFailed(objectType string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to parse ActivityPub object", err).
		WithMetadata("object_type", objectType)
}

// ObjectMissingField creates an error indicating an ActivityPub object is missing a required field.
func ObjectMissingField(field string) *AppError {
	return NewFederationError(CodeValidationFailed, "ActivityPub object missing required field").
		WithMetadata("field", field)
}

// ObjectInvalidField creates an error indicating an ActivityPub object has an invalid field.
func ObjectInvalidField(field, reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "ActivityPub object has invalid field").
		WithMetadata("field", field).
		WithMetadata("reason", reason)
}

// ActorNotFound creates an error indicating an ActivityPub actor was not found.
func ActorNotFound(actorID string) *AppError {
	return NewFederationError(CodeActorNotFound, "ActivityPub actor not found").
		WithMetadata("actor_id", actorID)
}

// ActorFetchFailed creates an error indicating failed to fetch a remote actor.
func ActorFetchFailed(actorID string, err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "Failed to fetch remote actor", err).
		WithMetadata("actor_id", actorID).AsRetryable()
}

// ActorURIInvalid creates an error indicating an invalid actor URI.
func ActorURIInvalid(uri string) *AppError {
	return NewFederationError(CodeInvalidActorURI, "Invalid actor URI").
		WithMetadata("uri", uri)
}

// ActorDomainBlocked creates an error indicating an actor domain is blocked.
func ActorDomainBlocked(domain string) *AppError {
	return NewFederationError(CodeFederationBlocked, "Actor domain is blocked").
		WithMetadata("domain", domain)
}

// ActorDomainNotAllowed creates an error indicating an actor domain is not in the allowed list.
func ActorDomainNotAllowed(domain string) *AppError {
	return NewFederationError(CodeFederationBlocked, "Actor domain not in allowed list").
		WithMetadata("domain", domain)
}

// HTTPSignatureVerificationFailed creates an error indicating HTTP signature verification failed.
func HTTPSignatureVerificationFailed(reason string) *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "HTTP signature verification failed").
		WithMetadata("reason", reason)
}

// SignatureMissing creates an error indicating an HTTP signature is missing.
func SignatureMissing() *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "HTTP signature is missing")
}

// SignatureInvalid creates an error indicating an invalid HTTP signature.
func SignatureInvalid(reason string) *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "Invalid HTTP signature").
		WithMetadata("reason", reason)
}

// SignatureExpired creates an error indicating an HTTP signature has expired.
func SignatureExpired() *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "HTTP signature has expired")
}

// SigningKeyNotFound creates an error indicating a signing key was not found.
func SigningKeyNotFound(keyID string) *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "Signing key not found").
		WithMetadata("key_id", keyID)
}

// SigningKeyInvalid creates an error indicating an invalid signing key.
func SigningKeyInvalid(keyID string) *AppError {
	return NewFederationError(CodeSignatureVerifyFailed, "Invalid signing key").
		WithMetadata("key_id", keyID)
}

// InboxProcessingFailed creates an error indicating inbox processing failed.
func InboxProcessingFailed(reason string, err error) *AppError {
	return NewFederationInternalError(CodeInboxProcessingFailed, "Inbox processing failed", err).
		WithMetadata("reason", reason).AsRetryable()
}

// InboxMessageInvalid creates an error indicating an invalid inbox message.
func InboxMessageInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid inbox message").
		WithMetadata("reason", reason)
}

// InboxMessageDuplicate creates an error indicating a duplicate inbox message.
func InboxMessageDuplicate(activityID string) *AppError {
	return NewFederationError(CodeAlreadyExists, "Duplicate inbox message").
		WithMetadata("activity_id", activityID)
}

// InboxUnauthorized creates an error indicating unauthorized inbox access.
func InboxUnauthorized(actorID string) *AppError {
	return NewFederationError(CodeUnauthorized, "Unauthorized inbox access").
		WithMetadata("actor_id", actorID)
}

// OutboxProcessingFailed creates an error indicating outbox processing failed.
func OutboxProcessingFailed(reason string, err error) *AppError {
	return NewFederationInternalError(CodeOutboxProcessingFailed, "Outbox processing failed", err).
		WithMetadata("reason", reason).AsRetryable()
}

// OutboxUnauthorized creates an error indicating unauthorized outbox access.
func OutboxUnauthorized(actorID string) *AppError {
	return NewFederationError(CodeUnauthorized, "Unauthorized outbox access").
		WithMetadata("actor_id", actorID)
}

// OutboxActivityInvalid creates an error indicating an invalid outbox activity.
func OutboxActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid outbox activity").
		WithMetadata("reason", reason)
}

// DeliveryFailed creates an error indicating federation delivery failed.
func DeliveryFailed(recipient string, err error) *AppError {
	return NewFederationInternalError(CodeDeliveryFailed, "Federation delivery failed", err).
		WithMetadata("recipient", recipient).AsRetryable()
}

// DeliveryTimeout creates an error indicating federation delivery timed out.
func DeliveryTimeout(recipient string) *AppError {
	return NewFederationError(CodeTimeout, "Federation delivery timed out").
		WithMetadata("recipient", recipient).AsRetryable()
}

// DeliveryRejected creates an error indicating federation delivery was rejected.
func DeliveryRejected(recipient string, statusCode int) *AppError {
	return NewFederationError(CodeDeliveryFailed, "Federation delivery rejected").
		WithMetadata("recipient", recipient).
		WithMetadata("status_code", statusCode)
}

// DeliveryPermanentFailure creates an error indicating a permanent delivery failure.
func DeliveryPermanentFailure(recipient string, reason string) *AppError {
	return NewFederationError(CodeDeliveryFailed, "Permanent delivery failure").
		WithMetadata("recipient", recipient).
		WithMetadata("reason", reason)
}

// DeliveryToInboxesFailed creates an error indicating failed delivery to multiple inboxes.
func DeliveryToInboxesFailed(count int, err error) *AppError {
	return NewFederationInternalError(CodeDeliveryFailed, "Failed to deliver to multiple inboxes", err).
		WithMetadata("inbox_count", count).AsRetryable()
}

// DeliveryToDomainsFailed creates an error indicating failed delivery to multiple domains.
func DeliveryToDomainsFailed(count int, err error) *AppError {
	return NewFederationInternalError(CodeDeliveryFailed, "Failed to deliver to multiple domains", err).
		WithMetadata("domain_count", count).AsRetryable()
}

// NoSharedInboxFound creates an error indicating no shared inbox was found for a domain.
func NoSharedInboxFound(domain string) *AppError {
	return NewFederationError(CodeRemoteFetchFailed, "No shared inbox found for domain").
		WithMetadata("domain", domain)
}

// RemoteFetchFailed creates an error indicating failed to fetch a remote resource.
func RemoteFetchFailed(url string, err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "Failed to fetch remote resource", err).
		WithMetadata("url", url).AsRetryable()
}

// RemoteFetchTimeout creates an error indicating remote fetch timed out.
func RemoteFetchTimeout(url string) *AppError {
	return NewFederationError(CodeTimeout, "Remote fetch timed out").
		WithMetadata("url", url).AsRetryable()
}

// RemoteFetchUnauthorized creates an error indicating remote fetch was unauthorized.
func RemoteFetchUnauthorized(url string) *AppError {
	return NewFederationError(CodeUnauthorized, "Remote fetch unauthorized").
		WithMetadata("url", url)
}

// RemoteFetchNotFound creates an error indicating a remote resource was not found.
func RemoteFetchNotFound(url string) *AppError {
	return NewFederationError(CodeNotFound, "Remote resource not found").
		WithMetadata("url", url)
}

// RemoteFetchRateLimited creates an error indicating remote fetch was rate limited.
func RemoteFetchRateLimited(url string) *AppError {
	return NewFederationError(CodeRateLimited, "Remote fetch rate limited").
		WithMetadata("url", url).AsRetryable()
}

// WebFingerFailed creates an error indicating WebFinger lookup failed.
func WebFingerFailed(identifier string, err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "WebFinger lookup failed", err).
		WithMetadata("identifier", identifier).AsRetryable()
}

// WebFingerNotFound creates an error indicating a WebFinger resource was not found.
func WebFingerNotFound(identifier string) *AppError {
	return NewFederationError(CodeNotFound, "WebFinger resource not found").
		WithMetadata("identifier", identifier)
}

// NodeInfoFailed creates an error indicating NodeInfo fetch failed.
func NodeInfoFailed(domain string, err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "NodeInfo fetch failed", err).
		WithMetadata("domain", domain).AsRetryable()
}

// FollowRequestInvalid creates an error indicating an invalid follow request.
func FollowRequestInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid follow request").
		WithMetadata("reason", reason)
}

// FollowAlreadyExists creates an error indicating a follow relationship already exists.
func FollowAlreadyExists(follower, followee string) *AppError {
	return NewFederationError(CodeAlreadyExists, "Follow relationship already exists").
		WithMetadata("follower", follower).
		WithMetadata("followee", followee)
}

// FollowNotFound creates an error indicating a follow relationship was not found.
func FollowNotFound(follower, followee string) *AppError {
	return NewFederationError(CodeNotFound, "Follow relationship not found").
		WithMetadata("follower", follower).
		WithMetadata("followee", followee)
}

// CreateActivityInvalid creates an error indicating an invalid create activity.
func CreateActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid create activity").
		WithMetadata("reason", reason)
}

// CreateObjectMissing creates an error indicating a create activity is missing its object.
func CreateObjectMissing() *AppError {
	return NewFederationError(CodeValidationFailed, "Create activity missing object")
}

// CreateObjectInvalid creates an error indicating an invalid create object.
func CreateObjectInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid create object").
		WithMetadata("reason", reason)
}

// UpdateActivityInvalid creates an error indicating an invalid update activity.
func UpdateActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid update activity").
		WithMetadata("reason", reason)
}

// UpdateObjectNotFound creates an error indicating an update target object was not found.
func UpdateObjectNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Update target object not found").
		WithMetadata("object_id", objectID)
}

// UpdateUnauthorized creates an error indicating unauthorized access to update an object.
func UpdateUnauthorized(actorID, objectID string) *AppError {
	return NewFederationError(CodeUnauthorized, "Unauthorized to update object").
		WithMetadata("actor_id", actorID).
		WithMetadata("object_id", objectID)
}

// DeleteActivityInvalid creates an error indicating an invalid delete activity.
func DeleteActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid delete activity").
		WithMetadata("reason", reason)
}

// DeleteObjectNotFound creates an error indicating a delete target object was not found.
func DeleteObjectNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Delete target object not found").
		WithMetadata("object_id", objectID)
}

// DeleteUnauthorized creates an error indicating unauthorized access to delete an object.
func DeleteUnauthorized(actorID, objectID string) *AppError {
	return NewFederationError(CodeUnauthorized, "Unauthorized to delete object").
		WithMetadata("actor_id", actorID).
		WithMetadata("object_id", objectID)
}

// LikeActivityInvalid creates an error indicating an invalid like activity.
func LikeActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid like activity").
		WithMetadata("reason", reason)
}

// LikeObjectNotFound creates an error indicating a like target object was not found.
func LikeObjectNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Like target object not found").
		WithMetadata("object_id", objectID)
}

// AnnounceActivityInvalid creates an error indicating an invalid announce activity.
func AnnounceActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid announce activity").
		WithMetadata("reason", reason)
}

// AnnounceObjectNotFound creates an error indicating an announce target object was not found.
func AnnounceObjectNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Announce target object not found").
		WithMetadata("object_id", objectID)
}

// UndoActivityInvalid creates an error indicating an invalid undo activity.
func UndoActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid undo activity").
		WithMetadata("reason", reason)
}

// UndoObjectNotFound creates an error indicating an undo target activity was not found.
func UndoObjectNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Undo target activity not found").
		WithMetadata("object_id", objectID)
}

// UndoUnauthorized creates an error indicating unauthorized access to undo an activity.
func UndoUnauthorized(actorID, activityID string) *AppError {
	return NewFederationError(CodeUnauthorized, "Unauthorized to undo activity").
		WithMetadata("actor_id", actorID).
		WithMetadata("activity_id", activityID)
}

// BlockActivityInvalid creates an error indicating an invalid block activity.
func BlockActivityInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid block activity").
		WithMetadata("reason", reason)
}

// BlockAlreadyExists creates an error indicating a block relationship already exists.
func BlockAlreadyExists(blocker, blocked string) *AppError {
	return NewFederationError(CodeAlreadyExists, "Block relationship already exists").
		WithMetadata("blocker", blocker).
		WithMetadata("blocked", blocked)
}

// InstanceNotFound creates an error indicating a federation instance was not found.
func InstanceNotFound(domain string) *AppError {
	return NewFederationError(CodeNotFound, "Federation instance not found").
		WithMetadata("domain", domain)
}

// InstanceSuspended creates an error indicating a federation instance is suspended.
func InstanceSuspended(domain string) *AppError {
	return NewFederationError(CodeForbidden, "Federation instance is suspended").
		WithMetadata("domain", domain)
}

// InstanceUnreachable creates an error indicating a federation instance is unreachable.
func InstanceUnreachable(domain string, err error) *AppError {
	return NewFederationInternalError(CodeExternalServiceUnavailable, "Federation instance unreachable", err).
		WithMetadata("domain", domain).AsRetryable()
}

// RoutingFailed creates an error indicating federation routing failed.
func RoutingFailed(destination string, err error) *AppError {
	return NewFederationInternalError(CodeDeliveryFailed, "Federation routing failed", err).
		WithMetadata("destination", destination).AsRetryable()
}

// CollectionFetchFailed creates an error indicating failed to fetch a collection.
func CollectionFetchFailed(collectionID string, err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "Failed to fetch collection", err).
		WithMetadata("collection_id", collectionID).AsRetryable()
}

// CollectionInvalid creates an error indicating an invalid collection.
func CollectionInvalid(collectionID, reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid collection").
		WithMetadata("collection_id", collectionID).
		WithMetadata("reason", reason)
}

// CollectionItemInvalid creates an error indicating an invalid collection item.
func CollectionItemInvalid(reason string) *AppError {
	return NewFederationError(CodeValidationFailed, "Invalid collection item").
		WithMetadata("reason", reason)
}

// MetricsCollectionFailed creates an error indicating federation metrics collection failed.
func MetricsCollectionFailed(metric string, err error) *AppError {
	return NewFederationInternalError(CodeInternal, "Federation metrics collection failed", err).
		WithMetadata("metric", metric)
}

// HealthCheckFailed creates an error indicating a federation health check failed.
func HealthCheckFailed(instance string, err error) *AppError {
	return NewFederationInternalError(CodeExternalServiceUnavailable, "Federation health check failed", err).
		WithMetadata("instance", instance)
}

// FederationErrorWithRemoteInfo adds remote instance and actor information to a federation error.
func FederationErrorWithRemoteInfo(baseErr *AppError, remoteInstance, remoteActor string) *AppError {
	return baseErr.WithMetadata("remote_instance", remoteInstance).
		WithMetadata("remote_actor", remoteActor)
}

// WrapRemoteError wraps an error with remote operation context and makes it retryable.
func WrapRemoteError(err error, operation, remoteInstance string) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, 
		fmt.Sprintf("Remote %s failed", operation), err).
		WithMetadata("remote_instance", remoteInstance).
		AsRetryable()
}

// ActivityPub Activity Processing Specific Errors

// EntityTypeExtractionFailed creates an error for entity type extraction failures.
func EntityTypeExtractionFailed(err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to get entity type", err)
}

// ActivityDirectionUnknown creates an error for unknown activity directions.
func ActivityDirectionUnknown(direction string) *AppError {
	return NewFederationError(CodeValidationFailed, "Unknown activity direction").
		WithMetadata("direction", direction)
}

// ObjectTypeUnsupported creates an error for unsupported object types.
func ObjectTypeUnsupported(objectType string) *AppError {
	return NewFederationError(CodeUnsupportedActivityType, "Unsupported object type").
		WithMetadata("object_type", objectType)
}

// UsernameExtractionFailed creates an error for username extraction failures.
func UsernameExtractionFailed(context string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to extract username", err).
		WithMetadata("context", context)
}

// ObjectIDExtractionFailed creates an error for object ID extraction failures.
func ObjectIDExtractionFailed(context string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to extract object ID", err).
		WithMetadata("context", context)
}

// ActivityTypeExtractionFailed creates an error for activity type extraction failures.
func ActivityTypeExtractionFailed(context string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to extract activity type", err).
		WithMetadata("context", context)
}

// ActorURIExtractionFailed creates an error for actor URI extraction failures.
func ActorURIExtractionFailed(context string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to extract actor URI", err).
		WithMetadata("context", context)
}

// TargetIDExtractionFailed creates an error for target ID extraction failures.
func TargetIDExtractionFailed(context string, err error) *AppError {
	return NewFederationInternalError(CodeActivityParsingFailed, "Failed to extract target ID", err).
		WithMetadata("context", context)
}

// TargetCollectionMissing creates an error when a target collection is missing.
func TargetCollectionMissing() *AppError {
	return NewFederationError(CodeValidationFailed, "Activity missing target collection")
}

// ObjectsNotFoundInActivity creates an error when no objects are found in an activity.
func ObjectsNotFoundInActivity() *AppError {
	return NewFederationError(CodeValidationFailed, "No objects found in activity")
}

// OriginalActivityFetchFailed creates an error for fetching original activity failures.
func OriginalActivityFetchFailed(err error) *AppError {
	return NewFederationInternalError(CodeRemoteFetchFailed, "Failed to fetch original activity", err).AsRetryable()
}

// ActivityNotFoundLocally creates an error when an activity is not found locally.
func ActivityNotFoundLocally(activityID string) *AppError {
	return NewFederationError(CodeNotFound, "Activity not found locally").
		WithMetadata("activity_id", activityID)
}

// ObjectHistoryNotFound creates an error when object history is not available.
func ObjectHistoryNotFound(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "No history found for object").
		WithMetadata("object_id", objectID)
}

// PreviousStateNotAvailable creates an error when previous state is not available.
func PreviousStateNotAvailable(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "Previous state not available for object").
		WithMetadata("object_id", objectID)
}

// ObjectNotDeleted creates an error when an object is not deleted as expected.
func ObjectNotDeleted(objectID string) *AppError {
	return NewFederationError(CodeInvalidStateTransition, "Object is not deleted").
		WithMetadata("object_id", objectID)
}

// TombstoneStatusCheckFailed creates an error for tombstone status check failures.
func TombstoneStatusCheckFailed(objectID string, err error) *AppError {
	return NewFederationInternalError(CodeInternal, "Failed to check tombstone status", err).
		WithMetadata("object_id", objectID)
}

// NoPreviousStateForRestoration creates an error when no previous state is available for restoration.
func NoPreviousStateForRestoration(objectID string) *AppError {
	return NewFederationError(CodeNotFound, "No previous state available for restoration").
		WithMetadata("object_id", objectID)
}

// MoveTargetMustBeSpecified creates an error when move activity must specify a target.
func MoveTargetMustBeSpecified() *AppError {
	return NewFederationError(CodeValidationFailed, "Move activity must specify a target account")
}

// FlaggedObjectsNotFound creates an error when no flagged objects are found.
func FlaggedObjectsNotFound() *AppError {
	return NewFederationError(CodeValidationFailed, "No flagged objects found in Flag activity")
}