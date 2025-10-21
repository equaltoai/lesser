package lift_test

import (
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example demonstrating a minimal Lift app with common middleware and a typed handler.
func Example_app() {
	app := lift.New()

	// Recommended global middleware stack
	app.Use(middleware.RequestID())
	app.Use(middleware.Logger())
	app.Use(middleware.Recover())
	app.Use(middleware.ErrorHandler())

	// JWT protection (optional)
	app.Use(middleware.JWTAuth(middleware.JWTConfig{Secret: "test-secret"}))

	// Typed handler with automatic parsing/validation
	type CreateUserRequest struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	type User struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (User, error) {
		// business logic
		return User{ID: "u_123", Name: req.Name, Email: req.Email}, nil
	}))

	// In Lambda: lambda.Start(app.HandleRequest)
}

// Example demonstrating simple non-HTTP event handlers.
func ExampleApp_events() {
	app := lift.New()

	// SQS handler pattern
	_ = app.Handle("SQS", "my-queue", func(ctx *lift.Context) error {
		// parse and process message from ctx.Request.Body
		return nil
	})

	// EventBridge handler pattern
	_ = app.Handle("EventBridge", "my.app.source", func(ctx *lift.Context) error {
		// inspect ctx.Request.Detail for event fields
		return nil
	})
}
