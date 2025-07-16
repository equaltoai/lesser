package dynamorm

import (
	"context"
	"os"
	"sync"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
)

var (
	// Global client instance for reuse across Lambda invocations
	client     core.DB
	clientOnce sync.Once
	clientErr  error
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
