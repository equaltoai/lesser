package main

import (
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
)

func main() {
	app := lift.New()

	// Add basic logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			log.Printf("Request: %s %s", ctx.Request.Method, ctx.Request.Path)
			return next.Handle(ctx)
		})
	})

	// Handle EventBridge scheduled events (like cron jobs)
	if err := app.EventBridge("scheduled-wakeup", func(ctx *lift.Context) error {
		log.Println("Wakeup event received!")

		// Get event details
		event := ctx.Request.RawEvent
		log.Printf("Event details: %+v", event)

		// Perform wakeup tasks
		// For example: warm up connections, pre-load caches, etc.
		if err := performWakeupTasks(ctx); err != nil {
			return lift.NewLiftError("WAKEUP_FAILED", "Failed to perform wakeup tasks", 500).WithCause(err)
		}

		return ctx.JSON(map[string]string{
			"status":  "success",
			"message": "Wakeup completed successfully",
		})
	}); err != nil {
		log.Fatalf("Failed to register EventBridge handler: %v", err)
	}

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}

func performWakeupTasks(_ *lift.Context) error {
	// Example wakeup tasks:
	// 1. Test database connectivity
	// 2. Warm up connection pools
	// 3. Pre-load frequently accessed data
	// 4. Clear expired caches

	log.Println("Performing wakeup tasks...")

	// Simulate some work
	if os.Getenv("ENVIRONMENT") == "production" {
		// Production-specific warmup
		log.Println("Production environment warmup")
	}

	return nil
}
