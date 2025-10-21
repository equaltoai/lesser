package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/pay-theory/lift/pkg/observability"
	"github.com/pay-theory/lift/pkg/observability/cloudwatch"
)

func main() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create AWS service clients
	cwClient := cloudwatchlogs.NewFromConfig(cfg)
	snsClient := sns.NewFromConfig(cfg)

	// Configure CloudWatch logger
	loggerConfig := observability.LoggerConfig{
		LogGroup:      fmt.Sprintf("/aws/lambda/my-service-%s-%s", os.Getenv("PARTNER"), os.Getenv("STAGE")),
		LogStream:     fmt.Sprintf("main-%d", time.Now().Unix()),
		BatchSize:     10,
		FlushInterval: 2 * time.Second,
		BufferSize:    50,
		Level:         "info",
		Format:        "json",
	}

	// Example 1: Using the helper function with default SNS topic
	logger1, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
		cloudwatch.WithDefaultErrorNotifications(snsClient))
	if err != nil {
		log.Fatalf("Failed to create CloudWatch logger: %v", err)
	}
	defer func() {
		if closeErr := logger1.Close(); closeErr != nil {
			log.Printf("Error closing logger1: %v", closeErr)
		}
	}()

	// Example 2: Using the helper function with custom SNS topic
	customTopicARN := "arn:aws:sns:us-west-2:123456789012:my-custom-alerts"
	logger2, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
		cloudwatch.WithErrorNotifications(snsClient, customTopicARN))
	if err != nil {
		log.Fatalf("Failed to create CloudWatch logger: %v", err) //nolint:gocritic // defer not needed if creation fails
	}
	defer func() {
		if closeErr := logger2.Close(); closeErr != nil {
			log.Printf("Error closing logger2: %v", closeErr)
		}
	}()

	// Example 3: Creating SNS notifier separately for more control
	defaultTopicARN := fmt.Sprintf("arn:aws:sns:%s:%s:cns-%s-%s",
		os.Getenv("AWS_REGION"),
		os.Getenv("AWS_ACCOUNT_ID"),
		os.Getenv("PARTNER"),
		os.Getenv("STAGE"))
	snsConfig := observability.SNSConfig{
		Client:   snsClient,
		TopicARN: defaultTopicARN,
	}
	notifier := observability.NewSNSNotifier(snsConfig)

	logger3, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient,
		cloudwatch.CloudWatchLoggerOptions{
			Notifier: notifier,
		})
	if err != nil {
		log.Fatalf("Failed to create CloudWatch logger: %v", err)
	}
	defer func() {
		if closeErr := logger3.Close(); closeErr != nil {
			log.Printf("Error closing logger3: %v", closeErr)
		}
	}()

	// Example 4: CloudWatch logger without SNS notifications
	loggerNoSNS, err := cloudwatch.NewCloudWatchLogger(loggerConfig, cwClient)
	if err != nil {
		log.Fatalf("Failed to create CloudWatch logger: %v", err)
	}
	defer func() {
		if err := loggerNoSNS.Close(); err != nil {
			log.Printf("Error closing loggerNoSNS: %v", err)
		}
	}()

	// Use the logger
	logger := logger1 // Use any of the configured loggers

	// Log various levels
	logger.Info("Application started")
	logger.Debug("Debug information", map[string]any{"component": "main"})
	logger.Warn("This is a warning", map[string]any{"retry_count": 3})

	// Error logs will trigger SNS notifications if configured
	logger.Error("Critical error occurred", map[string]any{
		"error_type":     "database_connection",
		"retry_attempts": 5,
	})

	// Flush logs before exiting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := logger.Flush(ctx); err != nil {
		log.Printf("Failed to flush logs: %v", err)
	}

	fmt.Println("CloudWatch logging with SNS notifications example completed")
}
