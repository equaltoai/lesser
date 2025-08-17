// Package awsinit provides standardized AWS service initialization for Lambda functions.
package awsinit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.uber.org/zap"
)

// ServiceConfig defines which AWS services to initialize
type ServiceConfig struct {
	// Service flags - only initialize what's needed
	RequiresDynamoDB     bool
	RequiresS3           bool
	RequiresSQS          bool
	RequiresCloudWatch   bool
	RequiresSecretsManager bool
	RequiresComprehend   bool
	RequiresMediaConvert bool
	RequiresSNS          bool
	RequiresLambda       bool
	
	// Configuration
	Region          string
	ServiceName     string
	RequestTimeout  time.Duration
	RetryMaxAttempts int
}

// AWSServices contains initialized AWS service clients
type AWSServices struct {
	Config           aws.Config
	DynamoDB         *dynamodb.Client
	S3               *s3.Client
	SQS              *sqs.Client
	CloudWatch       *cloudwatch.Client
	SecretsManager   *secretsmanager.Client
	Comprehend       *comprehend.Client
	MediaConvert     *mediaconvert.Client
	SNS              *sns.Client
	Lambda           *awslambda.Client
	Logger           *zap.Logger
}

// InitializeServices creates AWS service clients based on configuration
func InitializeServices(ctx context.Context, serviceConfig ServiceConfig, logger *zap.Logger) (*AWSServices, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set defaults
	if serviceConfig.Region == "" {
		serviceConfig.Region = os.Getenv("AWS_REGION")
		if serviceConfig.Region == "" {
			serviceConfig.Region = "us-east-1"
		}
	}
	
	if serviceConfig.RequestTimeout == 0 {
		serviceConfig.RequestTimeout = 30 * time.Second
	}
	
	if serviceConfig.RetryMaxAttempts == 0 {
		serviceConfig.RetryMaxAttempts = 3
	}

	start := time.Now()
	logger.Info("initializing AWS services",
		zap.String("service", serviceConfig.ServiceName),
		zap.String("region", serviceConfig.Region),
		zap.Duration("timeout", serviceConfig.RequestTimeout),
		zap.Int("retry_attempts", serviceConfig.RetryMaxAttempts))

	// Load AWS configuration with standardized options
	awsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(serviceConfig.Region),
		config.WithRetryMaxAttempts(serviceConfig.RetryMaxAttempts),
		config.WithHTTPClient(createHTTPClient(serviceConfig.RequestTimeout)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	services := &AWSServices{
		Config: awsConfig,
		Logger: logger,
	}

	// Initialize only required services
	if serviceConfig.RequiresDynamoDB {
		services.DynamoDB = dynamodb.NewFromConfig(awsConfig)
		logger.Debug("initialized DynamoDB client")
	}

	if serviceConfig.RequiresS3 {
		services.S3 = s3.NewFromConfig(awsConfig)
		logger.Debug("initialized S3 client")
	}

	if serviceConfig.RequiresSQS {
		services.SQS = sqs.NewFromConfig(awsConfig)
		logger.Debug("initialized SQS client")
	}

	if serviceConfig.RequiresCloudWatch {
		services.CloudWatch = cloudwatch.NewFromConfig(awsConfig)
		logger.Debug("initialized CloudWatch client")
	}

	if serviceConfig.RequiresSecretsManager {
		services.SecretsManager = secretsmanager.NewFromConfig(awsConfig)
		logger.Debug("initialized Secrets Manager client")
	}

	if serviceConfig.RequiresComprehend {
		services.Comprehend = comprehend.NewFromConfig(awsConfig)
		logger.Debug("initialized Comprehend client")
	}

	if serviceConfig.RequiresMediaConvert {
		services.MediaConvert = mediaconvert.NewFromConfig(awsConfig)
		logger.Debug("initialized MediaConvert client")
	}

	if serviceConfig.RequiresSNS {
		services.SNS = sns.NewFromConfig(awsConfig)
		logger.Debug("initialized SNS client")
	}

	if serviceConfig.RequiresLambda {
		services.Lambda = awslambda.NewFromConfig(awsConfig)
		logger.Debug("initialized Lambda client")
	}

	initDuration := time.Since(start)
	logger.Info("AWS services initialization completed",
		zap.String("service", serviceConfig.ServiceName),
		zap.Duration("init_duration", initDuration))

	return services, nil
}

// MustInitializeServices is like InitializeServices but panics on error
func MustInitializeServices(ctx context.Context, serviceConfig ServiceConfig, logger *zap.Logger) *AWSServices {
	services, err := InitializeServices(ctx, serviceConfig, logger)
	if err != nil {
		logger.Fatal("failed to initialize AWS services", zap.Error(err))
	}
	return services
}

// InitializeWithConfig creates services using an existing AWS config
func InitializeWithConfig(awsConfig aws.Config, serviceConfig ServiceConfig, logger *zap.Logger) *AWSServices {
	services := &AWSServices{
		Config: awsConfig,
		Logger: logger,
	}

	// Initialize only required services
	if serviceConfig.RequiresDynamoDB {
		services.DynamoDB = dynamodb.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresS3 {
		services.S3 = s3.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresSQS {
		services.SQS = sqs.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresCloudWatch {
		services.CloudWatch = cloudwatch.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresSecretsManager {
		services.SecretsManager = secretsmanager.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresComprehend {
		services.Comprehend = comprehend.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresMediaConvert {
		services.MediaConvert = mediaconvert.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresSNS {
		services.SNS = sns.NewFromConfig(awsConfig)
	}

	if serviceConfig.RequiresLambda {
		services.Lambda = awslambda.NewFromConfig(awsConfig)
	}

	logger.Info("AWS services initialized from existing config",
		zap.String("service", serviceConfig.ServiceName))

	return services
}

// Common service configurations for different Lambda types

// APIServiceConfig returns config for API Lambda functions
func APIServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           true,
		RequiresCloudWatch:   true,
		RequiresSecretsManager: false,
		RequiresComprehend:   false,
		RequiresMediaConvert: false,
		RequiresSQS:          false,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "api",
		RequestTimeout:       30 * time.Second,
		RetryMaxAttempts:     3,
	}
}

// ProcessorServiceConfig returns config for processor Lambda functions
func ProcessorServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           false,
		RequiresCloudWatch:   true,
		RequiresSecretsManager: false,
		RequiresComprehend:   false,
		RequiresMediaConvert: false,
		RequiresSQS:          true,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "processor",
		RequestTimeout:       30 * time.Second,
		RetryMaxAttempts:     3,
	}
}

// MediaServiceConfig returns config for media processing Lambda functions
func MediaServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           true,
		RequiresCloudWatch:   true,
		RequiresSecretsManager: false,
		RequiresComprehend:   false,
		RequiresMediaConvert: true,
		RequiresSQS:          true,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "media",
		RequestTimeout:       5 * time.Minute, // Media processing takes longer
		RetryMaxAttempts:     3,
	}
}

// FederationServiceConfig returns config for federation Lambda functions
func FederationServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           false,
		RequiresCloudWatch:   true,
		RequiresSecretsManager: false,
		RequiresComprehend:   false,
		RequiresMediaConvert: false,
		RequiresSQS:          true,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "federation",
		RequestTimeout:       30 * time.Second,
		RetryMaxAttempts:     3,
	}
}

// AIServiceConfig returns config for AI processing Lambda functions
func AIServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           true,
		RequiresCloudWatch:   true,
		RequiresSecretsManager: false,
		RequiresComprehend:   true,
		RequiresMediaConvert: false,
		RequiresSQS:          true,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "ai",
		RequestTimeout:       2 * time.Minute, // AI processing takes longer
		RetryMaxAttempts:     3,
	}
}

// BasicServiceConfig returns a minimal config for simple Lambda functions
func BasicServiceConfig() ServiceConfig {
	return ServiceConfig{
		RequiresDynamoDB:     true,
		RequiresS3:           false,
		RequiresCloudWatch:   false,
		RequiresSecretsManager: false,
		RequiresComprehend:   false,
		RequiresMediaConvert: false,
		RequiresSQS:          false,
		RequiresSNS:          false,
		RequiresLambda:       false,
		ServiceName:          "basic",
		RequestTimeout:       30 * time.Second,
		RetryMaxAttempts:     3,
	}
}

// createHTTPClient creates a customized HTTP client for AWS services
func createHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}