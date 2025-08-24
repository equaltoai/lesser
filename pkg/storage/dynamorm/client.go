package dynamorm

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"go.uber.org/zap"
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
func GetClient(_ context.Context) (core.DB, error) {
	clientOnce.Do(func() {
		cfg := config.Get()
		region := cfg.Region
		if err := common.ValidateRequiredParam("region", region); err != nil {
			region = "us-east-1" // Default region
		}

		sessionConfig := session.Config{
			Region: region,
		}

		// For local development, check for local DynamoDB endpoint from centralized config
		if cfg.DynamoDBEndpoint != "" {
			sessionConfig.Endpoint = cfg.DynamoDBEndpoint
			// Use fake credentials for local development
			if err := common.ValidateRequiredParam("AWS_ACCESS_KEY_ID", os.Getenv("AWS_ACCESS_KEY_ID")); err != nil {
				// Set credentials using environment variables instead
				_ = os.Setenv("AWS_ACCESS_KEY_ID", "fakeMyKeyId")
				_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "fakeSecretAccessKey")
			}
		}

		// Initialize with standard client creation
		client, clientErr = dynamorm.New(sessionConfig)
	})

	return client, clientErr
}

// GetLambdaClient returns a singleton DynamORM Lambda-optimized client instance
// This ensures that the client is only initialized once per Lambda container
// and includes Lambda-specific optimizations like timeout handling
func GetLambdaClient(ctx context.Context) (*dynamorm.LambdaDB, error) {
	clientOnce.Do(func() {
		zap.L().Info("initializing Lambda-optimized DynamORM client")
		startTime := time.Now()

		// Create Lambda-optimized client
		var err error
		lambdaDB, err = dynamorm.NewLambdaOptimized()
		if err != nil {
			clientErr = err
			zap.L().Error("failed to initialize DynamORM", zap.Error(err))
			return
		}

		// Store the standard client interface for compatibility
		client = lambdaDB.WithLambdaTimeoutBuffer(defaultTimeoutBuffer)

		zap.L().Info("DynamORM initialized", zap.Duration("duration", time.Since(startTime)))
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
