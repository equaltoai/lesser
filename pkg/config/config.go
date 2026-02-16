// Package config provides application configuration management for Lesser's serverless deployment.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type secretsManagerValueGetter interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

var newSecretsManagerValueGetter = func(cfg aws.Config) secretsManagerValueGetter {
	return secretsmanager.NewFromConfig(cfg)
}

// InstanceMode controls which product surface is enabled for an instance.
type InstanceMode string

// Supported instance modes.
const (
	InstanceModeSocial InstanceMode = "social"
	InstanceModeCMS    InstanceMode = "cms"
	InstanceModeHybrid InstanceMode = "hybrid"
)

// Config holds the application configuration
type Config struct {
	// Instance configuration
	Domain       string // e.g., "example.com"
	InstanceName string // e.g., "My ActivityPub Server"
	InstanceMode InstanceMode

	// AWS configuration
	Region                  string
	DynamoTableName         string
	DynamoDBEndpoint        string // DynamoDB endpoint (for local development)
	S3BucketName            string
	SQSQueueURL             string
	ReputationTableName     string // For reputation/vouch storage
	AWSAccountID            string // AWS Account ID
	ExportProcessorQueueURL string // Export processor SQS queue URL
	ImportProcessorQueueURL string // Import processor SQS queue URL
	MediaProcessorQueueURL  string // Media processor SQS queue URL
	EnhancedRetryQueueURL   string // Enhanced federation retry SQS queue URL

	// Security
	JWTSecret            string // For client authentication
	JWTSecretARN         string // ARN pointing to stored secret (optional)
	KMSKeyID             string // AWS KMS key ID for encryption (optional)
	ReputationPrivateKey string // Private key for reputation system
	VAPIDPublicKey       string // VAPID public key for push notifications
	VAPIDSecretARN       string // ARN pointing to VAPID secret (private key + metadata)
	VAPIDSubject         string // Subject/identifier used for VAPID JWTs
	AdminUsername        string // Admin username for privileged operations
	SystemActorPublicKey string // System actor public key for recovery federation

	// lesser.host trust services (optional; managed instances)
	LesserHostURL             string // Base URL for lesser.host trust API
	LesserHostInstanceKey     string // Instance key for machine-to-machine trust API calls (server-side only)
	LesserHostInstanceKeyARN  string // ARN pointing to stored instance key (optional)
	LesserHostAttestationsURL string // Optional override for public attestations/JWKS base URL

	// Privacy Configuration
	PrivacyMasterKey     string // Master key for privacy hashing (required for audit privacy)
	EnablePrivacyHashing bool   // Enable privacy-preserving hashing in audit logs
	IPLevel              string // Privacy level for IP addresses: none, partial, full
	EmailLevel           string // Privacy level for email addresses: none, partial, full
	UsernameLevel        string // Privacy level for usernames: none, partial, full
	PIILevel             string // Privacy level for PII data: none, partial, full
	GenericLevel         string // Privacy level for generic data: none, partial, full

	// Privacy Key Rotation
	KeyRotationEnabled  bool          // Enable key rotation
	KeyRotationInterval time.Duration // Key rotation interval

	// Privacy Argon2 Parameters
	Argon2Memory  uint32 // Argon2 memory usage in KB
	Argon2Time    uint32 // Argon2 number of iterations
	Argon2Threads uint8  // Argon2 number of threads
	Argon2KeyLen  uint32 // Argon2 derived key length in bytes

	// ActivityPub URLs
	InboxURL     string // Inbox URL pattern
	OutboxURL    string // Outbox URL pattern
	FollowersURL string // Followers URL pattern
	FollowingURL string // Following URL pattern

	// Features
	MaxUploadSize          int64 // Maximum file upload size in bytes
	PageSize               int   // Default pagination size
	AllowRegistration      bool  // Whether new users can register
	AllowAgents            bool  // Whether agent accounts are enabled
	AllowAgentRegistration bool  // Whether new agent accounts can be registered/delegated
	AllowDeviceFlow        bool  // Whether OAuth device authorization is enabled

	// CLI automation safety rails (device-flow tokens classified as client_class=cli)
	CLIAutomationConcurrencyLimit   int           // Max concurrent in-flight requests per CLI session (sid)
	CLIAutomationBurstLimit         int           // Burst requests allowed per window
	CLIAutomationBurstWindow        time.Duration // Burst window duration
	CLIAutomationSustainedLimit     int           // Sustained requests allowed per window
	CLIAutomationSustainedWindow    time.Duration // Sustained window duration
	CLIAutomationErrorRateThreshold float64       // Fraction of requests in window that may be errors before lockout
	CLIAutomationErrorRateMin       int           // Minimum requests in window before error-rate lockout can trigger
	CLIAutomationErrorRateWindow    time.Duration // Error-rate evaluation window
	CLIAutomationLockoutDuration    time.Duration // Lockout duration when error-rate threshold exceeded

	// CMS Configuration
	CMSLongFormPublishingEnabled  bool // Enable Article creation and CMS reads
	CMSDraftSystemEnabled         bool // Enable draft storage and editing workflows
	CMSRevisionHistoryEnabled     bool // Enable revision history and restores
	CMSScheduledPublishingEnabled bool // Enable scheduled publishing worker behavior
	CMSSeriesEnabled              bool // Enable series organization
	CMSCategoriesEnabled          bool // Enable categories organization
	CMSMaxRevisionsPerObject      int  // Maximum revisions retained per CMS object (0 = unlimited)

	// Moderation Features
	DisableAWSModeration bool // Master switch to disable all AWS moderation services
	DisableComprehend    bool // Disable AWS Comprehend text analysis
	DisableRekognition   bool // Disable AWS Rekognition image/video analysis

	// Development & Debug Features
	DisableMetrics                bool // Disable metrics collection
	DisableCostTracking           bool // Disable cost tracking
	DisableRateLimiting           bool // Disable rate limiting
	DisableFederationRateLimiting bool // Disable federation-specific rate limiting
	DisableAI                     bool // Disable AI features
	EnablePlayground              bool // Enable GraphQL playground
	GraphQLAllowIntrospection     bool // Allow GraphQL introspection (non-debug deployments should keep this off)
	GraphQLMaxDepth               int  // Maximum GraphQL query depth (0 disables)
	GraphQLMaxComplexity          int  // Maximum GraphQL query complexity (0 disables)
	GraphQLParserTokenLimit       int  // Maximum GraphQL parser tokens (0 disables)
	GraphQLRequestTimeout         time.Duration
	TranslationEnabled            bool   // Enable translation features
	XRayTracingEnabled            bool   // Enable X-Ray tracing
	DebugMode                     bool   // Enable debug mode
	LogLevel                      string // Log level (debug, info, warn, error)

	// Testing Configuration
	IntegrationTestsEnabled bool // Enable integration tests
	CIEnvironment           bool // Running in CI environment
	IntegrationTestMode     bool // Integration test mode enabled

	// AI Configuration
	BedrockModelID string // AWS Bedrock model ID for AI features

	// Media Streaming Configuration
	MediaSourceBucketName    string // S3 bucket for original media uploads
	MediaStreamingBucketName string // S3 bucket for transcoded streaming outputs
	MediaConvertEndpoint     string // AWS MediaConvert custom endpoint
	MediaConvertRoleArn      string // IAM role ARN for MediaConvert
	CloudFrontDomain         string // CloudFront distribution domain for streaming
	CloudFrontKeyPairID      string // CloudFront key pair ID for signed URLs
	CloudFrontPrivateKeyPath string // Path or secret ARN for CloudFront private key
	ManifestTTLHours         int    // Default TTL for streaming manifests (hours)

	// ML Moderation Configuration
	ModerationTrainingBucketName string   // S3 bucket for ML training datasets
	ModerationModelMetadataTable string   // DynamoDB table for model metadata
	BedrockTrainingRegion        string   // AWS region for Bedrock training jobs
	BedrockInferenceModelID      string   // Bedrock model ID for moderation inference
	BedrockGuardrailID           string   // Bedrock guardrail ID for inference safety
	BedrockGuardrailVersion      string   // Bedrock guardrail version (defaults to "DRAFT")
	BedrockCustomizationRoleARN  string   // IAM role ARN for Bedrock model customization jobs
	ModerationMLEnabled          bool     // Feature flag to enable ML moderation
	ModerationMLTenants          []string // List of tenant IDs allowed to use ML moderation

	// Alerting & Monitoring
	AlertSNSTopicArn          string // SNS topic ARN for alerts
	AlertWebhookURL           string // Webhook URL for alerts
	AlertWebhookVerifySSL     bool   // Verify TLS certificates for alert webhooks (default true)
	AlertEmail                string // Email address for alerts
	WebSocketEndpoint         string // WebSocket endpoint for real-time updates
	NotificationRetryQueueURL string // Notification retry queue URL

	// WebSocket & Streaming
	ConnectionsTable   string // DynamoDB table for WebSocket connections
	SubscriptionsTable string // DynamoDB table for subscriptions
	StreamEventsTable  string // DynamoDB table for SSE stream event log
	IdleTimeoutMinutes int    // WebSocket idle timeout in minutes
	StaleTimeoutHours  int    // Stale connection timeout in hours

	// Budget & Cost Management
	BudgetAlertWebhookURL  string // Budget alert webhook URL
	BudgetAlertSNSTopicArn string // Budget alert SNS topic ARN
	CostHistoryTableName   string // Cost history DynamoDB table name

	// Notification System
	NotificationDLQURL       string // Notification dead letter queue URL
	PushNotificationTopicArn string // Push notification SNS topic ARN
	PushNotificationQueueURL string // Push notification queue URL

	// Dead Letter Queue Configuration
	DLQEnabled         bool     // Enable DLQ processing
	DLQMaxRetries      int      // Maximum number of retries before sending to DLQ
	DLQRetryDelay      int      // Retry delay in seconds
	DLQFailFast        bool     // Send to DLQ immediately on certain errors
	DLQPermanentErrors []string // Error patterns that should go straight to DLQ
	DLQTransientErrors []string // Error patterns that should be retried

	// Queue URLs for job processing
	ImportQueueURL     string // Import processing queue URL
	ExportQueueURL     string // Export generation queue URL
	MediaQueueURL      string // Media processing queue URL
	ScheduledQueueURL  string // Scheduled publishing queue URL
	FederationQueueURL string // Federation queue URL

	// Media & Storage
	MediaBucketName           string // S3 bucket for media storage
	S3MediaBucket             string // Alternative S3 bucket name
	DynamoDBEncryptionKey     string // DynamoDB encryption key
	ActorPrivateKeyEncryption string // Actor private key encryption key

	// Delivery & Processing
	FederationDeliveryMode   string // Federation delivery mode
	AuthorizedFetchEnabled   bool   // Authorized fetch enabled
	ModerationMode           string // Moderation mode setting
	AllowPublicStatusHistory bool   // Allow public access to status history

	// Deployment info
	Stage   string // Deployment stage (dev, staging, prod)
	Version string // Application version

	// Service identification
	Environment         string // Runtime environment (development, staging, production)
	ServiceName         string // Service name (lesser, api, federation, etc.)
	MonitoringEnabled   bool   // Enable monitoring and alerting
	XrayTracingEnabled  bool   // Enable X-Ray tracing
	EmfMetricsEnabled   bool   // Enable EMF metrics
	LambdaLogGroupName  string // Lambda log group name
	LambdaLogStreamName string // Lambda log stream name

	// Instance configuration
	InstanceTitle       string   // Instance title/name
	InstanceShortDesc   string   // Short description of the instance
	InstanceDescription string   // Detailed description of the instance
	InstanceAdminEmail  string   // Admin email for the instance
	InstanceLanguages   []string // Supported languages
	MaxStatusChars      int      // Maximum characters in a status
	MaxMediaSize        int64    // Maximum media file size in bytes
	MaxVideoSize        int64    // Maximum video file size in bytes
	RegistrationsOpen   bool     // Whether registrations are open
	ApprovalRequired    bool     // Whether new registrations require approval
	InvitesEnabled      bool     // Whether invites are enabled
	FederationEnabled   bool     // Whether federation is enabled

	// Tips (TipSplitter integration; non-secret public config)
	TipEnabled         bool   // Whether on-chain tipping is enabled for this instance
	TipChainID         int    // EVM chain ID (e.g. 8453)
	TipContractAddress string // TipSplitter contract address
}

var config *Config

// Get returns the current configuration
func Get() *Config {
	if config == nil {
		config = loadConfig()
	}
	return config
}

// ResetForTests clears cached configuration so tests can vary environment variables
// safely within a single package test run.
//
// This should only be used in tests.
func ResetForTests() {
	config = nil
	jwtSecretLoader = struct {
		once  sync.Once
		value string
		err   error
	}{}
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	envCanonical, _, stageRaw := resolveEnvironmentAndStage()
	envForFields := firstNonEmpty(envCanonical, "development")
	stageForFields := firstNonEmpty(stageRaw, envCanonical, "dev")

	dynamoTable := resolveDynamoTableName()
	mediaBucket := resolveMediaBucketName("lesser-media")
	domainName := resolveDomain("localhost")

	importQueue := resolveQueueURL("IMPORT_QUEUE_URL", "IMPORT_PROCESSOR_QUEUE_URL")
	exportQueue := resolveQueueURL("EXPORT_QUEUE_URL", "EXPORT_PROCESSOR_QUEUE_URL")
	mediaQueue := resolveQueueURL("MEDIA_QUEUE_URL", "MEDIA_PROCESSOR_QUEUE_URL")
	scheduledQueue := resolveQueueURL("SCHEDULED_QUEUE_URL")
	federationQueue := resolveQueueURL("FEDERATION_DELIVERY_QUEUE_URL", "FEDERATION_QUEUE_URL")
	pushQueue := resolveQueueURL("PUSH_NOTIFICATION_QUEUE_URL", "PUSH_QUEUE_URL")

	instanceMode := parseInstanceMode(getEnvOrDefault("INSTANCE_MODE", string(InstanceModeHybrid)))
	cmsEnabledByMode := instanceMode == InstanceModeCMS || instanceMode == InstanceModeHybrid

	cfg := &Config{
		Domain:       domainName,
		InstanceName: getEnvOrDefault("INSTANCE_NAME", "Lesser ActivityPub Server"),
		InstanceMode: instanceMode,

		Region:                  getEnvOrDefault("AWS_REGION", "us-east-1"),
		DynamoTableName:         dynamoTable,
		DynamoDBEndpoint:        getEnvOrDefault("DYNAMODB_ENDPOINT", ""),
		S3BucketName:            mediaBucket,
		SQSQueueURL:             getEnvOrDefault("SQS_QUEUE_URL", ""),
		ExportProcessorQueueURL: exportQueue,
		ImportProcessorQueueURL: importQueue,
		MediaProcessorQueueURL:  mediaQueue,
		EnhancedRetryQueueURL:   getEnvOrDefault("ENHANCED_RETRY_QUEUE_URL", ""),
		ReputationTableName:     getEnvOrDefault("REPUTATION_TABLE_NAME", "lesser-reputation"),
		AWSAccountID:            getEnvOrDefault("AWS_ACCOUNT_ID", ""),

		JWTSecret:            mustGetJWTSecret(),
		JWTSecretARN:         getEnvOrDefault("JWT_SECRET_ARN", ""),
		KMSKeyID:             getEnvOrDefault("KMS_KEY_ID", ""), // Optional - defaults to AWS managed key
		ReputationPrivateKey: getEnvOrDefault("REPUTATION_PRIVATE_KEY", ""),
		VAPIDPublicKey:       getEnvOrDefault("VAPID_PUBLIC_KEY", ""),
		VAPIDSecretARN:       getEnvOrDefault("VAPID_SECRET_ARN", ""),
		VAPIDSubject:         getEnvOrDefault("VAPID_SUBJECT", ""),
		AdminUsername:        getEnvOrDefault("ADMIN_USERNAME", ""),
		SystemActorPublicKey: getEnvOrDefault("SYSTEM_ACTOR_PUBLIC_KEY", ""),
		LesserHostURL:        strings.TrimRight(strings.TrimSpace(getEnvOrDefault("LESSER_HOST_URL", "https://lesser.host")), "/"),
		LesserHostInstanceKey: getOptionalSecretFromEnvOrARN(
			"LESSER_HOST_INSTANCE_KEY",
			"LESSER_HOST_INSTANCE_KEY_ARN",
		),
		LesserHostInstanceKeyARN:  strings.TrimSpace(getEnvOrDefault("LESSER_HOST_INSTANCE_KEY_ARN", "")),
		LesserHostAttestationsURL: strings.TrimRight(strings.TrimSpace(getEnvOrDefault("LESSER_HOST_ATTESTATIONS_URL", "")), "/"),

		// Privacy configuration
		PrivacyMasterKey:     getEnvOrDefault("PRIVACY_MASTER_KEY", ""),
		EnablePrivacyHashing: getEnvAsBoolOrDefault("ENABLE_PRIVACY_HASHING", false),
		IPLevel:              getEnvOrDefault("IP_PRIVACY_LEVEL", "partial"),
		EmailLevel:           getEnvOrDefault("EMAIL_PRIVACY_LEVEL", "partial"),
		UsernameLevel:        getEnvOrDefault("USERNAME_PRIVACY_LEVEL", "full"),
		PIILevel:             getEnvOrDefault("PII_PRIVACY_LEVEL", "full"),
		GenericLevel:         getEnvOrDefault("GENERIC_PRIVACY_LEVEL", "full"),

		// Privacy Key Rotation
		KeyRotationEnabled:  getEnvAsBoolOrDefault("KEY_ROTATION_ENABLED", false),
		KeyRotationInterval: getEnvAsDurationOrDefault("KEY_ROTATION_INTERVAL", 24*time.Hour),

		// Privacy Argon2 Parameters
		Argon2Memory:  getEnvAsUint32OrDefault("ARGON2_MEMORY", 65536),
		Argon2Time:    getEnvAsUint32OrDefault("ARGON2_TIME", 3),
		Argon2Threads: getEnvAsUint8OrDefault("ARGON2_THREADS", 4),
		Argon2KeyLen:  getEnvAsUint32OrDefault("ARGON2_KEY_LENGTH", 32),

		MaxUploadSize:                   getEnvAsInt64OrDefault("MAX_UPLOAD_SIZE", 10*1024*1024), // 10MB default
		PageSize:                        getEnvAsIntOrDefault("PAGE_SIZE", 20),
		AllowRegistration:               getEnvAsBoolOrDefault("ALLOW_REGISTRATION", false),
		AllowAgents:                     getEnvAsBoolOrDefault("ALLOW_AGENTS", false),
		AllowAgentRegistration:          getEnvAsBoolOrDefault("ALLOW_AGENT_REGISTRATION", false),
		AllowDeviceFlow:                 getEnvAsBoolOrDefault("ALLOW_DEVICE_FLOW", false),
		CLIAutomationConcurrencyLimit:   getEnvAsIntOrDefault("CLI_AUTOMATION_CONCURRENCY_LIMIT", 2),
		CLIAutomationBurstLimit:         getEnvAsIntOrDefault("CLI_AUTOMATION_BURST_LIMIT", 20),
		CLIAutomationBurstWindow:        getEnvAsDurationOrDefault("CLI_AUTOMATION_BURST_WINDOW", 10*time.Second),
		CLIAutomationSustainedLimit:     getEnvAsIntOrDefault("CLI_AUTOMATION_SUSTAINED_LIMIT", 60),
		CLIAutomationSustainedWindow:    getEnvAsDurationOrDefault("CLI_AUTOMATION_SUSTAINED_WINDOW", time.Minute),
		CLIAutomationErrorRateThreshold: getEnvAsFloat64OrDefault("CLI_AUTOMATION_ERROR_RATE_THRESHOLD", 0.10),
		CLIAutomationErrorRateMin:       getEnvAsIntOrDefault("CLI_AUTOMATION_ERROR_RATE_MIN_REQUESTS", 10),
		CLIAutomationErrorRateWindow:    getEnvAsDurationOrDefault("CLI_AUTOMATION_ERROR_RATE_WINDOW", time.Minute),
		CLIAutomationLockoutDuration:    getEnvAsDurationOrDefault("CLI_AUTOMATION_LOCKOUT_DURATION", time.Hour),

		// CMS Configuration
		CMSLongFormPublishingEnabled:  getEnvAsBoolOrDefault("CMS_LONG_FORM_PUBLISHING_ENABLED", cmsEnabledByMode),
		CMSDraftSystemEnabled:         getEnvAsBoolOrDefault("CMS_DRAFT_SYSTEM_ENABLED", cmsEnabledByMode),
		CMSRevisionHistoryEnabled:     getEnvAsBoolOrDefault("CMS_REVISION_HISTORY_ENABLED", cmsEnabledByMode),
		CMSScheduledPublishingEnabled: getEnvAsBoolOrDefault("CMS_SCHEDULED_PUBLISHING_ENABLED", cmsEnabledByMode),
		CMSSeriesEnabled:              getEnvAsBoolOrDefault("CMS_SERIES_ENABLED", cmsEnabledByMode),
		CMSCategoriesEnabled:          getEnvAsBoolOrDefault("CMS_CATEGORIES_ENABLED", cmsEnabledByMode),
		CMSMaxRevisionsPerObject:      getEnvAsIntOrDefault("CMS_MAX_REVISIONS_PER_OBJECT", 0),

		// Moderation flags - default to false (AWS enabled by default)
		DisableAWSModeration: getEnvAsBoolOrDefault("DISABLE_AWS_MODERATION", false),
		DisableComprehend:    getEnvAsBoolOrDefault("DISABLE_COMPREHEND", false),
		DisableRekognition:   getEnvAsBoolOrDefault("DISABLE_REKOGNITION", false),

		// Development & Debug Features
		DisableMetrics:                getEnvAsBoolOrDefault("DISABLE_METRICS", false),
		DisableCostTracking:           getEnvAsBoolOrDefault("DISABLE_COST_TRACKING", false),
		DisableRateLimiting:           getEnvAsBoolOrDefault("DISABLE_RATE_LIMITING", false),
		DisableFederationRateLimiting: getEnvAsBoolOrDefault("DISABLE_FEDERATION_RATE_LIMITING", false),
		DisableAI:                     getEnvAsBoolOrDefault("DISABLE_AI", false),
		EnablePlayground:              getEnvAsBoolOrDefault("ENABLE_PLAYGROUND", false),
		GraphQLAllowIntrospection:     getEnvAsBoolOrDefault("GRAPHQL_ALLOW_INTROSPECTION", false),
		GraphQLMaxDepth:               getEnvAsIntOrDefault("GRAPHQL_MAX_DEPTH", 12),
		GraphQLMaxComplexity:          getEnvAsIntOrDefault("GRAPHQL_MAX_COMPLEXITY", 2000),
		GraphQLParserTokenLimit:       getEnvAsIntOrDefault("GRAPHQL_PARSER_TOKEN_LIMIT", 15000),
		GraphQLRequestTimeout:         getEnvAsDurationOrDefault("GRAPHQL_REQUEST_TIMEOUT", 25*time.Second),
		TranslationEnabled:            getEnvAsBoolOrDefault("TRANSLATION_ENABLED", false),
		XRayTracingEnabled:            getEnvAsBoolOrDefault("XRAY_TRACING_ENABLED", true),
		DebugMode:                     getEnvAsBoolOrDefault("DEBUG", false),
		LogLevel:                      getEnvOrDefault("LOG_LEVEL", "info"),

		// Testing Configuration
		IntegrationTestsEnabled: getEnvAsBoolOrDefault("INTEGRATION_TESTS", false),
		CIEnvironment:           getEnvAsBoolOrDefault("CI", false),
		IntegrationTestMode:     getEnvAsBoolOrDefault("INTEGRATION_TEST", false),

		// AI Configuration
		BedrockModelID: getEnvOrDefault("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku-20240307-v1:0"),

		// Alerting & Monitoring
		AlertSNSTopicArn:          getEnvOrDefault("ALERT_SNS_TOPIC_ARN", ""),
		AlertWebhookURL:           getEnvOrDefault("ALERT_WEBHOOK_URL", ""),
		AlertWebhookVerifySSL:     getEnvAsBoolOrDefault("ALERT_WEBHOOK_VERIFY_SSL", true),
		AlertEmail:                getEnvOrDefault("ALERT_EMAIL", ""),
		WebSocketEndpoint:         resolveWebsocketEndpoint(),
		NotificationRetryQueueURL: getEnvOrDefault("NOTIFICATION_RETRY_QUEUE_URL", ""),

		// WebSocket & Streaming
		ConnectionsTable:   getEnvOrDefault("CONNECTIONS_TABLE", ""),
		SubscriptionsTable: getEnvOrDefault("SUBSCRIPTIONS_TABLE", ""),
		StreamEventsTable:  getEnvOrDefault("STREAM_EVENTS_TABLE_NAME", ""),
		IdleTimeoutMinutes: getEnvAsIntOrDefault("IDLE_TIMEOUT_MINUTES", 30),
		StaleTimeoutHours:  getEnvAsIntOrDefault("STALE_TIMEOUT_HOURS", 24),

		// Budget & Cost Management
		BudgetAlertWebhookURL:  getEnvOrDefault("BUDGET_ALERT_WEBHOOK_URL", ""),
		BudgetAlertSNSTopicArn: getEnvOrDefault("BUDGET_ALERT_SNS_TOPIC_ARN", ""),
		CostHistoryTableName:   getEnvOrDefault("COST_HISTORY_TABLE_NAME", ""),

		// Notification System
		NotificationDLQURL:       getEnvOrDefault("NOTIFICATION_DLQ_URL", ""),
		PushNotificationTopicArn: getEnvOrDefault("PUSH_NOTIFICATION_TOPIC_ARN", ""),
		PushNotificationQueueURL: pushQueue,

		// Dead Letter Queue Configuration
		DLQEnabled:         getEnvAsBoolOrDefault("DLQ_ENABLED", false),
		DLQMaxRetries:      getEnvAsIntOrDefault("DLQ_MAX_RETRIES", 3),
		DLQRetryDelay:      getEnvAsIntOrDefault("DLQ_RETRY_DELAY", 60),
		DLQFailFast:        getEnvAsBoolOrDefault("DLQ_FAIL_FAST", false),
		DLQPermanentErrors: getEnvAsStringSliceOrDefault("DLQ_PERMANENT_ERRORS", []string{}),
		DLQTransientErrors: getEnvAsStringSliceOrDefault("DLQ_TRANSIENT_ERRORS", []string{}),

		// Queue URLs for job processing
		ImportQueueURL:     importQueue,
		ExportQueueURL:     exportQueue,
		MediaQueueURL:      mediaQueue,
		ScheduledQueueURL:  scheduledQueue,
		FederationQueueURL: federationQueue,

		// Media & Storage
		MediaBucketName:           mediaBucket,
		S3MediaBucket:             getEnvOrDefault("S3_MEDIA_BUCKET", ""),
		DynamoDBEncryptionKey:     getEnvOrDefault("DYNAMODB_ENCRYPTION_KEY", ""),
		ActorPrivateKeyEncryption: getEnvOrDefault("ACTOR_PRIVATE_KEY_ENCRYPTION", ""),

		// Media Streaming Configuration
		MediaSourceBucketName:    getEnvOrDefault("MEDIA_SOURCE_BUCKET_NAME", ""),
		MediaStreamingBucketName: getEnvOrDefault("MEDIA_STREAMING_BUCKET_NAME", ""),
		MediaConvertEndpoint:     getEnvOrDefault("MEDIA_CONVERT_ENDPOINT", ""),
		MediaConvertRoleArn:      getEnvOrDefault("MEDIA_CONVERT_ROLE_ARN", ""),
		CloudFrontDomain:         getEnvOrDefault("CLOUDFRONT_DOMAIN", ""),
		CloudFrontKeyPairID:      getEnvOrDefault("CLOUDFRONT_KEY_PAIR_ID", ""),
		CloudFrontPrivateKeyPath: getEnvOrDefault("CLOUDFRONT_PRIVATE_KEY_PATH", ""),
		ManifestTTLHours:         getEnvAsIntOrDefault("MANIFEST_TTL_HOURS", 24),

		// ML Moderation Configuration
		ModerationTrainingBucketName: getEnvOrDefault("MODERATION_TRAINING_BUCKET_NAME", ""),
		ModerationModelMetadataTable: getEnvOrDefault("MODERATION_MODEL_METADATA_TABLE", ""),
		BedrockTrainingRegion:        getEnvOrDefault("BEDROCK_TRAINING_REGION", "us-east-1"),
		BedrockInferenceModelID:      getEnvOrDefault("BEDROCK_INFERENCE_MODEL_ID", ""),
		BedrockGuardrailID:           getEnvOrDefault("BEDROCK_GUARDRAIL_ID", ""),
		BedrockGuardrailVersion:      getEnvOrDefault("BEDROCK_GUARDRAIL_VERSION", "DRAFT"),
		BedrockCustomizationRoleARN:  getEnvOrDefault("BEDROCK_CUSTOMIZATION_ROLE_ARN", ""),
		ModerationMLEnabled:          getEnvAsBoolOrDefault("MODERATION_ML_ENABLED", false),
		ModerationMLTenants:          getEnvAsStringSliceOrDefault("MODERATION_ML_TENANTS", []string{}),

		// Delivery & Processing
		FederationDeliveryMode: getEnvOrDefault("FEDERATION_DELIVERY_MODE", ""),
		AuthorizedFetchEnabled: getEnvAsBoolOrDefault("AUTHORIZED_FETCH_ENABLED", false),
		ModerationMode:         getEnvOrDefault("MODERATION_MODE", ""),

		Stage:   stageForFields,
		Version: getEnvOrDefault("VERSION", "v1.0.0"),

		// Service identification
		Environment:         envForFields,
		ServiceName:         getEnvOrDefault("SERVICE_NAME", "lesser"),
		MonitoringEnabled:   getEnvAsBoolOrDefault("MONITORING_ENABLED", true),
		XrayTracingEnabled:  getEnvAsBoolOrDefault("XRAY_TRACING_ENABLED", false),
		EmfMetricsEnabled:   getEnvAsBoolOrDefault("EMF_METRICS_ENABLED", true),
		LambdaLogGroupName:  getEnvOrDefault("AWS_LAMBDA_LOG_GROUP_NAME", ""),
		LambdaLogStreamName: getEnvOrDefault("AWS_LAMBDA_LOG_STREAM_NAME", ""),

		// Instance configuration
		InstanceTitle:            getEnvOrDefault("INSTANCE_TITLE", "Lesser Instance"),
		InstanceShortDesc:        getEnvOrDefault("INSTANCE_SHORT_DESC", "A personal ActivityPub server"),
		InstanceDescription:      getEnvOrDefault("INSTANCE_DESCRIPTION", "A lightweight, serverless ActivityPub implementation"),
		InstanceAdminEmail:       getEnvOrDefault("INSTANCE_ADMIN_EMAIL", "admin@localhost"),
		InstanceLanguages:        getEnvAsStringSliceOrDefault("INSTANCE_LANGUAGES", []string{"en"}),
		MaxStatusChars:           getEnvAsIntOrDefault("MAX_STATUS_CHARS", 5000),
		MaxMediaSize:             getEnvAsInt64OrDefault("MAX_MEDIA_SIZE", 10*1024*1024), // 10MB
		MaxVideoSize:             getEnvAsInt64OrDefault("MAX_VIDEO_SIZE", 40*1024*1024), // 40MB
		RegistrationsOpen:        getEnvAsBoolOrDefault("REGISTRATIONS_OPEN", false),
		ApprovalRequired:         getEnvAsBoolOrDefault("APPROVAL_REQUIRED", true),
		InvitesEnabled:           getEnvAsBoolOrDefault("INVITES_ENABLED", false),
		FederationEnabled:        getEnvAsBoolOrDefault("FEDERATION_ENABLED", true),
		AllowPublicStatusHistory: getEnvAsBoolOrDefault("ALLOW_PUBLIC_STATUS_HISTORY", false),

		// Tips (TipSplitter integration; public config)
		TipEnabled:         getEnvAsBoolOrDefault("TIP_ENABLED", false),
		TipChainID:         getEnvAsIntOrDefault("TIP_CHAIN_ID", 0),
		TipContractAddress: strings.TrimSpace(getEnvOrDefault("TIP_CONTRACT_ADDRESS", "")),
	}

	return cfg
}

// BaseURL returns the base URL for the instance
func (c *Config) BaseURL() string {
	protocol := "https"
	if c.Domain == "localhost" || c.Domain == "127.0.0.1" {
		protocol = "http"
	}
	return fmt.Sprintf("%s://%s", protocol, c.Domain)
}

// ActorURL returns the URL for an actor
func (c *Config) ActorURL(username string) string {
	return fmt.Sprintf("%s/users/%s", c.BaseURL(), username)
}

// ObjectURL returns the URL for an object
func (c *Config) ObjectURL(objectType, id string) string {
	return fmt.Sprintf("%s/%s/%s", c.BaseURL(), objectType, id)
}

// EffectiveInstanceMode returns the normalized instance mode (defaults to `hybrid`).
func (c *Config) EffectiveInstanceMode() InstanceMode {
	if c == nil {
		return InstanceModeHybrid
	}
	return parseInstanceMode(string(c.InstanceMode))
}

// CMSEnabled reports whether the CMS surface is enabled for this instance mode.
func (c *Config) CMSEnabled() bool {
	mode := c.EffectiveInstanceMode()
	return mode == InstanceModeCMS || mode == InstanceModeHybrid
}

// CMSLongFormEnabled reports whether long-form publishing (Articles) is enabled.
func (c *Config) CMSLongFormEnabled() bool {
	return c != nil && c.CMSEnabled() && c.CMSLongFormPublishingEnabled
}

// CMSDraftsEnabled reports whether the draft system is enabled.
func (c *Config) CMSDraftsEnabled() bool {
	return c != nil && c.CMSEnabled() && c.CMSDraftSystemEnabled
}

// CMSRevisionsEnabled reports whether revision history is enabled.
func (c *Config) CMSRevisionsEnabled() bool {
	return c != nil && c.CMSEnabled() && c.CMSRevisionHistoryEnabled
}

// CMSSchedulingEnabled reports whether scheduled publishing is enabled.
func (c *Config) CMSSchedulingEnabled() bool {
	return c != nil && c.CMSEnabled() && c.CMSScheduledPublishingEnabled
}

// CMSSeriesAllowed reports whether series organization is enabled.
func (c *Config) CMSSeriesAllowed() bool {
	return c != nil && c.CMSEnabled() && c.CMSSeriesEnabled
}

// CMSCategoriesAllowed reports whether category organization is enabled.
func (c *Config) CMSCategoriesAllowed() bool {
	return c != nil && c.CMSEnabled() && c.CMSCategoriesEnabled
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

var jwtSecretLoader struct {
	once  sync.Once
	value string
	err   error
}

func mustGetJWTSecret() string {
	// Check for plain text secret (not recommended for production)
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return secret
	}
	if isRunningTests() {
		// Avoid reaching out to AWS Secrets Manager during unit tests.
		return "dummy"
	}

	// Get ARN from environment
	arn := os.Getenv("JWT_SECRET_ARN")
	if arn == "" {
		// Default to secret name (will be looked up in the region)
		arn = "lesser/jwt-secret"
	}

	// Load secret from AWS Secrets Manager (once)
	jwtSecretLoader.once.Do(func() {
		jwtSecretLoader.value, jwtSecretLoader.err = fetchSecretValue(arn)
	})

	if jwtSecretLoader.err != nil {
		panic(fmt.Sprintf("Failed to resolve JWT secret from %s: %v", arn, jwtSecretLoader.err))
	}

	return jwtSecretLoader.value
}

func fetchSecretValue(arn string) (string, error) {
	ctx := context.Background()
	cfg, err := loadDefaultAWSConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}

	sm := newSecretsManagerValueGetter(cfg)
	output, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &arn})
	if err != nil {
		return "", fmt.Errorf("get secret value: %w", err)
	}

	if output.SecretString == nil {
		return "", errors.New("secret does not contain SecretString")
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(*output.SecretString), &payload); err != nil {
		return "", fmt.Errorf("unmarshal secret string: %w", err)
	}

	val, ok := payload["secret"]
	if !ok || val == "" {
		return "", errors.New("secret payload missing 'secret' key")
	}

	return val, nil
}

func getOptionalSecretFromEnvOrARN(valueKey, arnKey string) string {
	if v := strings.TrimSpace(os.Getenv(valueKey)); v != "" {
		return v
	}

	arn := strings.TrimSpace(os.Getenv(arnKey))
	if arn == "" {
		return ""
	}
	if isRunningTests() {
		// Avoid reaching out to AWS Secrets Manager during unit tests.
		return ""
	}

	val, err := fetchSecretValue(arn)
	if err != nil {
		panic(fmt.Sprintf("Failed to resolve %s from %s: %v", arnKey, arn, err))
	}
	return val
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

func getEnvAsInt64OrDefault(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int64
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

func getEnvAsBoolOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

func getEnvAsStringSliceOrDefault(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Parse comma-separated list
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}

func getEnvAsDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}

func getEnvAsUint32OrDefault(key string, defaultValue uint32) uint32 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result uint32
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

func getEnvAsUint8OrDefault(key string, defaultValue uint8) uint8 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result uint8
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

func getEnvAsFloat64OrDefault(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result float64
	if _, err := fmt.Sscanf(value, "%f", &result); err != nil {
		return defaultValue
	}
	return result
}

// Helper functions for health checks

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func resolveEnvFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func resolveEnvironmentAndStage() (env string, envRaw string, stage string) {
	envRaw = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	stage = strings.TrimSpace(os.Getenv("STAGE"))
	env = firstNonEmpty(envRaw, stage)
	return env, envRaw, stage
}

func parseInstanceMode(value string) InstanceMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(InstanceModeSocial):
		return InstanceModeSocial
	case string(InstanceModeCMS):
		return InstanceModeCMS
	case string(InstanceModeHybrid), "":
		return InstanceModeHybrid
	default:
		return InstanceModeHybrid
	}
}

func isRunningTests() bool {
	// Common patterns:
	// - os.Args[0] ends with ".test" for test binaries
	// - go test passes flags prefixed with "-test."
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}
	return false
}

func resolveDynamoTableName() string {
	table := resolveEnvFirst("DYNAMODB_TABLE", "DYNAMO_TABLE_NAME")
	if table != "" {
		return table
	}

	env, envRaw, stage := resolveEnvironmentAndStage()
	derived := firstNonEmpty(env, envRaw, stage)
	if derived == "" {
		if isRunningTests() {
			return "test-table"
		}
		// Default to dev for local tooling (CLI/help/tests outside `go test`) to avoid hard-failing on missing env.
		// Production environments should always provide an explicit table name.
		derived = "dev"
	}
	return fmt.Sprintf("lesser-%s", strings.ToLower(derived))
}

func resolveDomain(defaultValue string) string {
	if v := resolveEnvFirst("DOMAIN_NAME", "DOMAIN"); v != "" {
		return v
	}
	return defaultValue
}

func resolveMediaBucketName(defaultValue string) string {
	if v := resolveEnvFirst("S3_BUCKET_NAME", "S3_BUCKET", "S3_MEDIA_BUCKET", "MEDIA_BUCKET_NAME"); v != "" {
		return v
	}
	return defaultValue
}

func resolveQueueURL(canonical string, aliases ...string) string {
	if v := resolveEnvFirst(canonical); v != "" {
		return v
	}
	return resolveEnvFirst(aliases...)
}

func resolveWebsocketEndpoint() string {
	return resolveEnvFirst("WEBSOCKET_ENDPOINT", "WEBSOCKET_API_URL")
}

// GetEnvironment returns the current environment
func GetEnvironment() string {
	env, _, _ := resolveEnvironmentAndStage()
	if env == "" {
		return "development"
	}
	return env
}

// GetS3Bucket returns the S3 bucket name
func GetS3Bucket() string {
	return resolveEnvFirst("S3_BUCKET_NAME", "S3_BUCKET", "S3_MEDIA_BUCKET", "MEDIA_BUCKET_NAME")
}

// GetPrivateKeySecret returns the private key secret name
func GetPrivateKeySecret() string {
	if secret := strings.TrimSpace(os.Getenv("PRIVATE_KEY_SECRET")); secret != "" {
		return secret
	}
	return strings.TrimSpace(os.Getenv("PRIVATE_KEY_SECRET_ARN"))
}

// GetDomainName returns the domain name
func GetDomainName() string {
	return resolveEnvFirst("DOMAIN_NAME", "DOMAIN")
}

// GetDynamoTableName returns the DynamoDB table name
func GetDynamoTableName() string {
	return GetMainTableName()
}

// GetMainTableName returns the canonical DynamoDB table name for the current stage/environment.
func GetMainTableName() string {
	return resolveDynamoTableName()
}

// GetStreamEventsTableName returns the DynamoDB table used for SSE stream event log storage.
func GetStreamEventsTableName() string {
	return resolveEnvFirst("STREAM_EVENTS_TABLE_NAME")
}
