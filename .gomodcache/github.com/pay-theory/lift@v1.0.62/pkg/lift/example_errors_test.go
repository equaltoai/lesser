package lift_test

import (
	"github.com/pay-theory/lift/pkg/lift"
)

// Example demonstrating structured error responses.
func Example_errors() {
	app := lift.New()

	_ = app.GET("/missing", func(ctx *lift.Context) error {
		return lift.NotFound("resource not found")
	})

	_ = app.GET("/validate", func(ctx *lift.Context) error {
		return lift.ValidationError("invalid input")
	})

	_ = app.GET("/forbidden", func(ctx *lift.Context) error {
		return lift.AuthorizationError("insufficient permissions")
	})
}
