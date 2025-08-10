package observability

// Metric Names - Core Performance
const (
	// Latency metrics
	MetricLatency      = "Latency"
	MetricLatencyP50   = "LatencyP50"
	MetricLatencyP90   = "LatencyP90" 
	MetricLatencyP99   = "LatencyP99"

	// Throughput metrics
	MetricThroughput       = "Throughput"
	MetricRequestsPerSecond = "RequestsPerSecond"
	
	// Error metrics
	MetricErrors         = "Errors"
	MetricErrorRate      = "ErrorRate"
	MetricSuccess        = "Success"
	MetricSuccessRate    = "SuccessRate"
	
	// Concurrency metrics
	MetricConcurrency        = "Concurrency"
	MetricActiveConnections  = "ActiveConnections"
	MetricColdStarts         = "ColdStarts"
	MetricColdStartDuration  = "ColdStartDuration"
)

// Metric Names - Business Logic
const (
	// Federation metrics
	MetricFederationSuccess    = "FederationSuccess"
	MetricFederationError      = "FederationError"
	MetricFederationLatency    = "FederationLatency"
	MetricInboxMessages        = "InboxMessages"
	MetricOutboxMessages       = "OutboxMessages"
	MetricSignatureVerification = "SignatureVerification"
	
	// Queue metrics
	MetricQueueDepth         = "QueueDepth"
	MetricQueueDepthCritical = "QueueDepthCritical"
	MetricQueueDepthWarning  = "QueueDepthWarning"
	MetricQueueDepthHealthy  = "QueueDepthHealthy"
	MetricQueueProcessingTime = "QueueProcessingTime"
	
	// Media metrics
	MetricMediaProcessing      = "MediaProcessing"
	MetricMediaProcessingTime  = "MediaProcessingTime"
	MetricMediaUpload          = "MediaUpload"
	MetricMediaTranscoding     = "MediaTranscoding"
	MetricMediaStorage         = "MediaStorage"
	
	// User activity metrics
	MetricPostsPerMinute    = "PostsPerMinute"
	MetricFollowsPerMinute  = "FollowsPerMinute"
	MetricLikesPerMinute    = "LikesPerMinute"
	MetricActiveUsers       = "ActiveUsers"
	MetricDailyActiveUsers  = "DailyActiveUsers"
)

// Metric Names - Infrastructure
const (
	// Database metrics
	MetricDynamoReadLatency   = "DynamoReadLatency"
	MetricDynamoWriteLatency  = "DynamoWriteLatency"
	MetricDynamoReadCapacity  = "DynamoReadCapacity"
	MetricDynamoWriteCapacity = "DynamoWriteCapacity"
	MetricDynamoThrottling    = "DynamoThrottling"
	
	// Lambda metrics
	MetricLambdaDuration    = "LambdaDuration"
	MetricLambdaMemoryUsed  = "LambdaMemoryUsed"
	MetricLambdaTimeout     = "LambdaTimeout"
	MetricLambdaConcurrency = "LambdaConcurrency"
	
	// Cost metrics
	MetricCost            = "Cost"
	MetricCostMicrocents  = "CostMicrocents"
	MetricCostPerUser     = "CostPerUser"
	MetricCostPerRequest  = "CostPerRequest"
	
	// Health metrics
	MetricSystemHealth     = "SystemHealth"
	MetricComponentHealth  = "ComponentHealth"
	MetricHealthCheck      = "HealthCheck"
)

// Dimension Names
const (
	DimensionService      = "Service"
	DimensionOperation    = "Operation"
	DimensionEndpoint     = "Endpoint"
	DimensionMethod       = "Method"
	DimensionStatusCode   = "StatusCode"
	DimensionErrorType    = "ErrorType"
	DimensionEnvironment  = "Environment"
	DimensionRegion       = "Region"
	DimensionInstance     = "Instance"
	DimensionQueue        = "Queue"
	DimensionResource     = "Resource"
	DimensionComponent    = "Component"
	DimensionMediaType    = "MediaType"
	DimensionUserType     = "UserType"
)

// Alert Thresholds - P0 (Critical)
const (
	// P0 thresholds require immediate attention
	AlertP0ErrorRatePercent      = 10.0  // 10% error rate
	AlertP0LatencyP99Milliseconds = 5000  // 5 second P99 latency
	AlertP0QueueDepthMessages    = 10000  // 10k messages in queue
	AlertP0CostDollarsPerHour    = 10.0   // $10/hour spend rate
	AlertP0MemoryUtilizationPercent = 95.0 // 95% memory utilization
)

// Alert Thresholds - P1 (High)
const (
	// P1 thresholds require prompt attention
	AlertP1ErrorRatePercent      = 5.0   // 5% error rate
	AlertP1LatencyP90Milliseconds = 2000  // 2 second P90 latency
	AlertP1QueueDepthMessages    = 1000   // 1k messages in queue
	AlertP1CostDollarsPerHour    = 1.0    // $1/hour spend rate
	AlertP1MemoryUtilizationPercent = 85.0 // 85% memory utilization
	AlertP1FederationFailurePercent = 20.0 // 20% federation failures
)

// Alert Thresholds - P2 (Warning)
const (
	// P2 thresholds for early warning
	AlertP2ErrorRatePercent      = 2.0   // 2% error rate  
	AlertP2LatencyP90Milliseconds = 1000  // 1 second P90 latency
	AlertP2QueueDepthMessages    = 100    // 100 messages in queue
	AlertP2CostDollarsPerHour    = 0.10   // $0.10/hour spend rate
	AlertP2MemoryUtilizationPercent = 75.0 // 75% memory utilization
	AlertP2ColdStartsPerMinute   = 10     // 10 cold starts per minute
)

// Alert Evaluation Windows (in minutes)
const (
	AlertWindowP0Minutes = 2  // P0 alerts evaluate over 2 minutes
	AlertWindowP1Minutes = 5  // P1 alerts evaluate over 5 minutes  
	AlertWindowP2Minutes = 10 // P2 alerts evaluate over 10 minutes
)

// Health Check Endpoints
const (
	HealthEndpointLive     = "/health/live"
	HealthEndpointReady    = "/health/ready"
	HealthEndpointDetailed = "/health/detailed"
)

// Health Status Values
const (
	HealthStatusHealthy  = "healthy"
	HealthStatusWarning  = "warning"
	HealthStatusCritical = "critical"
	HealthStatusUnknown  = "unknown"
)

// Metric Units (following CloudWatch standards)
const (
	UnitSeconds      = "Seconds"
	UnitMilliseconds = "Milliseconds" 
	UnitMicroseconds = "Microseconds"
	UnitCount        = "Count"
	UnitCountPerSecond = "Count/Second"
	UnitPercent      = "Percent"
	UnitBytes        = "Bytes"
	UnitKilobytes    = "Kilobytes"
	UnitMegabytes    = "Megabytes"
	UnitGigabytes    = "Gigabytes"
	UnitNone         = "None"
)

// Sampling Configuration
const (
	TracingSampleRatePercent = 10.0  // Sample 10% of traces
	MetricsSampleRatePercent = 100.0 // Sample all metrics
	LogsSampleRatePercent    = 100.0 // Sample all logs
)

// Performance Configuration  
const (
	// Maximum overhead targets
	MaxMetricsOverheadPercent = 1.0   // Max 1% performance overhead
	MaxBatchSize              = 100   // Max metrics per batch
	MaxFlushIntervalSeconds   = 30    // Max time before forced flush
	
	// Buffer sizes
	MetricsBufferSize = 1000   // Max buffered metrics
	LogsBufferSize    = 10000  // Max buffered log entries
)

// Runbook URLs
const (
	RunbookBaseURL           = "https://docs.lesser.app/runbooks"
	RunbookHighErrorRate     = RunbookBaseURL + "/high-error-rate"
	RunbookHighLatency       = RunbookBaseURL + "/high-latency"
	RunbookHighCost          = RunbookBaseURL + "/high-cost"
	RunbookQueueBacklog      = RunbookBaseURL + "/queue-backlog"
	RunbookHealthFailure     = RunbookBaseURL + "/health-failure"
	RunbookSecurityIncident  = RunbookBaseURL + "/security-incident"
	RunbookCapacityIssue     = RunbookBaseURL + "/capacity-issue"
	RunbookFederationIssue   = RunbookBaseURL + "/federation-issue"
	RunbookColdStartIssue    = RunbookBaseURL + "/cold-start-issue"
)

// Error Classifications
const (
	ErrorTypeValidation   = "validation"
	ErrorTypeAuthentication = "authentication" 
	ErrorTypeAuthorization = "authorization"
	ErrorTypeRateLimit    = "rate_limit"
	ErrorTypeTimeout      = "timeout"
	ErrorTypeInternal     = "internal"
	ErrorTypeDependency   = "dependency"
	ErrorTypeFederation   = "federation"
	ErrorTypeNotFound     = "not_found"
	ErrorTypeConflict     = "conflict"
)

// Status constants (legacy compatibility)
const (
	StatusUnknown = "unknown"
)

// Media type constants for dashboards
const (
	mediaTypeImage = "image"
	mediaTypeVideo = "video"
	mediaTypeAudio = "audio"
	mediaTypeGifv  = "gifv"
)
