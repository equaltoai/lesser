// Package config provides application configuration management for Lesser's serverless deployment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	// Instance configuration
	Domain       string // e.g., "example.com"
	InstanceName string // e.g., "My ActivityPub Server"

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
	KMSKeyID             string // AWS KMS key ID for encryption (optional)
	ReputationPrivateKey string // Private key for reputation system
	VAPIDPublicKey       string // VAPID public key for push notifications
	AdminUsername        string // Admin username for privileged operations
	SystemActorPublicKey string // System actor public key for recovery federation

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
	MaxUploadSize     int64 // Maximum file upload size in bytes
	PageSize          int   // Default pagination size
	AllowRegistration bool  // Whether new users can register

	// Moderation Features
	DisableAWSModeration bool // Master switch to disable all AWS moderation services
	DisableComprehend    bool // Disable AWS Comprehend text analysis
	DisableRekognition   bool // Disable AWS Rekognition image/video analysis

	// Development & Debug Features
	DisableMetrics                bool   // Disable metrics collection
	DisableCostTracking           bool   // Disable cost tracking
	DisableRateLimiting           bool   // Disable rate limiting
	DisableFederationRateLimiting bool   // Disable federation-specific rate limiting
	DisableAI                     bool   // Disable AI features
	EnablePlayground              bool   // Enable GraphQL playground
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
	AlertEmail                string // Email address for alerts
	WebSocketEndpoint         string // WebSocket endpoint for real-time updates
	NotificationRetryQueueURL string // Notification retry queue URL

	// WebSocket & Streaming
	ConnectionsTable   string // DynamoDB table for WebSocket connections
	SubscriptionsTable string // DynamoDB table for subscriptions
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
	ImportQueueURL       string // Import processing queue URL
	ExportQueueURL       string // Export generation queue URL
	MediaQueueURL        string // Media processing queue URL
	ScheduledQueueURL    string // Scheduled publishing queue URL
	FederationQueueURL   string // Federation queue URL
	ImportExportQueueURL string // Import/export processing queue URL

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
}

var config *Config

// Get returns the current configuration
func Get() *Config {
	if config == nil {
		config = loadConfig()
	}
	return config
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	cfg := &Config{
		Domain:       getEnvOrDefault("DOMAIN", "localhost"),
		InstanceName: getEnvOrDefault("INSTANCE_NAME", "Lesser ActivityPub Server"),

		Region:                  getEnvOrDefault("AWS_REGION", "us-east-1"),
		DynamoTableName:         getEnvOrDefault("DYNAMO_TABLE_NAME", "lesser-main"),
		DynamoDBEndpoint:        getEnvOrDefault("DYNAMODB_ENDPOINT", ""),
		S3BucketName:            getEnvOrDefault("S3_BUCKET_NAME", "lesser-media"),
		SQSQueueURL:             getEnvOrDefault("SQS_QUEUE_URL", ""),
		ExportProcessorQueueURL: getEnvOrDefault("EXPORT_PROCESSOR_QUEUE_URL", ""),
		ImportProcessorQueueURL: getEnvOrDefault("IMPORT_PROCESSOR_QUEUE_URL", ""),
		MediaProcessorQueueURL:  getEnvOrDefault("MEDIA_PROCESSOR_QUEUE_URL", ""),
		EnhancedRetryQueueURL:   getEnvOrDefault("ENHANCED_RETRY_QUEUE_URL", ""),
		ReputationTableName:     getEnvOrDefault("REPUTATION_TABLE_NAME", "lesser-reputation"),
		AWSAccountID:            getEnvOrDefault("AWS_ACCOUNT_ID", ""),

		JWTSecret:            getEnvOrPanic("JWT_SECRET"),
		KMSKeyID:             getEnvOrDefault("KMS_KEY_ID", ""), // Optional - defaults to AWS managed key
		ReputationPrivateKey: getEnvOrDefault("REPUTATION_PRIVATE_KEY", ""),
		VAPIDPublicKey:       getEnvOrDefault("VAPID_PUBLIC_KEY", ""),
		AdminUsername:        getEnvOrDefault("ADMIN_USERNAME", ""),
		SystemActorPublicKey: getEnvOrDefault("SYSTEM_ACTOR_PUBLIC_KEY", ""),

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

		MaxUploadSize:     getEnvAsInt64OrDefault("MAX_UPLOAD_SIZE", 10*1024*1024), // 10MB default
		PageSize:          getEnvAsIntOrDefault("PAGE_SIZE", 20),
		AllowRegistration: getEnvAsBoolOrDefault("ALLOW_REGISTRATION", false),

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
		AlertEmail:                getEnvOrDefault("ALERT_EMAIL", ""),
		WebSocketEndpoint:         getEnvOrDefault("WEBSOCKET_ENDPOINT", ""),
		NotificationRetryQueueURL: getEnvOrDefault("NOTIFICATION_RETRY_QUEUE_URL", ""),

		// WebSocket & Streaming
		ConnectionsTable:   getEnvOrDefault("CONNECTIONS_TABLE", ""),
		SubscriptionsTable: getEnvOrDefault("SUBSCRIPTIONS_TABLE", ""),
		IdleTimeoutMinutes: getEnvAsIntOrDefault("IDLE_TIMEOUT_MINUTES", 30),
		StaleTimeoutHours:  getEnvAsIntOrDefault("STALE_TIMEOUT_HOURS", 24),

		// Budget & Cost Management
		BudgetAlertWebhookURL:  getEnvOrDefault("BUDGET_ALERT_WEBHOOK_URL", ""),
		BudgetAlertSNSTopicArn: getEnvOrDefault("BUDGET_ALERT_SNS_TOPIC_ARN", ""),
		CostHistoryTableName:   getEnvOrDefault("COST_HISTORY_TABLE_NAME", ""),

		// Notification System
		NotificationDLQURL:       getEnvOrDefault("NOTIFICATION_DLQ_URL", ""),
		PushNotificationTopicArn: getEnvOrDefault("PUSH_NOTIFICATION_TOPIC_ARN", ""),
		PushNotificationQueueURL: getEnvOrDefault("PUSH_NOTIFICATION_QUEUE_URL", ""),

		// Dead Letter Queue Configuration
		DLQEnabled:         getEnvAsBoolOrDefault("DLQ_ENABLED", false),
		DLQMaxRetries:      getEnvAsIntOrDefault("DLQ_MAX_RETRIES", 3),
		DLQRetryDelay:      getEnvAsIntOrDefault("DLQ_RETRY_DELAY", 60),
		DLQFailFast:        getEnvAsBoolOrDefault("DLQ_FAIL_FAST", false),
		DLQPermanentErrors: getEnvAsStringSliceOrDefault("DLQ_PERMANENT_ERRORS", []string{}),
		DLQTransientErrors: getEnvAsStringSliceOrDefault("DLQ_TRANSIENT_ERRORS", []string{}),

		// Queue URLs for job processing
		ImportQueueURL:       getEnvOrDefault("IMPORT_QUEUE_URL", ""),
		ExportQueueURL:       getEnvOrDefault("EXPORT_QUEUE_URL", ""),
		MediaQueueURL:        getEnvOrDefault("MEDIA_QUEUE_URL", ""),
		ScheduledQueueURL:    getEnvOrDefault("SCHEDULED_QUEUE_URL", ""),
		FederationQueueURL:   getEnvOrDefault("FEDERATION_DELIVERY_QUEUE_URL", ""),
		ImportExportQueueURL: getEnvOrDefault("IMPORT_EXPORT_QUEUE_URL", ""),

		// Media & Storage
		MediaBucketName:           getEnvOrDefault("MEDIA_BUCKET_NAME", ""),
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

		Stage:   getEnvOrDefault("STAGE", "dev"),
		Version: getEnvOrDefault("VERSION", "v1.0.0"),

		// Service identification
		Environment:         getEnvOrDefault("ENVIRONMENT", "development"),
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

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Required environment variable %s is not set", key))
	}
	return value
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

// Helper functions for health checks

// GetEnvironment returns the current environment
func GetEnvironment() string {
	return getEnvOrDefault("ENVIRONMENT", "development")
}

// GetS3Bucket returns the S3 bucket name
func GetS3Bucket() string {
	// Check multiple possible environment variables
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("S3_BUCKET_NAME")
	}
	if bucket == "" {
		bucket = os.Getenv("MEDIA_BUCKET_NAME")
	}
	if bucket == "" {
		bucket = os.Getenv("S3_MEDIA_BUCKET")
	}
	return bucket
}

// GetPrivateKeySecret returns the private key secret name
func GetPrivateKeySecret() string {
	return os.Getenv("PRIVATE_KEY_SECRET")
}

// GetDomainName returns the domain name
func GetDomainName() string {
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = os.Getenv("DOMAIN")
	}
	return domain
}

// GetDynamoTableName returns the DynamoDB table name
func GetDynamoTableName() string {
	table := os.Getenv("DYNAMODB_TABLE")
	if table == "" {
		table = os.Getenv("DYNAMO_TABLE_NAME")
	}
	return table
}
