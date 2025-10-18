package errors

// Lambda domain errors
// Consolidates AWS Lambda, SQS, and event processing errors from cmd/ directories

// NewLambdaError creates a new Lambda error with the specified error code and message.
func NewLambdaError(code ErrorCode, message string) *AppError {
	return NewAppError(code, CategoryLambda, message)
}

// NewLambdaInternalError creates a Lambda error with internal details wrapped from an underlying error.
func NewLambdaInternalError(code ErrorCode, message string, internal error) *AppError {
	return WrapError(internal, code, CategoryLambda, message)
}

// LambdaTimeout creates an error indicating a Lambda function timed out.
func LambdaTimeout(functionName string) *AppError {
	return NewLambdaError(CodeLambdaTimeout, "Lambda function timed out").
		WithMetadata("function_name", functionName)
}

// LambdaColdStart creates an error indicating a Lambda cold start was detected.
func LambdaColdStart(functionName string, duration int64) *AppError {
	return NewLambdaError(CodeLambdaColdStart, "Lambda cold start detected").
		WithMetadata("function_name", functionName).
		WithMetadata("duration_ms", duration)
}

// LambdaMemoryExceeded creates an error indicating a Lambda memory limit was exceeded.
func LambdaMemoryExceeded(functionName string, memoryUsed int64) *AppError {
	return NewLambdaError(CodeLambdaMemoryExceeded, "Lambda memory limit exceeded").
		WithMetadata("function_name", functionName).
		WithMetadata("memory_used_mb", memoryUsed)
}

// LambdaInitializationFailed creates an error indicating Lambda initialization failed.
func LambdaInitializationFailed(functionName string, err error) *AppError {
	return NewLambdaInternalError(CodeInternal, "Lambda initialization failed", err).
		WithMetadata("function_name", functionName)
}

// LambdaConfigurationError creates an error indicating a Lambda configuration error.
func LambdaConfigurationError(functionName, setting string) *AppError {
	return NewLambdaError(CodeInternal, "Lambda configuration error").
		WithMetadata("function_name", functionName).
		WithMetadata("setting", setting)
}

// SQSMessageInvalid creates an error indicating an invalid SQS message.
func SQSMessageInvalid(messageID string, reason string) *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid SQS message").
		WithMetadata("message_id", messageID).
		WithMetadata("reason", reason)
}

// SQSMessageProcessingFailed creates an error indicating SQS message processing failed.
func SQSMessageProcessingFailed(messageID string, err error) *AppError {
	return NewLambdaInternalError(CodeSQSProcessingFailed, "SQS message processing failed", err).
		WithMetadata("message_id", messageID).AsRetryable()
}

// SQSBatchProcessingFailed creates an error indicating SQS batch processing failed.
func SQSBatchProcessingFailed(batchSize int, successCount int, err error) *AppError {
	return NewLambdaInternalError(CodeSQSProcessingFailed, "SQS batch processing failed", err).
		WithMetadata("batch_size", batchSize).
		WithMetadata("success_count", successCount).AsRetryable()
}

// SQSMessageTooLarge creates an error indicating an SQS message exceeds size limit.
func SQSMessageTooLarge(messageSize int64) *AppError {
	return NewLambdaError(CodeContentTooLarge, "SQS message exceeds size limit").
		WithMetadata("message_size", messageSize)
}

// SQSVisibilityTimeoutExceeded creates an error indicating SQS message visibility timeout was exceeded.
func SQSVisibilityTimeoutExceeded(messageID string) *AppError {
	return NewLambdaError(CodeTimeout, "SQS message visibility timeout exceeded").
		WithMetadata("message_id", messageID).AsRetryable()
}

// SQSRetryExhausted creates an error indicating SQS message retry attempts were exhausted.
func SQSRetryExhausted(messageID string, attempts int) *AppError {
	return NewLambdaError(CodeDLQRetryExhausted, "SQS message retry exhausted").
		WithMetadata("message_id", messageID).
		WithMetadata("attempts", attempts)
}

// DLQMessageSendFailed creates an error indicating failed to send message to DLQ.
func DLQMessageSendFailed(messageID string, err error) *AppError {
	return NewLambdaInternalError(CodeInternal, "Failed to send message to DLQ", err).
		WithMetadata("message_id", messageID).AsRetryable()
}

// DLQProcessingFailed creates an error indicating DLQ message processing failed.
func DLQProcessingFailed(messageID string, err error) *AppError {
	return NewLambdaInternalError(CodeInternal, "DLQ message processing failed", err).
		WithMetadata("message_id", messageID)
}

// DLQRetryExhausted creates an error indicating DLQ retry attempts were exhausted.
func DLQRetryExhausted(messageID string, maxAttempts int) *AppError {
	return NewLambdaError(CodeDLQRetryExhausted, "DLQ retry attempts exhausted").
		WithMetadata("message_id", messageID).
		WithMetadata("max_attempts", maxAttempts)
}

// EventProcessingFailed creates an error indicating event processing failed.
func EventProcessingFailed(eventType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Event processing failed", err).
		WithMetadata("event_type", eventType).AsRetryable()
}

// EventInvalid creates an error indicating an invalid event structure.
func EventInvalid(eventType, reason string) *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid event structure").
		WithMetadata("event_type", eventType).
		WithMetadata("reason", reason)
}

// EventMissingField creates an error indicating an event is missing a required field.
func EventMissingField(eventType, field string) *AppError {
	return NewLambdaError(CodeBadRequest, "Event missing required field").
		WithMetadata("event_type", eventType).
		WithMetadata("field", field)
}

// EventTooLarge creates an error indicating an event exceeds size limit.
func EventTooLarge(eventType string, size int64) *AppError {
	return NewLambdaError(CodeContentTooLarge, "Event exceeds size limit").
		WithMetadata("event_type", eventType).
		WithMetadata("size", size)
}

// EventParsingFailed creates an error indicating failed to parse an event.
func EventParsingFailed(eventType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Failed to parse event", err).
		WithMetadata("event_type", eventType)
}

// StreamRecordInvalid creates an error indicating an invalid DynamoDB stream record.
func StreamRecordInvalid(recordID string, reason string) *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid DynamoDB stream record").
		WithMetadata("record_id", recordID).
		WithMetadata("reason", reason)
}

// StreamRecordProcessingFailed creates an error indicating stream record processing failed.
func StreamRecordProcessingFailed(recordID string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Stream record processing failed", err).
		WithMetadata("record_id", recordID).AsRetryable()
}

// StreamNewImageMissing creates an error indicating a stream record is missing new image.
func StreamNewImageMissing(recordID string) *AppError {
	return NewLambdaError(CodeBadRequest, "Stream record missing new image").
		WithMetadata("record_id", recordID)
}

// StreamOldImageMissing creates an error indicating a stream record is missing old image for removal.
func StreamOldImageMissing(recordID string) *AppError {
	return NewLambdaError(CodeBadRequest, "Stream record missing old image for removal").
		WithMetadata("record_id", recordID)
}

// StreamUnmarshalFailed creates an error indicating failed to unmarshal stream image.
func StreamUnmarshalFailed(recordID string, imageType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Failed to unmarshal stream image", err).
		WithMetadata("record_id", recordID).
		WithMetadata("image_type", imageType)
}

// ActivityRecordUnmarshalFailed creates an error indicating activity record unmarshaling failed.
func ActivityRecordUnmarshalFailed(err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Activity record unmarshaling failed", err)
}

// ActivityObjectProcessingFailed creates an error indicating activity object processing failed.
func ActivityObjectProcessingFailed(activityType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Activity object processing failed", err).
		WithMetadata("activity_type", activityType).AsRetryable()
}

// BatchHasRetryableErrors creates an error indicating a batch contains retryable errors.
func BatchHasRetryableErrors(errorCount int) *AppError {
	return NewLambdaError(CodeSQSProcessingFailed, "Batch contains retryable errors").
		WithMetadata("error_count", errorCount).AsRetryable()
}

// BatchPartialSuccess creates an error indicating batch processing partially succeeded.
func BatchPartialSuccess(successCount, totalCount int) *AppError {
	return NewLambdaError(CodeInternal, "Batch processing partially succeeded").
		WithMetadata("success_count", successCount).
		WithMetadata("total_count", totalCount)
}

// TimelineRemovalFailed creates an error indicating timeline removal failed.
func TimelineRemovalFailed(actorID string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Timeline removal failed", err).
		WithMetadata("actor_id", actorID).AsRetryable()
}

// TimelineEntriesCreationFailed creates an error indicating timeline entries creation failed.
func TimelineEntriesCreationFailed(err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Timeline entries creation failed", err).AsRetryable()
}

// TimelineEntriesWriteFailed creates an error indicating timeline entries write failed.
func TimelineEntriesWriteFailed(err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Timeline entries write failed", err).AsRetryable()
}

// WorkflowStepFailed creates an error indicating a workflow step failed.
func WorkflowStepFailed(step string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Workflow step failed", err).
		WithMetadata("step", step).AsRetryable()
}

// WorkflowInvalidState creates an error indicating a workflow is in an invalid state.
func WorkflowInvalidState(currentState string) *AppError {
	return NewLambdaError(CodeInvalidStateTransition, "Workflow in invalid state").
		WithMetadata("current_state", currentState)
}

// WorkflowTimeout creates an error indicating workflow execution timed out.
func WorkflowTimeout(workflowName string, duration int64) *AppError {
	return NewLambdaError(CodeLambdaTimeout, "Workflow execution timed out").
		WithMetadata("workflow_name", workflowName).
		WithMetadata("duration_ms", duration)
}

// EnvironmentVariableMissing creates an error indicating a required environment variable is missing.
func EnvironmentVariableMissing(varName string) *AppError {
	return NewLambdaError(CodeInternal, "Required environment variable missing").
		WithMetadata("variable_name", varName)
}

// ServiceInitializationFailed creates an error indicating service initialization failed.
func ServiceInitializationFailed(serviceName string, err error) *AppError {
	return NewLambdaInternalError(CodeInternal, "Service initialization failed", err).
		WithMetadata("service_name", serviceName)
}

// ResourceAccessDenied creates an error indicating Lambda resource access was denied.
func ResourceAccessDenied(resource string) *AppError {
	return NewLambdaError(CodeForbidden, "Lambda resource access denied").
		WithMetadata("resource", resource)
}

// ExternalServiceUnavailable creates an error indicating an external service is unavailable.
func ExternalServiceUnavailable(serviceName string, err error) *AppError {
	return NewLambdaInternalError(CodeExternalServiceUnavailable, "External service unavailable", err).
		WithMetadata("service_name", serviceName).AsRetryable()
}

// ObjectMarshalingFailed creates an error indicating object marshaling failed.
func ObjectMarshalingFailed(objectType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Object marshaling failed", err).
		WithMetadata("object_type", objectType)
}

// ObjectUnmarshalingFailed creates an error indicating object unmarshaling failed.
func ObjectUnmarshalingFailed(objectType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Object unmarshaling failed", err).
		WithMetadata("object_type", objectType)
}

// ObjectValidationFailed creates an error indicating object validation failed.
func ObjectValidationFailed(objectType string, reason string) *AppError {
	return NewLambdaError(CodeValidationFailed, "Object validation failed").
		WithMetadata("object_type", objectType).
		WithMetadata("reason", reason)
}

// NotificationProcessingFailed creates an error indicating notification processing failed.
func NotificationProcessingFailed(notificationType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Notification processing failed", err).
		WithMetadata("notification_type", notificationType).AsRetryable()
}

// PushDeliveryFailed creates an error indicating push notification delivery failed.
func PushDeliveryFailed(deviceID string, err error) *AppError {
	return NewLambdaInternalError(CodeDeliveryFailed, "Push notification delivery failed", err).
		WithMetadata("device_id", deviceID).AsRetryable()
}

// CostTrackingInitFailed creates an error indicating cost tracking initialization failed.
func CostTrackingInitFailed(err error) *AppError {
	return NewLambdaInternalError(CodeInternal, "Cost tracking initialization failed", err)
}

// CostLimitApproaching creates an error indicating Lambda cost limit is approaching.
func CostLimitApproaching(functionName string, currentCost float64, limit float64) *AppError {
	return NewLambdaError(CodeQuotaExceeded, "Lambda cost limit approaching").
		WithMetadata("function_name", functionName).
		WithMetadata("current_cost", currentCost).
		WithMetadata("limit", limit)
}

// SearchIndexingFailed creates an error indicating search indexing failed.
func SearchIndexingFailed(indexType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Search indexing failed", err).
		WithMetadata("index_type", indexType).AsRetryable()
}

// StatusIndexingFailed creates an error indicating status indexing failed.
func StatusIndexingFailed(statusID string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Status indexing failed", err).
		WithMetadata("status_id", statusID).AsRetryable()
}

// MediaProcessingFailed creates an error indicating media processing failed.
func MediaProcessingFailed(mediaType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Media processing failed", err).
		WithMetadata("media_type", mediaType).AsRetryable()
}

// MediaJobFailed creates an error indicating media job processing failed.
func MediaJobFailed(jobID string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Media job processing failed", err).
		WithMetadata("job_id", jobID)
}

// ModerationProcessingFailed creates an error indicating moderation processing failed.
func ModerationProcessingFailed(contentType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Moderation processing failed", err).
		WithMetadata("content_type", contentType).AsRetryable()
}

// TrendAggregationFailed creates an error indicating trend aggregation failed.
func TrendAggregationFailed(trendType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Trend aggregation failed", err).
		WithMetadata("trend_type", trendType).AsRetryable()
}

// MetricsProcessingFailed creates an error indicating metrics processing failed.
func MetricsProcessingFailed(metricType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Metrics processing failed", err).
		WithMetadata("metric_type", metricType)
}

// WebSocketCostAggregationFailed creates an error indicating WebSocket cost aggregation failed.
func WebSocketCostAggregationFailed(err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "WebSocket cost aggregation failed", err).AsRetryable()
}

// StreamRouterFailed creates an error indicating stream routing failed.
func StreamRouterFailed(streamType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Stream routing failed", err).
		WithMetadata("stream_type", streamType).AsRetryable()
}

// StreamingEventProcessingFailed creates an error indicating streaming event processing failed.
func StreamingEventProcessingFailed(eventType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Streaming event processing failed", err).
		WithMetadata("event_type", eventType).AsRetryable()
}

// Streaming Service Errors

// StreamingInvalidMessageFormat creates an error indicating streaming message format is invalid.
func StreamingInvalidMessageFormat() *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid message format")
}

// StreamingConnectionNotFound creates an error indicating streaming connection was not found.
func StreamingConnectionNotFound() *AppError {
	return NewLambdaError(CodeNotFound, "Connection not found")
}

// StreamingUnknownMessageType creates an error indicating an unknown streaming message type.
func StreamingUnknownMessageType() *AppError {
	return NewLambdaError(CodeBadRequest, "Unknown message type")
}

// StreamingInvalidStream creates an error indicating an invalid stream.
func StreamingInvalidStream() *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid stream")
}

// StreamingAuthenticationRequired creates an error indicating authentication is required for stream.
func StreamingAuthenticationRequired() *AppError {
	return NewLambdaError(CodeUnauthorized, "Authentication required for stream")
}

// StreamingFailedToSubscribe creates an error indicating stream subscription failed.
func StreamingFailedToSubscribe() *AppError {
	return NewLambdaError(CodeInternal, "Failed to subscribe").AsRetryable()
}

// StreamingFailedToUnsubscribe creates an error indicating stream unsubscription failed.
func StreamingFailedToUnsubscribe() *AppError {
	return NewLambdaError(CodeInternal, "Failed to unsubscribe").AsRetryable()
}

// StreamingInvalidCommandFormat creates an error indicating invalid command format.
func StreamingInvalidCommandFormat() *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid command format")
}

// StreamingCommandExecutionFailed creates an error indicating command execution failed.
func StreamingCommandExecutionFailed() *AppError {
	return NewLambdaError(CodeInternal, "Command execution failed").AsRetryable()
}

// StreamingUnknownRoute creates an error indicating an unknown WebSocket route.
func StreamingUnknownRoute() *AppError {
	return NewLambdaError(CodeNotFound, "Unknown WebSocket route")
}

// WebSocket Cost Aggregator Errors

// WebSocketCostAllAlertMethodsFailed creates an error indicating all alert methods failed.
func WebSocketCostAllAlertMethodsFailed() *AppError {
	return NewLambdaError(CodeInternal, "All alert methods failed").AsRetryable()
}

// WebSocketCostMarshalAlertMessage creates an error indicating alert message marshaling failed.
func WebSocketCostMarshalAlertMessage() *AppError {
	return NewLambdaError(CodeInternal, "Failed to marshal alert message")
}

// WebSocketCostGetIdleConnections creates an error indicating failed to get idle connections.
func WebSocketCostGetIdleConnections() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get idle connections").AsRetryable()
}

// WebSocketCostGetHighCostUsers creates an error indicating failed to get high cost users.
func WebSocketCostGetHighCostUsers() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get high cost users").AsRetryable()
}

// WebSocketCostGetStaleConnections creates an error indicating failed to get stale connections.
func WebSocketCostGetStaleConnections() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get stale connections").AsRetryable()
}

// WebSocketCostPublishSNSMessage creates an error indicating SNS message publishing failed.
func WebSocketCostPublishSNSMessage() *AppError {
	return NewLambdaError(CodeInternal, "Failed to publish SNS message").AsRetryable()
}

// WebSocketCostCreateWebhookRequest creates an error indicating webhook request creation failed.
func WebSocketCostCreateWebhookRequest() *AppError {
	return NewLambdaError(CodeInternal, "Failed to create webhook request")
}

// WebSocketCostWebhookRequestFailed creates an error indicating webhook request failed.
func WebSocketCostWebhookRequestFailed() *AppError {
	return NewLambdaError(CodeInternal, "Webhook request failed").AsRetryable()
}

// WebSocketCostWebhookNon2xxStatus creates an error indicating webhook returned non-2xx status.
func WebSocketCostWebhookNon2xxStatus() *AppError {
	return NewLambdaError(CodeInternal, "Webhook returned non-2xx status")
}

// WebSocketCostTrackIdleConnections creates an error indicating failed to track idle connections.
func WebSocketCostTrackIdleConnections() *AppError {
	return NewLambdaError(CodeInternal, "Failed to track idle connections").AsRetryable()
}

// Note Processor Errors

// NoteProcessorPartialBatchFailure creates an error indicating partial batch failure processing stream records.
func NoteProcessorPartialBatchFailure() *AppError {
	return NewLambdaError(CodeSQSProcessingFailed, "Partial batch failure processing stream records").AsRetryable()
}

// NoteProcessorGetNote creates an error indicating failed to get note.
func NoteProcessorGetNote() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get note").AsRetryable()
}

// NoteProcessorGetVotes creates an error indicating failed to get votes.
func NoteProcessorGetVotes() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get votes").AsRetryable()
}

// NoteProcessorUpdateNoteAnalysis creates an error indicating failed to update note analysis.
func NoteProcessorUpdateNoteAnalysis() *AppError {
	return NewLambdaError(CodeInternal, "Failed to update note analysis").AsRetryable()
}

// NoteProcessorUpdateNoteScore creates an error indicating failed to update note score.
func NoteProcessorUpdateNoteScore() *AppError {
	return NewLambdaError(CodeInternal, "Failed to update note score").AsRetryable()
}

// NoteProcessorDetectSentiment creates an error indicating failed to detect sentiment.
func NoteProcessorDetectSentiment() *AppError {
	return NewLambdaError(CodeInternal, "Failed to detect sentiment").AsRetryable()
}

// Search Indexer Errors

// SearchIndexerPartialBatchFailure creates an error indicating partial batch failure during search indexing.
func SearchIndexerPartialBatchFailure() *AppError {
	return NewLambdaError(CodeSQSProcessingFailed, "Partial batch failure during search indexing").AsRetryable()
}

// SearchIndexerExtractIndexableContent creates an error indicating failed to extract indexable content.
func SearchIndexerExtractIndexableContent() *AppError {
	return NewLambdaError(CodeInternal, "Failed to extract indexable content")
}

// SearchIndexerUnmarshalStreamImage creates an error indicating failed to unmarshal stream image.
func SearchIndexerUnmarshalStreamImage() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to unmarshal stream image")
}

// SearchIndexerCreateSearchIndex creates an error indicating failed to create search index.
func SearchIndexerCreateSearchIndex() *AppError {
	return NewLambdaError(CodeInternal, "Failed to create search index").AsRetryable()
}

// SearchIndexerStoreSearchIndex creates an error indicating failed to store search index.
func SearchIndexerStoreSearchIndex() *AppError {
	return NewLambdaError(CodeInternal, "Failed to store search index").AsRetryable()
}

// SearchIndexerCreateActorSearchIndex creates an error indicating failed to create actor search index.
func SearchIndexerCreateActorSearchIndex() *AppError {
	return NewLambdaError(CodeInternal, "Failed to create actor search index").AsRetryable()
}

// Metrics Aggregator Errors

// MetricsAggregatorMissingRequiredFields creates an error indicating missing required fields.
func MetricsAggregatorMissingRequiredFields() *AppError {
	return NewLambdaError(CodeBadRequest, "Missing required fields")
}

// MetricsAggregatorServiceStatsRetrieval creates an error indicating failed to get service stats.
func MetricsAggregatorServiceStatsRetrieval() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get service stats").AsRetryable()
}

// MetricsAggregatorAggregation creates an error indicating failed to aggregate metrics.
func MetricsAggregatorAggregation() *AppError {
	return NewLambdaError(CodeInternal, "Failed to aggregate metrics").AsRetryable()
}

// MetricsAggregatorCleanup creates an error indicating failed to cleanup metrics.
func MetricsAggregatorCleanup() *AppError {
	return NewLambdaError(CodeInternal, "Failed to cleanup metrics").AsRetryable()
}

// MetricsAggregatorStreamRecordUnmarshal creates an error indicating failed to unmarshal metric from stream record.
func MetricsAggregatorStreamRecordUnmarshal() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to unmarshal metric from stream record")
}

// Federation Aggregator Errors

// FederationAggregatorAWSClientsInit creates an error indicating failed to initialize AWS clients.
func FederationAggregatorAWSClientsInit() *AppError {
	return NewLambdaError(CodeInternal, "Failed to initialize AWS clients")
}

// FederationAggregatorLambdaFunctionError creates an error indicating lambda function returned error.
func FederationAggregatorLambdaFunctionError() *AppError {
	return NewLambdaError(CodeInternal, "Lambda function returned error").AsRetryable()
}

// FederationAggregatorLambdaInvocationFailed creates an error indicating failed to invoke lambda and send SQS message.
func FederationAggregatorLambdaInvocationFailed() *AppError {
	return NewLambdaError(CodeInternal, "Failed to invoke lambda and send SQS message").AsRetryable()
}

// FederationAggregatorEventUnmarshal creates an error indicating failed to unmarshal aggregation event.
func FederationAggregatorEventUnmarshal() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to unmarshal aggregation event")
}

// FederationAggregatorEventMarshal creates an error indicating failed to marshal aggregation event.
func FederationAggregatorEventMarshal() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to marshal aggregation event")
}

// FederationAggregatorStore creates an error indicating failed to store aggregation.
func FederationAggregatorStore() *AppError {
	return NewLambdaError(CodeInternal, "Failed to store aggregation").AsRetryable()
}

// FederationAggregatorActivitiesGet creates an error indicating failed to get federation activities.
func FederationAggregatorActivitiesGet() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get federation activities").AsRetryable()
}

// FederationAggregatorMessageProcessingFailed creates an error indicating failed to process SQS message.
func FederationAggregatorMessageProcessingFailed() *AppError {
	return NewLambdaError(CodeSQSProcessingFailed, "Failed to process SQS message").AsRetryable()
}

// Enhanced Federation Processor Errors

// EnhancedFederationProcessorDynamORMInit creates an error indicating failure to initialize DynamORM client.
func EnhancedFederationProcessorDynamORMInit() *AppError {
	return NewLambdaError(CodeInternal, "Failed to initialize DynamORM")
}

// EnhancedFederationProcessorUnmarshalRetryMessage creates an error indicating failure to unmarshal retry message from SQS.
func EnhancedFederationProcessorUnmarshalRetryMessage() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to unmarshal retry message")
}

// EnhancedFederationProcessorProcessRetry creates an error indicating failure to process enhanced retry operation.
func EnhancedFederationProcessorProcessRetry() *AppError {
	return NewLambdaError(CodeInternal, "Failed to process enhanced retry").AsRetryable()
}

// Status Indexer Errors

// StatusIndexerPartialBatchFailure creates an error indicating partial batch failure.
func StatusIndexerPartialBatchFailure() *AppError {
	return NewLambdaError(CodeSQSProcessingFailed, "Partial batch failure").AsRetryable()
}

// StatusIndexerProcessStatusEvent creates an error indicating failed to process status event.
func StatusIndexerProcessStatusEvent() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to process status event").AsRetryable()
}

// StatusIndexerNoNewImage creates an error indicating no new image.
func StatusIndexerNoNewImage() *AppError {
	return NewLambdaError(CodeBadRequest, "No new image")
}

// StatusIndexerNoObjectData creates an error indicating no object data.
func StatusIndexerNoObjectData() *AppError {
	return NewLambdaError(CodeBadRequest, "No object data")
}

// StatusIndexerCountReplies creates an error indicating failed to count replies.
func StatusIndexerCountReplies() *AppError {
	return NewLambdaError(CodeInternal, "Failed to count replies").AsRetryable()
}

// Federation Delivery Errors

// FederationDeliveryInvalidMessageBody creates an error indicating invalid message body format.
func FederationDeliveryInvalidMessageBody() *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid message body format")
}

// FederationDeliveryMessageMarshalFailure creates an error indicating failed to marshal message for requeue.
func FederationDeliveryMessageMarshalFailure() *AppError {
	return NewLambdaError(CodeInternal, "Failed to marshal message for requeue")
}

// FederationDeliveryMessageRequeueFailure creates an error indicating failed to requeue message.
func FederationDeliveryMessageRequeueFailure() *AppError {
	return NewLambdaError(CodeInternal, "Failed to requeue message").AsRetryable()
}

// FederationDeliverySigningActorMissing creates an error indicating signing actor not found.
func FederationDeliverySigningActorMissing() *AppError {
	return NewLambdaError(CodeNotFound, "Signing actor not found")
}

// FederationDeliveryMaxAttemptsExceeded creates an error indicating delivery failed after maximum attempts.
func FederationDeliveryMaxAttemptsExceeded() *AppError {
	return NewLambdaError(CodeDeliveryFailed, "Delivery failed after maximum attempts")
}

// Cost Aggregator Errors

// CostAggregatorAggregationFailed creates an error indicating failed to aggregate costs.
func CostAggregatorAggregationFailed() *AppError {
	return NewLambdaError(CodeInternal, "Failed to aggregate costs").AsRetryable()
}

// CostAggregatorSNSMessageMarshal creates an error indicating failed to marshal SNS message.
func CostAggregatorSNSMessageMarshal() *AppError {
	return NewLambdaError(CodeInternal, "Failed to marshal SNS message")
}

// CostAggregatorSNSPublish creates an error indicating failed to publish SNS message.
func CostAggregatorSNSPublish() *AppError {
	return NewLambdaError(CodeInternal, "Failed to publish SNS message").AsRetryable()
}

// CostAggregatorCloudWatchMetric creates an error indicating failed to put CloudWatch metric.
func CostAggregatorCloudWatchMetric() *AppError {
	return NewLambdaError(CodeInternal, "Failed to put CloudWatch metric").AsRetryable()
}

// CostAggregatorEventMarshal creates an error indicating failed to marshal aggregation event.
func CostAggregatorEventMarshal() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to marshal aggregation event")
}

// CostAggregatorLambdaInvoke creates an error indicating failed to invoke lambda and send SQS message.
func CostAggregatorLambdaInvoke() *AppError {
	return NewLambdaError(CodeInternal, "Failed to invoke lambda and send SQS message").AsRetryable()
}

// CostAggregatorLambdaFunctionError creates an error indicating lambda function returned error.
func CostAggregatorLambdaFunctionError() *AppError {
	return NewLambdaError(CodeInternal, "Lambda function returned error").AsRetryable()
}

// AI Processor Errors

// AIProcessorContentExtractionFailed creates an error indicating failed to extract content from stream record.
func AIProcessorContentExtractionFailed() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to extract content from stream record")
}

// AIProcessorStreamUnmarshalFailed creates an error indicating failed to unmarshal stream record.
func AIProcessorStreamUnmarshalFailed() *AppError {
	return NewLambdaError(CodeEventProcessingFailed, "Failed to unmarshal stream record")
}

// AIProcessorInvalidObjectPK creates an error indicating invalid object primary key format.
func AIProcessorInvalidObjectPK() *AppError {
	return NewLambdaError(CodeBadRequest, "Invalid object primary key format")
}

// AIProcessorNotAnalyzableType creates an error indicating object type is not analyzable.
func AIProcessorNotAnalyzableType() *AppError {
	return NewLambdaError(CodeBadRequest, "Object type is not analyzable")
}

// AIProcessorAnalysisFailed creates an error indicating AI analysis failed.
func AIProcessorAnalysisFailed() *AppError {
	return NewLambdaError(CodeInternal, "AI analysis failed").AsRetryable()
}

// AIProcessorAnalysisSaveFailed creates an error indicating failed to save AI analysis.
func AIProcessorAnalysisSaveFailed() *AppError {
	return NewLambdaError(CodeInternal, "Failed to save AI analysis").AsRetryable()
}

// Trend Aggregator Errors

// TrendAggregatorHashtagRetrieval creates an error indicating failed to get recent hashtags.
func TrendAggregatorHashtagRetrieval() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get recent hashtags").AsRetryable()
}

// TrendAggregatorStatusRetrieval creates an error indicating failed to get recent statuses.
func TrendAggregatorStatusRetrieval() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get recent statuses").AsRetryable()
}

// TrendAggregatorLinkRetrieval creates an error indicating failed to get recent links.
func TrendAggregatorLinkRetrieval() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get recent links").AsRetryable()
}

// Init Deploy Errors

// InitDeployFailedToCreateOrUpdateSecret creates an error indicating failed to create or update secret.
func InitDeployFailedToCreateOrUpdateSecret() *AppError {
	return NewLambdaError(CodeInternal, "Failed to create or update secret").AsRetryable()
}

// InitDeployFailedToGeneratePrivateKey creates an error indicating failed to generate private key.
func InitDeployFailedToGeneratePrivateKey() *AppError {
	return NewLambdaError(CodeInternal, "Failed to generate private key")
}

// InitDeployFailedToMarshalPrivateKey creates an error indicating failed to marshal private key.
func InitDeployFailedToMarshalPrivateKey() *AppError {
	return NewLambdaError(CodeInternal, "Failed to marshal private key")
}

// InitDeployFailedToConvertToECDHKey creates an error indicating failed to convert to ECDH key.
func InitDeployFailedToConvertToECDHKey() *AppError {
	return NewLambdaError(CodeInternal, "Failed to convert to ECDH key")
}

// Report Trust Updater Errors

// ReportTrustUpdaterMissingKeys creates an error indicating missing keys.
func ReportTrustUpdaterMissingKeys() *AppError {
	return NewLambdaError(CodeBadRequest, "Missing keys")
}

// ReportTrustUpdaterReportRetrieval creates an error indicating failed to get report.
func ReportTrustUpdaterReportRetrieval() *AppError {
	return NewLambdaError(CodeInternal, "Failed to get report").AsRetryable()
}
