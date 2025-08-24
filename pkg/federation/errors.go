package federation

import "errors"

// Federation delivery errors
var (
	// ErrDeliveryFailed is returned when federation delivery fails
	ErrDeliveryFailed = errors.New("federation delivery failed")

	// ErrMessageParseFailed is returned when federation message parsing fails
	ErrMessageParseFailed = errors.New("failed to parse federation message")

	// ErrSigningActorNotFound is returned when signing actor cannot be retrieved
	ErrSigningActorNotFound = errors.New("signing actor not found")

	// ErrMessageMarshalFailed is returned when message marshaling fails
	ErrMessageMarshalFailed = errors.New("failed to marshal federation message")

	// ErrMessageRequeueFailed is returned when message requeuing fails
	ErrMessageRequeueFailed = errors.New("failed to requeue federation message")

	// ErrDeliveryPermanentFailure is returned when delivery permanently failed
	ErrDeliveryPermanentFailure = errors.New("delivery permanently failed")

	// ErrInstanceStatsNotFound is returned when instance stats cannot be found
	ErrInstanceStatsNotFound = errors.New("instance stats not found")

	// ErrDeliveryHTTPStatusFailed is returned when delivery fails with non-2xx status
	ErrDeliveryHTTPStatusFailed = errors.New("delivery failed with non-2xx status")

	// ErrDeliveryToInboxesFailed is returned when delivery to multiple inboxes fails
	ErrDeliveryToInboxesFailed = errors.New("failed to deliver to inboxes")

	// ErrDeliveryToDomainsFailed is returned when delivery to multiple domains fails
	ErrDeliveryToDomainsFailed = errors.New("failed to deliver to domains")

	// ErrDeliveryToRecipientsFailed is returned when delivery to multiple recipients fails
	ErrDeliveryToRecipientsFailed = errors.New("failed to deliver to recipients")

	// ErrNoSharedInboxFound is returned when no shared inbox found for domain
	ErrNoSharedInboxFound = errors.New("no shared inbox found for domain")

	// ErrFetchActorHTTPStatusFailed is returned when actor fetch fails with non-2xx status
	ErrFetchActorHTTPStatusFailed = errors.New("failed to fetch actor with non-2xx status")

	// ErrDeliveryDirectMessageToInboxesFailed is returned when direct message delivery to multiple inboxes fails
	ErrDeliveryDirectMessageToInboxesFailed = errors.New("failed to deliver direct message to inboxes")

	// ErrActivityMarshalFailed is returned when activity marshaling fails
	ErrActivityMarshalFailed = errors.New("failed to marshal activity")

	// ErrRequestCreationFailed is returned when HTTP request creation fails
	ErrRequestCreationFailed = errors.New("failed to create request")

	// ErrPrivateKeyRetrievalFailed is returned when private key retrieval fails
	ErrPrivateKeyRetrievalFailed = errors.New("failed to get private key")

	// ErrPrivateKeyParseFailed is returned when private key parsing fails
	ErrPrivateKeyParseFailed = errors.New("failed to parse private key")

	// ErrRequestSigningFailed is returned when HTTP request signing fails
	ErrRequestSigningFailed = errors.New("failed to sign request")

	// ErrHTTPRequestFailed is returned when HTTP request execution fails
	ErrHTTPRequestFailed = errors.New("failed to send request")

	// ErrGetFollowersFailed is returned when retrieving followers fails
	ErrGetFollowersFailed = errors.New("failed to get followers")

	// ErrActorDecodeFailed is returned when actor decoding fails
	ErrActorDecodeFailed = errors.New("failed to decode actor")

	// ErrFetchRemoteActorFailed is returned when remote actor fetching fails
	ErrFetchRemoteActorFailed = errors.New("failed to fetch actor")

	// Enhanced retry specific errors
	// ErrRetryMessageMarshalFailed is returned when retry message marshaling fails
	ErrRetryMessageMarshalFailed = errors.New("failed to marshal enhanced retry message")

	// ErrRetryQueueFailed is returned when queuing for enhanced retry fails
	ErrRetryQueueFailed = errors.New("failed to queue for enhanced retry")

	// ErrEnhancedDeliveryIDGenFailed is returned when enhanced delivery ID generation fails
	ErrEnhancedDeliveryIDGenFailed = errors.New("failed to generate enhanced delivery ID")

	// ErrRetryLimitExceeded is returned when maximum retry attempts are exceeded
	ErrRetryLimitExceeded = errors.New("retry limit exceeded")

	// ErrRetryDeliveryFailed is returned when retry delivery fails
	ErrRetryDeliveryFailed = errors.New("failed to retry delivery")

	// ErrBackoffCalculationFailed is returned when backoff calculation fails
	ErrBackoffCalculationFailed = errors.New("failed to calculate backoff delay")
)

// HTTP Signature errors
var (
	// ErrBuildSignatureString is returned when signature string building fails
	ErrBuildSignatureString = errors.New("failed to build signature string")

	// ErrECDSAVerificationFailed is returned when ECDSA signature verification fails
	ErrECDSAVerificationFailed = errors.New("ECDSA signature verification failed")

	// ErrEd25519VerificationFailed is returned when Ed25519 signature verification fails
	ErrEd25519VerificationFailed = errors.New("Ed25519 signature verification failed")

	// ErrUnsupportedPublicKeyType is returned when public key type is not supported for hs2019
	ErrUnsupportedPublicKeyType = errors.New("unsupported public key type for hs2019")

	// ErrAlgorithmRequiresRSA is returned when algorithm requires RSA key but different key type provided
	ErrAlgorithmRequiresRSA = errors.New("algorithm requires RSA key")

	// ErrAlgorithmRequiresECDSA is returned when algorithm requires ECDSA key but different key type provided
	ErrAlgorithmRequiresECDSA = errors.New("algorithm requires ECDSA key")

	// ErrAlgorithmRequiresEd25519 is returned when algorithm requires Ed25519 key but different key type provided
	ErrAlgorithmRequiresEd25519 = errors.New("algorithm requires Ed25519 key")

	// ErrUnsupportedAlgorithm is returned when signature algorithm is not supported
	ErrUnsupportedAlgorithm = errors.New("unsupported algorithm")

	// ErrSignatureFailed is returned when signing operation fails
	ErrSignatureFailed = errors.New("failed to sign")

	// ErrUnsupportedPrivateKeyType is returned when private key type is not supported for hs2019
	ErrUnsupportedPrivateKeyType = errors.New("unsupported private key type for hs2019")

	// ErrInvalidSignatureInputFormat is returned when signature-input format is invalid
	ErrInvalidSignatureInputFormat = errors.New("invalid signature-input format: missing parentheses")

	// ErrDecodeSignature is returned when signature decoding fails
	ErrDecodeSignature = errors.New("failed to decode signature")

	// ErrInvalidSignatureHeaderFormat is returned when signature header format is invalid
	ErrInvalidSignatureHeaderFormat = errors.New("invalid signature header format")

	// ErrMissingKeyID is returned when keyId is missing in signature
	ErrMissingKeyID = errors.New("missing keyId in signature")

	// ErrMissingSignatureValue is returned when signature value is missing
	ErrMissingSignatureValue = errors.New("missing signature value")

	// ErrRequiredHeaderNotFound is returned when a required header is not found
	ErrRequiredHeaderNotFound = errors.New("required header not found")

	// ErrFailedToParsePEMBlock is returned when PEM block parsing fails
	ErrFailedToParsePEMBlock = errors.New("failed to parse PEM block")

	// ErrUnsupportedKeyType is returned when key type is not supported
	ErrUnsupportedKeyType = errors.New("unsupported key type")

	// ErrKeySizeTooSmall is returned when key size is insufficient
	ErrKeySizeTooSmall = errors.New("key size must be at least 2048 bits")

	// ErrInvalidSignatureHeaderFormatWrapper is returned when signature header format validation fails
	ErrInvalidSignatureHeaderFormatWrapper = errors.New("invalid signature header format")

	// ErrDecodeSignatureFailed is returned when base64 signature decoding fails
	ErrDecodeSignatureFailed = errors.New("failed to decode signature")

	// ErrSignatureParseFailed is already defined above as general signature parsing error

	// ErrReadRequestBodyFailed is returned when reading HTTP request body fails
	ErrReadRequestBodyFailed = errors.New("failed to read request body")

	// ErrRSAKeyGenFailed is returned when RSA key generation fails
	ErrRSAKeyGenFailed = errors.New("failed to generate RSA key pair")

	// ErrMarshalPublicKeyFailed is returned when marshaling public key fails
	ErrMarshalPublicKeyFailed = errors.New("failed to marshal public key")

	// ErrMarshalPrivateKeyFailed is returned when marshaling private key fails
	ErrMarshalPrivateKeyFailed = errors.New("failed to marshal private key")
)

// Inbox recovery errors
var (
	// ErrMissingRequestIDInConfirmation is returned when request ID is missing in recovery confirmation
	ErrMissingRequestIDInConfirmation = errors.New("missing request ID in recovery confirmation")

	// ErrMissingActorInConfirmation is returned when actor is missing in recovery confirmation
	ErrMissingActorInConfirmation = errors.New("missing actor in recovery confirmation")

	// ErrMissingInviterUsername is returned when inviter username is missing in trustee acceptance
	ErrMissingInviterUsername = errors.New("missing inviter username in trustee acceptance")

	// ErrMissingActorInAcceptance is returned when actor is missing in trustee acceptance
	ErrMissingActorInAcceptance = errors.New("missing actor in trustee acceptance")
)

// Trend analysis errors
var (
	// ErrGetConnectionsFailed is returned when getting instance connections fails during trend analysis
	ErrGetConnectionsFailed = errors.New("failed to get connections")
)

// Authorized fetch errors
var (
	// ErrFetchObjectHTTPFailed is returned when HTTP request fails during object fetch
	ErrFetchObjectHTTPFailed = errors.New("failed to fetch object")

	// ErrInvalidActorObjectType is returned when fetched object is not a valid actor type
	ErrInvalidActorObjectType = errors.New("invalid actor object type")

	// ErrNotActorObject is returned when object type is not a valid actor type
	ErrNotActorObject = errors.New("not an actor object")

	// ErrMissingSignatureHeader is returned when signature header is missing in authorized fetch request
	ErrMissingSignatureHeader = errors.New("missing signature header")

	// ErrExtractActorIDFailed is returned when actor ID cannot be extracted from keyId
	ErrExtractActorIDFailed = errors.New("failed to extract actor ID from keyId")

	// ErrObjectIDMismatch is returned when fetched object ID doesn't match requested ID
	ErrObjectIDMismatch = errors.New("object ID mismatch")

	// ErrObjectMissingType is returned when object is missing type field
	ErrObjectMissingType = errors.New("object missing type field")

	// ErrFetchActorHTTPFailed is returned when HTTP request fails during actor fetch
	ErrFetchActorHTTPFailed = errors.New("failed to fetch actor")

	// ErrRepositoryAccessValidationFailed is returned when repository access validation fails
	ErrRepositoryAccessValidationFailed = errors.New("repository access validation failed")

	// ErrActorDataMarshalFailed is returned when actor data marshaling fails
	ErrActorDataMarshalFailed = errors.New("failed to marshal actor data")

	// ErrActorUnmarshalFailed is returned when actor unmarshaling fails
	ErrActorUnmarshalFailed = errors.New("failed to unmarshal actor")

	// ErrSignatureParseFailed is returned when signature parsing fails
	ErrSignatureParseFailed = errors.New("failed to parse signature")

	// ErrPublicKeyParseFailed is returned when public key parsing fails
	ErrPublicKeyParseFailed = errors.New("failed to parse public key")

	// ErrSignatureVerificationFailed is returned when signature verification fails
	ErrSignatureVerificationFailed = errors.New("signature verification failed")

	// ErrResponseDecodeFailed is returned when response decoding fails
	ErrResponseDecodeFailed = errors.New("failed to decode response")

	// ErrObjectValidationFailed is returned when object validation fails
	ErrObjectValidationFailed = errors.New("object validation failed")

	// ErrInvalidCachedPublicKey is returned when cached public key is invalid
	ErrInvalidCachedPublicKey = errors.New("invalid cached public key")

	// ErrPublicKeyFetchFailed is returned when public key fetch fails after retries
	ErrPublicKeyFetchFailed = errors.New("failed to fetch public key after retries")

	// ErrActorHasNoPublicKey is returned when actor has no public key
	ErrActorHasNoPublicKey = errors.New("actor has no public key")

	// ErrPublicKeyExtractionFailed is returned when public key extraction fails
	ErrPublicKeyExtractionFailed = errors.New("failed to extract public key")
)

// Relationship tracker errors
var (
	// ErrS3ClientNotConfigured is returned when S3 client is not configured for restore operation
	ErrS3ClientNotConfigured = errors.New("S3 client not configured for restore operation")

	// ErrArchiveContainsNoRelationships is returned when archive contains no relationships
	ErrArchiveContainsNoRelationships = errors.New("archive contains no relationships")

	// Database operation errors
	// ErrGetFederationEdgesFailed is returned when getting federation edges fails
	ErrGetFederationEdgesFailed = errors.New("failed to get federation edges")

	// ErrGetConnectionsFailed is already defined above in trend analysis errors

	// ErrGetCreateRelationshipFailed is returned when getting or creating relationship fails
	ErrGetCreateRelationshipFailed = errors.New("failed to get/create relationship")

	// ErrGetCreateAggregateFailed is returned when getting or creating aggregate fails
	ErrGetCreateAggregateFailed = errors.New("failed to get/create aggregate")

	// ErrQueryRelationshipFailed is returned when querying relationship fails
	ErrQueryRelationshipFailed = errors.New("failed to query relationship")

	// ErrQueryAggregateFailed is returned when querying aggregate fails
	ErrQueryAggregateFailed = errors.New("failed to query aggregate")

	// ErrSaveRelationshipFailed is returned when saving relationship fails
	ErrSaveRelationshipFailed = errors.New("failed to save relationship")

	// ErrSaveAggregateFailed is returned when saving aggregate fails
	ErrSaveAggregateFailed = errors.New("failed to save aggregate")

	// ErrQueryDormantRelationshipsFailed is returned when querying dormant relationships fails
	ErrQueryDormantRelationshipsFailed = errors.New("failed to query dormant relationships")

	// ErrQueryUserRelationshipsFailed is returned when querying user relationships fails
	ErrQueryUserRelationshipsFailed = errors.New("failed to query user relationships")

	// ErrQueryRelationshipsByStateFailed is returned when querying relationships by state fails
	ErrQueryRelationshipsByStateFailed = errors.New("failed to query relationships by state")

	// ErrGetRelationshipFailed is returned when getting relationship fails
	ErrGetRelationshipFailed = errors.New("failed to get relationship")

	// ErrSaveStateTransitionFailed is returned when saving state transition fails
	ErrSaveStateTransitionFailed = errors.New("failed to save state transition")

	// ErrGetAggregateFailed is returned when getting aggregate fails
	ErrGetAggregateFailed = errors.New("failed to get aggregate")

	// ErrGetActiveRelationshipsFailed is returned when getting active relationships fails
	ErrGetActiveRelationshipsFailed = errors.New("failed to get active relationships")

	// ErrCheckArchivedRelationshipFailed is returned when checking for archived relationship fails
	ErrCheckArchivedRelationshipFailed = errors.New("failed to check for archived relationship")

	// ErrSaveRestoredRelationshipFailed is returned when saving restored relationship fails
	ErrSaveRestoredRelationshipFailed = errors.New("failed to save restored relationship")

	// ErrSaveReactivatedRelationshipFailed is returned when saving reactivated relationship fails
	ErrSaveReactivatedRelationshipFailed = errors.New("failed to save reactivated relationship")

	// Archive operation errors
	// ErrMarshalArchiveDataFailed is returned when marshaling archive data fails
	ErrMarshalArchiveDataFailed = errors.New("failed to marshal archive data")

	// ErrCompressDataFailed is returned when compressing data fails
	ErrCompressDataFailed = errors.New("failed to compress data")

	// ErrCloseGzipWriterFailed is returned when closing gzip writer fails
	ErrCloseGzipWriterFailed = errors.New("failed to close gzip writer")

	// ErrArchiveToS3Failed is returned when archiving to S3 fails after retries
	ErrArchiveToS3Failed = errors.New("failed to archive to S3 after retries")

	// ErrMarshalBatchArchiveDataFailed is returned when marshaling batch archive data fails
	ErrMarshalBatchArchiveDataFailed = errors.New("failed to marshal batch archive data")

	// ErrCompressBatchDataFailed is returned when compressing batch data fails
	ErrCompressBatchDataFailed = errors.New("failed to compress batch data")

	// ErrUploadBatchArchiveToS3Failed is returned when uploading batch archive to S3 fails
	ErrUploadBatchArchiveToS3Failed = errors.New("failed to upload batch archive to S3")

	// ErrCreateGzipReaderFailed is returned when creating gzip reader fails
	ErrCreateGzipReaderFailed = errors.New("failed to create gzip reader")

	// ErrReadCompressedDataFailed is returned when reading compressed data fails
	ErrReadCompressedDataFailed = errors.New("failed to read compressed data")

	// ErrUnmarshalArchiveDataFailed is returned when unmarshaling archive data fails
	ErrUnmarshalArchiveDataFailed = errors.New("failed to unmarshal archive data")

	// ErrRestoreFromS3Failed is returned when restoring from S3 fails after retries
	ErrRestoreFromS3Failed = errors.New("failed to restore from S3 after retries")

	// ErrDownloadArchiveFromS3Failed is returned when downloading archive from S3 fails
	ErrDownloadArchiveFromS3Failed = errors.New("failed to download archive from S3")

	// ErrDeleteS3ArchiveFailed is returned when deleting S3 archive fails
	ErrDeleteS3ArchiveFailed = errors.New("failed to delete S3 archive")

	// ErrBatchWriteRestoredRelationshipsFailed is returned when batch writing restored relationships fails
	ErrBatchWriteRestoredRelationshipsFailed = errors.New("failed to batch write restored relationships")
)

// Relay errors
var (
	// ErrInvalidRelayURL is returned when relay URL is invalid
	ErrInvalidRelayURL = errors.New("invalid relay URL")

	// ErrFetchRelayActorFailed is returned when relay actor fetching fails
	ErrFetchRelayActorFailed = errors.New("failed to fetch relay actor")

	// ErrGetActorFailed is returned when actor retrieval fails
	ErrGetActorFailed = errors.New("failed to get actor")

	// ErrStoreRelayInfoFailed is returned when storing relay info fails
	ErrStoreRelayInfoFailed = errors.New("failed to store relay info")

	// ErrDeliverFollowActivityFailed is returned when delivering follow activity fails
	ErrDeliverFollowActivityFailed = errors.New("failed to deliver follow activity")

	// ErrRelayNotFound is returned when relay is not found
	ErrRelayNotFound = errors.New("relay not found")

	// ErrRemoveRelayInfoFailed is returned when removing relay info fails
	ErrRemoveRelayInfoFailed = errors.New("failed to remove relay info")

	// ErrUnknownInactiveRelay is returned when activity is from unknown or inactive relay
	ErrUnknownInactiveRelay = errors.New("activity from unknown or inactive relay")

	// ErrRelayForwardingFailed is returned when forwarding to multiple relays fails
	ErrRelayForwardingFailed = errors.New("failed to forward to relays")

	// ErrFetchRelayActorHTTPFailed is returned when relay actor fetch fails with non-OK status
	ErrFetchRelayActorHTTPFailed = errors.New("failed to fetch relay actor")

	// ErrNotRelayActor is returned when fetched actor is not a relay type
	ErrNotRelayActor = errors.New("not a relay actor")

	// ErrMarshalAnnouncedObjectFailed is returned when marshaling announced object fails
	ErrMarshalAnnouncedObjectFailed = errors.New("failed to marshal announced object")

	// ErrUnmarshalAnnouncedActivityFailed is returned when unmarshaling announced activity fails
	ErrUnmarshalAnnouncedActivityFailed = errors.New("failed to unmarshal announced activity")

	// ErrInvalidAnnouncedObjectType is returned when announced object type is invalid
	ErrInvalidAnnouncedObjectType = errors.New("invalid announced object type")

	// ErrRelayBudgetExceeded is returned when relay operation would exceed budget
	ErrRelayBudgetExceeded = errors.New("relay operation would exceed daily budget")

	// ErrRelayBudgetCreationFailed is returned when relay budget creation fails
	ErrRelayBudgetCreationFailed = errors.New("failed to create relay budget")

	// ErrRelayBudgetAlreadyExceeded is returned when relay budget is already exceeded
	ErrRelayBudgetAlreadyExceeded = errors.New("relay budget already exceeded")

	// ErrRelayOperationsPaused is returned when relay operations are paused due to budget limits
	ErrRelayOperationsPaused = errors.New("relay operations paused due to budget limit")

	// ErrRelayCostSummaryFailed is returned when getting relay cost summary fails
	ErrRelayCostSummaryFailed = errors.New("failed to get relay cost summary")
)

// Remote search errors
var (
	// ErrWebFingerLookupFailed is returned when webfinger lookup fails
	ErrWebFingerLookupFailed = errors.New("webfinger lookup failed")

	// ErrWebFingerRequestFailed is returned when webfinger request fails
	ErrWebFingerRequestFailed = errors.New("webfinger request failed")

	// ErrWebFingerNon2xxStatus is returned when webfinger returns non-2xx status
	ErrWebFingerNon2xxStatus = errors.New("webfinger returned non-2xx status")

	// ErrWebFingerResponseParseFailed is returned when webfinger response parsing fails
	ErrWebFingerResponseParseFailed = errors.New("failed to parse webfinger response")

	// ErrNoActivityPubLinkFound is returned when no ActivityPub link found in webfinger response
	ErrNoActivityPubLinkFound = errors.New("no ActivityPub link found in webfinger response")

	// ErrFetchRemoteActorFailed is already defined above as general actor fetching error

	// ErrRemoteActorNon2xxStatus is returned when remote actor fetch returns non-2xx status
	ErrRemoteActorNon2xxStatus = errors.New("failed to fetch actor: non-2xx status")

	// ErrRemoteActorDecodeFailed is returned when remote actor decoding fails
	ErrRemoteActorDecodeFailed = errors.New("failed to decode actor")

	// ErrInvalidActorMissingFields is returned when actor is missing required fields
	ErrInvalidActorMissingFields = errors.New("invalid actor: missing required fields")

	// ErrInvalidUsernameFormat is returned when username format is invalid
	ErrInvalidUsernameFormat = errors.New("invalid username")

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = errors.New("invalid domain")

	// ErrInvalidHandleFormat is returned when handle format is invalid
	ErrInvalidHandleFormat = errors.New("invalid handle format")

	// ErrGetKnownInstancesFailed is returned when getting known instances fails
	ErrGetKnownInstancesFailed = errors.New("failed to get known instances")

	// ErrCreateSearchRequestFailed is returned when creating search request fails
	ErrCreateSearchRequestFailed = errors.New("failed to create request")

	// ErrSearchRequestFailed is returned when search request fails
	ErrSearchRequestFailed = errors.New("search request failed")

	// ErrSearchNon2xxStatus is returned when search returns non-2xx status
	ErrSearchNon2xxStatus = errors.New("search returned non-2xx status")

	// ErrSearchResponseDecodeFailed is returned when search response decoding fails
	ErrSearchResponseDecodeFailed = errors.New("failed to decode search response")
)

// Compression pipeline errors
var (
	// ErrCompressionFailed is returned when data compression fails
	ErrCompressionFailed = errors.New("compression failed")

	// ErrDecompressionFailed is returned when data decompression fails
	ErrDecompressionFailed = errors.New("decompression failed")

	// ErrCompressionAlgorithmUnsupported is returned when compression algorithm is not supported
	ErrCompressionAlgorithmUnsupported = errors.New("compression algorithm unsupported")

	// ErrCompressionRatioInvalid is returned when compression ratio is invalid
	ErrCompressionRatioInvalid = errors.New("compression ratio invalid")

	// ErrPayloadCompressionFailed is returned when payload compression fails
	ErrPayloadCompressionFailed = errors.New("payload compression failed")

	// ErrMetricMarshalFailed is returned when metric marshaling for compression fails
	ErrMetricMarshalFailed = errors.New("failed to marshal metric")

	// ErrGzipWriteFailed is returned when writing to gzip writer fails
	ErrGzipWriteFailed = errors.New("failed to write gzip data")

	// ErrGzipCloseFailed is returned when closing gzip writer fails
	ErrGzipCloseFailed = errors.New("failed to close gzip writer")

	// ErrOldMetricsRetrievalFailed is returned when retrieving old metrics fails
	ErrOldMetricsRetrievalFailed = errors.New("failed to get old metrics")

	// ErrDataArchivalFailed is returned when data archival fails
	ErrDataArchivalFailed = errors.New("data archival failed")
)

// Analytics aggregator errors
var (
	// ErrFederationMetricStoreFailed is returned when storing federation metric fails
	ErrFederationMetricStoreFailed = errors.New("failed to store federation metric")

	// ErrHealthScoreRetrieveFailed is returned when retrieving health score fails
	ErrHealthScoreRetrieveFailed = errors.New("failed to retrieve health score")

	// ErrRecentMetricsRetrieveFailed is returned when retrieving recent metrics fails
	ErrRecentMetricsRetrieveFailed = errors.New("failed to retrieve recent metrics")

	// ErrUnhealthyDomainsRetrieveFailed is returned when retrieving unhealthy domains fails
	ErrUnhealthyDomainsRetrieveFailed = errors.New("failed to retrieve unhealthy domains")
)

// Remote actor caching errors
var (
	// ErrRemoteActorCacheRetrieveFailed is returned when retrieving cached remote actor fails
	ErrRemoteActorCacheRetrieveFailed = errors.New("failed to get cached remote actor")

	// ErrRemoteActorCacheUpdateFailed is returned when updating cached remote actor fails
	ErrRemoteActorCacheUpdateFailed = errors.New("failed to update cached remote actor")

	// ErrRemoteActorCacheStoreFailed is returned when storing cached remote actor fails
	ErrRemoteActorCacheStoreFailed = errors.New("failed to cache remote actor")
)
