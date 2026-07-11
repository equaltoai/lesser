package theorydb

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	tabletypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
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

	lambdaClientEnvMu                 sync.Mutex
	newDynamormLambdaOptimizedWithEnv = func(opts lambdaOptimizedClientOptions) (*tabletheory.LambdaDB, error) {
		lambdaClientEnvMu.Lock()
		defer lambdaClientEnvMu.Unlock()

		return withLambdaOptimizedEnvironment(opts, newDynamormLambdaOptimized)
	}
)

type typeConverterRegistrar interface {
	RegisterTypeConverter(reflect.Type, tabletypes.CustomConverter) error
}

type lambdaOptimizedClientOptions struct {
	Region   string
	Endpoint string
}

func lambdaOptimizedClientOptionsFor(region string) lambdaOptimizedClientOptions {
	opts := lambdaOptimizedClientOptions{
		Region: selectLambdaOptimizedRegion(region),
	}

	if cfg := getAppConfig(); cfg != nil {
		opts.Endpoint = strings.TrimSpace(cfg.DynamoDBEndpoint)
	}

	return opts
}

func selectLambdaOptimizedRegion(region string) string {
	if trimmed := strings.TrimSpace(region); trimmed != "" {
		return trimmed
	}
	if envRegion := strings.TrimSpace(os.Getenv("AWS_REGION")); envRegion != "" {
		return envRegion
	}
	if envDefault := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); envDefault != "" {
		return envDefault
	}
	return "us-east-1"
}

func withLambdaOptimizedEnvironment(
	opts lambdaOptimizedClientOptions,
	factory func() (*tabletheory.LambdaDB, error),
) (*tabletheory.LambdaDB, error) {
	if factory == nil {
		return nil, fmt.Errorf("lambda-optimized DynamORM factory is nil")
	}

	// TableTheory's Lambda helper intentionally reads AWS runtime settings from
	// the environment. Keep lesser's explicit-region and local-endpoint behavior
	// by bridging them through the AWS SDK environment variables only while the
	// Lambda-optimized client is constructed.
	restoreRegion := setTemporaryEnv("AWS_REGION", opts.Region)
	defer restoreRegion()

	restoreEndpoint := func() {}
	if opts.Endpoint != "" {
		ensureLocalDynamoDBCredentials()
		restoreEndpoint = setTemporaryEnv("AWS_ENDPOINT_URL_DYNAMODB", opts.Endpoint)
	}
	defer restoreEndpoint()

	return factory()
}

func setTemporaryEnv(key, value string) func() {
	previous, existed := os.LookupEnv(key)
	_ = os.Setenv(key, value)

	return func() {
		if existed {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	}
}

func ensureLocalDynamoDBCredentials() {
	if strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")) == "" {
		_ = os.Setenv("AWS_ACCESS_KEY_ID", "fakeMyKeyId")
	}
	if strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")) == "" {
		_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "fakeSecretAccessKey")
	}
}

func newConfiguredLambdaOptimizedClient(opts lambdaOptimizedClientOptions) (*tabletheory.LambdaDB, error) {
	lambdaClient, err := newDynamormLambdaOptimizedWithEnv(opts)
	if err != nil {
		return nil, err
	}
	if lambdaClient == nil {
		return nil, fmt.Errorf("initialize Lambda-optimized DynamORM: nil client")
	}

	lambdaClient = lambdaClient.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
		Buffer: defaultTimeoutBuffer,
	})
	if err := registerDefaultTypeConverters(lambdaClient); err != nil {
		return nil, err
	}

	return lambdaClient, nil
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
		lambdaDB, err = newConfiguredLambdaOptimizedClient(lambdaOptimizedClientOptionsFor(""))
		if err != nil {
			clientErr = err
			zap.L().Error("failed to initialize DynamORM", zap.Error(err))
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

type lambdaTimeoutApplier interface {
	WithLambdaTimeout(context.Context) core.DB
}

// WithLambdaTimeout returns a DB scoped to the supplied Lambda invocation
// deadline when the client supports TableTheory Lambda timeout handling.
func WithLambdaTimeout(ctx context.Context, db core.DB) core.DB {
	if db == nil || ctx == nil {
		return db
	}

	if lambdaDB, ok := db.(*tabletheory.LambdaDB); ok {
		return lambdaDB.WithLambdaTimeout(ctx)
	}

	if timeoutDB, ok := db.(lambdaTimeoutApplier); ok {
		return timeoutDB.WithLambdaTimeout(ctx)
	}

	return db
}
