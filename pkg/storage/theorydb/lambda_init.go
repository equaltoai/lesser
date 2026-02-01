package theorydb

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// LambdaInitOptions provides configuration for Lambda initialization
type LambdaInitOptions struct {
	// Models to pre-register during initialization
	Models []any

	// EnableCostTracking wraps the DB client with cost tracking
	EnableCostTracking bool

	// Logger for cost tracking and debugging
	Logger *zap.Logger

	// RequestID for cost tracking context
	RequestID string

	// OperationType for cost tracking context
	OperationType string

	// PrewarmConnections creates and validates connections during init
	PrewarmConnections bool

	// ConnectionCount number of connections to prewarm (default: 2)
	ConnectionCount int

	// EnableLazyLoading defers non-critical initialization
	EnableLazyLoading bool

	// TimeoutBuffer for Lambda context timeout handling
	TimeoutBuffer time.Duration
}

// Global initialization state to track performance metrics
var initMetrics = struct {
	mu              sync.RWMutex
	coldStartTime   time.Duration
	modelCount      int
	connectionCount int
	initialized     bool
}{}

// LambdaInit is a helper function to initialize DynamORM in Lambda functions
// It creates a Lambda-optimized client and pre-registers the provided models
// This should be called in the init() function of Lambda handlers
func LambdaInit(models ...any) (core.DB, error) {
	// Use default options for backward compatibility
	return LambdaInitWithOptions(&LambdaInitOptions{
		Models:             models,
		PrewarmConnections: true,
		ConnectionCount:    2,
	})
}

// LambdaInitWithOptions initializes DynamORM with advanced performance options
func LambdaInitWithOptions(opts *LambdaInitOptions) (core.DB, error) {
	// Use default options if nil
	if opts == nil {
		opts = &LambdaInitOptions{}
	}

	startTime := time.Now()
	defer func() {
		initMetrics.mu.Lock()
		initMetrics.coldStartTime = time.Since(startTime)
		initMetrics.initialized = true
		initMetrics.mu.Unlock()

		zap.L().Info("DynamORM Lambda initialization completed",
			zap.Duration("cold_start_time", initMetrics.coldStartTime),
			zap.Int("model_count", initMetrics.modelCount),
			zap.Int("connection_count", initMetrics.connectionCount))
	}()

	// Set runtime optimizations for Lambda
	optimizeRuntime()

	zap.L().Info("initializing Lambda-optimized DynamORM client")

	// Get the Lambda-optimized client
	lambdaDB, err := GetLambdaClient(context.Background())
	if err != nil {
		zap.L().Error("failed to initialize DynamORM", zap.Error(err))
		return nil, err
	}

	// Apply timeout buffer if specified
	var db core.DB = lambdaDB
	if opts.TimeoutBuffer > 0 {
		db = lambdaDB.WithLambdaTimeoutBuffer(opts.TimeoutBuffer)
	}

	// Pre-register models to reduce cold start time
	if len(opts.Models) > 0 {
		if err := preRegisterModelsParallel(lambdaDB, opts.Models); err != nil {
			zap.L().Error("failed to pre-register models", zap.Error(err))
			return db, err
		}
		initMetrics.modelCount = len(opts.Models)
	}

	// Prewarm connections if enabled
	if opts.PrewarmConnections {
		count := opts.ConnectionCount
		if count <= 0 {
			count = 2 // Default connection count
		}
		if err := prewarmConnections(db, count); err != nil {
			zap.L().Error("failed to prewarm connections", zap.Error(err))
			// Non-fatal error - continue with initialization
		}
		initMetrics.connectionCount = count
	}

	// Wrap with cost tracking if enabled
	if opts.EnableCostTracking && opts.Logger != nil {
		tracker := cost.NewWithRequest(opts.RequestID, opts.OperationType)
		return cost.NewTrackingDB(db, tracker, opts.Logger), nil
	}

	return db, nil
}

// optimizeRuntime sets runtime parameters optimized for Lambda
func optimizeRuntime() {
	// Reduce GC frequency for better cold start performance
	// Lambda functions are short-lived, so we can be more aggressive
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Set GC percentage higher to reduce GC overhead during initialization
	// Only override if GOGC is not explicitly set in environment
	cfg := config.Get()
	if cfg.Stage == "dev" || cfg.Stage == "prod" {
		debug.SetGCPercent(500) // Reduce GC frequency during init

		// Reset after initialization using a goroutine
		go func() {
			time.Sleep(100 * time.Millisecond)
			debug.SetGCPercent(100) // Return to normal
		}()
	}
}

// preRegisterModelsParallel registers models in parallel for faster initialization
func preRegisterModelsParallel(lambdaDB *tabletheory.LambdaDB, models []any) error {
	if err := common.ValidateSliceNotEmpty("models", models); err != nil {
		return nil
	}

	// For small numbers of models, use sequential registration
	if len(models) <= 3 {
		return lambdaDB.PreRegisterModels(models...)
	}

	// For larger numbers, use parallel registration
	var wg sync.WaitGroup
	errChan := make(chan error, len(models))

	for _, model := range models {
		wg.Add(1)
		go func(m any) {
			defer wg.Done()
			if err := lambdaDB.PreRegisterModels(m); err != nil {
				errChan <- err
			}
		}(model)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// prewarmConnections creates and validates connections to reduce cold start latency
func prewarmConnections(db core.DB, count int) error {
	zap.L().Info("prewarming connections", zap.Int("count", count))

	var wg sync.WaitGroup
	errChan := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Perform a lightweight operation to establish connection
			// Using a non-existent key to minimize data transfer
			testKey := struct {
				PK string `dynamodbav:"PK"`
				SK string `dynamodbav:"SK"`
			}{
				PK: "test#prewarm",
				SK: "test#prewarm",
			}

			// This will establish a connection without returning data
			// We expect this to fail since the key doesn't exist, but that's ok
			// We just want to establish the connection
			_ = db.Model(&testKey).First(&testKey)
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for connection errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// GetInitMetrics returns initialization performance metrics
func GetInitMetrics() (coldStartTime time.Duration, modelCount int, connectionCount int) {
	initMetrics.mu.RLock()
	defer initMetrics.mu.RUnlock()
	return initMetrics.coldStartTime, initMetrics.modelCount, initMetrics.connectionCount
}

// LazyLoader provides deferred initialization for non-critical components
type LazyLoader struct {
	mu     sync.Mutex
	loader func() (any, error)
	value  any
	err    error
	loaded bool
}

// NewLazyLoader creates a new lazy loader for deferred initialization
func NewLazyLoader(loader func() (any, error)) *LazyLoader {
	return &LazyLoader{
		loader: loader,
	}
}

// Get returns the lazily loaded value, initializing it on first access
func (l *LazyLoader) Get() (any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.loaded {
		l.value, l.err = l.loader()
		l.loaded = true
	}

	return l.value, l.err
}

// Reset clears the cached value, forcing re-initialization on next access
func (l *LazyLoader) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loaded = false
	l.value = nil
	l.err = nil
}

// Example Lambda initialization patterns:

/*
// Basic initialization with backward compatibility:
package main

import (
	"context"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

var db core.DB

func init() {
	var err error
	// Simple initialization with models
	db, err = tabletheory.LambdaInit(&User{}, &Post{}, &Comment{})
	if err != nil {
		panic(err)
	}
}
*/

/*
// Advanced initialization with performance optimizations:
package main

import (
	"context"
	"time"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

var (
	db     core.DB
	logger *zap.Logger
)

// Define your models
type User struct {
	tabletheory.StandardModel
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type Post struct {
	tabletheory.StandardModel
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Initialize with advanced options
func init() {
	logger = common.Logger()

	var err error
	db, err = tabletheory.LambdaInitWithOptions(&tabletheory.LambdaInitOptions{
		// Models to pre-register
		Models: []any{&User{}, &Post{}},

		// Enable cost tracking
		EnableCostTracking: true,
		Logger:            logger,
		RequestID:         "lambda-init",
		OperationType:     "api-handler",

		// Performance optimizations
		PrewarmConnections: true,
		ConnectionCount:    3, // Prewarm 3 connections
		TimeoutBuffer:      500 * time.Millisecond,

		// Enable lazy loading for non-critical components
		EnableLazyLoading: true,
	})
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	// Log initialization metrics
	coldStart, models, connections := tabletheory.GetInitMetrics()
	logger.Info("DynamORM initialized",
		zap.Duration("cold_start_time", coldStart),
		zap.Int("models_registered", models),
		zap.Int("connections_prewarmed", connections),
	)
}

// Handler with cost tracking context
func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Cost tracking is automatically handled by the wrapped DB
	user := &User{ID: "user123"}
	err := db.Model(user).Where("ID", "=", "user123").First(user)
	if err != nil {
		return nil, err
	}

	// ... rest of handler logic
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Body:       "Success",
	}, nil
}

func main() {
	lambda.Start(handler)
}
*/

/*
// Example with lazy loading for non-critical components:
package main

import (
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/ai"
)

var (
	db            core.DB
	aiServiceLazy *tabletheory.LazyLoader
)

func init() {
	// Initialize core DB with models
	db, tabletheory.LambdaInit(&User{}, &Post{})

	// Defer AI service initialization until first use
	aiServiceLazy = tabletheory.NewLazyLoader(func() (any, error) {
		return ai.NewService(ai.Config{
			Region: "us-east-1",
		})
	})
}

func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// AI service is only initialized if this path is taken
	if event.Path == "/api/v1/ai/generate" {
		aiService, err := aiServiceLazy.Get()
		if err != nil {
			return nil, err
		}
		// Use AI service...
		_ = aiService
	}

	// Regular DB operations don't trigger AI initialization
	// ...
}
*/
