package routing

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Circuit breaker errors - consolidated to use centralized error system
var (
	// ErrCircuitStateRetrieveFailed is returned when circuit state cannot be retrieved
	ErrCircuitStateRetrieveFailed = errors.FailedToGet("circuit state", stdErrors.New("failed to get circuit state"))

	// ErrCircuitStateSaveFailed is returned when circuit state cannot be saved
	ErrCircuitStateSaveFailed = errors.FailedToSave("circuit state", stdErrors.New("failed to save circuit state"))

	// ErrCircuitStateUpdateFailed is returned when circuit state cannot be updated
	ErrCircuitStateUpdateFailed = errors.FailedToUpdate("circuit state", stdErrors.New("failed to update circuit state"))

	// ErrInvalidCircuitTransition is returned when attempting an invalid state transition
	ErrInvalidCircuitTransition = errors.InvalidValue("circuit_state", []string{"open", "half-open", "closed"}, "half-open")
)

// Health checking errors - consolidated to use centralized error system
var (
	// ErrHealthHistoryRetrieveFailed is returned when health history cannot be retrieved
	ErrHealthHistoryRetrieveFailed = errors.FailedToGet("health history", stdErrors.New("failed to get health history"))

	// ErrNoHealthDataAvailable is returned when no health data is available for aggregation
	ErrNoHealthDataAvailable = errors.ResourceUnavailable("health data")

	// ErrInvalidURL is returned when an instance URL is invalid
	ErrInvalidURL = errors.URLInvalid("")

	// ErrHealthCheckRequestFailed is returned when a health check HTTP request fails
	ErrHealthCheckRequestFailed = errors.NetworkError("health check request", stdErrors.New("health check request failed"))

	// ErrServerError is returned when the server returns a 5xx status code
	ErrServerError = errors.ExternalAPIError("health check", 500, stdErrors.New("server error"))

	// ErrClientError is returned when the server returns a 4xx status code
	ErrClientError = errors.ExternalAPIError("health check", 400, stdErrors.New("client error"))
)

// Query optimization errors - consolidated to use centralized error system
var (
	// ErrBatchQueryFailed is returned when a batch query operation fails
	ErrBatchQueryFailed = errors.BatchOperationFailed("query", stdErrors.New("batch query failed"))

	// ErrInstanceNotFound is returned when a requested instance is not found
	ErrInstanceNotFound = errors.ItemNotFound("instance")

	// ErrInvalidResultType is returned when query result has unexpected type
	ErrInvalidResultType = errors.InvalidFormat("result_type", "expected format")

	// ErrBatchStatusQueryFailed is returned when a batch status query fails
	ErrBatchStatusQueryFailed = errors.BatchOperationFailed("status query", stdErrors.New("batch status query failed"))

	// ErrFallbackStatusQueryFailed is returned when fallback status query fails
	ErrFallbackStatusQueryFailed = errors.FailedToQuery("status", stdErrors.New("fallback status query failed"))

	// ErrPrewarmActiveInstancesFailed is returned when prewarming active instances fails
	ErrPrewarmActiveInstancesFailed = errors.ProcessingFailed("instance prewarming", stdErrors.New("instance prewarming failed"))

	// ErrPrewarmActiveInstancesInMemoryFailed is returned when prewarming active instances in memory fails
	ErrPrewarmActiveInstancesInMemoryFailed = errors.ProcessingFailed("in-memory instance prewarming", stdErrors.New("in-memory instance prewarming failed"))

	// ErrCoordinatorStopped is returned when the batch coordinator is stopped
	ErrCoordinatorStopped = errors.ServiceUnavailable("batch coordinator")

	// ErrBatchQueryTimeout is returned when a batch query times out
	ErrBatchQueryTimeout = errors.TimeoutError("batch query")

	// ErrUnknownQueryType is returned when an unknown query type is encountered
	ErrUnknownQueryType = errors.InvalidValue("query_type", []string{"batch", "single", "fallback"}, "")

	// ErrBatchGetInstancesFailed is returned when batch get instances operation fails
	ErrBatchGetInstancesFailed = errors.BatchOperationFailed("get instances", stdErrors.New("batch get instances failed"))
)

// Instance registry errors - consolidated to use centralized error system
var (
	// ErrInstanceRegistrationFailed is returned when instance registration fails
	ErrInstanceRegistrationFailed = errors.FailedToCreate("instance", stdErrors.New("failed to register instance"))

	// ErrInstanceUpdateFailed is returned when instance update fails
	ErrInstanceUpdateFailed = errors.FailedToUpdate("instance", stdErrors.New("failed to update instance"))

	// ErrInstanceUnregistrationFailed is returned when instance unregistration fails
	ErrInstanceUnregistrationFailed = errors.FailedToDelete("instance", stdErrors.New("failed to unregister instance"))

	// ErrInstanceHealthUpdateFailed is returned when instance health update fails
	ErrInstanceHealthUpdateFailed = errors.FailedToUpdate("instance health", stdErrors.New("failed to update instance health"))

	// ErrInstanceUsageUpdateFailed is returned when instance usage update fails
	ErrInstanceUsageUpdateFailed = errors.FailedToUpdate("instance usage", stdErrors.New("failed to update instance usage"))

	// ErrInstanceBatchGetFailed is returned when batch instance retrieval fails
	ErrInstanceBatchGetFailed = errors.BatchOperationFailed("get instances", stdErrors.New("batch get instances failed"))

	// ErrInstanceBatchCreateFailed is returned when batch instance creation fails
	ErrInstanceBatchCreateFailed = errors.BatchOperationFailed("create instances", stdErrors.New("batch create instances failed"))

	// ErrInstanceBatchHealthUpdateFailed is returned when batch health update fails
	ErrInstanceBatchHealthUpdateFailed = errors.BatchOperationFailed("update instances health", stdErrors.New("batch health update failed"))

	// ErrInstanceBatchUsageUpdateFailed is returned when batch usage update fails
	ErrInstanceBatchUsageUpdateFailed = errors.BatchOperationFailed("update instances usage", stdErrors.New("batch usage update failed"))
)

// Route management errors - consolidated to use centralized error system
var (
	// ErrGetRoutesFailed is returned when retrieving routes fails
	ErrGetRoutesFailed = errors.FailedToGet("routes", stdErrors.New("failed to get routes"))

	// ErrGetInstancesFailed is returned when retrieving instances fails
	ErrGetInstancesFailed = errors.FailedToGet("instances", stdErrors.New("failed to get instances"))

	// ErrRegisterInstanceFailed is returned when registering an instance fails
	ErrRegisterInstanceFailed = errors.FailedToCreate("instance", stdErrors.New("failed to register instance"))

	// ErrUpdateHealthFailed is returned when updating instance health fails
	ErrUpdateHealthFailed = errors.FailedToUpdate("health", stdErrors.New("failed to update health"))

	// ErrGetInstanceFailed is returned when retrieving a specific instance fails
	ErrGetInstanceFailed = errors.FailedToGet("instance", stdErrors.New("failed to get instance"))

	// ErrListInstancesFailed is returned when listing instances fails
	ErrListInstancesFailed = errors.FailedToList("instances", stdErrors.New("failed to list instances"))

	// ErrNoRoutesForDestination is returned when no routes exist for a destination
	ErrNoRoutesForDestination = errors.ItemNotFound("route")

	// ErrHealthRepositoryNotAvailable is returned when health repository is not configured
	ErrHealthRepositoryNotAvailable = errors.ServiceNotAvailable("health repository")

	// ErrGetUnhealthyInstancesFailed is returned when retrieving unhealthy instances fails
	ErrGetUnhealthyInstancesFailed = errors.FailedToGet("unhealthy instances", stdErrors.New("failed to get unhealthy instances"))

	// ErrNoRoutesAvailable is returned when no routes are available for any target
	ErrNoRoutesAvailable = errors.ResourceUnavailable("routes")

	// ErrGetRoutesInEmergencyMode is returned when getting routes fails in emergency mode
	ErrGetRoutesInEmergencyMode = errors.FailedToGet("routes in emergency mode", stdErrors.New("failed to get routes in emergency mode"))

	// ErrInvalidInboxURLs is returned when instance has invalid inbox URLs
	ErrInvalidInboxURLs = errors.URLInvalid("inbox URL")
)

// Message delivery errors - consolidated to use centralized error system
var (
	// ErrNoMessageTypeSupport is returned when no routes support the message type
	ErrNoMessageTypeSupport = errors.ResourceUnavailable("routes for message type")

	// ErrMessageQueuedBackpressure is returned when message is queued due to backpressure
	ErrMessageQueuedBackpressure = errors.TooManyRequests("message delivery")

	// ErrMessageQueuedEmergency is returned when message is queued due to emergency mode
	ErrMessageQueuedEmergency = errors.ServiceUnavailable("message delivery")

	// ErrMessageDroppedEmergency is returned when message is dropped due to emergency mode
	ErrMessageDroppedEmergency = errors.ServiceUnavailable("message delivery")

	// ErrGetSigningActorFailed is returned when getting signing actor fails
	ErrGetSigningActorFailed = errors.ActorNotFound("")

	// ErrFederationStoreNotConfigured is returned when federation store is not configured
	ErrFederationStoreNotConfigured = errors.ConfigurationMissing("federation_store")

	// ErrExtractUsernameFromActorID is returned when username cannot be extracted from actor ID
	ErrExtractUsernameFromActorID = errors.ParsingFailed("username from actor ID", stdErrors.New("failed to extract username"))

	// ErrGetActorFailed is returned when getting actor fails
	ErrGetActorFailed = errors.ActorFetchFailed("", stdErrors.New("failed to fetch actor"))

	// ErrMarshalActivityFailed is returned when marshaling activity fails
	ErrMarshalActivityFailed = errors.MarshalingFailed("activity", stdErrors.New("failed to marshal activity"))

	// ErrCreateRequestFailed is returned when creating HTTP request fails
	ErrCreateRequestFailed = errors.ProcessingFailed("request creation", stdErrors.New("request creation failed"))

	// ErrFederationStoreNotConfiguredForSigning is returned when federation store is not configured for signing
	ErrFederationStoreNotConfiguredForSigning = errors.ConfigurationMissing("federation_store_signing")

	// ErrGetPrivateKeyFailed is returned when getting private key fails
	ErrGetPrivateKeyFailed = errors.SigningKeyNotFound("")

	// ErrParsePrivateKeyFailed is returned when parsing private key fails
	ErrParsePrivateKeyFailed = errors.SigningKeyInvalid("")

	// ErrSignRequestFailed is returned when signing HTTP request fails
	ErrSignRequestFailed = errors.SignatureVerificationFailed()

	// ErrSendRequestFailed is returned when sending HTTP request fails
	ErrSendRequestFailed = errors.NetworkError("send request", stdErrors.New("failed to send request"))

	// ErrHTTPDeliveryFailed is returned when HTTP delivery fails with an error status
	ErrHTTPDeliveryFailed = errors.DeliveryFailed("", stdErrors.New("HTTP delivery failed"))

	// ErrInstanceUnhealthy is returned when instance health check determines instance is unhealthy
	ErrInstanceUnhealthy = errors.HealthCheckFailed("", stdErrors.New("instance unhealthy"))

	// ErrHealthCheckFailed is returned when a health check fails for an instance
	ErrHealthCheckFailed = errors.HealthCheckFailed("", stdErrors.New("health check failed"))
)

// Route optimization errors - consolidated to use centralized error system
var (
	// ErrRecordDeliveryResultFailed is returned when recording delivery result fails
	ErrRecordDeliveryResultFailed = errors.FailedToStore("delivery result", stdErrors.New("failed to record delivery result"))
)

// Metrics errors - consolidated to use centralized error system
var (
	// ErrQueryRouteMetricsFailed is returned when querying route metrics fails
	ErrQueryRouteMetricsFailed = errors.FailedToQuery("route metrics", stdErrors.New("failed to query route metrics"))

	// ErrQueryInstanceMetricsFailed is returned when querying instance metrics fails
	ErrQueryInstanceMetricsFailed = errors.FailedToQuery("instance metrics", stdErrors.New("failed to query instance metrics"))

	// ErrQueryGlobalMetricsFailed is returned when querying global metrics fails
	ErrQueryGlobalMetricsFailed = errors.FailedToQuery("global metrics", stdErrors.New("failed to query global metrics"))

	// ErrPersistMetricsWindowFailed is returned when persisting metrics window fails
	ErrPersistMetricsWindowFailed = errors.FailedToStore("metrics window", stdErrors.New("failed to persist metrics window"))

	// ErrBatchWriteMetricsFailed is returned when batch writing metrics fails
	ErrBatchWriteMetricsFailed = errors.BatchOperationFailed("write metrics", stdErrors.New("batch write metrics failed"))
)
