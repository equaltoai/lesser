// Package config provides application configuration management for Lesser's serverless deployment.
package config

import (
	"fmt"
	"os"
)

// Config holds the application configuration
type Config struct {
	// Instance configuration
	Domain       string // e.g., "example.com"
	InstanceName string // e.g., "My ActivityPub Server"

	// AWS configuration
	Region                     string
	DynamoTableName            string
	S3BucketName               string
	SQSQueueURL                string
	FederationDeliveryQueueURL string // Federation delivery SQS queue URL
	ReputationTableName        string // For reputation/vouch storage
	AWSAccountID               string // AWS Account ID
	ExportProcessorQueueURL    string // Export processor SQS queue URL
	ImportProcessorQueueURL    string // Import processor SQS queue URL
	MediaProcessorQueueURL     string // Media processor SQS queue URL

	// Security
	JWTSecret            string // For client authentication
	KMSKeyID             string // AWS KMS key ID for encryption (optional)
	ReputationPrivateKey string // Private key for reputation system

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

	// Deployment info
	Stage   string // Deployment stage (dev, staging, prod)
	Version string // Application version
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

		Region:                     getEnvOrDefault("AWS_REGION", "us-east-1"),
		DynamoTableName:            getEnvOrDefault("DYNAMO_TABLE_NAME", "lesser-main"),
		S3BucketName:               getEnvOrDefault("S3_BUCKET_NAME", "lesser-media"),
		SQSQueueURL:                getEnvOrDefault("SQS_QUEUE_URL", ""),
		FederationDeliveryQueueURL: getEnvOrDefault("FEDERATION_DELIVERY_QUEUE_URL", ""),
		ExportProcessorQueueURL:    getEnvOrDefault("EXPORT_PROCESSOR_QUEUE_URL", ""),
		ImportProcessorQueueURL:    getEnvOrDefault("IMPORT_PROCESSOR_QUEUE_URL", ""),
		MediaProcessorQueueURL:     getEnvOrDefault("MEDIA_PROCESSOR_QUEUE_URL", ""),
		ReputationTableName:        getEnvOrDefault("REPUTATION_TABLE_NAME", "lesser-reputation"),
		AWSAccountID:               getEnvOrDefault("AWS_ACCOUNT_ID", ""),

		JWTSecret:            getEnvOrPanic("JWT_SECRET"),
		KMSKeyID:             getEnvOrDefault("KMS_KEY_ID", ""), // Optional - defaults to AWS managed key
		ReputationPrivateKey: getEnvOrDefault("REPUTATION_PRIVATE_KEY", ""),

		MaxUploadSize:     getEnvAsInt64OrDefault("MAX_UPLOAD_SIZE", 10*1024*1024), // 10MB default
		PageSize:          getEnvAsIntOrDefault("PAGE_SIZE", 20),
		AllowRegistration: getEnvAsBoolOrDefault("ALLOW_REGISTRATION", false),

		// Moderation flags - default to false (AWS enabled by default)
		DisableAWSModeration: getEnvAsBoolOrDefault("DISABLE_AWS_MODERATION", false),
		DisableComprehend:    getEnvAsBoolOrDefault("DISABLE_COMPREHEND", false),
		DisableRekognition:   getEnvAsBoolOrDefault("DISABLE_REKOGNITION", false),

		Stage:   getEnvOrDefault("STAGE", "dev"),
		Version: getEnvOrDefault("VERSION", "v1.0.0"),
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
