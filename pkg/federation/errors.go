package federation

import "github.com/equaltoai/lesser/pkg/errors"

// Federation delivery errors - now using centralized error system
var (
	// ErrDeliveryFailed is returned when federation delivery fails
	ErrDeliveryFailed = errors.DeliveryFailed("", nil)

	// ErrMessageParseFailed is returned when federation message parsing fails
	ErrMessageParseFailed = errors.ActivityParsingFailed("message", nil)

	// ErrSigningActorNotFound is returned when signing actor cannot be retrieved
	ErrSigningActorNotFound = errors.ActorNotFound("")

	// ErrMessageMarshalFailed is returned when message marshaling fails
	ErrMessageMarshalFailed = errors.ActivityParsingFailed("message", nil)

	// ErrMessageRequeueFailed is returned when message requeuing fails
	ErrMessageRequeueFailed = errors.DeliveryFailed("", nil)

	// ErrDeliveryPermanentFailure is returned when delivery permanently failed
	ErrDeliveryPermanentFailure = errors.DeliveryPermanentFailure("", "")

	// ErrInstanceStatsNotFound is returned when instance stats cannot be found
	ErrInstanceStatsNotFound = errors.InstanceNotFound("")

	// ErrDeliveryHTTPStatusFailed is returned when delivery fails with non-2xx status
	ErrDeliveryHTTPStatusFailed = errors.DeliveryRejected("", 0)

	// ErrDeliveryToInboxesFailed is returned when delivery to multiple inboxes fails
	ErrDeliveryToInboxesFailed = errors.DeliveryToInboxesFailed(0, nil)

	// ErrDeliveryToDomainsFailed is returned when delivery to multiple domains fails
	ErrDeliveryToDomainsFailed = errors.DeliveryToDomainsFailed(0, nil)

	// ErrDeliveryToRecipientsFailed is returned when delivery to multiple recipients fails
	ErrDeliveryToRecipientsFailed = errors.DeliveryToInboxesFailed(0, nil)

	// ErrNoSharedInboxFound is returned when no shared inbox found for domain
	ErrNoSharedInboxFound = errors.NoSharedInboxFound("")

	// ErrFetchActorHTTPStatusFailed is returned when actor fetch fails with non-2xx status
	ErrFetchActorHTTPStatusFailed = errors.ActorFetchFailed("", nil)

	// ErrDeliveryDirectMessageToInboxesFailed is returned when direct message delivery to multiple inboxes fails
	ErrDeliveryDirectMessageToInboxesFailed = errors.DeliveryToInboxesFailed(0, nil)

	// ErrActivityMarshalFailed is returned when activity marshaling fails
	ErrActivityMarshalFailed = errors.ActivityParsingFailed("", nil)

	// ErrRequestCreationFailed is returned when HTTP request creation fails
	ErrRequestCreationFailed = errors.RemoteFetchFailed("", nil)

	// ErrPrivateKeyRetrievalFailed is returned when private key retrieval fails
	ErrPrivateKeyRetrievalFailed = errors.SigningKeyNotFound("")

	// ErrPrivateKeyParseFailed is returned when private key parsing fails
	ErrPrivateKeyParseFailed = errors.SigningKeyInvalid("")

	// ErrRequestSigningFailed is returned when HTTP request signing fails
	ErrRequestSigningFailed = errors.HTTPSignatureVerificationFailed("")

	// ErrHTTPRequestFailed is returned when HTTP request execution fails
	ErrHTTPRequestFailed = errors.RemoteFetchFailed("", nil)

	// ErrGetFollowersFailed is returned when retrieving followers fails
	ErrGetFollowersFailed = errors.CollectionFetchFailed("", nil)

	// ErrActorDecodeFailed is returned when actor decoding fails
	ErrActorDecodeFailed = errors.ActorFetchFailed("", nil)

	// ErrFetchRemoteActorFailed is returned when remote actor fetching fails
	ErrFetchRemoteActorFailed = errors.ActorFetchFailed("", nil)

	// Enhanced retry specific errors
	// ErrRetryMessageMarshalFailed is returned when retry message marshaling fails
	ErrRetryMessageMarshalFailed = errors.ActivityParsingFailed("retry message", nil)

	// ErrRetryQueueFailed is returned when queuing for enhanced retry fails
	ErrRetryQueueFailed = errors.DeliveryFailed("", nil)

	// ErrEnhancedDeliveryIDGenFailed is returned when enhanced delivery ID generation fails
	ErrEnhancedDeliveryIDGenFailed = errors.DeliveryFailed("", nil)

	// ErrRetryLimitExceeded is returned when maximum retry attempts are exceeded
	ErrRetryLimitExceeded = errors.DeliveryPermanentFailure("", "retry limit exceeded")

	// ErrRetryDeliveryFailed is returned when retry delivery fails
	ErrRetryDeliveryFailed = errors.DeliveryFailed("", nil)

	// ErrBackoffCalculationFailed is returned when backoff calculation fails
	ErrBackoffCalculationFailed = errors.DeliveryFailed("", nil)
)

// HTTP Signature errors - now using centralized error system
var (
	// ErrBuildSignatureString is returned when signature string building fails
	ErrBuildSignatureString = errors.SignatureInvalid("failed to build signature string")

	// ErrECDSAVerificationFailed is returned when ECDSA signature verification fails
	ErrECDSAVerificationFailed = errors.HTTPSignatureVerificationFailed("ECDSA signature verification failed")

	// ErrEd25519VerificationFailed is returned when Ed25519 signature verification fails
	ErrEd25519VerificationFailed = errors.HTTPSignatureVerificationFailed("Ed25519 signature verification failed")

	// ErrUnsupportedPublicKeyType is returned when public key type is not supported for hs2019
	ErrUnsupportedPublicKeyType = errors.SigningKeyInvalid("unsupported public key type for hs2019")

	// ErrAlgorithmRequiresRSA is returned when algorithm requires RSA key but different key type provided
	ErrAlgorithmRequiresRSA = errors.SignatureInvalid("algorithm requires RSA key")

	// ErrAlgorithmRequiresECDSA is returned when algorithm requires ECDSA key but different key type provided
	ErrAlgorithmRequiresECDSA = errors.SignatureInvalid("algorithm requires ECDSA key")

	// ErrAlgorithmRequiresEd25519 is returned when algorithm requires Ed25519 key but different key type provided
	ErrAlgorithmRequiresEd25519 = errors.SignatureInvalid("algorithm requires Ed25519 key")

	// ErrUnsupportedAlgorithm is returned when signature algorithm is not supported
	ErrUnsupportedAlgorithm = errors.SignatureInvalid("unsupported algorithm")

	// ErrSignatureFailed is returned when signing operation fails
	ErrSignatureFailed = errors.HTTPSignatureVerificationFailed("failed to sign")

	// ErrUnsupportedPrivateKeyType is returned when private key type is not supported for hs2019
	ErrUnsupportedPrivateKeyType = errors.SigningKeyInvalid("unsupported private key type for hs2019")

	// ErrInvalidSignatureInputFormat is returned when signature-input format is invalid
	ErrInvalidSignatureInputFormat = errors.SignatureInvalid("invalid signature-input format: missing parentheses")

	// ErrDecodeSignature is returned when signature decoding fails
	ErrDecodeSignature = errors.SignatureInvalid("failed to decode signature")

	// ErrInvalidSignatureHeaderFormat is returned when signature header format is invalid
	ErrInvalidSignatureHeaderFormat = errors.SignatureInvalid("invalid signature header format")

	// ErrMissingKeyID is returned when keyId is missing in signature
	ErrMissingKeyID = errors.SignatureMissing()

	// ErrMissingSignatureValue is returned when signature value is missing
	ErrMissingSignatureValue = errors.SignatureMissing()

	// ErrRequiredHeaderNotFound is returned when a required header is not found
	ErrRequiredHeaderNotFound = errors.SignatureInvalid("required header not found")

	// ErrFailedToParsePEMBlock is returned when PEM block parsing fails
	ErrFailedToParsePEMBlock = errors.SigningKeyInvalid("failed to parse PEM block")

	// ErrUnsupportedKeyType is returned when key type is not supported
	ErrUnsupportedKeyType = errors.SigningKeyInvalid("unsupported key type")

	// ErrKeySizeTooSmall is returned when key size is insufficient
	ErrKeySizeTooSmall = errors.SigningKeyInvalid("key size must be at least 2048 bits")

	// ErrInvalidSignatureHeaderFormatWrapper is returned when signature header format validation fails
	ErrInvalidSignatureHeaderFormatWrapper = errors.SignatureInvalid("invalid signature header format")

	// ErrDecodeSignatureFailed is returned when base64 signature decoding fails
	ErrDecodeSignatureFailed = errors.SignatureInvalid("failed to decode signature")

	// ErrSignatureParseFailed is already defined above as general signature parsing error

	// ErrReadRequestBodyFailed is returned when reading HTTP request body fails
	ErrReadRequestBodyFailed = errors.RemoteFetchFailed("", nil)

	// ErrRSAKeyGenFailed is returned when RSA key generation fails
	ErrRSAKeyGenFailed = errors.SigningKeyInvalid("failed to generate RSA key pair")

	// ErrMarshalPublicKeyFailed is returned when marshaling public key fails
	ErrMarshalPublicKeyFailed = errors.SigningKeyInvalid("failed to marshal public key")

	// ErrMarshalPrivateKeyFailed is returned when marshaling private key fails
	ErrMarshalPrivateKeyFailed = errors.SigningKeyInvalid("failed to marshal private key")
)

// Inbox recovery errors - now using centralized error system
var (
	// ErrMissingRequestIDInConfirmation is returned when request ID is missing in recovery confirmation
	ErrMissingRequestIDInConfirmation = errors.InboxMessageInvalid("missing request ID in recovery confirmation")

	// ErrMissingActorInConfirmation is returned when actor is missing in recovery confirmation
	ErrMissingActorInConfirmation = errors.InboxMessageInvalid("missing actor in recovery confirmation")

	// ErrMissingInviterUsername is returned when inviter username is missing in trustee acceptance
	ErrMissingInviterUsername = errors.InboxMessageInvalid("missing inviter username in trustee acceptance")

	// ErrMissingActorInAcceptance is returned when actor is missing in trustee acceptance
	ErrMissingActorInAcceptance = errors.InboxMessageInvalid("missing actor in trustee acceptance")
)

// Trend analysis errors - now using centralized error system
var (
	// ErrGetConnectionsFailed is returned when getting instance connections fails during trend analysis
	ErrGetConnectionsFailed = errors.RemoteFetchFailed("", nil)
)

// Authorized fetch errors - now using centralized error system
var (
	// ErrFetchObjectHTTPFailed is returned when HTTP request fails during object fetch
	ErrFetchObjectHTTPFailed = errors.RemoteFetchFailed("", nil)

	// ErrInvalidActorObjectType is returned when fetched object is not a valid actor type
	ErrInvalidActorObjectType = errors.ObjectInvalidField("type", "invalid actor object type")

	// ErrNotActorObject is returned when object type is not a valid actor type
	ErrNotActorObject = errors.ObjectInvalidField("type", "not an actor object")

	// ErrMissingSignatureHeader is returned when signature header is missing in authorized fetch request
	ErrMissingSignatureHeader = errors.SignatureMissing()

	// ErrExtractActorIDFailed is returned when actor ID cannot be extracted from keyId
	ErrExtractActorIDFailed = errors.ActorURIExtractionFailed("keyId extraction", nil)

	// ErrObjectIDMismatch is returned when fetched object ID doesn't match requested ID
	ErrObjectIDMismatch = errors.ObjectInvalidField("id", "object ID mismatch")

	// ErrObjectMissingType is returned when object is missing type field
	ErrObjectMissingType = errors.ObjectMissingField("type")

	// ErrFetchActorHTTPFailed is returned when HTTP request fails during actor fetch
	ErrFetchActorHTTPFailed = errors.ActorFetchFailed("", nil)

	// ErrRepositoryAccessValidationFailed is returned when repository access validation fails
	ErrRepositoryAccessValidationFailed = errors.RemoteFetchUnauthorized("")

	// ErrActorDataMarshalFailed is returned when actor data marshaling fails
	ErrActorDataMarshalFailed = errors.ActivityParsingFailed("actor", nil)

	// ErrActorUnmarshalFailed is returned when actor unmarshaling fails
	ErrActorUnmarshalFailed = errors.ActivityParsingFailed("actor", nil)

	// ErrSignatureParseFailed is returned when signature parsing fails
	ErrSignatureParseFailed = errors.SignatureInvalid("failed to parse signature")

	// ErrPublicKeyParseFailed is returned when public key parsing fails
	ErrPublicKeyParseFailed = errors.SigningKeyInvalid("failed to parse public key")

	// ErrSignatureVerificationFailed is returned when signature verification fails
	ErrSignatureVerificationFailed = errors.HTTPSignatureVerificationFailed("signature verification failed")

	// ErrResponseDecodeFailed is returned when response decoding fails
	ErrResponseDecodeFailed = errors.RemoteFetchFailed("", nil)

	// ErrObjectValidationFailed is returned when object validation fails
	ErrObjectValidationFailed = errors.ObjectParsingFailed("", nil)

	// ErrInvalidCachedPublicKey is returned when cached public key is invalid
	ErrInvalidCachedPublicKey = errors.SigningKeyInvalid("invalid cached public key")

	// ErrPublicKeyFetchFailed is returned when public key fetch fails after retries
	ErrPublicKeyFetchFailed = errors.SigningKeyNotFound("failed to fetch public key after retries")

	// ErrActorHasNoPublicKey is returned when actor has no public key
	ErrActorHasNoPublicKey = errors.SigningKeyNotFound("actor has no public key")

	// ErrPublicKeyExtractionFailed is returned when public key extraction fails
	ErrPublicKeyExtractionFailed = errors.SigningKeyInvalid("failed to extract public key")
)

// Relationship tracker errors - now using centralized error system
var (
	// ErrS3ClientNotConfigured is returned when S3 client is not configured for restore operation
	ErrS3ClientNotConfigured = errors.NewFederationError(errors.CodeDependencyNotMet, "S3 client not configured for restore operation")

	// ErrArchiveContainsNoRelationships is returned when archive contains no relationships
	ErrArchiveContainsNoRelationships = errors.CollectionInvalid("", "archive contains no relationships")

	// Database operation errors
	// ErrGetFederationEdgesFailed is returned when getting federation edges fails
	ErrGetFederationEdgesFailed = errors.CollectionFetchFailed("federation edges", nil)

	// ErrGetConnectionsFailed is already defined above in trend analysis errors

	// ErrGetCreateRelationshipFailed is returned when getting or creating relationship fails
	ErrGetCreateRelationshipFailed = errors.FollowNotFound("", "")

	// ErrGetCreateAggregateFailed is returned when getting or creating aggregate fails
	ErrGetCreateAggregateFailed = errors.CollectionFetchFailed("aggregate", nil)

	// ErrQueryRelationshipFailed is returned when querying relationship fails
	ErrQueryRelationshipFailed = errors.FollowNotFound("", "")

	// ErrQueryAggregateFailed is returned when querying aggregate fails
	ErrQueryAggregateFailed = errors.CollectionFetchFailed("aggregate", nil)

	// ErrSaveRelationshipFailed is returned when saving relationship fails
	ErrSaveRelationshipFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to save relationship", nil)

	// ErrSaveAggregateFailed is returned when saving aggregate fails
	ErrSaveAggregateFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to save aggregate", nil)

	// ErrQueryDormantRelationshipsFailed is returned when querying dormant relationships fails
	ErrQueryDormantRelationshipsFailed = errors.CollectionFetchFailed("dormant relationships", nil)

	// ErrQueryUserRelationshipsFailed is returned when querying user relationships fails
	ErrQueryUserRelationshipsFailed = errors.CollectionFetchFailed("user relationships", nil)

	// ErrQueryRelationshipsByStateFailed is returned when querying relationships by state fails
	ErrQueryRelationshipsByStateFailed = errors.CollectionFetchFailed("relationships by state", nil)

	// ErrGetRelationshipFailed is returned when getting relationship fails
	ErrGetRelationshipFailed = errors.FollowNotFound("", "")

	// ErrSaveStateTransitionFailed is returned when saving state transition fails
	ErrSaveStateTransitionFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to save state transition", nil)

	// ErrGetAggregateFailed is returned when getting aggregate fails
	ErrGetAggregateFailed = errors.CollectionFetchFailed("aggregate", nil)

	// ErrGetActiveRelationshipsFailed is returned when getting active relationships fails
	ErrGetActiveRelationshipsFailed = errors.CollectionFetchFailed("active relationships", nil)

	// ErrCheckArchivedRelationshipFailed is returned when checking for archived relationship fails
	ErrCheckArchivedRelationshipFailed = errors.FollowNotFound("", "")

	// ErrSaveRestoredRelationshipFailed is returned when saving restored relationship fails
	ErrSaveRestoredRelationshipFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to save restored relationship", nil)

	// ErrSaveReactivatedRelationshipFailed is returned when saving reactivated relationship fails
	ErrSaveReactivatedRelationshipFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to save reactivated relationship", nil)

	// Archive operation errors
	// ErrMarshalArchiveDataFailed is returned when marshaling archive data fails
	ErrMarshalArchiveDataFailed = errors.ActivityParsingFailed("archive data", nil)

	// ErrCompressDataFailed is returned when compressing data fails
	ErrCompressDataFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to compress data", nil)

	// ErrCloseGzipWriterFailed is returned when closing gzip writer fails
	ErrCloseGzipWriterFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to close gzip writer", nil)

	// ErrArchiveToS3Failed is returned when archiving to S3 fails after retries
	ErrArchiveToS3Failed = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "failed to archive to S3 after retries", nil)

	// ErrMarshalBatchArchiveDataFailed is returned when marshaling batch archive data fails
	ErrMarshalBatchArchiveDataFailed = errors.ActivityParsingFailed("batch archive data", nil)

	// ErrCompressBatchDataFailed is returned when compressing batch data fails
	ErrCompressBatchDataFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to compress batch data", nil)

	// ErrUploadBatchArchiveToS3Failed is returned when uploading batch archive to S3 fails
	ErrUploadBatchArchiveToS3Failed = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "failed to upload batch archive to S3", nil)

	// ErrCreateGzipReaderFailed is returned when creating gzip reader fails
	ErrCreateGzipReaderFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to create gzip reader", nil)

	// ErrReadCompressedDataFailed is returned when reading compressed data fails
	ErrReadCompressedDataFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to read compressed data", nil)

	// ErrUnmarshalArchiveDataFailed is returned when unmarshaling archive data fails
	ErrUnmarshalArchiveDataFailed = errors.ActivityParsingFailed("archive data", nil)

	// ErrRestoreFromS3Failed is returned when restoring from S3 fails after retries
	ErrRestoreFromS3Failed = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "failed to restore from S3 after retries", nil)

	// ErrDownloadArchiveFromS3Failed is returned when downloading archive from S3 fails
	ErrDownloadArchiveFromS3Failed = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "failed to download archive from S3", nil)

	// ErrDeleteS3ArchiveFailed is returned when deleting S3 archive fails
	ErrDeleteS3ArchiveFailed = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "failed to delete S3 archive", nil)

	// ErrBatchWriteRestoredRelationshipsFailed is returned when batch writing restored relationships fails
	ErrBatchWriteRestoredRelationshipsFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to batch write restored relationships", nil)
)

// Relay errors - now using centralized error system
var (
	// ErrInvalidRelayURL is returned when relay URL is invalid
	ErrInvalidRelayURL = errors.ActorURIInvalid("")

	// ErrFetchRelayActorFailed is returned when relay actor fetching fails
	ErrFetchRelayActorFailed = errors.ActorFetchFailed("", nil)

	// ErrGetActorFailed is returned when actor retrieval fails
	ErrGetActorFailed = errors.ActorNotFound("")

	// ErrStoreRelayInfoFailed is returned when storing relay info fails
	ErrStoreRelayInfoFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to store relay info", nil)

	// ErrDeliverFollowActivityFailed is returned when delivering follow activity fails
	ErrDeliverFollowActivityFailed = errors.DeliveryFailed("", nil)

	// ErrRelayNotFound is returned when relay is not found
	ErrRelayNotFound = errors.ActorNotFound("")

	// ErrRemoveRelayInfoFailed is returned when removing relay info fails
	ErrRemoveRelayInfoFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to remove relay info", nil)

	// ErrUnknownInactiveRelay is returned when activity is from unknown or inactive relay
	ErrUnknownInactiveRelay = errors.ActorNotFound("unknown or inactive relay")

	// ErrRelayForwardingFailed is returned when forwarding to multiple relays fails
	ErrRelayForwardingFailed = errors.DeliveryToInboxesFailed(0, nil)

	// ErrFetchRelayActorHTTPFailed is returned when relay actor fetch fails with non-OK status
	ErrFetchRelayActorHTTPFailed = errors.ActorFetchFailed("", nil)

	// ErrNotRelayActor is returned when fetched actor is not a relay type
	ErrNotRelayActor = errors.ObjectInvalidField("type", "not a relay actor")

	// ErrMarshalAnnouncedObjectFailed is returned when marshaling announced object fails
	ErrMarshalAnnouncedObjectFailed = errors.ActivityParsingFailed("announced object", nil)

	// ErrUnmarshalAnnouncedActivityFailed is returned when unmarshaling announced activity fails
	ErrUnmarshalAnnouncedActivityFailed = errors.ActivityParsingFailed("announced activity", nil)

	// ErrInvalidAnnouncedObjectType is returned when announced object type is invalid
	ErrInvalidAnnouncedObjectType = errors.ObjectInvalidField("type", "invalid announced object type")

	// ErrRelayBudgetExceeded is returned when relay operation would exceed budget
	ErrRelayBudgetExceeded = errors.NewFederationError(errors.CodeRateLimited, "relay operation would exceed daily budget")

	// ErrRelayBudgetCreationFailed is returned when relay budget creation fails
	ErrRelayBudgetCreationFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to create relay budget", nil)

	// ErrRelayBudgetAlreadyExceeded is returned when relay budget is already exceeded
	ErrRelayBudgetAlreadyExceeded = errors.NewFederationError(errors.CodeRateLimited, "relay budget already exceeded")

	// ErrRelayOperationsPaused is returned when relay operations are paused due to budget limits
	ErrRelayOperationsPaused = errors.NewFederationError(errors.CodeRateLimited, "relay operations paused due to budget limit")

	// ErrRelayCostSummaryFailed is returned when getting relay cost summary fails
	ErrRelayCostSummaryFailed = errors.MetricsCollectionFailed("relay cost summary", nil)
)

// Remote search errors - now using centralized error system
var (
	// ErrWebFingerLookupFailed is returned when webfinger lookup fails
	ErrWebFingerLookupFailed = errors.WebFingerFailed("", nil)

	// ErrWebFingerRequestFailed is returned when webfinger request fails
	ErrWebFingerRequestFailed = errors.WebFingerFailed("", nil)

	// ErrWebFingerNon2xxStatus is returned when webfinger returns non-2xx status
	ErrWebFingerNon2xxStatus = errors.WebFingerFailed("", nil)

	// ErrWebFingerResponseParseFailed is returned when webfinger response parsing fails
	ErrWebFingerResponseParseFailed = errors.WebFingerFailed("", nil)

	// ErrNoActivityPubLinkFound is returned when no ActivityPub link found in webfinger response
	ErrNoActivityPubLinkFound = errors.WebFingerNotFound("")

	// ErrFetchRemoteActorFailed is already defined above as general actor fetching error

	// ErrRemoteActorNon2xxStatus is returned when remote actor fetch returns non-2xx status
	ErrRemoteActorNon2xxStatus = errors.ActorFetchFailed("", nil)

	// ErrRemoteActorDecodeFailed is returned when remote actor decoding fails
	ErrRemoteActorDecodeFailed = errors.ActorFetchFailed("", nil)

	// ErrInvalidActorMissingFields is returned when actor is missing required fields
	ErrInvalidActorMissingFields = errors.ObjectMissingField("required fields")

	// ErrInvalidUsernameFormat is returned when username format is invalid
	ErrInvalidUsernameFormat = errors.UsernameExtractionFailed("format validation", nil)

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = errors.ActorURIInvalid("invalid domain format")

	// ErrInvalidHandleFormat is returned when handle format is invalid
	ErrInvalidHandleFormat = errors.ActorURIInvalid("invalid handle format")

	// ErrGetKnownInstancesFailed is returned when getting known instances fails
	ErrGetKnownInstancesFailed = errors.CollectionFetchFailed("known instances", nil)

	// ErrCreateSearchRequestFailed is returned when creating search request fails
	ErrCreateSearchRequestFailed = errors.RemoteFetchFailed("", nil)

	// ErrSearchRequestFailed is returned when search request fails
	ErrSearchRequestFailed = errors.RemoteFetchFailed("", nil)

	// ErrSearchNon2xxStatus is returned when search returns non-2xx status
	ErrSearchNon2xxStatus = errors.RemoteFetchFailed("", nil)

	// ErrSearchResponseDecodeFailed is returned when search response decoding fails
	ErrSearchResponseDecodeFailed = errors.RemoteFetchFailed("", nil)
)

// Compression pipeline errors - now using centralized error system
var (
	// ErrCompressionFailed is returned when data compression fails
	ErrCompressionFailed = errors.NewFederationInternalError(errors.CodeInternal, "compression failed", nil)

	// ErrDecompressionFailed is returned when data decompression fails
	ErrDecompressionFailed = errors.NewFederationInternalError(errors.CodeInternal, "decompression failed", nil)

	// ErrCompressionAlgorithmUnsupported is returned when compression algorithm is not supported
	ErrCompressionAlgorithmUnsupported = errors.NewFederationError(errors.CodeValidationFailed, "compression algorithm unsupported")

	// ErrCompressionRatioInvalid is returned when compression ratio is invalid
	ErrCompressionRatioInvalid = errors.NewFederationError(errors.CodeValidationFailed, "compression ratio invalid")

	// ErrPayloadCompressionFailed is returned when payload compression fails
	ErrPayloadCompressionFailed = errors.NewFederationInternalError(errors.CodeInternal, "payload compression failed", nil)

	// ErrMetricMarshalFailed is returned when metric marshaling for compression fails
	ErrMetricMarshalFailed = errors.ActivityParsingFailed("metric", nil)

	// ErrGzipWriteFailed is returned when writing to gzip writer fails
	ErrGzipWriteFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to write gzip data", nil)

	// ErrGzipCloseFailed is returned when closing gzip writer fails
	ErrGzipCloseFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to close gzip writer", nil)

	// ErrOldMetricsRetrievalFailed is returned when retrieving old metrics fails
	ErrOldMetricsRetrievalFailed = errors.MetricsCollectionFailed("old metrics retrieval", nil)

	// ErrDataArchivalFailed is returned when data archival fails
	ErrDataArchivalFailed = errors.NewFederationInternalError(errors.CodeInternal, "data archival failed", nil)
)

// Analytics aggregator errors - now using centralized error system
var (
	// ErrFederationMetricStoreFailed is returned when storing federation metric fails
	ErrFederationMetricStoreFailed = errors.MetricsCollectionFailed("federation metric store", nil)

	// ErrHealthScoreRetrieveFailed is returned when retrieving health score fails
	ErrHealthScoreRetrieveFailed = errors.HealthCheckFailed("", nil)

	// ErrRecentMetricsRetrieveFailed is returned when retrieving recent metrics fails
	ErrRecentMetricsRetrieveFailed = errors.MetricsCollectionFailed("recent metrics", nil)

	// ErrUnhealthyDomainsRetrieveFailed is returned when retrieving unhealthy domains fails
	ErrUnhealthyDomainsRetrieveFailed = errors.CollectionFetchFailed("unhealthy domains", nil)
)

// Remote actor caching errors - now using centralized error system
var (
	// ErrRemoteActorCacheRetrieveFailed is returned when retrieving cached remote actor fails
	ErrRemoteActorCacheRetrieveFailed = errors.ActorNotFound("cached remote actor")

	// ErrRemoteActorCacheUpdateFailed is returned when updating cached remote actor fails
	ErrRemoteActorCacheUpdateFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to update cached remote actor", nil)

	// ErrRemoteActorCacheStoreFailed is returned when storing cached remote actor fails
	ErrRemoteActorCacheStoreFailed = errors.NewFederationInternalError(errors.CodeInternal, "failed to cache remote actor", nil)
)
