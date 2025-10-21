package middleware_test

import (
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example demonstrating JWT authentication configuration.
func ExampleJWTAuth() {
	app := lift.New()

	// Protect all routes under /api with JWT
	api := app.Group("/api")
	api.Use(middleware.JWTAuth(middleware.JWTConfig{
		Secret:      "test-secret",
		Algorithm:   "HS256",
		TokenLookup: "header:Authorization",
		SkipPaths:   []string{"/api/public"},
	}))

	// Register protected routes under /api
	_ = api.GET("/profile", func(ctx *lift.Context) error { return ctx.Text("ok") })
}
