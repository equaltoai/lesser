package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"github.com/pay-theory/lift/pkg/observability"
	"github.com/pay-theory/lift/pkg/observability/zap"
)

func main() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create SNS client
	snsClient := sns.NewFromConfig(cfg)

	// Configure Zap logger with SNS notifications
	loggerConfig := observability.LoggerConfig{
		Level:  "info",
		Format: "json", // JSON format for structured logs in CloudWatch
	}

	// Create Zap logger with default SNS error notifications
	logger, err := zap.NewZapLogger(loggerConfig, zap.WithDefaultErrorNotifications(snsClient))
	if err != nil {
		log.Fatalf("Failed to create Zap logger: %v", err)
	}

	// Create Lift app with the logger
	app := lift.New()
	app.WithLogger(logger)

	// Set up defer after app is created to ensure cleanup happens
	defer func() {
		if err := logger.Close(); err != nil {
			log.Printf("Error closing logger: %v", err)
		}
	}()

	// Add observability middleware with the logger
	app.Use(middleware.ObservabilityMiddleware(middleware.ObservabilityConfig{
		Logger: logger,
	}))

	// Example route that logs at different levels
	if err := app.GET("/test", func(ctx *lift.Context) error {
		// Info log - will appear in CloudWatch Logs
		ctx.Logger.Info("Test endpoint called", map[string]any{
			"method": ctx.Request.Method,
			"path":   ctx.Request.Path,
		})

		// Simulate an error - will trigger SNS notification
		if ctx.Query("error") == "true" {
			ctx.Logger.Error("Simulated error occurred", map[string]any{
				"error_type": "TEST_ERROR",
				"details":    "This is a test error to demonstrate SNS notifications",
			})
			return ctx.Status(500).JSON(map[string]string{
				"error": "Simulated error",
			})
		}

		return ctx.JSON(map[string]string{
			"message": "Success",
			"logger":  "zap-with-sns",
		})
	}); err != nil {
		logger.Error("Failed to register GET /test", map[string]any{"error": err})
		return
	}

	// Alternative: Use custom SNS topic
	if err := app.GET("/custom", func(ctx *lift.Context) error {
		// Example with custom SNS topic ARN
		customTopicARN := fmt.Sprintf("arn:aws:sns:%s:%s:my-custom-alerts",
			os.Getenv("AWS_REGION"),
			os.Getenv("AWS_ACCOUNT_ID"))

		customLogger, err := zap.NewZapLogger(loggerConfig,
			zap.WithErrorNotifications(snsClient, customTopicARN))
		if err != nil {
			log.Printf("Failed to create custom logger: %v", err)
			return ctx.Status(500).JSON(map[string]string{
				"error": "Failed to create custom logger",
			})
		}

		customLogger.Error("Error with custom SNS topic", map[string]any{
			"topic": customTopicARN,
		})

		return ctx.JSON(map[string]string{
			"message": "Sent to custom topic",
		})
	}); err != nil {
		logger.Error("Failed to register GET /custom", map[string]any{"error": err})
		return
	}

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}
