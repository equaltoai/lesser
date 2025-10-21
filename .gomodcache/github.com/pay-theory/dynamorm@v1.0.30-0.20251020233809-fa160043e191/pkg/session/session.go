// Package session provides AWS session management and DynamoDB client configuration
package session

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// configLoadFunc is a variable to allow mocking config.LoadDefaultConfig in tests
var configLoadFunc = config.LoadDefaultConfig

// Config holds the configuration for DynamORM
type Config struct {
	// AWS region
	Region string

	// Optional endpoint for local development (e.g., DynamoDB Local)
	Endpoint string

	// Maximum number of retries for failed requests
	MaxRetries int

	// Default read capacity units for table creation
	DefaultRCU int64

	// Default write capacity units for table creation
	DefaultWCU int64

	// Whether to automatically create tables if they don't exist
	AutoMigrate bool

	// Whether to enable metrics collection
	EnableMetrics bool

	// Custom AWS config options
	AWSConfigOptions []func(*config.LoadOptions) error

	// Custom DynamoDB client options
	DynamoDBOptions []func(*dynamodb.Options)

	// Credentials provider
	CredentialsProvider aws.CredentialsProvider
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Region:        "us-east-1",
		MaxRetries:    3,
		DefaultRCU:    5,
		DefaultWCU:    5,
		AutoMigrate:   false,
		EnableMetrics: false,
	}
}

// Session manages the AWS session and DynamoDB client
type Session struct {
	config    *Config
	awsConfig aws.Config
	client    *dynamodb.Client
}

// NewSession creates a new session with the given configuration
func NewSession(cfg *Config) (*Session, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Build AWS config options
	options := make([]func(*config.LoadOptions) error, 0, len(cfg.AWSConfigOptions)+5)

	// Add region if specified
	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	// Add credentials provider if specified
	if cfg.CredentialsProvider != nil {
		options = append(options, config.WithCredentialsProvider(cfg.CredentialsProvider))
	}

	// Add retry configuration
	maxAttempts := cfg.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 3 // Default
	}
	options = append(options, config.WithRetryMode(aws.RetryModeStandard))
	options = append(options, config.WithRetryMaxAttempts(maxAttempts))

	// Add HTTP client
	options = append(options, config.WithHTTPClient(&http.Client{}))

	// Add custom options
	options = append(options, cfg.AWSConfigOptions...)

	// Load AWS config
	awsConfig, err := configLoadFunc(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Ensure we have a valid retryer
	if awsConfig.Retryer == nil {
		awsConfig.Retryer = func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = maxAttempts
			})
		}
	}

	// Create DynamoDB client options
	clientOptions := []func(*dynamodb.Options){
		func(o *dynamodb.Options) {
			o.Region = awsConfig.Region

			// Apply endpoint override if specified
			if cfg.Endpoint != "" {
				o.BaseEndpoint = aws.String(cfg.Endpoint)
			}

			// Ensure retryer is set
			if o.Retryer == nil {
				o.Retryer = awsConfig.Retryer()
			}

			// Ensure HTTP client is set
			if o.HTTPClient == nil {
				o.HTTPClient = &http.Client{}
			}
		},
	}

	// Add custom DynamoDB options
	clientOptions = append(clientOptions, cfg.DynamoDBOptions...)

	// Create client with options
	client := dynamodb.NewFromConfig(awsConfig, clientOptions...)

	// Ensure client is not nil
	if client == nil {
		return nil, fmt.Errorf("failed to create DynamoDB client")
	}

	return &Session{
		config:    cfg,
		awsConfig: awsConfig,
		client:    client,
	}, nil
}

// Client returns the DynamoDB client
func (s *Session) Client() (*dynamodb.Client, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	if s.client == nil {
		return nil, fmt.Errorf("DynamoDB client is nil")
	}
	return s.client, nil
}

// Config returns the session configuration
func (s *Session) Config() *Config {
	return s.config
}

// AWSConfig returns the AWS configuration
func (s *Session) AWSConfig() aws.Config {
	return s.awsConfig
}

// WithContext returns a new session with the given context
func (s *Session) WithContext(ctx context.Context) *Session {
	// DynamoDB client operations accept context at the operation level
	// This method is here for consistency with the DB interface
	return s
}
