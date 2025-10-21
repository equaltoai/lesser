package middleware_test

import (
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example of input validation middleware using the default secure config.
func ExampleInputValidation() {
	app := lift.New()

	// Apply comprehensive request validation (headers, content-type, sizes, path/query params, basic XSS/SQLi checks).
	app.Use(middleware.InputValidation(middleware.DefaultValidationConfig()))

	_ = app.POST("/submit", func(ctx *lift.Context) error {
		// If we got here, request passed validation.
		return ctx.Text("ok")
	})
}
