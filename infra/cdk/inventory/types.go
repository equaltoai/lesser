package inventory

// LambdaType enumerates supported Lambda deployment patterns.
type LambdaType string

const (
	LambdaTypeAPIHTTP            LambdaType = "api-http"
	LambdaTypeAPIWS              LambdaType = "api-ws"
	LambdaTypeProcessorStream    LambdaType = "processor-stream"
	LambdaTypeProcessorSQS       LambdaType = "processor-sqs"
	LambdaTypeProcessorScheduled LambdaType = "processor-scheduled"
	LambdaTypeHybrid             LambdaType = "hybrid"
)

// RoleClass captures the IAM role class required by a Lambda.
type RoleClass string

const (
	RoleClassBasic      RoleClass = "basic"
	RoleClassEncryption RoleClass = "encryption"
)

// HTTPRoute represents a method/path pair owned by a Lambda.
type HTTPRoute struct {
	Method string `json:"method" yaml:"method"`
	Path   string `json:"path" yaml:"path"`
}

// WebSocketRoute represents a WebSocket route key handled by a Lambda.
// Typical route keys are $connect, $disconnect, and $default.
type WebSocketRoute struct {
	API      string `json:"api,omitempty" yaml:"api,omitempty"` // e.g. "streaming", "graphql-ws"
	RouteKey string `json:"routeKey" yaml:"routeKey"`           // e.g. "$connect"
}

// SQSTrigger declares an SQS event source configuration.
type SQSTrigger struct {
	Queue                    string `json:"queue" yaml:"queue"`
	DeadLetterQueue          string `json:"deadLetterQueue,omitempty" yaml:"deadLetterQueue,omitempty"`
	ConsumeDeadLetterQueue   bool   `json:"consumeDeadLetterQueue,omitempty" yaml:"consumeDeadLetterQueue,omitempty"`
	BatchSize                int    `json:"batchSize,omitempty" yaml:"batchSize,omitempty"`
	MaxBatchingWindowSeconds int    `json:"maxBatchingWindowSeconds,omitempty" yaml:"maxBatchingWindowSeconds,omitempty"`
	EnablePartialFailure     bool   `json:"enablePartialFailure,omitempty" yaml:"enablePartialFailure,omitempty"`
}

// StreamStartingPosition enumerates supported stream start positions.
type StreamStartingPosition string

const (
	StreamStartLatest      StreamStartingPosition = "LATEST"
	StreamStartTrimHorizon StreamStartingPosition = "TRIM_HORIZON"
)

// StreamTrigger declares a DynamoDB/Kinesis stream event source configuration.
type StreamTrigger struct {
	SourceTable              string                 `json:"sourceTable,omitempty" yaml:"sourceTable,omitempty"`
	StreamArn                string                 `json:"streamArn,omitempty" yaml:"streamArn,omitempty"`
	BatchSize                int                    `json:"batchSize,omitempty" yaml:"batchSize,omitempty"`
	MaxBatchingWindowSeconds int                    `json:"maxBatchingWindowSeconds,omitempty" yaml:"maxBatchingWindowSeconds,omitempty"`
	ParallelizationFactor    int                    `json:"parallelizationFactor,omitempty" yaml:"parallelizationFactor,omitempty"`
	StartingPosition         StreamStartingPosition `json:"startingPosition,omitempty" yaml:"startingPosition,omitempty"`
	MaxRetryAttempts         int                    `json:"maxRetryAttempts,omitempty" yaml:"maxRetryAttempts,omitempty"`
	MaxRecordAgeSeconds      int                    `json:"maxRecordAgeSeconds,omitempty" yaml:"maxRecordAgeSeconds,omitempty"`
	EnableBisectOnError      bool                   `json:"enableBisectOnError,omitempty" yaml:"enableBisectOnError,omitempty"`
	ReportBatchItemFailures  bool                   `json:"reportBatchItemFailures,omitempty" yaml:"reportBatchItemFailures,omitempty"`
}

// ScheduleTrigger declares a cron/rate-based schedule trigger.
type ScheduleTrigger struct {
	Expression string `json:"expression" yaml:"expression"` // cron(...) or rate(...)
	Input      string `json:"input,omitempty" yaml:"input,omitempty"`
}

// OperationalDefaults defines baseline settings applied unless overridden.
type OperationalDefaults struct {
	MemoryMB         int `json:"memoryMb" yaml:"memoryMb"`
	TimeoutSeconds   int `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	LogRetentionDays int `json:"logRetentionDays" yaml:"logRetentionDays"`
}

// LambdaOverrides allows per-Lambda overrides to the operational defaults.
type LambdaOverrides struct {
	MemoryMB         *int `json:"memoryMb,omitempty" yaml:"memoryMb,omitempty"`
	TimeoutSeconds   *int `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	LogRetentionDays *int `json:"logRetentionDays,omitempty" yaml:"logRetentionDays,omitempty"`
}

// LambdaSpec is the machine-readable inventory entry for a single Lambda.
type LambdaSpec struct {
	Name             string            `json:"name" yaml:"name"`
	Type             LambdaType        `json:"type" yaml:"type"`
	Role             RoleClass         `json:"role" yaml:"role"`
	HTTPRoutes       []HTTPRoute       `json:"httpRoutes,omitempty" yaml:"httpRoutes,omitempty"`
	WebSocketRoutes  []WebSocketRoute  `json:"webSocketRoutes,omitempty" yaml:"webSocketRoutes,omitempty"`
	SQSTriggers      []SQSTrigger      `json:"sqsTriggers,omitempty" yaml:"sqsTriggers,omitempty"`
	StreamTriggers   []StreamTrigger   `json:"streamTriggers,omitempty" yaml:"streamTriggers,omitempty"`
	ScheduleTriggers []ScheduleTrigger `json:"scheduleTriggers,omitempty" yaml:"scheduleTriggers,omitempty"`
	Overrides        LambdaOverrides   `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	RequiredEnvVars  []string          `json:"requiredEnvVars,omitempty" yaml:"requiredEnvVars,omitempty"`
}

// Inventory holds the operational defaults and full Lambda set.
type Inventory struct {
	Defaults OperationalDefaults `json:"defaults" yaml:"defaults"`
	Lambdas  []LambdaSpec        `json:"lambdas" yaml:"lambdas"`
}

// BaselineDefaults capture Lift-aligned non-production defaults.
var BaselineDefaults = OperationalDefaults{
	MemoryMB:         512,
	TimeoutSeconds:   30,
	LogRetentionDays: 7,
}

// ProductionDefaults provide Lift-aligned production defaults.
var ProductionDefaults = OperationalDefaults{
	MemoryMB:         3008,
	TimeoutSeconds:   30,
	LogRetentionDays: 30,
}
