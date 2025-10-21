package lift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// Config represents the application configuration.
// It includes settings for logging, CORS, timeouts, and other application-wide configurations.
type Config struct {
	LogLevel        string   `json:"log_level"`
	AllowedOrigins  []string `json:"allowed_origins"`
	MaxRequestSize  int64    `json:"max_request_size"`
	MaxResponseSize int64    `json:"max_response_size"`
	Timeout         int      `json:"timeout_seconds"`
	MetricsEnabled  bool     `json:"metrics_enabled"`
	TracingEnabled  bool     `json:"tracing_enabled"`
	Debug           bool     `json:"debug"`
	CORSEnabled     bool     `json:"cors_enabled"`
	RequireTenantID bool     `json:"require_tenant_id"`
}

// DefaultConfig returns a configuration with sensible defaults.
// It provides a baseline configuration that can be customized further.
func DefaultConfig() *Config {
	return &Config{
		MaxRequestSize:  10 * 1024 * 1024, // 10MB
		MaxResponseSize: 6 * 1024 * 1024,  // 6MB (Lambda limit)
		Timeout:         30,               // 30 seconds
		LogLevel:        "INFO",
		MetricsEnabled:  true,
		TracingEnabled:  false,
		Debug:           false,
		CORSEnabled:     true,
		AllowedOrigins:  []string{"*"},
		RequireTenantID: false,
	}
}

// AppOption is a function that configures an App.
// It allows for functional configuration of the App struct.
type AppOption func(*App)

type middlewareEntry struct {
	handler         Middleware
	appliesToEvents bool
}

// App represents the main application container.
// It is the central structure for configuring and running a Lift application.
// The App struct holds the configuration, middleware, routes, and other application-wide settings.
// App represents a Lift application. It is the main entry point for creating
// and configuring a serverless application. The App struct holds the
// configuration, middleware, routes, and other application-wide settings.
//
// Fields:
//   - logger: The logger used by the application
//   - metrics: The metrics collector used by the application
//   - db: A generic database connection
//   - router: The router used to match incoming requests to handlers
//   - eventRouter: The event router used to match incoming events to handlers
//   - config: The application configuration
//   - adapterRegistry: The registry of event adapters
//   - wsOptions: WebSocket options
//   - wsRoutes: WebSocket routes
//   - middleware: The middleware stack
//   - preferredAdapters: The preferred event adapters
//   - features: Feature flags
//   - mu: A mutex for synchronizing access to the App
//   - started: Whether the application has been started
//   - hasInterceptingMiddleware: Whether the application has intercepting middleware
type App struct { //nolint:govet // fieldalignment: keep readable order; negligible impact
	// Interfaces (16 bytes) first
	logger  Logger
	metrics MetricsCollector
	tracer  any
	db      any

	// Pointers (8 bytes)
	router          *Router
	eventRouter     *EventRouter
	config          *Config
	adapterRegistry *adapters.AdapterRegistry
	wsOptions       *WebSocketOptions
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc

	// Maps/slices (24 bytes)
	wsRoutes          map[string]WebSocketHandler
	middleware        []middlewareEntry
	preferredAdapters []adapters.TriggerType
	features          map[string]bool
	shutdownHooks     []func()

	metricsFactory func(*Config) MetricsCollector
	tracerFactory  func(*Config) any

	// Sync primitives and small scalars last
	mu                        sync.RWMutex
	started                   bool
	hasInterceptingMiddleware bool
}

// New creates a new Lift application with the given options.
// The options are applied in the order they are provided.
//
// Parameters:
//   - options: A variadic list of AppOption functions to configure the application
//
// Returns:
//   - A pointer to the newly created App
//
// Example:
//
//	app := lift.New(
//	    lift.WithConfig(&lift.Config{LogLevel: "DEBUG"}),
//	    lift.WithLogger(myLogger),
//	)
//
// New creates a new Lift application with the given options. The options are
// applied in the order they are provided.
//
// Parameters:
//   - options: A variadic list of AppOption functions to configure the application
//
// Returns:
//   - A pointer to the newly created App
//
// Example:
//
//	app := lift.New(
//	    lift.WithConfig(&lift.Config{LogLevel: "DEBUG"}),
//	    lift.WithLogger(myLogger),
//	)
func New(options ...AppOption) *App {
	app := &App{
		router:          NewRouter(),
		eventRouter:     NewEventRouter(),
		middleware:      make([]middlewareEntry, 0),
		config:          DefaultConfig(),
		adapterRegistry: adapters.NewAdapterRegistry(),
		features:        make(map[string]bool),
		started:         false,
	}

	// Apply options
	for _, opt := range options {
		opt(app)
	}

	app.ensureLifecycleContextLocked()

	return app
}

func (a *App) ensureLifecycleContextLocked() {
	if a.lifecycleCtx == nil || a.lifecycleCancel == nil {
		a.lifecycleCtx, a.lifecycleCancel = context.WithCancel(context.Background())
	}
}

// LifecycleContext returns a process-scoped context that is canceled when the
// application stops. Middleware can use it to manage background goroutines.
func (a *App) LifecycleContext() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ensureLifecycleContextLocked()
	return a.lifecycleCtx
}

// RegisterShutdownHook registers a cleanup hook that executes when the
// application stops. Hooks run in LIFO order and panics are recovered to avoid
// interrupting other cleanup tasks.
func (a *App) RegisterShutdownHook(hook func()) {
	if hook == nil {
		return
	}

	a.mu.Lock()
	a.shutdownHooks = append(a.shutdownHooks, hook)
	a.mu.Unlock()
}

// Stop cancels the lifecycle context and executes registered shutdown hooks.
// It is safe to call multiple times; subsequent calls become no-ops.
func (a *App) Stop() {
	a.mu.Lock()
	if a.lifecycleCancel == nil && len(a.shutdownHooks) == 0 {
		a.mu.Unlock()
		return
	}

	cancel := a.lifecycleCancel
	a.lifecycleCancel = nil
	a.lifecycleCtx = nil
	hooks := make([]func(), len(a.shutdownHooks))
	copy(hooks, a.shutdownHooks)
	a.shutdownHooks = nil
	a.started = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	for i := len(hooks) - 1; i >= 0; i-- {
		hook := hooks[i]
		if hook == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					_ = r // ignore panic to allow other shutdown hooks to complete
				}
			}()
			hook()
		}()
	}
}

// Use adds middleware to the application.
// Middleware is executed in the order it is added (last added runs closest to the handler).
// By default middleware only runs for HTTP/WebSocket requests; global middleware such as
// logging/metrics can opt-in to non-HTTP triggers via lift.MarkGlobalMiddleware.
func (a *App) Use(mw func(Handler) Handler) *App {
	a.addMiddleware(Middleware(mw), false)
	return a
}

type eventScopedMiddleware interface {
	AppliesToEvents() bool
}

func (a *App) addMiddleware(mw Middleware, appliesToEvents bool) {
	if mw == nil {
		return
	}

	if scoped, ok := any(mw).(eventScopedMiddleware); ok {
		appliesToEvents = scoped.AppliesToEvents()
	}

	if !appliesToEvents {
		appliesToEvents = middlewareAppliesToEvents(mw)
	}

	if interceptor, ok := any(mw).(ResponseInterceptor); ok && interceptor.NeedsResponseInterception() {
		a.hasInterceptingMiddleware = true
	}

	a.middleware = append(a.middleware, middlewareEntry{
		handler:         mw,
		appliesToEvents: appliesToEvents,
	})
}

func (a *App) httpMiddlewareChain() []Middleware {
	chain := make([]Middleware, 0, len(a.middleware))
	for _, entry := range a.middleware {
		chain = append(chain, entry.handler)
	}
	return chain
}

func (a *App) eventMiddlewareChain() []Middleware {
	chain := make([]Middleware, 0, len(a.middleware))
	for _, entry := range a.middleware {
		if !entry.appliesToEvents {
			continue
		}
		chain = append(chain, entry.handler)
	}
	return chain
}

func (a *App) ensureTelemetry() {
	if !a.config.MetricsEnabled {
		a.metrics = nil
	} else if a.metrics == nil && a.metricsFactory != nil {
		a.metrics = a.metricsFactory(a.config)
	}

	if !a.config.TracingEnabled {
		a.tracer = nil
	} else if a.tracer == nil && a.tracerFactory != nil {
		a.tracer = a.tracerFactory(a.config)
	}
}

// GET registers a GET route.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) GET(path string, handler any) error {
	return a.Handle("GET", path, handler)
}

// POST registers a POST route.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) POST(path string, handler any) error {
	return a.Handle("POST", path, handler)
}

// PUT registers a PUT route.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) PUT(path string, handler any) error {
	return a.Handle("PUT", path, handler)
}

// DELETE registers a DELETE route.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) DELETE(path string, handler any) error {
	return a.Handle("DELETE", path, handler)
}

// PATCH registers a PATCH route.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) PATCH(path string, handler any) error {
	return a.Handle("PATCH", path, handler)
}

// Handle registers a route with the specified method and path.
//
// Parameters:
//   - method: The HTTP method for the route
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) Handle(method, path string, handler any) error {
	// Check if this is an event trigger type
	triggerType := parseTriggerType(method)
	if triggerType != TriggerUnknown && triggerType != TriggerAPIGateway {
		// This is a non-HTTP event, use the event router
		var eventHandler EventHandler
		switch v := handler.(type) {
		case EventHandler:
			eventHandler = v
		case func(*Context) error:
			eventHandler = EventHandlerFunc(v)
		default:
			// Convert to event handler
			h, err := convertHandlerUsingReflection(handler)
			if err != nil {
				return fmt.Errorf("unsupported handler type: %w", err)
			}
			eventHandler = EventHandlerFunc(h.Handle)
		}

		a.eventRouter.AddEventRoute(triggerType, path, eventHandler)
		return nil
	}

	// This is an HTTP route
	var h Handler
	switch v := handler.(type) {
	case Handler:
		h = v
	case func(*Context) error:
		h = HandlerFunc(v)
	default:
		// Use reflection to support additional handler types
		reflectedHandler, err := convertHandlerUsingReflection(handler)
		if err != nil {
			return fmt.Errorf("unsupported handler type: %w", err)
		}
		h = reflectedHandler
	}

	a.router.AddRoute(method, path, h)
	return nil
}

// WithConfig sets the application configuration.
//
// Parameters:
//   - config: A pointer to the Config struct
//
// Returns:
//   - A pointer to the App
func (a *App) WithConfig(config *Config) *App {
	a.config = config
	return a
}

// WithLogger sets the application logger.
//
// Parameters:
//   - logger: A Logger instance
//
// Returns:
//   - A pointer to the App
func (a *App) WithLogger(logger Logger) *App {
	a.logger = logger
	return a
}

// WithMetrics sets the metrics collector.
//
// Parameters:
//   - metrics: A MetricsCollector instance
//
// Returns:
//   - A pointer to the App
func (a *App) WithMetrics(metrics MetricsCollector) *App {
	a.metrics = metrics
	return a
}

// WithMetricsFactory registers a factory used to lazily construct a metrics collector when metrics are enabled.
func (a *App) WithMetricsFactory(factory func(*Config) MetricsCollector) *App {
	a.metricsFactory = factory
	return a
}

// WithDatabase sets the database connection.
//
// Parameters:
//   - db: A database connection
//
// Returns:
//   - A pointer to the App
func (a *App) WithDatabase(db any) *App {
	a.db = db
	return a
}

// WithTracer sets the tracer implementation used by the application when tracing is enabled.
func (a *App) WithTracer(tracer any) *App {
	a.tracer = tracer
	return a
}

// WithTracerFactory registers a factory used to lazily construct a tracer when tracing is enabled.
func (a *App) WithTracerFactory(factory func(*Config) any) *App {
	a.tracerFactory = factory
	return a
}

// WithPreferredAdapters sets an ordered list of preferred event adapters for parsing Lambda events.
// If set, the app will try these adapters (in order) before falling back to auto-detection.
//
// Parameters:
//   - order: A variadic list of TriggerType
//
// Returns:
//   - A pointer to the App
func (a *App) WithPreferredAdapters(order ...adapters.TriggerType) *App {
	a.preferredAdapters = append([]adapters.TriggerType(nil), order...)
	return a
}

// Group creates a new route group with the specified prefix.
//
// Parameters:
//   - prefix: The URL prefix for the route group
//
// Returns:
//   - A pointer to the RouteGroup
func (a *App) Group(prefix string) *RouteGroup {
	return &RouteGroup{
		app:    a,
		prefix: prefix,
	}
}

// RouteGroup represents a group of routes with a common prefix.
// It allows for organizing routes under a common URL prefix.
type RouteGroup struct {
	app    *App
	prefix string
}

// GET registers a GET route in this group.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (rg *RouteGroup) GET(path string, handler any) error {
	return rg.app.GET(rg.prefix+path, handler)
}

// POST registers a POST route in this group.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (rg *RouteGroup) POST(path string, handler any) error {
	return rg.app.POST(rg.prefix+path, handler)
}

// PUT registers a PUT route in this group.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (rg *RouteGroup) PUT(path string, handler any) error {
	return rg.app.PUT(rg.prefix+path, handler)
}

// DELETE registers a DELETE route in this group.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (rg *RouteGroup) DELETE(path string, handler any) error {
	return rg.app.DELETE(rg.prefix+path, handler)
}

// PATCH registers a PATCH route in this group.
//
// Parameters:
//   - path: The URL path for the route
//   - handler: The handler function for the route
//
// Returns:
//   - An error if the handler type is unsupported
func (rg *RouteGroup) PATCH(path string, handler any) error {
	return rg.app.PATCH(rg.prefix+path, handler)
}

// Group creates a sub-group with an additional prefix.
//
// Parameters:
//   - prefix: The additional URL prefix for the sub-group
//
// Returns:
//   - A pointer to the RouteGroup
func (rg *RouteGroup) Group(prefix string) *RouteGroup {
	return &RouteGroup{
		app:    rg.app,
		prefix: rg.prefix + prefix,
	}
}

// Use attaches middleware to this route group. Delegates to app-level chain.
//
// Parameters:
//   - mw: The middleware function
//
// Returns:
//   - A pointer to the RouteGroup
func (rg *RouteGroup) Use(mw func(Handler) Handler) *RouteGroup {
	rg.app.Use(mw)
	return rg
}

// Start prepares the application for handling requests.
//
// Returns:
//   - An error if the application is already started
func (a *App) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}

	a.ensureTelemetry()
	a.ensureLifecycleContextLocked()

	// Apply global middleware to router
	a.router.SetMiddleware(a.httpMiddlewareChain())

	a.started = true
	return nil
}

// IsLambda returns true if the code is running in an AWS Lambda environment.
//
// Returns:
//   - A boolean indicating if the code is running in AWS Lambda
func (a *App) IsLambda() bool {
	// Check for AWS Lambda-specific environment variables
	// AWS_LAMBDA_FUNCTION_NAME is set in all Lambda runtime environments
	// LAMBDA_TASK_ROOT is the path to the Lambda function code
	// AWS_EXECUTION_ENV contains the runtime identifier (e.g., AWS_Lambda_go1.x)
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" ||
		os.Getenv("LAMBDA_TASK_ROOT") != "" ||
		os.Getenv("AWS_EXECUTION_ENV") != ""
}

// HandleRequest processes an incoming Lambda request.
//
// Parameters:
//   - ctx: The context for the request
//   - event: The Lambda event
//
// Returns:
//   - The response from the handler
//   - An error if the request handling fails
func (a *App) HandleRequest(ctx context.Context, event any) (any, error) {
	builder := newRequestHandlerBuilder(ctx, a, event)
	return builder.build()
}

// requestHandlerBuilder builds and executes Lambda request handling.
// It is responsible for constructing the request handling pipeline.
type requestHandlerBuilder struct {
	app      *App
	ctx      context.Context
	event    any
	liftCtx  *Context
	request  *Request
	routeErr error
	cancel   context.CancelFunc
}

// newRequestHandlerBuilder creates a new request handler builder.
//
// Parameters:
//   - ctx: The context for the request
//   - app: The App instance
//   - event: The Lambda event
//
// Returns:
//   - A pointer to the requestHandlerBuilder
func newRequestHandlerBuilder(ctx context.Context, app *App, event any) *requestHandlerBuilder {
	return &requestHandlerBuilder{
		app:   app,
		ctx:   ctx,
		event: event,
	}
}

// build executes the complete request handling pipeline.
//
// Returns:
//   - The response from the handler
//   - An error if the request handling fails
func (b *requestHandlerBuilder) build() (any, error) {
	if err := b.ensureAppStarted(); err != nil {
		return nil, err
	}

	if err := b.parseEvent(); err != nil {
		return nil, err
	}

	b.createContext()
	if err := b.applyRequestGuardrails(); err != nil {
		return b.app.handleError(b.liftCtx, err)
	}
	b.configureContext()
	defer b.cancelTimeout()
	b.routeRequest()

	if b.routeErr != nil {
		return b.app.handleError(b.liftCtx, b.routeErr)
	}

	if err := b.enforceTenantRequirement(); err != nil {
		return b.app.handleError(b.liftCtx, err)
	}

	if err := b.validateResponseGuardrails(); err != nil {
		return b.app.handleError(b.liftCtx, err)
	}

	if err := b.liftCtx.FlushResponse(); err != nil {
		return nil, err
	}

	return b.liftCtx.Response, nil
}

// ensureAppStarted ensures the app is properly initialized.
//
// Returns:
//   - An error if the application fails to start
func (b *requestHandlerBuilder) ensureAppStarted() error {
	return b.app.Start()
}

// parseEvent converts the Lambda event to a Request.
//
// Returns:
//   - An error if the event parsing fails
func (b *requestHandlerBuilder) parseEvent() error {
	req, err := b.app.parseEvent(b.event)
	if err != nil {
		return err
	}
	b.request = req
	return nil
}

// createContext creates the enhanced Lift context.
// It initializes the context with the request and other dependencies.
func (b *requestHandlerBuilder) createContext() {
	b.liftCtx = NewContext(b.ctx, b.request)
}

func (b *requestHandlerBuilder) cancelTimeout() {
	if b.cancel != nil {
		b.cancel()
	}
}

// configureContext sets up the context with app dependencies.
// It configures the context with the logger, metrics, and database.
func (b *requestHandlerBuilder) configureContext() {
	if timeoutSeconds := b.app.config.Timeout; timeoutSeconds > 0 {
		timeout := time.Duration(timeoutSeconds) * time.Second
		ctxWithTimeout, cancel := context.WithTimeout(b.ctx, timeout)
		b.liftCtx.Context = ctxWithTimeout
		b.cancel = cancel
	} else {
		b.liftCtx.Context = b.ctx
		b.cancel = nil
	}

	if b.app.hasInterceptingMiddleware {
		b.liftCtx.EnableResponseBuffering()
	}

	if b.app.logger != nil {
		b.liftCtx.Logger = b.app.logger
	}
	if b.app.metrics != nil {
		b.liftCtx.Metrics = b.app.metrics
	}
	if b.app.tracer != nil {
		b.liftCtx.SetTracer(b.app.tracer)
	}
	if b.app.db != nil {
		b.liftCtx.DB = b.app.db
	}
}

func (b *requestHandlerBuilder) applyRequestGuardrails() error {
	maxSize := b.app.config.MaxRequestSize
	if maxSize <= 0 || b.request == nil {
		return nil
	}

	if bodyLen := int64(len(b.request.Body)); bodyLen > 0 && bodyLen > maxSize {
		details := map[string]any{
			"size_bytes":   bodyLen,
			"limit_bytes":  maxSize,
			"trigger_type": string(b.request.TriggerType),
		}
		return b.guardrailViolation(ErrorCodePayloadTooLarge, 413, "Request body exceeds configured limit", details)
	}

	switch b.request.TriggerType {
	case adapters.TriggerSQS:
		for idx, record := range b.request.Records {
			recordSize := estimateSQSRecordSize(record)
			if recordSize > maxSize {
				details := map[string]any{
					"size_bytes":   recordSize,
					"limit_bytes":  maxSize,
					"record_index": idx,
					"trigger_type": string(b.request.TriggerType),
				}
				return b.guardrailViolation(ErrorCodePayloadTooLarge, 413, "SQS record body exceeds configured limit", details)
			}
		}
	case adapters.TriggerS3:
		for idx, record := range b.request.Records {
			recordSize := estimateS3RecordSize(record)
			if recordSize > maxSize {
				details := map[string]any{
					"size_bytes":   recordSize,
					"limit_bytes":  maxSize,
					"record_index": idx,
					"trigger_type": string(b.request.TriggerType),
				}
				return b.guardrailViolation(ErrorCodePayloadTooLarge, 413, "S3 object size exceeds configured limit", details)
			}
		}
	}

	return nil
}

func (b *requestHandlerBuilder) enforceTenantRequirement() error {
	if !b.app.config.RequireTenantID {
		return nil
	}

	tenantID := b.liftCtx.GetTenantID()
	if tenantID == "" && b.liftCtx.Request != nil {
		if header := b.liftCtx.Request.GetHeader("X-Tenant-ID"); header != "" {
			b.liftCtx.SetTenantID(header)
			tenantID = header
		}
	}

	if tenantID != "" {
		return nil
	}

	details := map[string]any{
		"trigger_type": string(b.request.TriggerType),
	}
	return b.guardrailViolation(ErrorCodeTenantRequired, 400, "Tenant ID is required", details)
}

func (b *requestHandlerBuilder) validateResponseGuardrails() error {
	maxSize := b.app.config.MaxResponseSize
	if maxSize <= 0 {
		return nil
	}

	size, err := b.responsePayloadSize()
	if err != nil {
		return err
	}

	if size > maxSize {
		details := map[string]any{
			"size_bytes":   size,
			"limit_bytes":  maxSize,
			"trigger_type": string(b.request.TriggerType),
		}
		return b.guardrailViolation(ErrorCodePayloadTooLarge, 413, "Response body exceeds configured limit", details)
	}

	return nil
}

func (b *requestHandlerBuilder) responsePayloadSize() (int64, error) {
	if b.liftCtx == nil || b.liftCtx.Response == nil {
		return 0, nil
	}

	body := b.liftCtx.Response.Body
	if body == nil {
		return 0, nil
	}

	switch v := body.(type) {
	case string:
		return int64(len(v)), nil
	case []byte:
		return int64(len(v)), nil
	default:
		serialized, err := json.Marshal(v)
		if err != nil {
			return 0, NewLiftError(ErrorCodeMarshalError, "Failed to marshal response body", 500).WithCause(err)
		}
		return int64(len(serialized)), nil
	}
}

func (b *requestHandlerBuilder) guardrailViolation(code string, status int, message string, details map[string]any) *LiftError {
	b.prepareErrorResponse()
	if details == nil {
		details = make(map[string]any)
	}
	if _, exists := details["trigger_type"]; !exists && b.request != nil {
		details["trigger_type"] = string(b.request.TriggerType)
	}

	if b.app.logger != nil {
		fields := map[string]any{
			"code": code,
		}
		for k, v := range details {
			fields[k] = v
		}
		b.app.logger.Warn(message, fields)
	}

	if b.app.metrics != nil {
		tags := map[string]string{
			"code": code,
		}
		if trigger, ok := details["trigger_type"].(string); ok {
			tags["trigger_type"] = trigger
		}
		b.app.metrics.Counter("lift_guardrail_violation_total", tags).Inc()
	}

	return NewLiftError(code, message, status).WithDetails(details)
}

func (b *requestHandlerBuilder) prepareErrorResponse() {
	if b.liftCtx == nil {
		return
	}
	b.liftCtx.Response = NewResponse()
	if b.app.hasInterceptingMiddleware {
		b.liftCtx.EnableResponseBuffering()
	}
}

func estimateSQSRecordSize(record any) int64 {
	recordMap, ok := record.(map[string]any)
	if !ok {
		return 0
	}

	if body, ok := recordMap["body"].(string); ok {
		return int64(len(body))
	}

	return 0
}

func estimateS3RecordSize(record any) int64 {
	recordMap, ok := record.(map[string]any)
	if !ok {
		return 0
	}

	s3Field, ok := recordMap["s3"].(map[string]any)
	if !ok {
		return 0
	}

	object, ok := s3Field["object"].(map[string]any)
	if !ok {
		return 0
	}

	sizeVal, exists := object["size"]
	if !exists {
		return 0
	}

	switch v := sizeVal.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed
		}
	}

	return 0
}

// routeRequest routes the request based on trigger type.
// It determines the type of request and routes it accordingly.
func (b *requestHandlerBuilder) routeRequest() {
	err := b.executeWithTimeout(func() error {
		switch {
		case b.request.TriggerType == adapters.TriggerWebSocket:
			return b.routeWebSocket()
		case b.isEventTrigger():
			return b.routeEvent()
		default:
			return b.routeHTTP()
		}
	})
	if err != nil {
		b.routeErr = err
	}
}

func (b *requestHandlerBuilder) executeWithTimeout(fn func() error) error {
	if b.app.config.Timeout <= 0 {
		return fn()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn()
	}()

	select {
	case err := <-errCh:
		return err
	case <-b.liftCtx.Done():
		err := b.liftCtx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			details := map[string]any{
				"timeout_seconds": b.app.config.Timeout,
			}
			return b.guardrailViolation(ErrorCodeTimeout, 504, "Request timed out", details)
		}
		return err
	}
}

// isEventTrigger checks if this is a non-HTTP event trigger.
//
// Returns:
//   - A boolean indicating if the request is a non-HTTP event
func (b *requestHandlerBuilder) isEventTrigger() bool {
	return b.request.TriggerType != adapters.TriggerAPIGateway &&
		b.request.TriggerType != adapters.TriggerAPIGatewayV2 &&
		b.request.TriggerType != adapters.TriggerUnknown
}

// routeWebSocket handles WebSocket routing.
// It routes WebSocket requests to the appropriate handler.

func (b *requestHandlerBuilder) routeWebSocket() error {
	routeKey := b.extractRouteKey()
	handler := b.app.RouteWebSocket(routeKey)

	if handler == nil {
		return NewLiftError("WEBSOCKET_ROUTE_NOT_FOUND",
			fmt.Sprintf("No handler for WebSocket route: %s", routeKey), 404)
	}

	finalHandler := b.applyMiddleware(handler)
	finalHandler = b.applyConnectionManagement(finalHandler)

	return finalHandler.Handle(b.liftCtx)
}

// extractRouteKey gets the WebSocket route key from metadata.
//
// Returns:
//   - The route key as a string
func (b *requestHandlerBuilder) extractRouteKey() string {
	if metadata, ok := b.request.Metadata["routeKey"].(string); ok {
		return metadata
	}
	return ""
}

// applyMiddleware wraps the handler with middleware.
//
// Parameters:
//   - handler: The handler to wrap
//
// Returns:
//   - The wrapped handler
func (b *requestHandlerBuilder) applyMiddleware(handler Handler) Handler {
	finalHandler := handler
	chain := b.app.httpMiddlewareChain()
	for i := len(chain) - 1; i >= 0; i-- {
		finalHandler = chain[i](finalHandler)
	}
	return finalHandler
}

// applyConnectionManagement adds WebSocket connection management if enabled.
//
// Parameters:
//   - handler: The handler to wrap
//
// Returns:
//   - The wrapped handler
func (b *requestHandlerBuilder) applyConnectionManagement(handler Handler) Handler {
	if b.app.wsOptions != nil && b.app.wsOptions.EnableAutoConnectionManagement {
		return wrapWithConnectionManagement(handler, b.app.wsOptions.ConnectionStore)
	}
	return handler
}

// routeEvent handles non-HTTP event routing.
// It routes non-HTTP events to the appropriate handler.
func (b *requestHandlerBuilder) routeEvent() error {
	return b.app.eventRouter.HandleEvent(b.liftCtx, b.app.eventMiddlewareChain())
}

// routeHTTP handles HTTP request routing.
// It routes HTTP requests to the appropriate handler.
func (b *requestHandlerBuilder) routeHTTP() error {
	return b.app.router.Handle(b.liftCtx)
}

// parseEvent converts a Lambda event to our Request structure.
//
// Parameters:
//   - event: The Lambda event
//
// Returns:
//   - The parsed Request
//   - An error if the event parsing fails
func (a *App) parseEvent(event any) (*Request, error) {
	a.logEventDebug(event)

	if req, ok := a.tryPreferredAdapters(event); ok {
		return req, nil
	}

	return a.detectAndAdaptEvent(event)
}

// logEventDebug logs useful debug information about the incoming event.
// It logs debug information if debug mode is enabled.
func (a *App) logEventDebug(event any) {
	if !a.config.Debug || a.logger == nil {
		return
	}
	// Avoid logging user-controlled content; just record that an event arrived.
	a.logger.Debug("Parsing Lambda event")
	if eventMap, ok := event.(map[string]any); ok {
		a.logger.WithField("field_count", len(eventMap)).Debug("Event fields detected")
	}
}

// tryPreferredAdapters attempts to parse using preferred adapters first.
//
// Parameters:
//   - event: The Lambda event
//
// Returns:
//   - The parsed Request
//   - A boolean indicating if the parsing was successful
func (a *App) tryPreferredAdapters(event any) (*Request, bool) {
	if len(a.preferredAdapters) == 0 {
		return nil, false
	}
	for _, tt := range a.preferredAdapters {
		adapter, ok := a.adapterRegistry.GetAdapter(tt)
		if !ok || !adapter.CanHandle(event) {
			continue
		}
		if req, err := adapter.Adapt(event); err == nil {
			// Do not log method/path which can be user-controlled.
			return NewRequest(req), true
		}
	}
	return nil, false
}

// detectAndAdaptEvent uses the registry to detect and adapt events.
//
// Parameters:
//   - event: The Lambda event
//
// Returns:
//   - The parsed Request
//   - An error if the event parsing fails
func (a *App) detectAndAdaptEvent(event any) (*Request, error) {
	adapterRequest, err := a.adapterRegistry.DetectAndAdapt(event)
	if err != nil {
		// Avoid echoing error text that may include user input.
		if a.config.Debug && a.logger != nil {
			a.logger.Error("Failed to parse Lambda event")
		}
		return nil, err
	}
	// Do not log method/path; success is enough for debug tracing.
	return NewRequest(adapterRequest), nil
}

// handleError processes errors and returns appropriate responses.
//
// Parameters:
//   - ctx: The context for the request
//   - err: The error to handle
//
// Returns:
//   - The error response
//   - An error if the error handling fails
func (a *App) handleError(ctx *Context, err error) (any, error) {
	// Handle Lift errors properly by setting appropriate status codes
	if liftErr, ok := err.(*LiftError); ok {
		resp := map[string]any{
			"code":    liftErr.Code,
			"message": liftErr.Message,
		}

		// Include details if present
		if len(liftErr.Details) > 0 {
			resp["details"] = liftErr.Details
		}

		if err := ctx.Status(liftErr.StatusCode).JSON(resp); err != nil {
			// Log error but continue - we're already in error handling
			return nil, fmt.Errorf("failed to send error response: %w", err)
		}
		return ctx.Response, nil
	}

	// For non-Lift errors, set 500 status
	if err := ctx.Status(500).JSON(map[string]string{
		"error": "Internal server error",
	}); err != nil {
		return nil, fmt.Errorf("failed to send internal server error response: %w", err)
	}

	return ctx.Response, nil
}

// HandleTestRequest processes a test request directly through the router.
// This is used by the testing framework to bypass event parsing.
//
// Parameters:
//   - ctx: The context for the request
//
// Returns:
//   - An error if the request handling fails
//
// This is used by the testing framework to bypass event parsing
func (a *App) HandleTestRequest(ctx *Context) error {
	// Ensure the app is started
	if err := a.Start(); err != nil {
		return err
	}

	// Use the router directly to handle the request
	if err := a.router.Handle(ctx); err != nil {
		// Handle Lift errors properly by setting appropriate status codes
		if liftErr, ok := err.(*LiftError); ok {
			if jsonErr := ctx.Status(liftErr.StatusCode).JSON(map[string]any{
				"error":   liftErr.Code,
				"message": liftErr.Message,
			}); jsonErr != nil {
				return fmt.Errorf("failed to send error response: %w", jsonErr)
			}
			return nil // Don't return error, status is set in response
		}

		// For non-Lift errors, set 500 status
		if jsonErr := ctx.Status(500).JSON(map[string]any{
			"error":   "Internal Server Error",
			"message": err.Error(),
		}); jsonErr != nil {
			return fmt.Errorf("failed to send internal server error response: %w", jsonErr)
		}
		return nil // Don't return error, status is set in response
	}

	return nil
}

// GetEventRouter returns the EventRouter for accessing event routes (mainly for testing).
//
// Returns:
//   - The EventRouter
func (a *App) GetEventRouter() *EventRouter {
	return a.eventRouter
}

// SQS registers a handler for SQS events.
//
// Parameters:
//   - pattern: The pattern for the SQS event
//   - handler: The handler function for the event
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) SQS(pattern string, handler any) error {
	h, err := a.convertEventHandler(handler)
	if err != nil {
		return fmt.Errorf("invalid SQS handler: %w", err)
	}
	a.eventRouter.AddEventRoute(TriggerSQS, pattern, h)
	return nil
}

// S3 registers a handler for S3 events.
//
// Parameters:
//   - pattern: The pattern for the S3 event
//   - handler: The handler function for the event
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) S3(pattern string, handler any) error {
	h, err := a.convertEventHandler(handler)
	if err != nil {
		return fmt.Errorf("invalid S3 handler: %w", err)
	}
	a.eventRouter.AddEventRoute(TriggerS3, pattern, h)
	return nil
}

// EventBridge registers a handler for EventBridge events.
//
// Parameters:
//   - pattern: The pattern for the EventBridge event
//   - handler: The handler function for the event
//
// Returns:
//   - An error if the handler type is unsupported
func (a *App) EventBridge(pattern string, handler any) error {
	h, err := a.convertEventHandler(handler)
	if err != nil {
		return fmt.Errorf("invalid EventBridge handler: %w", err)
	}
	a.eventRouter.AddEventRoute(TriggerEventBridge, pattern, h)
	return nil
}

// convertEventHandler converts various handler types to EventHandler.
//
// Parameters:
//   - handler: The handler to convert
//
// Returns:
//   - The converted EventHandler
//   - An error if the conversion fails
func (a *App) convertEventHandler(handler any) (EventHandler, error) {
	// Check if it's already an EventHandler
	if eh, ok := handler.(EventHandler); ok {
		return eh, nil
	}

	// Check if it's an EventHandlerFunc
	if ehf, ok := handler.(func(*Context) error); ok {
		return EventHandlerFunc(ehf), nil
	}

	// Try to convert as HTTP handler and wrap it
	httpHandler, err := convertHandlerUsingReflection(handler)
	if err != nil {
		return nil, err
	}

	// Wrap HTTP handler as EventHandler
	return EventHandlerFunc(func(ctx *Context) error {
		return httpHandler.Handle(ctx)
	}), nil
}

// convertHandlerUsingReflection converts various handler function types to the Handler interface using reflection.
//
// Parameters:
//   - handler: The handler to convert
//
// Returns:
//   - The converted Handler
//   - An error if the conversion fails
func convertHandlerUsingReflection(handler any) (Handler, error) {
	v := reflect.ValueOf(handler)
	t := reflect.TypeOf(handler)

	// Ensure handler is a function
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be a function, got %T", handler)
	}

	// Validate handler function signature at registration time for security
	if err := validateHandlerSignature(t); err != nil {
		return nil, err
	}

	// Convert to our Handler interface based on the function signature
	return createReflectedHandler(v, t), nil
}

// validateHandlerSignature validates that the handler function has a supported signature.
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - An error if the signature is unsupported
//
// handlerPattern represents the different handler signature patterns supported
type handlerPattern int

const (
	patternContextError    handlerPattern = iota // func(*Context) error
	patternContextResponse                       // func(*Context) (any, error)
	patternSimpleError                           // func() error
	patternSimpleResponse                        // func() (any, error)
	patternModelError                            // func(RequestModel) error
	patternModelResponse                         // func(RequestModel) (ResponseModel, error)
	patternUnsupported
)

// handlerValidator provides validation for handler signatures
type handlerValidator struct{}

// validateHandlerSignature validates that a handler function has a supported signature
func validateHandlerSignature(t reflect.Type) error {
	validator := &handlerValidator{}
	pattern := validator.identifyPattern(t)

	if pattern == patternUnsupported {
		return fmt.Errorf("unsupported handler signature: %s", t.String())
	}

	return nil
}

// identifyPattern determines which handler pattern a function type matches.
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The handlerPattern
func (v *handlerValidator) identifyPattern(t reflect.Type) handlerPattern {
	numIn := t.NumIn()
	numOut := t.NumOut()

	switch {
	case numIn == 1 && numOut == 1:
		return v.validateSingleInOut(t)
	case numIn == 1 && numOut == 2:
		return v.validateSingleInDoubleOut(t)
	case numIn == 0 && numOut == 1:
		return v.validateNoInSingleOut(t)
	case numIn == 0 && numOut == 2:
		return v.validateNoInDoubleOut(t)
	default:
		return patternUnsupported
	}
}

// validateSingleInOut validates patterns: func(*Context) error OR func(RequestModel) error.
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The handlerPattern
func (v *handlerValidator) validateSingleInOut(t reflect.Type) handlerPattern {
	if !isErrorType(t.Out(0)) {
		return patternUnsupported
	}

	if isContextType(t.In(0)) {
		return patternContextError
	}

	return patternModelError
}

// validateSingleInDoubleOut validates patterns: func(*Context) (any, error) OR func(RequestModel) (ResponseModel, error).
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The handlerPattern
func (v *handlerValidator) validateSingleInDoubleOut(t reflect.Type) handlerPattern {
	if !isInterfaceType(t.Out(0)) || !isErrorType(t.Out(1)) {
		return patternUnsupported
	}

	if isContextType(t.In(0)) {
		return patternContextResponse
	}

	return patternModelResponse
}

// validateNoInSingleOut validates pattern: func() error.
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The handlerPattern
func (v *handlerValidator) validateNoInSingleOut(t reflect.Type) handlerPattern {
	if isErrorType(t.Out(0)) {
		return patternSimpleError
	}
	return patternUnsupported
}

// validateNoInDoubleOut validates pattern: func() (any, error).
//
// Parameters:
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The handlerPattern
func (v *handlerValidator) validateNoInDoubleOut(t reflect.Type) handlerPattern {
	if isInterfaceType(t.Out(0)) && isErrorType(t.Out(1)) {
		return patternSimpleResponse
	}
	return patternUnsupported
}

// handlerExecutor handles the execution of different handler patterns.
// It is responsible for executing the handler function based on its signature.
// Memory optimized: struct with 48 pointer bytes could be 32
type handlerExecutor struct {
	// reflect.Type (16 bytes - interface)
	funcType reflect.Type
	// reflect.Value (24 bytes)
	value reflect.Value
	// enum (8 bytes on 64-bit)
	pattern handlerPattern
}

// createReflectedHandler creates a Handler from a reflected function.
//
// Parameters:
//   - v: The reflect.Value of the handler function
//   - t: The reflect.Type of the handler function
//
// Returns:
//   - The Handler
func createReflectedHandler(v reflect.Value, t reflect.Type) Handler {
	validator := &handlerValidator{}
	pattern := validator.identifyPattern(t)

	executor := &handlerExecutor{
		value:    v,
		pattern:  pattern,
		funcType: t,
	}

	return HandlerFunc(func(ctx *Context) error {
		return executor.execute(ctx)
	})
}

// execute runs the handler function based on its identified pattern.
//
// Parameters:
//   - ctx: The context for the request
//
// Returns:
//   - An error if the execution fails
func (e *handlerExecutor) execute(ctx *Context) error {
	callArgs, err := e.prepareArgs(ctx)
	if err != nil {
		return err
	}

	results := e.value.Call(callArgs)
	return e.handleResults(ctx, results)
}

// prepareArgs prepares the arguments for the function call based on the pattern.
//
// Parameters:
//   - ctx: The context for the request
//
// Returns:
//   - The arguments for the function call
//   - An error if the preparation fails
func (e *handlerExecutor) prepareArgs(ctx *Context) ([]reflect.Value, error) {
	switch e.pattern {
	case patternContextError, patternContextResponse:
		return []reflect.Value{reflect.ValueOf(ctx)}, nil

	case patternSimpleError, patternSimpleResponse:
		return []reflect.Value{}, nil

	case patternModelError, patternModelResponse:
		return e.prepareModelArgs(ctx)

	default:
		return nil, fmt.Errorf("unsupported handler pattern during execution")
	}
}

// prepareModelArgs prepares arguments for model-based handlers.
//
// Parameters:
//   - ctx: The context for the request
//
// Returns:
//   - The arguments for the function call
//   - An error if the preparation fails
func (e *handlerExecutor) prepareModelArgs(ctx *Context) ([]reflect.Value, error) {
	requestType := e.funcType.In(0)
	requestValue := reflect.New(requestType).Interface()

	if err := ctx.ParseRequest(requestValue); err != nil {
		return nil, err
	}

	return []reflect.Value{reflect.ValueOf(requestValue).Elem()}, nil
}

// handleResults processes the return values from the handler function.
//
// Parameters:
//   - ctx: The context for the request
//   - results: The return values from the handler function
//
// Returns:
//   - An error if the result handling fails
func (e *handlerExecutor) handleResults(ctx *Context, results []reflect.Value) error {
	switch len(results) {
	case 1:
		return e.handleSingleResult(results[0])
	case 2:
		return e.handleDoubleResult(ctx, results[0], results[1])
	default:
		return fmt.Errorf("unexpected number of return values: %d", len(results))
	}
}

// handleSingleResult handles functions that return only an error.
//
// Parameters:
//   - result: The return value from the handler function
//
// Returns:
//   - An error if the result handling fails
func (e *handlerExecutor) handleSingleResult(result reflect.Value) error {
	if result.IsNil() {
		return nil
	}

	if err, ok := result.Interface().(error); ok {
		return err
	}

	return fmt.Errorf("handler returned non-error value: %v", result.Interface())
}

// handleDoubleResult handles functions that return (value, error).
//
// Parameters:
//   - ctx: The context for the request
//   - valueResult: The value returned by the handler function
//   - errorResult: The error returned by the handler function
//
// Returns:
//   - An error if the result handling fails
func (e *handlerExecutor) handleDoubleResult(ctx *Context, valueResult, errorResult reflect.Value) error {
	if !errorResult.IsNil() {
		if err, ok := errorResult.Interface().(error); ok {
			return err
		}
		return fmt.Errorf("handler returned non-error value in error position: %v", errorResult.Interface())
	}

	responseValue := valueResult.Interface()
	if err := ctx.JSON(responseValue); err != nil {
		return fmt.Errorf("failed to send JSON response: %w", err)
	}
	return nil
}

// Helper functions for type checking.
// These functions are used to validate the types of handler functions.

func isContextType(t reflect.Type) bool {
	// Check if it's a pointer to Context
	if t.Kind() != reflect.Ptr {
		return false
	}
	elem := t.Elem()
	return elem.Name() == "Context" && elem.PkgPath() == "github.com/pay-theory/lift/pkg/lift"
}

func isErrorType(t reflect.Type) bool {
	// Check if it implements the error interface
	errorInterface := reflect.TypeOf((*error)(nil)).Elem()
	return t.Implements(errorInterface)
}

func isInterfaceType(_ reflect.Type) bool {
	// Accept any type for response values (any)
	return true
}

// parseTriggerType converts a string to a TriggerType.
//
// Parameters:
//   - s: The string to convert
//
// Returns:
//   - The TriggerType
func parseTriggerType(s string) TriggerType {
	switch s {
	case "SQS":
		return TriggerSQS
	case "S3":
		return TriggerS3
	case "EventBridge":
		return TriggerEventBridge
	case "CONNECT", "DISCONNECT", "MESSAGE":
		return TriggerWebSocket
	default:
		// Check if it's an HTTP method
		if s == "GET" || s == "POST" || s == "PUT" || s == "DELETE" || s == "PATCH" || s == "HEAD" || s == "OPTIONS" {
			return TriggerAPIGateway
		}
		return TriggerUnknown
	}
}

// RunLocalTest runs local testing logic when not in Lambda environment.
// It is used to run tests locally without deploying to AWS Lambda.
func (a *App) RunLocalTest() {
	if a.IsLambda() {
		return
	}

	// Load test event from file
	testFile := os.Getenv("TEST_FILE")
	if testFile == "" {
		if a.logger != nil {
			a.logger.Info("No test file defined", nil)
		}
		return
	}

	// Validate file path to prevent directory traversal
	testFile = filepath.Clean(testFile)
	if strings.Contains(testFile, "..") {
		if a.logger != nil {
			a.logger.Error("Invalid test file path", map[string]any{
				"path": testFile,
			})
		}
		return
	}

	eventData, err := os.ReadFile(testFile)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("Error reading test event file", map[string]any{
				"error": err,
			})
		}
		return
	}

	// First unmarshal into a generic map to determine the event type
	var rawEvent map[string]any
	if err := json.Unmarshal(eventData, &rawEvent); err != nil {
		if a.logger != nil {
			a.logger.Error("Error unmarshaling test event", map[string]any{
				"error": err,
			})
		}
		return
	}

	// Create context with timeout for local testing
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Run the test event locally
	if _, err := a.HandleRequest(ctx, rawEvent); err != nil {
		// Log generic debug message without echoing user input.
		if a.logger != nil {
			a.logger.Debug("Error handling request during local test")
		} else {
			fmt.Println("Debug: Error handling request during local test")
		}
	}
}

// WithDebug enables debug mode for the application.
//
// Returns:
//   - An AppOption function that enables debug mode
func WithDebug() AppOption {
	return func(app *App) {
		app.config.Debug = true
		// Also set log level to DEBUG if using the default logger
		if app.config.LogLevel == "INFO" {
			app.config.LogLevel = "DEBUG"
		}
	}
}

// WithConfig sets a custom configuration for the application.
//
// Parameters:
//   - config: A pointer to the Config struct
//
// Returns:
//   - An AppOption function that sets the configuration
func WithConfig(config *Config) AppOption {
	return func(app *App) {
		app.config = config
	}
}
