package middleware_test

import (
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example demonstrating rate limiting with the Limited-backed middleware.
func ExampleUserRateLimitWithLimited() {
	app := lift.New()

	// Create limiter. In production, ensure AWS_REGION is set.
	limiter, _ := middleware.UserRateLimitWithLimited(100, 15*time.Minute)
	if limiter != nil {
		app.Use(limiter)
	}

	_ = app.GET("/data", func(ctx *lift.Context) error { return ctx.Text("ok") })
}
