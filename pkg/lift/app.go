package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// App represents a Lift application
type App struct {
	middlewares []Middleware
	routes      map[string]map[string]Handler
	sqsHandlers map[string]SQSHandler
	s3Handlers  map[string]S3Handler
	debug       bool
}

// Handler is the interface that all request handlers must implement
type Handler interface {
	Handle(ctx *Context) error
}

// HandlerFunc is a function that implements the Handler interface
type HandlerFunc func(ctx *Context) error

// Handle calls the handler function
func (f HandlerFunc) Handle(ctx *Context) error {
	return f(ctx)
}

// SQSHandler is a function that handles SQS events
type SQSHandler func(ctx context.Context, event events.SQSEvent) error

// S3Handler is a function that handles S3 events
type S3Handler func(ctx context.Context, event events.S3Event) error

// New creates a new Lift application
func New(options ...Option) *App {
	app := &App{
		middlewares: []Middleware{},
		routes:      make(map[string]map[string]Handler),
		sqsHandlers: make(map[string]SQSHandler),
		s3Handlers:  make(map[string]S3Handler),
	}

	// Apply options
	for _, option := range options {
		option(app)
	}

	return app
}

// Option is a function that configures an App
type Option func(*App)

// WithDebug enables debug mode
func WithDebug(debug bool) Option {
	return func(app *App) {
		app.debug = debug
	}
}

// Use adds middleware to the application
func (app *App) Use(middleware Middleware) {
	app.middlewares = append(app.middlewares, middleware)
}

// GET registers a handler for GET requests
func (app *App) GET(path string, handler Handler) {
	app.addRoute("GET", path, handler)
}

// POST registers a handler for POST requests
func (app *App) POST(path string, handler Handler) {
	app.addRoute("POST", path, handler)
}

// PUT registers a handler for PUT requests
func (app *App) PUT(path string, handler Handler) {
	app.addRoute("PUT", path, handler)
}

// DELETE registers a handler for DELETE requests
func (app *App) DELETE(path string, handler Handler) {
	app.addRoute("DELETE", path, handler)
}

// PATCH registers a handler for PATCH requests
func (app *App) PATCH(path string, handler Handler) {
	app.addRoute("PATCH", path, handler)
}

// OPTIONS registers a handler for OPTIONS requests
func (app *App) OPTIONS(path string, handler Handler) {
	app.addRoute("OPTIONS", path, handler)
}

// SQS registers a handler for SQS events
func (app *App) SQS(queueName string, handler SQSHandler) {
	app.sqsHandlers[queueName] = handler
}

// S3 registers a handler for S3 events
func (app *App) S3(bucketName string, handler S3Handler) {
	app.s3Handlers[bucketName] = handler
}

// addRoute adds a route to the application
func (app *App) addRoute(method, path string, handler Handler) {
	if app.routes[method] == nil {
		app.routes[method] = make(map[string]Handler)
	}
	app.routes[method][path] = handler
}

// HandleRequest is the entry point for AWS Lambda
func (app *App) HandleRequest(ctx context.Context, event interface{}) (interface{}, error) {
	// Log request if debug mode is enabled
	if app.debug {
		eventJSON, _ := json.MarshalIndent(event, "", "  ")
		log.Printf("Received event: %s", string(eventJSON))
	}

	// Get Lambda context information
	var requestID string
	var deadline time.Time
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		requestID = lc.AwsRequestID
		deadline = lambdacontext.Deadline(ctx)
	}

	// Handle different event types
	switch e := event.(type) {
	case events.APIGatewayProxyRequest:
		return app.handleAPIGateway(ctx, e, requestID, deadline)
	case events.APIGatewayV2HTTPRequest:
		return app.handleAPIGatewayV2(ctx, e, requestID, deadline)
	case events.SQSEvent:
		return nil, app.handleSQS(ctx, e)
	case events.S3Event:
		return nil, app.handleS3(ctx, e)
	default:
		return nil, fmt.Errorf("unsupported event type: %T", event)
	}
}

// handleAPIGateway handles API Gateway proxy events
func (app *App) handleAPIGateway(ctx context.Context, event events.APIGatewayProxyRequest, requestID string, deadline time.Time) (events.APIGatewayProxyResponse, error) {
	// Create context
	liftCtx := NewContext(ctx, event, requestID, deadline)

	// Find handler
	handler, ok := app.findHandler(event.HTTPMethod, event.Path)
	if !ok {
		return events.APIGatewayProxyResponse{
			StatusCode: 404,
			Body:       `{"error":"Not Found"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	// Apply middleware
	for i := len(app.middlewares) - 1; i >= 0; i-- {
		handler = app.middlewares[i](handler)
	}

	// Handle request
	err := handler.Handle(liftCtx)
	if err != nil {
		return app.handleError(err)
	}

	// Return response
	return liftCtx.Response, nil
}

// handleAPIGatewayV2 handles API Gateway v2 HTTP events
func (app *App) handleAPIGatewayV2(ctx context.Context, event events.APIGatewayV2HTTPRequest, requestID string, deadline time.Time) (events.APIGatewayV2HTTPResponse, error) {
	// Convert to v1 event for now (we'll implement native v2 support later)
	v1Event := events.APIGatewayProxyRequest{
		Resource:                        event.RouteKey,
		Path:                            event.RequestContext.HTTP.Path,
		HTTPMethod:                      event.RequestContext.HTTP.Method,
		Headers:                         event.Headers,
		MultiValueHeaders:               make(map[string][]string),
		QueryStringParameters:           event.QueryStringParameters,
		MultiValueQueryStringParameters: make(map[string][]string),
		PathParameters:                  event.PathParameters,
		StageVariables:                  event.StageVariables,
		Body:                            event.Body,
		IsBase64Encoded:                 event.IsBase64Encoded,
		RequestContext: events.APIGatewayProxyRequestContext{
			AccountID:  event.RequestContext.AccountID,
			RequestID:  event.RequestContext.RequestID,
			Stage:      event.RequestContext.Stage,
			Identity:   events.APIGatewayRequestIdentity{},
			ResourceID: event.RouteKey,
			Path:       event.RequestContext.HTTP.Path,
		},
	}

	// Handle as v1 event
	v1Response, err := app.handleAPIGateway(ctx, v1Event, requestID, deadline)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}

	// Convert to v2 response
	v2Response := events.APIGatewayV2HTTPResponse{
		StatusCode:      v1Response.StatusCode,
		Headers:         v1Response.Headers,
		Body:            v1Response.Body,
		IsBase64Encoded: v1Response.IsBase64Encoded,
		Cookies:         []string{},
	}

	return v2Response, nil
}

// handleSQS handles SQS events
func (app *App) handleSQS(ctx context.Context, event events.SQSEvent) error {
	// Extract queue name from ARN
	if len(event.Records) == 0 {
		return nil // No records to process
	}

	// Get queue name from the first record
	queueARN := event.Records[0].EventSourceARN
	queueName := extractQueueName(queueARN)

	// Find handler
	handler, ok := app.sqsHandlers[queueName]
	if !ok {
		// Try with wildcard handler
		handler, ok = app.sqsHandlers["*"]
		if !ok {
			return fmt.Errorf("no handler registered for queue: %s", queueName)
		}
	}

	// Handle event
	return handler(ctx, event)
}

// handleS3 handles S3 events
func (app *App) handleS3(ctx context.Context, event events.S3Event) error {
	// Extract bucket name
	if len(event.Records) == 0 {
		return nil // No records to process
	}

	// Get bucket name from the first record
	bucketName := event.Records[0].S3.Bucket.Name

	// Find handler
	handler, ok := app.s3Handlers[bucketName]
	if !ok {
		// Try with wildcard handler
		handler, ok = app.s3Handlers["*"]
		if !ok {
			return fmt.Errorf("no handler registered for bucket: %s", bucketName)
		}
	}

	// Handle event
	return handler(ctx, event)
}

// findHandler finds a handler for the given method and path
func (app *App) findHandler(method, path string) (Handler, bool) {
	if routes, ok := app.routes[method]; ok {
		if handler, ok := routes[path]; ok {
			return handler, true
		}
	}
	return nil, false
}

// handleError converts an error to an API Gateway response
func (app *App) handleError(err error) (events.APIGatewayProxyResponse, error) {
	// Check if it's a Lift error
	if liftErr, ok := err.(*Error); ok {
		return events.APIGatewayProxyResponse{
			StatusCode: liftErr.StatusCode,
			Body:       liftErr.JSON(),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	// Default error response
	statusCode := 500
	errorMessage := "Internal Server Error"

	if app.debug {
		errorMessage = err.Error()
	}

	errorResponse := map[string]string{
		"error": errorMessage,
	}

	responseBody, _ := json.Marshal(errorResponse)

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Body:       string(responseBody),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}, nil
}

// extractQueueName extracts the queue name from an ARN
func extractQueueName(arn string) string {
	// ARN format: arn:aws:sqs:region:account-id:queue-name
	// We just want the last part
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == ':' {
			return arn[i+1:]
		}
	}
	return arn
}

// SimpleHandler is a helper for creating type-safe handlers
func SimpleHandler[Req any, Resp any](handler func(ctx *Context, req Req) (Resp, error)) Handler {
	return HandlerFunc(func(ctx *Context) error {
		var req Req
		if err := ctx.ParseRequest(&req); err != nil {
			return err
		}

		resp, err := handler(ctx, req)
		if err != nil {
			return err
		}

		return ctx.JSON(resp)
	})
}
