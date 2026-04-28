package federation

import (
	"errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

// Federation delivery errors - now using centralized error system
var (
	// ErrDeliveryFailed is returned when federation delivery fails
	ErrDeliveryFailed = apperrors.DeliveryFailed("delivery", errors.New("delivery failed"))

	// ErrMessageParseFailed is returned when federation message parsing fails
	ErrMessageParseFailed = apperrors.ActivityParsingFailed("message", errors.New("message parsing failed"))

	// ErrSigningActorNotFound is returned when signing actor cannot be retrieved
	ErrSigningActorNotFound = apperrors.ActorNotFound("")

	// ErrMessageMarshalFailed is returned when message marshaling fails
	ErrMessageMarshalFailed = apperrors.ActivityParsingFailed("message", errors.New("message marshaling failed"))

	// ErrMessageRequeueFailed is returned when message requeuing fails
	ErrMessageRequeueFailed = apperrors.DeliveryFailed("requeue", errors.New("message requeue failed"))

	// ErrDeliveryPermanentFailure is returned when delivery permanently failed
	ErrDeliveryPermanentFailure = apperrors.DeliveryPermanentFailure("", "")

	// ErrInstanceStatsNotFound is returned when instance stats cannot be found
	ErrInstanceStatsNotFound = apperrors.InstanceNotFound("")

	// ErrDeliveryHTTPStatusFailed is returned when delivery fails with non-2xx status
	ErrDeliveryHTTPStatusFailed = apperrors.DeliveryRejected("", 0)

	// ErrDeliveryToInboxesFailed is returned when delivery to multiple inboxes fails
	ErrDeliveryToInboxesFailed = apperrors.DeliveryToInboxesFailed(0, errors.New("delivery to inboxes failed"))

	// ErrDeliveryToDomainsFailed is returned when delivery to multiple domains fails
	ErrDeliveryToDomainsFailed = apperrors.DeliveryToDomainsFailed(0, errors.New("delivery to domains failed"))

	// ErrDeliveryToRecipientsFailed is returned when delivery to multiple recipients fails
	ErrDeliveryToRecipientsFailed = apperrors.DeliveryToInboxesFailed(0, errors.New("delivery to recipients failed"))

	// ErrNoSharedInboxFound is returned when no shared inbox found for domain
	ErrNoSharedInboxFound = apperrors.NoSharedInboxFound("")

	// ErrFetchActorHTTPStatusFailed is returned when actor fetch fails with non-2xx status
	ErrFetchActorHTTPStatusFailed = apperrors.ActorFetchFailed("", errors.New("actor fetch failed"))

	// ErrDeliveryDirectMessageToInboxesFailed is returned when direct message delivery to multiple inboxes fails
	ErrDeliveryDirectMessageToInboxesFailed = apperrors.DeliveryToInboxesFailed(0, errors.New("direct message delivery failed"))

	// ErrActivityMarshalFailed is returned when activity marshaling fails
	ErrActivityMarshalFailed = apperrors.ActivityParsingFailed("activity", errors.New("activity marshaling failed"))

	// ErrRequestCreationFailed is returned when HTTP request creation fails
	ErrRequestCreationFailed = apperrors.RemoteFetchFailed("", errors.New("request creation failed"))

	// ErrPrivateKeyRetrievalFailed is returned when private key retrieval fails
	ErrPrivateKeyRetrievalFailed = apperrors.SigningKeyNotFound("")

	// ErrPrivateKeyParseFailed is returned when private key parsing fails
	ErrPrivateKeyParseFailed = apperrors.SigningKeyInvalid("")

	// ErrRequestSigningFailed is returned when HTTP request signing fails
	ErrRequestSigningFailed = apperrors.HTTPSignatureVerificationFailed("")

	// ErrHTTPRequestFailed is returned when HTTP request execution fails
	ErrHTTPRequestFailed = apperrors.RemoteFetchFailed("", errors.New("http request failed"))

	// ErrGetFollowersFailed is returned when retrieving followers fails
	ErrGetFollowersFailed = apperrors.CollectionFetchFailed("", errors.New("get followers failed"))

	// ErrActorDecodeFailed is returned when actor decoding fails
	ErrActorDecodeFailed = apperrors.ActorFetchFailed("", errors.New("actor decode failed"))

	// ErrFetchRemoteActorFailed is returned when remote actor fetching fails
	ErrFetchRemoteActorFailed = apperrors.ActorFetchFailed("", errors.New("fetch remote actor failed"))

	// Enhanced retry specific errors
	// ErrRetryMessageMarshalFailed is returned when retry message marshaling fails
	ErrRetryMessageMarshalFailed = apperrors.ActivityParsingFailed("retry message", errors.New("retry message marshaling failed"))

	// ErrRetryQueueFailed is returned when queuing for enhanced retry fails
	ErrRetryQueueFailed = apperrors.DeliveryFailed("retry queue", errors.New("retry queue failed"))

	// ErrEnhancedDeliveryIDGenFailed is returned when enhanced delivery ID generation fails
	ErrEnhancedDeliveryIDGenFailed = apperrors.DeliveryFailed("id gen", errors.New("delivery ID generation failed"))

	// ErrRetryLimitExceeded is returned when maximum retry attempts are exceeded
	ErrRetryLimitExceeded = apperrors.DeliveryPermanentFailure("", "retry limit exceeded")

	// ErrRetryDeliveryFailed is returned when retry delivery fails
	ErrRetryDeliveryFailed = apperrors.DeliveryFailed("retry", errors.New("retry delivery failed"))

	// ErrBackoffCalculationFailed is returned when backoff calculation fails
	ErrBackoffCalculationFailed = apperrors.DeliveryFailed("backoff", errors.New("backoff calculation failed"))
)

// HTTP Signature errors - now using centralized error system
var (
	// ErrBuildSignatureString is returned when signature string building fails
	ErrBuildSignatureString = apperrors.SignatureInvalid("failed to build signature string")

	// ErrECDSAVerificationFailed is returned when ECDSA signature verification fails
	ErrECDSAVerificationFailed = apperrors.HTTPSignatureVerificationFailed("ECDSA signature verification failed")

	// ErrEd25519VerificationFailed is returned when Ed25519 signature verification fails
	ErrEd25519VerificationFailed = apperrors.HTTPSignatureVerificationFailed("Ed25519 signature verification failed")

	// ErrUnsupportedPublicKeyType is returned when public key type is not supported for hs2019
	ErrUnsupportedPublicKeyType = apperrors.SigningKeyInvalid("unsupported public key type for hs2019")

	// ErrAlgorithmRequiresRSA is returned when algorithm requires RSA key but different key type provided
	ErrAlgorithmRequiresRSA = apperrors.SignatureInvalid("algorithm requires RSA key")

	// ErrAlgorithmRequiresECDSA is returned when algorithm requires ECDSA key but different key type provided
	ErrAlgorithmRequiresECDSA = apperrors.SignatureInvalid("algorithm requires ECDSA key")

	// ErrAlgorithmRequiresEd25519 is returned when algorithm requires Ed25519 key but different key type provided
	ErrAlgorithmRequiresEd25519 = apperrors.SignatureInvalid("algorithm requires Ed25519 key")

	// ErrUnsupportedAlgorithm is returned when signature algorithm is not supported
	ErrUnsupportedAlgorithm = apperrors.SignatureInvalid("unsupported algorithm")

	// ErrSignatureFailed is returned when signing operation fails
	ErrSignatureFailed = apperrors.HTTPSignatureVerificationFailed("failed to sign")

	// ErrUnsupportedPrivateKeyType is returned when private key type is not supported for hs2019
	ErrUnsupportedPrivateKeyType = apperrors.SigningKeyInvalid("unsupported private key type for hs2019")

	// ErrInvalidSignatureInputFormat is returned when signature-input format is invalid
	ErrInvalidSignatureInputFormat = apperrors.SignatureInvalid("invalid signature-input format: missing parentheses")

	// ErrDecodeSignature is returned when signature decoding fails
	ErrDecodeSignature = apperrors.SignatureInvalid("failed to decode signature")

	// ErrInvalidSignatureHeaderFormat is returned when signature header format is invalid
	ErrInvalidSignatureHeaderFormat = apperrors.SignatureInvalid("invalid signature header format")

	// ErrMissingKeyID is returned when keyId is missing in signature
	ErrMissingKeyID = apperrors.SignatureMissing()

	// ErrMissingSignatureValue is returned when signature value is missing
	ErrMissingSignatureValue = apperrors.SignatureMissing()

	// ErrRequiredHeaderNotFound is returned when a required header is not found
	ErrRequiredHeaderNotFound = apperrors.SignatureInvalid("required header not found")

	// ErrFailedToParsePEMBlock is returned when PEM block parsing fails
	ErrFailedToParsePEMBlock = apperrors.SigningKeyInvalid("failed to parse PEM block")

	// ErrUnsupportedKeyType is returned when key type is not supported
	ErrUnsupportedKeyType = apperrors.SigningKeyInvalid("unsupported key type")

	// ErrKeySizeTooSmall is returned when key size is insufficient
	ErrKeySizeTooSmall = apperrors.SigningKeyInvalid("key size must be at least 2048 bits")

	// ErrInvalidSignatureHeaderFormatWrapper is returned when signature header format validation fails
	ErrInvalidSignatureHeaderFormatWrapper = apperrors.SignatureInvalid("invalid signature header format")

	// ErrDecodeSignatureFailed is returned when base64 signature decoding fails
	ErrDecodeSignatureFailed = apperrors.SignatureInvalid("failed to decode signature")

	// ErrSignatureParseFailed is already defined above as general signature parsing error

	// ErrReadRequestBodyFailed is returned when reading HTTP request body fails
	ErrReadRequestBodyFailed = apperrors.RemoteFetchFailed("", errors.New("read request body failed"))

	// ErrRSAKeyGenFailed is returned when RSA key generation fails
	ErrRSAKeyGenFailed = apperrors.SigningKeyInvalid("failed to generate RSA key pair")

	// ErrMarshalPublicKeyFailed is returned when marshaling public key fails
	ErrMarshalPublicKeyFailed = apperrors.SigningKeyInvalid("failed to marshal public key")

	// ErrMarshalPrivateKeyFailed is returned when marshaling private key fails
	ErrMarshalPrivateKeyFailed = apperrors.SigningKeyInvalid("failed to marshal private key")
)

// Inbox recovery errors - now using centralized error system
var (
	// ErrMissingRequestIDInConfirmation is returned when request ID is missing in recovery confirmation
	ErrMissingRequestIDInConfirmation = apperrors.InboxMessageInvalid("missing request ID in recovery confirmation")

	// ErrMissingActorInConfirmation is returned when actor is missing in recovery confirmation
	ErrMissingActorInConfirmation = apperrors.InboxMessageInvalid("missing actor in recovery confirmation")

	// ErrMissingInviterUsername is returned when inviter username is missing in trustee acceptance
	ErrMissingInviterUsername = apperrors.InboxMessageInvalid("missing inviter username in trustee acceptance")

	// ErrMissingActorInAcceptance is returned when actor is missing in trustee acceptance
	ErrMissingActorInAcceptance = apperrors.InboxMessageInvalid("missing actor in trustee acceptance")
)

// Trend analysis errors - now using centralized error system
var (
	// ErrGetConnectionsFailed is returned when getting instance connections fails during trend analysis
	ErrGetConnectionsFailed = apperrors.RemoteFetchFailed("", errors.New("get connections failed"))
)

// Authorized fetch errors - now using centralized error system
var (
	// ErrFetchObjectHTTPFailed is returned when HTTP request fails during object fetch
	ErrFetchObjectHTTPFailed = apperrors.RemoteFetchFailed("", errors.New("fetch object http failed"))

	// ErrInvalidActorObjectType is returned when fetched object is not a valid actor type
	ErrInvalidActorObjectType = apperrors.ObjectInvalidField("type", "invalid actor object type")

	// ErrNotActorObject is returned when object type is not a valid actor type
	ErrNotActorObject = apperrors.ObjectInvalidField("type", "not an actor object")

	// ErrMissingSignatureHeader is returned when signature header is missing in authorized fetch request
	ErrMissingSignatureHeader = apperrors.SignatureMissing()

	// ErrExtractActorIDFailed is returned when actor ID cannot be extracted from keyId
	ErrExtractActorIDFailed = apperrors.ActorURIExtractionFailed("keyId extraction", errors.New("extract actor ID failed"))

	// ErrObjectIDMismatch is returned when fetched object ID doesn't match requested ID
	ErrObjectIDMismatch = apperrors.ObjectInvalidField("id", "object ID mismatch")

	// ErrObjectMissingType is returned when object is missing type field
	ErrObjectMissingType = apperrors.ObjectMissingField("type")

	// ErrFetchActorHTTPFailed is returned when HTTP request fails during actor fetch
	ErrFetchActorHTTPFailed = apperrors.ActorFetchFailed("", errors.New("fetch actor http failed"))

	// ErrRepositoryAccessValidationFailed is returned when repository access validation fails
	ErrRepositoryAccessValidationFailed = apperrors.RemoteFetchUnauthorized("")

	// ErrActorDataMarshalFailed is returned when actor data marshaling fails
	ErrActorDataMarshalFailed = apperrors.ActivityParsingFailed("actor", errors.New("actor data marshal failed"))

	// ErrActorUnmarshalFailed is returned when actor unmarshaling fails
	ErrActorUnmarshalFailed = apperrors.ActivityParsingFailed("actor", errors.New("actor unmarshal failed"))

	// ErrSignatureParseFailed is returned when signature parsing fails
	ErrSignatureParseFailed = apperrors.SignatureInvalid("failed to parse signature")

	// ErrPublicKeyParseFailed is returned when public key parsing fails
	ErrPublicKeyParseFailed = apperrors.SigningKeyInvalid("failed to parse public key")

	// ErrSignatureVerificationFailed is returned when signature verification fails
	ErrSignatureVerificationFailed = apperrors.HTTPSignatureVerificationFailed("signature verification failed")

	// ErrResponseDecodeFailed is returned when response decoding fails
	ErrResponseDecodeFailed = apperrors.RemoteFetchFailed("", errors.New("response decode failed"))

	// ErrObjectValidationFailed is returned when object validation fails
	ErrObjectValidationFailed = apperrors.ObjectParsingFailed("", errors.New("object validation failed"))

	// ErrInvalidCachedPublicKey is returned when cached public key is invalid
	ErrInvalidCachedPublicKey = apperrors.SigningKeyInvalid("invalid cached public key")

	// ErrPublicKeyFetchFailed is returned when public key fetch fails after retries
	ErrPublicKeyFetchFailed = apperrors.SigningKeyNotFound("failed to fetch public key after retries")

	// ErrActorHasNoPublicKey is returned when actor has no public key
	ErrActorHasNoPublicKey = apperrors.SigningKeyNotFound("actor has no public key")

	// ErrPublicKeyExtractionFailed is returned when public key extraction fails
	ErrPublicKeyExtractionFailed = apperrors.SigningKeyInvalid("failed to extract public key")
)

// Relationship tracker errors - now using centralized error system
var (
	// ErrS3ClientNotConfigured is returned when S3 client is not configured for restore operation
	ErrS3ClientNotConfigured = apperrors.NewFederationError(apperrors.CodeDependencyNotMet, "S3 client not configured for restore operation")

	// ErrArchiveContainsNoRelationships is returned when archive contains no relationships
	ErrArchiveContainsNoRelationships = apperrors.CollectionInvalid("", "archive contains no relationships")

	// Database operation errors
	// ErrGetFederationEdgesFailed is returned when getting federation edges fails
	ErrGetFederationEdgesFailed = apperrors.CollectionFetchFailed("federation edges", errors.New("get federation edges failed"))

	// ErrGetConnectionsFailed is already defined above in trend analysis errors

	// ErrGetCreateRelationshipFailed is returned when getting or creating relationship fails
	ErrGetCreateRelationshipFailed = apperrors.FollowNotFound("", "")

	// ErrGetCreateAggregateFailed is returned when getting or creating aggregate fails
	ErrGetCreateAggregateFailed = apperrors.CollectionFetchFailed("aggregate", errors.New("get create aggregate failed"))

	// ErrQueryRelationshipFailed is returned when querying relationship fails
	ErrQueryRelationshipFailed = apperrors.FollowNotFound("", "")

	// ErrQueryAggregateFailed is returned when querying aggregate fails
	ErrQueryAggregateFailed = apperrors.CollectionFetchFailed("aggregate", errors.New("query aggregate failed"))

	// ErrSaveRelationshipFailed is returned when saving relationship fails
	ErrSaveRelationshipFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to save relationship", errors.New("save relationship failed"))

	// ErrSaveAggregateFailed is returned when saving aggregate fails
	ErrSaveAggregateFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to save aggregate", errors.New("save aggregate failed"))

	// ErrQueryDormantRelationshipsFailed is returned when querying dormant relationships fails
	ErrQueryDormantRelationshipsFailed = apperrors.CollectionFetchFailed("dormant relationships", errors.New("query dormant relationships failed"))

	// ErrQueryUserRelationshipsFailed is returned when querying user relationships fails
	ErrQueryUserRelationshipsFailed = apperrors.CollectionFetchFailed("user relationships", errors.New("query user relationships failed"))

	// ErrQueryRelationshipsByStateFailed is returned when querying relationships by state fails
	ErrQueryRelationshipsByStateFailed = apperrors.CollectionFetchFailed("relationships by state", errors.New("query relationships by state failed"))

	// ErrGetRelationshipFailed is returned when getting relationship fails
	ErrGetRelationshipFailed = apperrors.FollowNotFound("", "")

	// ErrSaveStateTransitionFailed is returned when saving state transition fails
	ErrSaveStateTransitionFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to save state transition", errors.New("save state transition failed"))

	// ErrGetAggregateFailed is returned when getting aggregate fails
	ErrGetAggregateFailed = apperrors.CollectionFetchFailed("aggregate", errors.New("get aggregate failed"))

	// ErrGetActiveRelationshipsFailed is returned when getting active relationships fails
	ErrGetActiveRelationshipsFailed = apperrors.CollectionFetchFailed("active relationships", errors.New("get active relationships failed"))

	// ErrCheckArchivedRelationshipFailed is returned when checking for archived relationship fails
	ErrCheckArchivedRelationshipFailed = apperrors.FollowNotFound("", "")

	// ErrSaveRestoredRelationshipFailed is returned when saving restored relationship fails
	ErrSaveRestoredRelationshipFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to save restored relationship", errors.New("save restored relationship failed"))

	// ErrSaveReactivatedRelationshipFailed is returned when saving reactivated relationship fails
	ErrSaveReactivatedRelationshipFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to save reactivated relationship", errors.New("save reactivated relationship failed"))

	// Archive operation errors
	// ErrMarshalArchiveDataFailed is returned when marshaling archive data fails
	ErrMarshalArchiveDataFailed = apperrors.ActivityParsingFailed("archive data", errors.New("marshal archive data failed"))

	// ErrCompressDataFailed is returned when compressing data fails
	ErrCompressDataFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to compress data", errors.New("compress data failed"))

	// ErrCloseGzipWriterFailed is returned when closing gzip writer fails
	ErrCloseGzipWriterFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to close gzip writer", errors.New("close gzip writer failed"))

	// ErrArchiveToS3Failed is returned when archiving to S3 fails after retries
	ErrArchiveToS3Failed = apperrors.NewFederationInternalError(apperrors.CodeExternalServiceUnavailable, "failed to archive to S3 after retries", errors.New("archive to s3 failed"))

	// ErrMarshalBatchArchiveDataFailed is returned when marshaling batch archive data fails
	ErrMarshalBatchArchiveDataFailed = apperrors.ActivityParsingFailed("batch archive data", errors.New("marshal batch archive data failed"))

	// ErrCompressBatchDataFailed is returned when compressing batch data fails
	ErrCompressBatchDataFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to compress batch data", errors.New("compress batch data failed"))

	// ErrUploadBatchArchiveToS3Failed is returned when uploading batch archive to S3 fails
	ErrUploadBatchArchiveToS3Failed = apperrors.NewFederationInternalError(apperrors.CodeExternalServiceUnavailable, "failed to upload batch archive to S3", errors.New("upload batch archive to s3 failed"))

	// ErrCreateGzipReaderFailed is returned when creating gzip reader fails
	ErrCreateGzipReaderFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to create gzip reader", errors.New("create gzip reader failed"))

	// ErrReadCompressedDataFailed is returned when reading compressed data fails
	ErrReadCompressedDataFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to read compressed data", errors.New("read compressed data failed"))

	// ErrUnmarshalArchiveDataFailed is returned when unmarshaling archive data fails
	ErrUnmarshalArchiveDataFailed = apperrors.ActivityParsingFailed("archive data", errors.New("unmarshal archive data failed"))

	// ErrRestoreFromS3Failed is returned when restoring from S3 fails after retries
	ErrRestoreFromS3Failed = apperrors.NewFederationInternalError(apperrors.CodeExternalServiceUnavailable, "failed to restore from S3 after retries", errors.New("restore from s3 failed"))

	// ErrDownloadArchiveFromS3Failed is returned when downloading archive from S3 fails
	ErrDownloadArchiveFromS3Failed = apperrors.NewFederationInternalError(apperrors.CodeExternalServiceUnavailable, "failed to download archive from S3", errors.New("download archive from s3 failed"))

	// ErrDeleteS3ArchiveFailed is returned when deleting S3 archive fails
	ErrDeleteS3ArchiveFailed = apperrors.NewFederationInternalError(apperrors.CodeExternalServiceUnavailable, "failed to delete S3 archive", errors.New("delete s3 archive failed"))

	// ErrBatchWriteRestoredRelationshipsFailed is returned when batch writing restored relationships fails
	ErrBatchWriteRestoredRelationshipsFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to batch write restored relationships", errors.New("batch write restored relationships failed"))
)

// Relay errors - now using centralized error system
var (
	// ErrInvalidRelayURL is returned when relay URL is invalid
	ErrInvalidRelayURL = apperrors.ActorURIInvalid("")

	// ErrFetchRelayActorFailed is returned when relay actor fetching fails
	ErrFetchRelayActorFailed = apperrors.ActorFetchFailed("", errors.New("fetch relay actor failed"))

	// ErrGetActorFailed is returned when actor retrieval fails
	ErrGetActorFailed = apperrors.ActorNotFound("")

	// ErrStoreRelayInfoFailed is returned when storing relay info fails
	ErrStoreRelayInfoFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to store relay info", errors.New("store relay info failed"))

	// ErrDeliverFollowActivityFailed is returned when delivering follow activity fails
	ErrDeliverFollowActivityFailed = apperrors.DeliveryFailed("follow", errors.New("deliver follow activity failed"))

	// ErrRelayNotFound is returned when relay is not found
	ErrRelayNotFound = apperrors.ActorNotFound("")

	// ErrRemoveRelayInfoFailed is returned when removing relay info fails
	ErrRemoveRelayInfoFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to remove relay info", errors.New("remove relay info failed"))

	// ErrUnknownInactiveRelay is returned when activity is from unknown or inactive relay
	ErrUnknownInactiveRelay = apperrors.ActorNotFound("unknown or inactive relay")

	// ErrRelayForwardingFailed is returned when forwarding to multiple relays fails
	ErrRelayForwardingFailed = apperrors.DeliveryToInboxesFailed(0, errors.New("relay forwarding failed"))

	// ErrFetchRelayActorHTTPFailed is returned when relay actor fetch fails with non-OK status
	ErrFetchRelayActorHTTPFailed = apperrors.ActorFetchFailed("", errors.New("fetch relay actor http failed"))

	// ErrNotRelayActor is returned when fetched actor is not a relay type
	ErrNotRelayActor = apperrors.ObjectInvalidField("type", "not a relay actor")

	// ErrMarshalAnnouncedObjectFailed is returned when marshaling announced object fails
	ErrMarshalAnnouncedObjectFailed = apperrors.ActivityParsingFailed("announced object", errors.New("marshal announced object failed"))

	// ErrUnmarshalAnnouncedActivityFailed is returned when unmarshaling announced activity fails
	ErrUnmarshalAnnouncedActivityFailed = apperrors.ActivityParsingFailed("announced activity", errors.New("unmarshal announced activity failed"))

	// ErrInvalidAnnouncedObjectType is returned when announced object type is invalid
	ErrInvalidAnnouncedObjectType = apperrors.ObjectInvalidField("type", "invalid announced object type")

	// ErrRelayBudgetExceeded is returned when relay operation would exceed budget
	ErrRelayBudgetExceeded = apperrors.NewFederationError(apperrors.CodeRateLimited, "relay operation would exceed daily budget")

	// ErrRelayBudgetCreationFailed is returned when relay budget creation fails
	ErrRelayBudgetCreationFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to create relay budget", errors.New("relay budget creation failed"))

	// ErrRelayBudgetAlreadyExceeded is returned when relay budget is already exceeded
	ErrRelayBudgetAlreadyExceeded = apperrors.NewFederationError(apperrors.CodeRateLimited, "relay budget already exceeded")

	// ErrRelayOperationsPaused is returned when relay operations are paused due to budget limits
	ErrRelayOperationsPaused = apperrors.NewFederationError(apperrors.CodeRateLimited, "relay operations paused due to budget limit")

	// ErrRelayCostSummaryFailed is returned when getting relay cost summary fails
	ErrRelayCostSummaryFailed = apperrors.MetricsCollectionFailed("relay cost summary", errors.New("relay cost summary failed"))
)

// Remote search errors - now using centralized error system
var (
	// ErrWebFingerLookupFailed is returned when webfinger lookup fails
	ErrWebFingerLookupFailed = apperrors.WebFingerFailed("", errors.New("webfinger lookup failed"))

	// ErrWebFingerRequestFailed is returned when webfinger request fails
	ErrWebFingerRequestFailed = apperrors.WebFingerFailed("", errors.New("webfinger request failed"))

	// ErrWebFingerNon2xxStatus is returned when webfinger returns non-2xx status
	ErrWebFingerNon2xxStatus = apperrors.WebFingerFailed("", errors.New("webfinger non 2xx status"))

	// ErrWebFingerResponseParseFailed is returned when webfinger response parsing fails
	ErrWebFingerResponseParseFailed = apperrors.WebFingerFailed("", errors.New("webfinger response parse failed"))

	// ErrNoActivityPubLinkFound is returned when no ActivityPub link found in webfinger response
	ErrNoActivityPubLinkFound = apperrors.WebFingerNotFound("")

	// ErrFetchRemoteActorFailed is already defined above as general actor fetching error

	// ErrRemoteActorNon2xxStatus is returned when remote actor fetch returns non-2xx status
	ErrRemoteActorNon2xxStatus = apperrors.ActorFetchFailed("", errors.New("remote actor non 2xx status"))

	// ErrRemoteActorDecodeFailed is returned when remote actor decoding fails
	ErrRemoteActorDecodeFailed = apperrors.ActorFetchFailed("", errors.New("remote actor decode failed"))

	// ErrInvalidActorMissingFields is returned when actor is missing required fields
	ErrInvalidActorMissingFields = apperrors.ObjectMissingField("required fields")

	// ErrActorDomainMismatch is returned when a remote actor's declared ID
	// belongs to a different domain than the URL used to fetch it.
	ErrActorDomainMismatch = apperrors.ActorFetchFailed("", errors.New("actor domain mismatch — possible spoofing"))

	// ErrInvalidUsernameFormat is returned when username format is invalid
	ErrInvalidUsernameFormat = apperrors.UsernameExtractionFailed("format validation", errors.New("invalid username format"))

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = apperrors.ActorURIInvalid("invalid domain format")

	// ErrInvalidHandleFormat is returned when handle format is invalid
	ErrInvalidHandleFormat = apperrors.ActorURIInvalid("invalid handle format")

	// ErrGetKnownInstancesFailed is returned when getting known instances fails
	ErrGetKnownInstancesFailed = apperrors.CollectionFetchFailed("known instances", errors.New("get known instances failed"))

	// ErrCreateSearchRequestFailed is returned when creating search request fails
	ErrCreateSearchRequestFailed = apperrors.RemoteFetchFailed("", errors.New("create search request failed"))

	// ErrSearchRequestFailed is returned when search request fails
	ErrSearchRequestFailed = apperrors.RemoteFetchFailed("", errors.New("search request failed"))

	// ErrSearchNon2xxStatus is returned when search returns non-2xx status
	ErrSearchNon2xxStatus = apperrors.RemoteFetchFailed("", errors.New("search non 2xx status"))

	// ErrSearchResponseDecodeFailed is returned when search response decoding fails
	ErrSearchResponseDecodeFailed = apperrors.RemoteFetchFailed("", errors.New("search response decode failed"))
)

// Compression pipeline errors - now using centralized error system
var (
	// ErrCompressionFailed is returned when data compression fails
	ErrCompressionFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "compression failed", errors.New("compression failed"))

	// ErrDecompressionFailed is returned when data decompression fails
	ErrDecompressionFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "decompression failed", errors.New("decompression failed"))

	// ErrCompressionAlgorithmUnsupported is returned when compression algorithm is not supported
	ErrCompressionAlgorithmUnsupported = apperrors.NewFederationError(apperrors.CodeValidationFailed, "compression algorithm unsupported")

	// ErrCompressionRatioInvalid is returned when compression ratio is invalid
	ErrCompressionRatioInvalid = apperrors.NewFederationError(apperrors.CodeValidationFailed, "compression ratio invalid")

	// ErrPayloadCompressionFailed is returned when payload compression fails
	ErrPayloadCompressionFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "payload compression failed", errors.New("payload compression failed"))

	// ErrMetricMarshalFailed is returned when metric marshaling for compression fails
	ErrMetricMarshalFailed = apperrors.ActivityParsingFailed("metric", errors.New("metric marshal failed"))

	// ErrGzipWriteFailed is returned when writing to gzip writer fails
	ErrGzipWriteFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to write gzip data", errors.New("gzip write failed"))

	// ErrGzipCloseFailed is returned when closing gzip writer fails
	ErrGzipCloseFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to close gzip writer", errors.New("gzip close failed"))

	// ErrOldMetricsRetrievalFailed is returned when retrieving old metrics fails
	ErrOldMetricsRetrievalFailed = apperrors.MetricsCollectionFailed("old metrics retrieval", errors.New("old metrics retrieval failed"))

	// ErrDataArchivalFailed is returned when data archival fails
	ErrDataArchivalFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "data archival failed", errors.New("data archival failed"))
)

// Analytics aggregator errors - now using centralized error system
var (
	// ErrFederationMetricStoreFailed is returned when storing federation metric fails
	ErrFederationMetricStoreFailed = apperrors.MetricsCollectionFailed("federation metric store", errors.New("federation metric store failed"))

	// ErrHealthScoreRetrieveFailed is returned when retrieving health score fails
	ErrHealthScoreRetrieveFailed = apperrors.HealthCheckFailed("", errors.New("health score retrieve failed"))

	// ErrRecentMetricsRetrieveFailed is returned when retrieving recent metrics fails
	ErrRecentMetricsRetrieveFailed = apperrors.MetricsCollectionFailed("recent metrics", errors.New("recent metrics retrieval failed"))

	// ErrUnhealthyDomainsRetrieveFailed is returned when retrieving unhealthy domains fails
	ErrUnhealthyDomainsRetrieveFailed = apperrors.CollectionFetchFailed("unhealthy domains", errors.New("unhealthy domains retrieval failed"))
)

// Remote actor caching errors - now using centralized error system
var (
	// ErrRemoteActorCacheRetrieveFailed is returned when retrieving cached remote actor fails
	ErrRemoteActorCacheRetrieveFailed = apperrors.ActorNotFound("cached remote actor")

	// ErrRemoteActorCacheUpdateFailed is returned when updating cached remote actor fails
	ErrRemoteActorCacheUpdateFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to update cached remote actor", errors.New("remote actor cache update failed"))

	// ErrRemoteActorCacheStoreFailed is returned when storing cached remote actor fails
	ErrRemoteActorCacheStoreFailed = apperrors.NewFederationInternalError(apperrors.CodeInternal, "failed to cache remote actor", errors.New("remote actor cache store failed"))
)
