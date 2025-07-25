package dynamorm

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
)

var (
	// Global client instance for reuse across Lambda invocations
	client     core.DB
	lambdaDB   *dynamorm.LambdaDB
	clientOnce sync.Once
	clientErr  error

	// Default timeout buffer to prevent Lambda timeouts
	// This is subtracted from the Lambda function timeout
	defaultTimeoutBuffer = 500 * time.Millisecond
)

// GetClient returns a singleton DynamORM client instance
// This ensures that the client is only initialized once per Lambda container
func GetClient(ctx context.Context) (core.DB, error) {
	clientOnce.Do(func() {
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1" // Default region
		}

		config := session.Config{
			Region: region,
		}

		// For local development, check for local DynamoDB endpoint
		endpoint := os.Getenv("DYNAMODB_ENDPOINT")
		if endpoint != "" {
			config.Endpoint = endpoint
			// Use fake credentials for local development
			if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
				// Set credentials using environment variables instead
				os.Setenv("AWS_ACCESS_KEY_ID", "fakeMyKeyId")
				os.Setenv("AWS_SECRET_ACCESS_KEY", "fakeSecretAccessKey")
			}
		}

		// Initialize with standard client creation
		client, clientErr = dynamorm.New(config)
	})

	return client, clientErr
}

// GetLambdaClient returns a singleton DynamORM Lambda-optimized client instance
// This ensures that the client is only initialized once per Lambda container
// and includes Lambda-specific optimizations like timeout handling
func GetLambdaClient(ctx context.Context) (*dynamorm.LambdaDB, error) {
	clientOnce.Do(func() {
		log.Println("Initializing Lambda-optimized DynamORM client...")
		startTime := time.Now()

		// Create Lambda-optimized client
		var err error
		lambdaDB, err = dynamorm.NewLambdaOptimized()
		if err != nil {
			clientErr = err
			log.Printf("Failed to initialize DynamORM: %v", err)
			return
		}

		// Store the standard client interface for compatibility
		client = lambdaDB.WithLambdaTimeoutBuffer(defaultTimeoutBuffer)

		log.Printf("DynamORM initialized in %v", time.Since(startTime))
	})

	// Apply Lambda context timeout if available
	if ctx != nil && lambdaDB != nil {
		return lambdaDB.WithLambdaTimeout(ctx), clientErr
	}

	return lambdaDB, clientErr
}

// InitializeModels pre-registers models with the DynamORM client to reduce cold start time
// This should be called in the init() function of Lambda handlers
func InitializeModels(models ...any) error {
	db, err := GetLambdaClient(context.Background())
	if err != nil {
		return err
	}

	// Pre-register models to reduce cold start time
	return db.PreRegisterModels(models...)
}

// WithTimeoutBuffer returns a new client with the specified timeout buffer
func WithTimeoutBuffer(db core.DB, buffer time.Duration) core.DB {
	if db == nil {
		return nil
	}

	// Type assertion to get the LambdaDB
	lambdaDB, ok := db.(*dynamorm.LambdaDB)
	if !ok {
		// If not a LambdaDB, return the original DB
		return db
	}

	return lambdaDB.WithLambdaTimeoutBuffer(buffer)
}
