package lift

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// ExampleRequest represents an example request
type ExampleRequest struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"min=0,max=150"`
}

// ExampleResponse represents an example response
type ExampleResponse struct {
	Message string `json:"message"`
	UserID  string `json:"user_id,omitempty"`
}

// ExampleHandler is an example handler
func ExampleHandler(ctx *Context) error {
	var req ExampleRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	// Get user ID from context
	userID := ctx.UserID()

	// Create response
	resp := ExampleResponse{
		Message: "Hello, " + req.Name,
	}

	// Add user ID if available
	if userID != "" {
		resp.UserID = userID
	}

	return ctx.JSON(resp)
}

// ExampleTypeSafeHandler is an example type-safe handler
func ExampleTypeSafeHandler(ctx *Context, req ExampleRequest) (ExampleResponse, error) {
	// Get user ID from context
	userID := ctx.UserID()

	// Create response
	resp := ExampleResponse{
		Message: "Hello, " + req.Name,
	}

	// Add user ID if available
	if userID != "" {
		resp.UserID = userID
	}

	return resp, nil
}

// ExampleMain is an example main function
func ExampleMain() {
	// Create Lift application
	app := New(WithDebug(true))

	// Add middleware
	app.Use(RequestLogger())
	app.Use(RequestID())
	app.Use(Recover())

	// Add JWT middleware if secret is provided
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret != "" {
		app.Use(JWT(JWTConfig{
			SecretKey:       jwtSecret,
			RequireTenantID: true,
		}))
	}

	// Add tenant middleware
	app.Use(TenantMiddleware(DefaultTenantConfig()))

	// Add routes
	app.GET("/health", HandlerFunc(func(ctx *Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	}))

	app.POST("/example", HandlerFunc(ExampleHandler))
	app.POST("/example-typed", SimpleHandler(ExampleTypeSafeHandler))

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}

// ExampleSQSHandler is an example SQS handler
func ExampleSQSHandler(ctx context.Context, event events.SQSEvent) error {
	// Process SQS event
	for _, record := range event.Records {
		// Process record
		_ = record.Body
	}

	return nil
}

// ExampleS3Handler is an example S3 handler
func ExampleS3Handler(ctx context.Context, event events.S3Event) error {
	// Process S3 event
	for _, record := range event.Records {
		// Process record
		_ = record.S3.Object.Key
	}

	return nil
}
