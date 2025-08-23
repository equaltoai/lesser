package routing

import "errors"

// Circuit breaker errors
var (
	// ErrCircuitStateRetrieveFailed is returned when circuit state cannot be retrieved
	ErrCircuitStateRetrieveFailed = errors.New("failed to retrieve circuit state")

	// ErrCircuitStateSaveFailed is returned when circuit state cannot be saved
	ErrCircuitStateSaveFailed = errors.New("failed to save circuit state")

	// ErrCircuitStateUpdateFailed is returned when circuit state cannot be updated
	ErrCircuitStateUpdateFailed = errors.New("failed to update circuit state")

	// ErrInvalidCircuitTransition is returned when attempting an invalid state transition
	ErrInvalidCircuitTransition = errors.New("circuit must be open to transition to half-open")
)

// Health checking errors
var (
	// ErrHealthHistoryRetrieveFailed is returned when health history cannot be retrieved
	ErrHealthHistoryRetrieveFailed = errors.New("failed to get health history")

	// ErrNoHealthDataAvailable is returned when no health data is available for aggregation
	ErrNoHealthDataAvailable = errors.New("no health data available")

	// ErrInvalidURL is returned when an instance URL is invalid
	ErrInvalidURL = errors.New("invalid URL")

	// ErrHealthCheckRequestFailed is returned when a health check HTTP request fails
	ErrHealthCheckRequestFailed = errors.New("request failed")

	// ErrServerError is returned when the server returns a 5xx status code
	ErrServerError = errors.New("server error")

	// ErrClientError is returned when the server returns a 4xx status code
	ErrClientError = errors.New("client error")
)

// Query optimization errors
var (
	// ErrBatchQueryFailed is returned when a batch query operation fails
	ErrBatchQueryFailed = errors.New("batch query failed")

	// ErrInstanceNotFound is returned when a requested instance is not found
	ErrInstanceNotFound = errors.New("instance not found")

	// ErrInvalidResultType is returned when query result has unexpected type
	ErrInvalidResultType = errors.New("invalid result type")

	// ErrBatchStatusQueryFailed is returned when a batch status query fails
	ErrBatchStatusQueryFailed = errors.New("batch status query failed")

	// ErrFallbackStatusQueryFailed is returned when fallback status query fails
	ErrFallbackStatusQueryFailed = errors.New("fallback status query failed")

	// ErrPrewarmActiveInstancesFailed is returned when prewarming active instances fails
	ErrPrewarmActiveInstancesFailed = errors.New("failed to prewarm active instances")

	// ErrPrewarmActiveInstancesInMemoryFailed is returned when prewarming active instances in memory fails
	ErrPrewarmActiveInstancesInMemoryFailed = errors.New("failed to prewarm active instances in memory")

	// ErrCoordinatorStopped is returned when the batch coordinator is stopped
	ErrCoordinatorStopped = errors.New("coordinator stopped")

	// ErrBatchQueryTimeout is returned when a batch query times out
	ErrBatchQueryTimeout = errors.New("batch query timeout")

	// ErrUnknownQueryType is returned when an unknown query type is encountered
	ErrUnknownQueryType = errors.New("unknown query type")

	// ErrBatchGetInstancesFailed is returned when batch get instances operation fails
	ErrBatchGetInstancesFailed = errors.New("batch get instances failed")
)

// Instance registry errors
var (
	// ErrInstanceRegistrationFailed is returned when instance registration fails
	ErrInstanceRegistrationFailed = errors.New("failed to register instance")

	// ErrInstanceUpdateFailed is returned when instance update fails
	ErrInstanceUpdateFailed = errors.New("failed to update instance")

	// ErrInstanceUnregistrationFailed is returned when instance unregistration fails
	ErrInstanceUnregistrationFailed = errors.New("failed to unregister instance")

	// ErrInstanceHealthUpdateFailed is returned when instance health update fails
	ErrInstanceHealthUpdateFailed = errors.New("failed to update instance health")

	// ErrInstanceUsageUpdateFailed is returned when instance usage update fails
	ErrInstanceUsageUpdateFailed = errors.New("failed to update instance usage")

	// ErrInstanceBatchGetFailed is returned when batch instance retrieval fails
	ErrInstanceBatchGetFailed = errors.New("failed to batch get instances")

	// ErrInstanceBatchCreateFailed is returned when batch instance creation fails
	ErrInstanceBatchCreateFailed = errors.New("failed to batch create instances")

	// ErrInstanceBatchHealthUpdateFailed is returned when batch health update fails
	ErrInstanceBatchHealthUpdateFailed = errors.New("failed to batch update instances health")

	// ErrInstanceBatchUsageUpdateFailed is returned when batch usage update fails
	ErrInstanceBatchUsageUpdateFailed = errors.New("failed to batch update instances usage")
)

// Route management errors
var (
	// ErrGetRoutesFailed is returned when retrieving routes fails
	ErrGetRoutesFailed = errors.New("failed to get routes")

	// ErrGetInstancesFailed is returned when retrieving instances fails
	ErrGetInstancesFailed = errors.New("failed to get instances")

	// ErrRegisterInstanceFailed is returned when registering an instance fails
	ErrRegisterInstanceFailed = errors.New("failed to register instance")

	// ErrUpdateHealthFailed is returned when updating instance health fails
	ErrUpdateHealthFailed = errors.New("failed to update health")

	// ErrGetInstanceFailed is returned when retrieving a specific instance fails
	ErrGetInstanceFailed = errors.New("failed to get instance")

	// ErrListInstancesFailed is returned when listing instances fails
	ErrListInstancesFailed = errors.New("failed to list instances")

	// ErrNoRoutesForDestination is returned when no routes exist for a destination
	ErrNoRoutesForDestination = errors.New("no routes for destination")

	// ErrHealthRepositoryNotAvailable is returned when health repository is not configured
	ErrHealthRepositoryNotAvailable = errors.New("health repository not available")

	// ErrGetUnhealthyInstancesFailed is returned when retrieving unhealthy instances fails
	ErrGetUnhealthyInstancesFailed = errors.New("failed to get unhealthy instances")

	// ErrNoRoutesAvailable is returned when no routes are available for any target
	ErrNoRoutesAvailable = errors.New("no routes available for any target")

	// ErrGetRoutesInEmergencyMode is returned when getting routes fails in emergency mode
	ErrGetRoutesInEmergencyMode = errors.New("failed to get routes in emergency mode")

	// ErrInvalidInboxURLs is returned when instance has invalid inbox URLs
	ErrInvalidInboxURLs = errors.New("invalid inbox URLs")
)

// Message delivery errors
var (
	// ErrNoMessageTypeSupport is returned when no routes support the message type
	ErrNoMessageTypeSupport = errors.New("no routes support message type")

	// ErrMessageQueuedBackpressure is returned when message is queued due to backpressure
	ErrMessageQueuedBackpressure = errors.New("message queued due to emergency backpressure")

	// ErrMessageQueuedEmergency is returned when message is queued due to emergency mode
	ErrMessageQueuedEmergency = errors.New("message queued due to emergency mode")

	// ErrMessageDroppedEmergency is returned when message is dropped due to emergency mode
	ErrMessageDroppedEmergency = errors.New("message dropped due to emergency mode")

	// ErrGetSigningActorFailed is returned when getting signing actor fails
	ErrGetSigningActorFailed = errors.New("failed to get signing actor")

	// ErrFederationStoreNotConfigured is returned when federation store is not configured
	ErrFederationStoreNotConfigured = errors.New("federation store not configured")

	// ErrExtractUsernameFromActorID is returned when username cannot be extracted from actor ID
	ErrExtractUsernameFromActorID = errors.New("could not extract username from actor ID")

	// ErrGetActorFailed is returned when getting actor fails
	ErrGetActorFailed = errors.New("failed to get actor")

	// ErrMarshalActivityFailed is returned when marshaling activity fails
	ErrMarshalActivityFailed = errors.New("failed to marshal activity")

	// ErrCreateRequestFailed is returned when creating HTTP request fails
	ErrCreateRequestFailed = errors.New("failed to create request")

	// ErrFederationStoreNotConfiguredForSigning is returned when federation store is not configured for signing
	ErrFederationStoreNotConfiguredForSigning = errors.New("federation store not configured for signing")

	// ErrGetPrivateKeyFailed is returned when getting private key fails
	ErrGetPrivateKeyFailed = errors.New("failed to get private key")

	// ErrParsePrivateKeyFailed is returned when parsing private key fails
	ErrParsePrivateKeyFailed = errors.New("failed to parse private key")

	// ErrSignRequestFailed is returned when signing HTTP request fails
	ErrSignRequestFailed = errors.New("failed to sign request")

	// ErrSendRequestFailed is returned when sending HTTP request fails
	ErrSendRequestFailed = errors.New("failed to send request")

	// ErrHTTPDeliveryFailed is returned when HTTP delivery fails with an error status
	ErrHTTPDeliveryFailed = errors.New("HTTP delivery failed")

	// ErrInstanceUnhealthy is returned when instance health check determines instance is unhealthy
	ErrInstanceUnhealthy = errors.New("instance unhealthy")

	// ErrHealthCheckFailed is returned when a health check fails for an instance
	ErrHealthCheckFailed = errors.New("health check failed")
)

// Route optimization errors
var (
	// ErrRecordDeliveryResultFailed is returned when recording delivery result fails
	ErrRecordDeliveryResultFailed = errors.New("failed to record delivery result")
)

// Metrics errors
var (
	// ErrQueryRouteMetricsFailed is returned when querying route metrics fails
	ErrQueryRouteMetricsFailed = errors.New("query route metrics failed")

	// ErrQueryInstanceMetricsFailed is returned when querying instance metrics fails
	ErrQueryInstanceMetricsFailed = errors.New("query instance metrics failed")

	// ErrQueryGlobalMetricsFailed is returned when querying global metrics fails
	ErrQueryGlobalMetricsFailed = errors.New("query global metrics failed")

	// ErrPersistMetricsWindowFailed is returned when persisting metrics window fails
	ErrPersistMetricsWindowFailed = errors.New("failed to persist metrics window")

	// ErrBatchWriteMetricsFailed is returned when batch writing metrics fails
	ErrBatchWriteMetricsFailed = errors.New("batch write failed")
)