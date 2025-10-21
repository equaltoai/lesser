// Package lift provides a Lambda-native framework for building serverless
// applications in Go. It offers a unified Context that standardizes request
// parsing, response writing, authentication claims, logging, and metrics across
// HTTP and non-HTTP event sources. Lift includes a simple router, an event
// adapter registry (API Gateway v1/v2, SQS, S3, EventBridge, WebSocket), typed
// handler helpers, structured errors, and production-ready middleware.
//
// Key Features:
//
//   - Type-safe handlers using Go generics
//   - Unified Context interface for request handling
//   - Middleware support for cross-cutting concerns
//   - Built-in validation and error handling
//   - Multi-tenant support
//   - AWS Lambda integration
//
// Design Philosophy:
//
// Lift is designed to be opinionated yet flexible, providing consistent,
// production-safe patterns while reducing boilerplate for event-driven backends.
// It emphasizes type safety, error handling, and multi-tenant support.
//
// Typical Usage:
//
//	// Create a new Lift application
//	app := lift.New(
//	    lift.WithConfig(&lift.Config{LogLevel: "DEBUG"}),
//	    lift.WithLogger(myLogger),
//	)
//	defer app.Stop()
//
//	// Add middleware
//	app.Use(middleware.RequestID())
//	app.Use(middleware.Logger())
//	app.Use(middleware.Recover())
//
//	// Add routes
//	app.GET("/health", func(ctx *lift.Context) error {
//	    return ctx.JSON(map[string]string{"status": "healthy"})
//	})
//
//	app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req UserRequest) (UserResponse, error) {
//	    // Handle request
//	    return UserResponse{ID: "user_123"}, nil
//	}))
//
//	// Start the application
//	if err := app.Start(); err != nil {
//	    log.Fatalf("Failed to start app: %v", err)
//	}
//
// For more information, see the README.md file in the project root.
//
// App Struct and Methods:
//
// The App struct is the main application container in Lift. It holds the
// configuration, middleware, routes, and other application-wide settings.
//
//	// New creates a new Lift application with the given options.
//	// The options are applied in the order they are provided.
//	// Parameters:
//	//   - options: A variadic list of AppOption functions to configure the application
//	// Returns:
//	//   - A pointer to the newly created App
//	func New(options ...AppOption) *App
//
//	// Use adds middleware to the application.
//	// Middleware is executed in the order it is added (last added runs closest to the handler).
//	// By default middleware applies to HTTP/WebSocket routes; call lift.MarkGlobalMiddleware to
//	// opt into non-HTTP triggers such as SQS or S3.
//	// Parameters:
//	//   - mw: The middleware function
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) Use(mw func(Handler) Handler) *App
//
//	// GET registers a GET route.
//	// Parameters:
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) GET(path string, handler any) error
//
//	// POST registers a POST route.
//	// Parameters:
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) POST(path string, handler any) error
//
//	// PUT registers a PUT route.
//	// Parameters:
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) PUT(path string, handler any) error
//
//	// DELETE registers a DELETE route.
//	// Parameters:
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) DELETE(path string, handler any) error
//
//	// PATCH registers a PATCH route.
//	// Parameters:
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) PATCH(path string, handler any) error
//
//	// Handle registers a route with the specified method and path.
//	// Parameters:
//	//   - method: The HTTP method for the route
//	//   - path: The URL path for the route
//	//   - handler: The handler function for the route
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) Handle(method, path string, handler any) error
//
//	// WithConfig sets the application configuration.
//	// Parameters:
//	//   - config: A pointer to the Config struct
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) WithConfig(config *Config) *App
//
//	// WithLogger sets the application logger.
//	// Parameters:
//	//   - logger: A Logger instance
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) WithLogger(logger Logger) *App
//
//	// WithMetrics sets the metrics collector.
//	// Parameters:
//	//   - metrics: A MetricsCollector instance
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) WithMetrics(metrics MetricsCollector) *App
//
//	// WithDatabase sets the database connection.
//	// Parameters:
//	//   - db: A database connection
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) WithDatabase(db any) *App
//
//	// WithPreferredAdapters sets an ordered list of preferred event adapters for parsing Lambda events.
//	// If set, the app will try these adapters (in order) before falling back to auto-detection.
//	// Parameters:
//	//   - order: A variadic list of TriggerType
//	// Returns:
//	//   - A pointer to the App
//	func (a *App) WithPreferredAdapters(order ...adapters.TriggerType) *App
//
//	// Group creates a new route group with the specified prefix.
//	// Parameters:
//	//   - prefix: The URL prefix for the route group
//	// Returns:
//	//   - A pointer to the RouteGroup
//	func (a *App) Group(prefix string) *RouteGroup
//
//	// Start prepares the application for handling requests.
//	// Returns:
//	//   - An error if the application is already started
//	func (a *App) Start() error
//
//	// Stop gracefully shuts down background components and cancels the
//	// application lifecycle context. It is safe to call multiple times.
//	func (a *App) Stop()
//
//	// IsLambda returns true if the code is running in an AWS Lambda environment.
//	// Returns:
//	//   - A boolean indicating if the code is running in AWS Lambda
//	func (a *App) IsLambda() bool
//
//	// HandleRequest processes an incoming Lambda request.
//	// Parameters:
//	//   - ctx: The context for the request
//	//   - event: The Lambda event
//	// Returns:
//	//   - The response from the handler
//	//   - An error if the request handling fails
//	func (a *App) HandleRequest(ctx context.Context, event any) (any, error)
//
//	// HandleTestRequest processes a test request directly through the router.
//	// This is used by the testing framework to bypass event parsing.
//	// Parameters:
//	//   - ctx: The context for the request
//	// Returns:
//	//   - An error if the request handling fails
//	func (a *App) HandleTestRequest(ctx *Context) error
//
//	// GetEventRouter returns the EventRouter for accessing event routes (mainly for testing).
//	// Returns:
//	//   - The EventRouter
//	func (a *App) GetEventRouter() *EventRouter
//
//	// SQS registers a handler for SQS events.
//	// Parameters:
//	//   - pattern: The pattern for the SQS event
//	//   - handler: The handler function for the event
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) SQS(pattern string, handler any) error
//
//	// S3 registers a handler for S3 events.
//	// Parameters:
//	//   - pattern: The pattern for the S3 event
//	//   - handler: The handler function for the event
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) S3(pattern string, handler any) error
//
//	// EventBridge registers a handler for EventBridge events.
//	// Parameters:
//	//   - pattern: The pattern for the EventBridge event
//	//   - handler: The handler function for the event
//	// Returns:
//	//   - An error if the handler type is unsupported
//	func (a *App) EventBridge(pattern string, handler any) error
//
//	// RunLocalTest runs local testing logic when not in Lambda environment.
//	// It is used to run tests locally without deploying to AWS Lambda.
//	func (a *App) RunLocalTest()
//
//	// WithDebug enables debug mode for the application.
//	// Returns:
//	//   - An AppOption function that enables debug mode
//	func WithDebug() AppOption
//
//	// WithConfig sets a custom configuration for the application.
//	// Parameters:
//	//   - config: A pointer to the Config struct
//	// Returns:
//	//   - An AppOption function that sets the configuration
//	func WithConfig(config *Config) AppOption
package lift
