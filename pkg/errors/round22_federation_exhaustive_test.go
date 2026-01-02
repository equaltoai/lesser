package errors

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFederationConstructors_ExhaustiveCoverage(t *testing.T) {
	boom := stdErrors.New("boom")

	base := NewFederationError(CodeRemoteFetchFailed, "base")

	constructors := []struct {
		name string
		fn   func() *AppError
	}{
		{name: "NewFederationError", fn: func() *AppError { return NewFederationError(CodeRemoteFetchFailed, "msg") }},
		{name: "NewFederationInternalError", fn: func() *AppError { return NewFederationInternalError(CodeRemoteFetchFailed, "msg", boom) }},
		{name: "ActivityParsingFailed", fn: func() *AppError { return ActivityParsingFailed("Create", boom) }},
		{name: "ActivityTypeUnsupported", fn: func() *AppError { return ActivityTypeUnsupported("Unknown") }},
		{name: "ActivityMissingField", fn: func() *AppError { return ActivityMissingField("actor") }},
		{name: "ActivityInvalidField", fn: func() *AppError { return ActivityInvalidField("actor", "reason") }},
		{name: "ObjectParsingFailed", fn: func() *AppError { return ObjectParsingFailed("Note", boom) }},
		{name: "ObjectMissingField", fn: func() *AppError { return ObjectMissingField("id") }},
		{name: "ObjectInvalidField", fn: func() *AppError { return ObjectInvalidField("id", "reason") }},
		{name: "ActorNotFound", fn: func() *AppError { return ActorNotFound("actor") }},
		{name: "ActorFetchFailed", fn: func() *AppError { return ActorFetchFailed("actor", boom) }},
		{name: "ActorURIInvalid", fn: func() *AppError { return ActorURIInvalid("uri") }},
		{name: "ActorDomainBlocked", fn: func() *AppError { return ActorDomainBlocked("example.com") }},
		{name: "ActorDomainNotAllowed", fn: func() *AppError { return ActorDomainNotAllowed("example.com") }},
		{name: "HTTPSignatureVerificationFailed", fn: func() *AppError { return HTTPSignatureVerificationFailed("reason") }},
		{name: "SignatureMissing", fn: func() *AppError { return SignatureMissing() }},
		{name: "SignatureInvalid", fn: func() *AppError { return SignatureInvalid("reason") }},
		{name: "SignatureExpired", fn: func() *AppError { return SignatureExpired() }},
		{name: "SigningKeyNotFound", fn: func() *AppError { return SigningKeyNotFound("kid") }},
		{name: "SigningKeyInvalid", fn: func() *AppError { return SigningKeyInvalid("kid") }},
		{name: "InboxProcessingFailed", fn: func() *AppError { return InboxProcessingFailed("reason", boom) }},
		{name: "InboxMessageInvalid", fn: func() *AppError { return InboxMessageInvalid("reason") }},
		{name: "InboxMessageDuplicate", fn: func() *AppError { return InboxMessageDuplicate("aid") }},
		{name: "InboxUnauthorized", fn: func() *AppError { return InboxUnauthorized("actor") }},
		{name: "OutboxProcessingFailed", fn: func() *AppError { return OutboxProcessingFailed("reason", boom) }},
		{name: "OutboxUnauthorized", fn: func() *AppError { return OutboxUnauthorized("actor") }},
		{name: "OutboxActivityInvalid", fn: func() *AppError { return OutboxActivityInvalid("reason") }},
		{name: "DeliveryFailed", fn: func() *AppError { return DeliveryFailed("recipient", boom) }},
		{name: "DeliveryTimeout", fn: func() *AppError { return DeliveryTimeout("recipient") }},
		{name: "DeliveryRejected", fn: func() *AppError { return DeliveryRejected("recipient", 500) }},
		{name: "DeliveryPermanentFailure", fn: func() *AppError { return DeliveryPermanentFailure("recipient", "reason") }},
		{name: "DeliveryToInboxesFailed", fn: func() *AppError { return DeliveryToInboxesFailed(2, boom) }},
		{name: "DeliveryToDomainsFailed", fn: func() *AppError { return DeliveryToDomainsFailed(2, boom) }},
		{name: "NoSharedInboxFound", fn: func() *AppError { return NoSharedInboxFound("example.com") }},
		{name: "RemoteFetchFailed", fn: func() *AppError { return RemoteFetchFailed("url", boom) }},
		{name: "RemoteFetchTimeout", fn: func() *AppError { return RemoteFetchTimeout("url") }},
		{name: "RemoteFetchUnauthorized", fn: func() *AppError { return RemoteFetchUnauthorized("url") }},
		{name: "RemoteFetchNotFound", fn: func() *AppError { return RemoteFetchNotFound("url") }},
		{name: "RemoteFetchRateLimited", fn: func() *AppError { return RemoteFetchRateLimited("url") }},
		{name: "WebFingerFailed", fn: func() *AppError { return WebFingerFailed("acct", boom) }},
		{name: "WebFingerNotFound", fn: func() *AppError { return WebFingerNotFound("acct") }},
		{name: "NodeInfoFailed", fn: func() *AppError { return NodeInfoFailed("example.com", boom) }},
		{name: "FollowRequestInvalid", fn: func() *AppError { return FollowRequestInvalid("reason") }},
		{name: "FollowAlreadyExists", fn: func() *AppError { return FollowAlreadyExists("a", "b") }},
		{name: "FollowNotFound", fn: func() *AppError { return FollowNotFound("a", "b") }},
		{name: "CreateActivityInvalid", fn: func() *AppError { return CreateActivityInvalid("reason") }},
		{name: "CreateObjectMissing", fn: func() *AppError { return CreateObjectMissing() }},
		{name: "CreateObjectInvalid", fn: func() *AppError { return CreateObjectInvalid("reason") }},
		{name: "UpdateActivityInvalid", fn: func() *AppError { return UpdateActivityInvalid("reason") }},
		{name: "UpdateObjectNotFound", fn: func() *AppError { return UpdateObjectNotFound("oid") }},
		{name: "UpdateUnauthorized", fn: func() *AppError { return UpdateUnauthorized("actor", "oid") }},
		{name: "DeleteActivityInvalid", fn: func() *AppError { return DeleteActivityInvalid("reason") }},
		{name: "DeleteObjectNotFound", fn: func() *AppError { return DeleteObjectNotFound("oid") }},
		{name: "DeleteUnauthorized", fn: func() *AppError { return DeleteUnauthorized("actor", "oid") }},
		{name: "LikeActivityInvalid", fn: func() *AppError { return LikeActivityInvalid("reason") }},
		{name: "LikeObjectNotFound", fn: func() *AppError { return LikeObjectNotFound("oid") }},
		{name: "AnnounceActivityInvalid", fn: func() *AppError { return AnnounceActivityInvalid("reason") }},
		{name: "AnnounceObjectNotFound", fn: func() *AppError { return AnnounceObjectNotFound("oid") }},
		{name: "UndoActivityInvalid", fn: func() *AppError { return UndoActivityInvalid("reason") }},
		{name: "UndoObjectNotFound", fn: func() *AppError { return UndoObjectNotFound("oid") }},
		{name: "UndoUnauthorized", fn: func() *AppError { return UndoUnauthorized("actor", "aid") }},
		{name: "BlockActivityInvalid", fn: func() *AppError { return BlockActivityInvalid("reason") }},
		{name: "BlockAlreadyExists", fn: func() *AppError { return BlockAlreadyExists("a", "b") }},
		{name: "InstanceNotFound", fn: func() *AppError { return InstanceNotFound("example.com") }},
		{name: "InstanceSuspended", fn: func() *AppError { return InstanceSuspended("example.com") }},
		{name: "InstanceUnreachable", fn: func() *AppError { return InstanceUnreachable("example.com", boom) }},
		{name: "RoutingFailed", fn: func() *AppError { return RoutingFailed("dest", boom) }},
		{name: "CollectionFetchFailed", fn: func() *AppError { return CollectionFetchFailed("cid", boom) }},
		{name: "CollectionInvalid", fn: func() *AppError { return CollectionInvalid("cid", "reason") }},
		{name: "CollectionItemInvalid", fn: func() *AppError { return CollectionItemInvalid("reason") }},
		{name: "MetricsCollectionFailed", fn: func() *AppError { return MetricsCollectionFailed("m", boom) }},
		{name: "HealthCheckFailed", fn: func() *AppError { return HealthCheckFailed("instance", boom) }},
		{name: "FederationErrorWithRemoteInfo", fn: func() *AppError { return FederationErrorWithRemoteInfo(base, "remote", "actor") }},
		{name: "WrapRemoteError", fn: func() *AppError { return WrapRemoteError(boom, "op", "remote") }},
		{name: "EntityTypeExtractionFailed", fn: func() *AppError { return EntityTypeExtractionFailed(boom) }},
		{name: "ActivityDirectionUnknown", fn: func() *AppError { return ActivityDirectionUnknown("dir") }},
		{name: "ObjectTypeUnsupported", fn: func() *AppError { return ObjectTypeUnsupported("obj") }},
		{name: "UsernameExtractionFailed", fn: func() *AppError { return UsernameExtractionFailed("ctx", boom) }},
		{name: "ObjectIDExtractionFailed", fn: func() *AppError { return ObjectIDExtractionFailed("ctx", boom) }},
		{name: "ActivityTypeExtractionFailed", fn: func() *AppError { return ActivityTypeExtractionFailed("ctx", boom) }},
		{name: "ActorURIExtractionFailed", fn: func() *AppError { return ActorURIExtractionFailed("ctx", boom) }},
		{name: "TargetIDExtractionFailed", fn: func() *AppError { return TargetIDExtractionFailed("ctx", boom) }},
		{name: "TargetCollectionMissing", fn: func() *AppError { return TargetCollectionMissing() }},
		{name: "ObjectsNotFoundInActivity", fn: func() *AppError { return ObjectsNotFoundInActivity() }},
		{name: "OriginalActivityFetchFailed", fn: func() *AppError { return OriginalActivityFetchFailed(boom) }},
		{name: "ActivityNotFoundLocally", fn: func() *AppError { return ActivityNotFoundLocally("aid") }},
		{name: "ObjectHistoryNotFound", fn: func() *AppError { return ObjectHistoryNotFound("oid") }},
		{name: "PreviousStateNotAvailable", fn: func() *AppError { return PreviousStateNotAvailable("oid") }},
		{name: "ObjectNotDeleted", fn: func() *AppError { return ObjectNotDeleted("oid") }},
		{name: "TombstoneStatusCheckFailed", fn: func() *AppError { return TombstoneStatusCheckFailed("oid", boom) }},
		{name: "NoPreviousStateForRestoration", fn: func() *AppError { return NoPreviousStateForRestoration("oid") }},
		{name: "MoveTargetMustBeSpecified", fn: func() *AppError { return MoveTargetMustBeSpecified() }},
		{name: "FlaggedObjectsNotFound", fn: func() *AppError { return FlaggedObjectsNotFound() }},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			assertAppErrorBasics(t, err)
			assert.Equal(t, CategoryFederation, err.Category)
		})
	}
}
