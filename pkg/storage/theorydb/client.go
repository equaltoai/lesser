package theorydb

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
	tabletypes "github.com/theory-cloud/tabletheory/pkg/types"
	"go.uber.org/zap"
)

var (
	// Global client instance for reuse across Lambda invocations
	client     core.DB
	lambdaDB   *tabletheory.LambdaDB
	clientOnce sync.Once
	clientErr  error

	// Default timeout buffer to prevent Lambda timeouts
	// This is subtracted from the Lambda function timeout
	defaultTimeoutBuffer = 500 * time.Millisecond

	getAppConfig               = config.Get
	newDynamormStandardClient  = func(cfg session.Config) (core.DB, error) { return tabletheory.New(cfg) }
	newDynamormLambdaOptimized = tabletheory.NewLambdaOptimized
)

type typeConverterRegistrar interface {
	RegisterTypeConverter(reflect.Type, tabletypes.CustomConverter) error
}

func registerDefaultTypeConverters(db core.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	registrar, ok := db.(typeConverterRegistrar)
	if !ok {
		// Non-fatal: tests may use mocked core.DB without converter support.
		return nil
	}

	if err := registrar.RegisterTypeConverter(mapStringAnyType, mapStringAnyConverter{}); err != nil {
		return fmt.Errorf("register map[string]any converter: %w", err)
	}
	if err := registrar.RegisterTypeConverter(sliceAnyType, sliceAnyConverter{}); err != nil {
		return fmt.Errorf("register []any converter: %w", err)
	}
	if err := registrar.RegisterTypeConverter(activityPubNoteType, activityPubNoteConverter{}); err != nil {
		return fmt.Errorf("register activitypub.Note converter: %w", err)
	}
	if err := registrar.RegisterTypeConverter(activityPubContextValueType, activityPubContextValueConverter{}); err != nil {
		return fmt.Errorf("register activitypub.ContextValue converter: %w", err)
	}
	if err := registrar.RegisterTypeConverter(agentsCapabilitiesType, agentCapabilitiesConverter{}); err != nil {
		return fmt.Errorf("register agents.Capabilities converter: %w", err)
	}

	return nil
}

// RegisterDefaultTypeConverters applies Lesser's custom TheoryDB converters to an
// existing client. CLI tools that open ad hoc clients should call this so live
// verification uses the same model encoding rules as production handlers.
func RegisterDefaultTypeConverters(db core.DB) error {
	return registerDefaultTypeConverters(db)
}

// GetClient returns a singleton DynamORM client instance
// This ensures that the client is only initialized once per Lambda container
func GetClient(_ context.Context) (core.DB, error) {
	clientOnce.Do(func() {
		cfg := getAppConfig()
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
		client, clientErr = newDynamormStandardClient(sessionConfig)
		if clientErr != nil {
			return
		}
		if err := registerDefaultTypeConverters(client); err != nil {
			clientErr = err
		}
	})

	return client, clientErr
}

func getLambdaOptimizedClient() (*tabletheory.LambdaDB, error) {
	clientOnce.Do(func() {
		zap.L().Info("initializing Lambda-optimized DynamORM client")
		startTime := time.Now()

		// Create Lambda-optimized client
		var err error
		lambdaDB, err = newDynamormLambdaOptimized()
		if err != nil {
			clientErr = err
			zap.L().Error("failed to initialize DynamORM", zap.Error(err))
			return
		}
		if lambdaDB == nil {
			clientErr = fmt.Errorf("initialize Lambda-optimized DynamORM: nil client")
			zap.L().Error("failed to initialize DynamORM", zap.Error(clientErr))
			return
		}

		// Store the standard client interface for compatibility
		lambdaDB = lambdaDB.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
			Buffer: defaultTimeoutBuffer,
		})
		if err := registerDefaultTypeConverters(lambdaDB); err != nil {
			clientErr = err
			zap.L().Error("failed to register type converters", zap.Error(err))
			return
		}
		client = lambdaDB

		zap.L().Info("DynamORM initialized", zap.Duration("duration", time.Since(startTime)))
	})

	return lambdaDB, clientErr
}

// GetLambdaClient returns a singleton DynamORM Lambda-optimized DB client instance.
// This ensures that the client is only initialized once per Lambda container
// and includes Lambda-specific optimizations like timeout handling. The returned
// core.DB preserves lesser's configured Lambda timeout safety buffer before
// applying the caller's Lambda context deadline.
func GetLambdaClient(ctx context.Context) (core.DB, error) {
	lambdaClient, err := getLambdaOptimizedClient()
	if err != nil {
		return nil, err
	}
	if client == nil {
		return lambdaClient, clientErr
	}

	// Apply Lambda context timeout if available
	if ctx != nil {
		return lambdaClient.WithLambdaTimeout(ctx), clientErr
	}

	return client, clientErr
}

// InitializeModels pre-registers models with the DynamORM client to reduce cold start time
// This should be called in the init() function of Lambda handlers
func InitializeModels(models ...any) error {
	db, err := getLambdaOptimizedClient()
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
	lambdaDB, ok := db.(*tabletheory.LambdaDB)
	if !ok {
		// If not a LambdaDB, return the original DB
		return db
	}

	return lambdaDB.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
		Buffer: buffer,
	})
}
